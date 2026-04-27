package reconcile

import (
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
)

func TestFullReconcilerBuild(t *testing.T) {
	reconciler := NewFullReconciler("cluster-a")
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{{Metadata: resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment"}},
		Pods: []resources.Pod{{
			Metadata:        resources.Metadata{UID: "p1", Name: "frontend-abc123", Namespace: "default", Labels: map[string]string{"app": "frontend"}},
			NodeName:        "node-a",
			OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "w1"}},
			ContainerImages: []string{"registry.example.com/frontend:v1@sha256:deadbeef"},
		}},
		Nodes: []resources.Node{{Metadata: resources.Metadata{UID: "n1", Name: "node-a"}}},
	}

	nodes, edges, evidence := reconciler.Build(snapshot)
	if len(nodes) == 0 {
		t.Fatal("expected nodes to be built")
	}
	if len(edges) == 0 {
		t.Fatal("expected edges to be built")
	}
	if evidence == nil {
		t.Fatal("expected evidence slice")
	}
	if reconciler.Builder() == nil {
		t.Fatal("expected builder to be retained")
	}
}

func TestFullReconcilerRebuild(t *testing.T) {
	reconciler := NewFullReconciler("cluster-a")
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{{Metadata: resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment"}},
		Pods: []resources.Pod{{
			Metadata:        resources.Metadata{UID: "p1", Name: "frontend-abc123", Namespace: "default"},
			OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "w1"}},
		}},
	}
	result := reconciler.Rebuild(snapshot)
	if len(result.Nodes) == 0 {
		t.Fatal("expected rebuild nodes")
	}
	if len(result.Edges) == 0 {
		t.Fatal("expected rebuild edges")
	}
	if result.Evidence == nil {
		t.Fatal("expected rebuild evidence slice")
	}
}
