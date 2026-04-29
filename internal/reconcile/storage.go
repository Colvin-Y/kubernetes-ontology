package reconcile

import (
	"fmt"
	"sort"
	"strings"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type StorageApplyResult struct {
	Applied                bool
	UpsertedPVCs           int
	UpsertedPVs            int
	UpsertedStorageClasses int
	UpsertedCSIDrivers     int
	UpsertedEdges          int
	DeletedEdges           int
	DeletedNodes           int
}

type StorageReconciler struct {
	cluster string
	kernel  *graph.Kernel
	csi     *infer.Registry
	rules   []infer.CSIComponentRule
}

func NewStorageReconciler(cluster string, kernel *graph.Kernel) *StorageReconciler {
	return NewStorageReconcilerWithCSIComponentRules(cluster, kernel, nil)
}

func NewStorageReconcilerWithCSIComponentRules(cluster string, kernel *graph.Kernel, rules []infer.CSIComponentRule) *StorageReconciler {
	effectiveRules := infer.EffectiveCSIComponentRules(rules)
	return &StorageReconciler{
		cluster: cluster,
		kernel:  kernel,
		csi:     infer.NewCSIComponentRegistry(effectiveRules),
		rules:   effectiveRules,
	}
}

func (r *StorageReconciler) Apply(snapshot collectk8s.Snapshot) (StorageApplyResult, error) {
	if r.kernel == nil {
		return StorageApplyResult{}, fmt.Errorf("storage reconciler requires kernel")
	}

	currentPVCs := pvcIDs(r.cluster, snapshot)
	currentPVs := pvIDs(r.cluster, snapshot)
	currentStorageClasses := storageClassIDs(r.cluster, snapshot)
	currentCSIDrivers := csiDriverIDs(r.cluster, snapshot, r.rules)
	result := StorageApplyResult{Applied: true}
	result.DeletedEdges = r.deleteStorageEdges()
	result.DeletedNodes = r.deleteStaleStorageNodes(currentPVCs, currentPVs, currentStorageClasses, currentCSIDrivers)

	for _, pvc := range snapshot.PVCs {
		if err := r.kernel.UpsertNode(pvcNode(r.cluster, pvc)); err != nil {
			return result, err
		}
		result.UpsertedPVCs++
	}
	for _, pv := range snapshot.PVs {
		if err := r.kernel.UpsertNode(pvNode(r.cluster, pv)); err != nil {
			return result, err
		}
		result.UpsertedPVs++
	}
	for _, storageClass := range snapshot.StorageClasses {
		if err := r.kernel.UpsertNode(storageClassNode(r.cluster, storageClass)); err != nil {
			return result, err
		}
		result.UpsertedStorageClasses++
	}
	for _, csiDriver := range snapshot.CSIDrivers {
		if err := r.kernel.UpsertNode(csiDriverNode(r.cluster, csiDriver)); err != nil {
			return result, err
		}
		result.UpsertedCSIDrivers++
	}
	for _, storageClass := range snapshot.StorageClasses {
		if storageClass.Provisioner == "" {
			continue
		}
		if _, observed := findCSIDriver(snapshot.CSIDrivers, storageClass.Provisioner); observed {
			continue
		}
		if !infer.IsCSIProvisioner(storageClass.Provisioner, false, r.rules) {
			continue
		}
		if err := r.kernel.UpsertNode(syntheticCSIDriverNode(r.cluster, storageClass.Provisioner)); err != nil {
			return result, err
		}
		result.UpsertedCSIDrivers++
	}

	upsertedEdges, err := r.rebuildStorageEdges(snapshot, currentPVCs, currentPVs, currentStorageClasses, currentCSIDrivers)
	if err != nil {
		return result, err
	}
	result.UpsertedEdges = upsertedEdges
	return result, nil
}

func (r *StorageReconciler) deleteStorageEdges() int {
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if !isStorageEdge(edge.Kind) {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted
}

func (r *StorageReconciler) deleteStaleStorageNodes(currentPVCs, currentPVs, currentStorageClasses, currentCSIDrivers map[string]model.CanonicalID) int {
	current := make(map[model.CanonicalID]struct{}, len(currentPVCs)+len(currentPVs)+len(currentStorageClasses)+len(currentCSIDrivers))
	for _, id := range currentPVCs {
		current[id] = struct{}{}
	}
	for _, id := range currentPVs {
		current[id] = struct{}{}
	}
	for _, id := range currentStorageClasses {
		current[id] = struct{}{}
	}
	for _, id := range currentCSIDrivers {
		current[id] = struct{}{}
	}

	deleted := 0
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindPVC && node.Kind != model.NodeKindPV && node.Kind != model.NodeKindStorageClass && node.Kind != model.NodeKindCSIDriver {
			continue
		}
		if _, ok := current[node.ID]; ok {
			continue
		}
		_ = r.kernel.DeleteNode(node.ID)
		deleted++
	}
	return deleted
}

func (r *StorageReconciler) rebuildStorageEdges(snapshot collectk8s.Snapshot, currentPVCs, currentPVs, currentStorageClasses, currentCSIDrivers map[string]model.CanonicalID) (int, error) {
	upserted := 0
	for _, pod := range snapshot.Pods {
		podID := model.NewCanonicalID(model.ResourceRef{Cluster: r.cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
		for _, ref := range pod.PVCRefs {
			pvcID, ok := currentPVCs[pod.Metadata.Namespace+"/"+ref]
			if !ok {
				continue
			}
			if err := r.kernel.UpsertEdge(explicit.PodMountsPVC(podID, pvcID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}

	for _, pvc := range snapshot.PVCs {
		pvcID := currentPVCs[pvc.Metadata.Namespace+"/"+pvc.Metadata.Name]
		if pvc.VolumeName != "" {
			if pvID, ok := currentPVs[pvc.VolumeName]; ok {
				if err := r.kernel.UpsertEdge(explicit.PVCBoundToPV(pvcID, pvID)); err != nil {
					return upserted, err
				}
				upserted++
			}
		}
		storageClassName := storageClassNameForPVC(pvc, snapshot.PVs)
		if storageClassName == "" {
			continue
		}
		storageClassID, ok := currentStorageClasses[storageClassName]
		if !ok {
			continue
		}
		resolver := "pvc-storageclass/v1"
		if pvc.StorageClassName == "" {
			resolver = "pvc-bound-pv-storageclass/v1"
		}
		if err := r.kernel.UpsertEdge(explicit.ResourceUsesStorageClass(pvcID, storageClassID, resolver)); err != nil {
			return upserted, err
		}
		upserted++
	}

	for _, pv := range snapshot.PVs {
		if pv.StorageClassName == "" {
			continue
		}
		pvID, pvOK := currentPVs[pv.Metadata.Name]
		storageClassID, storageClassOK := currentStorageClasses[pv.StorageClassName]
		if !pvOK || !storageClassOK {
			continue
		}
		if err := r.kernel.UpsertEdge(explicit.ResourceUsesStorageClass(pvID, storageClassID, "pv-storageclass/v1")); err != nil {
			return upserted, err
		}
		upserted++
	}

	for _, storageClass := range snapshot.StorageClasses {
		if storageClass.Provisioner == "" {
			continue
		}
		storageClassID, storageClassOK := currentStorageClasses[storageClass.Metadata.Name]
		driverID, driverOK := currentCSIDrivers[storageClass.Provisioner]
		if !storageClassOK || !driverOK {
			continue
		}
		if err := r.kernel.UpsertEdge(infer.StorageClassProvisionedByCSIDriver(storageClassID, driverID)); err != nil {
			return upserted, err
		}
		upserted++
	}

	infraPods := storageInfraPods(r.cluster, snapshot)
	for _, pv := range snapshot.PVs {
		pvID, ok := currentPVs[pv.Metadata.Name]
		if !ok {
			continue
		}
		driver, _ := pv.CSI["driver"]
		if driver == "" {
			continue
		}
		correlator, ok := r.csi.Correlator(driver)
		if !ok {
			continue
		}
		pvNode := pvNode(r.cluster, pv)
		pvNode.ID = pvID
		correlation := infer.CorrelatePVToCSIComponents(correlator, pvNode, csiAgentNodeNamesForPV(pv, snapshot), infraPods)
		for _, edge := range correlation.Edges {
			if err := r.kernel.UpsertEdge(edge); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	for driverName, driverID := range currentCSIDrivers {
		driverNode, ok := r.kernel.GetNode(driverID)
		if !ok {
			driverNode = model.Node{ID: driverID, Kind: model.NodeKindCSIDriver, SourceKind: "CSIDriver", Name: driverName}
		}
		correlation := infer.InferCSIComponentEdges(driverNode, infraPods, r.rules)
		for _, edge := range correlation.Edges {
			if err := r.kernel.UpsertEdge(edge); err != nil {
				return upserted, err
			}
			upserted++
		}
	}
	return upserted, nil
}

func isStorageEdge(kind model.EdgeKind) bool {
	switch kind {
	case model.EdgeKindMountsPVC,
		model.EdgeKindBoundToPV,
		model.EdgeKindUsesStorageClass,
		model.EdgeKindProvisionedByCSIDriver,
		model.EdgeKindImplementedByCSIController,
		model.EdgeKindImplementedByCSINodeAgent,
		model.EdgeKindServedByCSINodeAgent,
		model.EdgeKindManagedByCSIController:
		return true
	default:
		return false
	}
}

func pvcIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.PVCs))
	for _, pvc := range snapshot.PVCs {
		out[pvc.Metadata.Namespace+"/"+pvc.Metadata.Name] = pvcID(cluster, pvc)
	}
	return out
}

func pvIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.PVs))
	for _, pv := range snapshot.PVs {
		out[pv.Metadata.Name] = pvID(cluster, pv)
	}
	return out
}

func storageClassIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.StorageClasses))
	for _, storageClass := range snapshot.StorageClasses {
		out[storageClass.Metadata.Name] = storageClassID(cluster, storageClass)
	}
	return out
}

