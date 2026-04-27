package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
)

type GraphStateDocument struct {
	Source      string               `json:"source,omitempty"`
	Mode        string               `json:"mode,omitempty"`
	Status      json.RawMessage      `json:"status,omitempty"`
	Entry       json.RawMessage      `json:"entry,omitempty"`
	Nodes       []api.DiagnosticNode `json:"nodes,omitempty"`
	Edges       []api.DiagnosticEdge `json:"edges,omitempty"`
	CollectedAt json.RawMessage      `json:"collectedAt,omitempty"`
	Explanation []string             `json:"explanation,omitempty"`
	BaseGraph   *GraphStateGraph     `json:"baseGraph,omitempty"`
	Expansions  []GraphExpansion     `json:"expansions,omitempty"`
	ExportedAt  string               `json:"exportedAt,omitempty"`
}

type GraphStateGraph struct {
	Source      string               `json:"source,omitempty"`
	Mode        string               `json:"mode,omitempty"`
	Status      json.RawMessage      `json:"status,omitempty"`
	Entry       json.RawMessage      `json:"entry,omitempty"`
	Nodes       []api.DiagnosticNode `json:"nodes,omitempty"`
	Edges       []api.DiagnosticEdge `json:"edges,omitempty"`
	CollectedAt json.RawMessage      `json:"collectedAt,omitempty"`
	Explanation []string             `json:"explanation,omitempty"`
}

type GraphExpansion struct {
	GraphStateGraph
	ExpandedFrom string `json:"expandedFrom"`
	Depth        int    `json:"depth,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func CollapseGraphExpansion(doc GraphStateDocument, entityID string) (GraphStateDocument, error) {
	if entityID == "" {
		return GraphStateDocument{}, fmt.Errorf("%w: entity-id is required with --collapse-node", ErrInvalidDiagnosticQuery)
	}
	if doc.BaseGraph == nil {
		return GraphStateDocument{}, fmt.Errorf("%w: graph file does not include expansion state", ErrInvalidDiagnosticQuery)
	}
	remaining := make([]GraphExpansion, 0, len(doc.Expansions))
	removed := false
	for _, expansion := range doc.Expansions {
		if expansion.ExpandedFrom == entityID {
			removed = true
			continue
		}
		remaining = append(remaining, expansion)
	}
	if !removed {
		return GraphStateDocument{}, fmt.Errorf("%w: selected node has no expanded layer to collapse", ErrInvalidDiagnosticQuery)
	}

	out := GraphStateDocument{
		Source:      doc.BaseGraph.Source,
		Mode:        doc.BaseGraph.Mode,
		Status:      cloneRaw(doc.BaseGraph.Status),
		Entry:       cloneRaw(doc.BaseGraph.Entry),
		CollectedAt: cloneRaw(doc.BaseGraph.CollectedAt),
		Explanation: append([]string{}, doc.BaseGraph.Explanation...),
		BaseGraph:   cloneGraph(doc.BaseGraph),
		Expansions:  remaining,
		ExportedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	nodeByID := make(map[string]api.DiagnosticNode, len(doc.BaseGraph.Nodes))
	for _, node := range doc.BaseGraph.Nodes {
		nodeByID[node.CanonicalID] = node
	}
	edgeByKey := make(map[string]api.DiagnosticEdge, len(doc.BaseGraph.Edges))
	for _, edge := range doc.BaseGraph.Edges {
		edgeByKey[diagnosticEdgeKey(edge)] = edge
	}
	for _, expansion := range remaining {
		for _, node := range expansion.Nodes {
			nodeByID[node.CanonicalID] = node
		}
		for _, edge := range expansion.Edges {
			edgeByKey[diagnosticEdgeKey(edge)] = edge
		}
		out.Explanation = append(out.Explanation, expansion.Explanation...)
		if len(expansion.CollectedAt) > 0 {
			out.CollectedAt = cloneRaw(expansion.CollectedAt)
		}
	}
	out.Explanation = append(out.Explanation, fmt.Sprintf("collapsed %s", entityID))
	out.Nodes = sortedGraphStateNodes(nodeByID)
	out.Edges = sortedGraphStateEdges(edgeByKey)
	return out, nil
}

func cloneGraph(graph *GraphStateGraph) *GraphStateGraph {
	if graph == nil {
		return nil
	}
	out := *graph
	out.Status = cloneRaw(graph.Status)
	out.Entry = cloneRaw(graph.Entry)
	out.CollectedAt = cloneRaw(graph.CollectedAt)
	out.Nodes = append([]api.DiagnosticNode{}, graph.Nodes...)
	out.Edges = append([]api.DiagnosticEdge{}, graph.Edges...)
	out.Explanation = append([]string{}, graph.Explanation...)
	return &out
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage{}, raw...)
}

func sortedGraphStateNodes(nodes map[string]api.DiagnosticNode) []api.DiagnosticNode {
	out := make([]api.DiagnosticNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].CanonicalID < out[j].CanonicalID
	})
	return out
}

func sortedGraphStateEdges(edges map[string]api.DiagnosticEdge) []api.DiagnosticEdge {
	out := make([]api.DiagnosticEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool {
		return diagnosticEdgeKey(out[i]) < diagnosticEdgeKey(out[j])
	})
	return out
}

func diagnosticEdgeKey(edge api.DiagnosticEdge) string {
	return edge.From + "\x00" + edge.To + "\x00" + string(edge.Kind)
}
