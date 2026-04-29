package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
)

type Runtime interface {
	RuntimeStatus() query.RuntimeStatus
	Ontology() ontology.Backend
	QueryDiagnosticSubgraph(ctx context.Context, entryKind, namespace, name string, options query.DiagnosticOptions) (api.DiagnosticSubgraph, error)
}

var diagnosticRequestTimeout = 25 * time.Second

func NewHandler(runtime Runtime) http.Handler {
	mux := http.NewServeMux()
	s := &handler{runtime: runtime}
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /entity", s.entity)
	mux.HandleFunc("GET /entities", s.entities)
	mux.HandleFunc("GET /relations", s.relations)
	mux.HandleFunc("GET /neighbors", s.neighbors)
	mux.HandleFunc("GET /expand", s.expand)
	mux.HandleFunc("GET /diagnostic", s.diagnosticGeneric)
	mux.HandleFunc("GET /diagnostic/pod", s.diagnosticPod)
	mux.HandleFunc("GET /diagnostic/workload", s.diagnosticWorkload)
	return mux
}

type handler struct {
	runtime Runtime
}

func (h *handler) healthz(w http.ResponseWriter, r *http.Request) {
	status := h.runtime.RuntimeStatus()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      status.Ready,
		"phase":   status.Phase,
		"cluster": status.Cluster,
	})
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.runtime.RuntimeStatus())
}

func (h *handler) entity(w http.ResponseWriter, r *http.Request) {
	backend, ok := h.backend(w)
	if !ok {
		return
	}
	entity, found, err := backend.FindEntity(r.Context(), ontology.EntityRef{
		ID:        model.CanonicalID(firstNonEmpty(r.URL.Query().Get("entityGlobalId"), r.URL.Query().Get("id"))),
		Kind:      r.URL.Query().Get("kind"),
		Namespace: r.URL.Query().Get("namespace"),
		Name:      r.URL.Query().Get("name"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, errors.New("entity not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entity":    entity,
		"freshness": query.FreshnessFromRuntimeStatus(h.runtime.RuntimeStatus()),
	})
}

func (h *handler) entities(w http.ResponseWriter, r *http.Request) {
	backend, ok := h.backend(w)
	if !ok {
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entities, err := backend.ListEntities(r.Context(), ontology.EntityQuery{
		Kind:      r.URL.Query().Get("kind"),
		Namespace: r.URL.Query().Get("namespace"),
		Name:      r.URL.Query().Get("name"),
		Limit:     limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     entities,
		"count":     len(entities),
		"freshness": query.FreshnessFromRuntimeStatus(h.runtime.RuntimeStatus()),
	})
}

func (h *handler) relations(w http.ResponseWriter, r *http.Request) {
	backend, ok := h.backend(w)
	if !ok {
		return
	}
	relQuery, err := relationQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	relations, err := backend.ListRelations(r.Context(), relQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     relations,
		"count":     len(relations),
		"freshness": query.FreshnessFromRuntimeStatus(h.runtime.RuntimeStatus()),
	})
}

func (h *handler) neighbors(w http.ResponseWriter, r *http.Request) {
	backend, ok := h.backend(w)
	if !ok {
		return
	}
	id := model.CanonicalID(firstNonEmpty(r.URL.Query().Get("entityGlobalId"), r.URL.Query().Get("id")))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("entityGlobalId or id is required"))
		return
	}
	relQuery, err := relationQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	relations, err := backend.Neighbors(r.Context(), id, relQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     relations,
		"count":     len(relations),
		"freshness": query.FreshnessFromRuntimeStatus(h.runtime.RuntimeStatus()),
	})
}

func (h *handler) expand(w http.ResponseWriter, r *http.Request) {
	id := model.CanonicalID(firstNonEmpty(r.URL.Query().Get("entityGlobalId"), r.URL.Query().Get("id")))
	depth, err := parseOptionalExpandDepth(r.URL.Query().Get("depth"), "depth")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	limit, err := parseExpandLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	backend, ok := h.backend(w)
	if !ok {
		return
	}
	result, err := query.ExpandSubgraph(r.Context(), backend, query.ExpandOptions{
		EntityID:     id,
		Depth:        depth,
		Direction:    ontology.Direction(r.URL.Query().Get("direction")),
		RelationKind: r.URL.Query().Get("kind"),
		Limit:        limit,
	})
	if err != nil {
		writeExpandError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, query.NewGraphSubgraphResponse(result, h.runtime.RuntimeStatus()))
}

func (h *handler) diagnosticPod(w http.ResponseWriter, r *http.Request) {
	h.diagnostic(w, r, "Pod")
}

func (h *handler) diagnosticWorkload(w http.ResponseWriter, r *http.Request) {
	h.diagnostic(w, r, "Workload")
}

func (h *handler) diagnosticGeneric(w http.ResponseWriter, r *http.Request) {
	h.diagnostic(w, r, r.URL.Query().Get("kind"))
}

