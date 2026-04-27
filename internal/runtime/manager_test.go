package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	collectk8sresources "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/query"
)

func TestManagerStart(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "w1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123", Namespace: "default", UID: "p1", Labels: map[string]string{"app": "frontend"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "n1"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "s1"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "frontend"}},
		},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.Phase != PhaseReady {
		t.Fatalf("expected ready phase, got %s", status.Phase)
	}
	if !status.Ready {
		t.Fatal("expected runtime to be marked ready")
	}
	if status.Cluster != "cluster-a" {
		t.Fatalf("expected cluster-a, got %s", status.Cluster)
	}
	if status.NodeCount == 0 {
		t.Fatal("expected node count to be recorded")
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected initial full rebuild count 1, got %d", status.FullRebuildCount)
	}
	if manager.Facade() == nil || manager.Facade().Diagnostic == nil {
		t.Fatal("expected diagnostic facade to be initialized")
	}
	if len(manager.Snapshot().Pods) != 1 {
		t.Fatalf("expected snapshot pods to be preserved, got %d", len(manager.Snapshot().Pods))
	}
}

func TestQueryDiagnosticSubgraphHonorsContextWhileLockBlocked(t *testing.T) {
	manager := NewManager("cluster-a", nil)
	manager.mu.Lock()
	defer manager.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := manager.QueryDiagnosticSubgraph(ctx, "Pod", "default", "frontend", query.DiagnosticOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("expected lock wait to honor context quickly, took %s", elapsed)
	}
}

func TestManagerApplyCollectDoesNotBlockDiagnosticQueries(t *testing.T) {
	initial := collectk8s.Snapshot{
		Pods: []collectk8sresources.Pod{{
			Metadata: collectk8sresources.Metadata{UID: "p1", Name: "frontend", Namespace: "default"},
		}},
	}
	blocking := &blockingCollector{
		initial:      initial,
		collecting:   make(chan struct{}),
		release:      make(chan struct{}),
		returnedNext: initial,
	}
	manager := NewManager("cluster-a", blocking)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	applyDone := make(chan error, 1)
	go func() {
		applyDone <- manager.Apply(context.Background(), collectk8s.ChangeEvent{
			Kind:      "pod",
			Namespace: "default",
			Name:      "frontend",
			Change:    collectk8s.ChangeTypeUpsert,
		})
	}()
	<-blocking.collecting

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := manager.QueryDiagnosticSubgraph(ctx, "Pod", "default", "frontend", query.DiagnosticOptions{})
	if err != nil {
		t.Fatalf("expected diagnostic query to proceed while apply is collecting, got %v", err)
	}
	if result.Entry.CanonicalID == "" {
		t.Fatal("expected diagnostic entry from existing graph")
	}

	close(blocking.release)
	if err := <-applyDone; err != nil {
		t.Fatal(err)
	}
}

