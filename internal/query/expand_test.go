package query

import (
	"context"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestExpandSubgraph(t *testing.T) {
	backend, podA, nodeA, podB := expandFixture(t)

	result, err := ExpandSubgraph(context.Background(), backend, ExpandOptions{
		EntityID:  podA,
		Depth:     1,
		Direction: ontology.DirectionBoth,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("expected one-hop pod/node subgraph, got nodes=%d edges=%d", len(result.Nodes), len(result.Edges))
	}
	if result.Entry.CanonicalID != podA.String() {
		t.Fatalf("unexpected entry: %+v", result.Entry)
	}

	deeper, err := ExpandSubgraph(context.Background(), backend, ExpandOptions{
		EntityID:  podA,
		Depth:     2,
		Direction: ontology.DirectionBoth,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsNode(deeper, podB.String()) || !containsNode(deeper, nodeA.String()) {
		t.Fatalf("expected two-hop subgraph to include node and sibling pod, got %+v", deeper.Nodes)
	}
}

func TestExpandSubgraphDirection(t *testing.T) {
	backend, podA, _, _ := expandFixture(t)
	out, err := ExpandSubgraph(context.Background(), backend, ExpandOptions{
		EntityID:  podA,
		Depth:     1,
		Direction: ontology.DirectionOut,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("expected outgoing edge, got %+v", out.Edges)
	}
	in, err := ExpandSubgraph(context.Background(), backend, ExpandOptions{
		EntityID:  podA,
		Depth:     1,
		Direction: ontology.DirectionIn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Edges) != 0 {
		t.Fatalf("expected no incoming edges, got %+v", in.Edges)
	}
}

func TestValidateExpandOptions(t *testing.T) {
	if err := ValidateExpandOptions(ExpandOptions{EntityID: "pod", Depth: MaxExpandDepth + 1}); err == nil {
		t.Fatal("expected too-large depth to fail")
	}
	if err := ValidateExpandOptions(ExpandOptions{EntityID: "pod", Direction: "sideways"}); err == nil {
		t.Fatal("expected invalid direction to fail")
	}
}

func expandFixture(t *testing.T) (ontology.Backend, model.CanonicalID, model.CanonicalID, model.CanonicalID) {
	t.Helper()
	store := memorystore.NewStore()
	backend := ontology.NewKernelBackend(graph.NewKernel(store, store))
	ctx := context.Background()
	podA := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "pod-a", UID: "pa"})
	nodeA := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Node", Name: "node-a", UID: "na"})
	podB := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "pod-b", UID: "pb"})
	for _, entity := range []ontology.Entity{
		{ID: podA, Kind: "Pod", Namespace: "default", Name: "pod-a"},
		{ID: nodeA, Kind: "Node", Name: "node-a"},
		{ID: podB, Kind: "Pod", Namespace: "default", Name: "pod-b"},
	} {
		if err := backend.UpsertEntity(ctx, entity); err != nil {
			t.Fatal(err)
		}
	}
	for _, relation := range []ontology.Relation{
		{From: podA, To: nodeA, Kind: "scheduled_on"},
		{From: podB, To: nodeA, Kind: "scheduled_on"},
	} {
		if err := backend.UpsertRelation(ctx, relation); err != nil {
			t.Fatal(err)
		}
	}
	return backend, podA, nodeA, podB
}

func containsNode(result api.GraphSubgraph, id string) bool {
	for _, node := range result.Nodes {
		if node.CanonicalID == id {
			return true
		}
	}
	return false
}
