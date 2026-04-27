package owner

import (
	"reflect"
	"testing"

	"github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s/resources"
)

func TestChainResolverResolvesRecursiveControllerOwners(t *testing.T) {
	resolver := NewChainResolver("cluster-a",
		[]resources.Workload{{
			Metadata:       resources.Metadata{UID: "deploy-uid", Name: "frontend", Namespace: "default"},
			ControllerKind: "Deployment",
		}},
		[]resources.ReplicaSet{
			{
				Metadata:        resources.Metadata{UID: "rs-root-uid", Name: "frontend-root", Namespace: "default"},
				OwnerReferences: []resources.OwnerReference{{Kind: "Deployment", Name: "frontend", UID: "deploy-uid", Controller: true}},
			},
			{
				Metadata:        resources.Metadata{UID: "rs-leaf-uid", Name: "frontend-leaf", Namespace: "default"},
				OwnerReferences: []resources.OwnerReference{{Kind: "ReplicaSet", Name: "frontend-root", UID: "rs-root-uid", Controller: true}},
			},
		},
	)

	targets := resolver.ResolvePodWorkloads(resources.Pod{
		Metadata:        resources.Metadata{Name: "frontend-pod", Namespace: "default"},
		OwnerReferences: []resources.OwnerReference{{Kind: "ReplicaSet", Name: "frontend-leaf", UID: "rs-leaf-uid", Controller: true}},
	})
	if len(targets) != 1 {
		t.Fatalf("expected one workload target, got %#v", targets)
	}
	if targets[0].Kind != "Deployment" || targets[0].Name != "frontend" || targets[0].UID != "deploy-uid" {
		t.Fatalf("unexpected workload target: %#v", targets[0])
	}
}

func TestChainResolverPrefersControllerOwnerReference(t *testing.T) {
	resolver := NewChainResolver("cluster-a",
		[]resources.Workload{
			{Metadata: resources.Metadata{UID: "controller-uid", Name: "controller", Namespace: "default"}, ControllerKind: "Deployment"},
			{Metadata: resources.Metadata{UID: "observer-uid", Name: "observer", Namespace: "default"}, ControllerKind: "Job"},
		},
		nil,
	)

	targets := resolver.ResolveOwnerReferences("default", []resources.OwnerReference{
		{Kind: "Job", Name: "observer", UID: "observer-uid"},
		{Kind: "Deployment", Name: "controller", UID: "controller-uid", Controller: true},
	})
	got := targetNames(targets)
	want := []string{"Deployment/controller"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets: got %#v want %#v", got, want)
	}
}

func TestChainResolverFallsBackToAllOwnersWithoutControllerMarker(t *testing.T) {
	resolver := NewChainResolver("cluster-a",
		[]resources.Workload{
			{Metadata: resources.Metadata{UID: "a-uid", Name: "a", Namespace: "default"}, ControllerKind: "Deployment"},
			{Metadata: resources.Metadata{UID: "b-uid", Name: "b", Namespace: "default"}, ControllerKind: "Job"},
		},
		nil,
	)

	targets := resolver.ResolveOwnerReferences("default", []resources.OwnerReference{
		{Kind: "Job", Name: "b", UID: "b-uid"},
		{Kind: "Deployment", Name: "a", UID: "a-uid"},
	})
	got := targetNames(targets)
	want := []string{"Deployment/a", "Job/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected targets: got %#v want %#v", got, want)
	}
}

func targetNames(targets []WorkloadTarget) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.Kind+"/"+target.Name)
	}
	return out
}
