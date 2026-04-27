package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
)

func TestReadOnlyCollectorCollect(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "w1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123", Namespace: "default", UID: "p1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "n1"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "open-local", UID: "sc1"}, Provisioner: "local.csi.aliyun.com"},
		&storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: "local.csi.aliyun.com", UID: "driver1"}},
	)
	collector := NewReadOnlyCollector(client, "cluster-a", "default")

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(snapshot.Workloads))
	}
	if len(snapshot.Pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(snapshot.Pods))
	}
	if len(snapshot.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(snapshot.Nodes))
	}
	if len(snapshot.StorageClasses) != 1 {
		t.Fatalf("expected 1 storage class, got %d", len(snapshot.StorageClasses))
	}
	if len(snapshot.CSIDrivers) != 1 {
		t.Fatalf("expected 1 csi driver, got %d", len(snapshot.CSIDrivers))
	}
}

func TestReadOnlyCollectorCollectsContextNamespaces(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "p1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "csi-node", Namespace: "kube-system", UID: "p2"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: "other", UID: "p3"}},
	)
	collector := NewReadOnlyCollector(client, "cluster-a", "default", "kube-system", "default")

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Pods) != 2 {
		t.Fatalf("expected 2 pods from context namespaces, got %d", len(snapshot.Pods))
	}
	namespaces := map[string]bool{}
	for _, pod := range snapshot.Pods {
		namespaces[pod.Metadata.Namespace] = true
	}
	if !namespaces["default"] || !namespaces["kube-system"] || namespaces["other"] {
		t.Fatalf("unexpected namespaces collected: %#v", namespaces)
	}
}

func TestReadOnlyCollectorCollectsClusterScopedResourcesOnce(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "n1"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-a", UID: "pv1"}},
	)
	nodeLists := 0
	pvLists := 0
	client.Fake.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		nodeLists++
		return false, nil, nil
	})
	client.Fake.PrependReactor("list", "persistentvolumes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pvLists++
		return false, nil, nil
	})
	collector := NewReadOnlyCollector(client, "cluster-a", "default", "kube-system")

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Nodes) != 1 || len(snapshot.PVs) != 1 {
		t.Fatalf("expected one node and one pv in snapshot, got nodes=%d pvs=%d", len(snapshot.Nodes), len(snapshot.PVs))
	}
	if nodeLists != 1 || pvLists != 1 {
		t.Fatalf("expected cluster-scoped resources to be listed once, got nodeLists=%d pvLists=%d", nodeLists, pvLists)
	}
}

func TestReadOnlyCollectorDegradesWhenStorageAPIsAreForbidden(t *testing.T) {
	client := k8sfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "p1"}},
	)
	for _, resource := range []string{"storageclasses", "csidrivers"} {
		resource := resource
		client.Fake.PrependReactor("list", resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "storage.k8s.io", Resource: resource}, "", nil)
		})
	}
	collector := NewReadOnlyCollector(client, "cluster-a", "default")

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Pods) != 1 {
		t.Fatalf("expected pod collection to continue, got %d pods", len(snapshot.Pods))
	}
	if len(snapshot.StorageClasses) != 0 || len(snapshot.CSIDrivers) != 0 {
		t.Fatalf("expected forbidden storage APIs to degrade to empty lists, got sc=%d drivers=%d", len(snapshot.StorageClasses), len(snapshot.CSIDrivers))
	}
}

func TestReadOnlyCollectorCollectsConfiguredWorkloadResources(t *testing.T) {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme,
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps.kruise.io/v1alpha1",
			"kind":       "AdvancedStatefulSet",
			"metadata": map[string]any{
				"name":      "proxy-redis-cq01mg647b8wj86qz",
				"namespace": "redis",
				"uid":       "asts-uid",
				"ownerReferences": []any{map[string]any{
					"apiVersion": "db.example.com/v1",
					"kind":       "RedisCluster",
					"name":       "redis-cq01mg647b8wj86qz",
					"uid":        "redis-uid",
					"controller": true,
				}},
			},
			"spec": map[string]any{"replicas": int64(3)},
		}},
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "db.example.com/v1",
			"kind":       "RedisCluster",
			"metadata": map[string]any{
				"name":      "redis-cq01mg647b8wj86qz",
				"namespace": "redis",
				"uid":       "redis-uid",
			},
		}},
	)
	collector := NewReadOnlyCollectorWithOptions(k8sfake.NewSimpleClientset(), "cluster-a", CollectorOptions{
		ContextNamespaces: []string{"redis"},
		DynamicClient:     dynamicClient,
		WorkloadResources: []WorkloadResource{
			{Group: "apps.kruise.io", Version: "v1alpha1", Resource: "advancedstatefulsets", Kind: "AdvancedStatefulSet", Namespaced: true},
			{Group: "db.example.com", Version: "v1", Resource: "redisclusters", Kind: "RedisCluster", Namespaced: true},
		},
	})

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workloads) != 2 {
		t.Fatalf("expected configured workloads to be collected, got %d", len(snapshot.Workloads))
	}
	var asts resources.Workload
	for _, workload := range snapshot.Workloads {
		if workload.ControllerKind == "AdvancedStatefulSet" {
			asts = workload
		}
	}
	if asts.Metadata.Name == "" {
		t.Fatal("expected advanced statefulset workload")
	}
	if asts.APIVersion != "apps.kruise.io/v1alpha1" || asts.Replicas != 3 {
		t.Fatalf("unexpected custom workload normalization: %#v", asts)
	}
	if len(asts.OwnerReferences) != 1 || asts.OwnerReferences[0].Kind != "RedisCluster" || !asts.OwnerReferences[0].Controller {
		t.Fatalf("expected rediscluster owner reference, got %#v", asts.OwnerReferences)
	}
}

func TestParseWorkloadResources(t *testing.T) {
	parsed, err := ParseWorkloadResources("apps.kruise.io/v1alpha1/advancedstatefulsets/AdvancedStatefulSet,db.example.com/v1/redisclusters/RedisCluster")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected two resources, got %d", len(parsed))
	}
	if parsed[0].GVR().String() != "apps.kruise.io/v1alpha1, Resource=advancedstatefulsets" || parsed[0].Kind != "AdvancedStatefulSet" || !parsed[0].Namespaced {
		t.Fatalf("unexpected parsed workload resource: %#v", parsed[0])
	}
}
