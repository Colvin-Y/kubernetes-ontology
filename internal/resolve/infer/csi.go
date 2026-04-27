package infer

import (
	"fmt"
	"strings"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

type CorrelationResult struct {
	Edges    []model.Edge
	Evidence []string
}

type CSICorrelator interface {
	Driver() string
	Correlate(pv model.Node, affinityNodeName string, infraPods []model.Node) CorrelationResult
}

type Registry struct {
	correlators map[string]CSICorrelator
}

func NewRegistry(correlators ...CSICorrelator) *Registry {
	m := make(map[string]CSICorrelator, len(correlators))
	for _, correlator := range correlators {
		m[correlator.Driver()] = correlator
	}
	return &Registry{correlators: m}
}

func (r *Registry) Correlator(driver string) (CSICorrelator, bool) {
	correlator, ok := r.correlators[driver]
	return correlator, ok
}

type OpenLocalCorrelator struct{}

type CSIComponentRule struct {
	Driver                string
	ControllerPodPrefixes []string
	NodeAgentPodPrefixes  []string
}

func DefaultCSIComponentRules() []CSIComponentRule {
	return []CSIComponentRule{{
		Driver:                "local.csi.aliyun.com",
		ControllerPodPrefixes: []string{"open-local-controller-", "open-local-scheduler-extender-"},
		NodeAgentPodPrefixes:  []string{"open-local-agent-"},
	}}
}

func IsCSIProvisioner(provisioner string, observedCSIDriver bool, rules []CSIComponentRule) bool {
	if provisioner == "" {
		return false
	}
	if observedCSIDriver {
		return true
	}
	if _, ok := componentRuleForDriver(provisioner, rules); ok {
		return true
	}
	return looksLikeCSIProvisioner(provisioner)
}

func InferCSIComponentEdges(driver model.Node, infraPods []model.Node, rules []CSIComponentRule) CorrelationResult {
	result := CorrelationResult{Edges: make([]model.Edge, 0), Evidence: make([]string, 0)}
	rule, ok := componentRuleForDriver(driver.Name, rules)
	if !ok {
		return result
	}
	foundController := false
	foundAgent := false
	for _, infraPod := range infraPods {
		if infraPod.Kind != model.NodeKindPod || infraPod.Namespace != "kube-system" {
			continue
		}
		if hasAnyPrefix(infraPod.Name, rule.ControllerPodPrefixes) {
			result.Edges = append(result.Edges, csiDriverImplementedByController(driver.ID, infraPod.ID, "csi-component-rule/"+rule.Driver+"/v1"))
			foundController = true
			continue
		}
		if hasAnyPrefix(infraPod.Name, rule.NodeAgentPodPrefixes) {
			result.Edges = append(result.Edges, csiDriverImplementedByNodeAgent(driver.ID, infraPod.ID, "csi-component-rule/"+rule.Driver+"/v1"))
			foundAgent = true
		}
	}
	if !foundController {
		result.Evidence = append(result.Evidence, fmt.Sprintf("csi: no controller component found for driver %s", driver.Name))
	}
	if !foundAgent {
		result.Evidence = append(result.Evidence, fmt.Sprintf("csi: no node-agent component found for driver %s", driver.Name))
	}
	return result
}

func componentRuleForDriver(driver string, rules []CSIComponentRule) (CSIComponentRule, bool) {
	for _, rule := range rules {
		if rule.Driver == driver {
			return rule, true
		}
	}
	return CSIComponentRule{}, false
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func looksLikeCSIProvisioner(provisioner string) bool {
	if strings.HasPrefix(provisioner, "kubernetes.io/") {
		return false
	}
	return strings.HasPrefix(provisioner, "csi.") ||
		strings.Contains(provisioner, ".csi.") ||
		strings.Contains(provisioner, "-csi.") ||
		strings.Contains(provisioner, ".csi-") ||
		strings.HasSuffix(provisioner, ".csi")
}

func (OpenLocalCorrelator) Driver() string {
	return "local.csi.aliyun.com"
}

func (OpenLocalCorrelator) Correlate(pv model.Node, affinityNodeName string, infraPods []model.Node) CorrelationResult {
	result := CorrelationResult{Edges: make([]model.Edge, 0), Evidence: make([]string, 0)}
	foundAgent := false
	for _, infraPod := range infraPods {
		if infraPod.Kind != model.NodeKindPod || infraPod.Namespace != "kube-system" {
			continue
		}
		name := infraPod.Name
		if strings.HasPrefix(name, "open-local-agent-") {
			if affinityNodeName != "" {
				if podNode, _ := infraPod.Attributes["nodeName"].(string); podNode == affinityNodeName {
					result.Edges = append(result.Edges, pvServedByCSINodeAgent(pv.ID, infraPod.ID, "csi-open-local-agent/v1"))
					foundAgent = true
				}
			}
			continue
		}
		if strings.HasPrefix(name, "open-local-controller-") || strings.HasPrefix(name, "open-local-scheduler-extender-") {
			result.Edges = append(result.Edges, pvManagedByCSIController(pv.ID, infraPod.ID, "csi-open-local-controller/v1"))
		}
	}
	if affinityNodeName == "" {
		result.Evidence = append(result.Evidence, "csi: PV affinity node missing for driver local.csi.aliyun.com")
	} else if !foundAgent {
		result.Evidence = append(result.Evidence, fmt.Sprintf("csi: no open-local node agent found on PV affinity node %s", affinityNodeName))
	}
	return result
}

func pvServedByCSINodeAgent(pvID, targetID model.CanonicalID, resolver string) model.Edge {
	return model.NewEdgeWithResolver(pvID, targetID, model.EdgeKindServedByCSINodeAgent, resolver)
}

func pvManagedByCSIController(pvID, targetID model.CanonicalID, resolver string) model.Edge {
	return model.NewEdgeWithResolver(pvID, targetID, model.EdgeKindManagedByCSIController, resolver)
}

func csiDriverImplementedByController(driverID, targetID model.CanonicalID, resolver string) model.Edge {
	return model.NewEdgeWithResolver(driverID, targetID, model.EdgeKindImplementedByCSIController, resolver)
}

func csiDriverImplementedByNodeAgent(driverID, targetID model.CanonicalID, resolver string) model.Edge {
	return model.NewEdgeWithResolver(driverID, targetID, model.EdgeKindImplementedByCSINodeAgent, resolver)
}
