package explicit

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

func PodScheduledOn(podID, nodeID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, nodeID, model.EdgeKindScheduledOn)
}

func PodUsesImage(podID, imageID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, imageID, model.EdgeKindUsesImage)
}

func PodUsesConfigMap(podID, configMapID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, configMapID, model.EdgeKindUsesConfigMap)
}

func PodUsesSecret(podID, secretID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, secretID, model.EdgeKindUsesSecret)
}

func PodMountsPVC(podID, pvcID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, pvcID, model.EdgeKindMountsPVC)
}

func PVCBoundToPV(pvcID, pvID model.CanonicalID) model.Edge {
	return model.NewEdge(pvcID, pvID, model.EdgeKindBoundToPV)
}

func ResourceUsesStorageClass(resourceID, storageClassID model.CanonicalID, resolver string) model.Edge {
	return model.NewEdgeWithResolver(resourceID, storageClassID, model.EdgeKindUsesStorageClass, resolver)
}

func PodUsesServiceAccount(podID, saID model.CanonicalID) model.Edge {
	return model.NewEdge(podID, saID, model.EdgeKindUsesServiceAccount)
}

func EventReportsOn(eventID, targetID model.CanonicalID) model.Edge {
	return model.NewEdge(eventID, targetID, model.EdgeKindReportedByEvent)
}
