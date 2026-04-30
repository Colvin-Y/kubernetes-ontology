package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
	"github.com/Colvin-Y/kubernetes-ontology/internal/runtime"
)

func TestHandlerServesStatusEntitiesAndDiagnostics(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "w1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123", Namespace: "default", UID: "p1", Labels: map[string]string{"app": "frontend"}}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "s1"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "frontend"}},
		},
	)
	manager := runtime.NewManager("cluster-a", collectk8s.NewReadOnlyCollector(client, "cluster-a", "default"))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(manager))
	defer server.Close()

	var status struct {
		Ready bool `json:"Ready"`
	}
	getJSON(t, server.URL+"/status", &status)
	if !status.Ready {
		t.Fatal("expected ready runtime")
	}

	var entities struct {
		Items []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"items"`
		Count int `json:"count"`
	}
	getJSON(t, server.URL+"/entities?kind=Pod&namespace=default", &entities)
	if entities.Count != 1 || entities.Items[0].Name != "frontend-abc123" {
		t.Fatalf("expected one pod entity, got %+v", entities)
	}

	var diagnostic struct {
		Entry struct {
			CanonicalID string `json:"canonicalId"`
		} `json:"entry"`
		Nodes []any `json:"nodes"`
	}
	getJSON(t, server.URL+"/diagnostic/pod?namespace=default&name=frontend-abc123", &diagnostic)
	if diagnostic.Entry.CanonicalID == "" || len(diagnostic.Nodes) == 0 {
		t.Fatalf("expected diagnostic subgraph, got %+v", diagnostic)
	}
}

func TestDiagnosticResponseIncludesIncidentMetadata(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{diagnosticResult: api.DiagnosticSubgraph{
		Recipe: query.DiagnosticRecipeHelmUpgradeRuntimeFailure,
		Lanes: []api.DiagnosticLane{{
			ID:    "observed-runtime",
			Title: "Observed Runtime",
		}},
	}}))
	defer server.Close()

	var diagnostic struct {
		SchemaVersion string               `json:"schemaVersion"`
		Recipe        string               `json:"recipe"`
		Lanes         []api.DiagnosticLane `json:"lanes"`
	}
	getJSON(t, server.URL+"/diagnostic/pod?namespace=default&name=frontend", &diagnostic)
	if diagnostic.SchemaVersion != api.DiagnosticSchemaVersionV1Alpha1 {
		t.Fatalf("expected schema version metadata, got %q", diagnostic.SchemaVersion)
	}
	if diagnostic.Recipe != query.DiagnosticRecipeHelmUpgradeRuntimeFailure {
		t.Fatalf("expected recipe metadata, got %q", diagnostic.Recipe)
	}
	if len(diagnostic.Lanes) != 1 || diagnostic.Lanes[0].ID != "observed-runtime" {
		t.Fatalf("expected lane metadata, got %#v", diagnostic.Lanes)
	}
}

func TestDiagnosticValidatesRequiredParameters(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}

	var payload query.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "name is required" || payload.Code != "invalid_request" || payload.Status != http.StatusBadRequest {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestDiagnosticMapsTypedErrorsToHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "not ready", err: query.ErrDiagnosticNotReady, want: http.StatusServiceUnavailable},
		{name: "entry missing", err: query.ErrDiagnosticEntryNotFound, want: http.StatusNotFound},
		{name: "invalid request", err: query.ErrInvalidDiagnosticQuery, want: http.StatusBadRequest},
		{name: "internal", err: errors.New("boom"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(NewHandler(stubRuntime{diagnosticErr: tt.err}))
			defer server.Close()

			resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("expected status %d, got %d", tt.want, resp.StatusCode)
			}
			var payload query.ErrorResponse
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error == "" || payload.Code == "" || payload.Status != tt.want {
				t.Fatalf("expected machine-readable error payload, got %+v", payload)
			}
		})
	}
}

func TestDiagnosticRejectsInvalidDepth(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&maxDepth=-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestDiagnosticGenericEndpointPassesKind(t *testing.T) {
	var entryNodeKind string
	server := httptest.NewServer(NewHandler(stubRuntime{capturedEntryNodeKind: &entryNodeKind}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic?kind=PVC&namespace=default&name=data")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if entryNodeKind != "PVC" {
		t.Fatalf("expected PVC entry kind, got %q", entryNodeKind)
	}
}

func TestExpandReturnsSubgraph(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "p1"}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "n1"}},
	)
	manager := runtime.NewManager("cluster-a", collectk8s.NewReadOnlyCollector(client, "cluster-a", "default"))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(manager))
	defer server.Close()

	var entity struct {
		Entity struct {
			ID string `json:"entityGlobalId"`
		} `json:"entity"`
	}
	getJSON(t, server.URL+"/entity?kind=Pod&namespace=default&name=frontend", &entity)
	var expanded struct {
		Nodes     []any           `json:"nodes"`
		Edges     []any           `json:"edges"`
		NodeCount int             `json:"nodeCount"`
		EdgeCount int             `json:"edgeCount"`
		Freshness query.Freshness `json:"freshness"`
	}
	getJSON(t, server.URL+"/expand?entityGlobalId="+url.QueryEscape(entity.Entity.ID)+"&depth=1", &expanded)
	if len(expanded.Nodes) != 2 || len(expanded.Edges) != 1 {
		t.Fatalf("expected one-hop expand result, got nodes=%d edges=%d", len(expanded.Nodes), len(expanded.Edges))
	}
	if expanded.NodeCount != 2 || expanded.EdgeCount != 1 || !expanded.Freshness.Ready {
		t.Fatalf("expected additive graph metadata, got %+v", expanded)
	}
}

func TestExpandRejectsInvalidDepth(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/expand?entityGlobalId=pod&depth=99")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestDiagnosticParsesTerminalOptions(t *testing.T) {
	var got query.DiagnosticOptions
	server := httptest.NewServer(NewHandler(stubRuntime{capturedOptions: &got}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&terminalKinds=ServiceAccount,Secret&expandTerminalNodes=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if !got.ExpandTerminalNodes {
		t.Fatal("expected expandTerminalNodes to be parsed")
	}
	if len(got.TerminalNodeKinds) != 2 || got.TerminalNodeKinds[0] != api.NodeKindServiceAccount || got.TerminalNodeKinds[1] != api.NodeKindSecret {
		t.Fatalf("unexpected terminal kinds: %#v", got.TerminalNodeKinds)
	}
}

func TestDiagnosticParsesBudgetOptions(t *testing.T) {
	var got query.DiagnosticOptions
	server := httptest.NewServer(NewHandler(stubRuntime{capturedOptions: &got}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&maxNodes=25&maxEdges=50")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got.MaxNodes != 25 || got.MaxEdges != 50 {
		t.Fatalf("expected diagnostic budgets to be parsed, got %+v", got)
	}
}

func TestDiagnosticParsesRecipeOption(t *testing.T) {
	var got query.DiagnosticOptions
	server := httptest.NewServer(NewHandler(stubRuntime{capturedOptions: &got}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&recipe=helm-upgrade-runtime-failure")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if got.Recipe != "helm-upgrade-runtime-failure" {
		t.Fatalf("expected recipe to be parsed, got %+v", got)
	}
}

func TestDiagnosticRejectsInvalidRecipe(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&recipe=nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestDiagnosticRejectsInvalidBudgetLimit(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&maxNodes=-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestDiagnosticRejectsInvalidTerminalKind(t *testing.T) {
	server := httptest.NewServer(NewHandler(stubRuntime{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend&terminalKinds=NoSuchKind")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestDiagnosticDeadlineCancelsRuntimeQuery(t *testing.T) {
	previous := diagnosticRequestTimeout
	diagnosticRequestTimeout = 10 * time.Millisecond
	defer func() { diagnosticRequestTimeout = previous }()

	server := httptest.NewServer(NewHandler(stubRuntime{waitForContext: true}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/diagnostic/pod?namespace=default&name=frontend")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected status 504, got %d", resp.StatusCode)
	}
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

type stubRuntime struct {
	diagnosticErr         error
	diagnosticResult      api.DiagnosticSubgraph
	waitForContext        bool
	capturedOptions       *query.DiagnosticOptions
	capturedEntryNodeKind *string
}

func (s stubRuntime) RuntimeStatus() query.RuntimeStatus {
	return query.RuntimeStatus{}
}

func (s stubRuntime) Ontology() ontology.Backend {
	return nil
}

func (s stubRuntime) QueryDiagnosticSubgraph(ctx context.Context, entryKind, namespace, name string, options query.DiagnosticOptions) (api.DiagnosticSubgraph, error) {
	if s.capturedEntryNodeKind != nil {
		*s.capturedEntryNodeKind = entryKind
	}
	if s.capturedOptions != nil {
		*s.capturedOptions = options
	}
	if s.waitForContext {
		<-ctx.Done()
		return api.DiagnosticSubgraph{}, ctx.Err()
	}
	if s.diagnosticErr != nil {
		return api.DiagnosticSubgraph{}, s.diagnosticErr
	}
	return s.diagnosticResult, nil
}
