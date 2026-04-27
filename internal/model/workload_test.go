package model

import "testing"

func TestWorkloadID(t *testing.T) {
	got := WorkloadID("cluster-a", "default", "Deployment", "frontend", "uid-1")
	want := CanonicalID("cluster-a/apps/Workload/default/Deployment:frontend/uid-1/_")
	if got != want {
		t.Fatalf("unexpected workload id: got %q want %q", got, want)
	}
}

func TestNewWorkload(t *testing.T) {
	w := NewWorkload("cluster-a", "default", "StatefulSet", "ledger", "uid-2", map[string]any{"replicas": 3})
	if w.ControllerKind != "StatefulSet" {
		t.Fatalf("unexpected controller kind: %q", w.ControllerKind)
	}
	if w.ID == "" {
		t.Fatal("expected workload id to be set")
	}
}
