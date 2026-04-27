package reconcile

import (
	"reflect"
	"sort"
	"testing"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
)

func TestEventReconcilerMatchesFullRebuildEventEdges(t *testing.T) {
	initial := eventSnapshot("")
	next := eventSnapshot("Failed")
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewEventReconciler("cluster-a", kernel).Apply(next, "default", "frontend-1.123", collectk8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied {
		t.Fatal("expected event update to be applied")
	}
	if result.UpsertedReportEdges != 1 {
		t.Fatalf("expected one report edge, got %d", result.UpsertedReportEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	got := filteredEdgeKeys(kernel.ListEdges(), model.EdgeKindReportedByEvent)
	want := filteredEdgeKeys(full.ListEdges(), model.EdgeKindReportedByEvent)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reported event edges mismatch\ngot  %#v\nwant %#v", got, want)
	}
	gotEvents := eventNodeFingerprints(kernel.ListNodes())
	wantEvents := eventNodeFingerprints(full.ListNodes())
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("event nodes mismatch\ngot  %#v\nwant %#v", gotEvents, wantEvents)
	}
}

func TestEventReconcilerRebuildsWebhookEvidence(t *testing.T) {
	initial := webhookEventSnapshot("")
	next := webhookEventSnapshot("FailedCreate")
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewEventReconciler("cluster-a", kernel).Apply(next, "default", "frontend.456", collectk8s.ChangeTypeUpsert)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpsertedWebhookEdges != 1 {
		t.Fatalf("expected one webhook edge, got %d", result.UpsertedWebhookEdges)
	}

	full := kernelFromSnapshot(t, "cluster-a", next)
	got := filteredEdgeKeys(kernel.ListEdges(), model.EdgeKindAffectedByWebhook)
	want := filteredEdgeKeys(full.ListEdges(), model.EdgeKindAffectedByWebhook)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("webhook evidence edges mismatch\ngot  %#v\nwant %#v", got, want)
	}
}

func TestEventReconcilerDeletesMissingEvent(t *testing.T) {
	initial := eventSnapshot("Failed")
	next := eventSnapshot("")
	kernel := kernelFromSnapshot(t, "cluster-a", initial)

	result, err := NewEventReconciler("cluster-a", kernel).Apply(next, "default", "frontend-1.123", collectk8s.ChangeTypeDelete)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted {
		t.Fatal("expected delete result")
	}
	if len(filteredEdgeKeys(kernel.ListEdges(), model.EdgeKindReportedByEvent)) != 0 {
		t.Fatal("expected event report edges to be removed")
	}
	for _, node := range kernel.ListNodes() {
		if node.Kind == model.NodeKindEvent && node.Namespace == "default" && node.Name == "frontend-1.123" {
			t.Fatal("expected event node to be removed")
		}
	}
}

func eventSnapshot(reason string) collectk8s.Snapshot {
	snapshot := collectk8s.Snapshot{
		Pods: []resources.Pod{{Metadata: resources.Metadata{UID: "p1", Name: "frontend-1", Namespace: "default"}}},
	}
	if reason != "" {
		snapshot.Events = []resources.Event{{
			Metadata:     resources.Metadata{UID: "e1", Name: "frontend-1.123", Namespace: "default"},
			InvolvedKind: "Pod",
			InvolvedName: "frontend-1",
			InvolvedUID:  "p1",
			Reason:       reason,
			Message:      "pod failed",
		}}
	}
	return snapshot
}

func webhookEventSnapshot(reason string) collectk8s.Snapshot {
	snapshot := collectk8s.Snapshot{
		Workloads:      []resources.Workload{{Metadata: resources.Metadata{UID: "w1", Name: "frontend", Namespace: "default"}, ControllerKind: "Deployment"}},
		WebhookConfigs: []resources.WebhookConfig{{Metadata: resources.Metadata{UID: "wh1", Name: "policy-webhook"}, Kind: "ValidatingWebhookConfiguration"}},
	}
	if reason != "" {
		snapshot.Events = []resources.Event{{
			Metadata:     resources.Metadata{UID: "e1", Name: "frontend.456", Namespace: "default"},
			InvolvedKind: "Deployment",
			InvolvedName: "frontend",
			InvolvedUID:  "w1",
			Reason:       reason,
			Message:      "admission denied",
		}}
	}
	return snapshot
}

func filteredEdgeKeys(edges []model.Edge, kind model.EdgeKind) []string {
	out := make([]string, 0)
	for _, edge := range edges {
		if edge.Kind == kind {
			out = append(out, edge.Key())
		}
	}
	sort.Strings(out)
	return out
}

func eventNodeFingerprints(nodes []model.Node) []string {
	out := make([]string, 0)
	for _, node := range nodes {
		if node.Kind != model.NodeKindEvent {
			continue
		}
		reason, _ := node.Attributes["reason"].(string)
		message, _ := node.Attributes["message"].(string)
		out = append(out, node.ID.String()+"|"+reason+"|"+message)
	}
	sort.Strings(out)
	return out
}
