package infer

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestInferCSIComponentEdgesUsesExplicitRule(t *testing.T) {
	driver := model.Node{
		ID:   model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "CSIDriver", Name: "local.csi.aliyun.com"}),
		Kind: model.NodeKindCSIDriver,
		Name: "local.csi.aliyun.com",
	}
	controller := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-controller-0"}),
		Kind:      model.NodeKindPod,
		Name:      "open-local-controller-0",
		Namespace: "kube-system",
	}
	agent := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "open-local-agent-node-a"}),
		Kind:      model.NodeKindPod,
		Name:      "open-local-agent-node-a",
		Namespace: "kube-system",
	}

	result := InferCSIComponentEdges(driver, []model.Node{controller, agent}, DefaultCSIComponentRules())
	if len(result.Evidence) != 0 {
		t.Fatalf("expected no missing-component evidence, got %#v", result.Evidence)
	}
	kinds := map[model.EdgeKind]bool{}
	for _, edge := range result.Edges {
		kinds[edge.Kind] = true
	}
	if !kinds[model.EdgeKindImplementedByCSIController] {
		t.Fatal("expected controller implementation edge")
	}
	if !kinds[model.EdgeKindImplementedByCSINodeAgent] {
		t.Fatal("expected node-agent implementation edge")
	}
}

func TestInferCSIComponentEdgesSkipsUnknownDriver(t *testing.T) {
	driver := model.Node{
		ID:   model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "CSIDriver", Name: "unknown.csi.example.com"}),
		Kind: model.NodeKindCSIDriver,
		Name: "unknown.csi.example.com",
	}

	result := InferCSIComponentEdges(driver, nil, DefaultCSIComponentRules())
	if len(result.Edges) != 0 || len(result.Evidence) != 0 {
		t.Fatalf("expected unknown driver to be ignored, got edges=%d evidence=%#v", len(result.Edges), result.Evidence)
	}
}

func TestIsCSIProvisionerAvoidsKnownNonCSIProvisioners(t *testing.T) {
	if !IsCSIProvisioner("local.csi.aliyun.com", false, DefaultCSIComponentRules()) {
		t.Fatal("expected CSI-looking provisioner to resolve")
	}
	if !IsCSIProvisioner("driver.longhorn.io", true, nil) {
		t.Fatal("expected observed CSIDriver to resolve even without csi in name")
	}
	if IsCSIProvisioner("kubernetes.io/no-provisioner", false, DefaultCSIComponentRules()) {
		t.Fatal("expected in-tree no-provisioner to be ignored")
	}
}
