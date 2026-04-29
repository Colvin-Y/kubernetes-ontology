package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/service/diagnostic"
)

var (
	ErrDiagnosticNotReady      = errors.New("diagnostic facade is not ready")
	ErrDiagnosticEntryNotFound = errors.New("diagnostic entry not found")
	ErrInvalidDiagnosticQuery  = errors.New("invalid diagnostic query")
)

const (
	MaxDiagnosticDepth = 10
	MaxDiagnosticNodes = 5000
	MaxDiagnosticEdges = 10000
)

type RuntimeStatus struct {
	Phase                       string
	Cluster                     string
	Ready                       bool
	NodeCount                   int
	EdgeCount                   int
	LastBootstrapAt             string
	LastAppliedChangeKind       string
	LastAppliedChangeNS         string
	LastAppliedChangeName       string
	LastAppliedChangeType       string
	LastAppliedChangeAt         string
	LastStrategy                string
	FullRebuildCount            int
	EventNarrowCount            int
	StorageNarrowCount          int
	ServiceNarrowCount          int
	PodNarrowCount              int
	WorkloadNarrowCount         int
	IdentitySecurityNarrowCount int
	LastError                   string
}

type Facade struct {
	cluster       string
	snapshot      collectk8s.Snapshot
	builder       *graph.Builder
	runtimeStatus RuntimeStatus
	Diagnostic    *diagnostic.Service
}

type DiagnosticOptions struct {
	MaxDepth            int
	StorageMaxDepth     int
	MaxNodes            int
	MaxEdges            int
	TerminalNodeKinds   []api.NodeKind
	ExpandTerminalNodes bool
}

