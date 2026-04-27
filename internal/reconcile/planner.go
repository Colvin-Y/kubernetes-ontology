package reconcile

import (
	"strings"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
)

type Planner interface {
	Plan(event collectk8s.ChangeEvent) Plan
}

type Scope struct {
	Kinds      []string
	Namespaces []string
	Names      []string
}

type Strategy string

const (
	StrategyFullRebuild            Strategy = "full-rebuild"
	StrategyEventNarrow            Strategy = "event-narrow"
	StrategyStorageNarrow          Strategy = "storage-narrow"
	StrategyServiceNarrow          Strategy = "service-narrow"
	StrategyPodNarrow              Strategy = "pod-narrow"
	StrategyWorkloadNarrow         Strategy = "workload-narrow"
	StrategyIdentitySecurityNarrow Strategy = "identity/security-narrow"
)

type Plan struct {
	Scope    Scope
	Strategy Strategy
}

type NoopPlanner struct{}

func NewNoopPlanner() *NoopPlanner {
	return &NoopPlanner{}
}

func (p *NoopPlanner) Plan(event collectk8s.ChangeEvent) Plan {
	return Plan{
		Scope: Scope{
			Kinds:      []string{event.Kind},
			Namespaces: []string{event.Namespace},
			Names:      []string{event.Name},
		},
		Strategy: StrategyForEvent(event),
	}
}

func StrategyForEvent(event collectk8s.ChangeEvent) Strategy {
	switch normalizeKind(event.Kind) {
	case "event":
		return StrategyEventNarrow
	case "pod":
		return StrategyPodNarrow
	case "workload", "deployment", "statefulset", "daemonset", "job", "replicaset":
		return StrategyWorkloadNarrow
	case "service":
		return StrategyServiceNarrow
	case "storage", "pv", "pvc", "persistentvolume", "persistentvolumeclaim":
		return StrategyStorageNarrow
	case "identity/security", "identity", "security", "serviceaccount", "rolebinding", "clusterrolebinding":
		return StrategyIdentitySecurityNarrow
	default:
		return StrategyFullRebuild
	}
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}
