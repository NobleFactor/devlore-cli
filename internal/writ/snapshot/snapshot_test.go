// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// initGitRepo creates a git repo at dir with an initial commit containing the given files.
// Files is a map of relative path → content.
//
// Parameters:
//   - t: test context
//   - dir: directory to initialize as a git repo
//   - files: map of relative path to file content
//
// Returns:
//   - string: the HEAD commit hash
func initGitRepo(t *testing.T, dir string, files map[string]string) string {
	t.Helper()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %s: %v", strings.Join(args, " "), out, err)
		}
	}

	run("init")
	run("config", "user.name", "test")
	run("config", "user.email", "test@test.com")

	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", path)
	}

	run("commit", "-m", "initial commit")

	// Get the commit hash
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// TestPinAndClose tests the basic pin → verify → close lifecycle.
func TestPinAndClose(t *testing.T) {
	// Override snapshots dir to temp
	origDir := snapshotsDir
	snapshotsDir = func() string { return filepath.Join(t.TempDir(), "snapshots") }
	defer func() { snapshotsDir = origDir }()

	repoDir := t.TempDir()
	hash := initGitRepo(t, repoDir, map[string]string{
		"Home/.bashrc":       "# bashrc",
		"System/etc/foo.cfg": "key=value",
	})

	snap, err := Pin(repoDir, "base")
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}

	// Verify snapshot fields
	if snap.Layer != "base" {
		t.Errorf("expected layer 'base', got %q", snap.Layer)
	}
	if snap.RepoPath != repoDir {
		t.Errorf("expected repo path %q, got %q", repoDir, snap.RepoPath)
	}
	if snap.CommitHash != hash {
		t.Errorf("expected hash %q, got %q", hash, snap.CommitHash)
	}

	// Verify worktree directory exists
	if !dirExists(snap.WorktreePath) {
		t.Fatalf("worktree directory does not exist: %s", snap.WorktreePath)
	}

	// Verify committed files are present
	bashrc := filepath.Join(snap.WorktreePath, "Home", ".bashrc")
	data, err := os.ReadFile(bashrc)
	if err != nil {
		t.Fatalf("read .bashrc from worktree: %v", err)
	}
	if string(data) != "# bashrc" {
		t.Errorf("expected '# bashrc', got %q", string(data))
	}

	// Close the snapshot
	if err := snap.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify worktree directory removed
	if dirExists(snap.WorktreePath) {
		t.Error("worktree directory should be removed after Close")
	}
}

