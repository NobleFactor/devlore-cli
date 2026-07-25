// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
	"github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// lintGoStyleExtensionYAML is the extension.yaml for the LintGoStyle extension, loaded from the actual file to keep
// tests in sync with the source of truth.
const lintGoStyleExtensionYAML = "../../.." +
	"/star/extensions/com.noblefactor.star.LintGoStyle/extension.yaml"

// loadTestExtension reads an extension.yaml into a star.Extension using document.Read.
func loadTestExtension(t *testing.T, path string) *star.Extension {
	t.Helper()

	ext, err := document.ReadFile[star.Extension](path)
	if err != nil {
		t.Fatalf("document.ReadFile(%s): %v", path, err)
	}
	return ext
}

// registryFromConfig registers the extension config, populates defaults, navigates to
// comment_schemas, and converts to a SchemaRegistry.
func registryFromConfig(t *testing.T, ext *star.Extension, configYAML string) *doctaxonomy.SchemaRegistry {
	t.Helper()

	config.ClearTypeCache()

	cfg := config.New()
	if err := cfg.RegisterExtension(ext.ConfigPath(), ext.ToConfigSpec()); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	if configYAML != "" {
		if err := cfg.MergeYAML([]byte(configYAML)); err != nil {
			t.Fatalf("MergeYAML: %v", err)
		}
	}

	schemasVal := cfg.Navigate("lint.go_style.comment_schemas")
	if schemasVal == nil {
		t.Fatal("Navigate(lint.go_style.comment_schemas) returned nil")
	}

	reg := schemasFromConfig(schemasVal)
	if reg == nil {
		t.Fatal("schemasFromConfig returned nil")
	}
	return reg
}

// TestConfigSchemas_DefaultsMatchRegistry verifies that the extension.yaml defaults produce a SchemaRegistry equivalent
// to DefaultRegistry().
func TestConfigSchemas_DefaultsMatchRegistry(t *testing.T) {
	ext := loadTestExtension(t, lintGoStyleExtensionYAML)
	got := registryFromConfig(t, ext, "")
	want := doctaxonomy.DefaultRegistry()

	for _, tc := range []struct {
		nodeType string
		format   string
	}{
		{"File", "go"},
		{"GenDecl", "go"},
		{"FuncDecl", "go"},
	} {
		gs := got.Lookup(tc.nodeType, tc.format)
		ws := want.Lookup(tc.nodeType, tc.format)
		if gs == nil {
			t.Errorf("config registry missing schema for %s:%s", tc.nodeType, tc.format)
			continue
		}
		if ws == nil {
			t.Errorf("default registry missing schema for %s:%s", tc.nodeType, tc.format)
			continue
		}
		if gs.Format != ws.Format {
			t.Errorf("%s: Format = %q, want %q", tc.nodeType, gs.Format, ws.Format)
		}
		if gs.NodeType != ws.NodeType {
			t.Errorf("%s: NodeType = %q, want %q", tc.nodeType, gs.NodeType, ws.NodeType)
		}
		if len(gs.Elements) != len(ws.Elements) {
			t.Errorf("%s: %d elements, want %d", tc.nodeType, len(gs.Elements), len(ws.Elements))
			continue
		}
		for i, ge := range gs.Elements {
			we := ws.Elements[i]
			if ge != we {
				t.Errorf("%s element[%d]: got %+v, want %+v", tc.nodeType, i, ge, we)
			}
		}
	}
}

