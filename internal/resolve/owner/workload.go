package owner

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

func WorkloadOwnsPod(workloadID, podID model.CanonicalID) model.Edge {
	return model.NewEdge(workloadID, podID, model.EdgeKindOwnsPod)
}

func PodManagedByWorkload(podID, workloadID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, workloadID, model.EdgeKindManagedBy)
}

func WorkloadControlledBy(childID, parentID model.CanonicalID) model.Edge {
	return model.NewEdge(childID, parentID, model.EdgeKindControlledBy)
}
