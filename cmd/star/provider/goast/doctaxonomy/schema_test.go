// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package doctaxonomy

import (
	"testing"

	"gopkg.in/yaml.v3"
)

const testSchemaYAML = `schemas:
  func_doc:
    format: go
    node_type: FuncDecl
    elements:
      - name: summary
        type: paragraph
        required: "true"
        order: 1
      - name: body
        type: block
        cardinality: "*"
        order: 2

  gen_decl:
    format: go
    node_type: GenDecl
    elements:
      - name: summary
        type: paragraph
        required: "true"
        order: 1
      - name: body
        type: block
        cardinality: "*"
        order: 2

  copyright:
    format: go
    node_type: File
    elements:
      - name: spdx
        type: verbatim
        required: "true"
        order: 1
      - name: copyright
        type: verbatim
        required: "true"
        order: 2
`

// TestParseSchemas verifies that YAML is correctly deserialized.
func TestParseSchemas(t *testing.T) {
	schemas, err := ParseSchemas([]byte(testSchemaYAML))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(schemas) != 3 {
		t.Fatalf("expected 3 schemas, got %d", len(schemas))
	}

	// Sorted by name: copyright, func_doc, gen_decl.
	if schemas[0].Name != "copyright" {
		t.Errorf("schema 0 name = %q, want 'copyright'", schemas[0].Name)
	}
	if schemas[1].Name != "func_doc" {
		t.Errorf("schema 1 name = %q, want 'func_doc'", schemas[1].Name)
	}
	if schemas[2].Name != "gen_decl" {
		t.Errorf("schema 2 name = %q, want 'gen_decl'", schemas[2].Name)
	}

	funcDoc := schemas[1]
	if funcDoc.Format != "go" {
		t.Errorf("func_doc format = %q, want 'go'", funcDoc.Format)
	}
	if funcDoc.NodeType != "FuncDecl" {
		t.Errorf("func_doc node_type = %q, want 'FuncDecl'", funcDoc.NodeType)
	}
	if len(funcDoc.Elements) != 2 {
		t.Fatalf("func_doc elements = %d, want 2", len(funcDoc.Elements))
	}
	if funcDoc.Elements[0].Name != "summary" {
		t.Errorf("element 0 name = %q, want 'summary'", funcDoc.Elements[0].Name)
	}
	if funcDoc.Elements[1].Name != "body" {
		t.Errorf("element 1 name = %q, want 'body'", funcDoc.Elements[1].Name)
	}
}

// TestSchemaRoundTrip verifies load → marshal → load produces identical schemas.
func TestSchemaRoundTrip(t *testing.T) {
	schemas1, err := ParseSchemas([]byte(testSchemaYAML))
	if err != nil {
		t.Fatalf("first parse error: %v", err)
	}

	// Marshal back to YAML.
	out := struct {
		Schemas map[string]CommentSchema `yaml:"schemas"`
	}{Schemas: make(map[string]CommentSchema)}
	for _, s := range schemas1 {
		out.Schemas[s.Name] = s
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Parse again.
	schemas2, err := ParseSchemas(data)
	if err != nil {
		t.Fatalf("second parse error: %v", err)
	}

	if len(schemas1) != len(schemas2) {
		t.Fatalf("schema count mismatch: %d vs %d", len(schemas1), len(schemas2))
	}

	for i := range schemas1 {
		if schemas1[i].Name != schemas2[i].Name {
			t.Errorf("schema %d name mismatch: %q vs %q", i, schemas1[i].Name, schemas2[i].Name)
		}
		if schemas1[i].Format != schemas2[i].Format {
			t.Errorf("schema %d format mismatch: %q vs %q", i, schemas1[i].Format, schemas2[i].Format)
		}
		if len(schemas1[i].Elements) != len(schemas2[i].Elements) {
			t.Errorf("schema %d element count mismatch: %d vs %d", i, len(schemas1[i].Elements), len(schemas2[i].Elements))
		}
	}
}

// TestSchemaRegistry verifies Register and Lookup.
func TestSchemaRegistry(t *testing.T) {
	reg := NewSchemaRegistry()

	schemas, err := ParseSchemas([]byte(testSchemaYAML))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	for _, s := range schemas {
		reg.Register(s)
	}

	funcDoc := reg.Lookup("FuncDecl", "go")
	if funcDoc == nil {
		t.Fatal("Lookup(FuncDecl, go) returned nil")
	}
	if funcDoc.Name != "func_doc" {
		t.Errorf("name = %q, want 'func_doc'", funcDoc.Name)
	}

	typeDoc := reg.Lookup("GenDecl", "go")
	if typeDoc == nil {
		t.Fatal("Lookup(GenDecl, go) returned nil")
	}

	missing := reg.Lookup("InterfaceDecl", "go")
	if missing != nil {
		t.Error("Lookup(InterfaceDecl, go) should return nil")
	}
}

// TestDefaultRegistry verifies that the programmatic default registry has the expected schemas.
func TestDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry()

	for _, tc := range []struct {
		nodeType string
		format   string
	}{
		{"File", "go"},
		{"GenDecl", "go"},
		{"FuncDecl", "go"},
	} {
		s := reg.Lookup(tc.nodeType, tc.format)
		if s == nil {
			t.Errorf("missing schema for %s:%s", tc.nodeType, tc.format)
		}
	}
}
