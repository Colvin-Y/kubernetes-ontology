package store

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

type GraphStore interface {
	UpsertNode(node model.Node) error
	DeleteNode(id model.CanonicalID) error
	UpsertEdge(edge model.Edge) error
	DeleteEdge(key string) error
	GetNode(id model.CanonicalID) (model.Node, bool)
	GetEdge(key string) (model.Edge, bool)
	ListNodes() []model.Node
	ListEdges() []model.Edge
}

type IndexStore interface {
	AddNeighbor(from model.CanonicalID, edgeKey string)
	RemoveNeighbor(from model.CanonicalID, edgeKey string)
	NeighborKeys(from model.CanonicalID) []string
}
