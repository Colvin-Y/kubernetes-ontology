package reconcile

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestPodReconcilerMatchesFullRebuildPodEdges(t *testing.T) {
	initial := podSnapshot("frontend", "node-a", "registry.example.com/app:v1", "Running")
	next := podSnapshot("backend", "node-b", "registry.example.com/app:v2", "Pending")
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewPodReconciler("cluster-a", kernel).Apply(next, "default", "app-0", k8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected pod update to be applied")
	}
	if result.DeletedImages != 1 {
		t.Fatalf("expected stale image to be pruned, got %d", result.DeletedImages)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	got := podScopedEdgeKeys(kernel.ListEdges(), "cluster-a/core/Pod/default/app-0/pod-uid/_")
	want := podScopedEdgeKeys(full.ListEdges(), "cluster-a/core/Pod/default/app-0/pod-uid/_")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pod edges mismatch\ngot  %#v\nwant %#v", got, want)
	}
	gotImages := imageNodeFingerprints(kernel.ListNodes())
	wantImages := imageNodeFingerprints(full.ListNodes())
	if !reflect.DeepEqual(gotImages, wantImages) {
		t.Fatalf("image nodes mismatch\ngot  %#v\nwant %#v", gotImages, wantImages)
	}
}

func TestPodReconcilerDeletesMissingPodAndPrunesImage(t *testing.T) {
	initial := podSnapshot("frontend", "node-a", "registry.example.com/app:v1", "Running")
	next := initial
	next.Pods = nil
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewPodReconciler("cluster-a", kernel).Apply(next, "default", "app-0", k8s.ChangeTypeDelete)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted {
		t.Fatal("expected pod delete result")
	}
	if result.DeletedImages != 1 {
		t.Fatalf("expected image to be pruned, got %d", result.DeletedImages)
	}
	for _, node := range kernel.ListNodes() {
		if node.Kind == model.NodeKindPod && node.Namespace == "default" && node.Name == "app-0" {
			t.Fatal("expected pod node to be removed")
		}
		if node.Kind == model.NodeKindImage {
			t.Fatal("expected orphan image node to be removed")
		}
	}
	for _, edge := range kernel.ListEdges() {
		if edge.From.String() == "cluster-a/core/Pod/default/app-0/pod-uid/_" || edge.To.String() == "cluster-a/core/Pod/default/app-0/pod-uid/_" {
			t.Fatalf("expected pod edges to be removed, found %s", edge.Key())
		}
	}
}

func podSnapshot(label, nodeName, image, phase string) k8s.Snapshot {
	return k8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: "w1", Name: "app", Namespace: "default"},
			ControllerKind: "Deployment",
			Replicas:       1,
		}},
		Pods: []resources.Pod{{
			Metadata:        resources.Metadata{UID: "pod-uid", Name: "app-0", Namespace: "default", Labels: map[string]string{"app": label}},
			NodeName:        nodeName,
			ServiceAccount:  "default",
			OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "app", UID: "w1"}},
			ContainerImages: []string{image},
			ConfigMapRefs:   []string{"app-config"},
			SecretRefs:      []string{"app-secret"},
			PVCRefs:         []string{"data"},
			Phase:           phase,
		}},
		Nodes:           []resources.Node{{Metadata: resources.Metadata{UID: "node-a-uid", Name: "node-a"}}, {Metadata: resources.Metadata{UID: "node-b-uid", Name: "node-b"}}},
		Services:        []resources.Service{{Metadata: resources.Metadata{UID: "svc-uid", Name: "app", Namespace: "default"}, Selector: map[string]string{"app": "frontend"}}},
		ConfigMaps:      []resources.ConfigMap{{Metadata: resources.Metadata{UID: "cm-uid", Name: "app-config", Namespace: "default"}}},
		Secrets:         []resources.Secret{{Metadata: resources.Metadata{UID: "secret-uid", Name: "app-secret", Namespace: "default"}}},
		ServiceAccounts: []resources.ServiceAccount{{Metadata: resources.Metadata{UID: "sa-uid", Name: "default", Namespace: "default"}}},
		PVCs:            []resources.PVC{{Metadata: resources.Metadata{UID: "pvc-uid", Name: "data", Namespace: "default"}, Status: "Bound"}},
	}
}

func podScopedEdgeKeys(edges []model.Edge, podID string) []string {
	id := model.CanonicalID(podID)
	out := make([]string, 0)
	for _, edge := range edges {
		if isPodScopedEdge(edge, id) {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}

func imageNodeFingerprints(nodes []model.Node) []string {
	out := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != model.NodeKindImage {
			continue
		}
		out = append(out, node.ID.String()+"|"+node.Name)
	}
	sort.Strings(out)
	return out
}
