package query

import (
	"errors"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
)

type Freshness struct {
	Ready                      bool   `json:"ready"`
	Phase                      string `json:"phase,omitempty"`
	Cluster                    string `json:"cluster,omitempty"`
	NodeCount                  int    `json:"nodeCount"`
	EdgeCount                  int    `json:"edgeCount"`
	LastRefreshAt              string `json:"lastRefreshAt,omitempty"`
	LastBootstrapAt            string `json:"lastBootstrapAt,omitempty"`
	LastAppliedChangeKind      string `json:"lastAppliedChangeKind,omitempty"`
	LastAppliedChangeNamespace string `json:"lastAppliedChangeNamespace,omitempty"`
	LastAppliedChangeName      string `json:"lastAppliedChangeName,omitempty"`
	LastAppliedChangeType      string `json:"lastAppliedChangeType,omitempty"`
	LastAppliedChangeAt        string `json:"lastAppliedChangeAt,omitempty"`
	LastStrategy               string `json:"lastStrategy,omitempty"`
	LastError                  string `json:"lastError,omitempty"`
}

func FreshnessFromRuntimeStatus(status RuntimeStatus) Freshness {
	lastRefreshAt := status.LastAppliedChangeAt
	if lastRefreshAt == "" {
		lastRefreshAt = status.LastBootstrapAt
	}
	return Freshness{
		Ready:                      status.Ready,
		Phase:                      status.Phase,
		Cluster:                    status.Cluster,
		NodeCount:                  status.NodeCount,
		EdgeCount:                  status.EdgeCount,
		LastRefreshAt:              lastRefreshAt,
		LastBootstrapAt:            status.LastBootstrapAt,
		LastAppliedChangeKind:      status.LastAppliedChangeKind,
		LastAppliedChangeNamespace: status.LastAppliedChangeNS,
		LastAppliedChangeName:      status.LastAppliedChangeName,
		LastAppliedChangeType:      status.LastAppliedChangeType,
		LastAppliedChangeAt:        status.LastAppliedChangeAt,
		LastStrategy:               status.LastStrategy,
		LastError:                  status.LastError,
	}
}

type DiagnosticSubgraphResponse struct {
	api.DiagnosticSubgraph
	NodeCount int       `json:"nodeCount"`
	EdgeCount int       `json:"edgeCount"`
	Freshness Freshness `json:"freshness"`
}

func NewDiagnosticSubgraphResponse(result api.DiagnosticSubgraph, status RuntimeStatus) DiagnosticSubgraphResponse {
	return DiagnosticSubgraphResponse{
		DiagnosticSubgraph: result,
		NodeCount:          len(result.Nodes),
		EdgeCount:          len(result.Edges),
		Freshness:          FreshnessFromRuntimeStatus(status),
	}
}

type GraphSubgraphResponse struct {
	api.GraphSubgraph
	NodeCount int       `json:"nodeCount"`
	EdgeCount int       `json:"edgeCount"`
	Freshness Freshness `json:"freshness"`
}

func NewGraphSubgraphResponse(result api.GraphSubgraph, status RuntimeStatus) GraphSubgraphResponse {
	return GraphSubgraphResponse{
		GraphSubgraph: result,
		NodeCount:     len(result.Nodes),
		EdgeCount:     len(result.Edges),
		Freshness:     FreshnessFromRuntimeStatus(status),
	}
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	Code      string `json:"code"`
	Status    int    `json:"status,omitempty"`
	Retryable bool   `json:"retryable"`
	Source    string `json:"source,omitempty"`
}

func NewErrorResponse(code string, status int, err error, retryable bool) ErrorResponse {
	if err == nil {
		err = errors.New("unknown error")
	}
	return ErrorResponse{
		Error:     err.Error(),
		Message:   err.Error(),
		Code:      code,
		Status:    status,
		Retryable: retryable,
	}
}
