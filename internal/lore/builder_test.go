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

// runGraph is a test helper that calls RunNodes with the graph's nodes and edges.
func runGraph(ctx context.Context, eng *execution.GraphExecutor, g *execution.Graph) ([]*execution.Result, error) {
	return eng.RunNodes(ctx, g.Nodes, g.Edges)
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
		if node.Operation == "package-install" {
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
				if node.Operation == "package-install" {
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
		Operation: "package-install",
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
				Operation: opName,
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
		if node.Operation == "package-install" {
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

// =============================================================================
// Phase-aware builder tests
// =============================================================================

// createLorePackage creates a lore package fixture in a temp directory with
// the given phase scripts. Returns the registry client and package name.
func createLorePackage(t *testing.T, pkgName string, scripts map[string]string) *lorepackage.Registry {
	t.Helper()
	tmpDir := t.TempDir()

	// Create package directory
	pkgDir := filepath.Join(tmpDir, "packages", pkgName)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write lifecycle.yaml
	lifecycle := "name: " + pkgName + "\nversion: 1.0.0\nplatforms: [Darwin]\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycle), 0644); err != nil {
		t.Fatal(err)
	}

	// Write phase scripts
	for relPath, content := range scripts {
		absPath := filepath.Join(pkgDir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return lorepackage.New("test", nil, tmpDir)
}

func TestBuildPhased_NativePMCreatesPhases(t *testing.T) {
	// Native PM packages should create Phase entries for each lifecycle phase
	// that has actions.
	tmpDir := t.TempDir()
	client := lorepackage.New("test", nil, tmpDir)

	result, err := Build(BuildConfig{
		Packages:       []string{"curl"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Native PM only has the "install" phase (required phase for deploy).
	if len(result.Graph.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(result.Graph.Phases))
	}

	phase := result.Graph.Phases[0]
	if phase.Name != "install" {
		t.Errorf("expected phase name 'install', got %q", phase.Name)
	}
	if phase.ID != "phase.curl.install" {
		t.Errorf("expected phase ID 'phase.curl.install', got %q", phase.ID)
	}
	if phase.Status != execution.PhasePending {
		t.Errorf("expected status pending, got %q", phase.Status)
	}
	if len(phase.NodeIDs) != 1 {
		t.Errorf("expected 1 node ID, got %d", len(phase.NodeIDs))
	}
	if phase.Compensate != "" {
		t.Errorf("native PM phase should not have compensate, got %q", phase.Compensate)
	}
}

func TestBuildPhased_LorePackageForwardOnly(t *testing.T) {
	// Lore package with only forward() — no compensate, no configure.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def forward(package, system, plan):
    plan.package.install(package.name)
`,
	})

	result, err := Build(BuildConfig{
		Packages:       []string{"testpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(result.Graph.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(result.Graph.Phases))
	}

	phase := result.Graph.Phases[0]
	if phase.Name != "install" {
		t.Errorf("expected phase name 'install', got %q", phase.Name)
	}
	if len(phase.NodeIDs) == 0 {
		t.Error("expected at least 1 node in phase")
	}
	if phase.Compensate != "" {
		t.Errorf("expected no compensate, got %q", phase.Compensate)
	}
	if phase.Retry != nil {
		t.Error("expected no retry policy")
	}
}

func TestBuildPhased_LorePackageWithCompensate(t *testing.T) {
	// Lore package with forward() and compensate() — should create a
	// compensating phase with its own nodes.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def forward(package, system, plan):
    plan.package.install(package.name)

def compensate(package, system, plan):
    plan.package.remove(package.name)
`,
	})

	result, err := Build(BuildConfig{
		Packages:       []string{"testpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 2 phases: forward + compensate.
	if len(result.Graph.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(result.Graph.Phases))
	}

	forward := result.Graph.Phases[0]
	compensate := result.Graph.Phases[1]

	if forward.Name != "install" {
		t.Errorf("forward phase name = %q, want 'install'", forward.Name)
	}
	if forward.Compensate != compensate.ID {
		t.Errorf("forward.Compensate = %q, want %q", forward.Compensate, compensate.ID)
	}

	if compensate.Name != "install.compensate" {
		t.Errorf("compensate phase name = %q, want 'install.compensate'", compensate.Name)
	}
	if len(compensate.NodeIDs) == 0 {
		t.Error("expected at least 1 node in compensate phase")
	}

	// Verify compensate phase has package-remove nodes.
	nodeSet := make(map[string]bool)
	for _, id := range compensate.NodeIDs {
		nodeSet[id] = true
	}
	foundRemove := false
	for _, n := range result.Graph.Nodes {
		if nodeSet[n.ID] && n.Operation == "package-remove" {
			foundRemove = true
			break
		}
	}
	if !foundRemove {
		t.Error("expected package-remove node in compensate phase")
	}
}

func TestBuildPhased_LorePackageWithConfigure(t *testing.T) {
	// Lore package with configure() hook — should set retry policy on phase.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def configure(phase):
    phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s", max_delay="30s")

def forward(package, system, plan):
    plan.package.install(package.name)
`,
	})

	result, err := Build(BuildConfig{
		Packages:       []string{"testpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(result.Graph.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(result.Graph.Phases))
	}

	phase := result.Graph.Phases[0]
	if phase.Retry == nil {
		t.Fatal("expected retry policy")
	}
	if phase.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", phase.Retry.MaxAttempts)
	}
	if phase.Retry.Backoff != execution.BackoffExponential {
		t.Errorf("Backoff = %q, want exponential", phase.Retry.Backoff)
	}
	if phase.Retry.InitialDelay != "1s" {
		t.Errorf("InitialDelay = %q, want '1s'", phase.Retry.InitialDelay)
	}
	if phase.Retry.MaxDelay != "30s" {
		t.Errorf("MaxDelay = %q, want '30s'", phase.Retry.MaxDelay)
	}
}

func TestBuildPhased_LorePackageFullSaga(t *testing.T) {
	// Full saga: forward + compensate + configure on multiple phases.
	client := createLorePackage(t, "ripgrep", map[string]string{
		"Darwin/Deploy/install.star": `
def configure(phase):
    phase.retry(max_attempts=2, backoff="linear", initial_delay="500ms")

def forward(package, system, plan):
    plan.package.install(package.name)

def compensate(package, system, plan):
    plan.package.remove(package.name)
`,
		"Darwin/Deploy/provision.star": `
def forward(package, system, plan):
    plan.shell(command="ln -sf /opt/rg/completions/_rg ~/.zsh/completions/_rg")

def compensate(package, system, plan):
    plan.shell(command="rm -f ~/.zsh/completions/_rg")
`,
	})

	result, err := Build(BuildConfig{
		Packages:       []string{"ripgrep"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 4 phases: install, install.compensate, provision, provision.compensate
	if len(result.Graph.Phases) != 4 {
		var names []string
		for _, p := range result.Graph.Phases {
			names = append(names, p.Name)
		}
		t.Fatalf("expected 4 phases, got %d: %v", len(result.Graph.Phases), names)
	}

	// Verify phase order and compensate references.
	installPhase := result.Graph.Phases[0]
	installComp := result.Graph.Phases[1]
	provisionPhase := result.Graph.Phases[2]
	provisionComp := result.Graph.Phases[3]

	if installPhase.Name != "install" {
		t.Errorf("phase[0] = %q, want 'install'", installPhase.Name)
	}
	if installPhase.Compensate != installComp.ID {
		t.Errorf("install.Compensate = %q, want %q", installPhase.Compensate, installComp.ID)
	}
	if installPhase.Retry == nil || installPhase.Retry.MaxAttempts != 2 {
		t.Error("install phase should have retry with max_attempts=2")
	}

	if provisionPhase.Name != "provision" {
		t.Errorf("phase[2] = %q, want 'provision'", provisionPhase.Name)
	}
	if provisionPhase.Compensate != provisionComp.ID {
		t.Errorf("provision.Compensate = %q, want %q", provisionPhase.Compensate, provisionComp.ID)
	}
	if provisionPhase.Retry != nil {
		t.Error("provision phase should not have retry policy")
	}

	// Verify nodes are correctly assigned to phases.
	if len(installPhase.NodeIDs) == 0 {
		t.Error("install phase has no nodes")
	}
	if len(installComp.NodeIDs) == 0 {
		t.Error("install.compensate phase has no nodes")
	}
	if len(provisionPhase.NodeIDs) == 0 {
		t.Error("provision phase has no nodes")
	}
	if len(provisionComp.NodeIDs) == 0 {
		t.Error("provision.compensate phase has no nodes")
	}
}

func TestBuildPhased_MissingForwardFunction(t *testing.T) {
	// Script without forward() should fail.
	client := createLorePackage(t, "badpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, system, plan):
    plan.package.install(package.name)
`,
	})

	_, err := Build(BuildConfig{
		Packages:       []string{"badpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err == nil {
		t.Fatal("expected error for missing forward() function")
	}
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
