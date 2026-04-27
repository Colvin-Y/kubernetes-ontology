package runtime

import (
	"context"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
	"github.com/Colvin-Y/kubernetes-ontology/internal/reconcile"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type bootstrapResult struct {
	Snapshot  collectk8s.Snapshot
	Builder   *graph.Builder
	Kernel    *graph.Kernel
	Facade    *query.Facade
	NodeCount int
	EdgeCount int
}

func bootstrapRuntime(ctx context.Context, cluster string, collector collectk8s.Collector, controllerRules []infer.WorkloadControllerRule) (bootstrapResult, error) {
	snapshot, err := collector.Collect(ctx)
	if err != nil {
		return bootstrapResult{}, err
	}

	reconciler := reconcile.NewFullReconcilerWithControllerRules(cluster, controllerRules)
	rebuild := reconciler.Rebuild(snapshot)
	_, kernel, _ := newKernel()
	for _, node := range rebuild.Nodes {
		if err := kernel.UpsertNode(node); err != nil {
			return bootstrapResult{}, err
		}
	}
	for _, edge := range rebuild.Edges {
		if err := kernel.UpsertEdge(edge); err != nil {
			return bootstrapResult{}, err
		}
	}

	return bootstrapResult{
		Snapshot:  snapshot,
		Builder:   reconciler.Builder(),
		Kernel:    kernel,
		Facade:    newDiagnosticFacade(cluster, snapshot, reconciler.Builder(), kernel),
		NodeCount: len(rebuild.Nodes),
		EdgeCount: len(rebuild.Edges),
	}, nil
}
