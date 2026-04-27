package reconcile

import (
	"fmt"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/enrich/oci"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/owner"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/selector"
)

type PodApplyResult struct {
	Applied       bool
	Deleted       bool
	UpsertedEdges int
	DeletedEdges  int
	DeletedImages int
}

type PodReconciler struct {
	cluster string
	kernel  *graph.Kernel
}

func NewPodReconciler(cluster string, kernel *graph.Kernel) *PodReconciler {
	return &PodReconciler{cluster: cluster, kernel: kernel}
}

func (r *PodReconciler) Apply(snapshot k8s.Snapshot, namespace, name string, change k8s.ChangeType) (PodApplyResult, error) {
	if r.kernel == nil {
		return PodApplyResult{}, fmt.Errorf("pod reconciler requires kernel")
	}
	if namespace == "" || name == "" {
		return PodApplyResult{}, fmt.Errorf("pod reconcile requires namespace and name")
	}

	pod, ok := findPod(snapshot, namespace, name)
	if change == k8s.ChangeTypeDelete || !ok {
		deletedEdges, imageIDs := r.deletePods(namespace, name, "")
		return PodApplyResult{Applied: deletedEdges > 0, Deleted: true, DeletedEdges: deletedEdges, DeletedImages: r.pruneImages(imageIDs)}, nil
	}

	podID := podID(r.cluster, pod)
	deletedEdges, imageIDs := r.deletePods(namespace, name, podID)
	if err := r.kernel.UpsertNode(podNode(r.cluster, pod)); err != nil {
		return PodApplyResult{}, err
	}
	upsertedEdges, err := r.rebuildPodEdges(snapshot, pod, podID)
	if err != nil {
		return PodApplyResult{}, err
	}
	return PodApplyResult{
		Applied:       true,
		UpsertedEdges: upsertedEdges,
		DeletedEdges:  deletedEdges,
		DeletedImages: r.pruneImages(imageIDs),
	}, nil
}

func (r *PodReconciler) deletePods(namespace, name string, keep model.CanonicalID) (int, []model.CanonicalID) {
	deletedEdges := 0
	imageIDs := make([]model.CanonicalID, 0)
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindPod || node.Namespace != namespace || node.Name != name || node.ID == keep {
			continue
		}
		for _, edge := range r.kernel.ListEdges() {
			if edge.Kind == model.EdgeKindUsesImage && edge.From == node.ID {
				imageIDs = append(imageIDs, edge.To)
			}
			if edge.From == node.ID || edge.To == node.ID {
				_ = r.kernel.DeleteEdge(edge.Key())
				deletedEdges++
			}
		}
		_ = r.kernel.DeleteNode(node.ID)
	}
	if keep != "" {
		extraDeleted, extraImages := r.deletePodScopedEdges(keep)
		deletedEdges += extraDeleted
		imageIDs = append(imageIDs, extraImages...)
	}
	return deletedEdges, imageIDs
}

