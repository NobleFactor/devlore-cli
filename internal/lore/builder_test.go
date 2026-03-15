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
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// runGraph is a test helper that calls RunNodes with the graph's nodes and edges.
func runGraph(ctx context.Context, eng *execution.GraphExecutor, g *op.Graph) ([]*execution.NodeResult, error) {
	return eng.RunNodes(ctx, g.Nodes, g.Edges)
}

func TestBuild_WithNativePMPackage(t *testing.T) {
	// Test that Build creates correct nodes for native PM packages.
	// Native PM packages use the namespaced "pkg.install" action that works
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

	// The first node should be a namespaced pkg.install action
	found := false
	for _, node := range result.Graph.Nodes {
		if node.ActionName() == "pkg.install" {
			found = true
			// Verify slot values
			if node.GetSlot("packages") != "curl" {
				t.Errorf("expected packages 'curl', got %q", node.GetSlot("packages"))
			}
			break
		}
	}
	if !found {
		t.Error("expected to find pkg.install action")
	}

	// Verify graph context is populated
	ctx := result.Graph.Context
	if ctx.Scope != "curl" {
		t.Errorf("Context.Scope = %q, want %q", ctx.Scope, "curl")
	}
	if len(ctx.Packages) != 1 || ctx.Packages[0] != "curl" {
		t.Errorf("Context.Packages = %v, want [curl]", ctx.Packages)
	}
	if ctx.TargetPlatform != "Linux.Debian" {
		t.Errorf("Context.TargetPlatform = %q, want %q", ctx.TargetPlatform, "Linux.Debian")
	}
}

func TestBuild_PlatformDetection(t *testing.T) {
	// Test that platform is correctly resolved and stored in result.
	// All platforms use the namespaced "pkg.install" action - the actual PM
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

			// All platforms use the namespaced "pkg.install" action
			found := false
			for _, node := range result.Graph.Nodes {
				if node.ActionName() == "pkg.install" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected to find pkg.install action")
			}
		})
	}
}

