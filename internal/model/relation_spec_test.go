package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestRelationSpecsCoverEdgeKinds(t *testing.T) {
	specs := RelationSpecs()
	covered := make(map[EdgeKind]bool, len(specs))
	for _, spec := range specs {
		if spec.Kind == "" {
			t.Fatal("relation spec has empty kind")
		}
		if spec.Comment == "" {
			t.Fatalf("relation spec %s has empty comment", spec.Kind)
		}
		if spec.Domain == "" || spec.Range == "" {
			t.Fatalf("relation spec %s must define domain and range", spec.Kind)
		}
		covered[spec.Kind] = true
	}
	for _, kind := range edgeKindsFromSource(t) {
		if !covered[kind] {
			t.Fatalf("edge kind %s is missing relation metadata", kind)
		}
	}
}

func TestNewEdgeUsesRelationMetadata(t *testing.T) {
	edge := NewEdge("pod-a", "node-a", EdgeKindScheduledOn)
	if edge.Provenance.SourceType != EdgeSourceTypeExplicitRef {
		t.Fatalf("expected explicit_ref source type, got %s", edge.Provenance.SourceType)
	}
	if edge.Provenance.State != EdgeStateAsserted {
		t.Fatalf("expected asserted state, got %s", edge.Provenance.State)
	}
	if edge.Provenance.Resolver != "pod-spec-node/v1" {
		t.Fatalf("expected pod-spec-node/v1 resolver, got %s", edge.Provenance.Resolver)
	}
	if edge.Provenance.LastSeenAt == nil {
		t.Fatal("expected last seen timestamp")
	}
}

func TestRelationSpecsReturnDefensiveCopies(t *testing.T) {
	specs := RelationSpecs()
	if len(specs) == 0 {
		t.Fatal("expected relation specs")
	}
	specs[0].Kind = "mutated"
	for index, spec := range specs {
		if len(spec.ResolverHints) == 0 {
			continue
		}
		specs[index].ResolverHints[0] = "mutated"
		break
	}

	if _, ok := RelationSpecFor("mutated"); ok {
		t.Fatal("external mutation changed relation spec kind")
	}
	spec, ok := RelationSpecFor(EdgeKindScheduledOn)
	if !ok {
		t.Fatal("expected scheduled_on relation spec")
	}
	if spec.ResolverHints[0] != "pod-spec-node/v1" {
		t.Fatalf("external mutation changed resolver hints, got %s", spec.ResolverHints[0])
	}

	spec.ResolverHints[0] = "mutated-again"
	again, ok := RelationSpecFor(EdgeKindScheduledOn)
	if !ok {
		t.Fatal("expected scheduled_on relation spec")
	}
	if again.ResolverHints[0] != "pod-spec-node/v1" {
		t.Fatalf("external mutation changed RelationSpecFor hints, got %s", again.ResolverHints[0])
	}
}

func edgeKindsFromSource(t *testing.T) []EdgeKind {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "edge.go", nil, 0)
	if err != nil {
		t.Fatalf("parse edge.go: %v", err)
	}
	out := make([]EdgeKind, 0)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "EdgeKind") {
					continue
				}
				if index >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[index].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", name.Name, err)
				}
				out = append(out, EdgeKind(value))
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no edge kind constants found")
	}
	return out
}