func (r *PodReconciler) deletePodScopedEdges(podID model.CanonicalID) (int, []model.CanonicalID) {
	deleted := 0
	imageIDs := make([]model.CanonicalID, 0)
	for _, edge := range r.kernel.ListEdges() {
		if !isPodScopedEdge(edge, podID) {
			continue
		}
		if edge.Kind == model.EdgeKindUsesImage && edge.From == podID {
			imageIDs = append(imageIDs, edge.To)
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted, imageIDs
}

func (r *PodReconciler) rebuildPodEdges(snapshot k8s.Snapshot, pod resources.Pod, id model.CanonicalID) (int, error) {
	upserted := 0
	nodes := nodeIDs(r.cluster, snapshot)
	configMaps := configMapIDs(r.cluster, snapshot)
	secrets := secretIDs(r.cluster, snapshot)
	serviceAccounts := serviceAccountIDs(r.cluster, snapshot)
	pvcs := pvcIDs(r.cluster, snapshot)
	ownerResolver := owner.NewChainResolver(r.cluster, snapshot.Workloads, snapshot.ReplicaSets)

	if pod.NodeName != "" {
		if nodeID, ok := nodes[pod.NodeName]; ok {
			if err := r.kernel.UpsertEdge(explicit.PodScheduledOn(id, nodeID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	for idx, image := range pod.ContainerImages {
		imageRef := oci.ParseImageRef(image)
		imageID := model.NewCanonicalID(model.ResourceRef{Cluster: r.cluster, Group: "core", Kind: "Image", Name: imageRef.Original})
		if err := r.kernel.UpsertNode(model.Node{ID: imageID, Kind: model.NodeKindImage, SourceKind: "Image", Name: imageRef.Original, Attributes: map[string]any{"repo": imageRef.Repo, "tag": imageRef.Tag, "digest": imageRef.Digest, "containerIndex": idx}}); err != nil {
			return upserted, err
		}
		if err := r.kernel.UpsertEdge(explicit.PodUsesImage(id, imageID)); err != nil {
			return upserted, err
		}
		upserted++
	}
	for _, ref := range pod.ConfigMapRefs {
		if configMapID, ok := configMaps[pod.Metadata.Namespace+"/"+ref]; ok {
			if err := r.kernel.UpsertEdge(explicit.PodUsesConfigMap(id, configMapID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	for _, ref := range pod.SecretRefs {
		if secretID, ok := secrets[pod.Metadata.Namespace+"/"+ref]; ok {
			if err := r.kernel.UpsertEdge(explicit.PodUsesSecret(id, secretID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	for _, ref := range pod.PVCRefs {
		if pvcID, ok := pvcs[pod.Metadata.Namespace+"/"+ref]; ok {
			if err := r.kernel.UpsertEdge(explicit.PodMountsPVC(id, pvcID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	if pod.ServiceAccount != "" {
		if accountID, ok := serviceAccounts[pod.Metadata.Namespace+"/"+pod.ServiceAccount]; ok {
			if err := r.kernel.UpsertEdge(explicit.PodUsesServiceAccount(id, accountID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	for _, target := range ownerResolver.ResolvePodWorkloads(pod) {
		if err := r.upsertOwnerEdges(id, target.ID); err != nil {
			return upserted, err
		}
		upserted += 2
	}
	for _, service := range snapshot.Services {
		if service.Metadata.Namespace != pod.Metadata.Namespace || !selector.LabelsMatch(service.Selector, pod.Metadata.Labels) {
			continue
		}
		if err := r.kernel.UpsertEdge(selector.ServiceSelectsPod(serviceID(r.cluster, service), id)); err != nil {
			return upserted, err
		}
		upserted++
	}
	return upserted, nil
}

func (r *PodReconciler) upsertOwnerEdges(podID, workloadID model.CanonicalID) error {
	if err := r.kernel.UpsertEdge(owner.PodManagedByWorkload(podID, workloadID)); err != nil {
		return err
	}
	return r.kernel.UpsertEdge(owner.WorkloadOwnsPod(workloadID, podID))
}

func (r *PodReconciler) pruneImages(imageIDs []model.CanonicalID) int {
	deleted := 0
	for _, imageID := range imageIDs {
		if r.nodeReferenced(imageID) {
			continue
		}
		node, ok := r.kernel.GetNode(imageID)
		if !ok || node.Kind != model.NodeKindImage {
			continue
		}
		_ = r.kernel.DeleteNode(imageID)
		deleted++
	}
	return deleted
}

func (r *PodReconciler) nodeReferenced(id model.CanonicalID) bool {
	for _, edge := range r.kernel.ListEdges() {
		if edge.From == id || edge.To == id {
			return true
		}
	}
	return false
}

func isPodScopedEdge(edge model.Edge, podID model.CanonicalID) bool {
	switch edge.Kind {
	case model.EdgeKindScheduledOn, model.EdgeKindUsesConfigMap, model.EdgeKindUsesSecret, model.EdgeKindUsesServiceAccount, model.EdgeKindMountsPVC, model.EdgeKindUsesImage, model.EdgeKindManagedBy:
		return edge.From == podID
	case model.EdgeKindOwnsPod, model.EdgeKindSelectsPod:
		return edge.To == podID
	default:
		return false
	}
}

func findPod(snapshot k8s.Snapshot, namespace, name string) (resources.Pod, bool) {
	for _, pod := range snapshot.Pods {
		if pod.Metadata.Namespace == namespace && pod.Metadata.Name == name {
			return pod, true
		}
	}
	return resources.Pod{}, false
}

func podID(cluster string, pod resources.Pod) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
}

func podNode(cluster string, pod resources.Pod) model.Node {
	return model.Node{
		ID:         podID(cluster, pod),
		Kind:       model.NodeKindPod,
		SourceKind: "Pod",
		Name:       pod.Metadata.Name,
		Namespace:  pod.Metadata.Namespace,
		Attributes: map[string]any{"phase": pod.Phase, "reason": pod.Reason, "nodeName": pod.NodeName},
	}
}

func nodeIDs(cluster string, snapshot k8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		out[node.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Node", Name: node.Metadata.Name, UID: node.Metadata.UID})
	}
	return out
}

func configMapIDs(cluster string, snapshot k8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.ConfigMaps))
	for _, item := range snapshot.ConfigMaps {
		out[item.Metadata.Namespace+"/"+item.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "ConfigMap", Namespace: item.Metadata.Namespace, Name: item.Metadata.Name, UID: item.Metadata.UID})
	}
	return out
}

func secretIDs(cluster string, snapshot k8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.Secrets))
	for _, item := range snapshot.Secrets {
		out[item.Metadata.Namespace+"/"+item.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Secret", Namespace: item.Metadata.Namespace, Name: item.Metadata.Name, UID: item.Metadata.UID})
	}
	return out
}

func serviceAccountIDs(cluster string, snapshot k8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.ServiceAccounts))
	for _, item := range snapshot.ServiceAccounts {
		out[item.Metadata.Namespace+"/"+item.Metadata.Name] = model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "ServiceAccount", Namespace: item.Metadata.Namespace, Name: item.Metadata.Name, UID: item.Metadata.UID})
	}
	return out
}
