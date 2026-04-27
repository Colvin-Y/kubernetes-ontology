package reconcile

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestIdentitySecurityReconcilerMatchesFullRebuildIdentityFacts(t *testing.T) {
	initial := identitySecuritySnapshot("app", false)
	initial.ServiceAccounts = initial.ServiceAccounts[:1]
	next := identitySecuritySnapshot("builder", true)
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewIdentitySecurityReconciler("cluster-a", kernel).Apply(next)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected identity/security update to be applied")
	}
	if result.UpsertedServiceAccounts != 2 || result.UpsertedRoleBindings != 1 || result.UpsertedClusterRoleBindings != 1 {
		t.Fatalf("expected identity nodes to be upserted, got sa=%d rb=%d crb=%d", result.UpsertedServiceAccounts, result.UpsertedRoleBindings, result.UpsertedClusterRoleBindings)
	}
	if result.UpsertedEdges != 3 {
		t.Fatalf("expected pod serviceaccount and RBAC binding edges, got %d", result.UpsertedEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	gotEdges := identitySecurityEdgeKeys(kernel.ListEdges())
	wantEdges := identitySecurityEdgeKeys(full.ListEdges())
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Fatalf("identity/security edges mismatch\ngot  %#v\nwant %#v", gotEdges, wantEdges)
	}
	gotNodes := identitySecurityNodeFingerprints(kernel.ListNodes())
	wantNodes := identitySecurityNodeFingerprints(full.ListNodes())
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("identity/security nodes mismatch\ngot  %#v\nwant %#v", gotNodes, wantNodes)
	}
}

func TestIdentitySecurityReconcilerDeletesMissingIdentityResources(t *testing.T) {
	initial := identitySecuritySnapshot("builder", true)
	next := collectk8s.Snapshot{
		Pods: []resources.Pod{{
			Metadata:       resources.Metadata{UID: "pod-uid", Name: "app-0", Namespace: "default"},
			ServiceAccount: "builder",
		}},
	}
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewIdentitySecurityReconciler("cluster-a", kernel).Apply(next)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedNodes != 4 {
		t.Fatalf("expected two serviceaccounts and two bindings to be deleted, got %d", result.DeletedNodes)
	}
	if len(identitySecurityEdgeKeys(kernel.ListEdges())) != 0 {
		t.Fatal("expected identity/security edges to be removed")
	}
	if len(identitySecurityNodeFingerprints(kernel.ListNodes())) != 0 {
		t.Fatal("expected identity/security nodes to be removed")
	}
}

func identitySecuritySnapshot(podServiceAccount string, includeClusterBinding bool) collectk8s.Snapshot {
	snapshot := collectk8s.Snapshot{
		Pods: []resources.Pod{{
			Metadata:       resources.Metadata{UID: "pod-uid", Name: "app-0", Namespace: "default"},
			ServiceAccount: podServiceAccount,
		}},
		ServiceAccounts: []resources.ServiceAccount{
			{Metadata: resources.Metadata{UID: "sa-app", Name: "app", Namespace: "default"}},
			{Metadata: resources.Metadata{UID: "sa-builder", Name: "builder", Namespace: "default"}},
		},
		RoleBindings: []resources.RoleBinding{{
			Metadata:     resources.Metadata{UID: "rb-uid", Name: "app-reader", Namespace: "default"},
			RoleRef:      "reader",
			SubjectKinds: []string{"ServiceAccount"},
			SubjectNames: []string{podServiceAccount},
		}},
	}
	if includeClusterBinding {
		snapshot.ClusterRoleBindings = []resources.ClusterRoleBinding{{
			Metadata:          resources.Metadata{UID: "crb-uid", Name: "builder-reader"},
			RoleRef:           "cluster-reader",
			SubjectKinds:      []string{"ServiceAccount"},
			SubjectNames:      []string{"builder"},
			SubjectNamespaces: []string{"default"},
		}}
	}
	return snapshot
}

func identitySecurityEdgeKeys(edges []model.Edge) []string {
	out := make([]string, 0)
	for _, edge := range edges {
		if isIdentitySecurityEdge(edge.Kind) {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}

func identitySecurityNodeFingerprints(nodes []model.Node) []string {
	out := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != model.NodeKindServiceAccount && node.Kind != model.NodeKindRoleBinding && node.Kind != model.NodeKindClusterRoleBinding {
			continue
		}
		out = append(out, fmt.Sprintf("%s|%s|%s|%s|%v|%v|%v|%v", node.ID, node.Kind, node.Namespace, node.Name, node.Attributes["roleRef"], node.Attributes["subjectKinds"], node.Attributes["subjectNames"], node.Attributes["subjectNamespaces"]))
	}
	sort.Strings(out)
	return out
}
