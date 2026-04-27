package k8s

import (
	"context"
	"fmt"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
)

type Snapshot struct {
	Workloads           []resources.Workload
	ReplicaSets         []resources.ReplicaSet
	Pods                []resources.Pod
	Nodes               []resources.Node
	Services            []resources.Service
	ConfigMaps          []resources.ConfigMap
	Secrets             []resources.Secret
	ServiceAccounts     []resources.ServiceAccount
	RoleBindings        []resources.RoleBinding
	ClusterRoleBindings []resources.ClusterRoleBinding
	PVCs                []resources.PVC
	PVs                 []resources.PV
	StorageClasses      []resources.StorageClass
	CSIDrivers          []resources.CSIDriver
	Events              []resources.Event
	WebhookConfigs      []resources.WebhookConfig
}

type Collector interface {
	Collect(ctx context.Context) (Snapshot, error)
}

type CollectorOptions struct {
	ContextNamespaces []string
	DynamicClient     dynamic.Interface
	WorkloadResources []WorkloadResource
}

type WorkloadResource struct {
	Group      string `json:"group" yaml:"group"`
	Version    string `json:"version" yaml:"version"`
	Resource   string `json:"resource" yaml:"resource"`
	Kind       string `json:"kind" yaml:"kind"`
	Namespaced bool   `json:"namespaced" yaml:"namespaced"`
}

func (r WorkloadResource) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Resource}
}

type ReadOnlyCollector struct {
	client            kubernetes.Interface
	dynamicClient     dynamic.Interface
	cluster           string
	contextNamespaces []string
	workloadResources []WorkloadResource
}

func NewReadOnlyCollector(client kubernetes.Interface, cluster string, contextNamespaces ...string) *ReadOnlyCollector {
	return NewReadOnlyCollectorWithOptions(client, cluster, CollectorOptions{ContextNamespaces: contextNamespaces})
}

func NewReadOnlyCollectorWithOptions(client kubernetes.Interface, cluster string, options CollectorOptions) *ReadOnlyCollector {
	return &ReadOnlyCollector{
		client:            client,
		dynamicClient:     options.DynamicClient,
		cluster:           cluster,
		contextNamespaces: options.ContextNamespaces,
		workloadResources: options.WorkloadResources,
	}
}

func ParseWorkloadResources(raw string) ([]WorkloadResource, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]WorkloadResource, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, "/")
		if len(fields) != 4 && len(fields) != 5 {
			return nil, fmt.Errorf("workload resource %q must be group/version/resource/kind[/scope]", part)
		}
		scope := "namespaced"
		if len(fields) == 5 {
			scope = strings.ToLower(strings.TrimSpace(fields[4]))
		}
		namespaced := scope != "cluster" && scope != "cluster-scoped"
		out = append(out, WorkloadResource{
			Group:      strings.TrimSpace(fields[0]),
			Version:    strings.TrimSpace(fields[1]),
			Resource:   strings.TrimSpace(fields[2]),
			Kind:       strings.TrimSpace(fields[3]),
			Namespaced: namespaced,
		})
	}
	return out, nil
}

func (c *ReadOnlyCollector) Collect(ctx context.Context) (Snapshot, error) {
	var out Snapshot
	for _, ns := range c.scopedNamespaces() {
		partial, err := c.collectNamespaced(ctx, ns)
		if err != nil {
			return Snapshot{}, err
		}
		out.Workloads = append(out.Workloads, partial.Workloads...)
		out.ReplicaSets = append(out.ReplicaSets, partial.ReplicaSets...)
		out.Pods = append(out.Pods, partial.Pods...)
		out.Services = append(out.Services, partial.Services...)
		out.ConfigMaps = append(out.ConfigMaps, partial.ConfigMaps...)
		out.Secrets = append(out.Secrets, partial.Secrets...)
		out.ServiceAccounts = append(out.ServiceAccounts, partial.ServiceAccounts...)
		out.RoleBindings = append(out.RoleBindings, partial.RoleBindings...)
		out.PVCs = append(out.PVCs, partial.PVCs...)
		out.Events = append(out.Events, partial.Events...)
	}
	clusterScopedWorkloads, err := c.collectClusterScopedWorkloads(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	out.Workloads = append(out.Workloads, clusterScopedWorkloads...)

	nodes, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range nodes.Items {
		out.Nodes = append(out.Nodes, resources.NormalizeNode(item))
	}

	clusterRoleBindings, err := c.client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range clusterRoleBindings.Items {
		out.ClusterRoleBindings = append(out.ClusterRoleBindings, resources.NormalizeClusterRoleBinding(item))
	}

	pvs, err := c.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range pvs.Items {
		out.PVs = append(out.PVs, resources.NormalizePV(item))
	}

	storageClasses, err := c.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsForbidden(err) && !apierrors.IsNotFound(err) {
			return Snapshot{}, err
		}
	} else {
		for _, item := range storageClasses.Items {
			out.StorageClasses = append(out.StorageClasses, resources.NormalizeStorageClass(item))
		}
	}

	csiDrivers, err := c.client.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsForbidden(err) && !apierrors.IsNotFound(err) {
			return Snapshot{}, err
		}
	} else {
		for _, item := range csiDrivers.Items {
			out.CSIDrivers = append(out.CSIDrivers, resources.NormalizeCSIDriver(item))
		}
	}

	mutating, err := c.client.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range mutating.Items {
		out.WebhookConfigs = append(out.WebhookConfigs, resources.NormalizeMutatingWebhookConfig(item))
	}

	validating, err := c.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range validating.Items {
		out.WebhookConfigs = append(out.WebhookConfigs, resources.NormalizeValidatingWebhookConfig(item))
	}

	_ = c.cluster
	return out, nil
}