func csiDriverIDs(cluster string, snapshot collectk8s.Snapshot, rules []infer.CSIComponentRule) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.CSIDrivers)+len(snapshot.StorageClasses))
	for _, csiDriver := range snapshot.CSIDrivers {
		out[csiDriver.Metadata.Name] = csiDriverID(cluster, csiDriver.Metadata.Name, csiDriver.Metadata.UID)
	}
	for _, storageClass := range snapshot.StorageClasses {
		if storageClass.Provisioner == "" {
			continue
		}
		if _, ok := out[storageClass.Provisioner]; ok {
			continue
		}
		if !infer.IsCSIProvisioner(storageClass.Provisioner, false, rules) {
			continue
		}
		out[storageClass.Provisioner] = csiDriverID(cluster, storageClass.Provisioner, "")
	}
	return out
}

func pvcID(cluster string, pvc resources.PVC) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "PVC", Namespace: pvc.Metadata.Namespace, Name: pvc.Metadata.Name, UID: pvc.Metadata.UID})
}

func pvID(cluster string, pv resources.PV) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "PV", Name: pv.Metadata.Name, UID: pv.Metadata.UID})
}

func storageClassID(cluster string, storageClass resources.StorageClass) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "storage.k8s.io", Kind: "StorageClass", Name: storageClass.Metadata.Name, UID: storageClass.Metadata.UID})
}

