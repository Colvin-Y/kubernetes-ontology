package diagnostic

import (
	"context"
	"errors"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type Service struct {
	kernel *graph.Kernel
}

func NewService(kernel *graph.Kernel) *Service {
	return &Service{kernel: kernel}
}

func DefaultExpansionPolicy() api.ExpansionPolicy {
	return api.ExpansionPolicy{
		MaxDepth:               2,
		StorageMaxDepth:        5,
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

	rootID := model.CanonicalID(entry.CanonicalID)
	rootNode, ok := s.kernel.GetNode(rootID)
	if !ok {
		return api.DiagnosticSubgraph{}, errors.New("entry node not found")
	}

	nodes := []api.DiagnosticNode{toAPINode(rootNode)}
	edges := make([]api.DiagnosticEdge, 0)
	seenNodes := map[string]struct{}{rootNode.ID.String(): {}}
	seenEdges := map[string]struct{}{}
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
				if !shouldTraverseFrom(current, currentNode, currentOK, edge, depth, policy) {
					continue
				}
				if _, seen := seenEdges[edge.Key()]; !seen {
					edges = append(edges, toAPIEdge(edge))
					seenEdges[edge.Key()] = struct{}{}
				}
				for _, nodeID := range []model.CanonicalID{edge.From, edge.To} {
					if _, seen := seenNodes[nodeID.String()]; seen {
						continue
					}
					node, ok := s.kernel.GetNode(nodeID)
					if !ok {
						continue
					}
					nodes = append(nodes, toAPINode(node))
					seenNodes[nodeID.String()] = struct{}{}
					nextFrontier = append(nextFrontier, nodeID)
				}
			}
		}
		frontier = nextFrontier
	}

	now := time.Now().UTC()
	explanations := summarizeEvidence(nodes)
	return api.DiagnosticSubgraph{
		Entry:       entry,
		Nodes:       nodes,
		Edges:       edges,
		CollectedAt: &now,
		Explanation: explanations,
	}, nil
}

func (s *Service) GetDiagnosticSubgraphByPod(namespace, name string, policy api.ExpansionPolicy) (api.DiagnosticSubgraph, error) {
	for _, node := range s.kernel.ListNodes() {
		if node.Kind != model.NodeKindPod || node.Namespace != namespace || node.Name != name {
			continue
		}
		entry := api.EntryRef{
			Kind:        api.EntryKindPod,
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

func shouldTraverseFrom(current model.CanonicalID, currentNode model.Node, currentOK bool, edge model.Edge, depth int, policy api.ExpansionPolicy) bool {
	if !shouldTraverse(edge.Kind, depth, policy) {
		return false
	}
	if !currentOK {
		return true
	}
	switch currentNode.Kind {
	case model.NodeKindStorageClass:
		return edge.From == current && edge.Kind == model.EdgeKindProvisionedByCSIDriver
	case model.NodeKindCSIDriver:
		return edge.From == current &&
			(edge.Kind == model.EdgeKindImplementedByCSIController ||
				edge.Kind == model.EdgeKindImplementedByCSINodeAgent)
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