func (c *ReadOnlyCollector) collectNamespaced(ctx context.Context, ns string) (Snapshot, error) {
	var out Snapshot

	deployments, err := c.client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range deployments.Items {
		out.Workloads = append(out.Workloads, resources.NormalizeDeployment(item))
	}

	statefulsets, err := c.client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range statefulsets.Items {
		out.Workloads = append(out.Workloads, resources.NormalizeStatefulSet(item))
	}

	daemonsets, err := c.client.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range daemonsets.Items {
		out.Workloads = append(out.Workloads, resources.NormalizeDaemonSet(item))
	}

	jobs, err := c.client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range jobs.Items {
		out.Workloads = append(out.Workloads, resources.NormalizeJob(item))
	}

	replicaSets, err := c.client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range replicaSets.Items {
		out.ReplicaSets = append(out.ReplicaSets, resources.NormalizeReplicaSet(item))
	}

	customWorkloads, err := c.collectNamespacedWorkloads(ctx, ns)
	if err != nil {
		return Snapshot{}, err
	}
	out.Workloads = append(out.Workloads, customWorkloads...)

	pods, err := c.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range pods.Items {
		out.Pods = append(out.Pods, resources.NormalizePod(item))
	}

	services, err := c.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range services.Items {
		out.Services = append(out.Services, resources.NormalizeService(item))
	}

	configMaps, err := c.client.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range configMaps.Items {
		out.ConfigMaps = append(out.ConfigMaps, resources.NormalizeConfigMap(item))
	}

	secrets, err := c.client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsForbidden(err) && !apierrors.IsNotFound(err) {
			return Snapshot{}, err
		}
	} else {
		for _, item := range secrets.Items {
			out.Secrets = append(out.Secrets, resources.NormalizeSecret(item))
		}
	}

	serviceAccounts, err := c.client.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range serviceAccounts.Items {
		out.ServiceAccounts = append(out.ServiceAccounts, resources.NormalizeServiceAccount(item))
	}

	roleBindings, err := c.client.RbacV1().RoleBindings(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range roleBindings.Items {
		out.RoleBindings = append(out.RoleBindings, resources.NormalizeRoleBinding(item))
	}

	pvcs, err := c.client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range pvcs.Items {
		out.PVCs = append(out.PVCs, resources.NormalizePVC(item))
	}

	events, err := c.client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	for _, item := range events.Items {
		out.Events = append(out.Events, resources.NormalizeEvent(item))
	}

	return out, nil
}

func (c *ReadOnlyCollector) collectNamespacedWorkloads(ctx context.Context, namespace string) ([]resources.Workload, error) {
	if c.dynamicClient == nil || len(c.workloadResources) == 0 {
		return nil, nil
	}
	out := make([]resources.Workload, 0)
	for _, resource := range c.workloadResources {
		if !resource.Namespaced {
			continue
		}
		items, err := c.dynamicClient.Resource(resource.GVR()).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		for _, item := range items.Items {
			out = append(out, resources.NormalizeUnstructuredWorkload(item, resource.Kind))
		}
	}
	return out, nil
}

func (c *ReadOnlyCollector) collectClusterScopedWorkloads(ctx context.Context) ([]resources.Workload, error) {
	if c.dynamicClient == nil || len(c.workloadResources) == 0 {
		return nil, nil
	}
	out := make([]resources.Workload, 0)
	for _, resource := range c.workloadResources {
		if resource.Namespaced {
			continue
		}
		items, err := c.dynamicClient.Resource(resource.GVR()).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		for _, item := range items.Items {
			out = append(out, resources.NormalizeUnstructuredWorkload(item, resource.Kind))
		}
	}
	return out, nil
}

func (c *ReadOnlyCollector) scopedNamespaces() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(c.contextNamespaces))
	appendNS := func(ns string) {
		if ns == "" {
			return
		}
		if _, ok := seen[ns]; ok {
			return
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}
	for _, ns := range c.contextNamespaces {
		appendNS(ns)
	}
	if len(out) == 0 {
		return []string{metav1.NamespaceAll}
	}
	return out
}

var (
	_ = appsv1.Deployment{}
	_ = batchv1.Job{}
	_ = corev1.Pod{}
	_ = rbacv1.RoleBinding{}
	_ = storagev1.StorageClass{}
	_ = admissionv1.MutatingWebhookConfiguration{}
)
