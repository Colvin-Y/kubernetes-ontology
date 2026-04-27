package selector

import "testing"

func TestLabelsMatch(t *testing.T) {
	if !LabelsMatch(map[string]string{"app": "frontend"}, map[string]string{"app": "frontend", "tier": "web"}) {
		t.Fatal("expected selector to match labels")
	}
	if LabelsMatch(map[string]string{"app": "frontend", "tier": "api"}, map[string]string{"app": "frontend", "tier": "web"}) {
		t.Fatal("expected selector mismatch")
	}
}
