package appconfig

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	collectk8s "github.com/Colvin-Y/kubernetes-ontology/internal/collect/k8s"
	"github.com/Colvin-Y/kubernetes-ontology/internal/resolve/infer"
)

type Config struct {
	Kubeconfig        string                         `yaml:"kubeconfig"`
	Cluster           string                         `yaml:"cluster"`
	Namespace         string                         `yaml:"namespace"`
	ContextNamespaces []string                       `yaml:"contextNamespaces"`
	WorkloadResources []collectk8s.WorkloadResource  `yaml:"workloadResources"`
	ControllerRules   []infer.WorkloadControllerRule `yaml:"controllerRules"`
	Server            ServerConfig                   `yaml:"server"`
	BootstrapTimeout  string                         `yaml:"bootstrapTimeout"`
	PollInterval      string                         `yaml:"pollInterval"`
	StreamMode        string                         `yaml:"streamMode"`
	ObserveDuration   string                         `yaml:"observeDuration"`
	MaxDepth          int                            `yaml:"maxDepth"`
	StorageMaxDepth   int                            `yaml:"storageMaxDepth"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	URL  string `yaml:"url"`
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}
