package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
)

type sequenceCollector struct {
	snapshots []Snapshot
	index     int
}

func (c *sequenceCollector) Collect(ctx context.Context) (Snapshot, error) {
	_ = ctx
	if len(c.snapshots) == 0 {
		return Snapshot{}, nil
	}
	if c.index >= len(c.snapshots) {
		return c.snapshots[len(c.snapshots)-1], nil
	}
	out := c.snapshots[c.index]
	c.index++
	return out, nil
}

type recordingSink struct {
	events []ChangeEvent
}

func (s *recordingSink) Apply(ctx context.Context, event ChangeEvent) error {
	_ = ctx
	s.events = append(s.events, event)
	return nil
}

func TestParseStreamMode(t *testing.T) {
	tests := []struct {
		raw  string
		want StreamMode
	}{
		{raw: "", want: StreamModeInformer},
		{raw: "informer", want: StreamModeInformer},
		{raw: " polling ", want: StreamModePolling},
	}
	for _, tt := range tests {
		got, err := ParseStreamMode(tt.raw)
		if err != nil {
			t.Fatalf("ParseStreamMode(%q): %v", tt.raw, err)
		}
		if got != tt.want {
			t.Fatalf("ParseStreamMode(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
	if _, err := ParseStreamMode("watch"); err == nil {
		t.Fatal("ParseStreamMode accepted unsupported mode")
	}
}

func TestInformerChangeEventClassification(t *testing.T) {
	controller := true
	tests := []struct {
		name   string
		obj    interface{}
		change ChangeType
		want   ChangeEvent
	}{
		{
			name:   "deployment workload",
			obj:    &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "workload", Namespace: "default", Name: "frontend", Change: ChangeTypeUpsert},
		},
		{
			name: "replicaset maps to owning workload",
			obj: &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
				Name:      "frontend-abc",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{
					Kind:       "Deployment",
					Name:       "frontend",
					Controller: &controller,
				}},
			}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "workload", Namespace: "default", Name: "frontend", Change: ChangeTypeUpsert},
		},
		{
			name:   "job workload",
			obj:    batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "workload", Namespace: "default", Name: "backup", Change: ChangeTypeUpsert},
		},
		{
			name: "custom workload",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps.kruise.io/v1beta1",
				"kind":       "StatefulSet",
				"metadata": map[string]interface{}{
					"name":      "redis",
					"namespace": "default",
				},
			}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "workload", Namespace: "default", Name: "redis", Change: ChangeTypeUpsert},
		},
		{
			name:   "pod",
			obj:    &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-0", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "pod", Namespace: "default", Name: "frontend-0", Change: ChangeTypeUpsert},
		},
		{
			name:   "service",
			obj:    &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "service", Namespace: "default", Name: "frontend", Change: ChangeTypeUpsert},
		},
		{
			name:   "persistent volume claim storage",
			obj:    &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "storage", Namespace: "default", Name: "data", Change: ChangeTypeUpsert},
		},
		{
			name:   "persistent volume storage",
			obj:    &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-data"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "storage", Name: "pv-data", Change: ChangeTypeUpsert},
		},
		{
			name:   "storage class storage",
			obj:    &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "storage", Name: "fast", Change: ChangeTypeUpsert},
		},
		{
			name:   "csi driver storage",
			obj:    &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: "local.csi.example.com"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "storage", Name: "local.csi.example.com", Change: ChangeTypeUpsert},
		},
		{
			name:   "event",
			obj:    &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "frontend.123", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "event", Namespace: "default", Name: "frontend.123", Change: ChangeTypeUpsert},
		},
		{
			name:   "service account identity",
			obj:    &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "frontend", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "identity/security", Namespace: "default", Name: "frontend", Change: ChangeTypeUpsert},
		},
		{
			name:   "role binding identity",
			obj:    &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "read-pods", Namespace: "default"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "identity/security", Namespace: "default", Name: "read-pods", Change: ChangeTypeUpsert},
		},
		{
			name:   "cluster role binding identity",
			obj:    &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "read-all-pods"}},
			change: ChangeTypeUpsert,
			want:   ChangeEvent{Kind: "identity/security", Name: "read-all-pods", Change: ChangeTypeUpsert},
		},
		{
			name:   "delete tombstone",
			obj:    cache.DeletedFinalStateUnknown{Obj: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "frontend-0", Namespace: "default"}}},
			change: ChangeTypeDelete,
			want:   ChangeEvent{Kind: "pod", Namespace: "default", Name: "frontend-0", Change: ChangeTypeDelete},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := changeEventForInformerObject(tt.obj, tt.change)
			if !ok {
				t.Fatal("object was not classified")
			}
			if got != tt.want {
				t.Fatalf("event = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPollingStreamDetectsServiceSelectorChange(t *testing.T) {
	collector := &sequenceCollector{snapshots: []Snapshot{
		{Services: []resources.Service{{Metadata: resources.Metadata{UID: "s1", Namespace: "default", Name: "frontend"}, Selector: map[string]string{"app": "frontend"}}}},
		{Services: []resources.Service{{Metadata: resources.Metadata{UID: "s1", Namespace: "default", Name: "frontend"}, Selector: map[string]string{"app": "backend"}}}},
	}}
	stream := NewPollingStream(collector, 0)
	sink := &recordingSink{}

	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected first tick to seed state only, got %d events", len(sink.events))
	}
	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one service change event, got %d", len(sink.events))
	}
	if sink.events[0].Kind != "service" {
		t.Fatalf("expected service change event, got %q", sink.events[0].Kind)
	}
	if sink.events[0].Namespace != "default" || sink.events[0].Name != "frontend" {
		t.Fatalf("expected service identity default/frontend, got %q/%q", sink.events[0].Namespace, sink.events[0].Name)
	}
}

