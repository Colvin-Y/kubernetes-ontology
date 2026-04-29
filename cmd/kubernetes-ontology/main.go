package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/appconfig"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
	"github.com/Colvin-Y/kubernetes-ontology/internal/runtime"
)

func main() {
	var configPath string
	var kubeconfig string
	var cluster string
	var entryKind string
	var namespace string
	var name string
	var contextNamespaces string
	var workloadResourcesRaw string
	var controllerRulesRaw string
	var csiComponentRulesRaw string
	var terminalKindsRaw string
	var maxDepth int
	var storageMaxDepth int
	var maxNodes int
	var maxEdges int
	var expandTerminalNodes bool
	var bootstrapTimeout time.Duration
	var statusOnly bool
	var diagnosePod bool
	var diagnoseWorkload bool
	var observeDuration time.Duration
	var pollInterval time.Duration
	var streamMode string
	var server string
	var machineErrors bool
	var getEntity bool
	var listEntities bool
	var listRelations bool
	var neighbors bool
	var expandNode bool
	var collapseNode bool
	var expandDepth int
	var graphFile string
	var entityID string
	var entityKind string
	var relationKind string
	var fromID string
	var toID string
	var direction string
	var limit int

	flag.StringVar(&configPath, "config", "", "Path to YAML config file")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	flag.StringVar(&cluster, "cluster", "default-cluster", "Logical cluster name for canonical IDs")
	flag.StringVar(&entryKind, "entry-kind", "", "Diagnostic entry kind: Pod, Workload, PVC, PV, StorageClass, CSIDriver, or another ontology node kind")
	flag.StringVar(&namespace, "namespace", "", "Namespace of diagnostic entry")
	flag.StringVar(&name, "name", "", "Name of diagnostic entry")
	flag.StringVar(&contextNamespaces, "context-namespaces", "", "Comma-separated namespaces to collect as ontology context. Empty means all namespaces.")
	flag.StringVar(&contextNamespaces, "namespaces", "", "Alias for --context-namespaces")
	flag.StringVar(&workloadResourcesRaw, "workload-resources", "", "Comma-separated custom workload resources as group/version/resource/kind[/scope], e.g. apps.kruise.io/v1alpha1/advancedstatefulsets/AdvancedStatefulSet")
	flag.StringVar(&controllerRulesRaw, "controller-rules", "", "Comma-separated workload controller display rules as apiVersion=...;kind=...;namespace=...;controller=prefix;daemon=prefix")
	flag.StringVar(&csiComponentRulesRaw, "csi-component-rules", "", "Comma-separated CSI component rules as driver=...;namespace=...;controller=prefix;agent=prefix")
	flag.StringVar(&terminalKindsRaw, "terminal-kinds", "", "Comma-separated diagnostic terminal node kinds. Empty uses defaults; 'none' disables terminal boundaries.")
	flag.IntVar(&maxDepth, "max-depth", 2, "Maximum general BFS depth for diagnostic subgraph traversal")
	flag.IntVar(&storageMaxDepth, "storage-max-depth", 5, "Maximum BFS depth for storage and CSI related traversal")
	flag.IntVar(&maxNodes, "max-nodes", 0, "Maximum diagnostic nodes to return. Empty uses the built-in safe default.")
	flag.IntVar(&maxEdges, "max-edges", 0, "Maximum diagnostic edges to return. Empty uses the built-in safe default.")
	flag.BoolVar(&expandTerminalNodes, "expand-terminal-nodes", false, "Traverse through diagnostic terminal nodes instead of stopping at them")
	flag.DurationVar(&bootstrapTimeout, "bootstrap-timeout", 2*time.Minute, "Timeout for initial full snapshot bootstrap")
	flag.BoolVar(&statusOnly, "status-only", false, "Bootstrap runtime and print runtime status instead of querying a diagnostic subgraph")
	flag.BoolVar(&statusOnly, "status", false, "Alias for --status-only")
	flag.BoolVar(&diagnosePod, "diagnose-pod", false, "Diagnose a Pod by --namespace and --name")
	flag.BoolVar(&diagnoseWorkload, "diagnose-workload", false, "Diagnose a Workload by --namespace and --name")
	flag.DurationVar(&observeDuration, "observe-duration", 0, "Keep observing the cluster for this duration before printing status or query output")
	flag.DurationVar(&pollInterval, "poll-interval", 10*time.Second, "Polling interval used with --observe-duration")
	flag.StringVar(&streamMode, "stream-mode", string(collectk8s.StreamModePolling), "Continuous update stream mode for --observe-duration: informer or polling")
	flag.StringVar(&server, "server", "", "HTTP server URL for an existing kubernetes-ontologyd instance")
	flag.BoolVar(&machineErrors, "machine-errors", false, "Print server query errors as JSON to stderr")
	flag.BoolVar(&getEntity, "get-entity", false, "Query one entity from --server by --entity-id or --entity-kind/--namespace/--name")
	flag.BoolVar(&getEntity, "resolve-entity", false, "Alias for --get-entity")
	flag.BoolVar(&listEntities, "list-entities", false, "List ontology entities from --server")
	flag.BoolVar(&listRelations, "list-relations", false, "List ontology relations from --server")
	flag.BoolVar(&listRelations, "list-filtered-relations", false, "Alias for --list-relations")
	flag.BoolVar(&neighbors, "neighbors", false, "List relations touching --entity-id from --server")
	flag.BoolVar(&expandNode, "expand-node", false, "Expand a node into a bounded topology subgraph by --entity-id")
	flag.BoolVar(&expandNode, "expand-entity", false, "Alias for --expand-node")
	flag.BoolVar(&collapseNode, "collapse-node", false, "Collapse an expanded node from a viewer state JSON file by --entity-id")
	flag.IntVar(&expandDepth, "expand-depth", query.DefaultExpandDepth, "Depth used with --expand-node")
	flag.StringVar(&graphFile, "graph-file", "", "Graph JSON file used with --collapse-node")
	flag.StringVar(&entityID, "entity-id", "", "Entity global ID used by server-side ontology queries")
	flag.StringVar(&entityKind, "entity-kind", "", "Entity kind used by server-side ontology queries")
	flag.StringVar(&relationKind, "relation-kind", "", "Relation kind used by server-side ontology queries")
	flag.StringVar(&fromID, "from", "", "Relation source entity global ID")
	flag.StringVar(&toID, "to", "", "Relation target entity global ID")
	flag.StringVar(&direction, "direction", "", "Neighbor direction: in, out, or both")
	flag.IntVar(&limit, "limit", 0, "Maximum entities or relations returned by server-side ontology queries")
	flag.Parse()

	setFlags := explicitFlags()
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(2)
	}
	if err := applyConfigDefaults(cfg, setFlags, &kubeconfig, &cluster, &namespace, &contextNamespaces, &streamMode, &bootstrapTimeout, &observeDuration, &pollInterval, &maxDepth, &storageMaxDepth); err != nil {
		fmt.Fprintf(os.Stderr, "apply config: %v\n", err)
		os.Exit(2)
	}
	parsedStreamMode, err := collectk8s.ParseStreamMode(streamMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse stream mode: %v\n", err)
		os.Exit(2)
	}
	terminalNodeKinds, terminalKindsDisable, err := query.ParseTerminalNodeKinds(terminalKindsRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse terminal kinds: %v\n", err)
		os.Exit(2)
	}
	if terminalKindsDisable {
		expandTerminalNodes = true
	}
	if diagnosePod && diagnoseWorkload {
		err := fmt.Errorf("only one of --diagnose-pod or --diagnose-workload may be set")
		if machineErrors {
			writeMachineError(os.Stderr, err)
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(2)
	}
	if diagnosePod {
		entryKind = "Pod"
	}
	if diagnoseWorkload {
		entryKind = "Workload"
	}

	if collapseNode {
		if err := collapseGraphFile(graphFile, entityID); err != nil {
			fmt.Fprintf(os.Stderr, "collapse node: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if server != "" {
		if err := queryServer(server, serverQueryOptions{
			statusOnly:          statusOnly,
			entryKind:           entryKind,
			namespace:           namespace,
			name:                name,
			maxDepth:            maxDepth,
			storageMaxDepth:     storageMaxDepth,
			maxNodes:            maxNodes,
			maxEdges:            maxEdges,
			terminalNodeKinds:   terminalNodeKinds,
			expandTerminalNodes: expandTerminalNodes,
			getEntity:           getEntity,
			listEntities:        listEntities,
			listRelations:       listRelations,
			neighbors:           neighbors,
			expandNode:          expandNode,
			expandDepth:         expandDepth,
			entityID:            entityID,
			entityKind:          entityKind,
			relationKind:        relationKind,
			fromID:              fromID,
			toID:                toID,
			direction:           direction,
			limit:               limit,
		}); err != nil {
			if machineErrors {
				writeMachineError(os.Stderr, err)
			} else {
				fmt.Fprintf(os.Stderr, "query server: %v\n", err)
			}
			os.Exit(1)
		}
		return
	}

	if kubeconfig == "" || (!statusOnly && !expandNode && (entryKind == "" || name == "")) {
		fmt.Fprintln(os.Stderr, "usage: kubernetes-ontology [--config <path>] [--server <url> | --kubeconfig <path>] [--status-only] --entry-kind <Pod|Workload|PVC|PV|StorageClass|CSIDriver|...> [--namespace <ns>] --name <name>")
		os.Exit(2)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build kubeconfig: %v\n", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	collectorNamespaces := collectionNamespaces(contextNamespaces, namespace)
	collector := collectk8s.NewReadOnlyCollectorWithOptions(clientset, cluster, collectk8s.CollectorOptions{
		ContextNamespaces: collectorNamespaces,
		DynamicClient:     dynamicClient,
		WorkloadResources: workloadResources,
	})
	manager := runtime.NewManagerWithOptions(cluster, collector, runtime.ManagerOptions{WorkloadControllerRules: controllerRules, CSIComponentRules: csiComponentRules})
	if err := manager.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap runtime: %v\n", err)
		os.Exit(1)
	}

	if observeDuration > 0 {
		observeCtx, observeCancel := context.WithCancel(context.Background())
		observeTimer := time.AfterFunc(observeDuration, observeCancel)
		defer observeTimer.Stop()
		defer observeCancel()
		if err := runContinuousStream(observeCtx, manager, continuousStreamOptions{
			mode:              parsedStreamMode,
			collector:         collector,
			clientset:         clientset,
			dynamicClient:     dynamicClient,
			contextNamespaces: collectorNamespaces,
			workloadResources: workloadResources,
			pollInterval:      pollInterval,
			fallbackLogPrefix: "observe runtime",
		}); err != nil && observeCtx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "observe runtime: %v\n", err)
			os.Exit(1)
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if statusOnly {
		if err := enc.Encode(manager.RuntimeStatus()); err != nil {
			fmt.Fprintf(os.Stderr, "encode status: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if expandNode {
		if entityID == "" {
			fmt.Fprintln(os.Stderr, "entity-id is required with --expand-node")
			os.Exit(2)
		}
		result, err := query.ExpandSubgraph(context.Background(), manager.Ontology(), query.ExpandOptions{
			EntityID:     model.CanonicalID(entityID),
			Depth:        expandDepth,
			Direction:    ontology.Direction(direction),
			RelationKind: relationKind,
			Limit:        limit,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "expand node: %v\n", err)
			os.Exit(1)
		}
		if err := enc.Encode(query.NewGraphSubgraphResponse(result, manager.RuntimeStatus())); err != nil {
			fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
			os.Exit(1)
		}
		return
	}

	result, err := manager.Facade().QueryDiagnosticSubgraph(entryKind, namespace, name, query.DiagnosticOptions{
		MaxDepth:            maxDepth,
		StorageMaxDepth:     storageMaxDepth,
		MaxNodes:            maxNodes,
		MaxEdges:            maxEdges,
		TerminalNodeKinds:   terminalNodeKinds,
		ExpandTerminalNodes: expandTerminalNodes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "diagnostic subgraph: %v\n", err)
		os.Exit(1)
	}

	if err := enc.Encode(query.NewDiagnosticSubgraphResponse(result, manager.RuntimeStatus())); err != nil {
		fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
		os.Exit(1)
	}
}

func explicitFlags() map[string]bool {
	out := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		out[f.Name] = true
	})
	return out
}

func applyConfigDefaults(cfg appconfig.Config, setFlags map[string]bool, kubeconfig, cluster, namespace, contextNamespaces, streamMode *string, bootstrapTimeout, observeDuration, pollInterval *time.Duration, maxDepth, storageMaxDepth *int) error {
	if cfg.Kubeconfig != "" && !setFlags["kubeconfig"] {
		*kubeconfig = cfg.Kubeconfig
	}
	if cfg.Cluster != "" && !setFlags["cluster"] {
		*cluster = cfg.Cluster
	}
	if cfg.Namespace != "" && !setFlags["namespace"] {
		*namespace = cfg.Namespace
	}
	if len(cfg.ContextNamespaces) > 0 && !setFlags["context-namespaces"] && !setFlags["namespaces"] {
		*contextNamespaces = strings.Join(cfg.ContextNamespaces, ",")
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
	if cfg.ObserveDuration != "" && !setFlags["observe-duration"] {
		value, err := time.ParseDuration(cfg.ObserveDuration)
		if err != nil {
			return fmt.Errorf("observeDuration: %w", err)
		}
		*observeDuration = value
	}
	if cfg.PollInterval != "" && !setFlags["poll-interval"] {
		value, err := time.ParseDuration(cfg.PollInterval)
		if err != nil {
			return fmt.Errorf("pollInterval: %w", err)
		}
		*pollInterval = value
	}
	if cfg.MaxDepth > 0 && !setFlags["max-depth"] {
		*maxDepth = cfg.MaxDepth
	}
	if cfg.StorageMaxDepth > 0 && !setFlags["storage-max-depth"] {
		*storageMaxDepth = cfg.StorageMaxDepth
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

func collectionNamespaces(contextNamespaces, entryNamespace string) []string {
	namespaces := splitCSV(contextNamespaces)
	if entryNamespace != "" {
		namespaces = append(namespaces, entryNamespace)
	}
	return dedupeStrings(namespaces)
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

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type continuousStreamOptions struct {
	mode              collectk8s.StreamMode
	collector         collectk8s.Collector
	clientset         kubernetes.Interface
	dynamicClient     dynamic.Interface
	contextNamespaces []string
	workloadResources []collectk8s.WorkloadResource
	pollInterval      time.Duration
	fallbackLogPrefix string
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
	fmt.Fprintf(os.Stderr, "%s: informer stream stopped (%v); falling back to polling\n", options.fallbackLogPrefix, err)
	return manager.RunStream(ctx, collectk8s.NewPollingStream(options.collector, options.pollInterval))
}

type serverQueryOptions struct {
	statusOnly          bool
	entryKind           string
	namespace           string
	name                string
	maxDepth            int
	storageMaxDepth     int
	maxNodes            int
	maxEdges            int
	terminalNodeKinds   []api.NodeKind
	expandTerminalNodes bool
	getEntity           bool
	listEntities        bool
	listRelations       bool
	neighbors           bool
	expandNode          bool
	expandDepth         int
	entityID            string
	entityKind          string
	relationKind        string
	fromID              string
	toID                string
	direction           string
	limit               int
}

func queryServer(server string, options serverQueryOptions) error {
	if options.statusOnly {
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/status")
	}
	if options.getEntity {
		values := entityValues(options)
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/entity?" + values.Encode())
	}
	if options.listEntities {
		values := entityValues(options)
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/entities?" + values.Encode())
	}
	if options.listRelations {
		values := relationValues(options)
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/relations?" + values.Encode())
	}
	if options.neighbors {
		if options.entityID == "" {
			return fmt.Errorf("entity-id is required with --neighbors")
		}
		values := relationValues(options)
		values.Set("entityGlobalId", options.entityID)
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/neighbors?" + values.Encode())
	}
	if options.expandNode {
		if options.entityID == "" {
			return fmt.Errorf("entity-id is required with --expand-node")
		}
		values := relationValues(options)
		values.Set("entityGlobalId", options.entityID)
		if options.expandDepth > 0 {
			values.Set("depth", strconv.Itoa(options.expandDepth))
		}
		return fetchAndPrintJSON(strings.TrimRight(server, "/") + "/expand?" + values.Encode())
	}
	if options.entryKind == "" || options.name == "" {
		return fmt.Errorf("entry-kind and name are required unless --status-only is set")
	}
	endpoint := "/diagnostic"
	values := url.Values{}
	values.Set("kind", options.entryKind)
	if options.namespace != "" {
		values.Set("namespace", options.namespace)
	}
	values.Set("name", options.name)
	if options.maxDepth > 0 {
		values.Set("maxDepth", strconv.Itoa(options.maxDepth))
	}
	if options.storageMaxDepth > 0 {
		values.Set("storageMaxDepth", strconv.Itoa(options.storageMaxDepth))
	}
	if options.maxNodes > 0 {
		values.Set("maxNodes", strconv.Itoa(options.maxNodes))
	}
	if options.maxEdges > 0 {
		values.Set("maxEdges", strconv.Itoa(options.maxEdges))
	}
	if len(options.terminalNodeKinds) > 0 {
		values.Set("terminalKinds", joinNodeKinds(options.terminalNodeKinds))
	}
	if options.expandTerminalNodes {
		values.Set("expandTerminalNodes", "true")
	}
	return fetchAndPrintJSON(strings.TrimRight(server, "/") + endpoint + "?" + values.Encode())
}

func joinNodeKinds(kinds []api.NodeKind) string {
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, string(kind))
	}
	return strings.Join(parts, ",")
}

func entityValues(options serverQueryOptions) url.Values {
	values := url.Values{}
	if options.entityID != "" {
		values.Set("entityGlobalId", options.entityID)
	}
	if options.entityKind != "" {
		values.Set("kind", options.entityKind)
	}
	if options.namespace != "" {
		values.Set("namespace", options.namespace)
	}
	if options.name != "" {
		values.Set("name", options.name)
	}
	if options.limit > 0 {
		values.Set("limit", strconv.Itoa(options.limit))
	}
	return values
}

func relationValues(options serverQueryOptions) url.Values {
	values := url.Values{}
	if options.fromID != "" {
		values.Set("from", options.fromID)
	}
	if options.toID != "" {
		values.Set("to", options.toID)
	}
	if options.relationKind != "" {
		values.Set("kind", options.relationKind)
	}
	if options.direction != "" {
		values.Set("direction", options.direction)
	}
	if options.limit > 0 {
		values.Set("limit", strconv.Itoa(options.limit))
	}
	return values
}

func fetchAndPrintJSON(rawURL string) error {
	resp, err := http.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return decodeServerError(resp)
	}
	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

type serverError struct {
	StatusCode int
	Status     string
	Payload    query.ErrorResponse
	RawBody    string
}

func (e *serverError) Error() string {
	if e.Payload.Error != "" {
		return fmt.Sprintf("server returned %s: %s", e.Status, e.Payload.Error)
	}
	if e.RawBody != "" {
		return fmt.Sprintf("server returned %s: %s", e.Status, e.RawBody)
	}
	return fmt.Sprintf("server returned %s", e.Status)
}

func decodeServerError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	rawBody := strings.TrimSpace(string(body))
	payload := query.ErrorResponse{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &payload)
	}
	if payload.Error == "" {
		message := rawBody
		if message == "" {
			message = resp.Status
		}
		payload = query.NewErrorResponse("server_error", resp.StatusCode, errors.New(message), retryableStatusCode(resp.StatusCode))
	} else {
		if payload.Message == "" {
			payload.Message = payload.Error
		}
		if payload.Code == "" {
			payload.Code = "server_error"
		}
		if payload.Status == 0 {
			payload.Status = resp.StatusCode
		}
		payload.Retryable = payload.Retryable || retryableStatusCode(resp.StatusCode)
	}
	if payload.Source == "" {
		payload.Source = "server"
	}
	return &serverError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Payload:    payload,
		RawBody:    rawBody,
	}
}

func writeMachineError(w io.Writer, err error) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(machineErrorResponse(err))
}

func machineErrorResponse(err error) query.ErrorResponse {
	var serverErr *serverError
	if errors.As(err, &serverErr) {
		return serverErr.Payload
	}
	response := query.NewErrorResponse("cli_error", 0, err, false)
	response.Source = "cli"
	return response
}

func retryableStatusCode(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func collapseGraphFile(path, entityID string) error {
	if path == "" {
		return fmt.Errorf("graph-file is required with --collapse-node")
	}
	if entityID == "" {
		return fmt.Errorf("entity-id is required with --collapse-node")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var doc query.GraphStateDocument
	if err := json.NewDecoder(file).Decode(&doc); err != nil {
		return err
	}
	collapsed, err := query.CollapseGraphExpansion(doc, entityID)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(collapsed)
}
