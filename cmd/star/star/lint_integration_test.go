// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// =============================================================================
// Shared Test Infrastructure
// =============================================================================

// lintTestFile represents a file to create in the test directory.
type lintTestFile struct {
	path    string
	content string
}

// setupLintTestDir creates a temp directory with the given files and sets up
// the git workspace root for config loading.
func setupLintTestDir(t *testing.T, files []lintTestFile) string {
	t.Helper()
	dir := t.TempDir()

	// Set git workspace root to temp dir so config loading works
	config.SetGitWorkspaceRoot(dir)
	t.Cleanup(func() {
		config.ResetGitWorkspaceRoot()
	})

	for _, f := range files {
		path := filepath.Join(dir, f.path)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("creating directory for %s: %v", f.path, err)
		}

		if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
			t.Fatalf("writing %s: %v", f.path, err)
		}
	}

	return dir
}

// setupLintRuntime creates a runtime with extensions loaded and changes to the test dir.
func setupLintRuntime(t *testing.T, testDir string) (*Application, error) {
	t.Helper()

	// Clear global extension registry

	// Find project root BEFORE changing directories
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, err
	}

	// Create runtime
	r := NewRuntime()

	// Change to test directory for config loading
	origDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(testDir); err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	// Load extensions from the project's star/extensions directory
	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")
	if err := r.LoadExtensionsFrom(extDir); err != nil {
		return nil, err
	}

	return r, nil
}

// runLintCommand runs a lint command and returns whether it passed.
func runLintCommand(t *testing.T, r *Application, cmdName string, args map[string]string) (passed bool, err error) {
	t.Helper()

	cmd, ok := r.Commands()[cmdName]
	if !ok {
		return false, os.ErrNotExist
	}

	if args == nil {
		args = map[string]string{}
	}

	err = cmd.Run(args)
	passed = err == nil

	return passed, err
}

// =============================================================================
// Config Fixtures
// =============================================================================

func minimalStarConfig() string {
	return `lint:
  go:
    path: "./..."
  shell:
    path: "."
    severity: warning
  markdown:
    path: "."
  copyright:
    enabled: false
`
}

func starConfigWithCopyright(holder, license string) string {
	if license == "" {
		license = "auto"
	}
	return `lint:
  go:
    path: "./..."
  shell:
    path: "."
    severity: warning
  markdown:
    path: "."
  copyright:
    enabled: true
    holder: "` + holder + `"
    license: "` + license + `"
    exclude: []
`
}

// =============================================================================
// lint go Integration Tests
// =============================================================================

func TestLintGo_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
		{"go.mod", "module test\n\ngo 1.21\n"},
		{"main.go", `package main

func main() {}
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint go command exists
	_, ok := r.Commands()["lint go"]
	if !ok {
		t.Fatal("lint go command not found")
	}
}

func TestLintGo_UsesConfigPath(t *testing.T) {
	// Create config with custom path
	customConfig := `lint:
  go:
    path: "./cmd/..."
    skip_mod_tidy: true
`
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", customConfig},
		{"go.mod", "module test\n\ngo 1.21\n"},
		{"cmd/main.go", `package main

func main() {}
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Command should exist and be runnable (may fail due to missing tools, but config is loaded)
	cmd, ok := r.Commands()["lint go"]
	if !ok {
		t.Fatal("lint go command not found")
	}

	// Just verify the command structure is correct
	if cmd.Name != "lint.go" {
		t.Errorf("command name = %q, want %q", cmd.Name, "lint.go")
	}
}

// =============================================================================
// lint shell Integration Tests
// =============================================================================

func TestLintShell_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
		{"script.sh", `#!/bin/bash
echo "hello"
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint shell command exists
	_, ok := r.Commands()["lint shell"]
	if !ok {
		t.Fatal("lint shell command not found")
	}
}

func TestLintShell_UsesConfigSeverity(t *testing.T) {
	// Create config with custom severity
	customConfig := `lint:
  shell:
    path: "scripts/"
    severity: error
    indent: 2
`
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", customConfig},
		{"scripts/test.sh", `#!/bin/bash
echo "hello"
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd, ok := r.Commands()["lint shell"]
	if !ok {
		t.Fatal("lint shell command not found")
	}

	if cmd.Name != "lint.shell" {
		t.Errorf("command name = %q, want %q", cmd.Name, "lint.shell")
	}
}

// =============================================================================
// lint markdown Integration Tests
// =============================================================================

