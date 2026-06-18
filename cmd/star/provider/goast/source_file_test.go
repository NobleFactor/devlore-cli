// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

const testSource = `package example

// Provider provides operations.
type Provider struct {
	name string
}

// NewProvider creates a new Provider.
//
// Parameters:
//   - name: the provider name.
//
// Returns:
//   - *Provider: the new provider.
func NewProvider(name string) *Provider {
	return &Provider{name: name}
}

// region EXPORTED METHODS

// Backup backs up data.
//
// Parameters:
//   - path: the file path to back up.
//
// Returns:
//   - string: the backup location.
//   - error: non-nil if the backup fails.
func (p *Provider) Backup(path string) (string, error) {
	return path, nil
}

// Restore restores data.
//
// Parameters:
//   - path: the file path to restore.
//
// Returns:
//   - error: non-nil if the restore fails.
func (p *Provider) Restore(path string) error {
	return nil
}

// endregion

// DefaultName is the default provider name.
const DefaultName = "default"

// MaxRetries is the maximum retry count.
var MaxRetries = 3
`

func TestSourceFile_Decls(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	decls := sf.Decls
	if len(decls) == 0 {
		t.Fatal("expected declarations")
	}

	for _, decl := range decls {
		if decl.DeclKind() == "" {
			t.Error("declaration with empty kind")
		}
		if decl.DeclKind() != "comment" && decl.DeclName() == "" {
			t.Errorf("non-comment declaration with empty name (kind=%s)", decl.DeclKind())
		}
	}
}

func TestSourceFile_Types(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	// --- Iterate types ---

	types := sf.Types
	if len(types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(types))
	}
	if types[0].Name != "Provider" {
		t.Errorf("expected Provider, got %s", types[0].Name)
	}
	if types[0].Comment().Text() == nil {
		t.Error("expected doc comment on Provider")
	}

	// --- Iterate methods on type ---

	methods := types[0].Methods
	if len(methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(methods))
	}
	for _, m := range methods {
		if m.Name == "" {
			t.Error("method with empty name")
		}
		if m.Comment().Text() == nil {
			t.Errorf("expected doc comment on method %s", m.Name)
		}
		if m.Returns == "" {
			t.Errorf("expected returns on method %s", m.Name)
		}
		for _, p := range m.Params {
			if p.Name == "" {
				t.Errorf("param with empty name on method %s", m.Name)
			}
			if p.Type == "" {
				t.Errorf("param with empty type on method %s", m.Name)
			}
		}
	}

	// --- Get by name ---

	provider := sf.GetType("Provider")
	if provider == nil {
		t.Fatal("GetType(Provider) returned nil")
	}

	backup := provider.GetMethod("Backup")
	if backup == nil {
		t.Fatal("GetMethod(Backup) returned nil")
	}
	if backup.Name != "Backup" {
		t.Errorf("expected Backup, got %s", backup.Name)
	}
	if len(backup.Params) != 1 {
		t.Fatalf("expected 1 param on Backup, got %d", len(backup.Params))
	}
	if backup.Params[0].Name != "path" {
		t.Errorf("expected param name 'path', got %s", backup.Params[0].Name)
	}
}

func TestSourceFile_Funcs(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	funcs := sf.Funcs
	if len(funcs) != 1 {
		t.Fatalf("expected 1 func, got %d", len(funcs))
	}
	if funcs[0].Name != "NewProvider" {
		t.Errorf("expected NewProvider, got %s", funcs[0].Name)
	}
	if funcs[0].Comment().Text() == nil {
		t.Error("expected doc comment on NewProvider")
	}

	newProvider := sf.GetFunc("NewProvider")
	if newProvider == nil {
		t.Fatal("GetFunc(NewProvider) returned nil")
	}
}

func TestSourceFile_Consts(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	consts := sf.Consts
	if len(consts) != 1 {
		t.Fatalf("expected 1 const group, got %d", len(consts))
	}
	if consts[0].Comment().Text() == nil {
		t.Error("expected doc comment on const")
	}

	entries := consts[0].Entries
	if len(entries) != 1 {
		t.Fatalf("expected 1 const entry, got %d", len(entries))
	}
	if entries[0].Name != "DefaultName" {
		t.Errorf("expected DefaultName, got %s", entries[0].Name)
	}
	if entries[0].Value != "default" {
		t.Errorf("expected 'default', got %s", entries[0].Value)
	}
}

func TestSourceFile_Vars(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	vars := sf.Vars
	if len(vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(vars))
	}
	if vars[0].Name != "MaxRetries" {
		t.Errorf("expected MaxRetries, got %s", vars[0].Name)
	}
	if vars[0].Comment().Text() == nil {
		t.Error("expected doc comment on MaxRetries")
	}
}

