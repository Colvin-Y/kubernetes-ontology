package reconcile

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

func TestWorkloadReconcilerMatchesFullRebuildOwnerEdges(t *testing.T) {
	initial := workloadSnapshot("w1", 1, false)
	next := workloadSnapshot("w2", 2, true)
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewWorkloadReconciler("cluster-a", kernel).Apply(next, "default", "frontend", collectk8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected workload update to be applied")
	}
	if result.DeletedNodes != 1 {
		t.Fatalf("expected stale workload node to be deleted, got %d", result.DeletedNodes)
	}
	if result.UpsertedEdges != 2 {
		t.Fatalf("expected owner edges to be rebuilt, got %d", result.UpsertedEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	gotEdges := workloadOwnerEdgeKeys(kernel.ListEdges())
	wantEdges := workloadOwnerEdgeKeys(full.ListEdges())
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Fatalf("workload owner edges mismatch\ngot  %#v\nwant %#v", gotEdges, wantEdges)
	}
	gotNodes := workloadNodeFingerprints(kernel.ListNodes())
	wantNodes := workloadNodeFingerprints(full.ListNodes())
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("workload nodes mismatch\ngot  %#v\nwant %#v", gotNodes, wantNodes)
	}
}

func TestWorkloadReconcilerDeletesMissingWorkload(t *testing.T) {
	initial := workloadSnapshot("w1", 1, false)
	next := initial
	next.Workloads = nil
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewWorkloadReconciler("cluster-a", kernel).Apply(next, "default", "frontend", collectk8s.ChangeTypeDelete)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted {
		t.Fatal("expected workload delete result")
	}
	if len(workloadOwnerEdgeKeys(kernel.ListEdges())) != 0 {
		t.Fatal("expected workload owner edges to be removed")
	}
	for _, node := range kernel.ListNodes() {
		if node.Kind == model.NodeKindWorkload && node.Namespace == "default" && node.Name == "frontend" {
			t.Fatal("expected workload node to be removed")
		}
	}
}

func TestWorkloadReconcilerRebuildsWorkloadOwnerEdges(t *testing.T) {
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{
			{
				Metadata:       resources.Metadata{UID: "redis-uid", Name: "redis-cq01mg647b8wj86qz", Namespace: "redis"},
				APIVersion:     "db.example.com/v1",
				ControllerKind: "RedisCluster",
			},
			{
				Metadata:       resources.Metadata{UID: "asts-uid", Name: "proxy-redis-cq01mg647b8wj86qz", Namespace: "redis"},
				APIVersion:     "apps.kruise.io/v1alpha1",
				ControllerKind: "AdvancedStatefulSet",
				OwnerReferences: []resources.OwnerReference{{
					APIVersion: "db.example.com/v1",
					Kind:       "RedisCluster",
					Name:       "redis-cq01mg647b8wj86qz",
					UID:        "redis-uid",
					Controller: true,
				}},
			},
		},
		Pods: []resources.Pod{{
			Metadata: resources.Metadata{UID: "pod-uid", Name: "proxy-redis-cq01mg647b8wj86qz-0", Namespace: "redis"},
			OwnerReferences: []resources.OwnerReference{{
				APIVersion: "apps.kruise.io/v1alpha1",
				Kind:       "AdvancedStatefulSet",
				Name:       "proxy-redis-cq01mg647b8wj86qz",
				UID:        "asts-uid",
				Controller: true,
			}},
		}},
	}
	kernel := kernelFromSnapshot(t, "cluster-a", collectk8s.Snapshot{})

	result, err := NewWorkloadReconciler("cluster-a", kernel).Apply(snapshot, "redis", "proxy-redis-cq01mg647b8wj86qz", collectk8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpsertedEdges != 5 {
		t.Fatalf("expected pod direct/ancestor owner edges and workload owner edge, got %d", result.UpsertedEdges)
	}
	foundControlledBy := false
	for _, edge := range kernel.ListEdges() {
		if edge.Kind == model.EdgeKindControlledBy {
			foundControlledBy = true
			break
		}
	}
	if !foundControlledBy {
		t.Fatal("expected controlled_by edge between configured workloads")
	}
}

