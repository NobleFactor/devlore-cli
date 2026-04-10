// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package doctaxonomy

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// DefaultRegistry returns a SchemaRegistry with the standard Go comment schemas.
//
// These match the defaults in the LintGoStyle extension config.
func DefaultRegistry() *SchemaRegistry {
	reg := NewSchemaRegistry()
	reg.Register(CommentSchema{
		Name: "copyright", Format: "go", NodeType: "File",
		Elements: []SchemaElement{
			{Name: "spdx", Type: "verbatim", Required: "true", Order: 1},
			{Name: "copyright", Type: "verbatim", Required: "true", Order: 2},
		},
	})
	reg.Register(CommentSchema{
		Name: "package_doc", Format: "go", NodeType: "PkgPath",
		SummaryPrefix: `PkgPath {name}\b`,
		Elements: []SchemaElement{
			{Name: "summary", Production: "item", Consumes: "Paragraph / Heading", Prefix: "PkgPath {name}", Required: "true", Order: 1},
			{Name: "body", Production: "item", Consumes: "*(Paragraph / Code / Heading)", Order: 2},
		},
	})
	reg.Register(CommentSchema{
		Name: "gen_decl", Format: "go", NodeType: "GenDecl",
		SummaryPrefix: `{name}\b`,
		Elements: []SchemaElement{
			{Name: "summary", Production: "item", Consumes: "Paragraph / Heading", Prefix: "{name}", Required: "true", Order: 1},
			{Name: "body", Production: "item", Consumes: "*(Paragraph / Code / Heading)", Order: 2},
		},
	})
	reg.Register(CommentSchema{
		Name: "func_doc", Format: "go", NodeType: "FuncDecl",
		SummaryPrefix: `{name}\b`,
		Elements: []SchemaElement{
			{Name: "summary", Production: "item", Consumes: "Paragraph / Heading", Prefix: "{name}", Required: "true", Order: 1},
			{Name: "body", Production: "item", Consumes: "*(Paragraph / Code / Heading)", Order: 2},
		},
	})
	return reg
}

// SchemaElement defines one element slot in a comment schema.
type SchemaElement struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Required    string `yaml:"required,omitempty"`
	Cardinality string `yaml:"cardinality,omitempty"`
	Order       int    `yaml:"order"`
	Header      string `yaml:"header,omitempty"`
	ItemTokens  string `yaml:"item_tokens,omitempty"`

	// Production model fields.
	Production string `yaml:"production,omitempty"`  // "item" or "list"
	Consumes   string `yaml:"consumes,omitempty"`    // ABNF: "Paragraph / Heading", "*(Paragraph / Code)", etc.
	Condition  string `yaml:"condition,omitempty"`   // "params", "returns", "exported", "receiver"
	Prefix     string `yaml:"prefix,omitempty"`      // fuzzy prefix pattern: "{name}", "Parameters:", "+"
	Split      string `yaml:"split,omitempty"`       // "sentence" — extract first sentence, remainder flows to next
	Slots      string `yaml:"slots,omitempty"`       // "params" or "returns" — slot names from declaration context
	SlotPrefix string `yaml:"slot_prefix,omitempty"` // fuzzy slot prefix: "{slot}"
}

// CommentSchema defines the structure of a doc comment for a given node type and format.
type CommentSchema struct {
	Name          string          `yaml:"-"`
	Format        string          `yaml:"format"`
	NodeType      string          `yaml:"node_type"`
	SummaryPrefix string          `yaml:"summary_prefix,omitempty"`
	Elements      []SchemaElement `yaml:"elements"`
}

// schemaFile is the top-level YAML structure.
type schemaFile struct {
	Schemas map[string]CommentSchema `yaml:"schemas"`
}

// LoadSchemas deserializes a YAML file into a slice of CommentSchema.
func LoadSchemas(path string) ([]CommentSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema file: %w", err)
	}
	return ParseSchemas(data)
}

// ParseSchemas deserializes YAML bytes into a slice of CommentSchema.
func ParseSchemas(data []byte) ([]CommentSchema, error) {
	var f schemaFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("unmarshal schemas: %w", err)
	}

	schemas := make([]CommentSchema, 0, len(f.Schemas))
	for name, s := range f.Schemas {
		s.Name = name
		schemas = append(schemas, s)
	}

	// Sort by name for deterministic ordering.
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})

	return schemas, nil
}

// SchemaRegistry holds loaded schemas keyed by (nodeType, format).
type SchemaRegistry struct {
	mu      sync.RWMutex
	schemas map[string]*CommentSchema // key: "nodeType:format"
}

// NewSchemaRegistry creates an empty registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		schemas: make(map[string]*CommentSchema),
	}
}

// Register adds a schema to the registry.
func (r *SchemaRegistry) Register(schema CommentSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := schema.NodeType + ":" + schema.Format
	s := schema // copy
	r.schemas[key] = &s
}

// Lookup finds a schema by node type and format.
//
// Returns nil if not found.
func (r *SchemaRegistry) Lookup(nodeType, format string) *CommentSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.schemas[nodeType+":"+format]
}