func csiDriverID(cluster, name, uid string) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "storage.k8s.io", Kind: "CSIDriver", Name: name, UID: uid})
}

func pvcNode(cluster string, pvc resources.PVC) model.Node {
	return model.Node{
		ID:         pvcID(cluster, pvc),
		Kind:       model.NodeKindPVC,
		SourceKind: "PersistentVolumeClaim",
		Name:       pvc.Metadata.Name,
		Namespace:  pvc.Metadata.Namespace,
		Attributes: map[string]any{"status": pvc.Status, "storageClassName": pvc.StorageClassName},
	}
}

func pvNode(cluster string, pv resources.PV) model.Node {
	return model.Node{
		ID:         pvID(cluster, pv),
		Kind:       model.NodeKindPV,
		SourceKind: "PersistentVolume",
		Name:       pv.Metadata.Name,
		Attributes: map[string]any{"status": pv.Status, "storageClassName": pv.StorageClassName, "csi": pv.CSI},
	}
}

func storageClassNode(cluster string, storageClass resources.StorageClass) model.Node {
	return model.Node{
		ID:         storageClassID(cluster, storageClass),
		Kind:       model.NodeKindStorageClass,
		SourceKind: "StorageClass",
		Name:       storageClass.Metadata.Name,
		Attributes: map[string]any{"provisioner": storageClass.Provisioner, "reclaimPolicy": storageClass.ReclaimPolicy, "volumeBindingMode": storageClass.VolumeBindingMode},
	}
}

