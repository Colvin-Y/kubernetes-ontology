package resources

import (
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Metadata struct {
	UID         string
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

type OwnerReference struct {
	APIVersion string
	Kind       string
	Name       string
	UID        string
	Controller bool
}

type Workload struct {
	Metadata        Metadata
	APIVersion      string
	ControllerKind  string
	Replicas        int32
	Conditions      map[string]string
	OwnerReferences []OwnerReference
}

type ReplicaSet struct {
	Metadata        Metadata
	Replicas        int32
	Conditions      map[string]string
	OwnerReferences []OwnerReference
}

type Pod struct {
	Metadata        Metadata
	NodeName        string
	ServiceAccount  string
	OwnerReferences []OwnerReference
	ContainerImages []string
	ConfigMapRefs   []string
	SecretRefs      []string
	PVCRefs         []string
	Phase           string
	Reason          string
}

type Node struct {
	Metadata   Metadata
	Conditions map[string]string
}

type Service struct {
	Metadata Metadata
	Selector map[string]string
}

type ConfigMap struct{ Metadata Metadata }

type Secret struct{ Metadata Metadata }

type ServiceAccount struct{ Metadata Metadata }

type RoleBinding struct {
	Metadata          Metadata
	RoleRef           string
	SubjectKinds      []string
	SubjectNames      []string
	SubjectNamespaces []string
}

type ClusterRoleBinding struct {
	Metadata          Metadata
	RoleRef           string
	SubjectKinds      []string
	SubjectNames      []string
	SubjectNamespaces []string
}

type PVC struct {
	Metadata         Metadata
	VolumeName       string
	StorageClassName string
	Status           string
}

type PV struct {
	Metadata         Metadata
	StorageClassName string
	Status           string
	CSI              map[string]string
}

type StorageClass struct {
	Metadata          Metadata
	Provisioner       string
	ReclaimPolicy     string
	VolumeBindingMode string
}

type CSIDriver struct {
	Metadata Metadata
}

type Event struct {
	Metadata     Metadata
	InvolvedKind string
	InvolvedName string
	InvolvedUID  string
	Reason       string
	Message      string
}

type WebhookConfig struct {
	Metadata Metadata
	Kind     string
}

func normalizeOwnerReferences(in []metav1.OwnerReference) []OwnerReference {
	out := make([]OwnerReference, 0, len(in))
	for _, item := range in {
		controller := item.Controller != nil && *item.Controller
		out = append(out, OwnerReference{APIVersion: item.APIVersion, Kind: item.Kind, Name: item.Name, UID: string(item.UID), Controller: controller})
	}
	return out
}

func NormalizeDeployment(in appsv1.Deployment) Workload {
	replicas := int32(1)
	if in.Spec.Replicas != nil {
		replicas = *in.Spec.Replicas
	}
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return Workload{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, APIVersion: "apps/v1", ControllerKind: "Deployment", Replicas: replicas, Conditions: conditions, OwnerReferences: normalizeOwnerReferences(in.OwnerReferences)}
}

func NormalizeStatefulSet(in appsv1.StatefulSet) Workload {
	replicas := int32(1)
	if in.Spec.Replicas != nil {
		replicas = *in.Spec.Replicas
	}
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return Workload{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, APIVersion: "apps/v1", ControllerKind: "StatefulSet", Replicas: replicas, Conditions: conditions, OwnerReferences: normalizeOwnerReferences(in.OwnerReferences)}
}

func NormalizeDaemonSet(in appsv1.DaemonSet) Workload {
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return Workload{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, APIVersion: "apps/v1", ControllerKind: "DaemonSet", Replicas: in.Status.DesiredNumberScheduled, Conditions: conditions, OwnerReferences: normalizeOwnerReferences(in.OwnerReferences)}
}

func NormalizeJob(in batchv1.Job) Workload {
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return Workload{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, APIVersion: "batch/v1", ControllerKind: "Job", Replicas: 1, Conditions: conditions, OwnerReferences: normalizeOwnerReferences(in.OwnerReferences)}
}

func NormalizeUnstructuredWorkload(in unstructured.Unstructured, kind string) Workload {
	if kind == "" {
		kind = in.GetKind()
	}
	replicas := int32(0)
	if value, ok, _ := unstructured.NestedInt64(in.Object, "status", "replicas"); ok {
		replicas = int32(value)
	} else if value, ok, _ := unstructured.NestedInt64(in.Object, "spec", "replicas"); ok {
		replicas = int32(value)
	}
	return Workload{
		Metadata:        Metadata{UID: string(in.GetUID()), Name: in.GetName(), Namespace: in.GetNamespace(), Labels: in.GetLabels(), Annotations: in.GetAnnotations()},
		APIVersion:      in.GetAPIVersion(),
		ControllerKind:  kind,
		Replicas:        replicas,
		Conditions:      unstructuredConditions(in),
		OwnerReferences: normalizeOwnerReferences(in.GetOwnerReferences()),
	}
}

func unstructuredConditions(in unstructured.Unstructured) map[string]string {
	conditions, ok, _ := unstructured.NestedSlice(in.Object, "status", "conditions")
	if !ok {
		return nil
	}
	out := make(map[string]string, len(conditions))
	for _, raw := range conditions {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		conditionType, _ := item["type"].(string)
		status, _ := item["status"].(string)
		if conditionType != "" {
			out[conditionType] = status
		}
	}
	return out
}

func NormalizeReplicaSet(in appsv1.ReplicaSet) ReplicaSet {
	replicas := int32(1)
	if in.Spec.Replicas != nil {
		replicas = *in.Spec.Replicas
	}
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return ReplicaSet{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, Replicas: replicas, Conditions: conditions, OwnerReferences: normalizeOwnerReferences(in.OwnerReferences)}
}

func NormalizePod(in corev1.Pod) Pod {
	containerImages := make([]string, 0, len(in.Spec.Containers))
	configMapRefs := make([]string, 0)
	secretRefs := make([]string, 0)
	pvcRefs := make([]string, 0)
	for _, container := range in.Spec.Containers {
		containerImages = append(containerImages, container.Image)
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				configMapRefs = append(configMapRefs, env.ValueFrom.ConfigMapKeyRef.Name)
			}
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				secretRefs = append(secretRefs, env.ValueFrom.SecretKeyRef.Name)
			}
		}
	}
	for _, volume := range in.Spec.Volumes {
		if volume.ConfigMap != nil {
			configMapRefs = append(configMapRefs, volume.ConfigMap.Name)
		}
		if volume.Secret != nil {
			secretRefs = append(secretRefs, volume.Secret.SecretName)
		}
		if volume.PersistentVolumeClaim != nil {
			pvcRefs = append(pvcRefs, volume.PersistentVolumeClaim.ClaimName)
		}
	}
	return Pod{
		Metadata:        Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations},
		NodeName:        in.Spec.NodeName,
		ServiceAccount:  in.Spec.ServiceAccountName,
		OwnerReferences: normalizeOwnerReferences(in.OwnerReferences),
		ContainerImages: containerImages,
		ConfigMapRefs:   configMapRefs,
		SecretRefs:      secretRefs,
		PVCRefs:         pvcRefs,
		Phase:           string(in.Status.Phase),
		Reason:          in.Status.Reason,
	}
}

