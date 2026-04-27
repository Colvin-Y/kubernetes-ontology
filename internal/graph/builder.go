package graph

import (
	"strings"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	k8sresources "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/enrich/oci"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/owner"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/selector"
)

type Builder struct {
	cluster           string
	csiCorrelator     *infer.Registry
	csiComponentRules []infer.CSIComponentRule
	controllerRules   []infer.WorkloadControllerRule
	lastEvidence      []string
}

func NewBuilder(cluster string) *Builder {
	return &Builder{
		cluster:           cluster,
		csiCorrelator:     infer.NewRegistry(infer.OpenLocalCorrelator{}),
		csiComponentRules: infer.DefaultCSIComponentRules(),
	}
}

func (b *Builder) SetWorkloadControllerRules(rules []infer.WorkloadControllerRule) {
	b.controllerRules = rules
}

func (b *Builder) Build(snapshot k8s.Snapshot) ([]model.Node, []model.Edge) {
	b.lastEvidence = nil
	nodes := make([]model.Node, 0)
	edges := make([]model.Edge, 0)

	podIDs := make(map[string]model.CanonicalID)
	workloadIDs := make(map[string]model.CanonicalID)
	nodeIDs := make(map[string]model.CanonicalID)
	serviceIDs := make(map[string]model.CanonicalID)
	configMapIDs := make(map[string]model.CanonicalID)
	secretIDs := make(map[string]model.CanonicalID)
	serviceAccountIDs := make(map[string]model.CanonicalID)
	roleBindingIDs := make(map[string]model.CanonicalID)
	clusterRoleBindingIDs := make(map[string]model.CanonicalID)
	pvcIDs := make(map[string]model.CanonicalID)
	pvIDs := make(map[string]model.CanonicalID)
	storageClassIDs := make(map[string]model.CanonicalID)
	csiDriverIDs := make(map[string]model.CanonicalID)
	webhookIDs := make(map[string]model.CanonicalID)
	eventTargetKinds := make(map[string]model.CanonicalID)

	for _, workload := range snapshot.Workloads {
		w := model.NewWorkload(b.cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID, map[string]any{"apiVersion": workload.APIVersion, "replicas": workload.Replicas, "conditions": workload.Conditions})
		workloadIDs[workload.Metadata.Namespace+"/"+workload.Metadata.Name] = w.ID
		eventTargetKinds[workload.ControllerKind+"/"+workload.Metadata.UID] = w.ID
		nodes = append(nodes, model.Node{ID: w.ID, Kind: model.NodeKindWorkload, SourceKind: workload.ControllerKind, Name: workload.Metadata.Name, Namespace: workload.Metadata.Namespace, Attributes: w.Attributes})
	}

	ownerResolver := owner.NewChainResolver(b.cluster, snapshot.Workloads, snapshot.ReplicaSets)
	for _, pod := range snapshot.Pods {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Pod", Namespace: pod.Metadata.Namespace, Name: pod.Metadata.Name, UID: pod.Metadata.UID})
		podIDs[pod.Metadata.Namespace+"/"+pod.Metadata.Name] = id
		eventTargetKinds["Pod/"+pod.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindPod, SourceKind: "Pod", Name: pod.Metadata.Name, Namespace: pod.Metadata.Namespace, Attributes: map[string]any{"phase": pod.Phase, "reason": pod.Reason, "nodeName": pod.NodeName}})

		for idx, image := range pod.ContainerImages {
			imageRef := oci.ParseImageRef(image)
			imageID := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Image", Name: imageRef.Original})
			nodes = append(nodes, model.Node{ID: imageID, Kind: model.NodeKindImage, SourceKind: "Image", Name: imageRef.Original, Attributes: map[string]any{"repo": imageRef.Repo, "tag": imageRef.Tag, "digest": imageRef.Digest, "containerIndex": idx}})
			edges = append(edges, explicit.PodUsesImage(id, imageID))
		}
	}

	for _, node := range snapshot.Nodes {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Node", Name: node.Metadata.Name, UID: node.Metadata.UID})
		nodeIDs[node.Metadata.Name] = id
		eventTargetKinds["Node/"+node.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindNode, SourceKind: "Node", Name: node.Metadata.Name, Attributes: map[string]any{"conditions": node.Conditions}})
	}

	for _, service := range snapshot.Services {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Service", Namespace: service.Metadata.Namespace, Name: service.Metadata.Name, UID: service.Metadata.UID})
		serviceIDs[service.Metadata.Namespace+"/"+service.Metadata.Name] = id
		eventTargetKinds["Service/"+service.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindService, SourceKind: "Service", Name: service.Metadata.Name, Namespace: service.Metadata.Namespace, Attributes: map[string]any{"selector": service.Selector}})
	}

	for _, configMap := range snapshot.ConfigMaps {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "ConfigMap", Namespace: configMap.Metadata.Namespace, Name: configMap.Metadata.Name, UID: configMap.Metadata.UID})
		configMapIDs[configMap.Metadata.Namespace+"/"+configMap.Metadata.Name] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindConfigMap, SourceKind: "ConfigMap", Name: configMap.Metadata.Name, Namespace: configMap.Metadata.Namespace})
	}

	for _, secret := range snapshot.Secrets {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Secret", Namespace: secret.Metadata.Namespace, Name: secret.Metadata.Name, UID: secret.Metadata.UID})
		secretIDs[secret.Metadata.Namespace+"/"+secret.Metadata.Name] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindSecret, SourceKind: "Secret", Name: secret.Metadata.Name, Namespace: secret.Metadata.Namespace})
	}

	for _, sa := range snapshot.ServiceAccounts {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "ServiceAccount", Namespace: sa.Metadata.Namespace, Name: sa.Metadata.Name, UID: sa.Metadata.UID})
		serviceAccountIDs[sa.Metadata.Namespace+"/"+sa.Metadata.Name] = id
		eventTargetKinds["ServiceAccount/"+sa.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindServiceAccount, SourceKind: "ServiceAccount", Name: sa.Metadata.Name, Namespace: sa.Metadata.Namespace})
	}

	for _, roleBinding := range snapshot.RoleBindings {
		id := roleBindingID(b.cluster, roleBinding)
		roleBindingIDs[roleBinding.Metadata.Namespace+"/"+roleBinding.Metadata.Name] = id
		nodes = append(nodes, roleBindingNode(b.cluster, roleBinding))
	}

	for _, clusterRoleBinding := range snapshot.ClusterRoleBindings {
		id := clusterRoleBindingID(b.cluster, clusterRoleBinding)
		clusterRoleBindingIDs[clusterRoleBinding.Metadata.Name] = id
		nodes = append(nodes, clusterRoleBindingNode(b.cluster, clusterRoleBinding))
	}

	for _, pvc := range snapshot.PVCs {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "PVC", Namespace: pvc.Metadata.Namespace, Name: pvc.Metadata.Name, UID: pvc.Metadata.UID})
		pvcIDs[pvc.Metadata.Namespace+"/"+pvc.Metadata.Name] = id
		eventTargetKinds["PersistentVolumeClaim/"+pvc.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindPVC, SourceKind: "PersistentVolumeClaim", Name: pvc.Metadata.Name, Namespace: pvc.Metadata.Namespace, Attributes: map[string]any{"status": pvc.Status, "storageClassName": pvc.StorageClassName}})
	}

	for _, pv := range snapshot.PVs {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "PV", Name: pv.Metadata.Name, UID: pv.Metadata.UID})
		pvIDs[pv.Metadata.Name] = id
		eventTargetKinds["PersistentVolume/"+pv.Metadata.UID] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindPV, SourceKind: "PersistentVolume", Name: pv.Metadata.Name, Attributes: map[string]any{"status": pv.Status, "storageClassName": pv.StorageClassName, "csi": pv.CSI}})
	}

	for _, storageClass := range snapshot.StorageClasses {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "storage.k8s.io", Kind: "StorageClass", Name: storageClass.Metadata.Name, UID: storageClass.Metadata.UID})
		storageClassIDs[storageClass.Metadata.Name] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindStorageClass, SourceKind: "StorageClass", Name: storageClass.Metadata.Name, Attributes: map[string]any{"provisioner": storageClass.Provisioner, "reclaimPolicy": storageClass.ReclaimPolicy, "volumeBindingMode": storageClass.VolumeBindingMode}})
	}

	for _, csiDriver := range snapshot.CSIDrivers {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "storage.k8s.io", Kind: "CSIDriver", Name: csiDriver.Metadata.Name, UID: csiDriver.Metadata.UID})
		csiDriverIDs[csiDriver.Metadata.Name] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindCSIDriver, SourceKind: "CSIDriver", Name: csiDriver.Metadata.Name})
	}

	for _, webhook := range snapshot.WebhookConfigs {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "admissionregistration.k8s.io", Kind: "WebhookConfig", Name: webhook.Metadata.Name, UID: webhook.Metadata.UID})
		webhookIDs[webhook.Metadata.Name] = id
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindWebhookConfig, SourceKind: webhook.Kind, Name: webhook.Metadata.Name})
	}

	for _, event := range snapshot.Events {
		id := model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "core", Kind: "Event", Namespace: event.Metadata.Namespace, Name: event.Metadata.Name, UID: event.Metadata.UID})
		nodes = append(nodes, model.Node{ID: id, Kind: model.NodeKindEvent, SourceKind: "Event", Name: event.Metadata.Name, Namespace: event.Metadata.Namespace, Attributes: map[string]any{"reason": event.Reason, "message": event.Message}})
		targetID, ok := eventTargetKinds[event.InvolvedKind+"/"+event.InvolvedUID]
		if ok {
			edges = append(edges, explicit.EventReportsOn(id, targetID))
		}
	}

	for _, pod := range snapshot.Pods {
		podID := podIDs[pod.Metadata.Namespace+"/"+pod.Metadata.Name]
		if pod.NodeName != "" {
			if nodeID, ok := nodeIDs[pod.NodeName]; ok {
				edges = append(edges, explicit.PodScheduledOn(podID, nodeID))
			}
		}
		for _, ref := range pod.ConfigMapRefs {
			if configMapID, ok := configMapIDs[pod.Metadata.Namespace+"/"+ref]; ok {
				edges = append(edges, explicit.PodUsesConfigMap(podID, configMapID))
			}
		}
		for _, ref := range pod.SecretRefs {
			if secretID, ok := secretIDs[pod.Metadata.Namespace+"/"+ref]; ok {
				edges = append(edges, explicit.PodUsesSecret(podID, secretID))
			}
		}
		for _, ref := range pod.PVCRefs {
			if pvcID, ok := pvcIDs[pod.Metadata.Namespace+"/"+ref]; ok {
				edges = append(edges, explicit.PodMountsPVC(podID, pvcID))
			}
		}
		if pod.ServiceAccount != "" {
			if saID, ok := serviceAccountIDs[pod.Metadata.Namespace+"/"+pod.ServiceAccount]; ok {
				edges = append(edges, explicit.PodUsesServiceAccount(podID, saID))
			}
		}
		for _, target := range ownerResolver.ResolvePodWorkloads(pod) {
			edges = append(edges, owner.PodManagedByWorkload(podID, target.ID))
			edges = append(edges, owner.WorkloadOwnsPod(target.ID, podID))
		}
	}

	for _, roleBinding := range snapshot.RoleBindings {
		bindingID := roleBindingIDs[roleBinding.Metadata.Namespace+"/"+roleBinding.Metadata.Name]
		for index, kind := range roleBinding.SubjectKinds {
			if !isServiceAccountSubject(kind) {
				continue
			}
			namespace := roleBindingSubjectNamespace(roleBinding, index)
			name := subjectName(roleBinding.SubjectNames, index)
			if namespace == "" || name == "" {
				continue
			}
			accountID, ok := serviceAccountIDs[namespace+"/"+name]
			if !ok {
				continue
			}
			edges = append(edges, infer.SubjectBoundByRoleBinding(accountID, bindingID))
		}
	}

	for _, clusterRoleBinding := range snapshot.ClusterRoleBindings {
		bindingID := clusterRoleBindingIDs[clusterRoleBinding.Metadata.Name]
		for index, kind := range clusterRoleBinding.SubjectKinds {
			if !isServiceAccountSubject(kind) {
				continue
			}
			namespace := clusterRoleBindingSubjectNamespace(clusterRoleBinding, index)
			name := subjectName(clusterRoleBinding.SubjectNames, index)
			if namespace == "" || name == "" {
				continue
			}
			accountID, ok := serviceAccountIDs[namespace+"/"+name]
			if !ok {
				continue
			}
			edges = append(edges, infer.SubjectBoundByRoleBinding(accountID, bindingID))
		}
	}

	for _, workload := range snapshot.Workloads {
		workloadID := model.WorkloadID(b.cluster, workload.Metadata.Namespace, workload.ControllerKind, workload.Metadata.Name, workload.Metadata.UID)
		for _, target := range ownerResolver.ResolveWorkloadOwners(workload) {
			if target.ID == workloadID {
				continue
			}
			edges = append(edges, owner.WorkloadControlledBy(workloadID, target.ID))
		}
	}
	edges = append(edges, infer.InferWorkloadControllerEdges(b.cluster, snapshot, b.controllerRules)...)

	for _, pvc := range snapshot.PVCs {
		if pvc.VolumeName != "" {
			if pvID, ok := pvIDs[pvc.VolumeName]; ok {
				edges = append(edges, explicit.PVCBoundToPV(pvcIDs[pvc.Metadata.Namespace+"/"+pvc.Metadata.Name], pvID))
			}
		}
		if storageClassName := storageClassNameForPVC(pvc, snapshot.PVs); storageClassName != "" {
			if storageClassID, ok := storageClassIDs[storageClassName]; ok {
				resolver := "pvc-storageclass/v1"
				if pvc.StorageClassName == "" {
					resolver = "pvc-bound-pv-storageclass/v1"
				}
				edges = append(edges, explicit.ResourceUsesStorageClass(pvcIDs[pvc.Metadata.Namespace+"/"+pvc.Metadata.Name], storageClassID, resolver))
			}
		}
	}

	for _, pv := range snapshot.PVs {
		if pv.StorageClassName == "" {
			continue
		}
		pvID, pvOK := pvIDs[pv.Metadata.Name]
		storageClassID, storageClassOK := storageClassIDs[pv.StorageClassName]
		if pvOK && storageClassOK {
			edges = append(edges, explicit.ResourceUsesStorageClass(pvID, storageClassID, "pv-storageclass/v1"))
		}
	}

	for _, storageClass := range snapshot.StorageClasses {
		if storageClass.Provisioner == "" {
			continue
		}
		driverID, ok := csiDriverIDs[storageClass.Provisioner]
		if !ok {
			if !infer.IsCSIProvisioner(storageClass.Provisioner, false, b.csiComponentRules) {
				continue
			}
			driverID = model.NewCanonicalID(model.ResourceRef{Cluster: b.cluster, Group: "storage.k8s.io", Kind: "CSIDriver", Name: storageClass.Provisioner})
			csiDriverIDs[storageClass.Provisioner] = driverID
			nodes = append(nodes, model.Node{ID: driverID, Kind: model.NodeKindCSIDriver, SourceKind: "CSIDriver", Name: storageClass.Provisioner, Attributes: map[string]any{"inferredFromStorageClass": true}})
		}
		edges = append(edges, infer.StorageClassProvisionedByCSIDriver(storageClassIDs[storageClass.Metadata.Name], driverID))
	}

	infraPods := make([]model.Node, 0)
	for _, node := range nodes {
		if node.Kind == model.NodeKindPod && node.Namespace == "kube-system" {
			infraPods = append(infraPods, node)
		}
	}
	for _, pv := range snapshot.PVs {
		pvID, ok := pvIDs[pv.Metadata.Name]
		if !ok {
			continue
		}
		driver, _ := pv.CSI["driver"]
		if driver == "" {
			continue
		}
		correlator, ok := b.csiCorrelator.Correlator(driver)
		if !ok {
			continue
		}
		affinityNodeName, _ := pv.CSI["nodeAffinity"]
		pvNode, ok := findNode(nodes, pvID)
		if !ok {
			continue
		}
		correlation := correlator.Correlate(pvNode, affinityNodeName, infraPods)
		edges = append(edges, correlation.Edges...)
		b.lastEvidence = append(b.lastEvidence, correlation.Evidence...)
	}
	for driverName, driverID := range csiDriverIDs {
		driverNode, ok := findNode(nodes, driverID)
		if !ok {
			driverNode = model.Node{ID: driverID, Kind: model.NodeKindCSIDriver, SourceKind: "CSIDriver", Name: driverName}
		}
		correlation := infer.InferCSIComponentEdges(driverNode, infraPods, b.csiComponentRules)
		edges = append(edges, correlation.Edges...)
		b.lastEvidence = append(b.lastEvidence, correlation.Evidence...)
	}

	for _, service := range snapshot.Services {
		serviceID := serviceIDs[service.Metadata.Namespace+"/"+service.Metadata.Name]
		for _, pod := range snapshot.Pods {
			if pod.Metadata.Namespace != service.Metadata.Namespace {
				continue
			}
			if selector.LabelsMatch(service.Selector, pod.Metadata.Labels) {
				edges = append(edges, selector.ServiceSelectsPod(serviceID, podIDs[pod.Metadata.Namespace+"/"+pod.Metadata.Name]))
			}
		}
	}

	for _, event := range snapshot.Events {
		if event.Reason == "FailedCreate" || event.Reason == "FailedAdmission" {
			for name, webhookID := range webhookIDs {
				if workloadID, ok := workloadIDs[event.Metadata.Namespace+"/"+event.InvolvedName]; ok {
					_ = name
					edges = append(edges, infer.WorkloadAffectedByWebhook(workloadID, webhookID))
					break
				}
			}
		}
	}

	return nodes, edges
}

