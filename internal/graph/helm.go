package graph

import (
	"strings"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

const (
	helmManagedByLabel        = "app.kubernetes.io/managed-by"
	helmInstanceLabel         = "app.kubernetes.io/instance"
	helmChartLabel            = "helm.sh/chart"
	helmReleaseNameAnnotation = "meta.helm.sh/release-name"
	helmReleaseNSAnnotation   = "meta.helm.sh/release-namespace"
)

type helmLabeledResource struct {
	ID          model.CanonicalID
	Kind        model.NodeKind
	SourceKind  string
	Namespace   string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

func inferHelmProvenance(cluster string, resources []helmLabeledResource) ([]model.Node, []model.Edge) {
	nodes := make([]model.Node, 0)
	edges := make([]model.Edge, 0)
	seenNodes := map[model.CanonicalID]struct{}{}
	seenEdges := map[string]struct{}{}

	addNode := func(node model.Node) {
		if node.ID == "" {
			return
		}
		if _, seen := seenNodes[node.ID]; seen {
			return
		}
		seenNodes[node.ID] = struct{}{}
		nodes = append(nodes, node)
	}
	addEdge := func(edge model.Edge) {
		if edge.From == "" || edge.To == "" {
			return
		}
		if _, seen := seenEdges[edge.Key()]; seen {
			return
		}
		seenEdges[edge.Key()] = struct{}{}
		edges = append(edges, edge)
	}

	for _, resource := range resources {
		releaseName := firstNonEmpty(resource.Annotations[helmReleaseNameAnnotation], resource.Labels[helmInstanceLabel])
		if releaseName == "" {
			continue
		}
		releaseNamespace := firstNonEmpty(resource.Annotations[helmReleaseNSAnnotation], resource.Namespace)
		releaseID := helmReleaseID(cluster, releaseNamespace, releaseName)
		confidence, tier := helmLabelConfidence(resource)
		addNode(model.Node{
			ID:         releaseID,
			Kind:       model.NodeKindHelmRelease,
			SourceKind: "HelmRelease",
			Name:       releaseName,
			Namespace:  releaseNamespace,
			Attributes: map[string]any{
				"confidence":       tier,
				"evidence":         "labels",
				"releaseName":      releaseName,
				"releaseNamespace": releaseNamespace,
			},
		})
		addEdge(helmManagedByReleaseEdge(resource.ID, releaseID, confidence, tier))

		chartLabel := strings.TrimSpace(resource.Labels[helmChartLabel])
		if chartLabel == "" {
			continue
		}
		chartName, chartVersion := parseHelmChartLabel(chartLabel)
		chartID := helmChartID(cluster, chartName, chartVersion)
		addNode(model.Node{
			ID:         chartID,
			Kind:       model.NodeKindHelmChart,
			SourceKind: "HelmChart",
			Name:       chartName,
			Attributes: map[string]any{
				"chart":          chartName,
				"version":        chartVersion,
				"label":          chartLabel,
				"confidence":     tier,
				"evidence":       "labels",
				"helmReleaseRef": releaseName,
			},
		})
		addEdge(helmInstallsChartEdge(releaseID, chartID, confidence, tier))
	}

	return nodes, edges
}

func helmReleaseID(cluster, namespace, name string) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{
		Cluster:   cluster,
		Group:     "helm.sh",
		Kind:      "HelmRelease",
		Namespace: namespace,
		Name:      name,
	})
}

func helmChartID(cluster, name, version string) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{
		Cluster: cluster,
		Group:   "helm.sh",
		Kind:    "HelmChart",
		Name:    name,
		UID:     version,
	})
}

func helmManagedByReleaseEdge(resourceID, releaseID model.CanonicalID, confidence float64, tier string) model.Edge {
	return helmEvidenceEdge(resourceID, releaseID, model.EdgeKindManagedByHelmRelease, confidence, "helm-labels/v1/"+tier)
}

func helmInstallsChartEdge(releaseID, chartID model.CanonicalID, confidence float64, tier string) model.Edge {
	return helmEvidenceEdge(releaseID, chartID, model.EdgeKindInstallsChart, confidence, "helm-labels/v1/"+tier)
}

func helmEvidenceEdge(from, to model.CanonicalID, kind model.EdgeKind, confidence float64, resolver string) model.Edge {
	now := time.Now().UTC()
	return model.Edge{
		From: from,
		To:   to,
		Kind: kind,
		Provenance: model.EdgeProvenance{
			SourceType: model.EdgeSourceTypeLabelEvidence,
			State:      model.EdgeStateInferred,
			Resolver:   resolver,
			LastSeenAt: &now,
			Confidence: &confidence,
		},
	}
}

func helmLabelConfidence(resource helmLabeledResource) (float64, string) {
	managedByHelm := strings.EqualFold(strings.TrimSpace(resource.Labels[helmManagedByLabel]), "Helm")
	hasReleaseAnnotation := strings.TrimSpace(resource.Annotations[helmReleaseNameAnnotation]) != ""
	hasChart := strings.TrimSpace(resource.Labels[helmChartLabel]) != ""
	switch {
	case managedByHelm && hasReleaseAnnotation:
		return 0.9, "strong"
	case managedByHelm:
		return 0.85, "strong"
	case hasChart:
		return 0.6, "weak"
	default:
		return 0.5, "weak"
	}
}

func parseHelmChartLabel(label string) (string, string) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", ""
	}
	index := strings.LastIndex(label, "-")
	if index <= 0 || index == len(label)-1 {
		return label, ""
	}
	return label[:index], label[index+1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
