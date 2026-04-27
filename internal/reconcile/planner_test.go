package reconcile

import (
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
)

func TestStrategyForEvent(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want Strategy
	}{
		{name: "event", kind: "event", want: StrategyEventNarrow},
		{name: "pod", kind: "Pod", want: StrategyPodNarrow},
		{name: "service", kind: "Service", want: StrategyServiceNarrow},
		{name: "storage", kind: "PersistentVolumeClaim", want: StrategyStorageNarrow},
		{name: "workload", kind: "Deployment", want: StrategyWorkloadNarrow},
		{name: "identity category", kind: "identity/security", want: StrategyIdentitySecurityNarrow},
		{name: "serviceaccount", kind: "ServiceAccount", want: StrategyIdentitySecurityNarrow},
		{name: "rolebinding", kind: "RoleBinding", want: StrategyIdentitySecurityNarrow},
		{name: "clusterrolebinding", kind: "ClusterRoleBinding", want: StrategyIdentitySecurityNarrow},
		{name: "fallback", kind: "Namespace", want: StrategyFullRebuild},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StrategyForEvent(collectk8s.ChangeEvent{Kind: tt.kind})
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestPlannerIncludesScopeAndStrategy(t *testing.T) {
	planner := NewNoopPlanner()
	plan := planner.Plan(collectk8s.ChangeEvent{
		Kind:      "service",
		Namespace: "default",
		Name:      "frontend",
	})

	if plan.Strategy != StrategyServiceNarrow {
		t.Fatalf("expected service strategy, got %s", plan.Strategy)
	}
	if len(plan.Scope.Kinds) != 1 || plan.Scope.Kinds[0] != "service" {
		t.Fatalf("expected service scope, got %#v", plan.Scope.Kinds)
	}
	if len(plan.Scope.Namespaces) != 1 || plan.Scope.Namespaces[0] != "default" {
		t.Fatalf("expected default namespace scope, got %#v", plan.Scope.Namespaces)
	}
	if len(plan.Scope.Names) != 1 || plan.Scope.Names[0] != "frontend" {
		t.Fatalf("expected frontend name scope, got %#v", plan.Scope.Names)
	}
}
