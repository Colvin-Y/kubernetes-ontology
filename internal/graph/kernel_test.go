package graph

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestKernelNeighborsUsesIndexedEdgeLookup(t *testing.T) {
	from := model.CanonicalID("cluster-a/core/Pod/default/frontend/p1/_")
	to := model.CanonicalID("cluster-a/core/Service/default/frontend/s1/_")
	edge := model.Edge{From: from, To: to, Kind: model.EdgeKindRelatedTo}

	kernel := NewKernel(
		&lookupOnlyStore{edges: map[string]model.Edge{edge.Key(): edge}},
		staticIndex{keys: []string{edge.Key(), "stale-key"}},
	)

	neighbors := kernel.Neighbors(from)
	if len(neighbors) != 1 {
		t.Fatalf("expected one indexed neighbor, got %d", len(neighbors))
	}
	if neighbors[0].Key() != edge.Key() {
		t.Fatalf("unexpected neighbor: got %s want %s", neighbors[0].Key(), edge.Key())
	}
}

type lookupOnlyStore struct {
	edges map[string]model.Edge
}

func (s *lookupOnlyStore) UpsertNode(model.Node) error        { return nil }
func (s *lookupOnlyStore) DeleteNode(model.CanonicalID) error { return nil }
func (s *lookupOnlyStore) UpsertEdge(model.Edge) error        { return nil }
func (s *lookupOnlyStore) DeleteEdge(string) error            { return nil }
func (s *lookupOnlyStore) GetNode(model.CanonicalID) (model.Node, bool) {
	return model.Node{}, false
}
func (s *lookupOnlyStore) GetEdge(key string) (model.Edge, bool) {
	edge, ok := s.edges[key]
	return edge, ok
}
func (s *lookupOnlyStore) ListNodes() []model.Node { return nil }
func (s *lookupOnlyStore) ListEdges() []model.Edge {
	panic("Neighbors should use indexed edge lookup, not scan all edges")
}

type staticIndex struct {
	keys []string
}

func (s staticIndex) AddNeighbor(model.CanonicalID, string)    {}
func (s staticIndex) RemoveNeighbor(model.CanonicalID, string) {}
func (s staticIndex) NeighborKeys(model.CanonicalID) []string  { return s.keys }