func TestSourceFile_FloatingComments(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	// --- Floating comments appear in Decls at their source position ---

	var comments []*CommentDecl
	for _, decl := range sf.Decls {
		if cd, ok := decl.(*CommentDecl); ok {
			comments = append(comments, cd)
		}
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 floating comments, got %d", len(comments))
	}
	if comments[0].Text() != "region EXPORTED METHODS" {
		t.Errorf("expected region marker, got %q", comments[0].Text())
	}
	if comments[1].Text() != "endregion" {
		t.Errorf("expected endregion marker, got %q", comments[1].Text())
	}

	// --- Floating comments have correct Decl interface values ---

	if comments[0].DeclName() != "" {
		t.Errorf("expected empty name, got %q", comments[0].DeclName())
	}
	if comments[0].DeclKind() != "comment" {
		t.Errorf("expected kind 'comment', got %q", comments[0].DeclKind())
	}
	if comments[0].DeclComment().Text() != nil {
		t.Error("expected nil text on floating comment DeclComment")
	}

	// --- Source order: region marker appears between NewProvider and Backup ---

	decls := sf.Decls
	var order []string
	for _, d := range decls {
		switch d.DeclKind() {
		case "comment":
			order = append(order, "comment:"+d.(*CommentDecl).Text())
		default:
			order = append(order, d.DeclKind()+":"+d.DeclName())
		}
	}

	// Find the region marker and verify it's between NewProvider and Backup.
	regionIdx := -1
	newProviderIdx := -1
	backupIdx := -1
	for i, s := range order {
		switch s {
		case "comment:region EXPORTED METHODS":
			regionIdx = i
		case "func:NewProvider":
			newProviderIdx = i
		case "method:Backup":
			backupIdx = i
		}
	}
	if regionIdx < 0 {
		t.Fatal("region marker not found in decls")
	}
	if newProviderIdx < 0 || backupIdx < 0 {
		t.Fatal("NewProvider or Backup not found in decls")
	}
	if regionIdx <= newProviderIdx || regionIdx >= backupIdx {
		t.Errorf("region marker at index %d not between NewProvider(%d) and Backup(%d)",
			regionIdx, newProviderIdx, backupIdx)
	}
}

