package infer

import (
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestParseWorkloadControllerRules(t *testing.T) {
	rules, err := ParseWorkloadControllerRules("apiVersion=apps.kruise.io/*;kind=*;namespace=kruise-system;controller=kruise-controller-manager;daemon=kruise-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %d", len(rules))
	}
	rule := rules[0]
	if rule.APIVersion != "apps.kruise.io/*" || rule.Kind != "*" || rule.ControllerNamespace != "kruise-system" {
		t.Fatalf("unexpected rule fields: %#v", rule)
	}
	if len(rule.ControllerPodPrefixes) != 1 || rule.ControllerPodPrefixes[0] != "kruise-controller-manager" {
		t.Fatalf("unexpected controller prefixes: %#v", rule.ControllerPodPrefixes)
	}
	if len(rule.NodeDaemonPodPrefixes) != 1 || rule.NodeDaemonPodPrefixes[0] != "kruise-daemon" {
		t.Fatalf("unexpected daemon prefixes: %#v", rule.NodeDaemonPodPrefixes)
	}
}

func TestInferWorkloadControllerEdges(t *testing.T) {
	snapshot := collectk8s.Snapshot{
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
			{Metadata: resources.Metadata{UID: "daemon-b", Name: "kruise-daemon-node-b", Namespace: "kruise-system"}, NodeName: "node-b"},
		},
	}

	edges := InferWorkloadControllerEdges("cluster-a", snapshot, []WorkloadControllerRule{{
		APIVersion:            "apps.kruise.io/*",
		Kind:                  "*",
		ControllerNamespace:   "kruise-system",
		ControllerPodPrefixes: []string{"kruise-controller-manager"},
		NodeDaemonPodPrefixes: []string{"kruise-daemon"},
	}})
	kinds := map[model.EdgeKind]int{}
	for _, edge := range edges {
		kinds[edge.Kind]++
	}
	if kinds[model.EdgeKindManagedByController] != 1 {
		t.Fatalf("expected controller manager edge, got %d", kinds[model.EdgeKindManagedByController])
	}
	if kinds[model.EdgeKindServedByNodeDaemon] != 1 {
		t.Fatalf("expected same-node daemon edge, got %d", kinds[model.EdgeKindServedByNodeDaemon])
	}
}
