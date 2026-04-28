package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Colvin-Y/kubernetes-ontology/internal/appconfig"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
	"github.com/Colvin-Y/kubernetes-ontology/internal/runtime"
	ontologyserver "github.com/Colvin-Y/kubernetes-ontology/internal/server"
)

func main() {
	var configPath string
	var kubeconfig string
	var cluster string
	var contextNamespaces string
	var workloadResourcesRaw string
	var controllerRulesRaw string
	var csiComponentRulesRaw string
	var addr string
	var bootstrapTimeout time.Duration
	var pollInterval time.Duration
	var streamMode string
	var disablePolling bool

	flag.StringVar(&configPath, "config", "", "Path to YAML config file")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file. If empty, in-cluster config is used.")
	flag.StringVar(&cluster, "cluster", "default-cluster", "Logical cluster name for canonical IDs")
	flag.StringVar(&contextNamespaces, "context-namespaces", "", "Comma-separated namespaces to collect as ontology context. Empty means all namespaces.")
	flag.StringVar(&contextNamespaces, "namespaces", "", "Alias for --context-namespaces")
	flag.StringVar(&workloadResourcesRaw, "workload-resources", "", "Comma-separated custom workload resources as group/version/resource/kind[/scope], e.g. apps.kruise.io/v1alpha1/advancedstatefulsets/AdvancedStatefulSet")
	flag.StringVar(&controllerRulesRaw, "controller-rules", "", "Comma-separated workload controller display rules as apiVersion=...;kind=...;namespace=...;controller=prefix;daemon=prefix")
	flag.StringVar(&csiComponentRulesRaw, "csi-component-rules", "", "Comma-separated CSI component rules as driver=...;namespace=...;controller=prefix;agent=prefix")
	flag.StringVar(&addr, "addr", "127.0.0.1:18080", "HTTP listen address")
	flag.DurationVar(&bootstrapTimeout, "bootstrap-timeout", 2*time.Minute, "Timeout for initial full snapshot bootstrap")
	flag.DurationVar(&pollInterval, "poll-interval", 10*time.Second, "Polling interval for continuous graph refresh")
	flag.StringVar(&streamMode, "stream-mode", string(collectk8s.StreamModeInformer), "Continuous update stream mode: informer or polling")
	flag.BoolVar(&disablePolling, "disable-polling", false, "Start the API after bootstrap without running a continuous stream")
	flag.Parse()

	setFlags := explicitFlags()
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(2)
	}
	if err := applyConfigDefaults(cfg, setFlags, &kubeconfig, &cluster, &contextNamespaces, &addr, &streamMode, &bootstrapTimeout, &pollInterval); err != nil {
		fmt.Fprintf(os.Stderr, "apply config: %v\n", err)
		os.Exit(2)
	}
	parsedStreamMode, err := collectk8s.ParseStreamMode(streamMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse stream mode: %v\n", err)
		os.Exit(2)
	}

	config, err := kubernetesConfig(kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build kubernetes config: %v\n", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build kubernetes client: %v\n", err)
		os.Exit(1)
	}
	workloadResources, err := workloadResourcesFromConfig(cfg, setFlags, workloadResourcesRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse workload resources: %v\n", err)
		os.Exit(2)
	}
	controllerRules, err := controllerRulesFromConfig(cfg, setFlags, controllerRulesRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse controller rules: %v\n", err)
		os.Exit(2)
	}
	csiComponentRules, err := csiComponentRulesFromConfig(cfg, setFlags, csiComponentRulesRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse csi component rules: %v\n", err)
		os.Exit(2)
	}
	var dynamicClient dynamic.Interface
	if len(workloadResources) > 0 {
		dynamicClient, err = dynamic.NewForConfig(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build dynamic kubernetes client: %v\n", err)
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	collectorNamespaces := splitCSV(contextNamespaces)
	collector := collectk8s.NewReadOnlyCollectorWithOptions(clientset, cluster, collectk8s.CollectorOptions{
		ContextNamespaces: collectorNamespaces,
		DynamicClient:     dynamicClient,
		WorkloadResources: workloadResources,
	})
	manager := runtime.NewManagerWithOptions(cluster, collector, runtime.ManagerOptions{WorkloadControllerRules: controllerRules, CSIComponentRules: csiComponentRules})
	bootstrapCtx, bootstrapCancel := context.WithTimeout(ctx, bootstrapTimeout)
	if err := manager.Start(bootstrapCtx); err != nil {
		bootstrapCancel()
		fmt.Fprintf(os.Stderr, "bootstrap runtime: %v\n", err)
		os.Exit(1)
	}
	bootstrapCancel()

	if !disablePolling {
		go func() {
			if err := runContinuousStream(ctx, manager, continuousStreamOptions{
				mode:              parsedStreamMode,
				collector:         collector,
				clientset:         clientset,
				dynamicClient:     dynamicClient,
				contextNamespaces: collectorNamespaces,
				workloadResources: workloadResources,
				pollInterval:      pollInterval,
			}); err != nil && ctx.Err() == nil && !errors.Is(err, context.Canceled) {
				log.Printf("%s stream stopped: %v", parsedStreamMode, err)
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           ontologyserver.NewHandler(manager),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("kubernetes-ontologyd listening on %s", addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "serve http: %v\n", err)
			os.Exit(1)
		}
	}
}

func explicitFlags() map[string]bool {
	out := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		out[f.Name] = true
	})
	return out
}

