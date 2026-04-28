package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubernetes-ontology.yaml")
	data := []byte(`
kubeconfig: /tmp/kubeconfig
cluster: test-cluster
namespace: redis
contextNamespaces:
  - redis
  - kruise-system
workloadResources:
  - group: apps.kruise.io
    version: v1alpha1
    resource: advancedstatefulsets
    kind: AdvancedStatefulSet
    namespaced: true
controllerRules:
  - apiVersion: apps.kruise.io/*
    kind: "*"
    namespace: kruise-system
    controllerPodPrefixes:
      - kruise-controller-manager
    nodeDaemonPodPrefixes:
      - kruise-daemon
csiComponentRules:
  - driver: diskplugin.csi.alibabacloud.com
    namespace: kube-system
    controllerPodPrefixes:
      - csi-provisioner-
    nodeAgentPodPrefixes:
      - csi-plugin-
server:
  addr: 127.0.0.1:18080
  url: http://127.0.0.1:18080
bootstrapTimeout: 2m
pollInterval: 5s
streamMode: informer
observeDuration: 40s
maxDepth: 3
storageMaxDepth: 6
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kubeconfig != "/tmp/kubeconfig" || cfg.Cluster != "test-cluster" || cfg.Namespace != "redis" {
		t.Fatalf("unexpected scalar config: %#v", cfg)
	}
	if got := len(cfg.ContextNamespaces); got != 2 {
		t.Fatalf("context namespace count = %d, want 2", got)
	}
	if got := len(cfg.WorkloadResources); got != 1 {
		t.Fatalf("workload resource count = %d, want 1", got)
	}
	if cfg.WorkloadResources[0].Kind != "AdvancedStatefulSet" || !cfg.WorkloadResources[0].Namespaced {
		t.Fatalf("unexpected workload resource: %#v", cfg.WorkloadResources[0])
	}
	if got := len(cfg.ControllerRules); got != 1 {
		t.Fatalf("controller rule count = %d, want 1", got)
	}
	if cfg.ControllerRules[0].APIVersion != "apps.kruise.io/*" || cfg.ControllerRules[0].ControllerNamespace != "kruise-system" {
		t.Fatalf("unexpected controller rule: %#v", cfg.ControllerRules[0])
	}
	if got := len(cfg.CSIComponentRules); got != 1 {
		t.Fatalf("csi component rule count = %d, want 1", got)
	}
	if cfg.CSIComponentRules[0].Driver != "diskplugin.csi.alibabacloud.com" || cfg.CSIComponentRules[0].ComponentNamespace != "kube-system" {
		t.Fatalf("unexpected csi component rule: %#v", cfg.CSIComponentRules[0])
	}
	if cfg.Server.Addr != "127.0.0.1:18080" || cfg.Server.URL != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected server config: %#v", cfg.Server)
	}
	if cfg.BootstrapTimeout != "2m" || cfg.PollInterval != "5s" || cfg.StreamMode != "informer" || cfg.ObserveDuration != "40s" || cfg.MaxDepth != 3 || cfg.StorageMaxDepth != 6 {
		t.Fatalf("unexpected runtime config: %#v", cfg)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubernetes-ontology.yaml")
	if err := os.WriteFile(path, []byte("kubeconfigs: /tmp/kubeconfig\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded with an unknown field")
	}
}
