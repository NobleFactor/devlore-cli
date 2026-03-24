// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/star"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// TestLifecycle_DiscoverRegisterActivate exercises the full extension lifecycle
// against the real bundled extensions in cmd/star/extensions/. It verifies that
// all extensions progress through Discovered → Registered → Activated.
func TestLifecycle_DiscoverRegisterActivate(t *testing.T) {
	projectRoot := findProjectRoot(t)
	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")

	if _, err := os.Stat(extDir); err != nil {
		t.Skipf("extensions directory not found: %s", extDir)
	}

	// Create a loader pointed at the real extensions directory (no embedded FS).
	loader := star.NewExtensionLoader(nil)

	// Step 1: Discover.
	exts, err := loader.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll() error: %v", err)
	}
	if len(exts) == 0 {
		t.Fatal("DiscoverAll() returned no extensions")
	}

	t.Logf("Discovered %d extensions", len(exts))
	for _, ext := range exts {
		t.Logf("  %s (source: %s)", ext.Name, ext.Source)
	}

	// Verify all discovered extensions have valid names and commands.
	for _, ext := range exts {
		if ext.Name == "" {
			t.Error("discovered extension with empty name")
		}
		if !ext.HasCommands() {
			t.Errorf("extension %s has no commands", ext.Name)
		}
	}

	// Step 2+3: Register and Activate via DiscoverAndLoad on a fresh runtime.
	runtime := star.NewRuntime()
	if err := runtime.DiscoverAndLoad(loader); err != nil {
		t.Fatalf("DiscoverAndLoad() error: %v", err)
	}

	// Verify all extensions are in the registry.
	registry := runtime.Registry()
	for _, ext := range exts {
		if registry.Get(ext.Name) == nil {
			t.Errorf("extension %s not found in registry after DiscoverAndLoad", ext.Name)
		}
	}

	// Verify commands were activated — every extension's commands should be in the command map.
	commands := runtime.Commands()
	if len(commands) == 0 {
		t.Fatal("no commands registered after DiscoverAndLoad")
	}

	t.Logf("Activated %d commands", len(commands))
	for name := range commands {
		t.Logf("  star %s", name)
	}

	// Verify each registered extension has its commands in the map.
	for _, ext := range exts {
		for _, cmd := range ext.Commands {
			// Command names use dots in the spec but spaces in the map.
			spaceName := dotToSpace(cmd.Name)
			if _, ok := commands[spaceName]; !ok {
				t.Errorf("command %q (from %s) not found in command map", spaceName, ext.Name)
			}
		}
	}
}

// TestLifecycle_EmbeddedExtensions verifies the lifecycle with the real embedded
// extensions compiled into the test binary.
func TestLifecycle_EmbeddedExtensions(t *testing.T) {
	projectRoot := findProjectRoot(t)
	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")

	embeddedFS := os.DirFS(extDir)

	loader := star.NewExtensionLoader(embeddedFS)
	runtime := star.NewRuntime()

	if err := runtime.DiscoverAndLoad(loader); err != nil {
		t.Fatalf("DiscoverAndLoad() error: %v", err)
	}

	commands := runtime.Commands()
	if len(commands) == 0 {
		t.Fatal("no commands after loading embedded extensions")
	}

	registry := runtime.Registry()
	if registry.Count() == 0 {
		t.Fatal("registry empty after loading embedded extensions")
	}

	t.Logf("Loaded %d extensions with %d commands from embedded FS", registry.Count(), len(commands))
}

// TestLifecycle_DeduplicationProjectOverridesEmbedded verifies that project-local
// extensions take priority over embedded extensions with the same name.
func TestLifecycle_DeduplicationProjectOverridesEmbedded(t *testing.T) {
	// Create a temp project directory with one extension that has the same name
	// as a bundled extension but different help text.
	tmpDir := t.TempDir()
	extName := "com.noblefactor.star.LintAll"
	extDir := filepath.Join(tmpDir, extName)
	cmdDir := filepath.Join(extDir, "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yaml := `extension: ` + extName + `
description: Project-local override
commands:
  - name: lint.all
    help: OVERRIDDEN
    implementation: commands/lint-all.star
`
	if err := os.WriteFile(filepath.Join(extDir, "extension.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	starContent := "def run(command, ctx):\n    pass\n"
	if err := os.WriteFile(filepath.Join(cmdDir, "lint-all.star"), []byte(starContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create an embedded FS with the same extension but different help.
	projectRoot := findProjectRoot(t)
	realExtDir := filepath.Join(projectRoot, "cmd", "star", "extensions")
	embeddedFS := os.DirFS(realExtDir)

	// Loader: project-local (tmpDir) is searched before embedded.
	loader := star.NewExtensionLoaderWithPaths([]string{tmpDir}, embeddedFS)

	exts, err := loader.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll() error: %v", err)
	}

	// Find the LintAll extension — should be the project-local one.
	var lintAll *star.Extension
	for _, ext := range exts {
		if ext.Name == extName {
			lintAll = ext
			break
		}
	}

	if lintAll == nil {
		t.Fatalf("extension %s not found", extName)
	}

	if lintAll.Description != "Project-local override" {
		t.Errorf("expected project-local override, got description %q", lintAll.Description)
	}

	if lintAll.Source != star.SourceProjectLocal {
		t.Errorf("expected SourceProjectLocal, got %s", lintAll.Source)
	}
}

// dotToSpace replaces dots with spaces for command name lookup.
func dotToSpace(s string) string {
	result := make([]byte, len(s))
	for i := range s {
		if s[i] == '.' {
			result[i] = ' '
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}

// findProjectRoot walks up from the working directory to find go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

// Ensure os.DirFS satisfies fs.FS at compile time.
var _ fs.FS = os.DirFS(".")
