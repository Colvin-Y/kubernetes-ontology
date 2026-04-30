package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestServeIndexIncludesProfessionalRenderer(t *testing.T) {
	viewer := httptest.NewServer(newHandler("http://kubernetes-ontology:18080", time.Second))
	defer viewer.Close()

	resp, err := http.Get(viewer.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "/vendor/cytoscape.min.js") || !strings.Contains(text, "Professional renderer") {
		t.Fatalf("expected professional renderer assets in index")
	}
	if !strings.Contains(text, `value="http://kubernetes-ontology:18080"`) {
		t.Fatalf("expected default ontology server to be injected")
	}
}

func TestServeCytoscapeAsset(t *testing.T) {
	viewer := httptest.NewServer(newHandler("http://kubernetes-ontology:18080", time.Second))
	defer viewer.Close()

	resp, err := http.Get(viewer.URL + "/vendor/cytoscape.min.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/javascript") {
		t.Fatalf("expected javascript content type, got %q", contentType)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body[:min(len(body), 300)]), "Cytoscape") {
		t.Fatalf("expected cytoscape asset body")
	}
}

func TestServeDiagnosticForwardsRecipeAndBudgets(t *testing.T) {
	var got url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer upstream.Close()

	viewer := httptest.NewServer(newHandler(upstream.URL, time.Second))
	defer viewer.Close()

	target := viewer.URL + "/diagnostic?" + url.Values{
		"kind":      []string{"Pod"},
		"namespace": []string{"default"},
		"name":      []string{"frontend"},
		"maxNodes":  []string{"25"},
		"maxEdges":  []string{"50"},
		"recipe":    []string{"helm-upgrade-runtime-failure"},
	}.Encode()
	resp, err := http.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got.Get("recipe") != "helm-upgrade-runtime-failure" || got.Get("maxNodes") != "25" || got.Get("maxEdges") != "50" {
		t.Fatalf("expected diagnostic options to be proxied, got %v", got)
	}
}

func TestServeDiagnosticPreservesStructuredUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "unsupported diagnostic recipe",
			"code":      "invalid_query",
			"status":    http.StatusBadRequest,
			"retryable": false,
			"source":    "server",
		})
	}))
	defer upstream.Close()

	viewer := httptest.NewServer(newHandler(upstream.URL, time.Second))
	defer viewer.Close()

	resp, err := http.Get(viewer.URL + "/diagnostic?kind=Pod&namespace=default&name=frontend")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "invalid_query" || payload["source"] != "server" {
		t.Fatalf("expected structured error to be preserved, got %+v", payload)
	}
}
