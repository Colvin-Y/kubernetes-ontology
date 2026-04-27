package runtime

import "time"

type Phase string

const (
	PhaseStarting      Phase = "starting"
	PhaseBootstrapping Phase = "bootstrapping"
	PhaseReady         Phase = "ready"
	PhaseDegraded      Phase = "degraded"
)

type Status struct {
	Phase                       Phase
	Cluster                     string
	Ready                       bool
	NodeCount                   int
	EdgeCount                   int
	LastBootstrapAt             *time.Time
	LastAppliedChangeKind       string
	LastAppliedChangeNS         string
	LastAppliedChangeName       string
	LastAppliedChangeType       string
	LastAppliedChangeAt         *time.Time
	LastStrategy                string
	FullRebuildCount            int
	EventNarrowCount            int
	StorageNarrowCount          int
	ServiceNarrowCount          int
	PodNarrowCount              int
	WorkloadNarrowCount         int
	IdentitySecurityNarrowCount int
	LastError                   string
}
