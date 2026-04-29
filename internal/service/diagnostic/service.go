package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type Service struct {
	kernel *graph.Kernel
}

const (
	DefaultDiagnosticMaxNodes = 200
	DefaultDiagnosticMaxEdges = 400
	MaxRankedEvidence         = 20
)

func NewService(kernel *graph.Kernel) *Service {
	return &Service{kernel: kernel}
}

func DefaultExpansionPolicy() api.ExpansionPolicy {
	return api.ExpansionPolicy{
		MaxDepth:               2,
		StorageMaxDepth:        5,
		MaxNodes:               DefaultDiagnosticMaxNodes,
		MaxEdges:               DefaultDiagnosticMaxEdges,
		TerminalNodeKinds:      DefaultTerminalNodeKinds(),
		IncludeSiblingPods:     true,
		IncludeRBAC:            true,
		IncludeEvents:          true,
		IncludeWebhookEvidence: true,
		IncludeStorage:         true,
		IncludeOCI:             true,
	}
}

func DefaultTerminalNodeKinds() []api.NodeKind {
	return []api.NodeKind{
		api.NodeKindService,
		api.NodeKindConfigMap,
		api.NodeKindSecret,
		api.NodeKindServiceAccount,
		api.NodeKindNode,
		api.NodeKindRoleBinding,
		api.NodeKindClusterRoleBinding,
		api.NodeKindEvent,
		api.NodeKindImage,
		api.NodeKindOCIArtifactMetadata,
		api.NodeKindWebhookConfig,
	}
}

func (s *Service) GetDiagnosticSubgraph(entry api.EntryRef, policy api.ExpansionPolicy) (api.DiagnosticSubgraph, error) {
	return s.GetDiagnosticSubgraphContext(context.Background(), entry, policy)
}

func (s *Service) GetDiagnosticSubgraphContext(ctx context.Context, entry api.EntryRef, policy api.ExpansionPolicy) (api.DiagnosticSubgraph, error) {
	if entry.Kind == "" {
		return api.DiagnosticSubgraph{}, errors.New("entry kind is required")
	}
	if entry.CanonicalID == "" {
		return api.DiagnosticSubgraph{}, errors.New("canonical entry id is required for phase-1 service")
	}
	if policy.MaxDepth <= 0 {
		policy.MaxDepth = 1
	}
	if policy.StorageMaxDepth <= 0 {
		policy.StorageMaxDepth = policy.MaxDepth
	}
	budget := diagnosticBudget(policy)

	rootID := model.CanonicalID(entry.CanonicalID)
	rootNode, ok := s.kernel.GetNode(rootID)
	if !ok {
		return api.DiagnosticSubgraph{}, errors.New("entry node not found")
	}

	nodes := []api.DiagnosticNode{toAPINode(rootNode)}
	edges := make([]api.DiagnosticEdge, 0)
	seenNodes := map[string]struct{}{rootNode.ID.String(): {}}
	seenEdges := map[string]struct{}{}
	truncationReasons := map[string]struct{}{}
	frontier := []model.CanonicalID{rootID}
	maxTraversalDepth := policy.MaxDepth
	if policy.IncludeStorage && policy.StorageMaxDepth > maxTraversalDepth {
		maxTraversalDepth = policy.StorageMaxDepth
	}
	terminalKinds := terminalNodeKindSet(policy)

	for depth := 0; depth < maxTraversalDepth; depth++ {
		if err := ctx.Err(); err != nil {
			return api.DiagnosticSubgraph{}, err
		}
		nextFrontier := make([]model.CanonicalID, 0)
		for _, current := range frontier {
			if err := ctx.Err(); err != nil {
				return api.DiagnosticSubgraph{}, err
			}
			currentNode, currentOK := s.kernel.GetNode(current)
			if currentOK && s.isTerminalNode(currentNode, rootID, rootNode.Kind, terminalKinds, policy.ExpandTerminalNodes) {
				continue
			}
			for _, edge := range s.kernel.Neighbors(current) {
				if err := ctx.Err(); err != nil {
					return api.DiagnosticSubgraph{}, err
				}
				if !s.shouldTraverseFrom(current, currentNode, currentOK, rootNode, edge, depth, policy) {
					continue
				}
				newNodes := make([]model.Node, 0, 2)
				for _, nodeID := range []model.CanonicalID{edge.From, edge.To} {
					if _, seen := seenNodes[nodeID.String()]; seen {
						continue
					}
					node, ok := s.kernel.GetNode(nodeID)
					if !ok {
						continue
					}
					newNodes = append(newNodes, node)
				}
				if len(nodes)+len(newNodes) > budget.MaxNodes {
					truncationReasons["maxNodes"] = struct{}{}
					continue
				}
				if _, seen := seenEdges[edge.Key()]; !seen {
					if len(edges) >= budget.MaxEdges {
						truncationReasons["maxEdges"] = struct{}{}
						continue
					}
					edges = append(edges, toAPIEdge(edge))
					seenEdges[edge.Key()] = struct{}{}
				}
				for _, node := range newNodes {
					nodes = append(nodes, toAPINode(node))
					seenNodes[node.ID.String()] = struct{}{}
					nextFrontier = append(nextFrontier, node.ID)
				}
			}
		}
		frontier = nextFrontier
	}

	now := time.Now().UTC()
	budget.NodeCount = len(nodes)
	budget.EdgeCount = len(edges)
	budget.TruncationReasons = sortedReasons(truncationReasons)
	budget.Truncated = len(budget.TruncationReasons) > 0
	explanations := summarizeEvidence(nodes)
	return api.DiagnosticSubgraph{
		Entry:          entry,
		Nodes:          nodes,
		Edges:          edges,
		CollectedAt:    &now,
		Explanation:    explanations,
		Warnings:       diagnosticWarnings(budget),
		Partial:        budget.Truncated,
		Budgets:        budget,
		RankedEvidence: rankEvidence(nodes),
	}, nil
}