func TestManagerApplyChangeEvent(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "w1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123", Namespace: "default", UID: "p1", Labels: map[string]string{"app": "frontend"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "n1"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "s1"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "frontend"}},
		},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := manager.Status()
	service, err := client.CoreV1().Services("default").Get(context.Background(), "frontend", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	service.Spec.Selector = map[string]string{"app": "backend"}
	if _, err := client.CoreV1().Services("default").Update(context.Background(), service, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "service",
		Namespace: "default",
		Name:      "frontend",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.Phase != PhaseReady || !status.Ready {
		t.Fatal("expected ready phase after apply")
	}
	if status.LastAppliedChangeKind != "service" {
		t.Fatalf("expected applied kind service, got %s", status.LastAppliedChangeKind)
	}
	if status.LastAppliedChangeNS != "default" {
		t.Fatalf("expected applied namespace default, got %s", status.LastAppliedChangeNS)
	}
	if status.LastAppliedChangeName != "frontend" {
		t.Fatalf("expected applied name frontend, got %s", status.LastAppliedChangeName)
	}
	if status.LastAppliedChangeType != string(collectk8s.ChangeTypeUpsert) {
		t.Fatalf("expected applied type %s, got %s", collectk8s.ChangeTypeUpsert, status.LastAppliedChangeType)
	}
	if status.LastAppliedChangeAt == nil {
		t.Fatal("expected applied timestamp")
	}
	if status.LastStrategy != "service-narrow" {
		t.Fatalf("expected service-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected service scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.ServiceNarrowCount != 1 {
		t.Fatalf("expected service narrow count 1, got %d", status.ServiceNarrowCount)
	}
	if status.EdgeCount >= before.EdgeCount {
		t.Fatalf("expected selector miss to remove service edge, before=%d after=%d", before.EdgeCount, status.EdgeCount)
	}
	runtimeStatus := manager.Facade().RuntimeStatus()
	if runtimeStatus.Cluster != "cluster-a" {
		t.Fatal("expected runtime status to stay wired after apply")
	}
	if runtimeStatus.LastAppliedChangeKind != "service" || runtimeStatus.LastAppliedChangeName != "frontend" {
		t.Fatal("expected runtime status to include last applied change")
	}
	if runtimeStatus.LastAppliedChangeAt == "" {
		t.Fatal("expected runtime status timestamp")
	}
	if runtimeStatus.LastStrategy != "service-narrow" {
		t.Fatalf("expected runtime status strategy service-narrow, got %s", runtimeStatus.LastStrategy)
	}
	if runtimeStatus.FullRebuildCount != 1 || runtimeStatus.ServiceNarrowCount != 1 {
		t.Fatalf("expected runtime status counters to be projected, got full=%d service=%d", runtimeStatus.FullRebuildCount, runtimeStatus.ServiceNarrowCount)
	}
}

func TestManagerFallbackRecordsFullRebuildWhenScopedMutationFails(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "s1"}},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "service",
		Namespace: "default",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}

	status := manager.Status()
	if status.LastStrategy != "full-rebuild" {
		t.Fatalf("expected fallback strategy full-rebuild, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 2 {
		t.Fatalf("expected fallback to increment full rebuild count, got %d", status.FullRebuildCount)
	}
	if status.ServiceNarrowCount != 0 {
		t.Fatalf("expected failed service narrow attempt not to increment narrow count, got %d", status.ServiceNarrowCount)
	}
	if status.LastAppliedChangeKind != "service" || status.LastAppliedChangeNS != "default" {
		t.Fatalf("expected last applied change to be preserved, got kind=%q namespace=%q", status.LastAppliedChangeKind, status.LastAppliedChangeNS)
	}
	if status.LastError != "" {
		t.Fatalf("expected successful fallback to clear last error, got %q", status.LastError)
	}
}

func TestManagerApplyEventChangeUsesScopedMutation(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123", Namespace: "default", UID: "p1"}},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := manager.Status()
	if _, err := client.CoreV1().Events("default").Create(context.Background(), &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "frontend-abc123.123", Namespace: "default", UID: "e1"},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: "default",
			Name:      "frontend-abc123",
			UID:       "p1",
		},
		Reason:  "Failed",
		Message: "pod failed",
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "event",
		Namespace: "default",
		Name:      "frontend-abc123.123",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.LastStrategy != "event-narrow" {
		t.Fatalf("expected event-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected event scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.EventNarrowCount != 1 {
		t.Fatalf("expected event narrow count 1, got %d", status.EventNarrowCount)
	}
	if status.NodeCount <= before.NodeCount {
		t.Fatalf("expected event node to increase node count, before=%d after=%d", before.NodeCount, status.NodeCount)
	}
	if status.EdgeCount <= before.EdgeCount {
		t.Fatalf("expected event report edge to increase edge count, before=%d after=%d", before.EdgeCount, status.EdgeCount)
	}
}

func TestManagerApplyStorageChangeUsesScopedMutation(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-0", Namespace: "default", UID: "pod-uid"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
				Name:         "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data"}},
			}}},
		},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := manager.Status()
	if _, err := client.CoreV1().PersistentVolumes().Create(context.Background(), &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-data", UID: "pv-uid"},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef:                      &corev1.ObjectReference{Namespace: "default", Name: "data", UID: "pvc-uid"},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().PersistentVolumeClaims("default").Create(context.Background(), &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default", UID: "pvc-uid"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-data"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "storage",
		Namespace: "default",
		Name:      "data",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.LastStrategy != "storage-narrow" {
		t.Fatalf("expected storage-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected storage scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.StorageNarrowCount != 1 {
		t.Fatalf("expected storage narrow count 1, got %d", status.StorageNarrowCount)
	}
	if status.NodeCount <= before.NodeCount {
		t.Fatalf("expected storage nodes to increase node count, before=%d after=%d", before.NodeCount, status.NodeCount)
	}
	if status.EdgeCount <= before.EdgeCount {
		t.Fatalf("expected storage edges to increase edge count, before=%d after=%d", before.EdgeCount, status.EdgeCount)
	}
}

func TestManagerApplyIdentitySecurityChangeUsesScopedMutation(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "sa-uid"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-0", Namespace: "default", UID: "pod-uid"},
			Spec:       corev1.PodSpec{ServiceAccountName: "app"},
		},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := manager.Status()
	if _, err := client.RbacV1().RoleBindings("default").Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "app-reader", Namespace: "default", UID: "rb-uid"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "reader"},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "app",
			Namespace: "default",
		}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "identity/security",
		Namespace: "default",
		Name:      "app-reader",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.LastStrategy != "identity/security-narrow" {
		t.Fatalf("expected identity/security-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected identity/security scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.IdentitySecurityNarrowCount != 1 {
		t.Fatalf("expected identity/security narrow count 1, got %d", status.IdentitySecurityNarrowCount)
	}
	if status.EdgeCount <= before.EdgeCount {
		t.Fatalf("expected binding edge to increase edge count, before=%d after=%d", before.EdgeCount, status.EdgeCount)
	}
}

