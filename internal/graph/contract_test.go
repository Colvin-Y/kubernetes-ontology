package graph_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/service/diagnostic"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestPodFixtureContract(t *testing.T) {
	builder := graph.NewBuilder("cluster-a")
	snapshot := collectk8s.Snapshot{
		Workloads:  []resources.Workload{{Metadata: resources.Metadata{UID: "workload-uid", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment", Replicas: 1, Conditions: map[string]string{"Progressing": "True"}}},
		Pods:       []resources.Pod{{Metadata: resources.Metadata{UID: "pod-uid", Name: "frontend-abc123", Namespace: "default", Labels: map[string]string{"app": "frontend"}}, NodeName: "node-a", OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "workload-uid"}}, ContainerImages: []string{"registry.example.com/frontend:v2@sha256:deadbeef"}, ConfigMapRefs: []string{"frontend-config"}, Phase: "Pending", Reason: "ImagePullBackOff"}},
		Nodes:      []resources.Node{{Metadata: resources.Metadata{UID: "node-uid", Name: "node-a"}, Conditions: map[string]string{"Ready": "True"}}},
		Services:   []resources.Service{{Metadata: resources.Metadata{UID: "service-uid", Name: "frontend-svc", Namespace: "default"}, Selector: map[string]string{"app": "frontend"}}},
		ConfigMaps: []resources.ConfigMap{{Metadata: resources.Metadata{UID: "configmap-uid", Name: "frontend-config", Namespace: "default"}}},
		Events:     []resources.Event{{Metadata: resources.Metadata{UID: "event-uid", Name: "frontend-abc123.12345", Namespace: "default"}, InvolvedKind: "Pod", InvolvedName: "frontend-abc123", InvolvedUID: "pod-uid", Reason: "Failed", Message: "Failed to pull image"}},
	}

	nodes, edges := builder.Build(snapshot)
	store := memorystore.NewStore()
	index := graph.NewReverseIndex()
	kernel := graph.NewKernel(store, index)
	for _, node := range nodes {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range edges {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
		index.AddNeighbor(edge.From, edge.Key())
		index.AddNeighbor(edge.To, edge.Key())
	}

	service := diagnostic.NewService(kernel)
	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: "cluster-a/core/Pod/default/frontend-abc123/pod-uid/_", Namespace: "default", Name: "frontend-abc123"}, diagnostic.DefaultExpansionPolicy())
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join("..", "fixtures", "pod_image_pull_failure.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden api.DiagnosticSubgraph
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	if golden.Entry.CanonicalID != result.Entry.CanonicalID {
		t.Fatalf("entry canonical id mismatch: got %s want %s", result.Entry.CanonicalID, golden.Entry.CanonicalID)
	}
	if len(result.Nodes) < 2 || len(result.Edges) < 2 {
		t.Fatalf("expected non-trivial subgraph, got nodes=%d edges=%d", len(result.Nodes), len(result.Edges))
	}
}