func storageClassNameForPVC(pvc k8sresources.PVC, pvs []k8sresources.PV) string {
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

func roleBindingID(cluster string, roleBinding k8sresources.RoleBinding) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "rbac.authorization.k8s.io", Kind: "RoleBinding", Namespace: roleBinding.Metadata.Namespace, Name: roleBinding.Metadata.Name, UID: roleBinding.Metadata.UID})
}

func clusterRoleBindingID(cluster string, clusterRoleBinding k8sresources.ClusterRoleBinding) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "rbac.authorization.k8s.io", Kind: "ClusterRoleBinding", Name: clusterRoleBinding.Metadata.Name, UID: clusterRoleBinding.Metadata.UID})
}

func roleBindingNode(cluster string, roleBinding k8sresources.RoleBinding) model.Node {
	return model.Node{
		ID:         roleBindingID(cluster, roleBinding),
		Kind:       model.NodeKindRoleBinding,
		SourceKind: "RoleBinding",
		Name:       roleBinding.Metadata.Name,
		Namespace:  roleBinding.Metadata.Namespace,
		Attributes: map[string]any{
			"roleRef":           roleBinding.RoleRef,
			"subjectKinds":      roleBinding.SubjectKinds,
			"subjectNames":      roleBinding.SubjectNames,
			"subjectNamespaces": roleBinding.SubjectNamespaces,
		},
	}
}

