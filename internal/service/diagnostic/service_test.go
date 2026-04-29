package diagnostic

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

func TestGetDiagnosticSubgraph(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	workload := model.Node{
		ID:         model.WorkloadID("cluster-a", "default", "Deployment", "frontend", "w1"),
		Kind:       model.NodeKindWorkload,
		SourceKind: "Deployment",
		Name:       "frontend",
		Namespace:  "default",
	}
	pod := model.Node{
		ID:         model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "frontend-abc123", UID: "p1"}),
		Kind:       model.NodeKindPod,
		SourceKind: "Pod",
		Name:       "frontend-abc123",
		Namespace:  "default",
	}

	if err := kernel.UpsertNode(workload); err != nil {
		t.Fatal(err)
	}
	if err := kernel.UpsertNode(pod); err != nil {
		t.Fatal(err)
	}
	if err := kernel.UpsertEdge(model.Edge{
		From: pod.ID,
		To:   workload.ID,
		Kind: model.EdgeKindManagedBy,
		Provenance: model.EdgeProvenance{
			SourceType: model.EdgeSourceTypeInferenceRule,
			State:      model.EdgeStateInferred,
			Resolver:   "owner-chain/v1",
		},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{
		Kind:        api.NodeKindPod,
		CanonicalID: pod.ID.String(),
	}, api.ExpansionPolicy{MaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Edges))
	}
	if result.Edges[0].Kind != api.EdgeKindManagedBy {
		t.Fatalf("unexpected edge kind: %s", result.Edges[0].Kind)
	}
}

func TestGetDiagnosticSubgraphByPodFindsCanonicalEntry(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	pod := model.Node{
		ID:         model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "frontend-abc123", UID: "p1"}),
		Kind:       model.NodeKindPod,
		SourceKind: "Pod",
		Name:       "frontend-abc123",
		Namespace:  "default",
	}
	if err := kernel.UpsertNode(pod); err != nil {
		t.Fatal(err)
	}

	result, err := service.GetDiagnosticSubgraphByPod("default", "frontend-abc123", api.ExpansionPolicy{MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Entry.CanonicalID != pod.ID.String() {
		t.Fatalf("expected canonical entry id %s, got %s", pod.ID.String(), result.Entry.CanonicalID)
	}
}

func TestGetDiagnosticSubgraphTraversesStorageClassAndCSIDriver(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	pod := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "app-0", UID: "p1"}), Kind: model.NodeKindPod, Name: "app-0", Namespace: "default"}
	pvc := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PVC", Namespace: "default", Name: "data", UID: "pvc1"}), Kind: model.NodeKindPVC, Name: "data", Namespace: "default"}
	otherPVC := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PVC", Namespace: "default", Name: "other-data", UID: "pvc2"}), Kind: model.NodeKindPVC, Name: "other-data", Namespace: "default"}
	otherPV := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PV", Name: "other-pv", UID: "pv2"}), Kind: model.NodeKindPV, Name: "other-pv"}
	storageClass := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "StorageClass", Name: "open-local", UID: "sc1"}), Kind: model.NodeKindStorageClass, Name: "open-local"}
	otherStorageClass := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "StorageClass", Name: "other-open-local", UID: "sc2"}), Kind: model.NodeKindStorageClass, Name: "other-open-local"}
	driver := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "CSIDriver", Name: "local.csi.aliyun.com", UID: "driver1"}), Kind: model.NodeKindCSIDriver, Name: "local.csi.aliyun.com"}
	controller := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-controller-0", UID: "c1"}), Kind: model.NodeKindPod, Name: "open-local-controller-0", Namespace: "kube-system"}

	for _, node := range []model.Node{pod, pvc, otherPVC, otherPV, storageClass, otherStorageClass, driver, controller} {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []model.Edge{
		{From: pod.ID, To: pvc.ID, Kind: model.EdgeKindMountsPVC},
		{From: pvc.ID, To: storageClass.ID, Kind: model.EdgeKindUsesStorageClass},
		{From: otherPVC.ID, To: otherPV.ID, Kind: model.EdgeKindBoundToPV},
		{From: otherPVC.ID, To: storageClass.ID, Kind: model.EdgeKindUsesStorageClass},
		{From: otherPV.ID, To: storageClass.ID, Kind: model.EdgeKindUsesStorageClass},
		{From: storageClass.ID, To: driver.ID, Kind: model.EdgeKindProvisionedByCSIDriver},
		{From: otherStorageClass.ID, To: driver.ID, Kind: model.EdgeKindProvisionedByCSIDriver},
		{From: driver.ID, To: controller.ID, Kind: model.EdgeKindImplementedByCSIController},
	} {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
	}

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: pod.ID.String()}, api.ExpansionPolicy{
		MaxDepth:        1,
		StorageMaxDepth: 4,
		IncludeStorage:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticContainsKind(result.Nodes, api.NodeKindCSIDriver) {
		t.Fatalf("expected storage traversal to include csi driver, got %#v", result.Nodes)
	}
	if !diagnosticContainsNode(result.Nodes, "open-local-controller-0") {
		t.Fatalf("expected storage traversal to include csi controller, got %#v", result.Nodes)
	}
	if diagnosticContainsNode(result.Nodes, "other-data") || diagnosticContainsNode(result.Nodes, "other-pv") || diagnosticContainsNode(result.Nodes, "other-open-local") {
		t.Fatalf("expected storage traversal to avoid sibling storage fan-out, got %#v", result.Nodes)
	}

	withoutStorage, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: pod.ID.String()}, api.ExpansionPolicy{
		MaxDepth:        4,
		StorageMaxDepth: 4,
		IncludeStorage:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnosticContainsKind(withoutStorage.Nodes, api.NodeKindPVC) {
		t.Fatalf("expected storage traversal to be excluded, got %#v", withoutStorage.Nodes)
	}
}

