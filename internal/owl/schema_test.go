package owl

import (
	"bytes"
	"encoding/xml"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWriteStaticOntologyProducesRDFXML(t *testing.T) {
	var out bytes.Buffer
	if err := WriteStaticOntology(&out); err != nil {
		t.Fatalf("write static ontology: %v", err)
	}
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(out.Bytes(), &root); err != nil {
		t.Fatalf("generated ontology is not well-formed XML: %v", err)
	}
	for _, expected := range []string{
		`<owl:Class rdf:about="#Pod">`,
		`<owl:ObjectProperty rdf:about="#managed_by">`,
		`<owl:DatatypeProperty rdf:about="#canonicalId">`,
		`owner-chain/v1`,
	} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("expected generated ontology to contain %q", expected)
		}
	}
}

func TestStaticOntologyCoversModelNodeKinds(t *testing.T) {
	values := constStringValues(t, filepath.Join("..", "model", "node.go"), "NodeKind")
	covered := map[string]bool{}
	for _, class := range Classes() {
		if class.NodeKind {
			covered[class.ID] = true
		}
	}
	for _, value := range values {
		if !covered[value] {
			t.Fatalf("model node kind %q is missing from OWL classes", value)
		}
	}
}

func TestStaticOntologyCoversModelEdgeKinds(t *testing.T) {
	values := constStringValues(t, filepath.Join("..", "model", "edge.go"), "EdgeKind")
	covered := map[string]bool{}
	for _, property := range ObjectProperties() {
		covered[string(property.Kind)] = true
	}
	for _, value := range values {
		if !covered[value] {
			t.Fatalf("model edge kind %q is missing from OWL object properties", value)
		}
	}
}

func constStringValues(t *testing.T, path, prefix string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0)
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
				if !strings.HasPrefix(name.Name, prefix) {
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
				out = append(out, value)
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s constants found in %s", prefix, path)
	}
	return out
}