func clusterRoleBindingNode(cluster string, clusterRoleBinding k8sresources.ClusterRoleBinding) model.Node {
	return model.Node{
		ID:         clusterRoleBindingID(cluster, clusterRoleBinding),
		Kind:       model.NodeKindClusterRoleBinding,
		SourceKind: "ClusterRoleBinding",
		Name:       clusterRoleBinding.Metadata.Name,
		Attributes: map[string]any{
			"roleRef":           clusterRoleBinding.RoleRef,
			"subjectKinds":      clusterRoleBinding.SubjectKinds,
			"subjectNames":      clusterRoleBinding.SubjectNames,
			"subjectNamespaces": clusterRoleBinding.SubjectNamespaces,
		},
	}
}

func isServiceAccountSubject(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "ServiceAccount")
}

func roleBindingSubjectNamespace(roleBinding k8sresources.RoleBinding, index int) string {
	namespace := subjectName(roleBinding.SubjectNamespaces, index)
	if namespace != "" {
		return namespace
	}
	return roleBinding.Metadata.Namespace
}

func clusterRoleBindingSubjectNamespace(clusterRoleBinding k8sresources.ClusterRoleBinding, index int) string {
	return subjectName(clusterRoleBinding.SubjectNamespaces, index)
}

func subjectName(subjects []string, index int) string {
	if index < 0 || index >= len(subjects) {
		return ""
	}
	return strings.TrimSpace(subjects[index])
}

func findNode(nodes []model.Node, id model.CanonicalID) (model.Node, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return model.Node{}, false
}

func (b *Builder) Evidence() []string {
	out := make([]string, len(b.lastEvidence))
	copy(out, b.lastEvidence)
	return out
}
