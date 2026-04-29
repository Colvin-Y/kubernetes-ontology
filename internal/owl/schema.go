package owl

import (
	"fmt"
	"io"
	"strings"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

const (
	OntologyIRI = "https://kubernetes-ontology.local/ontology"
	xsdIRI      = "http://www.w3.org/2001/XMLSchema#"
)

type ClassSpec struct {
	ID       string
	Label    string
	Comment  string
	Parents  []string
	NodeKind bool
}

type ObjectPropertySpec = model.RelationSpec

type DataPropertySpec struct {
	ID      string
	Label   string
	Comment string
	Domain  string
	Range   string
}

func Classes() []ClassSpec {
	return append([]ClassSpec(nil), classSpecs...)
}

func ObjectProperties() []ObjectPropertySpec {
	return model.RelationSpecs()
}

func DataProperties() []DataPropertySpec {
	return append([]DataPropertySpec(nil), dataPropertySpecs...)
}

func WriteStaticOntology(w io.Writer) error {
	writer := rdfWriter{w: w}
	writer.line(`<?xml version="1.0" encoding="UTF-8"?>`)
	writer.line(`<rdf:RDF`)
	writer.line(`    xmlns="` + OntologyIRI + `#"`)
	writer.line(`    xml:base="` + OntologyIRI + `"`)
	writer.line(`    xmlns:ko="` + OntologyIRI + `#"`)
	writer.line(`    xmlns:owl="http://www.w3.org/2002/07/owl#"`)
	writer.line(`    xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"`)
	writer.line(`    xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"`)
	writer.line(`    xmlns:xsd="` + xsdIRI + `">`)
	writer.line(`  <owl:Ontology rdf:about="` + OntologyIRI + `">`)
	writer.textElement(4, "rdfs:label", "kubernetes-ontology")
	writer.textElement(4, "rdfs:comment", "Static OWL schema generated from the kubernetes-ontology in-memory graph model. Runtime instances use canonicalId for identity and EdgeProvenance for evidence.")
	writer.line(`  </owl:Ontology>`)
	writer.blank()

	for _, property := range annotationProperties {
		writer.line(`  <owl:AnnotationProperty rdf:about="` + localRef(property) + `"/>`)
	}
	writer.blank()

	for _, class := range classSpecs {
		writer.line(`  <owl:Class rdf:about="` + localRef(class.ID) + `">`)
		writer.textElement(4, "rdfs:label", labelFor(class.Label, class.ID))
		writer.textElement(4, "rdfs:comment", class.Comment)
		for _, parent := range class.Parents {
			writer.line(`    <rdfs:subClassOf rdf:resource="` + localRef(parent) + `"/>`)
		}
		if class.NodeKind {
			writer.textElement(4, "ko:nodeKind", class.ID)
		}
		writer.line(`  </owl:Class>`)
	}
	writer.blank()

	for _, property := range ObjectProperties() {
		id := string(property.Kind)
		writer.line(`  <owl:ObjectProperty rdf:about="` + localRef(id) + `">`)
		writer.textElement(4, "rdfs:label", id)
		writer.textElement(4, "rdfs:comment", property.Comment)
		writer.textElement(4, "ko:edgeKind", id)
		if property.Domain != "" {
			writer.line(`    <rdfs:domain rdf:resource="` + localRef(property.Domain) + `"/>`)
		}
		if property.Range != "" {
			writer.line(`    <rdfs:range rdf:resource="` + localRef(property.Range) + `"/>`)
		}
		if property.InverseOf != "" {
			writer.line(`    <owl:inverseOf rdf:resource="` + localRef(string(property.InverseOf)) + `"/>`)
		}
		if property.DefaultSourceType != "" {
			writer.textElement(4, "ko:defaultSourceType", string(property.DefaultSourceType))
		}
		if property.DefaultState != "" {
			writer.textElement(4, "ko:defaultState", string(property.DefaultState))
		}
		for _, resolver := range property.ResolverHints {
			writer.textElement(4, "ko:resolverHint", resolver)
		}
		writer.line(`  </owl:ObjectProperty>`)
	}
	writer.blank()

	for _, property := range dataPropertySpecs {
		writer.line(`  <owl:DatatypeProperty rdf:about="` + localRef(property.ID) + `">`)
		writer.textElement(4, "rdfs:label", labelFor(property.Label, property.ID))
		writer.textElement(4, "rdfs:comment", property.Comment)
		if property.Domain != "" {
			writer.line(`    <rdfs:domain rdf:resource="` + localRef(property.Domain) + `"/>`)
		}
		if property.Range != "" {
			writer.line(`    <rdfs:range rdf:resource="` + datatypeRef(property.Range) + `"/>`)
		}
		writer.line(`  </owl:DatatypeProperty>`)
	}

	writer.line(`</rdf:RDF>`)
	return writer.err
}

type rdfWriter struct {
	w   io.Writer
	err error
}

func (w *rdfWriter) blank() {
	w.line("")
}

func (w *rdfWriter) line(line string) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintln(w.w, line)
}

