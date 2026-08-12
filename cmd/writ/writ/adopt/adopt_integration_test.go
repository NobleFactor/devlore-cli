// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package adopt_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/adopt"

	// Blank-import the op inventory so every provider's gen package init() runs and registers its
	// ProviderReceiverType with the framework. adopt.BuildGraph looks up the file and flow providers via the
	// receiver registry; without this import the lookup fails with "provider not registered."
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// configForTest builds an [*adopt.Config] rooted at `root` for the behavioral tests.
func configForTest(t *testing.T, root string, files ...string) *adopt.Config {

	t.Helper()

	// Keep receipt traces out of the user's real state directory.
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	return &adopt.Config{
		Files:      files,
		TargetRoot: root,
		LayerPath:  filepath.Join(root, "layers", "personal"),
		Project:    "behavioral-test",
	}
}

// runForTest drives the slice-A batch path: enumeration into scope groups, then one graph run per group.
func runForTest(t *testing.T, cfg *adopt.Config) (int, error) {

	t.Helper()

	groups := adopt.Collect(cfg)
	return adopt.RunBatches(context.Background(), cfg, groups)
}

// TestAdopt_HappyPath exercises the slice-A batch path end-to-end against a real temp tree: the source file moves
// into the layer tree under `<layer>/Home/<project>/<relpath>`, the original location becomes a symlink pointing at
// the moved file, and the count is 1.
func TestAdopt_HappyPath(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	const expectedContent = "adopted via slice A"
	if err := os.WriteFile(sourceFile, []byte(expectedContent), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	cfg := configForTest(t, root, sourceFile)

	count, err := runForTest(t, cfg)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if count != 1 {
		t.Fatalf("adopted = %d, want 1", count)
	}

	expectedDest := filepath.Join(cfg.LayerPath, "Home", cfg.Project, "source", "config.toml")

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

// TestAdopt_DryRun verifies that dry-run narrates during enumeration and never builds or runs a graph — the source
// file is left in place, no destination is created, and the would-adopt count reflects the enumeration.
func TestAdopt_DryRun(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	if err := os.WriteFile(sourceFile, []byte("dry-run probe"), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	cfg := configForTest(t, root, sourceFile)
	cfg.DryRun = true

	groups := adopt.Collect(cfg)
	total := 0
	for _, items := range groups {
		total += len(items)
	}
	if total != 1 {
		t.Fatalf("would-adopt count = %d, want 1", total)
	}

	dest := filepath.Join(cfg.LayerPath, "Home", cfg.Project, "source", "config.toml")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
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

// TestAdopt_DirectoryWalk adopts a directory recursively as ONE batch graph: both files move into the layer tree
// under their relative paths and each original location becomes a symlink.
func TestAdopt_DirectoryWalk(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceParent, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir source tree: %v", err)
	}

	seeds := map[string]string{
		filepath.Join(sourceParent, "top.txt"):            "top-content",
		filepath.Join(sourceParent, "nested", "deep.txt"): "deep-content",
	}
	for path, content := range seeds {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	cfg := configForTest(t, root, sourceParent)

	count, err := runForTest(t, cfg)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if count != len(seeds) {
		t.Fatalf("adopted = %d, want %d", count, len(seeds))
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

// TestAdopt_SkipSymlink verifies that enumeration short-circuits when the item is already a symlink: nothing is
// batched and the symlink is untouched.
func TestAdopt_SkipSymlink(t *testing.T) {

	root := t.TempDir()

	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	symlink := filepath.Join(root, "alias.txt")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	cfg := configForTest(t, root, symlink)

	groups := adopt.Collect(cfg)
	if len(groups) != 0 {
		t.Errorf("Collect(symlink) = %v, want empty (skip)", groups)
	}

	info, err := os.Lstat(symlink)
	if err != nil {
		t.Fatalf("lstat symlink after enumeration: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink %s was disturbed by enumeration", symlink)
	}
}

// TestAdopt_DestinationExists verifies the in-graph destination guard: an existing destination fails the run before
// that item's move dispatches, the source file is untouched, and the pre-existing destination is preserved.
func TestAdopt_DestinationExists(t *testing.T) {

	root := t.TempDir()

	sourceParent := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceParent, 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}

	sourceFile := filepath.Join(sourceParent, "config.toml")
	if err := os.WriteFile(sourceFile, []byte("source content"), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}

	cfg := configForTest(t, root, sourceFile)

	prePopulatedDest := filepath.Join(cfg.LayerPath, "Home", cfg.Project, "source", "config.toml")
	if err := os.MkdirAll(filepath.Dir(prePopulatedDest), 0o755); err != nil {
		t.Fatalf("mkdir pre-populated dest parent: %v", err)
	}
	if err := os.WriteFile(prePopulatedDest, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("seed pre-existing destination: %v", err)
	}

	count, err := runForTest(t, cfg)
	if err == nil {
		t.Fatal("adopt: expected the in-graph guard to fail the run for an existing destination")
	}
	if !strings.Contains(err.Error(), "destination already exists") {
		t.Errorf("error = %q, want it to name the existing destination", err)
	}
	if count != 0 {
		t.Errorf("adopted = %d, want 0", count)
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
