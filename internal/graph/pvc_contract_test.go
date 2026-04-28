package graph_test

import (
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

func TestBuilderBuildPVCToPV(t *testing.T) {
	builder := graph.NewBuilder("cluster-a")
	snapshot := collectk8s.Snapshot{
		PVCs: []resources.PVC{{Metadata: resources.Metadata{UID: "pvc-uid", Name: "data-app-0", Namespace: "default"}, VolumeName: "pv-data-app-0", Status: "Bound"}},
		PVs:  []resources.PV{{Metadata: resources.Metadata{UID: "pv-uid", Name: "pv-data-app-0"}, Status: "Bound", CSI: map[string]string{"driver": "csi.example.io", "handle": "vol-123"}}},
	}

	_, edges := builder.Build(snapshot)
	found := false
	for _, edge := range edges {
		if string(edge.Kind) == "bound_to_pv" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected pvc to pv edge")
	}
}

func TestBuilderBuildsPVCStorageTopology(t *testing.T) {
	builder := graph.NewBuilder("cluster-a")
	builder.SetCSIComponentRules(openLocalCSIComponentRules())
	snapshot := storageTopologySnapshot()

	nodes, edges := builder.Build(snapshot)
	nodeKinds := map[model.NodeKind]bool{}
	for _, node := range nodes {
		nodeKinds[node.Kind] = true
	}
	for _, kind := range []model.NodeKind{model.NodeKindPVC, model.NodeKindPV, model.NodeKindStorageClass, model.NodeKindCSIDriver, model.NodeKindPod} {
		if !nodeKinds[kind] {
			t.Fatalf("expected node kind %s in storage topology", kind)
		}
	}

	edgeKinds := map[model.EdgeKind]bool{}
	for _, edge := range edges {
		edgeKinds[edge.Kind] = true
	}
	for _, kind := range []model.EdgeKind{
		model.EdgeKindMountsPVC,
		model.EdgeKindBoundToPV,
		model.EdgeKindUsesStorageClass,
		model.EdgeKindProvisionedByCSIDriver,
		model.EdgeKindImplementedByCSIController,
		model.EdgeKindImplementedByCSINodeAgent,
	} {
		if !edgeKinds[kind] {
			t.Fatalf("expected edge kind %s in storage topology", kind)
		}
	}
}

func TestBuilderDoesNotSynthesizeCSIDriverForNonCSIProvisioner(t *testing.T) {
	builder := graph.NewBuilder("cluster-a")
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

	nodes, edges := builder.Build(snapshot)
	for _, node := range nodes {
		if node.Kind == model.NodeKindCSIDriver {
			t.Fatalf("did not expect synthetic CSIDriver for non-CSI provisioner: %#v", node)
		}
	}
	for _, edge := range edges {
		if edge.Kind == model.EdgeKindProvisionedByCSIDriver {
			t.Fatalf("did not expect provisioned_by_csi_driver edge for non-CSI provisioner: %#v", edge)
		}
	}
}

func TestBuilderUsesConfiguredCSIComponentRule(t *testing.T) {
	builder := graph.NewBuilder("cluster-a")
	builder.SetCSIComponentRules([]infer.CSIComponentRule{{
		Driver:                "diskplugin.csi.alibabacloud.com",
		ComponentNamespace:    "storage-system",
		ControllerPodPrefixes: []string{"disk-controller-"},
		NodeAgentPodPrefixes:  []string{"disk-agent-"},
	}})
	snapshot := collectk8s.Snapshot{
		Pods: []resources.Pod{
			{
				Metadata: resources.Metadata{UID: "controller-uid", Name: "disk-controller-0", Namespace: "storage-system"},
			},
			{
				Metadata: resources.Metadata{UID: "agent-uid", Name: "disk-agent-node-a", Namespace: "storage-system"},
				NodeName: "node-a",
			},
		},
		PVCs: []resources.PVC{{
			Metadata:         resources.Metadata{UID: "pvc-uid", Name: "data", Namespace: "default"},
			VolumeName:       "pv-data",
			StorageClassName: "cloud-disk",
		}},
		PVs: []resources.PV{{
			Metadata:         resources.Metadata{UID: "pv-uid", Name: "pv-data"},
			StorageClassName: "cloud-disk",
			CSI:              map[string]string{"driver": "diskplugin.csi.alibabacloud.com", "nodeAffinity": "node-a"},
		}},
		StorageClasses: []resources.StorageClass{{
			Metadata:    resources.Metadata{UID: "sc-uid", Name: "cloud-disk"},
			Provisioner: "diskplugin.csi.alibabacloud.com",
		}},
	}

	nodes, edges := builder.Build(snapshot)
	if !containsNodeKind(nodes, model.NodeKindCSIDriver) {
		t.Fatal("expected synthetic CSIDriver from configured provisioner")
	}
	edgeKinds := map[model.EdgeKind]bool{}
	for _, edge := range edges {
		edgeKinds[edge.Kind] = true
	}
	for _, kind := range []model.EdgeKind{
		model.EdgeKindProvisionedByCSIDriver,
		model.EdgeKindImplementedByCSIController,
		model.EdgeKindImplementedByCSINodeAgent,
		model.EdgeKindManagedByCSIController,
		model.EdgeKindServedByCSINodeAgent,
	} {
		if !edgeKinds[kind] {
			t.Fatalf("expected configured CSI edge kind %s", kind)
		}
	}
}

func storageTopologySnapshot() collectk8s.Snapshot {
	return collectk8s.Snapshot{
		Pods: []resources.Pod{
			{
				Metadata: resources.Metadata{UID: "pod-uid", Name: "app-0", Namespace: "default"},
				PVCRefs:  []string{"data"},
			},
			{
				Metadata: resources.Metadata{UID: "csi-controller-uid", Name: "open-local-controller-0", Namespace: "kube-system"},
			},
			{
				Metadata: resources.Metadata{UID: "csi-agent-uid", Name: "open-local-agent-node-a", Namespace: "kube-system"},
				NodeName: "node-a",
			},
		},
		PVCs: []resources.PVC{{
			Metadata:         resources.Metadata{UID: "pvc-uid", Name: "data", Namespace: "default"},
			VolumeName:       "pv-data",
			StorageClassName: "open-local",
			Status:           "Bound",
		}},
		PVs: []resources.PV{{
			Metadata:         resources.Metadata{UID: "pv-uid", Name: "pv-data"},
			StorageClassName: "open-local",
			Status:           "Bound",
			CSI:              map[string]string{"driver": "local.csi.aliyun.com", "handle": "vol-123", "nodeAffinity": "node-a"},
		}},
		StorageClasses: []resources.StorageClass{{
			Metadata:    resources.Metadata{UID: "sc-uid", Name: "open-local"},
			Provisioner: "local.csi.aliyun.com",
		}},
		CSIDrivers: []resources.CSIDriver{{
			Metadata: resources.Metadata{UID: "driver-uid", Name: "local.csi.aliyun.com"},
		}},
	}
}

func containsNodeKind(nodes []model.Node, kind model.NodeKind) bool {
	for _, node := range nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func openLocalCSIComponentRules() []infer.CSIComponentRule {
	return []infer.CSIComponentRule{{
		Driver:                "local.csi.aliyun.com",
		ComponentNamespace:    "kube-system",
		ControllerPodPrefixes: []string{"open-local-controller-", "open-local-scheduler-extender-"},
		NodeAgentPodPrefixes:  []string{"open-local-agent-"},
	}}
}
