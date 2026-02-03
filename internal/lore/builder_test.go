// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
)

// runGraph is a test helper that converts an execution.Graph to Executable slice and calls RunNodes.
func runGraph(ctx context.Context, eng *execution.GraphExecutor, g *execution.Graph) ([]*execution.Result, error) {
	executables := make([]execution.Executable, len(g.Nodes))
	for i, n := range g.Nodes {
		executables[i] = n
	}
	return eng.RunNodes(ctx, executables, g.Edges)
}

func TestBuild_WithNativePMPackage(t *testing.T) {
	// Test that Build creates correct nodes for native PM packages.
	// Native PM packages use the namespaced "pkg-install" operation that works
	// on all platforms. The actual PM is determined at execution time.

	tmpDir := t.TempDir()
	client := lorepackage.New("test", nil, tmpDir)

	// Build with a package name that won't exist in the cache,
	// so it falls back to native PM resolution
	result, err := Build(BuildConfig{
		Packages:       []string{"curl"},
		Platform:       "Linux.Debian",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have at least one node for the install phase
	if len(result.Graph.Nodes) == 0 {
		t.Error("expected at least 1 node, got 0")
	}

	// The first node should be a namespaced package-install operation
	found := false
	for _, node := range result.Graph.Nodes {
		if len(node.Operations) > 0 && node.Operations[0] == "package-install" {
			found = true
			// Verify slot values
			if node.GetSlot("packages") != "curl" {
				t.Errorf("expected packages 'curl', got %q", node.GetSlot("packages"))
			}
			break
		}
	}
	if !found {
		t.Error("expected to find package-install operation")
	}
}

func TestBuild_PlatformDetection(t *testing.T) {
	// Test that platform is correctly resolved and stored in result.
	// All platforms use the namespaced "package-install" operation - the actual PM
	// is determined at execution time by host.PackageManager().
	tmpDir := t.TempDir()
	client := lorepackage.New("test", nil, tmpDir)

	tests := []struct {
		platform string
	}{
		{"Darwin"},
		{"Linux.Debian"},
		{"Linux.Fedora"},
		{"Windows"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			result, err := Build(BuildConfig{
				Packages:       []string{"testpkg"},
				Platform:       tt.platform,
				RegistryClient: client,
			})
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}

			if result.Platform != tt.platform {
				t.Errorf("Platform = %q, want %q", result.Platform, tt.platform)
			}

			// All platforms use the namespaced "package-install" operation
			found := false
			for _, node := range result.Graph.Nodes {
				if len(node.Operations) > 0 && node.Operations[0] == "package-install" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected to find package-install operation")
			}
		})
	}
}