// TestConfigSchemas_ProviderPicksUpConfig verifies the full provider path: config in context -> schemaRegistry() returns
// config-based registry -> LoadSourceFile gets schemas with parameters/returns.
func TestConfigSchemas_ProviderPicksUpConfig(t *testing.T) {
	ext := loadTestExtension(t, lintGoStyleExtensionYAML)

	projectConfig := `
lint:
  go_style:
    comment_schemas:
      func_doc:
        format: go
        node_type: FuncDecl
        summary_prefix: "{name}\\b"
        elements:
          - name: summary
            type: paragraph
            required: "true"
            order: 1
          - name: body
            type: block
            cardinality: "*"
            order: 2
          - name: parameters
            type: section
            header: "Parameters:"
            item_tokens: ParamName
            order: 3
          - name: returns
            type: section
            header: "Returns:"
            item_tokens: ReturnType
            order: 4
          - name: directives
            type: directive
            cardinality: "*"
            order: 5
`
	config.ClearTypeCache()
	cfg := config.New()
	if err := cfg.RegisterExtension(ext.ConfigPath(), ext.ToConfigSpec()); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}
	if err := cfg.MergeYAML([]byte(projectConfig)); err != nil {
		t.Fatalf("MergeYAML: %v", err)
	}

	// Create provider with config in Application.Overrides — same wiring as the runtime, where star main
	// stamps cfg into Overrides before any provider's NewProvider runs.
	ctx := &op.RuntimeEnvironment{
		Application: &application.Application{
			Name:      "test",
			Overrides: map[string]any{"config": cfg},
		},
	}
	p := NewProvider(ctx)

	// schemaRegistry should return config-based registry, not defaults.
	reg := p.schemaRegistry()
	funcDoc := reg.Lookup("FuncDecl", "go")
	if funcDoc == nil {
		t.Fatal("schemaRegistry missing FuncDecl schema")
	}
	if len(funcDoc.Elements) != 5 {
		t.Fatalf("func_doc has %d elements, want 5 (summary, body, parameters, returns, directives)", len(funcDoc.Elements))
	}

	// Deep compare every element field against expected values.
	wantElements := []doctaxonomy.SchemaElement{
		{Name: "summary", Type: "paragraph", Required: "true", Order: 1},
		{Name: "body", Type: "block", Cardinality: "*", Order: 2},
		{Name: "parameters", Type: "section", Header: "Parameters:", ItemTokens: "ParamName", Order: 3},
		{Name: "returns", Type: "section", Header: "Returns:", ItemTokens: "ReturnType", Order: 4},
		{Name: "directives", Type: "directive", Cardinality: "*", Order: 5},
	}
	for i, want := range wantElements {
		got := funcDoc.Elements[i]
		if got != want {
			t.Errorf("func_doc.Elements[%d]:\n  got  %+v\n  want %+v", i, got, want)
		}
	}

	if funcDoc.SummaryPrefix != `{name}\b` {
		t.Errorf("func_doc.SummaryPrefix = %q, want %q", funcDoc.SummaryPrefix, `{name}\b`)
	}

	// gen_decl should still be present from defaults with 2 elements.
	genDecl := reg.Lookup("GenDecl", "go")
	if genDecl == nil {
		t.Fatal("schemaRegistry missing GenDecl schema — defaults lost")
	}
	if len(genDecl.Elements) != 2 {
		t.Errorf("gen_decl has %d elements, want 2", len(genDecl.Elements))
	}

	// package_doc should still be present from defaults.
	pkgDoc := reg.Lookup("PkgPath", "go")
	if pkgDoc == nil {
		t.Fatal("schemaRegistry missing PkgPath schema — defaults lost")
	}
}

// TestConfigSchemas_OverrideFromProjectConfig verifies that project config overrides extension defaults.
func TestConfigSchemas_OverrideFromProjectConfig(t *testing.T) {
	ext := loadTestExtension(t, lintGoStyleExtensionYAML)

	projectConfig := `
lint:
  go_style:
    comment_schemas:
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
          - name: parameters
            type: param_section
            required: if_params
            header: "Parameters:"
            item_tokens: ParamName
            order: 3
          - name: returns
            type: return_section
            required: if_returns
            header: "Returns:"
            item_tokens: ReturnType
            order: 4
          - name: directives
            type: directive
            cardinality: "*"
            order: 5
`

	reg := registryFromConfig(t, ext, projectConfig)

	// func_doc should have the overridden schema with 5 elements.
	funcDoc := reg.Lookup("FuncDecl", "go")
	if funcDoc == nil {
		t.Fatal("missing func_doc schema after override")
	}
	if len(funcDoc.Elements) != 5 {
		t.Fatalf("func_doc: %d elements, want 5", len(funcDoc.Elements))
	}

	names := make([]string, len(funcDoc.Elements))
	for i, e := range funcDoc.Elements {
		names[i] = e.Name
	}
	wantNames := []string{"summary", "body", "parameters", "returns", "directives"}
	for i, want := range wantNames {
		if names[i] != want {
			t.Errorf("element[%d] = %q, want %q", i, names[i], want)
		}
	}

	// Verify a project-specific field.
	params := funcDoc.Elements[2]
	if params.Required != "if_params" {
		t.Errorf("parameters.Required = %q, want %q", params.Required, "if_params")
	}
	if params.Header != "Parameters:" {
		t.Errorf("parameters.Header = %q, want %q", params.Header, "Parameters:")
	}

	// gen_decl should still have the default (2 elements) since we only
	// overrode func_doc.
	genDecl := reg.Lookup("GenDecl", "go")
	if genDecl == nil {
		t.Fatal("missing gen_decl schema — defaults lost during override")
	}
	if len(genDecl.Elements) != 2 {
		t.Errorf("gen_decl: %d elements, want 2 (defaults should survive partial override)", len(genDecl.Elements))
	}
}
