package reconcile

import (
	"reflect"
	"sort"
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestServiceReconcilerMatchesFullRebuildSelectorEdges(t *testing.T) {
	initial := serviceSnapshot(map[string]string{"app": "frontend"})
	next := serviceSnapshot(map[string]string{"app": "backend"})
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewServiceReconciler("cluster-a", kernel).Apply(next, "default", "frontend", collectk8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected service update to be applied")
	}
	if result.DeletedEdges != 1 {
		t.Fatalf("expected one old selector edge to be deleted, got %d", result.DeletedEdges)
	}
	if result.UpsertedEdges != 1 {
		t.Fatalf("expected one new selector edge to be upserted, got %d", result.UpsertedEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	got := selectorEdgeKeys(kernel.ListEdges())
	want := selectorEdgeKeys(full.ListEdges())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selector edges mismatch\ngot  %#v\nwant %#v", got, want)
	}
}

func TestServiceReconcilerDeletesMissingService(t *testing.T) {
	initial := serviceSnapshot(map[string]string{"app": "frontend"})
	next := serviceSnapshot(map[string]string{"app": "frontend"})
	next.Services = nil
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewServiceReconciler("cluster-a", kernel).Apply(next, "default", "frontend", collectk8s.ChangeTypeDelete)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted {
		t.Fatal("expected delete result")
	}
	if len(selectorEdgeKeys(kernel.ListEdges())) != 0 {
		t.Fatal("expected service selector edges to be removed")
	}
	for _, node := range kernel.ListNodes() {
		if node.Kind == model.NodeKindService && node.Namespace == "default" && node.Name == "frontend" {
			t.Fatal("expected service node to be removed")
		}
	}
}

func serviceSnapshot(selector map[string]string) collectk8s.Snapshot {
	return collectk8s.Snapshot{
		Pods: []resources.Pod{
			{Metadata: resources.Metadata{UID: "p1", Name: "frontend-1", Namespace: "default", Labels: map[string]string{"app": "frontend"}}},
			{Metadata: resources.Metadata{UID: "p2", Name: "backend-1", Namespace: "default", Labels: map[string]string{"app": "backend"}}},
		},
		Services: []resources.Service{{Metadata: resources.Metadata{UID: "s1", Name: "frontend", Namespace: "default"}, Selector: selector}},
	}
}

func kernelFromSnapshot(t *testing.T, cluster string, snapshot collectk8s.Snapshot) *graph.Kernel {
	t.Helper()
	builder := graph.NewBuilder(cluster)
	nodes, edges := builder.Build(snapshot)
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	for _, node := range nodes {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range edges {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
	}
	return kernel
}

func selectorEdgeKeys(edges []model.Edge) []string {
	out := make([]string, 0)
	for _, edge := range edges {
		if edge.Kind == model.EdgeKindSelectsPod {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}
