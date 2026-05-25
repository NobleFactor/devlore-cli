// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"testing"

	// Blank-import the op inventory so every provider's gen package init() runs and registers its
	// ProviderReceiverType with the framework. adopt.BuildGraph looks up the file provider via the
	// receiver registry; without this import the lookup fails with "file provider not registered."
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// TestAdoptFile_HappyPath exercises the Phase 6.C-rewired [adoptFile] end-to-end against a real temp tree.
//
// Verifies the same observable filesystem behavior the pre-13.0(n) implementation produced: source file moves into
// the project directory under its relative path; original location becomes a symlink pointing at the new path; the
// returned count is 1. The migration path now goes through [adopt.BuildGraph] → [op.Plan] →
// [op.GraphExecutor.Run] → [adopt.Run] under the hood; the test is concerned only with what's on disk afterwards.
func TestAdoptFile_HappyPath(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	const expectedContent = "adopted via Phase 6.C"
	if err := os.WriteFile(sourceFile, []byte(expectedContent), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	projectDir := filepath.Join(root, "project")

	count, err := adoptFile(sourceFile, root, projectDir, false, false)
	if err != nil {
		t.Fatalf("adoptFile: %v", err)
	}
	if count != 1 {
		t.Fatalf("adoptFile count = %d, want 1", count)
	}

	expectedDest := filepath.Join(projectDir, "source", "config.toml")

	destBytes, err := os.ReadFile(expectedDest)
	if err != nil {
		t.Fatalf("read destination %s: %v", expectedDest, err)
	}
	if got := string(destBytes); got != expectedContent {
		t.Errorf("destination content = %q, want %q", got, expectedContent)
	}

	originalInfo, err := os.Lstat(sourceFile)
	if err != nil {
		t.Fatalf("lstat original after adopt: %v", err)
	}
	if originalInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original path %s is not a symlink after adopt", sourceFile)
	}

	// Resolve the symlink and verify it points at the destination. file.Link emits relative-to-symlink-
	// directory targets under the confined os.Root model — absolute or relative is a presentation choice
	// the test does not lock in, but the resolved file must equal the expected destination.
	resolved, err := filepath.EvalSymlinks(sourceFile)
	if err != nil {
		t.Fatalf("eval symlink %s: %v", sourceFile, err)
	}
	expectedResolved, err := filepath.EvalSymlinks(expectedDest)
	if err != nil {
		t.Fatalf("eval expected dest %s: %v", expectedDest, err)
	}
	if resolved != expectedResolved {
		t.Errorf("symlink resolves to %q, want %q", resolved, expectedResolved)
	}
}

