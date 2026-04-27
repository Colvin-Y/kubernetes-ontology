package memory

import (
	"sync"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type Store struct {
	mu        sync.RWMutex
	nodes     map[model.CanonicalID]model.Node
	edges     map[string]model.Edge
	neighbors map[model.CanonicalID]map[string]struct{}
}

func NewStore() *Store {
	return &Store{
		nodes:     make(map[model.CanonicalID]model.Node),
		edges:     make(map[string]model.Edge),
		neighbors: make(map[model.CanonicalID]map[string]struct{}),
	}
}

func (s *Store) UpsertNode(node model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	return nil
}

func (s *Store) DeleteNode(id model.CanonicalID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, id)
	delete(s.neighbors, id)
	return nil
}

func (s *Store) UpsertEdge(edge model.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := edge.Key()
	s.edges[key] = edge
	s.addNeighborLocked(edge.From, key)
	s.addNeighborLocked(edge.To, key)
	return nil
}

func (s *Store) DeleteEdge(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	edge, ok := s.edges[key]
	if ok {
		s.removeNeighborLocked(edge.From, key)
		s.removeNeighborLocked(edge.To, key)
	}
	delete(s.edges, key)
	return nil
}

func (s *Store) GetNode(id model.CanonicalID) (model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[id]
	return node, ok
}

func (s *Store) GetEdge(key string) (model.Edge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	edge, ok := s.edges[key]
	return edge, ok
}

func (s *Store) ListNodes() []model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		out = append(out, node)
	}
	return out
}

func (s *Store) ListEdges() []model.Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Edge, 0, len(s.edges))
	for _, edge := range s.edges {
		out = append(out, edge)
	}
	return out
}

func (s *Store) AddNeighbor(from model.CanonicalID, edgeKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addNeighborLocked(from, edgeKey)
}

func (s *Store) RemoveNeighbor(from model.CanonicalID, edgeKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeNeighborLocked(from, edgeKey)
}

func (s *Store) NeighborKeys(from model.CanonicalID) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := s.neighbors[from]
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	return out
}

func (s *Store) addNeighborLocked(from model.CanonicalID, edgeKey string) {
	if _, ok := s.neighbors[from]; !ok {
		s.neighbors[from] = make(map[string]struct{})
	}
	s.neighbors[from][edgeKey] = struct{}{}
}

func (s *Store) removeNeighborLocked(from model.CanonicalID, edgeKey string) {
	if _, ok := s.neighbors[from]; !ok {
		return
	}
	delete(s.neighbors[from], edgeKey)
	if len(s.neighbors[from]) == 0 {
		delete(s.neighbors, from)
	}
}
