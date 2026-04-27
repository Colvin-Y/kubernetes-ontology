package infer

import (
	"fmt"
	"strings"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/owner"
)

type WorkloadControllerRule struct {
	APIVersion            string   `json:"apiVersion" yaml:"apiVersion"`
	Kind                  string   `json:"kind" yaml:"kind"`
	ControllerNamespace   string   `json:"namespace" yaml:"namespace"`
	ControllerPodPrefixes []string `json:"controllerPodPrefixes" yaml:"controllerPodPrefixes"`
	NodeDaemonPodPrefixes []string `json:"nodeDaemonPodPrefixes" yaml:"nodeDaemonPodPrefixes"`
}

func ParseWorkloadControllerRules(raw string) ([]WorkloadControllerRule, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]WorkloadControllerRule, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rule := WorkloadControllerRule{APIVersion: "*", Kind: "*"}
		for _, field := range strings.Split(part, ";") {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				return nil, fmt.Errorf("controller rule %q field %q must be key=value", part, field)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "apiVersion":
				rule.APIVersion = value
			case "kind":
				rule.Kind = value
			case "namespace":
				rule.ControllerNamespace = value
			case "controller":
				rule.ControllerPodPrefixes = splitRuleList(value)
			case "daemon":
				rule.NodeDaemonPodPrefixes = splitRuleList(value)
			default:
				return nil, fmt.Errorf("controller rule %q has unknown field %q", part, key)
			}
		}
		if len(rule.ControllerPodPrefixes) == 0 && len(rule.NodeDaemonPodPrefixes) == 0 {
			return nil, fmt.Errorf("controller rule %q must set controller or daemon", part)
		}
		out = append(out, rule)
	}
	return out, nil
}

func InferWorkloadControllerEdges(cluster string, snapshot collectk8s.Snapshot, rules []WorkloadControllerRule) []model.Edge {
	if len(rules) == 0 {
		return nil
	}
	resolver := owner.NewChainResolver(cluster, snapshot.Workloads, snapshot.ReplicaSets)
	podIDs := workloadControllerPodIDs(cluster, snapshot.Pods)
	out := make([]model.Edge, 0)
	for _, workload := range snapshot.Workloads {
		workloadID := model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
		ownedNodes := workloadPodNodes(workloadID, snapshot.Pods, resolver)
		for _, rule := range rules {
			if !rule.matches(workload) {
				continue
			}
			for _, pod := range snapshot.Pods {
				if !rule.matchesControllerNamespace(pod.Metadata.Namespace) {
					continue
				}
				podID := podIDs[pod.Metadata.Namespace+"/"+pod.Metadata.Name]
				if podID == "" {
					continue
				}
				if hasAnyPrefix(pod.Metadata.Name, rule.ControllerPodPrefixes) {
					out = append(out, WorkloadManagedByController(workloadID, podID))
				}
				if pod.NodeName != "" && ownedNodes[pod.NodeName] && hasAnyPrefix(pod.Metadata.Name, rule.NodeDaemonPodPrefixes) {
					out = append(out, WorkloadServedByNodeDaemon(workloadID, podID))
				}
			}
		}
	}
	return out
}

func (r WorkloadControllerRule) matches(workload resources.Workload) bool {
	return rulePatternMatches(r.APIVersion, workload.APIVersion) && rulePatternMatches(r.Kind, workload.ControllerKind)
}

func (r WorkloadControllerRule) matchesControllerNamespace(namespace string) bool {
	return r.ControllerNamespace == "" || r.ControllerNamespace == "*" || r.ControllerNamespace == namespace
}

func splitRuleList(raw string) []string {
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func rulePatternMatches(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func workloadControllerPodIDs(cluster string, pods []resources.Pod) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(pods))
	for _, pod := range pods {
		out[pod.Metadata.Namespace+"/"+pod.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
	}
	return out
}

func workloadPodNodes(workloadID model.CanonicalID, pods []resources.Pod, resolver *owner.ChainResolver) map[string]bool {
	out := make(map[string]bool)
	for _, pod := range pods {
		if pod.NodeName == "" {
			continue
		}
		for _, target := range resolver.ResolvePodWorkloads(pod) {
			if target.ID == workloadID {
				out[pod.NodeName] = true
				break
			}
		}
	}
	return out
}

func WorkloadManagedByController(workloadID, controllerPodID model.CanonicalID) model.Edge {
	return model.NewEdge(workloadID, controllerPodID, model.EdgeKindManagedByController)
}

func WorkloadServedByNodeDaemon(workloadID, daemonPodID model.CanonicalID) model.Edge {
	return model.NewEdge(workloadID, daemonPodID, model.EdgeKindServedByNodeDaemon)
}