func TestSourceFile_CleanupAndSave(t *testing.T) {
	sf, err := LoadSourceFile(testSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	// Inject context with styling config.
	sf.schemaReg = doctaxonomy.DefaultRegistry()
	sf.spacing = DefaultSpacingRules()
	sf.width = 120

	// Write to temp file.
	tmp := filepath.Join(t.TempDir(), "test.go")
	sf.filename = tmp

	sf.Cleanup()
	if err := sf.SaveAs(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	result, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(result)

	// The output should be valid Go — parse it.
	_, err = LoadSourceFile(got)
	if err != nil {
		t.Fatalf("Save output is not valid Go: %v\n---\n%s", err, got)
	}

	// Package name should survive round-trip.
	if !strings.Contains(got, "package example") {
		t.Error("missing package declaration")
	}

	// All declarations should survive round-trip.
	for _, name := range []string{"Provider", "NewProvider", "Backup", "Restore", "DefaultName", "MaxRetries"} {
		if !strings.Contains(got, name) {
			t.Errorf("missing declaration: %s", name)
		}
	}

	// Region markers should survive round-trip.
	if !strings.Contains(got, "region EXPORTED METHODS") {
		t.Error("missing region marker")
	}
	if !strings.Contains(got, "endregion") {
		t.Error("missing endregion marker")
	}
}

const allStylesSource = `// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package example provides test coverage for all comment styles.
package example

import "fmt"

// =============================================================================
// Provider
// =============================================================================

// Provider provides operations.
type Provider struct {
	name string
}

// NewProvider creates a new Provider.
func NewProvider(name string) *Provider {
	return &Provider{name: name}
}

// region EXPORTED METHODS

// Fallible actions

// Backup backs up data.
func (p *Provider) Backup(path string) (string, error) {
	return path, nil
}

func (p *Provider) Undocumented(x int) error {
	return nil
}

// endregion

// DefaultName is the default provider name.
const DefaultName = "default"

// MaxRetries is the maximum retry count.
var MaxRetries = 3

func helper() {
	fmt.Println("hi")
}
`

func TestSourceFile_AllStyles_RoundTrip(t *testing.T) {
	sf, err := LoadSourceFile(allStylesSource)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	sf.schemaReg = doctaxonomy.DefaultRegistry()
	sf.spacing = DefaultSpacingRules()
	sf.width = 120

	tmp := filepath.Join(t.TempDir(), "allstyles.go")
	sf.filename = tmp

	sf.Cleanup()
	if err := sf.SaveAs(tmp); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	result, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(result)

	// --- Must be valid Go ---

	sf2, err := LoadSourceFile(got)
	if err != nil {
		t.Fatalf("output is not valid Go:\n%v\n---OUTPUT---\n%s", err, got)
	}

	// --- All declarations survive round-trip ---

	for _, name := range []string{"Provider", "NewProvider", "Backup", "Undocumented", "DefaultName", "MaxRetries", "helper"} {
		if !strings.Contains(got, name) {
			t.Errorf("missing declaration: %s", name)
		}
	}

	// --- Undocumented func gets TODO stub ---

	undocMethod := sf2.GetType("Provider").GetMethod("Undocumented")
	if undocMethod == nil {
		t.Fatal("Undocumented method not found in re-parsed tree")
	}
	if undocMethod.Comment().Text() == nil {
		t.Error("Undocumented method should have a doc comment after Cleanup (TODO stub)")
	}

	// --- Undocumented top-level func gets TODO stub ---

	helperFunc := sf2.GetFunc("helper")
	if helperFunc == nil {
		t.Fatal("helper func not found in re-parsed tree")
	}
	if helperFunc.Comment().Text() == nil {
		t.Error("helper func should have a doc comment after Cleanup (TODO stub)")
	}

	t.Logf("Promise:\n%s", got)
}

func nobleFactorRegistry() *doctaxonomy.SchemaRegistry {
	reg := doctaxonomy.DefaultRegistry()
	reg.Register(doctaxonomy.CommentSchema{
		Name: "func_doc", Format: "go", NodeType: "FuncDecl",
		SummaryPrefix: `{name}\b`,
		Elements: []doctaxonomy.SchemaElement{
			{Name: "summary", Production: "item", Consumes: "Paragraph / Heading", Prefix: "{name}", Required: "true", Order: 1},
			{Name: "body", Production: "item", Consumes: "*(Paragraph / Code / Heading)", Order: 2},
			{Name: "parameters", Production: "list", Header: "Parameters:", Condition: "params", Required: "if_condition", Slots: "params", SlotPrefix: "{slot}", Order: 3},
			{Name: "returns", Production: "list", Header: "Returns:", Condition: "returns", Required: "if_condition", Slots: "returns", SlotPrefix: "{slot}", Order: 4},
			{Name: "directives", Production: "item", Consumes: "*Paragraph", Prefix: "+", Order: 5},
		},
	})
	return reg
}

func TestSourceFile_SingleParamFunction_GetsParametersStub(t *testing.T) {
	src := `package example

// NewAccessor creates a ConfigAccessor for the given value.
func NewAccessor(v interface{}) *ConfigAccessor {
	return nil
}

type ConfigAccessor struct{}
`
	sf, err := LoadSourceFile(src)
	if err != nil {
		t.Fatalf("LoadSourceFile: %v", err)
	}

	sf.schemaReg = nobleFactorRegistry()
	sf.spacing = DefaultSpacingRules()
	sf.width = 120

	tmp := filepath.Join(t.TempDir(), "test.go")
	sf.filename = tmp

	sf.Cleanup()
	if err := sf.SaveAs(tmp); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	result, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(result)

	if !strings.Contains(got, "Parameters:") {
		t.Errorf("expected Parameters section with TODO stub\n---OUTPUT---\n%s", got)
	}
	if !strings.Contains(got, "Returns:") {
		t.Errorf("expected Returns section with TODO stub\n---OUTPUT---\n%s", got)
	}

	t.Logf("Promise:\n%s", got)
}

func TestSourceFile_RealFile_RoundTrip(t *testing.T) {
	files := []string{
		"../../config/accessor.go",
		"../../config/config.go",
		"../../config/element.go",
		"../../config/root.go",
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("cannot read %s: %v", path, err)
			}

			sf, err := LoadSourceFile(string(content))
			if err != nil {
				t.Fatalf("LoadSourceFile: %v", err)
			}

			sf.schemaReg = doctaxonomy.DefaultRegistry()
			sf.spacing = DefaultSpacingRules()
			sf.width = 120

			tmp := filepath.Join(t.TempDir(), name)
			sf.filename = tmp

			sf.Cleanup()
			if err := sf.SaveAs(tmp); err != nil {
				t.Fatalf("SaveAs: %v", err)
			}

			result, err := os.ReadFile(tmp)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			got := string(result)

			_, err = LoadSourceFile(got)
			if err != nil {
				t.Fatalf("output is not valid Go:\n%v\n---FIRST 80 LINES---\n%s", err, firstNLines(got, 80))
			}

			if !strings.Contains(got, "package config") {
				t.Error("missing package declaration")
			}
		})
	}
}

func firstNLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
