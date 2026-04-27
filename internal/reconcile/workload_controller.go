package reconcile

import (
	"fmt"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type WorkloadControllerRuleReconciler struct {
	cluster string
	kernel  *graph.Kernel
	rules   []infer.WorkloadControllerRule
}

func NewWorkloadControllerRuleReconciler(cluster string, kernel *graph.Kernel, rules []infer.WorkloadControllerRule) *WorkloadControllerRuleReconciler {
	return &WorkloadControllerRuleReconciler{cluster: cluster, kernel: kernel, rules: rules}
}

func (r *WorkloadControllerRuleReconciler) Apply(snapshot collectk8s.Snapshot) (int, int, error) {
	if r.kernel == nil {
		return 0, 0, fmt.Errorf("workload controller rule reconciler requires kernel")
	}
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if !isWorkloadControllerRuleEdge(edge.Kind) {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	upserted := 0
	for _, edge := range infer.InferWorkloadControllerEdges(r.cluster, snapshot, r.rules) {
		if err := r.kernel.UpsertEdge(edge); err != nil {
			return upserted, deleted, err
		}
		upserted++
	}
	return upserted, deleted, nil
}

func isWorkloadControllerRuleEdge(kind model.EdgeKind) bool {
	return kind == model.EdgeKindManagedByController || kind == model.EdgeKindServedByNodeDaemon
}
