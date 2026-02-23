// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfidence_String(t *testing.T) {
	tests := []struct {
		c    Confidence
		want string
	}{
		{ConfidenceHigh, "HIGH"},
		{ConfidenceMedium, "MEDIUM"},
		{ConfidenceLow, "LOW"},
		{Confidence(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.c.String(); got != tt.want {
				t.Errorf("Confidence.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveWithConfidence_LoreRelease(t *testing.T) {
	// Create a temporary registry with a lore package
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "packages", "testpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create lifecycle.yaml
	lifecycleYAML := `name: testpkg
version: "1.0"
description: "Test package"
platforms:
  - Darwin
  - Linux
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{cacheDir: tmpDir}

	pkg, confidence, err := reg.ResolveWithConfidence("testpkg", "Darwin")
	if err != nil {
		t.Fatalf("ResolveWithConfidence() error = %v", err)
	}

	if pkg.Source != SourceLore {
		t.Errorf("pkg.Source = %v, want SourceLore", pkg.Source)
	}

	if confidence != ConfidenceHigh {
		t.Errorf("confidence = %v, want ConfidenceHigh", confidence)
	}
}

func TestResolveWithConfidence_NativePackage(t *testing.T) {
	// Create an empty registry (no lore packages)
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "packages"), 0o755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{cacheDir: tmpDir}

	// Resolve a package that doesn't exist in lore registry
	pkg, confidence, err := reg.ResolveWithConfidence("curl", "Darwin")
	if err != nil {
		t.Fatalf("ResolveWithConfidence() error = %v", err)
	}

	// Should be a native package (synthetic)
	if pkg.Source == SourceLore {
		t.Errorf("pkg.Source = SourceLore, want native source")
	}

	// Confidence depends on whether pm.Available() succeeds
	// In tests without a real PM, it will be LOW
	if confidence != ConfidenceLow && confidence != ConfidenceMedium {
		t.Errorf("confidence = %v, want ConfidenceLow or ConfidenceMedium", confidence)
	}
}

func TestSearchLore(t *testing.T) {
	// Create a temporary registry with packages
	tmpDir := t.TempDir()
	packagesDir := filepath.Join(tmpDir, "packages")

	// Create docker package
	dockerDir := filepath.Join(packagesDir, "docker")
	if err := os.MkdirAll(dockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dockerYAML := `name: docker
version: "24.0"
description: "Container runtime"
platforms: [Darwin, Linux, Windows]
`
	if err := os.WriteFile(filepath.Join(dockerDir, "lifecycle.yaml"), []byte(dockerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create kubectl package
	kubectlDir := filepath.Join(packagesDir, "kubectl")
	if err := os.MkdirAll(kubectlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	kubectlYAML := `name: kubectl
version: "1.28"
description: "Kubernetes CLI"
platforms: [Darwin, Linux, Windows]
`
	if err := os.WriteFile(filepath.Join(kubectlDir, "lifecycle.yaml"), []byte(kubectlYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{cacheDir: tmpDir}

	// Search for "docker"
	results, err := reg.searchLore("docker", 10)
	if err != nil {
		t.Fatalf("searchLore() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("searchLore(docker) returned %d results, want 1", len(results))
	}

	if len(results) > 0 {
		if results[0].Name != "docker" {
			t.Errorf("results[0].Name = %q, want \"docker\"", results[0].Name)
		}
		if results[0].Source != SourceLore {
			t.Errorf("results[0].Source = %v, want SourceLore", results[0].Source)
		}
		if results[0].Confidence != ConfidenceHigh {
			t.Errorf("results[0].Confidence = %v, want ConfidenceHigh", results[0].Confidence)
		}
	}

	// Search for "kube" (partial match)
	results, err = reg.searchLore("kube", 10)
	if err != nil {
		t.Fatalf("searchLore(kube) error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("searchLore(kube) returned %d results, want 1", len(results))
	}

	// Search for non-existent package
	results, err = reg.searchLore("nonexistent", 10)
	if err != nil {
		t.Fatalf("searchLore(nonexistent) error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("searchLore(nonexistent) returned %d results, want 0", len(results))
	}
}

func TestListPackages(t *testing.T) {
	// Create a temporary registry with packages
	tmpDir := t.TempDir()
	packagesDir := filepath.Join(tmpDir, "packages")

	packages := []struct {
		name        string
		version     string
		description string
	}{
		{"docker", "24.0", "Container runtime"},
		{"kubectl", "1.28", "Kubernetes CLI"},
		{"terraform", "1.5", "Infrastructure as code"},
	}

	for _, pkg := range packages {
		pkgDir := filepath.Join(packagesDir, pkg.name)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		yaml := "name: " + pkg.name + "\nversion: \"" + pkg.version + "\"\ndescription: \"" + pkg.description + "\"\nplatforms: [Darwin]\n"
		if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	reg := &Registry{cacheDir: tmpDir}

	results, err := reg.ListPackages()
	if err != nil {
		t.Fatalf("ListPackages() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ListPackages() returned %d results, want 3", len(results))
	}

	// All should be HIGH confidence lore packages
	for _, r := range results {
		if r.Source != SourceLore {
			t.Errorf("package %s Source = %v, want SourceLore", r.Name, r.Source)
		}
		if r.Confidence != ConfidenceHigh {
			t.Errorf("package %s Confidence = %v, want ConfidenceHigh", r.Name, r.Confidence)
		}
	}
}
