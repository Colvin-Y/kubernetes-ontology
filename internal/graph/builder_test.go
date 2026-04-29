package graph

import (
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

func TestBuilderBuild(t *testing.T) {
	builder := NewBuilder("cluster-a")
	snapshot := k8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"},
			ControllerKind: "Deployment",
			Replicas:       1,
		}},
		Pods: []resources.Pod{{
			Metadata:        resources.Metadata{UID: "p1", Name: "frontend-abc123", Namespace: "default", Labels: map[string]string{"app": "frontend"}},
			NodeName:        "node-a",
			OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "w1"}},
			ContainerImages: []string{"registry.example.com/frontend:v1@sha256:deadbeef"},
			ConfigMapRefs:   []string{"frontend-config"},
			Phase:           "Running",
		}},
		Nodes:      []resources.Node{{Metadata: resources.Metadata{UID: "n1", Name: "node-a"}}},
		Services:   []resources.Service{{Metadata: resources.Metadata{UID: "s1", Name: "frontend-svc", Namespace: "default"}, Selector: map[string]string{"app": "frontend"}}},
		ConfigMaps: []resources.ConfigMap{{Metadata: resources.Metadata{UID: "c1", Name: "frontend-config", Namespace: "default"}}},
	}

	nodes, edges := builder.Build(snapshot)
	if len(nodes) == 0 {
		t.Fatal("expected nodes to be built")
	}
	if len(edges) == 0 {
		t.Fatal("expected edges to be built")
	}
}

func TestBuilderBuildsConfiguredWorkloadOwnerChain(t *testing.T) {
	builder := NewBuilder("cluster-a")
	snapshot := k8s.Snapshot{
		Workloads: []resources.Workload{
			{
				Metadata:       resources.Metadata{UID: "redis-uid", Name: "redis-cq01mg647b8wj86qz", Namespace: "redis"},
				APIVersion:     "db.example.com/v1",
				ControllerKind: "RedisCluster",
			},
			{
				Metadata:       resources.Metadata{UID: "asts-uid", Name: "proxy-redis-cq01mg647b8wj86qz", Namespace: "redis"},
				APIVersion:     "apps.kruise.io/v1alpha1",
				ControllerKind: "AdvancedStatefulSet",
				OwnerReferences: []resources.OwnerReference{{
					APIVersion: "db.example.com/v1",
					Kind:       "RedisCluster",
					Name:       "redis-cq01mg647b8wj86qz",
					UID:        "redis-uid",
					Controller: true,
				}},
			},
		},
		Pods: []resources.Pod{{
			Metadata: resources.Metadata{UID: "pod-uid", Name: "proxy-redis-cq01mg647b8wj86qz-0", Namespace: "redis"},
			OwnerReferences: []resources.OwnerReference{{
				APIVersion: "apps.kruise.io/v1alpha1",
				Kind:       "AdvancedStatefulSet",
				Name:       "proxy-redis-cq01mg647b8wj86qz",
				UID:        "asts-uid",
				Controller: true,
			}},
		}},
	}

	nodes, edges := builder.Build(snapshot)
	if !hasWorkloadNode(nodes, "AdvancedStatefulSet", "proxy-redis-cq01mg647b8wj86qz") {
		t.Fatal("expected AdvancedStatefulSet workload node")
	}
	if !hasWorkloadNode(nodes, "RedisCluster", "redis-cq01mg647b8wj86qz") {
		t.Fatal("expected RedisCluster workload node")
	}
	kinds := map[model.EdgeKind]int{}
	for _, edge := range edges {
		kinds[edge.Kind]++
	}
	if kinds[model.EdgeKindManagedBy] != 2 {
		t.Fatalf("expected pod to resolve to direct and ancestor workloads, got %d managed_by edges", kinds[model.EdgeKindManagedBy])
	}
	if kinds[model.EdgeKindControlledBy] != 1 {
		t.Fatalf("expected workload controlled_by owner edge, got %d", kinds[model.EdgeKindControlledBy])
	}
}

