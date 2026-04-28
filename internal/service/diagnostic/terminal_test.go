package diagnostic

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestDefaultExpansionStopsAtTerminalSharedNodes(t *testing.T) {
	service, podA, podB, _ := terminalFixture(t)
	policy := DefaultExpansionPolicy()
	policy.MaxDepth = 2

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: podA.ID.String()}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosticContainsNode(result.Nodes, podB.Name) {
		t.Fatal("expected default terminal ServiceAccount boundary to hide unrelated pods")
	}
	if !diagnosticContainsKind(result.Nodes, api.NodeKindServiceAccount) {
		t.Fatal("expected terminal ServiceAccount node to remain visible")
	}
}

func TestExpansionCanTraverseTerminalNodes(t *testing.T) {
	service, podA, podB, _ := terminalFixture(t)
	policy := DefaultExpansionPolicy()
	policy.MaxDepth = 2
	policy.ExpandTerminalNodes = true

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: podA.ID.String()}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticContainsNode(result.Nodes, podB.Name) {
		t.Fatal("expected explicit terminal expansion to include pod sharing the ServiceAccount")
	}
}

func terminalFixture(t *testing.T) (*Service, model.Node, model.Node, model.Node) {
	t.Helper()
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	podA := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "frontend-a", UID: "pa"}), Kind: model.NodeKindPod, Name: "frontend-a", Namespace: "default"}
	podB := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "frontend-b", UID: "pb"}), Kind: model.NodeKindPod, Name: "frontend-b", Namespace: "default"}
	sa := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "ServiceAccount", Namespace: "default", Name: "default", UID: "sa"}), Kind: model.NodeKindServiceAccount, Name: "default", Namespace: "default"}

	for _, node := range []model.Node{podA, podB, sa} {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []model.Edge{
		{From: podA.ID, To: sa.ID, Kind: model.EdgeKindUsesServiceAccount},
		{From: podB.ID, To: sa.ID, Kind: model.EdgeKindUsesServiceAccount},
	} {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
	}
	return service, podA, podB, sa
}