func NormalizeNode(in corev1.Node) Node {
	conditions := make(map[string]string, len(in.Status.Conditions))
	for _, condition := range in.Status.Conditions {
		conditions[string(condition.Type)] = string(condition.Status)
	}
	return Node{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations}, Conditions: conditions}
}

func NormalizeService(in corev1.Service) Service {
	return Service{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, Selector: in.Spec.Selector}
}

func NormalizeConfigMap(in corev1.ConfigMap) ConfigMap {
	return ConfigMap{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}}
}

func NormalizeSecret(in corev1.Secret) Secret {
	return Secret{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}}
}

func NormalizeServiceAccount(in corev1.ServiceAccount) ServiceAccount {
	return ServiceAccount{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}}
}

func NormalizeRoleBinding(in rbacv1.RoleBinding) RoleBinding {
	subjectKinds := make([]string, 0, len(in.Subjects))
	subjectNames := make([]string, 0, len(in.Subjects))
	subjectNamespaces := make([]string, 0, len(in.Subjects))
	for _, subject := range in.Subjects {
		subjectKinds = append(subjectKinds, subject.Kind)
		subjectNames = append(subjectNames, subject.Name)
		subjectNamespaces = append(subjectNamespaces, subject.Namespace)
	}
	return RoleBinding{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, RoleRef: in.RoleRef.Name, SubjectKinds: subjectKinds, SubjectNames: subjectNames, SubjectNamespaces: subjectNamespaces}
}