func TestBuilderBuildsConfiguredControllerRuleEdges(t *testing.T) {
	builder := NewBuilder("cluster-a")
	builder.SetWorkloadControllerRules([]infer.WorkloadControllerRule{{
		APIVersion:            "apps.kruise.io/*",
		Kind:                  "*",
		ControllerNamespace:   "kruise-system",
		ControllerPodPrefixes: []string{"kruise-controller-manager"},
		NodeDaemonPodPrefixes: []string{"kruise-daemon"},
	}})
	snapshot := k8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: "asts-uid", Name: "proxy-redis-cq01mg647b8wj86qz", Namespace: "redis"},
			APIVersion:     "apps.kruise.io/v1alpha1",
			ControllerKind: "AdvancedStatefulSet",
		}},
		Pods: []resources.Pod{
			{
				Metadata: resources.Metadata{UID: "pod-uid", Name: "proxy-redis-cq01mg647b8wj86qz-0", Namespace: "redis"},
				NodeName: "node-a",
				OwnerReferences: []resources.OwnerReference{{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "AdvancedStatefulSet",
					Name:       "proxy-redis-cq01mg647b8wj86qz",
					UID:        "asts-uid",
					Controller: true,
				}},
			},
			{
				Metadata: resources.Metadata{UID: "manager-uid", Name: "kruise-controller-manager-7f9c", Namespace: "kruise-system"},
			},
			{
				Metadata: resources.Metadata{UID: "daemon-a-uid", Name: "kruise-daemon-node-a", Namespace: "kruise-system"},
				NodeName: "node-a",
			},
			{
				Metadata: resources.Metadata{UID: "daemon-b-uid", Name: "kruise-daemon-node-b", Namespace: "kruise-system"},
				NodeName: "node-b",
			},
		},
	}

	_, edges := builder.Build(snapshot)
	kinds := map[model.EdgeKind]int{}
	for _, edge := range edges {
		kinds[edge.Kind]++
	}
	if kinds[model.EdgeKindManagedByController] != 1 {
		t.Fatalf("expected one controller manager edge, got %d", kinds[model.EdgeKindManagedByController])
	}
	if kinds[model.EdgeKindServedByNodeDaemon] != 1 {
		t.Fatalf("expected one same-node daemon edge, got %d", kinds[model.EdgeKindServedByNodeDaemon])
	}
}

func TestBuilderBuildsRBACTopology(t *testing.T) {
	builder := NewBuilder("cluster-a")
	snapshot := k8s.Snapshot{
		ServiceAccounts: []resources.ServiceAccount{
			{Metadata: resources.Metadata{UID: "sa-app", Name: "app", Namespace: "default"}},
			{Metadata: resources.Metadata{UID: "sa-bot", Name: "bot", Namespace: "ci"}},
		},
		RoleBindings: []resources.RoleBinding{{
			Metadata:     resources.Metadata{UID: "rb-app", Name: "app-reader", Namespace: "default"},
			RoleRef:      "reader",
			SubjectKinds: []string{"ServiceAccount", "User"},
			SubjectNames: []string{"app", "jane"},
		}},
		ClusterRoleBindings: []resources.ClusterRoleBinding{{
			Metadata:          resources.Metadata{UID: "crb-bot", Name: "bot-reader"},
			RoleRef:           "cluster-reader",
			SubjectKinds:      []string{"ServiceAccount"},
			SubjectNames:      []string{"bot"},
			SubjectNamespaces: []string{"ci"},
		}},
	}

	nodes, edges := builder.Build(snapshot)
	if !hasNode(nodes, model.NodeKindRoleBinding, "default", "app-reader") {
		t.Fatal("expected rolebinding node")
	}
	if !hasNode(nodes, model.NodeKindClusterRoleBinding, "", "bot-reader") {
		t.Fatal("expected clusterrolebinding node")
	}

	appAccountID := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "ServiceAccount", Namespace: "default", Name: "app", UID: "sa-app"})
	botAccountID := model.NewCanonicalID(model.ResourceRef{Cluster: "cluster-a", Group: "core", Kind: "ServiceAccount", Namespace: "ci", Name: "bot", UID: "sa-bot"})
	appBindingID := roleBindingID("cluster-a", snapshot.RoleBindings[0])
	botBindingID := clusterRoleBindingID("cluster-a", snapshot.ClusterRoleBindings[0])
	if !hasRBACBindingEdge(t, edges, appAccountID, appBindingID) {
		t.Fatal("expected serviceaccount to rolebinding edge")
	}
	if !hasRBACBindingEdge(t, edges, botAccountID, botBindingID) {
		t.Fatal("expected serviceaccount to clusterrolebinding edge")
	}
	if countEdges(edges, model.EdgeKindBoundByRoleBinding) != 2 {
		t.Fatalf("expected only serviceaccount subject binding edges, got %d", countEdges(edges, model.EdgeKindBoundByRoleBinding))
	}
}