func TestWorkloadControllerRuleReconcilerRebuildsRuleEdges(t *testing.T) {
	snapshot := workloadControllerRuleSnapshot()
	kernel := kernelFromSnapshot(t, "cluster-a", snapshot)
	rules := []infer.WorkloadControllerRule{{
		APIVersion:            "apps.kruise.io/*",
		Kind:                  "*",
		ControllerNamespace:   "kruise-system",
		ControllerPodPrefixes: []string{"kruise-controller-manager"},
		NodeDaemonPodPrefixes: []string{"kruise-daemon"},
	}}

	upserted, deleted, err := NewWorkloadControllerRuleReconciler("cluster-a", kernel, rules).Apply(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 || upserted != 2 {
		t.Fatalf("expected controller rule edges to be rebuilt, got deleted=%d upserted=%d", deleted, upserted)
	}
	kinds := map[model.EdgeKind]int{}
	for _, edge := range kernel.ListEdges() {
		kinds[edge.Kind]++
	}
	if kinds[model.EdgeKindManagedByController] != 1 || kinds[model.EdgeKindServedByNodeDaemon] != 1 {
		t.Fatalf("unexpected controller rule edge counts: %#v", kinds)
	}
}

func workloadControllerRuleSnapshot() collectk8s.Snapshot {
	return collectk8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: "asts-uid", Name: "proxy-redis", Namespace: "redis"},
			APIVersion:     "apps.kruise.io/v1alpha1",
			ControllerKind: "AdvancedStatefulSet",
		}},
		Pods: []resources.Pod{
			{
				Metadata: resources.Metadata{UID: "app-pod", Name: "proxy-redis-0", Namespace: "redis"},
				NodeName: "node-a",
				OwnerReferences: []resources.OwnerReference{{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "AdvancedStatefulSet",
					Name:       "proxy-redis",
					UID:        "asts-uid",
					Controller: true,
				}},
			},
			{Metadata: resources.Metadata{UID: "manager", Name: "kruise-controller-manager-0", Namespace: "kruise-system"}},
			{Metadata: resources.Metadata{UID: "daemon-a", Name: "kruise-daemon-node-a", Namespace: "kruise-system"}, NodeName: "node-a"},
		},
	}
}

func workloadSnapshot(workloadUID string, replicas int32, recursiveOwner bool) collectk8s.Snapshot {
	snapshot := collectk8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: workloadUID, Name: "frontend", Namespace: "default"},
			ControllerKind: "Deployment",
			Replicas:       replicas,
			Conditions:     map[string]string{"Available": "True"},
		}},
		Pods: []resources.Pod{{
			Metadata: resources.Metadata{UID: "pod-uid", Name: "frontend-0", Namespace: "default"},
		}},
	}
	if !recursiveOwner {
		snapshot.Pods[0].OwnerReferences = []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: workloadUID, Controller: true}}
		return snapshot
	}

	snapshot.ReplicaSets = []resources.ReplicaSet{
		{
			Metadata:        resources.Metadata{UID: "rs-root-uid", Name: "frontend-root", Namespace: "default"},
			OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: workloadUID, Controller: true}},
		},
		{
			Metadata:        resources.Metadata{UID: "rs-leaf-uid", Name: "frontend-leaf", Namespace: "default"},
			OwnerReferences: []resources.OwnerReference{{Kind: "ReplicaSet", Name: "frontend-root", UID: "rs-root-uid", Controller: true}},
		},
	}
	snapshot.Pods[0].OwnerReferences = []resources.OwnerReference{{Kind: "ReplicaSet", Name: "frontend-leaf", UID: "rs-leaf-uid", Controller: true}}
	return snapshot
}

func workloadOwnerEdgeKeys(edges []model.Edge) []string {
	out := make([]string, 0)
	for _, edge := range edges {
		if edge.Kind == model.EdgeKindManagedBy || edge.Kind == model.EdgeKindOwnsPod || edge.Kind == model.EdgeKindControlledBy {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}

func workloadNodeFingerprints(nodes []model.Node) []string {
	out := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != model.NodeKindWorkload {
			continue
		}
		out = append(out, node.ID.String()+"|"+node.SourceKind+"|"+node.Name+"|"+fmt.Sprint(node.Attributes["replicas"]))
	}
	sort.Strings(out)
	return out
}
