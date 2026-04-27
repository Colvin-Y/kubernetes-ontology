package infer

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

func WorkloadAffectedByWebhook(workloadID, webhookID model.CanonicalID) model.Edge {
	return model.NewEdge(workloadID, webhookID, model.EdgeKindAffectedByWebhook)
}

func SubjectBoundByRoleBinding(subjectID, bindingID model.CanonicalID) model.Edge {
	return model.NewEdge(subjectID, bindingID, model.EdgeKindBoundByRoleBinding)
}

func StorageClassProvisionedByCSIDriver(storageClassID, csiDriverID model.CanonicalID) model.Edge {
	return model.NewEdge(storageClassID, csiDriverID, model.EdgeKindProvisionedByCSIDriver)
}