func TestPollingStreamDetectsPodLabelChange(t *testing.T) {
	collector := &sequenceCollector{snapshots: []Snapshot{
		{Pods: []resources.Pod{{Metadata: resources.Metadata{UID: "p1", Namespace: "default", Name: "frontend-1", Labels: map[string]string{"app": "frontend"}}}}},
		{Pods: []resources.Pod{{Metadata: resources.Metadata{UID: "p1", Namespace: "default", Name: "frontend-1", Labels: map[string]string{"app": "backend"}}}}},
	}}
	stream := NewPollingStream(collector, 0)
	sink := &recordingSink{}

	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one pod change event, got %d", len(sink.events))
	}
	if sink.events[0].Kind != "pod" {
		t.Fatalf("expected pod change event, got %q", sink.events[0].Kind)
	}
	if sink.events[0].Namespace != "default" || sink.events[0].Name != "frontend-1" {
		t.Fatalf("expected pod identity default/frontend-1, got %q/%q", sink.events[0].Namespace, sink.events[0].Name)
	}
}

func TestPollingStreamMapsReplicaSetChangeToOwningWorkload(t *testing.T) {
	workload := resources.Workload{Metadata: resources.Metadata{UID: "w1", Namespace: "default", Name: "frontend"}, ControllerKind: "Deployment"}
	rsOwner := []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "w1", Controller: true}}
	collector := &sequenceCollector{snapshots: []Snapshot{
		{
			Workloads:   []resources.Workload{workload},
			ReplicaSets: []resources.ReplicaSet{{Metadata: resources.Metadata{UID: "rs1", Namespace: "default", Name: "frontend-rs"}, Replicas: 1, OwnerReferences: rsOwner}},
		},
		{
			Workloads:   []resources.Workload{workload},
			ReplicaSets: []resources.ReplicaSet{{Metadata: resources.Metadata{UID: "rs1", Namespace: "default", Name: "frontend-rs"}, Replicas: 2, OwnerReferences: rsOwner}},
		},
	}}
	stream := NewPollingStream(collector, 0)
	sink := &recordingSink{}

	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one workload change event, got %d", len(sink.events))
	}
	if sink.events[0].Kind != "workload" {
		t.Fatalf("expected workload change event, got %q", sink.events[0].Kind)
	}
	if sink.events[0].Namespace != "default" || sink.events[0].Name != "frontend" {
		t.Fatalf("expected owning workload default/frontend, got %q/%q", sink.events[0].Namespace, sink.events[0].Name)
	}
}

func TestPollingStreamDetectsIdentitySecurityChange(t *testing.T) {
	collector := &sequenceCollector{snapshots: []Snapshot{
		{RoleBindings: []resources.RoleBinding{{
			Metadata:     resources.Metadata{UID: "rb1", Namespace: "default", Name: "app-reader"},
			RoleRef:      "reader",
			SubjectKinds: []string{"ServiceAccount"},
			SubjectNames: []string{"app"},
		}}},
		{RoleBindings: []resources.RoleBinding{{
			Metadata:     resources.Metadata{UID: "rb1", Namespace: "default", Name: "app-reader"},
			RoleRef:      "reader",
			SubjectKinds: []string{"ServiceAccount"},
			SubjectNames: []string{"builder"},
		}}},
	}}
	stream := NewPollingStream(collector, 0)
	sink := &recordingSink{}

	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if err := stream.tick(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one identity/security change event, got %d", len(sink.events))
	}
	if sink.events[0].Kind != "identity/security" {
		t.Fatalf("expected identity/security change event, got %q", sink.events[0].Kind)
	}
	if sink.events[0].Namespace != "default" || sink.events[0].Name != "app-reader" {
		t.Fatalf("expected rolebinding identity default/app-reader, got %q/%q", sink.events[0].Namespace, sink.events[0].Name)
	}
}