func (s *Service) GetDiagnosticSubgraphByPod(namespace, name string, policy api.ExpansionPolicy) (api.DiagnosticSubgraph, error) {
	for _, node := range s.kernel.ListNodes() {
		if node.Kind != model.NodeKindPod || node.Namespace != namespace || node.Name != name {
			continue
		}
		entry := api.EntryRef{
			Kind:        api.NodeKindPod,
			CanonicalID: node.ID.String(),
			Namespace:   namespace,
			Name:        name,
		}
		return s.GetDiagnosticSubgraph(entry, policy)
	}
	return api.DiagnosticSubgraph{}, errors.New("pod entry not found")
}

func (s *Service) FindNode(kind model.NodeKind, namespace, name string) (model.Node, bool, bool) {
	if s == nil || s.kernel == nil || name == "" {
		return model.Node{}, false, false
	}
	var found model.Node
	ok := false
	for _, node := range s.kernel.ListNodes() {
		if node.Kind != kind || node.Name != name {
			continue
		}
		if namespace != "" && node.Namespace != namespace {
			continue
		}
		if ok {
			return model.Node{}, false, true
		}
		found = node
		ok = true
	}
	return found, ok, false
}

func toAPINode(node model.Node) api.DiagnosticNode {
	return api.DiagnosticNode{
		CanonicalID: node.ID.String(),
		Kind:        api.NodeKind(node.Kind),
		SourceKind:  node.SourceKind,
		Name:        node.Name,
		Namespace:   node.Namespace,
		Attributes:  node.Attributes,
	}
}

func toAPIEdge(edge model.Edge) api.DiagnosticEdge {
	return api.DiagnosticEdge{
		From: edge.From.String(),
		To:   edge.To.String(),
		Kind: api.EdgeKind(edge.Kind),
		Provenance: api.EdgeProvenance{
			SourceType: api.EdgeSourceType(edge.Provenance.SourceType),
			State:      api.EdgeState(edge.Provenance.State),
			Resolver:   edge.Provenance.Resolver,
			LastSeenAt: edge.Provenance.LastSeenAt,
			Confidence: edge.Provenance.Confidence,
		},
	}
}

