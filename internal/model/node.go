package model

type NodeKind string

const (
	NodeKindCluster             NodeKind = "Cluster"
	NodeKindNamespace           NodeKind = "Namespace"
	NodeKindWorkload            NodeKind = "Workload"
	NodeKindPod                 NodeKind = "Pod"
	NodeKindContainer           NodeKind = "Container"
	NodeKindNode                NodeKind = "Node"
	NodeKindService             NodeKind = "Service"
	NodeKindConfigMap           NodeKind = "ConfigMap"
	NodeKindSecret              NodeKind = "Secret"
	NodeKindServiceAccount      NodeKind = "ServiceAccount"
	NodeKindRoleBinding         NodeKind = "RoleBinding"
	NodeKindClusterRoleBinding  NodeKind = "ClusterRoleBinding"
	NodeKindPVC                 NodeKind = "PVC"
	NodeKindPV                  NodeKind = "PV"
	NodeKindStorageClass        NodeKind = "StorageClass"
	NodeKindCSIDriver           NodeKind = "CSIDriver"
	NodeKindEvent               NodeKind = "Event"
	NodeKindImage               NodeKind = "Image"
	NodeKindOCIArtifactMetadata NodeKind = "OCIArtifactMetadata"
	NodeKindWebhookConfig       NodeKind = "WebhookConfig"
)

type Node struct {
	ID         CanonicalID
	Kind       NodeKind
	SourceKind string
	Name       string
	Namespace  string
	Attributes map[string]any
}
