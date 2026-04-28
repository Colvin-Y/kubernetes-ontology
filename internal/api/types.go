package api

import "time"

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

type EdgeKind string

const (
	EdgeKindBelongsToNamespace         EdgeKind = "belongs_to_namespace"
	EdgeKindManagedBy                  EdgeKind = "managed_by"
	EdgeKindControlledBy               EdgeKind = "controlled_by"
	EdgeKindOwnsPod                    EdgeKind = "owns_pod"
	EdgeKindScheduledOn                EdgeKind = "scheduled_on"
	EdgeKindSelectsPod                 EdgeKind = "selects_pod"
	EdgeKindSelectedByService          EdgeKind = "selected_by_service"
	EdgeKindUsesConfigMap              EdgeKind = "uses_config_map"
	EdgeKindUsesSecret                 EdgeKind = "uses_secret"
	EdgeKindUsesServiceAccount         EdgeKind = "uses_service_account"
	EdgeKindBoundByRoleBinding         EdgeKind = "bound_by_role_binding"
	EdgeKindMountsPVC                  EdgeKind = "mounts_pvc"
	EdgeKindBoundToPV                  EdgeKind = "bound_to_pv"
	EdgeKindUsesStorageClass           EdgeKind = "uses_storage_class"
	EdgeKindProvisionedByCSIDriver     EdgeKind = "provisioned_by_csi_driver"
	EdgeKindHasContainer               EdgeKind = "has_container"
	EdgeKindUsesImage                  EdgeKind = "uses_image"
	EdgeKindHasOCIArtifact             EdgeKind = "has_oci_artifact"
	EdgeKindReportedByEvent            EdgeKind = "reported_by_event"
	EdgeKindAffectedByWebhook          EdgeKind = "affected_by_webhook"
	EdgeKindManagedByController        EdgeKind = "managed_by_controller"
	EdgeKindServedByNodeDaemon         EdgeKind = "served_by_node_daemon"
	EdgeKindImplementedByCSIController EdgeKind = "implemented_by_csi_controller"
	EdgeKindImplementedByCSINodeAgent  EdgeKind = "implemented_by_csi_node_agent"
	EdgeKindServedByCSINodeAgent       EdgeKind = "served_by_csi_node_agent"
	EdgeKindManagedByCSIController     EdgeKind = "managed_by_csi_controller"
	EdgeKindRelatedTo                  EdgeKind = "related_to"
)

type EdgeSourceType string

const (
	EdgeSourceTypeExplicitRef       EdgeSourceType = "explicit_ref"
	EdgeSourceTypeSelectorMatch     EdgeSourceType = "selector_match"
	EdgeSourceTypeOwnerReference    EdgeSourceType = "owner_reference"
	EdgeSourceTypeBindingResolution EdgeSourceType = "binding_resolution"
	EdgeSourceTypeInferenceRule     EdgeSourceType = "inference_rule"
	EdgeSourceTypeObserved          EdgeSourceType = "observed"
)

type EdgeState string

const (
	EdgeStateAsserted EdgeState = "asserted"
	EdgeStateInferred EdgeState = "inferred"
	EdgeStateObserved EdgeState = "observed"
)

type EntryRef struct {
	Kind        NodeKind `json:"kind"`
	CanonicalID string   `json:"canonicalId,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	Name        string   `json:"name,omitempty"`
}

type ExpansionPolicy struct {
	MaxDepth               int        `json:"maxDepth"`
	StorageMaxDepth        int        `json:"storageMaxDepth"`
	TerminalNodeKinds      []NodeKind `json:"terminalNodeKinds,omitempty"`
	ExpandTerminalNodes    bool       `json:"expandTerminalNodes,omitempty"`
	IncludeSiblingPods     bool       `json:"includeSiblingPods"`
	IncludeRBAC            bool       `json:"includeRbac"`
	IncludeEvents          bool       `json:"includeEvents"`
	IncludeWebhookEvidence bool       `json:"includeWebhookEvidence"`
	IncludeStorage         bool       `json:"includeStorage"`
	IncludeOCI             bool       `json:"includeOci"`
}

type DiagnosticNode struct {
	CanonicalID string         `json:"canonicalId"`
	Kind        NodeKind       `json:"kind"`
	SourceKind  string         `json:"sourceKind,omitempty"`
	Name        string         `json:"name,omitempty"`
	Namespace   string         `json:"namespace,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type EdgeProvenance struct {
	SourceType EdgeSourceType `json:"sourceType"`
	State      EdgeState      `json:"state"`
	Resolver   string         `json:"resolver,omitempty"`
	LastSeenAt *time.Time     `json:"lastSeenAt,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
}

type DiagnosticEdge struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Kind       EdgeKind       `json:"kind"`
	Provenance EdgeProvenance `json:"provenance"`
}

type DiagnosticSubgraph struct {
	Entry       EntryRef         `json:"entry"`
	Nodes       []DiagnosticNode `json:"nodes"`
	Edges       []DiagnosticEdge `json:"edges"`
	CollectedAt *time.Time       `json:"collectedAt,omitempty"`
	Explanation []string         `json:"explanation,omitempty"`
}

type GraphSubgraph struct {
	Entry       DiagnosticNode   `json:"entry"`
	Nodes       []DiagnosticNode `json:"nodes"`
	Edges       []DiagnosticEdge `json:"edges"`
	CollectedAt *time.Time       `json:"collectedAt,omitempty"`
	Explanation []string         `json:"explanation,omitempty"`
}
