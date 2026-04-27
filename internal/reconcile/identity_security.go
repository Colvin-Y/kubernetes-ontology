package reconcile

import (
	"fmt"
	"strings"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/graph"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/explicit"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type IdentitySecurityApplyResult struct {
	Applied                     bool
	UpsertedServiceAccounts     int
	UpsertedRoleBindings        int
	UpsertedClusterRoleBindings int
	UpsertedEdges               int
	DeletedEdges                int
	DeletedNodes                int
}

type IdentitySecurityReconciler struct {
	cluster string
	kernel  *graph.Kernel
}

func NewIdentitySecurityReconciler(cluster string, kernel *graph.Kernel) *IdentitySecurityReconciler {
	return &IdentitySecurityReconciler{cluster: cluster, kernel: kernel}
}

func (r *IdentitySecurityReconciler) Apply(snapshot collectk8s.Snapshot) (IdentitySecurityApplyResult, error) {
	if r.kernel == nil {
		return IdentitySecurityApplyResult{}, fmt.Errorf("identity/security reconciler requires kernel")
	}

	currentServiceAccounts := serviceAccountIDs(r.cluster, snapshot)
	currentRoleBindings := roleBindingIDs(r.cluster, snapshot)
	currentClusterRoleBindings := clusterRoleBindingIDs(r.cluster, snapshot)
	result := IdentitySecurityApplyResult{Applied: true}
	result.DeletedEdges = r.deleteIdentitySecurityEdges()
	result.DeletedNodes = r.deleteStaleIdentitySecurityNodes(currentServiceAccounts, currentRoleBindings, currentClusterRoleBindings)

	for _, serviceAccount := range snapshot.ServiceAccounts {
		if err := r.kernel.UpsertNode(serviceAccountNode(r.cluster, serviceAccount)); err != nil {
			return result, err
		}
		result.UpsertedServiceAccounts++
	}
	for _, roleBinding := range snapshot.RoleBindings {
		if err := r.kernel.UpsertNode(roleBindingNode(r.cluster, roleBinding)); err != nil {
			return result, err
		}
		result.UpsertedRoleBindings++
	}
	for _, clusterRoleBinding := range snapshot.ClusterRoleBindings {
		if err := r.kernel.UpsertNode(clusterRoleBindingNode(r.cluster, clusterRoleBinding)); err != nil {
			return result, err
		}
		result.UpsertedClusterRoleBindings++
	}

	upsertedEdges, err := r.rebuildIdentitySecurityEdges(snapshot, currentServiceAccounts, currentRoleBindings, currentClusterRoleBindings)
	if err != nil {
		return result, err
	}
	result.UpsertedEdges = upsertedEdges
	return result, nil
}

func (r *IdentitySecurityReconciler) deleteIdentitySecurityEdges() int {
	deleted := 0
	for _, edge := range r.kernel.ListEdges() {
		if !isIdentitySecurityEdge(edge.Kind) {
			continue
		}
		_ = r.kernel.DeleteEdge(edge.Key())
		deleted++
	}
	return deleted
}

func (r *IdentitySecurityReconciler) deleteStaleIdentitySecurityNodes(currentServiceAccounts, currentRoleBindings, currentClusterRoleBindings map[string]model.CanonicalID) int {
	current := make(map[model.CanonicalID]struct{}, len(currentServiceAccounts)+len(currentRoleBindings)+len(currentClusterRoleBindings))
	for _, id := range currentServiceAccounts {
		current[id] = struct{}{}
	}
	for _, id := range currentRoleBindings {
		current[id] = struct{}{}
	}
	for _, id := range currentClusterRoleBindings {
		current[id] = struct{}{}
	}

	deleted := 0
	for _, node := range r.kernel.ListNodes() {
		if node.Kind != model.NodeKindServiceAccount && node.Kind != model.NodeKindRoleBinding && node.Kind != model.NodeKindClusterRoleBinding {
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

func (r *IdentitySecurityReconciler) rebuildIdentitySecurityEdges(snapshot collectk8s.Snapshot, currentServiceAccounts, currentRoleBindings, currentClusterRoleBindings map[string]model.CanonicalID) (int, error) {
	upserted := 0
	for _, pod := range snapshot.Pods {
		if pod.ServiceAccount == "" {
			continue
		}
		accountID, ok := currentServiceAccounts[pod.Metadata.Namespace+"/"+pod.ServiceAccount]
		if !ok {
			continue
		}
		if err := r.kernel.UpsertEdge(explicit.PodUsesServiceAccount(podID(r.cluster, pod), accountID)); err != nil {
			return upserted, err
		}
		upserted++
	}

	for _, roleBinding := range snapshot.RoleBindings {
		bindingID, ok := currentRoleBindings[roleBinding.Metadata.Namespace+"/"+roleBinding.Metadata.Name]
		if !ok {
			continue
		}
		for index, kind := range roleBinding.SubjectKinds {
			if !isServiceAccountSubject(kind) {
				continue
			}
			namespace := roleBindingSubjectNamespace(roleBinding, index)
			name := subjectValue(roleBinding.SubjectNames, index)
			if namespace == "" || name == "" {
				continue
			}
			accountID, ok := currentServiceAccounts[namespace+"/"+name]
			if !ok {
				continue
			}
			if err := r.kernel.UpsertEdge(infer.SubjectBoundByRoleBinding(accountID, bindingID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}

	for _, clusterRoleBinding := range snapshot.ClusterRoleBindings {
		bindingID, ok := currentClusterRoleBindings[clusterRoleBinding.Metadata.Name]
		if !ok {
			continue
		}
		for index, kind := range clusterRoleBinding.SubjectKinds {
			if !isServiceAccountSubject(kind) {
				continue
			}
			namespace := clusterRoleBindingSubjectNamespace(clusterRoleBinding, index)
			name := subjectValue(clusterRoleBinding.SubjectNames, index)
			if namespace == "" || name == "" {
				continue
			}
			accountID, ok := currentServiceAccounts[namespace+"/"+name]
			if !ok {
				continue
			}
			if err := r.kernel.UpsertEdge(infer.SubjectBoundByRoleBinding(accountID, bindingID)); err != nil {
				return upserted, err
			}
			upserted++
		}
	}

	return upserted, nil
}

func isIdentitySecurityEdge(kind model.EdgeKind) bool {
	return kind == model.EdgeKindUsesServiceAccount || kind == model.EdgeKindBoundByRoleBinding
}

func serviceAccountNode(cluster string, serviceAccount resources.ServiceAccount) model.Node {
	return model.Node{
		ID:         model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "core", Kind: "ServiceAccount", Namespace: serviceAccount.Metadata.Namespace, Name: serviceAccount.Metadata.Name, UID: serviceAccount.Metadata.UID}),
		Kind:       model.NodeKindServiceAccount,
		SourceKind: "ServiceAccount",
		Name:       serviceAccount.Metadata.Name,
		Namespace:  serviceAccount.Metadata.Namespace,
	}
}

func roleBindingIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.RoleBindings))
	for _, roleBinding := range snapshot.RoleBindings {
		out[roleBinding.Metadata.Namespace+"/"+roleBinding.Metadata.Name] = roleBindingID(cluster, roleBinding)
	}
	return out
}

func clusterRoleBindingIDs(cluster string, snapshot collectk8s.Snapshot) map[string]model.CanonicalID {
	out := make(map[string]model.CanonicalID, len(snapshot.ClusterRoleBindings))
	for _, clusterRoleBinding := range snapshot.ClusterRoleBindings {
		out[clusterRoleBinding.Metadata.Name] = clusterRoleBindingID(cluster, clusterRoleBinding)
	}
	return out
}

func roleBindingID(cluster string, roleBinding resources.RoleBinding) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "rbac.authorization.k8s.io", Kind: "RoleBinding", Namespace: roleBinding.Metadata.Namespace, Name: roleBinding.Metadata.Name, UID: roleBinding.Metadata.UID})
}

func clusterRoleBindingID(cluster string, clusterRoleBinding resources.ClusterRoleBinding) model.CanonicalID {
	return model.NewCanonicalID(model.ResourceRef{Cluster: cluster, Group: "rbac.authorization.k8s.io", Kind: "ClusterRoleBinding", Name: clusterRoleBinding.Metadata.Name, UID: clusterRoleBinding.Metadata.UID})
}

func roleBindingNode(cluster string, roleBinding resources.RoleBinding) model.Node {
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

func clusterRoleBindingNode(cluster string, clusterRoleBinding resources.ClusterRoleBinding) model.Node {
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

func roleBindingSubjectNamespace(roleBinding resources.RoleBinding, index int) string {
	namespace := subjectValue(roleBinding.SubjectNamespaces, index)
	if namespace != "" {
		return namespace
	}
	return roleBinding.Metadata.Namespace
}

func clusterRoleBindingSubjectNamespace(clusterRoleBinding resources.ClusterRoleBinding, index int) string {
	return subjectValue(clusterRoleBinding.SubjectNamespaces, index)
}

func subjectValue(subjects []string, index int) string {
	if index < 0 || index >= len(subjects) {
		return ""
	}
	return strings.TrimSpace(subjects[index])
}
