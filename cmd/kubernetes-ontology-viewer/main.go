package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/tools/visualize"
)

const defaultOntologyServer = "http://127.0.0.1:18080"

type upstreamError struct {
	status  int
	message string
}

func (e upstreamError) Error() string {
	return e.message
}

func main() {
	var host string
	var port int
	var addr string
	var server string
	var timeout time.Duration

	flag.StringVar(&host, "host", "127.0.0.1", "HTTP host for the viewer")
	flag.IntVar(&port, "port", 8765, "HTTP port for the viewer")
	flag.StringVar(&addr, "addr", "", "HTTP listen address. Overrides --host and --port when set.")
	flag.StringVar(&server, "server", envDefault("ONTOLOGY_SERVER", defaultOntologyServer), "Default kubernetes-ontologyd server URL")
	flag.DurationVar(&timeout, "upstream-timeout", envDuration("VIEWER_UPSTREAM_TIMEOUT_SECONDS", 30*time.Second), "Timeout for kubernetes-ontologyd requests")
	flag.Parse()

	if addr == "" {
		addr = net.JoinHostPort(host, strconv.Itoa(port))
	}

	handler := newHandler(server, timeout)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("kubernetes-ontology viewer listening on http://%s", addr)
	log.Printf("default ontology server: %s", server)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve viewer: %v", err)
	}
}

type handler struct {
	defaultServer string
	timeout       time.Duration
	client        *http.Client
	version       string
}

func newHandler(defaultServer string, timeout time.Duration) http.Handler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &handler{
		defaultServer: defaultServer,
		timeout:       timeout,
		client:        &http.Client{Timeout: timeout},
		version:       strconv.FormatInt(time.Now().Unix(), 10),
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	switch r.URL.Path {
	case "/":
		h.serveIndex(w)
	case "/topology":
		h.serveTopology(w, r)
	case "/diagnostic":
		h.serveDiagnostic(w, r)
	case "/expand":
		h.serveExpand(w, r)
	case "/proxy":
		h.serveProxy(w, r)
	case "/load":
		h.serveLoad(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) serveIndex(w http.ResponseWriter) {
	body := strings.ReplaceAll(visualize.IndexHTML, "__VIEWER_VERSION__", h.version)
	body = strings.ReplaceAll(body, "__ONTOLOGY_SERVER__", html.EscapeString(h.defaultServer))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

func (h *handler) serveTopology(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	server := first(q, "server", h.defaultServer)
	entityParams := url.Values{}
	entityParams.Set("limit", first(q, "entityLimit", "1000"))
	if namespace := first(q, "namespace", ""); namespace != "" {
		entityParams.Set("namespace", namespace)
	}
	if kind := first(q, "kind", ""); kind != "" {
		entityParams.Set("kind", kind)
	}

	status, err := h.fetchJSON(r.Context(), server, "/status")
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	entities, err := h.fetchJSON(r.Context(), server, "/entities?"+entityParams.Encode())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	relationParams := url.Values{"limit": []string{first(q, "relationLimit", "5000")}}
	relations, err := h.fetchJSON(r.Context(), server, "/relations?"+relationParams.Encode())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}

	writeJSON(w, map[string]any{
		"source":    "server",
		"server":    server,
		"status":    status,
		"entities":  entities,
		"relations": relations,
	}, http.StatusOK)
}

func (h *handler) serveDiagnostic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	server := first(q, "server", h.defaultServer)
	kind := first(q, "kind", "Pod")
	namespace := first(q, "namespace", "")
	name := first(q, "name", "")
	if namespace == "" && (strings.EqualFold(kind, "Pod") || strings.EqualFold(kind, "Workload")) {
		writeJSON(w, map[string]string{"error": "namespace is required"}, http.StatusBadRequest)
		return
	}
	if name == "" {
		writeJSON(w, map[string]string{"error": "name is required"}, http.StatusBadRequest)
		return
	}

	params := url.Values{}
	params.Set("kind", kind)
	if namespace != "" {
		params.Set("namespace", namespace)
	}
	params.Set("name", name)
	params.Set("maxDepth", first(q, "maxDepth", "2"))
	params.Set("storageMaxDepth", first(q, "storageMaxDepth", "5"))
	if terminalKinds := first(q, "terminalKinds", ""); terminalKinds != "" {
		params.Set("terminalKinds", terminalKinds)
	}
	if expandTerminalNodes := first(q, "expandTerminalNodes", ""); expandTerminalNodes != "" {
		params.Set("expandTerminalNodes", expandTerminalNodes)
	}

	data, err := h.fetchJSON(r.Context(), server, "/diagnostic?"+params.Encode())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, data, http.StatusOK)
}

func (h *handler) serveExpand(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	server := first(q, "server", h.defaultServer)
	entityID := first(q, "entityGlobalId", first(q, "id", ""))
	if entityID == "" {
		writeJSON(w, map[string]string{"error": "entityGlobalId or id is required"}, http.StatusBadRequest)
		return
	}
	params := url.Values{}
	params.Set("entityGlobalId", entityID)
	params.Set("depth", first(q, "depth", "1"))
	copyOptional(params, q, "direction")
	copyOptional(params, q, "kind")
	copyOptional(params, q, "limit")

	data, err := h.fetchJSON(r.Context(), server, "/expand?"+params.Encode())
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, data, http.StatusOK)
}

func (h *handler) serveProxy(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	server := first(q, "server", h.defaultServer)
	path := first(q, "path", "")
	if !strings.HasPrefix(path, "/") {
		writeJSON(w, map[string]string{"error": "path must start with /"}, http.StatusBadRequest)
		return
	}
	data, err := h.fetchBytes(r.Context(), server, path)
	if err != nil {
		h.writeUpstreamError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handler) serveLoad(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		writeJSON(w, map[string]string{"error": "path is required"}, http.StatusBadRequest)
		return
	}
	path := filepath.Clean(rawPath)
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to read file: %v", err)}, http.StatusNotFound)
		return
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to read json: %v", err)}, http.StatusBadRequest)
		return
	}
	writeJSON(w, payload, http.StatusOK)
}

func (h *handler) fetchJSON(ctx context.Context, server, path string) (any, error) {
	data, err := h.fetchBytes(ctx, server, path)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (h *handler) fetchBytes(ctx context.Context, server, path string) ([]byte, error) {
	base, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("server must use http or https")
	}
	next, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	target := base.ResolveReference(next)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return nil, upstreamError{status: http.StatusGatewayTimeout, message: fmt.Sprintf("upstream timeout after %s", h.timeout)}
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, upstreamError{status: resp.StatusCode, message: string(data)}
	}
	return data, nil
}

func (h *handler) writeUpstreamError(w http.ResponseWriter, err error) {
	var upstream upstreamError
	if errors.As(err, &upstream) {
		writeJSON(w, map[string]string{"error": upstream.message}, upstream.status)
		return
	}
	writeJSON(w, map[string]string{"error": err.Error()}, http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, payload any, status int) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func first(values url.Values, name, defaultValue string) string {
	items := values[name]
	if len(items) == 0 {
		return defaultValue
	}
	return items[0]
}

func copyOptional(dst, src url.Values, name string) {
	if value := first(src, name, ""); value != "" {
		dst.Set(name, value)
	}
}

func envDefault(name, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}
