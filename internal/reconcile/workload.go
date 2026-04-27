package reconcile

import (
	"fmt"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/owner"
)

type WorkloadApplyResult struct {
	Applied       bool
	Deleted       bool
	UpsertedEdges int
	DeletedEdges  int
	DeletedNodes  int
}

type WorkloadReconciler struct {
	cluster string
	kernel  *graph.Kernel
}

func NewWorkloadReconciler(cluster string, kernel *graph.Kernel) *WorkloadReconciler {
	return &WorkloadReconciler{cluster: cluster, kernel: kernel}
}

func (r *WorkloadReconciler) Apply(snapshot collectk8s.Snapshot, namespace, name string, change collectk8s.ChangeType) (WorkloadApplyResult, error) {
	if r.kernel == nil {
		return WorkloadApplyResult{}, fmt.Errorf("workload reconciler requires kernel")
	}
	if namespace == "" || name == "" {
		return WorkloadApplyResult{}, fmt.Errorf("workload reconcile requires namespace and name")
	}

	workloads := findWorkloads(snapshot, namespace, name)
	if change == collectk8s.ChangeTypeDelete || len(workloads) == 0 {
		deletedEdges, deletedNodes := r.deleteWorkloadNodes(namespace, name, nil)
		return WorkloadApplyResult{Applied: deletedNodes > 0 || deletedEdges > 0, Deleted: true, DeletedEdges: deletedEdges, DeletedNodes: deletedNodes}, nil
	}

	targetIDs := workloadTargetIDs(r.cluster, workloads)
	deletedEdges, deletedNodes := r.deleteWorkloadNodes(namespace, name, targetIDs)
	for _, workload := range workloads {
		id := model.WorkloadID(r.cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
		deletedEdges += r.deleteWorkloadScopedEdges(id)
		if err := r.kernel.UpsertNode(workloadNode(r.cluster, workload)); err != nil {
			return WorkloadApplyResult{}, err
		}
	}

	upsertedEdges, err := r.rebuildWorkloadEdges(snapshot, targetIDs)
	if err != nil {
		return WorkloadApplyResult{}, err
	}

	return WorkloadApplyResult{
		Applied:       true,
		UpsertedEdges: upsertedEdges,
		DeletedEdges:  deletedEdges,
		DeletedNodes:  deletedNodes,
	}, nil
}

func (r *WorkloadReconciler) rebuildWorkloadEdges(snapshot collectk8s.Snapshot, targetIDs map[model.CanonicalID]struct{}) (int, error) {
	upserted := 0
	resolver := owner.NewChainResolver(r.cluster, snapshot.Workloads, snapshot.ReplicaSets)
	workloadIDSet := workloadIDSet(r.cluster, snapshot.Workloads)
	for _, pod := range snapshot.Pods {
		podID := podID(r.cluster, pod)
		targets := resolver.ResolvePodWorkloads(pod)
		if !containsWorkloadTarget(targets, targetIDs) {
			continue
		}
		for _, target := range targets {
			if err := r.kernel.UpsertEdge(owner.PodManagedByWorkload(podID, target.ID)); err != nil {
				return upserted, err
			}
			if err := r.kernel.UpsertEdge(owner.WorkloadOwnsPod(target.ID, podID)); err != nil {
				return upserted, err
			}
			upserted += 2
		}
	}
	for _, workload := range snapshot.Workloads {
		workloadID := model.WorkloadID(r.cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
		for _, target := range resolver.ResolveWorkloadOwners(workload) {
			if target.ID == workloadID {
				continue
			}
			_, childInScope := targetIDs[workloadID]
			_, parentInScope := targetIDs[target.ID]
			if !childInScope && !parentInScope {
				continue
			}
			if _, ok := workloadIDSet[target.ID]; !ok {
				continue
			}
			if err := r.kernel.UpsertEdge(owner.WorkloadControlledBy(workloadID, target.ID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}

	targets := workloadTargets(r.cluster, snapshot.Workloads, targetIDs)
	for _, event := range snapshot.Events {
		eventNodeID := eventID(r.cluster, event)
		if _, ok := r.kernel.GetNode(eventNodeID); !ok {
			continue
		}
		for _, target := range targets {
			if event.InvolvedKind == target.Kind && event.InvolvedUID == target.UID {
				if err := r.kernel.UpsertEdge(explicit.EventReportsOn(eventNodeID, target.ID)); err != nil {
					return upserted, err
				}
				upserted++
			}
		}
	}

	webhooks := webhookIDs(r.cluster, snapshot)
	for _, event := range snapshot.Events {
		if event.Reason != "FailedCreate" && event.Reason != "FailedAdmission" {
			continue
		}
		for _, target := range targets {
			if event.Metadata.Namespace != target.Namespace || event.InvolvedName != target.Name {
				continue
			}
			for _, webhookID := range webhooks {
				if _, ok := r.kernel.GetNode(webhookID); !ok {
					continue
				}
				if err := r.kernel.UpsertEdge(infer.WorkloadAffectedByWebhook(target.ID, webhookID)); err != nil {
					return upserted, err
				}
				upserted++
				break
			}
		}
	}

	return upserted, nil
}

func containsWorkloadTarget(targets []owner.WorkloadTarget, ids map[model.CanonicalID]struct{}) bool {
	for _, target := range targets {
		if _, ok := ids[target.ID]; ok {
			return true
		}
	}
	return false
}

func (r *WorkloadReconciler) deleteWorkloadNodes(namespace, name string, keep map[model.CanonicalID]struct{}) (int, int) {
	deletedEdges := 0
	deletedNodes := 0
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindWorkload || node.Namespace != namespace || node.Name != name {
			continue
		}
		if _, ok := keep[node.ID]; ok {
			continue
		}
		deletedEdges += incidentEdgeCount(r.kernel, node.ID)
		_ = r.kernel.DeleteNode(node.ID)
		deletedNodes++
	}
	return deletedEdges, deletedNodes
}

func (r *WorkloadReconciler) deleteWorkloadScopedEdges(workloadID model.CanonicalID) int {
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if !isWorkloadScopedEdge(edge, workloadID) {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted
}

func isWorkloadScopedEdge(edge model.Edge, workloadID model.CanonicalID) bool {
	switch edge.Kind {
	case model.EdgeKindOwnsPod, model.EdgeKindAffectedByWebhook:
		return edge.From == workloadID
	case model.EdgeKindManagedBy, model.EdgeKindReportedByEvent:
		return edge.To == workloadID
	case model.EdgeKindControlledBy:
		return edge.From == workloadID || edge.To == workloadID
	default:
		return false
	}
}

func incidentEdgeCount(kernel *graph.Kernel, id model.CanonicalID) int {
	count := 0
	for _, edge := range kernel.ListEdges() {
		if edge.From == id || edge.To == id {
			count++
		}
	}
	return count
}

func findWorkloads(snapshot collectk8s.Snapshot, namespace, name string) []resources.Workload {
	out := make([]resources.Workload, 0)
	for _, workload := range snapshot.Workloads {
		if workload.Metadata.Namespace == namespace && workload.Metadata.Name == name {
			out = append(out, workload)
		}
	}
	return out
}

func workloadNode(cluster string, workload resources.Workload) model.Node {
	w := model.NewWorkload(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID, map[string]any{"apiVersion": workload.APIVersion, "replicas": workload.Replicas, "conditions": workload.Conditions})
	return model.Node{
		ID:         w.ID,
		Kind:       model.NodeKindWorkload,
		SourceKind: workload.ControllerKind,
		Name:       workload.Metadata.Name,
		Namespace:  workload.Metadata.Namespace,
		Attributes: w.Attributes,
	}
}

func workloadTargetIDs(cluster string, workloads []resources.Workload) map[model.CanonicalID]struct{} {
	out := make(map[model.CanonicalID]struct{}, len(workloads))
	for _, workload := range workloads {
		out[model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)] = struct{}{}
	}
	return out
}

func workloadIDSet(cluster string, workloads []resources.Workload) map[model.CanonicalID]struct{} {
	out := make(map[model.CanonicalID]struct{}, len(workloads))
	for _, workload := range workloads {
		out[model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)] = struct{}{}
	}
	return out
}

func workloadTargets(cluster string, workloads []resources.Workload, ids map[model.CanonicalID]struct{}) []owner.WorkloadTarget {
	out := make([]owner.WorkloadTarget, 0, len(ids))
	for _, workload := range workloads {
		id := model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
		if _, ok := ids[id]; !ok {
			continue
		}
		out = append(out, owner.WorkloadTarget{
			ID:         id,
			APIVersion: workload.APIVersion,
			Kind:       workload.ControllerKind,
			Namespace:  workload.Metadata.Namespace,
			Name:       workload.Metadata.Name,
			UID:        workload.Metadata.UID,
		})
	}
	return out
}