func csiDriverNode(cluster string, csiDriver resources.CSIDriver) model.Node {
	return model.Node{
		ID:         csiDriverID(cluster, csiDriver.Metadata.Name, csiDriver.Metadata.UID),
		Kind:       model.NodeKindCSIDriver,
		SourceKind: "CSIDriver",
		Name:       csiDriver.Metadata.Name,
	}
}

func syntheticCSIDriverNode(cluster, name string) model.Node {
	return model.Node{
		ID:         csiDriverID(cluster, name, ""),
		Kind:       model.NodeKindCSIDriver,
		SourceKind: "CSIDriver",
		Name:       name,
		Attributes: map[string]any{"inferredFromStorageClass": true},
	}
}

func findCSIDriver(csiDrivers []resources.CSIDriver, name string) (resources.CSIDriver, bool) {
	for _, csiDriver := range csiDrivers {
		if csiDriver.Metadata.Name == name {
			return csiDriver, true
		}
	}
	return resources.CSIDriver{}, false
}

func storageClassNameForPVC(pvc resources.PVC, pvs []resources.PV) string {
	if pvc.StorageClassName != "" {
		return pvc.StorageClassName
	}
	if pvc.VolumeName == "" {
		return ""
	}
	for _, pv := range pvs {
		if pv.Metadata.Name == pvc.VolumeName {
			return pv.StorageClassName
		}
	}
	return ""
}

func csiAgentNodeNamesForPV(pv resources.PV, snapshot collectk8s.Snapshot) []string {
	seen := make(map[string]struct{})
	addNode := func(nodeName string) {
		nodeName = strings.TrimSpace(nodeName)
		if nodeName == "" {
			return
		}
		seen[nodeName] = struct{}{}
	}
	addNode(pv.CSI["nodeAffinity"])
	for _, pvc := range snapshot.PVCs {
		if pvc.VolumeName != pv.Metadata.Name {
			continue
		}
		for _, pod := range snapshot.Pods {
			if pod.Metadata.Namespace != pvc.Metadata.Namespace || pod.NodeName == "" || !podReferencesPVC(pod, pvc.Metadata.Name) {
				continue
			}
			addNode(pod.NodeName)
		}
	}
	out := make([]string, 0, len(seen))
	for nodeName := range seen {
		out = append(out, nodeName)
	}
	sort.Strings(out)
	return out
}

func podReferencesPVC(pod resources.Pod, pvcName string) bool {
	for _, ref := range pod.PVCRefs {
		if ref == pvcName {
			return true
		}
	}
	return false
}

func storageInfraPods(cluster string, snapshot collectk8s.Snapshot) []model.Node {
	out := make([]model.Node, 0)
	for _, pod := range snapshot.Pods {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
		out = append(out, model.Node{
			ID:         id,
			Kind:       model.NodeKindPod,
			SourceKind: "Pod",
			Name:       pod.Metadata.Name,
			Namespace:  pod.Metadata.Namespace,
			Attributes: map[string]any{"phase": pod.Phase, "reason": pod.Reason, "nodeName": pod.NodeName},
		})
	}
	return out
}