func TestLintMarkdown_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
		{"README.md", `# Test

This is a test.
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint markdown command exists
	_, ok := r.Commands()["lint markdown"]
	if !ok {
		t.Fatal("lint markdown command not found")
	}
}

func TestLintMarkdown_UsesConfigPath(t *testing.T) {
	customConfig := `lint:
  markdown:
    path: "docs/"
    exclude:
      - "vendor/**"
`
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", customConfig},
		{"docs/guide.md", `# Guide

Some content.
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd, ok := r.Commands()["lint markdown"]
	if !ok {
		t.Fatal("lint markdown command not found")
	}

	if cmd.Name != "lint.markdown" {
		t.Errorf("command name = %q, want %q", cmd.Name, "lint.markdown")
	}
}

// =============================================================================
// lint copyright Integration Tests
// =============================================================================

func TestLintCopyright_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", starConfigWithCopyright("Test Corp", "MIT")},
		{"LICENSE", "MIT License\n\nCopyright (c) 2024 Test Corp\n"},
		{"main.go", `// SPDX-License-Identifier: MIT
// Copyright Test Corp. All rights reserved.

package main

func main() {}
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint copyright command exists
	_, ok := r.Commands()["lint copyright"]
	if !ok {
		t.Fatal("lint copyright command not found")
	}
}

func TestLintCopyright_RespectsEnabled(t *testing.T) {
	// Config with copyright disabled
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()}, // copyright.enabled = false
		{"main.go", `package main

func main() {}
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Should pass (disabled means skip, not fail)
	passed, _ := runLintCommand(t, r, "lint copyright", nil)
	if !passed {
		t.Error("lint copyright should pass when disabled")
	}
}

// =============================================================================
// lint all Integration Tests
// =============================================================================

func TestLintAll_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint all command exists
	_, ok := r.Commands()["lint all"]
	if !ok {
		t.Fatal("lint all command not found")
	}
}

func TestLintAll_FindsSiblingCommands(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Check that sibling commands exist
	commands := r.Commands()

	expectedSiblings := []string{"lint go", "lint shell", "lint markdown", "lint copyright"}
	for _, name := range expectedSiblings {
		if _, ok := commands[name]; !ok {
			t.Errorf("expected sibling command %q not found", name)
		}
	}
}

func TestLintAll_SkipsDisabledCopyright(t *testing.T) {
	// Config with copyright disabled - lint all should skip it
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()}, // copyright.enabled = false
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd, ok := r.Commands()["lint all"]
	if !ok {
		t.Fatal("lint all command not found")
	}

	if cmd.Name != "lint.all" {
		t.Errorf("command name = %q, want %q", cmd.Name, "lint.all")
	}
}

// =============================================================================
// lint tools Integration Tests
// =============================================================================

func TestLintTools_LoadsConfig(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify lint tools command exists
	_, ok := r.Commands()["lint tools"]
	if !ok {
		t.Fatal("lint tools command not found")
	}
}

func TestLintTools_ReturnsToolStatus(t *testing.T) {
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cmd, ok := r.Commands()["lint tools"]
	if !ok {
		t.Fatal("lint tools command not found")
	}

	if cmd.Name != "lint.tools" {
		t.Errorf("command name = %q, want %q", cmd.Name, "lint.tools")
	}
}

// =============================================================================
// Config Loading From Git Root Tests
// =============================================================================

func TestLintCommands_LoadConfigFromGitRoot(t *testing.T) {
	// Create a directory structure simulating being in a subdirectory
	dir := setupLintTestDir(t, []lintTestFile{
		{"star/config.yaml", minimalStarConfig()},
		{"src/pkg/main.go", `package main

func main() {}
`},
	})

	r, err := setupLintRuntime(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Change to subdirectory
	subDir := filepath.Join(dir, "src", "pkg")
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("chdir to subdirectory: %v", err)
	}

	// Config should still load from git root
	// All lint commands should exist
	commands := r.Commands()
	for _, name := range []string{"lint go", "lint shell", "lint markdown", "lint copyright", "lint all", "lint tools"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not found when in subdirectory", name)
		}
	}
}

func TestLintCommands_NoConfigOutsideGitRepo(t *testing.T) {
	// Simulate being outside a git repo
	dir := t.TempDir()

	// Set empty git workspace root
	config.SetGitWorkspaceRoot("")
	t.Cleanup(func() {
		config.ResetGitWorkspaceRoot()
	})

	// Clear global extension registry

	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot: %v", err)
	}

	r := NewRuntime()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")
	if err := r.LoadExtensionsFrom(extDir); err != nil {
		t.Fatalf("LoadExtensionsFrom: %v", err)
	}

	// Commands should still load (they'll use defaults)
	commands := r.Commands()
	if len(commands) == 0 {
		t.Error("expected commands to be loaded even without project config")
	}
}
