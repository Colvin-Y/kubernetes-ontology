package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOntologyCLIEndToEndMatchesCheckedInArtifact(t *testing.T) {
	output := filepath.Join(t.TempDir(), "kubernetes-ontology.owl")
	cmd := exec.Command("go", "run", ".", "--output", output)
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("run ontology CLI: %v\nstderr:\n%s", err, stderr.String())
	}

	generated, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read generated ontology: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("..", "..", "docs", "ontology", "kubernetes-ontology.owl"))
	if err != nil {
		t.Fatalf("read checked-in ontology: %v", err)
	}
	if !bytes.Equal(generated, golden) {
		t.Fatal("generated ontology does not match docs/ontology/kubernetes-ontology.owl; run make owl")
	}
}
