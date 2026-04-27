package query

import (
	"sort"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
)

func entityToDiagnosticNode(entity ontology.Entity) api.DiagnosticNode {
	return api.DiagnosticNode{
		CanonicalID: entity.ID.String(),
		Kind:        api.NodeKind(entity.Kind),
		SourceKind:  entity.SourceKind,
		Name:        entity.Name,
		Namespace:   entity.Namespace,
		Attributes:  entity.Attributes,
	}
}

func relationToDiagnosticEdge(relation ontology.Relation) api.DiagnosticEdge {
	return api.DiagnosticEdge{
		From: relation.From.String(),
		To:   relation.To.String(),
		Kind: api.EdgeKind(relation.Kind),
		Provenance: api.EdgeProvenance{
			SourceType: api.EdgeSourceType(relation.Provenance.SourceType),
			State:      api.EdgeState(relation.Provenance.State),
			Resolver:   relation.Provenance.Resolver,
			LastSeenAt: relation.Provenance.LastSeenAt,
			Confidence: relation.Provenance.Confidence,
		},
	}
}

func sortedDiagnosticNodes(nodes map[model.CanonicalID]ontology.Entity) []api.DiagnosticNode {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	out := make([]api.DiagnosticNode, 0, len(ids))
	for _, id := range ids {
		out = append(out, entityToDiagnosticNode(nodes[model.CanonicalID(id)]))
	}
	return out
}

func sortedDiagnosticEdges(edges map[string]ontology.Relation) []api.DiagnosticEdge {
	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]api.DiagnosticEdge, 0, len(keys))
	for _, key := range keys {
		out = append(out, relationToDiagnosticEdge(edges[key]))
	}
	return out
}

func relationKey(relation ontology.Relation) string {
	return relation.From.String() + "|" + relation.Kind + "|" + relation.To.String()
}

func mapKeys(values map[model.CanonicalID]struct{}) []model.CanonicalID {
	out := make([]model.CanonicalID, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
