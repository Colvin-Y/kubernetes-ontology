package graph

import (
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/store"
)

type Kernel struct {
	store store.GraphStore
	index store.IndexStore
}

func NewKernel(graphStore store.GraphStore, indexStore store.IndexStore) *Kernel {
	return &Kernel{store: graphStore, index: indexStore}
}

func (k *Kernel) UpsertNode(node model.Node) error {
	return k.store.UpsertNode(node)
}

func (k *Kernel) DeleteNode(id model.CanonicalID) error {
	for _, edge := range k.store.ListEdges() {
		if edge.From == id || edge.To == id {
			if err := k.DeleteEdge(edge.Key()); err != nil {
				return err
			}
		}
	}
	return k.store.DeleteNode(id)
}

func (k *Kernel) UpsertEdge(edge model.Edge) error {
	if err := k.store.UpsertEdge(edge); err != nil {
		return err
	}
	k.index.AddNeighbor(edge.From, edge.Key())
	k.index.AddNeighbor(edge.To, edge.Key())
	return nil
}

func (k *Kernel) DeleteEdge(key string) error {
	for _, edge := range k.store.ListEdges() {
		if edge.Key() == key {
			k.index.RemoveNeighbor(edge.From, key)
			k.index.RemoveNeighbor(edge.To, key)
			break
		}
	}
	return k.store.DeleteEdge(key)
}

func (k *Kernel) GetNode(id model.CanonicalID) (model.Node, bool) {
	return k.store.GetNode(id)
}

func (k *Kernel) ListNodes() []model.Node {
	return k.store.ListNodes()
}

func (k *Kernel) ListEdges() []model.Edge {
	return k.store.ListEdges()
}

func (k *Kernel) Neighbors(id model.CanonicalID) []model.Edge {
	keys := k.index.NeighborKeys(id)
	matched := make([]model.Edge, 0, len(keys))
	for _, key := range keys {
		if edge, ok := k.store.GetEdge(key); ok {
			matched = append(matched, edge)
		}
	}
	return matched
}
