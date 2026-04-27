package ontology

import (
	"context"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestKernelBackendEntityAndRelationQueries(t *testing.T) {
	store := memorystore.NewStore()
	backend := NewKernelBackend(graph.NewKernel(store, store))
	ctx := context.Background()

	podID := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "pod-a", UID: "p1"})
	nodeID := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Node", Name: "node-a", UID: "n1"})
	if err := backend.UpsertEntity(ctx, Entity{ID: podID, Kind: "Pod", Namespace: "default", Name: "pod-a"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.UpsertEntity(ctx, Entity{ID: nodeID, Kind: "Node", Name: "node-a"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.UpsertRelation(ctx, Relation{From: podID, To: nodeID, Kind: "scheduled_on"}); err != nil {
		t.Fatal(err)
	}

	entity, ok, err := backend.FindEntity(ctx, EntityRef{Kind: "Pod", Namespace: "default", Name: "pod-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || entity.ID != podID {
		t.Fatalf("expected pod entity, got ok=%v entity=%+v", ok, entity)
	}

	entities, err := backend.ListEntities(ctx, EntityQuery{Kind: "Pod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) != 1 || entities[0].ID != podID {
		t.Fatalf("expected one pod entity, got %+v", entities)
	}

	relations, err := backend.ListRelations(ctx, RelationQuery{From: podID, Kind: "scheduled_on"})
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 || relations[0].To != nodeID {
		t.Fatalf("expected scheduled_on relation, got %+v", relations)
	}

	neighbors, err := backend.Neighbors(ctx, podID, RelationQuery{Direction: DirectionOut})
	if err != nil {
		t.Fatal(err)
	}
	if len(neighbors) != 1 || neighbors[0].To != nodeID {
		t.Fatalf("expected outgoing neighbor, got %+v", neighbors)
	}

	stats, err := backend.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.EntityCount != 2 || stats.RelationCount != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