func TestBuildFromManifest(t *testing.T) {
	// Test building from a packages-manifest.yaml file
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages-manifest.yaml")

	manifest := `packages:
  - curl
  - jq
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := BuildFromManifest(manifestPath, "Linux.Debian")
	if err != nil {
		t.Fatalf("BuildFromManifest failed: %v", err)
	}

	if len(result.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(result.Packages))
	}

	if result.Packages[0] != "curl" || result.Packages[1] != "jq" {
		t.Errorf("packages = %v, want [curl, jq]", result.Packages)
	}
}

func TestBuildFromPackages(t *testing.T) {
	// Test the BuildFromPackages helper
	result, err := BuildFromPackages([]string{"git", "vim"}, "Darwin")
	if err != nil {
		t.Fatalf("BuildFromPackages failed: %v", err)
	}

	if len(result.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(result.Packages))
	}

	if result.Platform != "Darwin" {
		t.Errorf("Platform = %q, want Darwin", result.Platform)
	}
}

func TestBuild_EmptyPackageList(t *testing.T) {
	// Test that empty package list returns error
	_, err := Build(BuildConfig{
		Packages: []string{},
		Platform: "Darwin",
	})
	if err == nil {
		t.Error("expected error for empty package list")
	}
}

func TestBuild_MutuallyExclusiveConfig(t *testing.T) {
	// Test that specifying both ManifestPath and Packages returns error
	_, err := Build(BuildConfig{
		ManifestPath: "/some/path.yaml",
		Packages:     []string{"pkg"},
		Platform:     "Darwin",
	})
	if err == nil {
		t.Error("expected error when both ManifestPath and Packages specified")
	}
}

func TestEngineRunsPackageInstallOperations(t *testing.T) {
	// Integration test: build graph and run through engine with DryRun
	reg := execution.NewOperationRegistry()

	// Register all operations (file + package)
	for _, op := range execution.AllOps() {
		reg.Register(op)
	}

	eng := execution.NewGraphExecutor(reg, execution.ExecutorOptions{DryRun: true})

	// Create a graph with a namespaced package-install node
	node := &execution.Node{
		ID:         "package-install-testpkg",
		Operations: []string{"package-install"},
	}
	node.SetSlotImmediate("packages", "testpkg")
	graph := &execution.Graph{
		Nodes: []*execution.Node{node},
	}

	results, err := runGraph(context.Background(), eng, graph)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != execution.ResultCompleted {
		t.Errorf("expected status completed, got %s (error: %v)", results[0].Status, results[0].Error)
	}
}

func TestEngineRunsNamespacedPackageOps(t *testing.T) {
	// Test that all namespaced package operations can execute in dry-run mode
	reg := execution.NewOperationRegistry()
	for _, op := range execution.AllOps() {
		reg.Register(op)
	}

	eng := execution.NewGraphExecutor(reg, execution.ExecutorOptions{DryRun: true})

	// All platforms use these four namespaced package operations
	ops := []string{
		"package-install", "package-upgrade", "package-remove", "package-update",
	}

	for _, opName := range ops {
		t.Run(opName, func(t *testing.T) {
			node := &execution.Node{
				ID:         "test-" + opName,
				Operations: []string{opName},
			}
			if opName != "package-update" {
				node.SetSlotImmediate("packages", "testpkg")
			}

			graph := &execution.Graph{
				Nodes: []*execution.Node{node},
			}

			results, err := runGraph(context.Background(), eng, graph)
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			if results[0].Status != execution.ResultCompleted {
				t.Errorf("expected completed, got %s", results[0].Status)
			}
		})
	}
}

func TestNativePMNodeMetadata(t *testing.T) {
	// Test that native PM nodes have correct metadata.
	// With namespaced operations, manager is NOT set in metadata - it's determined
	// at execution time by host.PackageManager(). Only packages and phase are set.
	tmpDir := t.TempDir()
	client := lorepackage.New("test", nil, tmpDir)

	result, err := Build(BuildConfig{
		Packages:       []string{"nginx"},
		Platform:       "Linux.Debian",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Find the install node (uses namespaced "package-install" operation)
	var installNode *execution.Node
	for _, node := range result.Graph.Nodes {
		if len(node.Operations) > 0 && node.Operations[0] == "package-install" {
			installNode = node
			break
		}
	}

	if installNode == nil {
		t.Fatal("no package-install node found")
	}

	// Verify required slots
	if installNode.GetSlot("packages") == "" {
		t.Error("expected packages slot to be set")
	}
	if installNode.GetSlot("phase") == "" {
		t.Error("expected phase slot to be set")
	}
	// Note: manager is NOT set for namespaced operations
	// The PM is determined at execution time by host.PackageManager()
}

func TestDarwinPackageManagerPrefix(t *testing.T) {
	// Test that brew:, cask:, and port: prefixes are correctly parsed
	// This tests the prefix parsing logic for macOS package managers

	tests := []struct {
		input      string
		wantPkg    string
		wantPrefix string
	}{
		{"brew:wget", "wget", "brew"},
		{"cask:iterm2", "iterm2", "cask"},
		{"port:wget", "wget", "port"},
		{"wget", "wget", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pkg, prefix := lorepackage.ParsePackagePrefix(tt.input)
			if pkg != tt.wantPkg {
				t.Errorf("ParsePackagePrefix(%q) pkg = %q, want %q", tt.input, pkg, tt.wantPkg)
			}
			if prefix != tt.wantPrefix {
				t.Errorf("ParsePackagePrefix(%q) prefix = %q, want %q", tt.input, prefix, tt.wantPrefix)
			}
		})
	}
}