// TestPinWorktreeExcludesUncommitted verifies that the worktree does not
// contain uncommitted changes from the working directory.
func TestPinWorktreeExcludesUncommitted(t *testing.T) {
	origDir := snapshotsDir
	snapshotsDir = func() string { return filepath.Join(t.TempDir(), "snapshots") }
	defer func() { snapshotsDir = origDir }()

	repoDir := t.TempDir()
	initGitRepo(t, repoDir, map[string]string{
		"committed.txt": "committed content",
	})

	// Add an uncommitted file to the working directory
	if err := os.WriteFile(filepath.Join(repoDir, "uncommitted.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stage a file but don't commit
	cmd := exec.Command("git", "-C", repoDir, "add", "uncommitted.txt")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	snap, err := Pin(repoDir, "personal")
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	defer func() { _ = snap.Close() }()

	// Committed file should be present
	if _, err := os.Stat(filepath.Join(snap.WorktreePath, "committed.txt")); err != nil {
		t.Error("committed.txt should be in worktree")
	}

	// Uncommitted file should NOT be present
	if _, err := os.Stat(filepath.Join(snap.WorktreePath, "uncommitted.txt")); err == nil {
		t.Error("uncommitted.txt should NOT be in worktree")
	}
}

// TestPinReusesExistingWorktree verifies that Pin reuses an existing worktree.
func TestPinReusesExistingWorktree(t *testing.T) {
	snapDir := filepath.Join(t.TempDir(), "snapshots")
	origDir := snapshotsDir
	snapshotsDir = func() string { return snapDir }
	defer func() { snapshotsDir = origDir }()

	repoDir := t.TempDir()
	initGitRepo(t, repoDir, map[string]string{"a.txt": "a"})

	snap1, err := Pin(repoDir, "base")
	if err != nil {
		t.Fatalf("Pin 1: %v", err)
	}

	snap2, err := Pin(repoDir, "base")
	if err != nil {
		t.Fatalf("Pin 2: %v", err)
	}

	if snap1.WorktreePath != snap2.WorktreePath {
		t.Errorf("expected same worktree path, got %q and %q", snap1.WorktreePath, snap2.WorktreePath)
	}

	// Clean up (only need to close once since same worktree)
	_ = snap1.Close()
}

// TestPinAllDeduplicatesSharedRepo verifies that PinAll creates only one
// worktree when multiple sources share the same repository.
func TestPinAllDeduplicatesSharedRepo(t *testing.T) {
	origDir := snapshotsDir
	snapshotsDir = func() string { return filepath.Join(t.TempDir(), "snapshots") }
	defer func() { snapshotsDir = origDir }()

	repoDir := t.TempDir()
	hash := initGitRepo(t, repoDir, map[string]string{
		"Home/.bashrc":     "# bash",
		"System/etc/a.cfg": "a",
	})

	sources := []tree.LayerSource{
		{Layer: "base", Path: repoDir, SourceRoot: filepath.Join(repoDir, "System"), TargetName: "System"},
		{Layer: "base", Path: repoDir, SourceRoot: filepath.Join(repoDir, "Home"), TargetName: "Home"},
	}

	snapshots, cleanup, err := PinAll(sources)
	if err != nil {
		t.Fatalf("PinAll: %v", err)
	}
	defer cleanup()

	// Should produce only 1 snapshot (shared repo)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].CommitHash != hash {
		t.Errorf("expected hash %q, got %q", hash, snapshots[0].CommitHash)
	}
}

// TestPinAllMultipleRepos verifies PinAll with distinct repositories.
func TestPinAllMultipleRepos(t *testing.T) {
	origDir := snapshotsDir
	snapshotsDir = func() string { return filepath.Join(t.TempDir(), "snapshots") }
	defer func() { snapshotsDir = origDir }()

	baseDir := t.TempDir()
	initGitRepo(t, baseDir, map[string]string{"Home/.bashrc": "base"})

	personalDir := t.TempDir()
	initGitRepo(t, personalDir, map[string]string{"Home/.bashrc": "personal"})

	sources := []tree.LayerSource{
		{Layer: "base", Path: baseDir, SourceRoot: filepath.Join(baseDir, "Home"), TargetName: "Home"},
		{Layer: "personal", Path: personalDir, SourceRoot: filepath.Join(personalDir, "Home"), TargetName: "Home"},
	}

	snapshots, cleanup, err := PinAll(sources)
	if err != nil {
		t.Fatalf("PinAll: %v", err)
	}
	defer cleanup()

	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
}

// TestRewriteSources verifies that SourceRoot is correctly rewritten.
func TestRewriteSources(t *testing.T) {
	origDir := snapshotsDir
	snapshotsDir = func() string { return filepath.Join(t.TempDir(), "snapshots") }
	defer func() { snapshotsDir = origDir }()

	repoDir := t.TempDir()
	initGitRepo(t, repoDir, map[string]string{
		"Home/.bashrc":     "# bash",
		"System/etc/a.cfg": "a",
	})

	sources := []tree.LayerSource{
		{Layer: "base", Path: repoDir, SourceRoot: filepath.Join(repoDir, "System"), TargetRoot: "/", TargetName: "System"},
		{Layer: "base", Path: repoDir, SourceRoot: filepath.Join(repoDir, "Home"), TargetRoot: "/home/user", TargetName: "Home"},
	}

	snapshots, cleanup, err := PinAll(sources)
	if err != nil {
		t.Fatalf("PinAll: %v", err)
	}
	defer cleanup()

	rewritten := RewriteSources(sources, snapshots)

	if len(rewritten) != 2 {
		t.Fatalf("expected 2 rewritten sources, got %d", len(rewritten))
	}

	wt := snapshots[0].WorktreePath

	// System source should point to worktree/System
	expectedSystem := filepath.Join(wt, "System")
	if rewritten[0].SourceRoot != expectedSystem {
		t.Errorf("expected System SourceRoot %q, got %q", expectedSystem, rewritten[0].SourceRoot)
	}

	// Home source should point to worktree/Home
	expectedHome := filepath.Join(wt, "Home")
	if rewritten[1].SourceRoot != expectedHome {
		t.Errorf("expected Home SourceRoot %q, got %q", expectedHome, rewritten[1].SourceRoot)
	}

	// Other fields should be preserved
	if rewritten[0].TargetRoot != "/" {
		t.Errorf("expected TargetRoot '/', got %q", rewritten[0].TargetRoot)
	}
	if rewritten[1].TargetName != "Home" {
		t.Errorf("expected TargetName 'Home', got %q", rewritten[1].TargetName)
	}
}

// TestHashes verifies the layer→hash map construction.
func TestHashes(t *testing.T) {
	snapshots := []*Snapshot{
		{Layer: "base", CommitHash: "abc123"},
		{Layer: "personal", CommitHash: "def456"},
	}

	hashes := Hashes(snapshots)

	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
	if hashes["base"] != "abc123" {
		t.Errorf("expected base hash 'abc123', got %q", hashes["base"])
	}
	if hashes["personal"] != "def456" {
		t.Errorf("expected personal hash 'def456', got %q", hashes["personal"])
	}
}
