package reconcile

import (
	"fmt"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type EventApplyResult struct {
	Applied              bool
	Deleted              bool
	UpsertedReportEdges  int
	UpsertedWebhookEdges int
	DeletedEdges         int
}

type EventReconciler struct {
	cluster string
	kernel  *graph.Kernel
}

func NewEventReconciler(cluster string, kernel *graph.Kernel) *EventReconciler {
	return &EventReconciler{cluster: cluster, kernel: kernel}
}

func (r *EventReconciler) Apply(snapshot collectk8s.Snapshot, namespace, name string, change collectk8s.ChangeType) (EventApplyResult, error) {
	if r.kernel == nil {
		return EventApplyResult{}, fmt.Errorf("event reconciler requires kernel")
	}
	if namespace == "" || name == "" {
		return EventApplyResult{}, fmt.Errorf("event reconcile requires namespace and name")
	}

	deletedEventEdges, deletedEventNode := r.deleteEvent(namespace, name)
	deletedWebhookEdges := r.deleteWebhookEvidenceEdges()

	event, ok := findEvent(snapshot, namespace, name)
	if change == collectk8s.ChangeTypeDelete || !ok {
		upsertedWebhookEdges, err := r.rebuildWebhookEvidenceEdges(snapshot)
		if err != nil {
			return EventApplyResult{}, err
		}
		return EventApplyResult{
			Applied:              deletedEventNode || deletedEventEdges > 0 || deletedWebhookEdges > 0 || upsertedWebhookEdges > 0,
			Deleted:              true,
			UpsertedWebhookEdges: upsertedWebhookEdges,
			DeletedEdges:         deletedEventEdges + deletedWebhookEdges,
		}, nil
	}

	eventID := eventID(r.cluster, event)
	if err := r.kernel.UpsertNode(model.Node{
		ID:         eventID,
		Kind:       model.NodeKindEvent,
		SourceKind: "Event",
		Name:       event.Metadata.Name,
		Namespace:  event.Metadata.Namespace,
		Attributes: map[string]any{"reason": event.Reason, "message": event.Message},
	}); err != nil {
		return EventApplyResult{}, err
	}

	upsertedReportEdges := 0
	if targetID, ok := eventTargetIDs(r.cluster, snapshot)[event.InvolvedKind+"/"+event.InvolvedUID]; ok {
		if err := r.kernel.UpsertEdge(explicit.EventReportsOn(eventID, targetID)); err != nil {
			return EventApplyResult{}, err
		}
		upsertedReportEdges++
	}

	upsertedWebhookEdges, err := r.rebuildWebhookEvidenceEdges(snapshot)
	if err != nil {
		return EventApplyResult{}, err
	}

	return EventApplyResult{
		Applied:              true,
		UpsertedReportEdges:  upsertedReportEdges,
		UpsertedWebhookEdges: upsertedWebhookEdges,
		DeletedEdges:         deletedEventEdges + deletedWebhookEdges,
	}, nil
}

func (r *EventReconciler) deleteEvent(namespace, name string) (int, bool) {
	deletedEdges := 0
	deletedNode := false
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindEvent || node.Namespace != namespace || node.Name != name {
			continue
		}
		for _, edge := range r.kernel.ListEdges() {
			if edge.From == node.ID || edge.To == node.ID {
				_ = r.kernel.DeleteEdge(edge.Key())
				deletedEdges++
			}
		}
		_ = r.kernel.DeleteNode(node.ID)
		deletedNode = true
	}
	return deletedEdges, deletedNode
}

func (r *EventReconciler) deleteWebhookEvidenceEdges() int {
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if edge.Kind != model.EdgeKindAffectedByWebhook {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted
}

func (r *EventReconciler) rebuildWebhookEvidenceEdges(snapshot collectk8s.Snapshot) (int, error) {
	workloads := workloadIDs(r.cluster, snapshot)
	webhooks := webhookIDs(r.cluster, snapshot)
	if len(workloads) == 0 || len(webhooks) == 0 {
		return 0, nil
	}

	upserted := 0
	for _, event := range snapshot.Events {
		if event.Reason != "FailedCreate" && event.Reason != "FailedAdmission" {
			continue
		}
		workloadID, ok := workloads[event.Metadata.Namespace+"/"+event.InvolvedName]
		if !ok {
			continue
		}
		for _, webhookID := range webhooks {
			if err := r.kernel.UpsertEdge(infer.WorkloadAffectedByWebhook(workloadID, webhookID)); err != nil {
				return upserted, err
			}
			upserted++
			break
		}
	}
	return upserted, nil
}

func findEvent(snapshot collectk8s.Snapshot, namespace, name string) (resources.Event, bool) {
	for _, event := range snapshot.Events {
		if event.Metadata.Namespace == namespace && event.Metadata.Name == name {
			return event, true
		}
	}
	return resources.Event{}, false
}

func eventID(cluster string, event resources.Event) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{
		Cluster:   cluster,
		Group:     "core",
		Kind:      "Event",
		Namespace: event.Metadata.Namespace,
		Name:      event.Metadata.Name,
		UID:       event.Metadata.UID,
	})
}

func eventTargetIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID)
	for _, workload := range snapshot.Workloads {
		out[workload.ControllerKind+"/"+workload.Metadata.UID] = model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
	}
	for _, pod := range snapshot.Pods {
		out["Pod/"+pod.Metadata.UID] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
	}
	for _, node := range snapshot.Nodes {
		out["Node/"+node.Metadata.UID] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Node", Name: node.Metadata.Name, UID: node.Metadata.UID})
	}
	for _, service := range snapshot.Services {
		out["Service/"+service.Metadata.UID] = serviceID(cluster, service)
	}
	for _, account := range snapshot.ServiceAccounts {
		out["ServiceAccount/"+account.Metadata.UID] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "ServiceAccount", Namespace: account.Metadata.Namespace, Name: account.Metadata.Name, UID: account.Metadata.UID})
	}
	for _, pvc := range snapshot.PVCs {
		out["PersistentVolumeClaim/"+pvc.Metadata.UID] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "PVC", Namespace: pvc.Metadata.Namespace, Name: pvc.Metadata.Name, UID: pvc.Metadata.UID})
	}
	for _, pv := range snapshot.PVs {
		out["PersistentVolume/"+pv.Metadata.UID] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "PV", Name: pv.Metadata.Name, UID: pv.Metadata.UID})
	}
	return out
}

func workloadIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.Workloads))
	for _, workload := range snapshot.Workloads {
		out[workload.Metadata.Namespace+"/"+workload.Metadata.Name] = model.WorkloadID(cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
	}
	return out
}

func webhookIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.WebhookConfigs))
	for _, webhook := range snapshot.WebhookConfigs {
		out[webhook.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "admissionregistration.k8s.io", Kind: "WebhookConfig", Name: webhook.Metadata.Name, UID: webhook.Metadata.UID})
	}
	return out
}