func summarizeEvidence(nodes []api.DiagnosticNode) []string {
	explanations := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != api.NodeKindEvent {
			continue
		}
		reason, _ := node.Attributes["reason"].(string)
		message, _ := node.Attributes["message"].(string)
		if reason == "" && message == "" {
			continue
		}
		if message != "" {
			explanations = append(explanations, "event: "+reason+" - "+message)
		} else {
			explanations = append(explanations, "event: "+reason)
		}
	}
	return explanations
}

func diagnosticBudget(policy api.ExpansionPolicy) api.DiagnosticBudget {
	maxNodes := policy.MaxNodes
	if maxNodes <= 0 {
		maxNodes = DefaultDiagnosticMaxNodes
	}
	if maxNodes < 1 {
		maxNodes = 1
	}
	maxEdges := policy.MaxEdges
	if maxEdges <= 0 {
		maxEdges = DefaultDiagnosticMaxEdges
	}
	return api.DiagnosticBudget{
		MaxDepth:        policy.MaxDepth,
		StorageMaxDepth: policy.StorageMaxDepth,
		MaxNodes:        maxNodes,
		MaxEdges:        maxEdges,
	}
}

func sortedReasons(reasons map[string]struct{}) []string {
	if len(reasons) == 0 {
		return nil
	}
	out := make([]string, 0, len(reasons))
	for reason := range reasons {
		out = append(out, reason)
	}
	sort.Strings(out)
	return out
}

func diagnosticWarnings(budget api.DiagnosticBudget) []api.DiagnosticWarning {
	if !budget.Truncated {
		return nil
	}
	return []api.DiagnosticWarning{{
		Code:       "diagnostic_budget_exceeded",
		Severity:   "warning",
		Message:    fmt.Sprintf("diagnostic graph was truncated by budget: %s", strings.Join(budget.TruncationReasons, ",")),
		Source:     "diagnostic",
		NextAction: "rerun with a narrower namespace/depth or raise --max-nodes/--max-edges",
	}}
}