func (h *handler) diagnostic(w http.ResponseWriter, r *http.Request, kind string) {
	if kind == "" {
		writeError(w, http.StatusBadRequest, errors.New("kind is required"))
		return
	}
	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")
	if namespace == "" && (kind == "Pod" || kind == "Workload") {
		writeError(w, http.StatusBadRequest, errors.New("namespace is required"))
		return
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	maxDepth, err := parseOptionalDiagnosticDepth(r.URL.Query().Get("maxDepth"), "maxDepth")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	storageMaxDepth, err := parseOptionalDiagnosticDepth(r.URL.Query().Get("storageMaxDepth"), "storageMaxDepth")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	maxNodes, err := parseOptionalDiagnosticLimit(r.URL.Query().Get("maxNodes"), "maxNodes", query.MaxDiagnosticNodes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	maxEdges, err := parseOptionalDiagnosticLimit(r.URL.Query().Get("maxEdges"), "maxEdges", query.MaxDiagnosticEdges)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	terminalNodeKinds, terminalKindsDisable, err := query.ParseTerminalNodeKinds(r.URL.Query().Get("terminalKinds"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	expandTerminalNodes, err := parseOptionalBool(r.URL.Query().Get("expandTerminalNodes"), "expandTerminalNodes")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if terminalKindsDisable {
		expandTerminalNodes = true
	}
	ctx, cancel := context.WithTimeout(r.Context(), diagnosticRequestTimeout)
	defer cancel()
	result, err := h.runtime.QueryDiagnosticSubgraph(ctx, kind, namespace, name, query.DiagnosticOptions{
		MaxDepth:            maxDepth,
		StorageMaxDepth:     storageMaxDepth,
		MaxNodes:            maxNodes,
		MaxEdges:            maxEdges,
		TerminalNodeKinds:   terminalNodeKinds,
		ExpandTerminalNodes: expandTerminalNodes,
	})
	if err != nil {
		writeDiagnosticError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, query.NewDiagnosticSubgraphResponse(result, h.runtime.RuntimeStatus()))
}

func writeDiagnosticError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	case errors.Is(err, context.Canceled):
		writeError(w, http.StatusRequestTimeout, err)
	case errors.Is(err, query.ErrInvalidDiagnosticQuery):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, query.ErrDiagnosticNotReady):
		writeError(w, http.StatusServiceUnavailable, err)
	case errors.Is(err, query.ErrDiagnosticEntryNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeExpandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, query.ErrInvalidDiagnosticQuery):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, query.ErrDiagnosticEntryNotFound):
		writeError(w, http.StatusNotFound, errors.New("entity not found"))
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func parseOptionalDiagnosticDepth(raw, name string) (int, error) {
	depth, err := parseOptionalInt(raw)
	if err != nil {
		return 0, err
	}
	if depth < 0 {
		return 0, errors.New(name + " must be >= 0")
	}
	if depth > query.MaxDiagnosticDepth {
		return 0, errors.New(name + " must be <= " + strconv.Itoa(query.MaxDiagnosticDepth))
	}
	return depth, nil
}

func parseOptionalDiagnosticLimit(raw, name string, max int) (int, error) {
	limit, err := parseOptionalInt(raw)
	if err != nil {
		return 0, err
	}
	if limit < 0 {
		return 0, errors.New(name + " must be >= 0")
	}
	if limit > max {
		return 0, errors.New(name + " must be <= " + strconv.Itoa(max))
	}
	return limit, nil
}

func parseOptionalExpandDepth(raw, name string) (int, error) {
	depth, err := parseOptionalInt(raw)
	if err != nil {
		return 0, err
	}
	if depth < 0 {
		return 0, errors.New(name + " must be >= 0")
	}
	if depth > query.MaxExpandDepth {
		return 0, errors.New(name + " must be <= " + strconv.Itoa(query.MaxExpandDepth))
	}
	return depth, nil
}

func parseExpandLimit(raw string) (int, error) {
	limit, err := parseLimit(raw)
	if err != nil {
		return 0, err
	}
	if limit > query.MaxExpandLimit {
		return 0, errors.New("limit must be <= " + strconv.Itoa(query.MaxExpandLimit))
	}
	return limit, nil
}

func parseOptionalBool(raw, name string) (bool, error) {
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errors.New(name + " must be true or false")
	}
	return value, nil
}

func (h *handler) backend(w http.ResponseWriter) (ontology.Backend, bool) {
	backend := h.runtime.Ontology()
	if backend == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("ontology backend is not ready"))
		return nil, false
	}
	return backend, true
}

func relationQuery(r *http.Request) (ontology.RelationQuery, error) {
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		return ontology.RelationQuery{}, err
	}
	return ontology.RelationQuery{
		From:      model.CanonicalID(r.URL.Query().Get("from")),
		To:        model.CanonicalID(r.URL.Query().Get("to")),
		Kind:      r.URL.Query().Get("kind"),
		Direction: ontology.Direction(r.URL.Query().Get("direction")),
		Limit:     limit,
	}, nil
}

func parseLimit(raw string) (int, error) {
	limit, err := parseOptionalInt(raw)
	if err != nil {
		return 0, err
	}
	if limit < 0 {
		return 0, errors.New("limit must be >= 0")
	}
	return limit, nil
}

func parseOptionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	response := query.NewErrorResponse(errorCode(status, err), status, err, retryableStatus(status))
	response.Source = "server"
	writeJSON(w, status, response)
}

func errorCode(status int, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	case errors.Is(err, query.ErrInvalidDiagnosticQuery):
		return "invalid_query"
	case errors.Is(err, query.ErrDiagnosticNotReady):
		return "diagnostic_not_ready"
	case errors.Is(err, query.ErrDiagnosticEntryNotFound):
		return "entry_not_found"
	case status == http.StatusBadRequest:
		return "invalid_request"
	case status == http.StatusNotFound:
		return "not_found"
	case status == http.StatusServiceUnavailable:
		return "not_ready"
	case status == http.StatusGatewayTimeout:
		return "timeout"
	case status == http.StatusRequestTimeout:
		return "request_canceled"
	default:
		return "internal_error"
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