// TestAdoptFile_DryRun verifies that dry-run skips graph construction and dispatch — the source file is left in
// place, no destination is created, and the returned count still reflects "would-have-adopted".
func TestAdoptFile_DryRun(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	if err := os.WriteFile(sourceFile, []byte("dry-run probe"), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	projectDir := filepath.Join(root, "project")

	count, err := adoptFile(sourceFile, root, projectDir, false, true)
	if err != nil {
		t.Fatalf("adoptFile dry-run: %v", err)
	}
	if count != 1 {
		t.Fatalf("adoptFile dry-run count = %d, want 1", count)
	}

	if _, err := os.Stat(filepath.Join(projectDir, "source", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("destination should not exist after dry-run; err = %v", err)
	}

	info, err := os.Lstat(sourceFile)
	if err != nil {
		t.Fatalf("lstat source after dry-run: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("source should not be a symlink after dry-run")
	}
}

// TestAdoptItem_DirectoryWalk exercises the [adoptItem] → [adoptDirectory] → [adoptFile] path. A source directory
// containing multiple files is adopted recursively; each file moves into the project tree under its relative path,
// and each original location becomes a symlink. Verifies the per-file graph dispatched by 6.C's migration composes
// correctly under directory-walk iteration.
func TestAdoptItem_DirectoryWalk(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceParent, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir source tree: %v", err)
	}

	seeds := map[string]string{
		filepath.Join(sourceParent, "top.txt"):           "top-content",
		filepath.Join(sourceParent, "nested", "deep.txt"): "deep-content",
	}
	for path, content := range seeds {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	cfg := &AdoptConfig{Files: []string{sourceParent}}
	cfg.Tool = "writ"
	cfg.TargetRoot = root
	cfg.Layer = "personal"
	cfg.LayerPath = filepath.Join(root, "layers", "personal")
	cfg.Project = "behavioral-test"

	adopted := adoptItem(cfg, sourceParent)
	if adopted != len(seeds) {
		t.Fatalf("adoptItem(directory) = %d, want %d", adopted, len(seeds))
	}

	for path, expectedContent := range seeds {
		relFromHome, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("relpath %s: %v", path, err)
		}
		expectedDest := filepath.Join(cfg.LayerPath, "Home", cfg.Project, relFromHome)

		gotBytes, err := os.ReadFile(expectedDest)
		if err != nil {
			t.Fatalf("read destination %s: %v", expectedDest, err)
		}
		if got := string(gotBytes); got != expectedContent {
			t.Errorf("destination %s content = %q, want %q", expectedDest, got, expectedContent)
		}

		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat original %s after walk: %v", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("original %s is not a symlink after walk", path)
		}
	}
}

// TestAdoptItem_SkipSymlink verifies that [adoptItem] short-circuits when the item is already a symlink — the
// pre-migration semantics. Returns 0 adopted and leaves the symlink untouched.
func TestAdoptItem_SkipSymlink(t *testing.T) {

	root := t.TempDir()

	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	symlink := filepath.Join(root, "alias.txt")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	cfg := &AdoptConfig{Files: []string{symlink}}
	cfg.Tool = "writ"
	cfg.TargetRoot = root
	cfg.Layer = "personal"
	cfg.LayerPath = filepath.Join(root, "layers", "personal")
	cfg.Project = "behavioral-test"

	adopted := adoptItem(cfg, symlink)
	if adopted != 0 {
		t.Errorf("adoptItem(symlink) = %d, want 0 (skip)", adopted)
	}

	info, err := os.Lstat(symlink)
	if err != nil {
		t.Fatalf("lstat symlink after adoptItem: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink %s was disturbed by adoptItem", symlink)
	}
}

// TestAdoptFile_DestinationExists verifies that an existing destination short-circuits with a clear error and does
// not touch the source file.
func TestAdoptFile_DestinationExists(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	if err := os.WriteFile(sourceFile, []byte("source content"), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	projectDir := filepath.Join(root, "project")
	prePopulatedDest := filepath.Join(projectDir, "source", "config.toml")
	if err := os.MkdirAll(filepath.Dir(prePopulatedDest), 0o755); err != nil {
		t.Fatalf("mkdir pre-populated dest parent: %v", err)
	}
	if err := os.WriteFile(prePopulatedDest, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("seed pre-existing destination: %v", err)
	}

	count, err := adoptFile(sourceFile, root, projectDir, false, false)
	if err == nil {
		t.Fatal("adoptFile: expected error for existing destination")
	}
	if count != 0 {
		t.Errorf("adoptFile count = %d, want 0", count)
	}

	sourceBytes, readErr := os.ReadFile(sourceFile)
	if readErr != nil {
		t.Fatalf("source missing after failed adopt: %v", readErr)
	}
	if got := string(sourceBytes); got != "source content" {
		t.Errorf("source content disturbed: %q", got)
	}

	destBytes, readErr := os.ReadFile(prePopulatedDest)
	if readErr != nil {
		t.Fatalf("pre-existing destination missing: %v", readErr)
	}
	if got := string(destBytes); got != "pre-existing" {
		t.Errorf("pre-existing destination overwritten: %q", got)
	}
}
