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

	result := InferCSIComponentEdges(driver, []model.Node{controller, agent}, openLocalTestRules())
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

	result := InferCSIComponentEdges(driver, nil, openLocalTestRules())
	if len(result.Edges) != 0 || len(result.Evidence) != 0 {
		t.Fatalf("expected unknown driver to be ignored, got edges=%d evidence=%#v", len(result.Edges), result.Evidence)
	}
}

func TestInferCSIComponentEdgesUsesConfiguredNamespace(t *testing.T) {
	rules := []CSIComponentRule{{
		Driver:                "diskplugin.csi.alibabacloud.com",
		ComponentNamespace:    "storage-system",
		ControllerPodPrefixes: []string{"disk-controller-"},
		NodeAgentPodPrefixes:  []string{"disk-agent-"},
	}}
	driver := model.Node{
		ID:   model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "storage.k8s.io", Kind: "CSIDriver", Name: "diskplugin.csi.alibabacloud.com"}),
		Kind: model.NodeKindCSIDriver,
		Name: "diskplugin.csi.alibabacloud.com",
	}
	controller := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "storage-system", Name: "disk-controller-0"}),
		Kind:      model.NodeKindPod,
		Name:      "disk-controller-0",
		Namespace: "storage-system",
	}
	agent := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "storage-system", Name: "disk-agent-node-a"}),
		Kind:      model.NodeKindPod,
		Name:      "disk-agent-node-a",
		Namespace: "storage-system",
	}
	otherNamespace := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "kube-system", Name: "disk-controller-1"}),
		Kind:      model.NodeKindPod,
		Name:      "disk-controller-1",
		Namespace: "kube-system",
	}

	result := InferCSIComponentEdges(driver, []model.Node{controller, agent, otherNamespace}, rules)
	if len(result.Edges) != 2 {
		t.Fatalf("expected only configured namespace components, got %d edges", len(result.Edges))
	}
}

func TestCSIComponentRuleCorrelatesPVToComponents(t *testing.T) {
	rules := []CSIComponentRule{{
		Driver:                "diskplugin.csi.alibabacloud.com",
		ComponentNamespace:    "storage-system",
		ControllerPodPrefixes: []string{"disk-controller-"},
		NodeAgentPodPrefixes:  []string{"disk-agent-"},
	}}
	registry := NewCSIComponentRegistry(rules)
	correlator, ok := registry.Correlator("diskplugin.csi.alibabacloud.com")
	if !ok {
		t.Fatal("expected correlator for configured driver")
	}
	pv := model.Node{
		ID:   model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "PV", Name: "pv-data"}),
		Kind: model.NodeKindPV,
		Name: "pv-data",
	}
	controller := model.Node{
		ID:        model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "storage-system", Name: "disk-controller-0"}),
		Kind:      model.NodeKindPod,
		Name:      "disk-controller-0",
		Namespace: "storage-system",
	}
	agent := model.Node{
		ID:         model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "storage-system", Name: "disk-agent-node-a"}),
		Kind:       model.NodeKindPod,
		Name:       "disk-agent-node-a",
		Namespace:  "storage-system",
		Attributes: map[string]any{"nodeName": "node-a"},
	}
	otherAgent := model.Node{
		ID:         model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "Pod", Namespace: "storage-system", Name: "disk-agent-node-b"}),
		Kind:       model.NodeKindPod,
		Name:       "disk-agent-node-b",
		Namespace:  "storage-system",
		Attributes: map[string]any{"nodeName": "node-b"},
	}

	result := correlator.Correlate(pv, "node-a", []model.Node{controller, agent, otherAgent})
	kinds := map[model.EdgeKind]bool{}
	servedByAgentEdges := 0
	for _, edge := range result.Edges {
		kinds[edge.Kind] = true
		if edge.Kind == model.EdgeKindServedByCSINodeAgent {
			servedByAgentEdges++
			if edge.To != agent.ID {
				t.Fatalf("expected PV node-agent edge to target same-node agent, got %s", edge.To)
			}
		}
	}
	if !kinds[model.EdgeKindManagedByCSIController] {
		t.Fatal("expected PV controller edge")
	}
	if !kinds[model.EdgeKindServedByCSINodeAgent] {
		t.Fatal("expected PV node-agent edge")
	}
	if servedByAgentEdges != 1 {
		t.Fatalf("expected exactly one PV node-agent edge, got %d", servedByAgentEdges)
	}
}

func TestParseCSIComponentRules(t *testing.T) {
	rules, err := ParseCSIComponentRules("driver=diskplugin.csi.alibabacloud.com;namespace=storage-system;controller=disk-controller-|disk-provisioner-;agent=disk-agent-")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rules); got != 1 {
		t.Fatalf("rule count = %d, want 1", got)
	}
	if rules[0].Driver != "diskplugin.csi.alibabacloud.com" || rules[0].ComponentNamespace != "storage-system" {
		t.Fatalf("unexpected rule: %#v", rules[0])
	}
	if got := len(rules[0].ControllerPodPrefixes); got != 2 {
		t.Fatalf("controller prefix count = %d, want 2", got)
	}
}

func TestIsCSIProvisionerAvoidsKnownNonCSIProvisioners(t *testing.T) {
	if len(EffectiveCSIComponentRules(nil)) != 0 {
		t.Fatal("expected no built-in CSI component rules")
	}
	if !IsCSIProvisioner("local.csi.aliyun.com", false, nil) {
		t.Fatal("expected CSI-looking provisioner to resolve")
	}
	if !IsCSIProvisioner("local.csi.aliyun.com", false, openLocalTestRules()) {
		t.Fatal("expected configured provisioner to resolve")
	}
	if !IsCSIProvisioner("driver.longhorn.io", true, nil) {
		t.Fatal("expected observed CSIDriver to resolve even without csi in name")
	}
	if IsCSIProvisioner("kubernetes.io/no-provisioner", false, openLocalTestRules()) {
		t.Fatal("expected in-tree no-provisioner to be ignored")
	}
}

func openLocalTestRules() []CSIComponentRule {
	return []CSIComponentRule{{
		Driver:                "local.csi.aliyun.com",
		ComponentNamespace:    "kube-system",
		ControllerPodPrefixes: []string{"open-local-controller-", "open-local-scheduler-extender-"},
		NodeAgentPodPrefixes:  []string{"open-local-agent-"},
	}}
}
