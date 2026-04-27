package query

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
)

func TestCollapseGraphExpansion(t *testing.T) {
	doc := GraphStateDocument{
		BaseGraph: &GraphStateGraph{
			Entry: json.RawMessage(`{"kind":"Pod","name":"pod-a"}`),
			Nodes: []api.DiagnosticNode{
				{CanonicalID: "pod-a", Kind: api.NodeKindPod, Name: "pod-a"},
				{CanonicalID: "node-a", Kind: api.NodeKindNode, Name: "node-a"},
			},
			Edges: []api.DiagnosticEdge{
				{From: "pod-a", To: "node-a", Kind: api.EdgeKindScheduledOn},
			},
			Explanation: []string{"base graph"},
		},
		Expansions: []GraphExpansion{
			{
				ExpandedFrom: "node-a",
				GraphStateGraph: GraphStateGraph{
					Nodes: []api.DiagnosticNode{
						{CanonicalID: "node-a", Kind: api.NodeKindNode, Name: "node-a"},
						{CanonicalID: "pod-b", Kind: api.NodeKindPod, Name: "pod-b"},
					},
					Edges: []api.DiagnosticEdge{
						{From: "pod-b", To: "node-a", Kind: api.EdgeKindScheduledOn},
					},
					Explanation: []string{"expanded node-a"},
				},
			},
		},
	}

	collapsed, err := CollapseGraphExpansion(doc, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(collapsed.Expansions) != 0 {
		t.Fatalf("expected expansion layer to be removed, got %+v", collapsed.Expansions)
	}
	if hasGraphStateNode(collapsed.Nodes, "pod-b") {
		t.Fatalf("expected expansion-only node to be removed, got %+v", collapsed.Nodes)
	}
	if !hasGraphStateNode(collapsed.Nodes, "pod-a") || !hasGraphStateNode(collapsed.Nodes, "node-a") {
		t.Fatalf("expected base nodes to remain, got %+v", collapsed.Nodes)
	}
	if len(collapsed.Edges) != 1 || collapsed.Edges[0].From != "pod-a" {
		t.Fatalf("expected only base edge to remain, got %+v", collapsed.Edges)
	}
}

func TestCollapseGraphExpansionPreservesOverlappingLayers(t *testing.T) {
	doc := GraphStateDocument{
		BaseGraph: &GraphStateGraph{
			Nodes: []api.DiagnosticNode{{CanonicalID: "root", Kind: api.NodeKindPod, Name: "root"}},
		},
		Expansions: []GraphExpansion{
			{
				ExpandedFrom: "root",
				GraphStateGraph: GraphStateGraph{
					Nodes: []api.DiagnosticNode{{CanonicalID: "shared", Kind: api.NodeKindNode, Name: "shared"}},
				},
			},
			{
				ExpandedFrom: "shared",
				GraphStateGraph: GraphStateGraph{
					Nodes: []api.DiagnosticNode{
						{CanonicalID: "shared", Kind: api.NodeKindNode, Name: "shared"},
						{CanonicalID: "leaf", Kind: api.NodeKindPod, Name: "leaf"},
					},
				},
			},
		},
	}

	collapsed, err := CollapseGraphExpansion(doc, "root")
	if err != nil {
		t.Fatal(err)
	}
	if !hasGraphStateNode(collapsed.Nodes, "shared") || !hasGraphStateNode(collapsed.Nodes, "leaf") {
		t.Fatalf("expected remaining expansion to preserve its nodes, got %+v", collapsed.Nodes)
	}
	if len(collapsed.Expansions) != 1 || collapsed.Expansions[0].ExpandedFrom != "shared" {
		t.Fatalf("expected only shared expansion to remain, got %+v", collapsed.Expansions)
	}
}

func TestCollapseGraphExpansionRequiresExpansionState(t *testing.T) {
	_, err := CollapseGraphExpansion(GraphStateDocument{}, "node-a")
	if !errors.Is(err, ErrInvalidDiagnosticQuery) {
		t.Fatalf("expected invalid query error, got %v", err)
	}
}

func hasGraphStateNode(nodes []api.DiagnosticNode, id string) bool {
	for _, node := range nodes {
		if node.CanonicalID == id {
			return true
		}
	}
	return false
}