func ValidateDiagnosticOptions(options DiagnosticOptions) error {
	if options.MaxDepth < 0 {
		return fmt.Errorf("%w: maxDepth must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.StorageMaxDepth < 0 {
		return fmt.Errorf("%w: storageMaxDepth must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.MaxDepth > MaxDiagnosticDepth {
		return fmt.Errorf("%w: maxDepth must be <= %d", ErrInvalidDiagnosticQuery, MaxDiagnosticDepth)
	}
	if options.StorageMaxDepth > MaxDiagnosticDepth {
		return fmt.Errorf("%w: storageMaxDepth must be <= %d", ErrInvalidDiagnosticQuery, MaxDiagnosticDepth)
	}
	if options.MaxNodes < 0 {
		return fmt.Errorf("%w: maxNodes must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.MaxEdges < 0 {
		return fmt.Errorf("%w: maxEdges must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.MaxNodes > MaxDiagnosticNodes {
		return fmt.Errorf("%w: maxNodes must be <= %d", ErrInvalidDiagnosticQuery, MaxDiagnosticNodes)
	}
	if options.MaxEdges > MaxDiagnosticEdges {
		return fmt.Errorf("%w: maxEdges must be <= %d", ErrInvalidDiagnosticQuery, MaxDiagnosticEdges)
	}
	for _, kind := range options.TerminalNodeKinds {
		if _, ok := normalizeNodeKind(string(kind)); !ok {
			return fmt.Errorf("%w: unsupported terminal kind %q", ErrInvalidDiagnosticQuery, kind)
		}
	}
	return nil
}

func ParseTerminalNodeKinds(raw string) ([]api.NodeKind, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}
	if strings.EqualFold(raw, "none") {
		return nil, true, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]api.NodeKind, 0, len(parts))
	seen := make(map[api.NodeKind]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kind, ok := normalizeNodeKind(part)
		if !ok {
			return nil, false, fmt.Errorf("%w: unsupported terminal kind %q", ErrInvalidDiagnosticQuery, part)
		}
		if _, exists := seen[kind]; exists {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	return out, false, nil
}

func NewFacade(cluster string, snapshot collectk8s.Snapshot, builder *graph.Builder, diagnosticService *diagnostic.Service) *Facade {
	return &Facade{
		cluster:    cluster,
		snapshot:   snapshot,
		builder:    builder,
		Diagnostic: diagnosticService,
		runtimeStatus: RuntimeStatus{
			Cluster: cluster,
		},
	}
}

func (f *Facade) SetRuntimeStatus(status RuntimeStatus) {
	f.runtimeStatus = status
}

func (f *Facade) SetSnapshot(snapshot collectk8s.Snapshot) {
	f.snapshot = snapshot
}

func (f *Facade) RuntimeStatus() RuntimeStatus {
	return f.runtimeStatus
}

func (f *Facade) FindEntryID(entryKind, namespace, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidDiagnosticQuery)
	}
	normalizedKind, ok := normalizeNodeKind(entryKind)
	if !ok {
		return "", fmt.Errorf("%w: unsupported entry kind %q", ErrInvalidDiagnosticQuery, entryKind)
	}
	switch normalizedKind {
	case api.NodeKindPod:
		for _, pod := range f.snapshot.Pods {
			if pod.Metadata.Namespace == namespace && pod.Metadata.Name == name {
				return model.NewCanonicalID(model.ResourceRef{Cluster: f.cluster, Group: "core", Kind: "Pod", Namespace: namespace, Name: name, UID: pod.Metadata.UID}).String(), nil
			}
		}
	case api.NodeKindWorkload:
		for _, workload := range f.snapshot.Workloads {
			if workload.Metadata.Namespace == namespace && workload.Metadata.Name == name {
				return model.WorkloadID(f.cluster, namespace, workload.ControllerKind, name, workload.Metadata.UID).String(), nil
			}
		}
	}
	if f.Diagnostic != nil {
		lookupNamespace := namespace
		if !isNamespacedDiagnosticKind(normalizedKind) {
			lookupNamespace = ""
		}
		node, found, ambiguous := f.Diagnostic.FindNode(model.NodeKind(normalizedKind), lookupNamespace, name)
		if ambiguous {
			return "", fmt.Errorf("%w: kind=%s name=%s matched multiple nodes; set namespace", ErrInvalidDiagnosticQuery, normalizedKind, name)
		}
		if found {
			return node.ID.String(), nil
		}
	}
	return "", fmt.Errorf("%w: kind=%s namespace=%s name=%s", ErrDiagnosticEntryNotFound, normalizedKind, namespace, name)
}

func (f *Facade) DiagnosticPolicy(options DiagnosticOptions) api.ExpansionPolicy {
	policy := diagnostic.DefaultExpansionPolicy()
	if options.MaxDepth > 0 {
		policy.MaxDepth = options.MaxDepth
	}
	if options.StorageMaxDepth > 0 {
		policy.StorageMaxDepth = options.StorageMaxDepth
	}
	if options.MaxNodes > 0 {
		policy.MaxNodes = options.MaxNodes
	}
	if options.MaxEdges > 0 {
		policy.MaxEdges = options.MaxEdges
	}
	if options.ExpandTerminalNodes {
		policy.ExpandTerminalNodes = true
		policy.TerminalNodeKinds = nil
	} else if len(options.TerminalNodeKinds) > 0 {
		policy.TerminalNodeKinds = append([]api.NodeKind(nil), options.TerminalNodeKinds...)
	}
	return policy
}

func normalizeNodeKind(raw string) (api.NodeKind, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "persistentvolumeclaim", "persistentvolumeclaims":
		return api.NodeKindPVC, true
	case "persistentvolume", "persistentvolumes":
		return api.NodeKindPV, true
	case "storageclasses":
		return api.NodeKindStorageClass, true
	case "csidrivers":
		return api.NodeKindCSIDriver, true
	}
	kinds := []api.NodeKind{
		api.NodeKindCluster,
		api.NodeKindNamespace,
		api.NodeKindWorkload,
		api.NodeKindPod,
		api.NodeKindContainer,
		api.NodeKindNode,
		api.NodeKindService,
		api.NodeKindConfigMap,
		api.NodeKindSecret,
		api.NodeKindServiceAccount,
		api.NodeKindRoleBinding,
		api.NodeKindClusterRoleBinding,
		api.NodeKindPVC,
		api.NodeKindPV,
		api.NodeKindStorageClass,
		api.NodeKindCSIDriver,
		api.NodeKindEvent,
		api.NodeKindImage,
		api.NodeKindOCIArtifactMetadata,
		api.NodeKindWebhookConfig,
	}
	for _, kind := range kinds {
		if strings.EqualFold(raw, string(kind)) {
			return kind, true
		}
	}
	return "", false
}

func isNamespacedDiagnosticKind(kind api.NodeKind) bool {
	switch kind {
	case api.NodeKindPV, api.NodeKindStorageClass, api.NodeKindCSIDriver, api.NodeKindCluster, api.NodeKindNode, api.NodeKindClusterRoleBinding, api.NodeKindWebhookConfig, api.NodeKindImage, api.NodeKindOCIArtifactMetadata:
		return false
	default:
		return true
	}
}

func (f *Facade) QueryDiagnosticSubgraph(entryKind, namespace, name string, options DiagnosticOptions) (api.DiagnosticSubgraph, error) {
	return f.QueryDiagnosticSubgraphContext(context.Background(), entryKind, namespace, name, options)
}

func (f *Facade) QueryDiagnosticSubgraphContext(ctx context.Context, entryKind, namespace, name string, options DiagnosticOptions) (api.DiagnosticSubgraph, error) {
	if err := ValidateDiagnosticOptions(options); err != nil {
		return api.DiagnosticSubgraph{}, err
	}
	normalizedKind, ok := normalizeNodeKind(entryKind)
	if !ok {
		return api.DiagnosticSubgraph{}, fmt.Errorf("%w: unsupported entry kind %q", ErrInvalidDiagnosticQuery, entryKind)
	}
	entryID, err := f.FindEntryID(entryKind, namespace, name)
	if err != nil {
		return api.DiagnosticSubgraph{}, err
	}
	entryNamespace := namespace
	if !isNamespacedDiagnosticKind(normalizedKind) {
		entryNamespace = ""
	}

	result, err := f.Diagnostic.GetDiagnosticSubgraphContext(ctx, api.EntryRef{
		Kind:        normalizedKind,
		CanonicalID: entryID,
		Namespace:   entryNamespace,
		Name:        name,
	}, f.DiagnosticPolicy(options))
	if err != nil {
		return api.DiagnosticSubgraph{}, err
	}
	if len(f.builder.Evidence()) > 0 {
		result.Explanation = append(result.Explanation, f.builder.Evidence()...)
	}
	return result, nil
}
