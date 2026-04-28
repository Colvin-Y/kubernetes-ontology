package reconcile

import (
	"reflect"
	"sort"
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestStorageReconcilerMatchesFullRebuildStorageEdges(t *testing.T) {
	initial := storageSnapshot("", "")
	next := storageSnapshot("pv-data", "Bound")
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewStorageReconciler("cluster-a", kernel).Apply(next)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected storage update to be applied")
	}
	if result.UpsertedPVCs != 1 || result.UpsertedPVs != 1 || result.UpsertedStorageClasses != 1 || result.UpsertedCSIDrivers != 1 {
		t.Fatalf("expected one storage resource upsert, got pvc=%d pv=%d sc=%d driver=%d", result.UpsertedPVCs, result.UpsertedPVs, result.UpsertedStorageClasses, result.UpsertedCSIDrivers)
	}
	if result.UpsertedEdges != 5 {
		t.Fatalf("expected full storage topology edges, got %d", result.UpsertedEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	got := storageEdgeKeys(kernel.ListEdges())
	want := storageEdgeKeys(full.ListEdges())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("storage edges mismatch\ngot  %#v\nwant %#v", got, want)
	}
	gotNodes := storageNodeFingerprints(kernel.ListNodes())
	wantNodes := storageNodeFingerprints(full.ListNodes())
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("storage nodes mismatch\ngot  %#v\nwant %#v", gotNodes, wantNodes)
	}
}

func TestStorageReconcilerDeletesMissingStorage(t *testing.T) {
	initial := storageSnapshot("pv-data", "Bound")
	next := storageSnapshot("", "")
	next.PVCs = nil
	next.PVs = nil
	next.StorageClasses = nil
	next.CSIDrivers = nil
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewStorageReconciler("cluster-a", kernel).Apply(next)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedNodes != 4 {
		t.Fatalf("expected pvc, pv, storageclass, and csidriver nodes to be deleted, got %d", result.DeletedNodes)
	}
	if len(storageEdgeKeys(kernel.ListEdges())) != 0 {
		t.Fatal("expected storage edges to be removed")
	}
	if len(storageNodeFingerprints(kernel.ListNodes())) != 0 {
		t.Fatal("expected storage nodes to be removed")
	}
}

func TestStorageReconcilerSkipsNonCSIProvisionerDriver(t *testing.T) {
	snapshot := collectk8s.Snapshot{
		PVCs: []resources.PVC{{
			Metadata:         resources.Metadata{UID: "pvc-uid", Name: "data", Namespace: "default"},
			StorageClassName: "manual-local",
			Status:           "Bound",
		}},
		StorageClasses: []resources.StorageClass{{
			Metadata:    resources.Metadata{UID: "sc-uid", Name: "manual-local"},
			Provisioner: "kubernetes.io/no-provisioner",
		}},
	}
	kernel := kernelFromSnapshot(t, "cluster-a", collectk8s.Snapshot{})

	result, err := NewStorageReconciler("cluster-a", kernel).Apply(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpsertedCSIDrivers != 0 {
		t.Fatalf("expected no synthetic csidriver upserts, got %d", result.UpsertedCSIDrivers)
	}
	for _, node := range kernel.ListNodes() {
		if node.Kind == model.NodeKindCSIDriver {
			t.Fatalf("did not expect csidriver node for non-CSI provisioner: %#v", node)
		}
	}
	for _, edge := range kernel.ListEdges() {
		if edge.Kind == model.EdgeKindProvisionedByCSIDriver {
			t.Fatalf("did not expect provisioned_by_csi_driver edge for non-CSI provisioner: %#v", edge)
		}
	}
}

func storageSnapshot(volumeName, status string) collectk8s.Snapshot {
	if status == "" {
		status = "Pending"
	}
	return collectk8s.Snapshot{
		Pods: []resources.Pod{{
			Metadata: resources.Metadata{UID: "pod-uid", Name: "app-0", Namespace: "default"},
			PVCRefs:  []string{"data"},
		}, {
			Metadata: resources.Metadata{UID: "controller-uid", Name: "open-local-controller-0", Namespace: "kube-system"},
		}, {
			Metadata: resources.Metadata{UID: "agent-uid", Name: "open-local-agent-node-a", Namespace: "kube-system"},
			NodeName: "node-a",
		}},
		PVCs: []resources.PVC{{
			Metadata:         resources.Metadata{UID: "pvc-uid", Name: "data", Namespace: "default"},
			VolumeName:       volumeName,
			StorageClassName: "open-local",
			Status:           status,
		}},
		PVs: []resources.PV{{
			Metadata:         resources.Metadata{UID: "pv-uid", Name: "pv-data"},
			StorageClassName: "open-local",
			Status:           status,
			CSI:              map[string]string{"driver": "local.csi.aliyun.com", "handle": "vol-123", "nodeAffinity": "node-a"},
		}},
		StorageClasses: []resources.StorageClass{{
			Metadata:          resources.Metadata{UID: "sc-uid", Name: "open-local"},
			Provisioner:       "local.csi.aliyun.com",
			ReclaimPolicy:     "Delete",
			VolumeBindingMode: "WaitForFirstConsumer",
		}},
		CSIDrivers: []resources.CSIDriver{{
			Metadata: resources.Metadata{UID: "driver-uid", Name: "local.csi.aliyun.com"},
		}},
	}
}

func storageEdgeKeys(edges []model.Edge) []string {
	out := make([]string, 0)
	for _, edge := range edges {
		if isStorageEdge(edge.Kind) {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}

func storageNodeFingerprints(nodes []model.Node) []string {
	out := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != model.NodeKindPVC && node.Kind != model.NodeKindPV && node.Kind != model.NodeKindStorageClass && node.Kind != model.NodeKindCSIDriver {
			continue
		}
		status, _ := node.Attributes["status"].(string)
		provisioner, _ := node.Attributes["provisioner"].(string)
		out = append(out, node.ID.String()+"|"+string(node.Kind)+"|"+status+"|"+provisioner)
	}
	sort.Strings(out)
	return out
}
