package ontology

import (
	"context"
	"strings"

	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type Backend interface {
	UpsertEntity(ctx context.Context, entity Entity) error
	DeleteEntity(ctx context.Context, id model.CanonicalID) error
	GetEntity(ctx context.Context, id model.CanonicalID) (Entity, bool, error)
	FindEntity(ctx context.Context, ref EntityRef) (Entity, bool, error)
	ListEntities(ctx context.Context, query EntityQuery) ([]Entity, error)
	UpsertRelation(ctx context.Context, relation Relation) error
	DeleteRelation(ctx context.Context, relation Relation) error
	ListRelations(ctx context.Context, query RelationQuery) ([]Relation, error)
	Neighbors(ctx context.Context, id model.CanonicalID, query RelationQuery) ([]Relation, error)
	Stats(ctx context.Context) (Stats, error)
}

type KernelBackend struct {
	kernel *graph.Kernel
}

func NewKernelBackend(kernel *graph.Kernel) *KernelBackend {
	return &KernelBackend{kernel: kernel}
}

func (b *KernelBackend) UpsertEntity(ctx context.Context, entity Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.kernel.UpsertNode(NodeFromEntity(entity))
}

func (b *KernelBackend) DeleteEntity(ctx context.Context, id model.CanonicalID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.kernel.DeleteNode(id)
}

func (b *KernelBackend) GetEntity(ctx context.Context, id model.CanonicalID) (Entity, bool, error) {
	if err := ctx.Err(); err != nil {
		return Entity{}, false, err
	}
	node, ok := b.kernel.GetNode(id)
	if !ok {
		return Entity{}, false, nil
	}
	return EntityFromNode(node), true, nil
}

func (b *KernelBackend) FindEntity(ctx context.Context, ref EntityRef) (Entity, bool, error) {
	if err := ctx.Err(); err != nil {
		return Entity{}, false, err
	}
	if ref.ID != "" {
		return b.GetEntity(ctx, ref.ID)
	}
	entities, err := b.ListEntities(ctx, EntityQuery{Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name, Limit: 1})
	if err != nil || len(entities) == 0 {
		return Entity{}, false, err
	}
	return entities[0], true, nil
}

func (b *KernelBackend) ListEntities(ctx context.Context, query EntityQuery) ([]Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nodes := b.kernel.ListNodes()
	out := make([]Entity, 0, len(nodes))
	for _, node := range nodes {
		if !entityMatches(node, query) {
			continue
		}
		out = append(out, EntityFromNode(node))
		if query.Limit > 0 && len(out) >= query.Limit {
			break
		}
	}
	return out, nil
}

func (b *KernelBackend) UpsertRelation(ctx context.Context, relation Relation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.kernel.UpsertEdge(EdgeFromRelation(relation))
}

func (b *KernelBackend) DeleteRelation(ctx context.Context, relation Relation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.kernel.DeleteEdge(EdgeFromRelation(relation).Key())
}

func (b *KernelBackend) ListRelations(ctx context.Context, query RelationQuery) ([]Relation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	edges := b.kernel.ListEdges()
	out := make([]Relation, 0, len(edges))
	for _, edge := range edges {
		if !relationMatches(edge, query) {
			continue
		}
		out = append(out, RelationFromEdge(edge))
		if query.Limit > 0 && len(out) >= query.Limit {
			break
		}
	}
	return out, nil
}

func (b *KernelBackend) Neighbors(ctx context.Context, id model.CanonicalID, query RelationQuery) ([]Relation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query.From = ""
	query.To = ""
	edges := b.kernel.Neighbors(id)
	out := make([]Relation, 0, len(edges))
	for _, edge := range edges {
		if !relationTouches(edge, id, query.Direction) || !relationMatches(edge, query) {
			continue
		}
		out = append(out, RelationFromEdge(edge))
		if query.Limit > 0 && len(out) >= query.Limit {
			break
		}
	}
	return out, nil
}

func (b *KernelBackend) Stats(ctx context.Context) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	return Stats{
		EntityCount:   len(b.kernel.ListNodes()),
		RelationCount: len(b.kernel.ListEdges()),
	}, nil
}

func entityMatches(node model.Node, query EntityQuery) bool {
	if query.Kind != "" && !sameKind(query.Kind, string(node.Kind), node.SourceKind) {
		return false
	}
	if query.Namespace != "" && query.Namespace != node.Namespace {
		return false
	}
	if query.Name != "" && query.Name != node.Name {
		return false
	}
	return true
}

func relationMatches(edge model.Edge, query RelationQuery) bool {
	if query.From != "" && query.From != edge.From {
		return false
	}
	if query.To != "" && query.To != edge.To {
		return false
	}
	if query.Kind != "" && !strings.EqualFold(query.Kind, string(edge.Kind)) {
		return false
	}
	if query.Direction != "" && query.Direction != DirectionBoth {
		if query.Direction == DirectionOut && query.From != "" && query.From != edge.From {
			return false
		}
		if query.Direction == DirectionIn && query.To != "" && query.To != edge.To {
			return false
		}
	}
	return true
}

func relationTouches(edge model.Edge, id model.CanonicalID, direction Direction) bool {
	switch direction {
	case DirectionOut:
		return edge.From == id
	case DirectionIn:
		return edge.To == id
	default:
		return edge.From == id || edge.To == id
	}
}

func sameKind(want, nodeKind, sourceKind string) bool {
	return strings.EqualFold(want, nodeKind) || strings.EqualFold(want, sourceKind)
}
