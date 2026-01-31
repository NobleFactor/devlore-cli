// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

func TestParse_YAML_SimplePackages(t *testing.T) {
	yaml := `packages:
  - gh
  - jq
  - ripgrep
`
	m, err := Parse([]byte(yaml), "packages-manifest.yaml")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(m.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(m.Packages))
	}

	expected := []string{"gh", "jq", "ripgrep"}
	for i, pkg := range m.Packages {
		if pkg.Name != expected[i] {
			t.Errorf("packages[%d]: expected %q, got %q", i, expected[i], pkg.Name)
		}
		if len(pkg.With) != 0 {
			t.Errorf("packages[%d]: expected no features, got %v", i, pkg.With)
		}
	}
}

func TestParse_YAML_PackagesWithFeatures(t *testing.T) {
	yaml := `packages:
  - gh
  - neovim:
      with: [lsp, treesitter]
  - docker:
      with:
        - rootless
        - compose
`
	m, err := Parse([]byte(yaml), "packages-manifest.yaml")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(m.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(m.Packages))
	}

	// Check gh (simple)
	if m.Packages[0].Name != "gh" {
		t.Errorf("packages[0]: expected 'gh', got %q", m.Packages[0].Name)
	}

	// Check neovim (with features)
	if m.Packages[1].Name != "neovim" {
		t.Errorf("packages[1]: expected 'neovim', got %q", m.Packages[1].Name)
	}
	if len(m.Packages[1].With) != 2 {
		t.Errorf("packages[1]: expected 2 features, got %d", len(m.Packages[1].With))
	}
	if m.Packages[1].With[0] != "lsp" || m.Packages[1].With[1] != "treesitter" {
		t.Errorf("packages[1]: unexpected features: %v", m.Packages[1].With)
	}

	// Check docker (with features)
	if m.Packages[2].Name != "docker" {
		t.Errorf("packages[2]: expected 'docker', got %q", m.Packages[2].Name)
	}
	if len(m.Packages[2].With) != 2 {
		t.Errorf("packages[2]: expected 2 features, got %d", len(m.Packages[2].With))
	}
}

func TestParse_JSON(t *testing.T) {
	json := `{
  "packages": [
    "gh",
    "jq",
    {"neovim": {"with": ["lsp", "treesitter"]}}
  ]
}`
	m, err := Parse([]byte(json), "packages-manifest.json")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(m.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(m.Packages))
	}

	if m.Packages[0].Name != "gh" {
		t.Errorf("packages[0]: expected 'gh', got %q", m.Packages[0].Name)
	}
	if m.Packages[2].Name != "neovim" {
		t.Errorf("packages[2]: expected 'neovim', got %q", m.Packages[2].Name)
	}
	if len(m.Packages[2].With) != 2 {
		t.Errorf("packages[2]: expected 2 features, got %d", len(m.Packages[2].With))
	}
}

func TestParse_EmptyWithOptions(t *testing.T) {
	yaml := `packages:
  - neovim:
`
	m, err := Parse([]byte(yaml), "packages-manifest.yaml")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(m.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(m.Packages))
	}

	if m.Packages[0].Name != "neovim" {
		t.Errorf("expected 'neovim', got %q", m.Packages[0].Name)
	}
	if len(m.Packages[0].With) != 0 {
		t.Errorf("expected no features, got %v", m.Packages[0].With)
	}
}

func TestValidate_Valid(t *testing.T) {
	yaml := `packages:
  - gh
  - neovim:
      with: [lsp]
`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err != nil {
		t.Errorf("expected valid manifest, got error: %v", err)
	}
}

func TestValidate_MissingPackages(t *testing.T) {
	yaml := `something: else`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for missing 'packages' field")
	}
}

func TestValidate_PackagesNotArray(t *testing.T) {
	yaml := `packages: "not an array"`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for 'packages' not being an array")
	}
}

func TestValidate_EmptyPackageName(t *testing.T) {
	yaml := `packages:
  - ""
`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for empty package name")
	}
}

func TestValidate_MultipleKeysInPackage(t *testing.T) {
	yaml := `packages:
  - neovim:
      with: [lsp]
    docker:
      with: [rootless]
`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for multiple keys in package object")
	}
}

func TestValidate_UnknownOption(t *testing.T) {
	yaml := `packages:
  - neovim:
      with: [lsp]
      from: brew
`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for unknown option 'from'")
	}
}

func TestValidate_UnknownTopLevelField(t *testing.T) {
	yaml := `packages:
  - gh
extra: field
`
	err := ValidateBytes([]byte(yaml), "packages-manifest.yaml")
	if err == nil {
		t.Error("expected error for unknown top-level field")
	}
}

func TestIsManifestFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"packages-manifest.yaml", true},
		{"packages-manifest.json", true},
		{"packages.manifest", true},
		{"packages.yaml", false},
		{"manifest.yaml", false},
		{".packages-manifest.yaml", false},
		{"dir/packages-manifest.yaml", true},
	}

	for _, tc := range tests {
		got := IsManifestFile(tc.filename)
		if got != tc.expected {
			t.Errorf("IsManifestFile(%q): expected %v, got %v", tc.filename, tc.expected, got)
		}
	}
}

func TestLoad_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")

	content := `packages:
  - gh
  - neovim:
      with: [lsp]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(m.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(m.Packages))
	}
}

func TestPackageEntry_String(t *testing.T) {
	tests := []struct {
		entry    PackageEntry
		expected string
	}{
		{PackageEntry{Name: "gh"}, "gh"},
		{PackageEntry{Name: "neovim", With: []string{"lsp"}}, "neovim --with lsp"},
		{PackageEntry{Name: "docker", With: []string{"rootless", "compose"}}, "docker --with rootless --with compose"},
	}

	for _, tc := range tests {
		got := tc.entry.String()
		if got != tc.expected {
			t.Errorf("String(): expected %q, got %q", tc.expected, got)
		}
	}
}

func TestPackagesManifest_PackageNames(t *testing.T) {
	m := &PackagesManifest{
		Packages: []PackageEntry{
			{Name: "gh"},
			{Name: "neovim", With: []string{"lsp"}},
			{Name: "docker"},
		},
	}

	names := m.PackageNames()
	expected := []string{"gh", "neovim", "docker"}

	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d]: expected %q, got %q", i, expected[i], name)
		}
	}
}

// ============================================================================
// Builder Tests
// ============================================================================

func TestBuilder_BuildGraphFromManifest(t *testing.T) {
	builder := NewBuilder()
	manifest := &PackagesManifest{
		Packages: []PackageEntry{
			{Name: "gh"},
			{Name: "neovim", With: []string{"lsp", "treesitter"}},
			{Name: "docker", With: []string{"rootless"}},
		},
	}

	graph, err := builder.BuildGraphFromManifest(context.TODO(), manifest, defaultBuildOpts())
	if err != nil {
		t.Fatalf("BuildGraphFromManifest failed: %v", err)
	}

	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(graph.Nodes))
	}

	// Check first node (simple package)
	if graph.Nodes[0].ID != "gh" {
		t.Errorf("nodes[0].ID: expected 'gh', got %q", graph.Nodes[0].ID)
	}
	if graph.Nodes[0].Metadata["package"] != "gh" {
		t.Errorf("nodes[0].Metadata['package']: expected 'gh', got %q", graph.Nodes[0].Metadata["package"])
	}

	// Check second node (with features)
	if graph.Nodes[1].ID != "neovim" {
		t.Errorf("nodes[1].ID: expected 'neovim', got %q", graph.Nodes[1].ID)
	}
	if graph.Nodes[1].Metadata["feature_count"] != "2" {
		t.Errorf("nodes[1].Metadata['feature_count']: expected '2', got %q", graph.Nodes[1].Metadata["feature_count"])
	}
	if graph.Nodes[1].Metadata["feature.0"] != "lsp" {
		t.Errorf("nodes[1].Metadata['feature.0']: expected 'lsp', got %q", graph.Nodes[1].Metadata["feature.0"])
	}

	// Check operations (four-phase pipeline)
	expectedOps := []string{"prepare", "install", "provision", "verify"}
	for i, node := range graph.Nodes {
		if len(node.Operations) != 4 {
			t.Errorf("nodes[%d].Operations: expected 4 ops, got %d", i, len(node.Operations))
		}
		for j, op := range node.Operations {
			if op != expectedOps[j] {
				t.Errorf("nodes[%d].Operations[%d]: expected %q, got %q", i, j, expectedOps[j], op)
			}
		}
	}
}

func TestBuilder_BuildGraph_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")

	content := `packages:
  - gh
  - neovim:
      with: [lsp]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	builder := NewBuilder()
	graph, err := builder.BuildSubgraph(context.TODO(), path, defaultBuildOpts())
	if err != nil {
		t.Fatalf("BuildSubgraph failed: %v", err)
	}

	if len(graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(graph.Nodes))
	}

	if graph.Nodes[0].ID != "gh" {
		t.Errorf("nodes[0].ID: expected 'gh', got %q", graph.Nodes[0].ID)
	}
	if graph.Nodes[1].ID != "neovim" {
		t.Errorf("nodes[1].ID: expected 'neovim', got %q", graph.Nodes[1].ID)
	}
}

func TestBuilder_BuildGraph_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")

	// Missing 'packages' field
	content := `invalid: content`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	builder := NewBuilder()
	_, err := builder.BuildSubgraph(context.TODO(), path, defaultBuildOpts())
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func defaultBuildOpts() execution.BuildOptions {
	return execution.BuildOptions{
		DryRun:   false,
		Features: nil,
		Data:     make(map[string]any),
	}
}
