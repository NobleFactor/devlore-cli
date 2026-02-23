// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/manifest"
)

func TestManifestLoad_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages.yaml")

	content := `packages:
  - gh
  - jq
  - ripgrep
  - neovim:
      with: [lsp, treesitter]
  - docker:
      with: [rootless, compose]
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 5 {
		t.Fatalf("expected 5 packages, got %d", len(packages))
	}

	// Check simple packages
	if packages[0].Name != "gh" {
		t.Errorf("expected package 0 to be 'gh', got %q", packages[0].Name)
	}
	if len(packages[0].With) != 0 {
		t.Errorf("expected package 0 to have no features, got %v", packages[0].With)
	}

	// Check neovim with features
	if packages[3].Name != "neovim" {
		t.Errorf("expected package 3 to be 'neovim', got %q", packages[3].Name)
	}
	if len(packages[3].With) != 2 {
		t.Errorf("expected neovim to have 2 features, got %d", len(packages[3].With))
	}
	if packages[3].With[0] != "lsp" || packages[3].With[1] != "treesitter" {
		t.Errorf("expected neovim features [lsp, treesitter], got %v", packages[3].With)
	}

	// Check docker with features
	if packages[4].Name != "docker" {
		t.Errorf("expected package 4 to be 'docker', got %q", packages[4].Name)
	}
	if len(packages[4].With) != 2 {
		t.Errorf("expected docker to have 2 features, got %d", len(packages[4].With))
	}
}

func TestManifestLoad_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages.json")

	content := `{
  "packages": [
    "gh",
    "jq",
    {"neovim": {"with": ["lsp", "treesitter"]}}
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(packages))
	}

	// Check simple packages
	if packages[0].Name != "gh" {
		t.Errorf("expected package 0 to be 'gh', got %q", packages[0].Name)
	}

	// Check neovim with features
	if packages[2].Name != "neovim" {
		t.Errorf("expected package 2 to be 'neovim', got %q", packages[2].Name)
	}
	if len(packages[2].With) != 2 {
		t.Errorf("expected neovim to have 2 features, got %d", len(packages[2].With))
	}
}

func TestManifestLoad_ManifestExtension(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "team.manifest")

	content := `packages:
  - kubectl
  - helm
  - terraform:
      with: [aws, gcp]
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(packages))
	}

	if packages[2].Name != "terraform" {
		t.Errorf("expected package 2 to be 'terraform', got %q", packages[2].Name)
	}
	if len(packages[2].With) != 2 {
		t.Errorf("expected terraform to have 2 features, got %d", len(packages[2].With))
	}
}

func TestManifestLoad_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "empty.yaml")

	content := `packages: []`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	if len(m.Packages) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(m.Packages))
	}
}

func TestManifestLoad_NotFound(t *testing.T) {
	_, err := manifest.Load("/nonexistent/path/manifest.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