func NormalizeClusterRoleBinding(in rbacv1.ClusterRoleBinding) ClusterRoleBinding {
	subjectKinds := make([]string, 0, len(in.Subjects))
	subjectNames := make([]string, 0, len(in.Subjects))
	subjectNamespaces := make([]string, 0, len(in.Subjects))
	for _, subject := range in.Subjects {
		subjectKinds = append(subjectKinds, subject.Kind)
		subjectNames = append(subjectNames, subject.Name)
		subjectNamespaces = append(subjectNamespaces, subject.Namespace)
	}
	return ClusterRoleBinding{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations}, RoleRef: in.RoleRef.Name, SubjectKinds: subjectKinds, SubjectNames: subjectNames, SubjectNamespaces: subjectNamespaces}
}

func NormalizePVC(in corev1.PersistentVolumeClaim) PVC {
	return PVC{
		Metadata:         Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations},
		VolumeName:       in.Spec.VolumeName,
		StorageClassName: storageClassName(in.Spec.StorageClassName, in.Annotations),
		Status:           string(in.Status.Phase),
	}
}

func NormalizePV(in corev1.PersistentVolume) PV {
	csi := map[string]string{}
	if in.Spec.CSI != nil {
		csi["driver"] = in.Spec.CSI.Driver
		csi["handle"] = in.Spec.CSI.VolumeHandle
		if in.Spec.NodeAffinity != nil && len(in.Spec.NodeAffinity.Required.NodeSelectorTerms) > 0 {
			for _, expr := range in.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions {
				if expr.Key == "kubernetes.io/hostname" && len(expr.Values) > 0 {
					csi["nodeAffinity"] = expr.Values[0]
					break
				}
			}
		}
	}
	return PV{
		Metadata:         Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations},
		StorageClassName: in.Spec.StorageClassName,
		Status:           string(in.Status.Phase),
		CSI:              csi,
	}
}

func NormalizeStorageClass(in storagev1.StorageClass) StorageClass {
	reclaimPolicy := ""
	if in.ReclaimPolicy != nil {
		reclaimPolicy = string(*in.ReclaimPolicy)
	}
	volumeBindingMode := ""
	if in.VolumeBindingMode != nil {
		volumeBindingMode = string(*in.VolumeBindingMode)
	}
	return StorageClass{
		Metadata:          Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations},
		Provisioner:       in.Provisioner,
		ReclaimPolicy:     reclaimPolicy,
		VolumeBindingMode: volumeBindingMode,
	}
}

func NormalizeCSIDriver(in storagev1.CSIDriver) CSIDriver {
	return CSIDriver{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations}}
}

func storageClassName(spec *string, annotations map[string]string) string {
	if spec != nil {
		return *spec
	}
	if annotations == nil {
		return ""
	}
	return annotations["volume.beta.kubernetes.io/storage-class"]
}

func NormalizeEvent(in corev1.Event) Event {
	return Event{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Namespace: in.Namespace, Labels: in.Labels, Annotations: in.Annotations}, InvolvedKind: in.InvolvedObject.Kind, InvolvedName: in.InvolvedObject.Name, InvolvedUID: string(in.InvolvedObject.UID), Reason: in.Reason, Message: in.Message}
}

func NormalizeMutatingWebhookConfig(in admissionv1.MutatingWebhookConfiguration) WebhookConfig {
	return WebhookConfig{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations}, Kind: "MutatingWebhookConfiguration"}
}

func NormalizeValidatingWebhookConfig(in admissionv1.ValidatingWebhookConfiguration) WebhookConfig {
	return WebhookConfig{Metadata: Metadata{UID: string(in.UID), Name: in.Name, Labels: in.Labels, Annotations: in.Annotations}, Kind: "ValidatingWebhookConfiguration"}
}
