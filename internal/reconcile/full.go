package reconcile

import (
	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type FullRebuildResult struct {
	Nodes    []model.Node
	Edges    []model.Edge
	Evidence []string
}

type FullReconciler struct {
	cluster string
	builder *graph.Builder
}

func NewFullReconciler(cluster string) *FullReconciler {
	return &FullReconciler{cluster: cluster, builder: graph.NewBuilder(cluster)}
}

func NewFullReconcilerWithControllerRules(cluster string, rules []infer.WorkloadControllerRule) *FullReconciler {
	builder := graph.NewBuilder(cluster)
	builder.SetWorkloadControllerRules(rules)
	return &FullReconciler{cluster: cluster, builder: builder}
}

func (r *FullReconciler) Rebuild(snapshot collectk8s.Snapshot) FullRebuildResult {
	nodes, edges := r.builder.Build(snapshot)
	return FullRebuildResult{
		Nodes:    nodes,
		Edges:    edges,
		Evidence: r.builder.Evidence(),
	}
}

func (r *FullReconciler) Build(snapshot collectk8s.Snapshot) ([]model.Node, []model.Edge, []string) {
	result := r.Rebuild(snapshot)
	return result.Nodes, result.Edges, result.Evidence
}

func (r *FullReconciler) Builder() *graph.Builder {
	return r.builder
}
