package query

import (
	"testing"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/service/diagnostic"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestFacadeFindEntryID(t *testing.T) {
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{{Metadata: resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment"}},
		Pods:      []resources.Pod{{Metadata: resources.Metadata{UID: "p1", Name: "frontend-abc123", Namespace: "default"}}},
	}
	facade := NewFacade("cluster-a", snapshot, graph.NewBuilder("cluster-a"), diagnostic.NewService(graph.NewKernel(memorystore.NewStore(), memorystore.NewStore())))

	podID, err := facade.FindEntryID("Pod", "default", "frontend-abc123")
	if err != nil {
		t.Fatal(err)
	}
	if podID == "" {
		t.Fatal("expected pod canonical id")
	}

	workloadID, err := facade.FindEntryID("Workload", "default", "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if workloadID == "" {
		t.Fatal("expected workload canonical id")
	}
}

func TestFacadeDiagnosticPolicy(t *testing.T) {
	facade := NewFacade("cluster-a", collectk8s.Snapshot{}, graph.NewBuilder("cluster-a"), diagnostic.NewService(graph.NewKernel(memorystore.NewStore(), memorystore.NewStore())))
	policy := facade.DiagnosticPolicy(DiagnosticOptions{MaxDepth: 3, StorageMaxDepth: 7})
	if policy.MaxDepth != 3 {
		t.Fatalf("expected max depth 3, got %d", policy.MaxDepth)
	}
	if policy.StorageMaxDepth != 7 {
		t.Fatalf("expected storage max depth 7, got %d", policy.StorageMaxDepth)
	}
	if !policy.IncludeEvents || !policy.IncludeStorage || !policy.IncludeOCI {
		t.Fatal("expected default diagnostic includes to remain enabled")
	}
	if len(policy.TerminalNodeKinds) == 0 {
		t.Fatal("expected default terminal node kinds")
	}
	custom := facade.DiagnosticPolicy(DiagnosticOptions{TerminalNodeKinds: []api.NodeKind{api.NodeKindSecret}})
	if len(custom.TerminalNodeKinds) != 1 || custom.TerminalNodeKinds[0] != api.NodeKindSecret {
		t.Fatalf("expected custom terminal kinds, got %#v", custom.TerminalNodeKinds)
	}
	expanded := facade.DiagnosticPolicy(DiagnosticOptions{ExpandTerminalNodes: true})
	if !expanded.ExpandTerminalNodes || len(expanded.TerminalNodeKinds) != 0 {
		t.Fatalf("expected terminal expansion, got %#v", expanded)
	}
}

func TestParseTerminalNodeKinds(t *testing.T) {
	kinds, disable, err := ParseTerminalNodeKinds("serviceaccount, Secret,ServiceAccount")
	if err != nil {
		t.Fatal(err)
	}
	if disable {
		t.Fatal("did not expect terminal kinds to disable boundaries")
	}
	if len(kinds) != 2 || kinds[0] != api.NodeKindServiceAccount || kinds[1] != api.NodeKindSecret {
		t.Fatalf("unexpected terminal kinds: %#v", kinds)
	}
	none, disable, err := ParseTerminalNodeKinds("none")
	if err != nil {
		t.Fatal(err)
	}
	if !disable || len(none) != 0 {
		t.Fatalf("expected none to disable terminal boundaries, got disable=%t kinds=%#v", disable, none)
	}
	if _, _, err := ParseTerminalNodeKinds("DefinitelyNotAKind"); err == nil {
		t.Fatal("expected invalid terminal kind to fail")
	}
}

func TestFacadeRuntimeStatus(t *testing.T) {
	facade := NewFacade("cluster-a", collectk8s.Snapshot{}, graph.NewBuilder("cluster-a"), diagnostic.NewService(graph.NewKernel(memorystore.NewStore(), memorystore.NewStore())))
	bootstrapAt := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	facade.SetRuntimeStatus(RuntimeStatus{
		Phase:              "ready",
		Cluster:            "cluster-a",
		Ready:              true,
		NodeCount:          12,
		EdgeCount:          21,
		LastBootstrapAt:    bootstrapAt.Format(time.RFC3339),
		LastStrategy:       "service-narrow",
		FullRebuildCount:   2,
		ServiceNarrowCount: 1,
	})
	status := facade.RuntimeStatus()
	if status.Phase != "ready" || !status.Ready {
		t.Fatal("expected ready runtime status")
	}
	if status.NodeCount != 12 || status.EdgeCount != 21 {
		t.Fatal("expected runtime graph counts to be preserved")
	}
	if status.LastStrategy != "service-narrow" {
		t.Fatalf("expected strategy to be preserved, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 2 || status.ServiceNarrowCount != 1 {
		t.Fatalf("expected counters to be preserved, got full=%d service=%d", status.FullRebuildCount, status.ServiceNarrowCount)
	}
}

func TestFreshnessFromRuntimeStatus(t *testing.T) {
	freshness := FreshnessFromRuntimeStatus(RuntimeStatus{
		Phase:                 "ready",
		Cluster:               "cluster-a",
		Ready:                 true,
		NodeCount:             3,
		EdgeCount:             2,
		LastBootstrapAt:       "2026-04-23T10:00:00Z",
		LastAppliedChangeKind: "pod",
		LastAppliedChangeNS:   "default",
		LastAppliedChangeName: "frontend",
		LastAppliedChangeType: "upsert",
		LastAppliedChangeAt:   "2026-04-23T10:01:00Z",
		LastStrategy:          "pod-narrow",
	})
	if !freshness.Ready || freshness.LastRefreshAt != "2026-04-23T10:01:00Z" {
		t.Fatalf("unexpected freshness: %+v", freshness)
	}
	if freshness.LastAppliedChangeNamespace != "default" || freshness.LastStrategy != "pod-narrow" {
		t.Fatalf("expected change metadata to be preserved, got %+v", freshness)
	}
}

func TestFacadeQueryDiagnosticSubgraph(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := diagnostic.NewService(kernel)
	builder := graph.NewBuilder("cluster-a")
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{{Metadata: resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment"}},
		Pods:      []resources.Pod{{Metadata: resources.Metadata{UID: "p1", Name: "frontend-abc123", Namespace: "default"}}},
	}

	nodes, edges := builder.Build(snapshot)
	for _, node := range nodes {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range edges {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
		store.AddNeighbor(edge.From, edge.Key())
		store.AddNeighbor(edge.To, edge.Key())
	}

	facade := NewFacade("cluster-a", snapshot, builder, service)
	result, err := facade.QueryDiagnosticSubgraph("Pod", "default", "frontend-abc123", DiagnosticOptions{MaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Entry.CanonicalID == "" {
		t.Fatal("expected canonical entry id")
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected diagnostic nodes")
	}
}