func TestBuilderInfersHelmReleaseAndChartFromLabels(t *testing.T) {
	builder := NewBuilder("cluster-a")
	helmLabels := map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   "checkout",
		"helm.sh/chart":                "checkout-api-1.2.3",
	}
	helmAnnotations := map[string]string{
		"meta.helm.sh/release-name":      "checkout",
		"meta.helm.sh/release-namespace": "payments",
	}
	snapshot := k8s.Snapshot{
		Workloads: []resources.Workload{{
			Metadata:       resources.Metadata{UID: "deploy-uid", Name: "checkout-api", Namespace: "payments", Labels: helmLabels, Annotations: helmAnnotations},
			APIVersion:     "apps/v1",
			ControllerKind: "Deployment",
		}},
		Services: []resources.Service{{
			Metadata: resources.Metadata{UID: "svc-uid", Name: "checkout-api", Namespace: "payments", Labels: helmLabels, Annotations: helmAnnotations},
			Selector: map[string]string{"app": "checkout"},
		}},
	}

	nodes, edges := builder.Build(snapshot)
	if !hasNode(nodes, model.NodeKindHelmRelease, "payments", "checkout") {
		t.Fatal("expected HelmRelease node")
	}
	if !hasNode(nodes, model.NodeKindHelmChart, "", "checkout-api") {
		t.Fatal("expected HelmChart node")
	}
	if countEdges(edges, model.EdgeKindManagedByHelmRelease) != 2 {
		t.Fatalf("expected workload and service Helm release edges, got %d", countEdges(edges, model.EdgeKindManagedByHelmRelease))
	}
	if countEdges(edges, model.EdgeKindInstallsChart) != 1 {
		t.Fatalf("expected deduplicated release-to-chart edge, got %d", countEdges(edges, model.EdgeKindInstallsChart))
	}
	for _, edge := range edges {
		if edge.Kind != model.EdgeKindManagedByHelmRelease {
			continue
		}
		if edge.Provenance.SourceType != model.EdgeSourceTypeLabelEvidence {
			t.Fatalf("expected label evidence provenance, got %s", edge.Provenance.SourceType)
		}
		if edge.Provenance.Confidence == nil || *edge.Provenance.Confidence < 0.8 {
			t.Fatalf("expected strong confidence, got %#v", edge.Provenance.Confidence)
		}
	}
}

func hasWorkloadNode(nodes []model.Node, sourceKind, name string) bool {
	for _, node := range nodes {
		if node.Kind == model.NodeKindWorkload && node.SourceKind == sourceKind && node.Name == name {
			return true
		}
	}
	return false
}

func hasNode(nodes []model.Node, kind model.NodeKind, namespace, name string) bool {
	for _, node := range nodes {
		if node.Kind == kind && node.Namespace == namespace && node.Name == name {
			return true
		}
	}
	return false
}

func hasRBACBindingEdge(t *testing.T, edges []model.Edge, from, to model.CanonicalID) bool {
	t.Helper()
	for _, edge := range edges {
		if edge.From != from || edge.To != to || edge.Kind != model.EdgeKindBoundByRoleBinding {
			continue
		}
		if edge.Provenance.SourceType != model.EdgeSourceTypeBindingResolution {
			t.Fatalf("expected binding_resolution provenance, got %s", edge.Provenance.SourceType)
		}
		if edge.Provenance.State != model.EdgeStateInferred {
			t.Fatalf("expected inferred RBAC edge, got %s", edge.Provenance.State)
		}
		if edge.Provenance.Resolver != "rbac-binding/v1" {
			t.Fatalf("expected rbac-binding/v1 resolver, got %s", edge.Provenance.Resolver)
		}
		return true
	}
	return false
}

func countEdges(edges []model.Edge, kind model.EdgeKind) int {
	count := 0
	for _, edge := range edges {
		if edge.Kind == kind {
			count++
		}
	}
	return count
}