func rankEvidence(nodes []api.DiagnosticNode) []api.RankedEvidence {
	items := make([]api.RankedEvidence, 0)
	for _, node := range nodes {
		if node.Kind != api.NodeKindEvent {
			continue
		}
		reason, _ := node.Attributes["reason"].(string)
		message, _ := node.Attributes["message"].(string)
		if reason == "" && message == "" {
			continue
		}
		severity, score := classifyEventEvidence(reason, message)
		items = append(items, api.RankedEvidence{
			Source:     "event",
			NodeID:     node.CanonicalID,
			Kind:       string(node.Kind),
			Severity:   severity,
			Reason:     reason,
			Message:    message,
			Confidence: "observed",
			Score:      score,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Reason != items[j].Reason {
			return items[i].Reason < items[j].Reason
		}
		return items[i].NodeID < items[j].NodeID
	})
	if len(items) > MaxRankedEvidence {
		items = items[:MaxRankedEvidence]
	}
	for i := range items {
		items[i].Rank = i + 1
	}
	return items
}

func classifyEventEvidence(reason, message string) (string, float64) {
	text := strings.ToLower(reason + " " + message)
	for _, token := range []string{"fail", "error", "backoff", "forbidden", "denied", "timeout", "unhealthy", "invalid"} {
		if strings.Contains(text, token) {
			return "warning", 90
		}
	}
	if reason != "" || message != "" {
		return "info", 50
	}
	return "unknown", 0
}

func shouldTraverse(kind model.EdgeKind, depth int, policy api.ExpansionPolicy) bool {
	if kind == model.EdgeKindMountsPVC ||
		kind == model.EdgeKindBoundToPV ||
		kind == model.EdgeKindUsesStorageClass ||
		kind == model.EdgeKindProvisionedByCSIDriver ||
		kind == model.EdgeKindImplementedByCSIController ||
		kind == model.EdgeKindImplementedByCSINodeAgent ||
		kind == model.EdgeKindManagedByCSIController ||
		kind == model.EdgeKindServedByCSINodeAgent {
		if !policy.IncludeStorage {
			return false
		}
		return depth < policy.StorageMaxDepth
	}
	return depth < policy.MaxDepth
}

func (s *Service) shouldTraverseFrom(current model.CanonicalID, currentNode model.Node, currentOK bool, rootNode model.Node, edge model.Edge, depth int, policy api.ExpansionPolicy) bool {
	if !shouldTraverse(edge.Kind, depth, policy) {
		return false
	}
	if !currentOK {
		return true
	}
	switch currentNode.Kind {
	case model.NodeKindStorageClass:
		return edge.From == current && edge.Kind == model.EdgeKindProvisionedByCSIDriver
	case model.NodeKindPV:
		if edge.Kind == model.EdgeKindServedByCSINodeAgent && rootNode.Kind == model.NodeKindPod {
			return edge.From == current && s.csiAgentMatchesRootPodNode(edge.To, rootNode)
		}
		return true
	case model.NodeKindCSIDriver:
		if edge.From != current {
			return false
		}
		if edge.Kind == model.EdgeKindImplementedByCSIController {
			return true
		}
		return edge.Kind == model.EdgeKindImplementedByCSINodeAgent && shouldTraverseDriverNodeAgents(rootNode.Kind)
	default:
		return true
	}
}

func (s *Service) csiAgentMatchesRootPodNode(agentID model.CanonicalID, rootNode model.Node) bool {
	rootNodeName, _ := rootNode.Attributes["nodeName"].(string)
	if rootNodeName == "" || s.kernel == nil {
		return false
	}
	agent, ok := s.kernel.GetNode(agentID)
	if !ok {
		return false
	}
	agentNodeName, _ := agent.Attributes["nodeName"].(string)
	return agentNodeName == rootNodeName
}

func shouldTraverseDriverNodeAgents(rootKind model.NodeKind) bool {
	switch rootKind {
	case model.NodeKindPod, model.NodeKindWorkload, model.NodeKindPVC, model.NodeKindPV:
		return false
	default:
		return true
	}
}

func terminalNodeKindSet(policy api.ExpansionPolicy) map[model.NodeKind]struct{} {
	if policy.ExpandTerminalNodes || len(policy.TerminalNodeKinds) == 0 {
		return nil
	}
	out := make(map[model.NodeKind]struct{}, len(policy.TerminalNodeKinds))
	for _, kind := range policy.TerminalNodeKinds {
		out[model.NodeKind(kind)] = struct{}{}
	}
	return out
}

func (s *Service) isTerminalNode(node model.Node, rootID model.CanonicalID, rootKind model.NodeKind, terminalKinds map[model.NodeKind]struct{}, expandTerminalNodes bool) bool {
	if expandTerminalNodes {
		return false
	}
	if s.isTerminalCSINode(node) {
		return true
	}
	if node.ID != rootID && rootKind == model.NodeKindPod && node.Kind == model.NodeKindPod {
		return true
	}
	if len(terminalKinds) == 0 {
		return false
	}
	_, ok := terminalKinds[node.Kind]
	return ok
}

func (s *Service) isTerminalCSINode(node model.Node) bool {
	if node.Kind != model.NodeKindPod || s.kernel == nil {
		return false
	}
	for _, edge := range s.kernel.Neighbors(node.ID) {
		switch edge.Kind {
		case model.EdgeKindImplementedByCSIController,
			model.EdgeKindImplementedByCSINodeAgent,
			model.EdgeKindManagedByCSIController,
			model.EdgeKindServedByCSINodeAgent:
			return true
		}
	}
	return false
}