func applyConfigDefaults(cfg appconfig.Config, setFlags map[string]bool, kubeconfig, cluster, contextNamespaces, addr, streamMode *string, bootstrapTimeout, pollInterval *time.Duration) error {
	if cfg.Kubeconfig != "" && !setFlags["kubeconfig"] {
		*kubeconfig = cfg.Kubeconfig
	}
	if cfg.Cluster != "" && !setFlags["cluster"] {
		*cluster = cfg.Cluster
	}
	if len(cfg.ContextNamespaces) > 0 && !setFlags["context-namespaces"] && !setFlags["namespaces"] {
		*contextNamespaces = strings.Join(cfg.ContextNamespaces, ",")
	}
	if cfg.Server.Addr != "" && !setFlags["addr"] {
		*addr = cfg.Server.Addr
	}
	if cfg.StreamMode != "" && !setFlags["stream-mode"] {
		*streamMode = cfg.StreamMode
	}
	if cfg.BootstrapTimeout != "" && !setFlags["bootstrap-timeout"] {
		value, err := time.ParseDuration(cfg.BootstrapTimeout)
		if err != nil {
			return fmt.Errorf("bootstrapTimeout: %w", err)
		}
		*bootstrapTimeout = value
	}
	if cfg.PollInterval != "" && !setFlags["poll-interval"] {
		value, err := time.ParseDuration(cfg.PollInterval)
		if err != nil {
			return fmt.Errorf("pollInterval: %w", err)
		}
		*pollInterval = value
	}
	return nil
}

func workloadResourcesFromConfig(cfg appconfig.Config, setFlags map[string]bool, raw string) ([]collectk8s.WorkloadResource, error) {
	if len(cfg.WorkloadResources) > 0 && !setFlags["workload-resources"] {
		return cfg.WorkloadResources, nil
	}
	return collectk8s.ParseWorkloadResources(raw)
}

func controllerRulesFromConfig(cfg appconfig.Config, setFlags map[string]bool, raw string) ([]infer.WorkloadControllerRule, error) {
	if len(cfg.ControllerRules) > 0 && !setFlags["controller-rules"] {
		return cfg.ControllerRules, nil
	}
	return infer.ParseWorkloadControllerRules(raw)
}

func csiComponentRulesFromConfig(cfg appconfig.Config, setFlags map[string]bool, raw string) ([]infer.CSIComponentRule, error) {
	if len(cfg.CSIComponentRules) > 0 && !setFlags["csi-component-rules"] {
		return infer.EffectiveCSIComponentRules(cfg.CSIComponentRules), nil
	}
	rules, err := infer.ParseCSIComponentRules(raw)
	if err != nil {
		return nil, err
	}
	return infer.EffectiveCSIComponentRules(rules), nil
}

type continuousStreamOptions struct {
	mode              collectk8s.StreamMode
	collector         collectk8s.Collector
	clientset         kubernetes.Interface
	dynamicClient     dynamic.Interface
	contextNamespaces []string
	workloadResources []collectk8s.WorkloadResource
	pollInterval      time.Duration
}

func runContinuousStream(ctx context.Context, manager *runtime.Manager, options continuousStreamOptions) error {
	if options.mode == collectk8s.StreamModePolling {
		return manager.RunStream(ctx, collectk8s.NewPollingStream(options.collector, options.pollInterval))
	}
	err := manager.RunStream(ctx, collectk8s.NewInformerStream(options.clientset, collectk8s.InformerStreamOptions{
		ContextNamespaces: options.contextNamespaces,
		DynamicClient:     options.dynamicClient,
		WorkloadResources: options.workloadResources,
	}))
	if err == nil || ctx.Err() != nil {
		return err
	}
	log.Printf("informer stream stopped: %v; falling back to polling", err)
	return manager.RunStream(ctx, collectk8s.NewPollingStream(options.collector, options.pollInterval))
}

func kubernetesConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
