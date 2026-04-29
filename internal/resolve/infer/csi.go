package infer

import (
	"fmt"
	"sort"
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

type CSIComponentRule struct {
	Driver                string   `json:"driver" yaml:"driver"`
	ComponentNamespace    string   `json:"namespace" yaml:"namespace"`
	ControllerPodPrefixes []string `json:"controllerPodPrefixes" yaml:"controllerPodPrefixes"`
	NodeAgentPodPrefixes  []string `json:"nodeAgentPodPrefixes" yaml:"nodeAgentPodPrefixes"`
}

func EffectiveCSIComponentRules(configured []CSIComponentRule) []CSIComponentRule {
	if len(configured) == 0 {
		return nil
	}
	out := make([]CSIComponentRule, 0, len(configured))
	for _, rule := range configured {
		out = append(out, rule)
	}
	return out
}

func ParseCSIComponentRules(raw string) ([]CSIComponentRule, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]CSIComponentRule, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rule := CSIComponentRule{}
		for _, field := range strings.Split(part, ";") {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				return nil, fmt.Errorf("csi component rule %q field %q must be key=value", part, field)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "driver":
				rule.Driver = value
			case "namespace":
				rule.ComponentNamespace = value
			case "controller":
				rule.ControllerPodPrefixes = splitRuleList(value)
			case "agent", "nodeAgent":
				rule.NodeAgentPodPrefixes = splitRuleList(value)
			default:
				return nil, fmt.Errorf("csi component rule %q has unknown field %q", part, key)
			}
		}
		if rule.Driver == "" {
			return nil, fmt.Errorf("csi component rule %q must set driver", part)
		}
		if len(rule.ControllerPodPrefixes) == 0 && len(rule.NodeAgentPodPrefixes) == 0 {
			return nil, fmt.Errorf("csi component rule %q must set controller or agent", part)
		}
		out = append(out, rule)
	}
	return out, nil
}

func NewCSIComponentRegistry(rules []CSIComponentRule) *Registry {
	correlators := make([]CSICorrelator, 0, len(rules))
	for _, rule := range rules {
		if rule.Driver == "" {
			continue
		}
		correlators = append(correlators, componentRuleCorrelator{rule: rule})
	}
	return NewRegistry(correlators...)
}

func CorrelatePVToCSIComponents(correlator CSICorrelator, pv model.Node, nodeNames []string, infraPods []model.Node) CorrelationResult {
	normalizedNodeNames := normalizeNodeNames(nodeNames)
	if len(normalizedNodeNames) == 0 {
		return correlator.Correlate(pv, "", infraPods)
	}

	result := CorrelationResult{Edges: make([]model.Edge, 0), Evidence: make([]string, 0)}
	seenEdges := make(map[string]struct{})
	seenEvidence := make(map[string]struct{})
	for _, nodeName := range normalizedNodeNames {
		correlation := correlator.Correlate(pv, nodeName, infraPods)
		for _, edge := range correlation.Edges {
			if _, seen := seenEdges[edge.Key()]; seen {
				continue
			}
			result.Edges = append(result.Edges, edge)
			seenEdges[edge.Key()] = struct{}{}
		}
		for _, evidence := range correlation.Evidence {
			if _, seen := seenEvidence[evidence]; seen {
				continue
			}
			result.Evidence = append(result.Evidence, evidence)
			seenEvidence[evidence] = struct{}{}
		}
	}
	return result
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
		if infraPod.Kind != model.NodeKindPod || !rule.matchesComponentNamespace(infraPod.Namespace) {
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

type componentRuleCorrelator struct {
	rule CSIComponentRule
}

func (c componentRuleCorrelator) Driver() string {
	return c.rule.Driver
}

func (c componentRuleCorrelator) Correlate(pv model.Node, affinityNodeName string, infraPods []model.Node) CorrelationResult {
	result := CorrelationResult{Edges: make([]model.Edge, 0), Evidence: make([]string, 0)}
	foundAgent := false
	foundController := false
	for _, infraPod := range infraPods {
		if infraPod.Kind != model.NodeKindPod || !c.rule.matchesComponentNamespace(infraPod.Namespace) {
			continue
		}
		name := infraPod.Name
		if hasAnyPrefix(name, c.rule.ControllerPodPrefixes) {
			result.Edges = append(result.Edges, pvManagedByCSIController(pv.ID, infraPod.ID, "csi-component-rule/"+c.rule.Driver+"/pv-controller/v1"))
			foundController = true
			continue
		}
		if hasAnyPrefix(name, c.rule.NodeAgentPodPrefixes) {
			if affinityNodeName == "" {
				continue
			}
			if podNode, _ := infraPod.Attributes["nodeName"].(string); podNode == affinityNodeName {
				result.Edges = append(result.Edges, pvServedByCSINodeAgent(pv.ID, infraPod.ID, "csi-component-rule/"+c.rule.Driver+"/pv-agent/v1"))
				foundAgent = true
			}
		}
	}
	if len(c.rule.ControllerPodPrefixes) > 0 && !foundController {
		result.Evidence = append(result.Evidence, fmt.Sprintf("csi: no controller component found for PV driver %s", c.rule.Driver))
	}
	if len(c.rule.NodeAgentPodPrefixes) > 0 {
		if affinityNodeName == "" {
			result.Evidence = append(result.Evidence, fmt.Sprintf("csi: PV affinity or consuming pod node missing for driver %s", c.rule.Driver))
		} else if !foundAgent {
			result.Evidence = append(result.Evidence, fmt.Sprintf("csi: no node agent found for driver %s on node %s", c.rule.Driver, affinityNodeName))
		}
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

func (r CSIComponentRule) matchesComponentNamespace(namespace string) bool {
	return r.ComponentNamespace == "" || r.ComponentNamespace == "*" || r.ComponentNamespace == namespace
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func normalizeNodeNames(nodeNames []string) []string {
	seen := make(map[string]struct{}, len(nodeNames))
	for _, nodeName := range nodeNames {
		nodeName = strings.TrimSpace(nodeName)
		if nodeName == "" {
			continue
		}
		seen[nodeName] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for nodeName := range seen {
		out = append(out, nodeName)
	}
	sort.Strings(out)
	return out
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
