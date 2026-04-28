package diagnostic

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestGetDiagnosticSubgraphRespectsMaxDepth(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	a := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "a", UID: "a1"}), Kind: model.NodeKindPod, Name: "a", Namespace: "default"}
	b := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Service", Namespace: "default", Name: "b", UID: "b1"}), Kind: model.NodeKindService, Name: "b", Namespace: "default"}
	c := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "ConfigMap", Namespace: "default", Name: "c", UID: "c1"}), Kind: model.NodeKindConfigMap, Name: "c", Namespace: "default"}

	for _, node := range []model.Node{a, b, c} {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []model.Edge{
		{From: a.ID, To: b.ID, Kind: model.EdgeKindRelatedTo},
		{From: b.ID, To: c.ID, Kind: model.EdgeKindRelatedTo},
	} {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
	}

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: a.ID.String()}, api.ExpansionPolicy{MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected depth-1 traversal to return 2 nodes, got %d", len(result.Nodes))
	}
}