func TestManagerApplyPodChangeUsesScopedMutation(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "p1", Labels: map[string]string{"app": "frontend"}}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "s1"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "frontend"}},
		},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := manager.Status()
	pod, err := client.CoreV1().Pods("default").Get(context.Background(), "frontend", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod.Labels = map[string]string{"app": "backend"}
	if _, err := client.CoreV1().Pods("default").Update(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "pod",
		Namespace: "default",
		Name:      "frontend",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.LastStrategy != "pod-narrow" {
		t.Fatalf("expected pod-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected pod scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.PodNarrowCount != 1 {
		t.Fatalf("expected pod narrow count 1, got %d", status.PodNarrowCount)
	}
	if status.EdgeCount >= before.EdgeCount {
		t.Fatalf("expected selector miss to remove service edge, before=%d after=%d", before.EdgeCount, status.EdgeCount)
	}
}

func TestManagerApplyWorkloadChangeUsesScopedMutation(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default", UID: "w1"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-abc123",
			Namespace: "default",
			UID:       "p1",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       "Deployment",
				Name:       "frontend",
				UID:        "w1",
				Controller: boolPtr(true),
			}},
		}},
	)
	collector := collectk8s.NewReadOnlyCollector(client, "cluster-a", "default")
	manager := NewManager("cluster-a", collector)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deployment, err := client.AppsV1().Deployments("default").Get(context.Background(), "frontend", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deployment.Spec.Replicas = int32Ptr(2)
	if _, err := client.AppsV1().Deployments("default").Update(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Apply(context.Background(), collectk8s.ChangeEvent{
		Kind:      "workload",
		Namespace: "default",
		Name:      "frontend",
		Change:    collectk8s.ChangeTypeUpsert,
	}); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if status.LastStrategy != "workload-narrow" {
		t.Fatalf("expected workload-narrow strategy, got %s", status.LastStrategy)
	}
	if status.FullRebuildCount != 1 {
		t.Fatalf("expected workload scoped apply to avoid second full rebuild, got full rebuild count %d", status.FullRebuildCount)
	}
	if status.WorkloadNarrowCount != 1 {
		t.Fatalf("expected workload narrow count 1, got %d", status.WorkloadNarrowCount)
	}
	if got := workloadReplicas(manager, "frontend"); got != int32(2) {
		t.Fatalf("expected workload replicas to update to 2, got %#v", got)
	}
}

func workloadReplicas(manager *Manager, name string) any {
	for _, node := range manager.kernel.ListNodes() {
		if node.Kind == model.NodeKindWorkload && node.Name == name {
			return node.Attributes["replicas"]
		}
	}
	return nil
}

func int32Ptr(v int32) *int32 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

type blockingCollector struct {
	initial      collectk8s.Snapshot
	returnedNext collectk8s.Snapshot
	collecting   chan struct{}
	release      chan struct{}
	calls        int
	notified     bool
}

func (c *blockingCollector) Collect(ctx context.Context) (collectk8s.Snapshot, error) {
	c.calls++
	if c.calls == 1 {
		return c.initial, nil
	}
	if !c.notified {
		close(c.collecting)
		c.notified = true
	}
	select {
	case <-ctx.Done():
		return collectk8s.Snapshot{}, ctx.Err()
	case <-c.release:
		return c.returnedNext, nil
	}
}