func (w *rdfWriter) textElement(indent int, name, value string) {
	if value == "" {
		return
	}
	padding := strings.Repeat(" ", indent)
	w.line(padding + "<" + name + ">" + xmlEscape(value) + "</" + name + ">")
}

func localRef(id string) string {
	return "#" + id
}

func datatypeRef(id string) string {
	return xsdIRI + id
}

func labelFor(label, fallback string) string {
	if label != "" {
		return label
	}
	return fallback
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

var annotationProperties = []string{
	"nodeKind",
	"edgeKind",
	"defaultSourceType",
	"defaultState",
	"resolverHint",
}

var classSpecs = []ClassSpec{
	{ID: "Entity", Comment: "Any addressable ontology item in the local semantic graph."},
	{ID: "KubernetesResource", Comment: "A Kubernetes API object or a normalized Kubernetes-derived resource.", Parents: []string{"Entity"}},
	{ID: "NamespacedResource", Comment: "A Kubernetes resource scoped to a namespace.", Parents: []string{"KubernetesResource"}},
	{ID: "ClusterScopedResource", Comment: "A Kubernetes resource without namespace scope.", Parents: []string{"KubernetesResource"}},
	{ID: "StorageClassConsumer", Comment: "A resource that may reference a Kubernetes StorageClass.", Parents: []string{"KubernetesResource"}},
	{ID: "RBACBinding", Comment: "A RoleBinding or ClusterRoleBinding that binds subjects to a role reference.", Parents: []string{"KubernetesResource"}},
	{ID: "Package", Comment: "A packaging or release-management entity inferred from Kubernetes metadata.", Parents: []string{"Entity"}},
	{ID: "RelationAssertion", Comment: "A reified edge assertion used when runtime materialization needs edge provenance.", Parents: []string{"Entity"}},
	{ID: string(model.NodeKindCluster), Comment: "A Kubernetes cluster identity used by canonical IDs and graph context.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
	{ID: string(model.NodeKindNamespace), Comment: "A Kubernetes namespace.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
	{ID: string(model.NodeKindWorkload), Comment: "A normalized workload controller such as Deployment, StatefulSet, DaemonSet, Job, or configured custom workload.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindPod), Comment: "A Kubernetes Pod. Current graph logic attaches scheduling, owner, runtime image, service account, config, secret, PVC, and controller evidence to Pods.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindContainer), Comment: "A container within a Pod. The current builder keeps image use on Pod-level edges but reserves this class for container-level expansion.", Parents: []string{"Entity"}, NodeKind: true},
	{ID: string(model.NodeKindNode), Comment: "A Kubernetes Node.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
	{ID: string(model.NodeKindService), Comment: "A Kubernetes Service with selector-based Pod recovery.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindConfigMap), Comment: "A Kubernetes ConfigMap referenced by Pods.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindSecret), Comment: "A Kubernetes Secret referenced by Pods.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindServiceAccount), Comment: "A Kubernetes ServiceAccount used by Pods and bound by RBAC bindings.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindRoleBinding), Comment: "A Kubernetes RoleBinding.", Parents: []string{"NamespacedResource", "RBACBinding"}, NodeKind: true},
	{ID: string(model.NodeKindClusterRoleBinding), Comment: "A Kubernetes ClusterRoleBinding.", Parents: []string{"ClusterScopedResource", "RBACBinding"}, NodeKind: true},
	{ID: string(model.NodeKindPVC), Comment: "A Kubernetes PersistentVolumeClaim.", Parents: []string{"NamespacedResource", "StorageClassConsumer"}, NodeKind: true},
	{ID: string(model.NodeKindPV), Comment: "A Kubernetes PersistentVolume.", Parents: []string{"ClusterScopedResource", "StorageClassConsumer"}, NodeKind: true},
	{ID: string(model.NodeKindStorageClass), Comment: "A Kubernetes StorageClass.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
	{ID: string(model.NodeKindCSIDriver), Comment: "A Kubernetes CSIDriver or inferred CSI-shaped provisioner.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
	{ID: string(model.NodeKindHelmRelease), Comment: "A Helm release inferred from standard Helm labels and annotations on Kubernetes resources.", Parents: []string{"Package", "NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindHelmChart), Comment: "A Helm chart inferred from helm.sh/chart labels.", Parents: []string{"Package"}, NodeKind: true},
	{ID: string(model.NodeKindEvent), Comment: "A Kubernetes Event used as observed diagnostic evidence.", Parents: []string{"NamespacedResource"}, NodeKind: true},
	{ID: string(model.NodeKindImage), Comment: "A container image reference parsed from Pod container specs.", Parents: []string{"Entity"}, NodeKind: true},
	{ID: string(model.NodeKindOCIArtifactMetadata), Comment: "OCI artifact metadata associated with an image.", Parents: []string{"Entity"}, NodeKind: true},
	{ID: string(model.NodeKindWebhookConfig), Comment: "A MutatingWebhookConfiguration or ValidatingWebhookConfiguration used as admission evidence.", Parents: []string{"ClusterScopedResource"}, NodeKind: true},
}

var dataPropertySpecs = []DataPropertySpec{
	{ID: "canonicalId", Comment: "Stable identity key used by ontology queries and graph joins.", Domain: "Entity", Range: "string"},
	{ID: "kind", Comment: "Normalized node kind.", Domain: "Entity", Range: "string"},
	{ID: "sourceKind", Comment: "Original Kubernetes source kind when available.", Domain: "Entity", Range: "string"},
	{ID: "name", Comment: "Kubernetes object name or normalized entity name.", Domain: "Entity", Range: "string"},
	{ID: "namespace", Comment: "Kubernetes namespace for namespaced resources.", Domain: "NamespacedResource", Range: "string"},
	{ID: "apiVersion", Comment: "Kubernetes API version captured for workload resources.", Domain: string(model.NodeKindWorkload), Range: "string"},
	{ID: "replicas", Comment: "Replica count captured for workload resources.", Domain: string(model.NodeKindWorkload), Range: "integer"},
	{ID: "conditions", Comment: "Serialized Kubernetes conditions map.", Domain: "KubernetesResource", Range: "string"},
	{ID: "phase", Comment: "Pod phase captured from pod status.", Domain: string(model.NodeKindPod), Range: "string"},
	{ID: "reason", Comment: "Pod status reason or Event reason.", Domain: "KubernetesResource", Range: "string"},
	{ID: "nodeName", Comment: "Node name assigned to a Pod.", Domain: string(model.NodeKindPod), Range: "string"},
	{ID: "selector", Comment: "Serialized Service selector map.", Domain: string(model.NodeKindService), Range: "string"},
	{ID: "status", Comment: "Status string captured for persistent storage resources.", Domain: "StorageClassConsumer", Range: "string"},
	{ID: "storageClassName", Comment: "StorageClass name referenced by a PVC or PV.", Domain: "StorageClassConsumer", Range: "string"},
	{ID: "provisioner", Comment: "StorageClass provisioner string.", Domain: string(model.NodeKindStorageClass), Range: "string"},
	{ID: "reclaimPolicy", Comment: "StorageClass reclaim policy.", Domain: string(model.NodeKindStorageClass), Range: "string"},
	{ID: "volumeBindingMode", Comment: "StorageClass volume binding mode.", Domain: string(model.NodeKindStorageClass), Range: "string"},
	{ID: "csi", Comment: "Serialized PersistentVolume CSI metadata map.", Domain: string(model.NodeKindPV), Range: "string"},
	{ID: "roleRef", Comment: "Role reference name on an RBAC binding.", Domain: "RBACBinding", Range: "string"},
	{ID: "subjectKinds", Comment: "Serialized subject kind list on an RBAC binding.", Domain: "RBACBinding", Range: "string"},
	{ID: "subjectNames", Comment: "Serialized subject name list on an RBAC binding.", Domain: "RBACBinding", Range: "string"},
	{ID: "subjectNamespaces", Comment: "Serialized subject namespace list on an RBAC binding.", Domain: "RBACBinding", Range: "string"},
	{ID: "message", Comment: "Event message used as diagnostic evidence.", Domain: string(model.NodeKindEvent), Range: "string"},
	{ID: "repo", Comment: "Parsed image repository.", Domain: string(model.NodeKindImage), Range: "string"},
	{ID: "tag", Comment: "Parsed image tag.", Domain: string(model.NodeKindImage), Range: "string"},
	{ID: "digest", Comment: "Parsed image digest.", Domain: string(model.NodeKindImage), Range: "string"},
	{ID: "containerIndex", Comment: "Container index from which the image reference was collected.", Domain: string(model.NodeKindImage), Range: "integer"},
	{ID: "inferredFromStorageClass", Comment: "True when a CSIDriver node is synthesized from a CSI-shaped StorageClass provisioner.", Domain: string(model.NodeKindCSIDriver), Range: "boolean"},
	{ID: "chart", Comment: "Helm chart name inferred from helm.sh/chart labels.", Domain: string(model.NodeKindHelmChart), Range: "string"},
	{ID: "version", Comment: "Helm chart version inferred from helm.sh/chart labels.", Domain: string(model.NodeKindHelmChart), Range: "string"},
	{ID: "releaseName", Comment: "Helm release name inferred from labels or annotations.", Domain: string(model.NodeKindHelmRelease), Range: "string"},
	{ID: "releaseNamespace", Comment: "Helm release namespace inferred from annotations or resource namespace.", Domain: string(model.NodeKindHelmRelease), Range: "string"},
	{ID: "relationKind", Comment: "Edge kind on a reified RelationAssertion.", Domain: "RelationAssertion", Range: "string"},
	{ID: "fromCanonicalId", Comment: "Source canonicalId on a reified RelationAssertion.", Domain: "RelationAssertion", Range: "string"},
	{ID: "toCanonicalId", Comment: "Target canonicalId on a reified RelationAssertion.", Domain: "RelationAssertion", Range: "string"},
	{ID: "sourceType", Comment: "Edge provenance source type: explicit_ref, selector_match, owner_reference, binding_resolution, inference_rule, observed, or label_evidence.", Domain: "RelationAssertion", Range: "string"},
	{ID: "state", Comment: "Edge provenance state: asserted, inferred, or observed.", Domain: "RelationAssertion", Range: "string"},
	{ID: "resolver", Comment: "Resolver identifier that produced the relation.", Domain: "RelationAssertion", Range: "string"},
	{ID: "lastSeenAt", Comment: "Timestamp when the relation was last observed or generated.", Domain: "RelationAssertion", Range: "dateTime"},
	{ID: "confidence", Comment: "Optional confidence value on inferred relations.", Domain: "RelationAssertion", Range: "decimal"},
}
