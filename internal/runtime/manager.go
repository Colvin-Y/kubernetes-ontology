package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
	"github.com/Colvin-Y/kubernetes-ontology/internal/reconcile"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
	"github.com/Colvin-Y/kubernetes-ontology/internal/service/diagnostic"
	memorystore "github.com/Colvin-Y/kubernetes-ontology/internal/store/memory"
)

type ManagerOptions struct {
	WorkloadControllerRules []infer.WorkloadControllerRule
}

type Manager struct {
	mu                      sync.RWMutex
	collector               collectk8s.Collector
	cluster                 string
	status                  Status
	facade                  *query.Facade
	snapshot                collectk8s.Snapshot
	builder                 *graph.Builder
	kernel                  *graph.Kernel
	planner                 reconcile.Planner
	workloadControllerRules []infer.WorkloadControllerRule
}

func NewManager(cluster string, collector collectk8s.Collector) *Manager {
	return NewManagerWithOptions(cluster, collector, ManagerOptions{})
}

func NewManagerWithOptions(cluster string, collector collectk8s.Collector, options ManagerOptions) *Manager {
	return &Manager{
		collector:               collector,
		cluster:                 cluster,
		status:                  Status{Phase: PhaseStarting, Cluster: cluster},
		planner:                 reconcile.NewNoopPlanner(),
		workloadControllerRules: options.WorkloadControllerRules,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.status = Status{Phase: PhaseBootstrapping, Cluster: m.cluster}
	m.mu.Unlock()
	bootstrap, err := bootstrapRuntime(ctx, m.cluster, m.collector, m.workloadControllerRules)
	if err != nil {
		m.mu.Lock()
		m.markDegradedLocked(err)
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.applyBootstrapResult(bootstrap)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Apply(ctx context.Context, event collectk8s.ChangeEvent) error {
	plan := m.planner.Plan(event)
	if plan.Strategy == reconcile.StrategyServiceNarrow {
		if err := m.applyServiceChange(ctx, event, plan); err == nil {
			return nil
		}
	}
	if plan.Strategy == reconcile.StrategyEventNarrow {
		if err := m.applyEventChange(ctx, event, plan); err == nil {
			return nil
		}
	}
	if plan.Strategy == reconcile.StrategyStorageNarrow {
		if err := m.applyStorageChange(ctx, event, plan); err == nil {
			return nil
		}
	}
	if plan.Strategy == reconcile.StrategyIdentitySecurityNarrow {
		if err := m.applyIdentitySecurityChange(ctx, event, plan); err == nil {
			return nil
		}
	}
	if plan.Strategy == reconcile.StrategyPodNarrow {
		if err := m.applyPodChange(ctx, event, plan); err == nil {
			return nil
		}
	}
	if plan.Strategy == reconcile.StrategyWorkloadNarrow {
		if err := m.applyWorkloadChange(ctx, event, plan); err == nil {
			return nil
		}
	}

	bootstrap, err := bootstrapRuntime(ctx, m.cluster, m.collector, m.workloadControllerRules)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.markDegradedLocked(err)
		m.setFacadeRuntimeStatusLocked()
		return err
	}
	m.applyBootstrapResult(bootstrap)
	now := time.Now().UTC()
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(reconcile.StrategyFullRebuild)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyServiceChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("service scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewServiceReconciler(m.cluster, m.kernel).Apply(snapshot, event.Namespace, event.Name, event.Change); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyEventChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("event scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewEventReconciler(m.cluster, m.kernel).Apply(snapshot, event.Namespace, event.Name, event.Change); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyStorageChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("storage scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewStorageReconciler(m.cluster, m.kernel).Apply(snapshot); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyIdentitySecurityChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("identity/security scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewIdentitySecurityReconciler(m.cluster, m.kernel).Apply(snapshot); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyPodChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("pod scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewPodReconciler(m.cluster, m.kernel).Apply(snapshot, event.Namespace, event.Name, event.Change); err != nil {
		return err
	}
	if _, _, err := reconcile.NewWorkloadControllerRuleReconciler(m.cluster, m.kernel, m.workloadControllerRules).Apply(snapshot); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) applyWorkloadChange(ctx context.Context, event collectk8s.ChangeEvent, plan reconcile.Plan) error {
	snapshot, err := m.collectSnapshot(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kernel == nil || m.facade == nil {
		return fmt.Errorf("workload scoped apply requires initialized runtime")
	}
	if _, err := reconcile.NewWorkloadReconciler(m.cluster, m.kernel).Apply(snapshot, event.Namespace, event.Name, event.Change); err != nil {
		return err
	}
	if _, _, err := reconcile.NewWorkloadControllerRuleReconciler(m.cluster, m.kernel, m.workloadControllerRules).Apply(snapshot); err != nil {
		return err
	}

	now := time.Now().UTC()
	m.snapshot = snapshot
	m.facade.SetSnapshot(snapshot)
	m.status.Phase = PhaseReady
	m.status.Ready = true
	m.status.NodeCount = len(m.kernel.ListNodes())
	m.status.EdgeCount = len(m.kernel.ListEdges())
	m.status.LastAppliedChangeKind = event.Kind
	m.status.LastAppliedChangeNS = event.Namespace
	m.status.LastAppliedChangeName = event.Name
	m.status.LastAppliedChangeType = string(event.Change)
	m.status.LastAppliedChangeAt = &now
	m.status.LastStrategy = string(plan.Strategy)
	m.status.LastError = ""
	m.incrementStrategy(plan.Strategy)
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	return nil
}

func (m *Manager) RunStream(ctx context.Context, stream collectk8s.Stream) error {
	return stream.Run(ctx, m)
}

func (m *Manager) collectSnapshot(ctx context.Context) (collectk8s.Snapshot, error) {
	snapshot, err := m.collector.Collect(ctx)
	if err == nil {
		return snapshot, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markDegradedLocked(err)
	m.setFacadeRuntimeStatusLocked()
	return collectk8s.Snapshot{}, err
}

func (m *Manager) applyBootstrapResult(bootstrap bootstrapResult) {
	now := time.Now().UTC()
	previous := m.status
	m.facade = bootstrap.Facade
	m.snapshot = bootstrap.Snapshot
	m.builder = bootstrap.Builder
	m.kernel = bootstrap.Kernel
	m.status = Status{
		Phase:                       PhaseReady,
		Cluster:                     m.cluster,
		Ready:                       true,
		NodeCount:                   bootstrap.NodeCount,
		EdgeCount:                   bootstrap.EdgeCount,
		LastBootstrapAt:             &now,
		LastAppliedChangeKind:       previous.LastAppliedChangeKind,
		LastAppliedChangeNS:         previous.LastAppliedChangeNS,
		LastAppliedChangeName:       previous.LastAppliedChangeName,
		LastAppliedChangeType:       previous.LastAppliedChangeType,
		LastAppliedChangeAt:         previous.LastAppliedChangeAt,
		LastStrategy:                previous.LastStrategy,
		FullRebuildCount:            previous.FullRebuildCount + 1,
		EventNarrowCount:            previous.EventNarrowCount,
		StorageNarrowCount:          previous.StorageNarrowCount,
		ServiceNarrowCount:          previous.ServiceNarrowCount,
		PodNarrowCount:              previous.PodNarrowCount,
		WorkloadNarrowCount:         previous.WorkloadNarrowCount,
		IdentitySecurityNarrowCount: previous.IdentitySecurityNarrowCount,
	}
	m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
}

func (m *Manager) markDegradedLocked(err error) {
	previous := m.status
	m.status = Status{
		Phase:                       PhaseDegraded,
		Cluster:                     m.cluster,
		Ready:                       previous.Ready,
		NodeCount:                   previous.NodeCount,
		EdgeCount:                   previous.EdgeCount,
		LastBootstrapAt:             previous.LastBootstrapAt,
		LastAppliedChangeKind:       previous.LastAppliedChangeKind,
		LastAppliedChangeNS:         previous.LastAppliedChangeNS,
		LastAppliedChangeName:       previous.LastAppliedChangeName,
		LastAppliedChangeType:       previous.LastAppliedChangeType,
		LastAppliedChangeAt:         previous.LastAppliedChangeAt,
		LastStrategy:                previous.LastStrategy,
		FullRebuildCount:            previous.FullRebuildCount,
		EventNarrowCount:            previous.EventNarrowCount,
		StorageNarrowCount:          previous.StorageNarrowCount,
		ServiceNarrowCount:          previous.ServiceNarrowCount,
		PodNarrowCount:              previous.PodNarrowCount,
		WorkloadNarrowCount:         previous.WorkloadNarrowCount,
		IdentitySecurityNarrowCount: previous.IdentitySecurityNarrowCount,
		LastError:                   err.Error(),
	}
}

func (m *Manager) setFacadeRuntimeStatusLocked() {
	if m.facade != nil {
		m.facade.SetRuntimeStatus(m.runtimeStatusLocked())
	}
}

func (m *Manager) incrementStrategy(strategy reconcile.Strategy) {
	switch strategy {
	case reconcile.StrategyEventNarrow:
		m.status.EventNarrowCount++
	case reconcile.StrategyStorageNarrow:
		m.status.StorageNarrowCount++
	case reconcile.StrategyServiceNarrow:
		m.status.ServiceNarrowCount++
	case reconcile.StrategyPodNarrow:
		m.status.PodNarrowCount++
	case reconcile.StrategyWorkloadNarrow:
		m.status.WorkloadNarrowCount++
	case reconcile.StrategyIdentitySecurityNarrow:
		m.status.IdentitySecurityNarrowCount++
	}
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) RuntimeStatus() query.RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimeStatusLocked()
}

func (m *Manager) runtimeStatusLocked() query.RuntimeStatus {
	status := query.RuntimeStatus{
		Phase:                       string(m.status.Phase),
		Cluster:                     m.status.Cluster,
		Ready:                       m.status.Ready,
		NodeCount:                   m.status.NodeCount,
		EdgeCount:                   m.status.EdgeCount,
		LastAppliedChangeKind:       m.status.LastAppliedChangeKind,
		LastAppliedChangeNS:         m.status.LastAppliedChangeNS,
		LastAppliedChangeName:       m.status.LastAppliedChangeName,
		LastAppliedChangeType:       m.status.LastAppliedChangeType,
		LastStrategy:                m.status.LastStrategy,
		FullRebuildCount:            m.status.FullRebuildCount,
		EventNarrowCount:            m.status.EventNarrowCount,
		StorageNarrowCount:          m.status.StorageNarrowCount,
		ServiceNarrowCount:          m.status.ServiceNarrowCount,
		PodNarrowCount:              m.status.PodNarrowCount,
		WorkloadNarrowCount:         m.status.WorkloadNarrowCount,
		IdentitySecurityNarrowCount: m.status.IdentitySecurityNarrowCount,
		LastError:                   m.status.LastError,
	}
	if m.status.LastBootstrapAt != nil {
		status.LastBootstrapAt = m.status.LastBootstrapAt.UTC().Format(time.RFC3339)
	}
	if m.status.LastAppliedChangeAt != nil {
		status.LastAppliedChangeAt = m.status.LastAppliedChangeAt.UTC().Format(time.RFC3339)
	}
	return status
}

func (m *Manager) Facade() *query.Facade {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.facade
}

func (m *Manager) QueryDiagnosticSubgraph(ctx context.Context, entryKind, namespace, name string, options query.DiagnosticOptions) (api.DiagnosticSubgraph, error) {
	if err := m.rLockContext(ctx); err != nil {
		return api.DiagnosticSubgraph{}, err
	}
	defer m.mu.RUnlock()
	if m.facade == nil {
		return api.DiagnosticSubgraph{}, query.ErrDiagnosticNotReady
	}
	return m.facade.QueryDiagnosticSubgraphContext(ctx, entryKind, namespace, name, options)
}

func (m *Manager) rLockContext(ctx context.Context) error {
	for {
		if m.mu.TryRLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *Manager) Ontology() ontology.Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.kernel == nil {
		return nil
	}
	return ontology.NewKernelBackend(m.kernel)
}

func (m *Manager) Snapshot() collectk8s.Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func newDiagnosticFacade(cluster string, snapshot collectk8s.Snapshot, builder *graph.Builder, kernel *graph.Kernel) *query.Facade {
	return query.NewFacade(cluster, snapshot, builder, diagnostic.NewService(kernel))
}

func newKernel() (*memorystore.Store, *graph.Kernel, *graph.ReverseIndex) {
	store := memorystore.NewStore()
	kernel := graph.NewKernel(store, store)
	return store, kernel, graph.NewReverseIndex()
}
