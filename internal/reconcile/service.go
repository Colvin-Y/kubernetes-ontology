package reconcile

import (
	"fmt"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/selector"
)

type ServiceApplyResult struct {
	Applied       bool
	Deleted       bool
	UpsertedEdges int
	DeletedEdges  int
}

type ServiceReconciler struct {
	cluster string
	kernel  *graph.Kernel
}

func NewServiceReconciler(cluster string, kernel *graph.Kernel) *ServiceReconciler {
	return &ServiceReconciler{cluster: cluster, kernel: kernel}
}

func (r *ServiceReconciler) Apply(snapshot collectk8s.Snapshot, namespace, name string, change collectk8s.ChangeType) (ServiceApplyResult, error) {
	if r.kernel == nil {
		return ServiceApplyResult{}, fmt.Errorf("service reconciler requires kernel")
	}
	if namespace == "" || name == "" {
		return ServiceApplyResult{}, fmt.Errorf("service reconcile requires namespace and name")
	}

	service, ok := findService(snapshot, namespace, name)
	if change == collectk8s.ChangeTypeDelete || !ok {
		deletedEdges, deletedNode := r.deleteService(namespace, name)
		return ServiceApplyResult{Applied: deletedNode || deletedEdges > 0, Deleted: true, DeletedEdges: deletedEdges}, nil
	}

	serviceID := serviceID(r.cluster, service)
	node := model.Node{
		ID:         serviceID,
		Kind:       model.NodeKindService,
		SourceKind: "Service",
		Name:       service.Metadata.Name,
		Namespace:  service.Metadata.Namespace,
		Attributes: map[string]any{"selector": service.Selector},
	}
	if err := r.kernel.UpsertNode(node); err != nil {
		return ServiceApplyResult{}, err
	}

	deletedEdges := r.deleteSelectorEdges(serviceID)
	upsertedEdges := 0
	for _, pod := range snapshot.Pods {
		if pod.Metadata.Namespace != service.Metadata.Namespace {
			continue
		}
		if !selector.LabelsMatch(service.Selector, pod.Metadata.Labels) {
			continue
		}
		podID := model.NewCanonicalID(model.ResourceRef{Cluster: r.cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
		if err := r.kernel.UpsertEdge(selector.ServiceSelectsPod(serviceID, podID)); err != nil {
			return ServiceApplyResult{}, err
		}
		upsertedEdges++
	}

	return ServiceApplyResult{Applied: true, UpsertedEdges: upsertedEdges, DeletedEdges: deletedEdges}, nil
}

func (r *ServiceReconciler) deleteService(namespace, name string) (int, bool) {
	deletedEdges := 0
	deletedNode := false
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindService || node.Namespace != namespace || node.Name != name {
			continue
		}
		deletedEdges += r.deleteSelectorEdges(node.ID)
		_ = r.kernel.DeleteNode(node.ID)
		deletedNode = true
	}
	return deletedEdges, deletedNode
}

func (r *ServiceReconciler) deleteSelectorEdges(serviceID model.CanonicalID) int {
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if edge.From != serviceID || edge.Kind != model.EdgeKindSelectsPod {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted
}

func findService(snapshot collectk8s.Snapshot, namespace, name string) (resources.Service, bool) {
	for _, service := range snapshot.Services {
		if service.Metadata.Namespace == namespace && service.Metadata.Name == name {
			return service, true
		}
	}
	return resources.Service{}, false
}

func serviceID(cluster string, service resources.Service) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{
		Cluster:   cluster,
		Group:     "core",
		Kind:      "Service",
		Namespace: service.Metadata.Namespace,
		Name:      service.Metadata.Name,
		UID:       service.Metadata.UID,
	})
}