func TestBuildFromManifest(t *testing.T) {
	// Test building from a packages-manifest.yaml file
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages-manifest.yaml")

	manifest := `packages:
  - name: curl
  - name: jq
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
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

	// Verify graph context for multi-package build
	ctx := result.Graph.Context
	if ctx.Scope != "curl+jq" {
		t.Errorf("Context.Scope = %q, want %q", ctx.Scope, "curl+jq")
	}
	if len(ctx.Packages) != 2 {
		t.Errorf("Context.Packages = %v, want [curl, jq]", ctx.Packages)
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

func TestEngineRunsPackageInstallActions(t *testing.T) {
	// Integration test: build graph and run through engine with DryRun
	reg := op.NewActionRegistry()

	// Register all actions (file + package)
	tmpDir := t.TempDir()
	root := op.NewRootReaderWriter(tmpDir)
	defer root.Close()
	opCtx := op.Context{ContextBase: op.ContextBase{Root: root}}
	op.InitAll(reg, opCtx)

	eng := execution.NewGraphExecutor(execution.ExecutorOptions{Root: tmpDir, DryRun: true})

	// Create a graph with a namespaced pkg.install node
	node := &op.Node{
		ID:     "package-install-testpkg",
		Action: reg.MustGet("pkg.install"),
	}
	node.SetSlotImmediate("packages", []string{"testpkg"})
	node.SetSlotImmediate("manager", "brew")
	node.SetSlotImmediate("cask", false)
	graph := &op.Graph{
		Nodes: []*op.Node{node},
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

func TestEngineRunsNamespacedPackageActions(t *testing.T) {
	// Test that all namespaced package actions can execute in dry-run mode
	reg := op.NewActionRegistry()
	tmpDir := t.TempDir()
	root := op.NewRootReaderWriter(tmpDir)
	defer root.Close()
	opCtx := op.Context{ContextBase: op.ContextBase{Root: root}}
	op.InitAll(reg, opCtx)

	eng := execution.NewGraphExecutor(execution.ExecutorOptions{Root: tmpDir, DryRun: true})

	// All platforms use these four namespaced package actions
	actions := []string{
		"pkg.install", "pkg.upgrade", "pkg.remove", "pkg.update",
	}

	for _, opName := range actions {
		t.Run(opName, func(t *testing.T) {
			node := &op.Node{
				ID:     "test-" + opName,
				Action: reg.MustGet(opName),
			}
			node.SetSlotImmediate("manager", "brew")
			if opName != "pkg.update" {
				node.SetSlotImmediate("packages", []string{"testpkg"})
				node.SetSlotImmediate("cask", false)
			}

			graph := &op.Graph{
				Nodes: []*op.Node{node},
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
	// With namespaced actions, manager is NOT set in metadata - it's determined
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

	// Find the install node (uses namespaced "pkg.install" action)
	var installNode *op.Node
	for _, node := range result.Graph.Nodes {
		if node.ActionName() == "pkg.install" {
			installNode = node
			break
		}
	}

	if installNode == nil {
		t.Fatal("no pkg.install node found")
	}

	// Verify required slots
	if installNode.GetSlot("packages") == "" {
		t.Error("expected packages slot to be set")
	}
	if installNode.GetSlot("phase") == "" {
		t.Error("expected phase slot to be set")
	}
	// Note: manager is NOT set for namespaced actions
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
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write lifecycle.yaml
	lifecycle := "name: " + pkgName + "\nversion: 1.0.0\nplatforms: [Darwin]\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycle), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write phase scripts
	for relPath, content := range scripts {
		absPath := filepath.Join(pkgDir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
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
	if phase.Status != op.PhasePending {
		t.Errorf("expected status pending, got %q", phase.Status)
	}
	if len(phase.NodeIDs) != 1 {
		t.Errorf("expected 1 node ID, got %d", len(phase.NodeIDs))
	}
}

func TestBuildPhased_LorePackageForwardOnly(t *testing.T) {
	// Lore package with phase-named entry point — no compensation needed.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
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
	if phase.Retry != nil {
		t.Error("expected no retry policy")
	}
}

func TestBuildPhased_LorePackageWithRetry(t *testing.T) {
	// Lore package with phase.retry() — retry configured in the entry point.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s", max_delay="30s")
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
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
	if phase.Retry.Backoff != op.BackoffExponential {
		t.Errorf("Backoff = %q, want exponential", phase.Retry.Backoff)
	}
	if phase.Retry.InitialDelay != "1s" {
		t.Errorf("InitialDelay = %q, want '1s'", phase.Retry.InitialDelay)
	}
	if phase.Retry.MaxDelay != "30s" {
		t.Errorf("MaxDelay = %q, want '30s'", phase.Retry.MaxDelay)
	}
}

func TestBuildPhased_LorePackageMultiPhase(t *testing.T) {
	// Multi-phase package with retry on install only.
	client := createLorePackage(t, "ripgrep", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    phase.retry(max_attempts=2, backoff="linear", initial_delay="500ms")
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
`,
		"Darwin/Deploy/provision.star": `
def provision(package, phase):
    plan.shell.exec(command="ln -sf /opt/rg/completions/_rg ~/.zsh/completions/_rg")
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

	// Should have 2 phases: install, provision (no compensate phases).
	if len(result.Graph.Phases) != 2 {
		var names []string
		for _, p := range result.Graph.Phases {
			names = append(names, p.Name)
		}
		t.Fatalf("expected 2 phases, got %d: %v", len(result.Graph.Phases), names)
	}

	installPhase := result.Graph.Phases[0]
	provisionPhase := result.Graph.Phases[1]

	if installPhase.Name != "install" {
		t.Errorf("phase[0] = %q, want 'install'", installPhase.Name)
	}
	if installPhase.Retry == nil || installPhase.Retry.MaxAttempts != 2 {
		t.Error("install phase should have retry with max_attempts=2")
	}

	if provisionPhase.Name != "provision" {
		t.Errorf("phase[1] = %q, want 'provision'", provisionPhase.Name)
	}
	if provisionPhase.Retry != nil {
		t.Error("provision phase should not have retry policy")
	}

	// Verify nodes are correctly assigned to phases.
	if len(installPhase.NodeIDs) == 0 {
		t.Error("install phase has no nodes")
	}
	if len(provisionPhase.NodeIDs) == 0 {
		t.Error("provision phase has no nodes")
	}
}

func TestBuildPhased_MissingEntryPoint(t *testing.T) {
	// Script without a phase-named entry point should fail.
	client := createLorePackage(t, "badpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def forward(package, system, plan):
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
`,
	})

	_, err := Build(BuildConfig{
		Packages:       []string{"badpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err == nil {
		t.Fatal("expected error for missing phase-named entry point")
	}
}

func TestBuildPhased_PhaseContextAttributes(t *testing.T) {
	// Verify that phase.name and phase.action are accessible.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    if phase.name != "install":
        fail("expected phase.name='install', got '%s'" % phase.name)
    if phase.action != "deploy":
        fail("expected phase.action='deploy', got '%s'" % phase.action)
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
`,
	})

	_, err := Build(BuildConfig{
		Packages:       []string{"testpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
}

func TestBuildPhased_OutputFunctions(t *testing.T) {
	// Verify that ui.note(), ui.success() are available as globals.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    ui.note("installing %s" % package.name)
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
    ui.success("done")
`,
	})

	_, err := Build(BuildConfig{
		Packages:       []string{"testpkg"},
		Platform:       "Darwin",
		RegistryClient: client,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
}

func TestBuildPhased_PlanIsGlobal(t *testing.T) {
	// Verify that plan is a global, not a call argument.
	// The script accesses plan without receiving it as an argument.
	client := createLorePackage(t, "testpkg", map[string]string{
		"Darwin/Deploy/install.star": `
def install(package, phase):
    plan.pkg.install(packages=[package.name], manager="brew", cask=False)
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

	// Verify the graph has nodes (proves plan.pkg.install() worked).
	found := false
	for _, node := range result.Graph.Nodes {
		if node.ActionName() == "pkg.install" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pkg.install node from plan.pkg.install()")
	}
}

func TestPlanner_PlanPackages(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages-manifest.yaml")

	manifestContent := `packages:
  - name: curl
  - name: jq
    with: [json-path]
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := op.NewActionRegistry()
	root := op.NewRootReaderWriter(tmpDir)
	defer root.Close()
	opCtx := op.Context{ContextBase: op.ContextBase{Root: root}}
	op.InitAll(reg, opCtx)

	planner := &Planner{
		Platform:       "Linux.Debian",
		ActionRegistry: reg,
	}

	graph := &op.Graph{}
	names, err := planner.PlanPackages(graph, manifestPath)
	if err != nil {
		t.Fatalf("PlanPackages failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 package names, got %d", len(names))
	}
	if names[0] != "curl" || names[1] != "jq" {
		t.Errorf("names = %v, want [curl, jq]", names)
	}

	// Verify nodes were added to the graph
	if len(graph.Nodes) == 0 {
		t.Error("expected nodes to be added to graph")
	}

	// Verify phases were added
	if len(graph.Phases) == 0 {
		t.Error("expected phases to be added to graph")
	}
}

func TestPlanner_PlanByName(t *testing.T) {
	tmpDir := t.TempDir()
	reg := op.NewActionRegistry()
	root := op.NewRootReaderWriter(tmpDir)
	defer root.Close()
	opCtx := op.Context{ContextBase: op.ContextBase{Root: root}}
	op.InitAll(reg, opCtx)

	planner := &Planner{
		Platform:       "Darwin",
		ActionRegistry: reg,
	}

	graph := &op.Graph{}
	names, err := planner.PlanByName(graph, []string{"git", "vim"})
	if err != nil {
		t.Fatalf("PlanByName failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 package names, got %d", len(names))
	}
	if names[0] != "git" || names[1] != "vim" {
		t.Errorf("names = %v, want [git, vim]", names)
	}

	if len(graph.Nodes) == 0 {
		t.Error("expected nodes to be added to graph")
	}
}

func TestMergeFeatures(t *testing.T) {
	tests := []struct {
		name   string
		pkg    []string
		global []string
		want   []string
	}{
		{"empty both", nil, nil, nil},
		{"global only", nil, []string{"a", "b"}, []string{"a", "b"}},
		{"per-pkg only", []string{"x", "y"}, nil, []string{"x", "y"}},
		{"merge unique", []string{"c"}, []string{"a", "b"}, []string{"c", "a", "b"}},
		{"dedup", []string{"b", "c"}, []string{"a", "b"}, []string{"b", "c", "a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeFeatures(tt.pkg, tt.global)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeFeatures(%v, %v) = %v, want %v", tt.pkg, tt.global, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mergeFeatures(%v, %v)[%d] = %q, want %q", tt.pkg, tt.global, i, got[i], tt.want[i])
				}
			}
		})
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