func TestGetDiagnosticSubgraphScopesCSINodeAgentsForPodStoragePath(t *testing.T) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	service := NewService(kernel)

	pod := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "default", Name: "app-0", UID: "p1"}), Kind: model.NodeKindPod, Name: "app-0", Namespace: "default", Attributes: map[string]any{"nodeName": "node-a"}}
	pvc := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PVC", Namespace: "default", Name: "data", UID: "pvc1"}), Kind: model.NodeKindPVC, Name: "data", Namespace: "default"}
	pv := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PV", Name: "pv-data", UID: "pv1"}), Kind: model.NodeKindPV, Name: "pv-data"}
	storageClass := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "StorageClass", Name: "open-local", UID: "sc1"}), Kind: model.NodeKindStorageClass, Name: "open-local"}
	driver := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "CSIDriver", Name: "local.csi.aliyun.com", UID: "driver1"}), Kind: model.NodeKindCSIDriver, Name: "local.csi.aliyun.com"}
	controller := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-controller-0", UID: "c1"}), Kind: model.NodeKindPod, Name: "open-local-controller-0", Namespace: "kube-system"}
	sameNodeAgent := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-agent-node-a", UID: "a1"}), Kind: model.NodeKindPod, Name: "open-local-agent-node-a", Namespace: "kube-system", Attributes: map[string]any{"nodeName": "node-a"}}
	otherNodeAgent := model.Node{ID: model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-agent-node-b", UID: "a2"}), Kind: model.NodeKindPod, Name: "open-local-agent-node-b", Namespace: "kube-system", Attributes: map[string]any{"nodeName": "node-b"}}

	for _, node := range []model.Node{pod, pvc, pv, storageClass, driver, controller, sameNodeAgent, otherNodeAgent} {
		if err := kernel.UpsertNode(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, edge := range []model.Edge{
		model.NewEdge(pod.ID, pvc.ID, model.EdgeKindMountsPVC),
		model.NewEdge(pvc.ID, pv.ID, model.EdgeKindBoundToPV),
		model.NewEdge(pvc.ID, storageClass.ID, model.EdgeKindUsesStorageClass),
		model.NewEdge(pv.ID, storageClass.ID, model.EdgeKindUsesStorageClass),
		model.NewEdge(storageClass.ID, driver.ID, model.EdgeKindProvisionedByCSIDriver),
		model.NewEdge(pv.ID, sameNodeAgent.ID, model.EdgeKindServedByCSINodeAgent),
		model.NewEdge(pv.ID, otherNodeAgent.ID, model.EdgeKindServedByCSINodeAgent),
		model.NewEdge(driver.ID, controller.ID, model.EdgeKindImplementedByCSIController),
		model.NewEdge(driver.ID, sameNodeAgent.ID, model.EdgeKindImplementedByCSINodeAgent),
		model.NewEdge(driver.ID, otherNodeAgent.ID, model.EdgeKindImplementedByCSINodeAgent),
	} {
		if err := kernel.UpsertEdge(edge); err != nil {
			t.Fatal(err)
		}
	}

	result, err := service.GetDiagnosticSubgraph(api.EntryRef{Kind: api.NodeKindPod, CanonicalID: pod.ID.String()}, api.ExpansionPolicy{
		MaxDepth:        1,
		StorageMaxDepth: 5,
		IncludeStorage:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticContainsNode(result.Nodes, "open-local-agent-node-a") {
		t.Fatalf("expected pod storage traversal to include same-node CSI agent, got %#v", result.Nodes)
	}
	if diagnosticContainsNode(result.Nodes, "open-local-agent-node-b") {
		t.Fatalf("expected pod storage traversal to exclude off-node CSI agent, got %#v", result.Nodes)
	}
	if !diagnosticContainsNode(result.Nodes, "open-local-controller-0") {
		t.Fatalf("expected pod storage traversal to include CSI controller, got %#v", result.Nodes)
	}
}

func diagnosticContainsKind(nodes []api.DiagnosticNode, kind api.NodeKind) bool {
	for _, node := range nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func diagnosticContainsNode(nodes []api.DiagnosticNode, name string) bool {
	for _, node := range nodes {
		if node.Name == name {
			return true
		}
	}
	return false
}
