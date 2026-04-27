package graph

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

type ReverseIndex struct {
	neighbors map[model.CanonicalID]map[string]struct{}
}

func NewReverseIndex() *ReverseIndex {
	return &ReverseIndex{neighbors: make(map[model.CanonicalID]map[string]struct{})}
}

func (r *ReverseIndex) AddNeighbor(from model.CanonicalID, edgeKey string) {
	if _, ok := r.neighbors[from]; !ok {
		r.neighbors[from] = make(map[string]struct{})
	}
	r.neighbors[from][edgeKey] = struct{}{}
}

func (r *ReverseIndex) RemoveNeighbor(from model.CanonicalID, edgeKey string) {
	if _, ok := r.neighbors[from]; !ok {
		return
	}
	delete(r.neighbors[from], edgeKey)
	if len(r.neighbors[from]) == 0 {
		delete(r.neighbors, from)
	}
}

func (r *ReverseIndex) NeighborKeys(from model.CanonicalID) []string {
	keys := r.neighbors[from]
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	return out
}
