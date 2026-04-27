package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
)

func TestCollectionNamespacesMergesEntryNamespace(t *testing.T) {
	got := collectionNamespaces("default, kube-system,default", "payments")
	want := []string{"default", "kube-system", "payments"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected namespaces: got %#v want %#v", got, want)
	}
}

func TestCollectionNamespacesCanCollectAll(t *testing.T) {
	got := collectionNamespaces("", "")
	if len(got) != 0 {
		t.Fatalf("expected empty namespace list for collect-all, got %#v", got)
	}
}

func TestQueryServerReturnsStructuredServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(query.ErrorResponse{
			Error:     "ontology backend is not ready",
			Message:   "ontology backend is not ready",
			Code:      "not_ready",
			Status:    http.StatusServiceUnavailable,
			Retryable: true,
			Source:    "server",
		})
	}))
	defer server.Close()

	err := queryServer(server.URL, serverQueryOptions{statusOnly: true})
	var serverErr *serverError
	if !errors.As(err, &serverErr) {
		t.Fatalf("expected serverError, got %T %[1]v", err)
	}
	if serverErr.Payload.Code != "not_ready" || !serverErr.Payload.Retryable || serverErr.Payload.Status != http.StatusServiceUnavailable {
		t.Fatalf("unexpected error payload: %+v", serverErr.Payload)
	}
}

func TestWriteMachineErrorPrintsJSONPayload(t *testing.T) {
	err := &serverError{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Payload: query.ErrorResponse{
			Error:  "entity not found",
			Code:   "not_found",
			Status: http.StatusNotFound,
			Source: "server",
		},
	}

	var buf bytes.Buffer
	writeMachineError(&buf, err)
	var payload query.ErrorResponse
	if decodeErr := json.Unmarshal(buf.Bytes(), &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if payload.Error != "entity not found" || payload.Code != "not_found" || payload.Source != "server" {
		t.Fatalf("unexpected machine error payload: %+v", payload)
	}
}
