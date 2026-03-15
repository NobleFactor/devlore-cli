// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Load ---

func TestLoad_YAML_SimplePackages(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: gh
  - name: jq
  - name: ripgrep
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
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

func TestLoad_YAML_PackagesWithFeatures(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: gh
  - name: neovim
    with: [lsp, treesitter]
  - name: docker
    with:
      - rootless
      - compose
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(m.Packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(m.Packages))
	}

	if m.Packages[0].Name != "gh" {
		t.Errorf("packages[0]: expected 'gh', got %q", m.Packages[0].Name)
	}

	if m.Packages[1].Name != "neovim" {
		t.Errorf("packages[1]: expected 'neovim', got %q", m.Packages[1].Name)
	}
	if len(m.Packages[1].With) != 2 {
		t.Errorf("packages[1]: expected 2 features, got %d", len(m.Packages[1].With))
	}
	if m.Packages[1].With[0] != "lsp" || m.Packages[1].With[1] != "treesitter" {
		t.Errorf("packages[1]: unexpected features: %v", m.Packages[1].With)
	}

	if m.Packages[2].Name != "docker" {
		t.Errorf("packages[2]: expected 'docker', got %q", m.Packages[2].Name)
	}
	if len(m.Packages[2].With) != 2 {
		t.Errorf("packages[2]: expected 2 features, got %d", len(m.Packages[2].With))
	}
}

func TestLoad_JSON(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.json")
	content := `{
  "packages": [
    {"name": "gh"},
    {"name": "jq"},
    {"name": "neovim", "with": ["lsp", "treesitter"]}
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
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

func TestLoad_EmptyPackages(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	if err := os.WriteFile(path, []byte("packages: []\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(m.Packages) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(m.Packages))
	}
}

func TestLoad_NotFound(t *testing.T) {

	_, err := Load("/nonexistent/path/manifest.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// --- Validate ---

func TestValidate_Valid(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: gh
  - name: neovim
    with: [lsp]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err != nil {
		t.Errorf("expected valid manifest, got error: %v", err)
	}
}

func TestValidate_MissingPackages(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	if err := os.WriteFile(path, []byte("something: else\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for missing 'packages' field")
	}
}

func TestValidate_PackagesNotArray(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	if err := os.WriteFile(path, []byte("packages: \"not an array\"\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for 'packages' not being an array")
	}
}

func TestValidate_MissingName(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - with: [lsp]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for missing 'name' field")
	}
}

func TestValidate_EmptyName(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: ""
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for empty package name")
	}
}

func TestValidate_UnknownField(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: neovim
    with: [lsp]
    from: brew
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for unknown field 'from'")
	}
}

func TestValidate_UnknownTopLevelField(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - name: gh
extra: field
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for unknown top-level field")
	}
}

func TestValidate_EntryNotObject(t *testing.T) {

	dir := t.TempDir()
	path := filepath.Join(dir, "packages-manifest.yaml")
	content := `packages:
  - just a string
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := Validate(path); err == nil {
		t.Error("expected error for bare string entry")
	}
}

// --- IsManifestFile ---

func TestIsManifestFile(t *testing.T) {

	tests := []struct {
		filename string
		expected bool
	}{
		{"packages-manifest.yaml", true},
		{"packages-manifest.json", true},
		{"packages.manifest", false},
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

// --- PackageNames ---

func TestPackageNames(t *testing.T) {

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

// --- String ---

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
