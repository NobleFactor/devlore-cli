// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package snapshot provides git worktree-based snapshots for hermetic planning.
// Each layer source is pinned to a commit hash, and the planner reads from a
// detached worktree at that hash. This guarantees immutability during planning:
// uncommitted changes, staged files, and untracked files are invisible.
package snapshot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// Snapshot represents a git worktree pinned to a specific commit.
type Snapshot struct {
	// Layer is the layer name ("base", "team", "personal").
	Layer string

	// RepoPath is the original repository path.
	RepoPath string

	// CommitHash is the full commit hash this worktree is pinned to.
	CommitHash string

	// WorktreePath is the path to the detached worktree directory.
	WorktreePath string
}

// Pin creates a detached git worktree pinned to HEAD for the given repository.
// The worktree is placed under ${XDG_CACHE_HOME}/devlore/snapshots/<layer>-<hash[:12]>/.
//
// Parameters:
//   - repoPath: path to the git repository
//   - layer: layer name for directory naming ("base", "team", "personal")
//
// Returns:
//   - *Snapshot: the pinned snapshot
//   - error: git command failure or directory creation error
func Pin(repoPath, layer string) (*Snapshot, error) {
	// Resolve HEAD commit hash
	hash, err := gitRevParseHEAD(repoPath)
	if err != nil {
		return nil, fmt.Errorf("pin %s: resolve HEAD: %w", layer, err)
	}

	// Build worktree path
	short := hash
	if len(short) > 12 {
		short = short[:12]
	}
	worktreeDir := filepath.Join(snapshotsDir(), fmt.Sprintf("%s-%s", layer, short))

	// If worktree already exists at this path, reuse it
	if dirExists(worktreeDir) {
		return &Snapshot{
			Layer:        layer,
			RepoPath:     repoPath,
			CommitHash:   hash,
			WorktreePath: worktreeDir,
		}, nil
	}

	// Create detached worktree
	if err := gitWorktreeAdd(repoPath, worktreeDir, hash); err != nil {
		return nil, fmt.Errorf("pin %s: create worktree: %w", layer, err)
	}

	return &Snapshot{
		Layer:        layer,
		RepoPath:     repoPath,
		CommitHash:   hash,
		WorktreePath: worktreeDir,
	}, nil
}

// Close removes the git worktree and cleans up the directory.
//
// Returns:
//   - error: git worktree removal or directory cleanup error
func (s *Snapshot) Close() error {
	// Remove the git worktree registration
	if err := gitWorktreeRemove(s.RepoPath, s.WorktreePath); err != nil {
		// If git worktree remove fails, try force removal
		_ = os.RemoveAll(s.WorktreePath)
		_ = gitWorktreePrune(s.RepoPath)
		return nil //nolint:nilerr // best-effort cleanup; directory removed
	}
	return nil
}

// PinAll pins each unique repository from the given layer sources.
// Repositories are deduplicated by path — base, team, and personal may share a repo.
// Returns the snapshots and a cleanup function that closes all of them.
//
// Parameters:
//   - sources: layer sources from CollectLayerSources
//
// Returns:
//   - []*Snapshot: one snapshot per unique repository
//   - func(): cleanup function that closes all snapshots
//   - error: first pinning error encountered
func PinAll(sources []tree.LayerSource) ([]*Snapshot, func(), error) {
	seen := make(map[string]*Snapshot) // keyed by RepoPath
	var snapshots []*Snapshot

	for _, src := range sources {
		if _, ok := seen[src.Path]; ok {
			continue // Already pinned this repo
		}

		snap, err := Pin(src.Path, src.Layer)
		if err != nil {
			// Close any snapshots created so far
			closeAll(snapshots)
			return nil, nil, err
		}

		seen[src.Path] = snap
		snapshots = append(snapshots, snap)
	}

	cleanup := func() { closeAll(snapshots) }
	return snapshots, cleanup, nil
}

// RewriteSources returns a copy of sources with SourceRoot rewritten to point
// at the corresponding worktree path. Each source's SourceRoot is the repo path
// plus a subdirectory (e.g., /repo/Home); the rewrite replaces the repo prefix
// with the worktree path, preserving the subdirectory.
//
// Parameters:
//   - sources: original layer sources
//   - snapshots: pinned snapshots from PinAll
//
// Returns:
//   - []tree.LayerSource: sources with SourceRoot pointing to worktree paths
func RewriteSources(sources []tree.LayerSource, snapshots []*Snapshot) []tree.LayerSource {
	// Build repo → worktree mapping
	worktreeByRepo := make(map[string]string, len(snapshots))
	for _, snap := range snapshots {
		worktreeByRepo[snap.RepoPath] = snap.WorktreePath
	}

	rewritten := make([]tree.LayerSource, len(sources))
	for i, src := range sources {
		rewritten[i] = src
		worktree, ok := worktreeByRepo[src.Path]
		if !ok {
			continue
		}
		// SourceRoot = repo/Home or repo/System
		// Replace repo prefix with worktree path
		suffix, err := filepath.Rel(src.Path, src.SourceRoot)
		if err != nil {
			continue // Should not happen; keep original
		}
		rewritten[i].SourceRoot = filepath.Join(worktree, suffix)
	}
	return rewritten
}

// Hashes returns a layer→hash map from the given snapshots.
// Suitable for recording in GraphContext.CommitHashes.
//
// Parameters:
//   - snapshots: pinned snapshots
//
// Returns:
//   - map[string]string: layer name to full commit hash
func Hashes(snapshots []*Snapshot) map[string]string {
	m := make(map[string]string, len(snapshots))
	for _, s := range snapshots {
		m[s.Layer] = s.CommitHash
	}
	return m
}

// closeAll closes all snapshots, logging errors but not failing.
func closeAll(snapshots []*Snapshot) {
	for _, s := range snapshots {
		_ = s.Close()
	}
}

// snapshotsDir returns the snapshots cache directory.
// Variable for test overriding.
var snapshotsDir = func() string {
	return filepath.Join(cli.DevloreCacheHome(), "snapshots")
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ----------------------------------------------------------------------------
// Git commands
// ----------------------------------------------------------------------------

// gitRevParseHEAD resolves HEAD to a full commit hash.
func gitRevParseHEAD(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitWorktreeAdd creates a detached worktree at the given path for the given commit.
func gitWorktreeAdd(repoPath, worktreePath, commitHash string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "--detach", worktreePath, commitHash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitWorktreeRemove removes a worktree registration and directory.
func gitWorktreeRemove(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// gitWorktreePrune removes stale worktree entries.
func gitWorktreePrune(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
