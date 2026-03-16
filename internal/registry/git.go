// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package registry

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// gitTransport implements Transport using git.
// This is the demo-phase transport per ADR-014.
//
// Versioning model:
//   - Tags represent package versions (e.g., v1.0.0, v1.2.3)
//   - "latest" is a special tag tracking the latest release
//   - On main branch (or when ForceTags is set): "latest" resolves to the "latest" tag
//   - On other branches: "latest" resolves to HEAD (dev convenience)
type gitTransport struct {
	repoURL   string
	branch    string
	forceTags bool
}

// newGitTransport creates a git transport from configuration.
//
// Parameters:
//   - cfg: transport configuration with URL, Branch, and ForceTags
//
// Returns:
//   - *gitTransport: configured git transport
func newGitTransport(cfg Config) *gitTransport {

	return &gitTransport{
		repoURL:   cfg.URL,
		branch:    cfg.Branch,
		forceTags: cfg.ForceTags,
	}
}

// region EXPORTED METHODS

// region State management

// Name returns "git".
//
// Returns:
//   - string: transport type identifier
func (g *gitTransport) Name() string {
	return "git"
}

// endregion

// region Behaviors

// Fallible actions

// CheckoutVersion checks out a specific version (tag or ref) in the cache.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - version: version to checkout
//
// Returns:
//   - error: checkout failure
func (g *gitTransport) CheckoutVersion(ctx context.Context, cacheDir, version string) error {

	ref, err := g.ResolveVersion(ctx, cacheDir, version)
	if err != nil {
		return err
	}

	if ref == "HEAD" {
		return nil
	}

	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return fmt.Errorf("fetching tags: %w", err)
	}

	if err := g.runGit(ctx, cacheDir, "checkout", ref); err != nil {
		return fmt.Errorf("checkout %s: %w", ref, err)
	}

	return nil
}

// CurrentRef returns the current HEAD ref in the cache.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//
// Returns:
//   - string: HEAD commit SHA
//   - error: ref lookup failure
func (g *gitTransport) CurrentRef(ctx context.Context, cacheDir string) (string, error) {

	return g.getHeadRef(ctx, cacheDir)
}

// CurrentVersion returns the version tag at HEAD, or "" if HEAD is not tagged.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//
// Returns:
//   - string: version tag at HEAD, or empty string
//   - error: lookup failure
func (g *gitTransport) CurrentVersion(ctx context.Context, cacheDir string) (string, error) {

	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--exact-match", "HEAD")
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// HEAD is not tagged
		return "", nil
	}

	return strings.TrimSpace(stdout.String()), nil
}

// HasTag checks if a specific tag exists.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - tag: tag name to check
//
// Returns:
//   - bool: true if the tag exists
//   - error: lookup failure
func (g *gitTransport) HasTag(ctx context.Context, cacheDir, tag string) (bool, error) {

	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return false, err
	}

	cmd := exec.CommandContext(ctx, "git", "tag", "-l", tag)
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false, err
	}

	return strings.TrimSpace(stdout.String()) == tag, nil
}

// ListVersions returns all available version tags from the repository.
// Tags are returned in descending semver order (newest first).
// Non-semver tags are excluded.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//
// Returns:
//   - []string: version tags sorted newest-first
//   - error: listing failure
func (g *gitTransport) ListVersions(ctx context.Context, cacheDir string) ([]string, error) {

	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "tag", "-l")
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	// Filter and sort semver tags
	var versions []*semver.Version
	for _, tag := range lines {
		// Skip "latest" - it's special
		if tag == "latest" {
			continue
		}
		v, err := semver.NewVersion(tag)
		if err == nil {
			versions = append(versions, v)
		}
	}

	// Sort descending (newest first)
	sort.Sort(sort.Reverse(semver.Collection(versions)))

	result := make([]string, len(versions))
	for i, v := range versions {
		result[i] = v.Original()
	}

	return result, nil
}

// ResolveVersion resolves a version string to a git ref.
//
// Resolution rules:
//   - "latest" on main branch (or when ForceTags is set) → resolves to "latest" tag
//   - "latest" on other branches → resolves to HEAD
//   - Semver (e.g., "v1.0.0", "1.0.0") → resolves to that tag
//   - Empty string → same as "latest"
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - version: version string
//
// Returns:
//   - string: resolved ref (commit SHA or "HEAD")
//   - error: resolution failure
func (g *gitTransport) ResolveVersion(ctx context.Context, cacheDir, version string) (string, error) {

	if version == "" || version == "latest" {
		if g.branch == "main" || g.forceTags {
			return g.resolveTag(ctx, cacheDir, "latest")
		}
		return "HEAD", nil
	}

	return g.resolveTag(ctx, cacheDir, version)
}

// Sync clones or updates the registry cache.
// Per ADR-014, uses shallow clone and hard reset (cache semantics, not workspace).
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - opts: sync behavior options
//
// Returns:
//   - *SyncResult: information about what changed
//   - error: sync failure
func (g *gitTransport) Sync(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error) {

	gitDir := filepath.Join(cacheDir, ".git")
	if fileExists(gitDir) {
		return g.pull(ctx, cacheDir, opts)
	}

	return g.clone(ctx, cacheDir)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// clone performs initial shallow clone.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: target cache directory
//
// Returns:
//   - *SyncResult: sync result with FromClone=true
//   - error: clone failure
func (g *gitTransport) clone(ctx context.Context, cacheDir string) (*SyncResult, error) {

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return nil, fmt.Errorf("creating cache parent directory: %w", err)
	}

	// Remove any partial cache directory
	if err := os.RemoveAll(cacheDir); err != nil {
		return nil, fmt.Errorf("cleaning cache directory: %w", err)
	}

	// Shallow clone
	args := []string{
		"clone",
		"--depth", "1",
		"--single-branch",
		"--branch", g.branch,
		g.repoURL,
		cacheDir,
	}

	if err := g.runGit(ctx, "", args...); err != nil {
		return nil, fmt.Errorf("git clone: %w", err)
	}

	// Get current commit SHA
	ref, err := g.getHeadRef(ctx, cacheDir)
	if err != nil {
		return nil, fmt.Errorf("getting HEAD ref: %w", err)
	}

	// Write sync info
	syncedAt := time.Now()
	if err := g.writeSyncInfo(cacheDir, ref, syncedAt); err != nil {
		return nil, fmt.Errorf("writing sync info: %w", err)
	}

	return &SyncResult{
		Updated:   true,
		FromRef:   "",
		ToRef:     ref,
		SyncedAt:  syncedAt,
		FromClone: true,
	}, nil
}

// fetchTags fetches tags from the remote.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//
// Returns:
//   - error: fetch failure
func (g *gitTransport) fetchTags(ctx context.Context, cacheDir string) error {

	return g.runGit(ctx, cacheDir, "fetch", "--tags", "--force", "origin")
}

// getHeadRef returns the current HEAD commit SHA.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//
// Returns:
//   - string: HEAD commit SHA
//   - error: ref lookup failure
func (g *gitTransport) getHeadRef(ctx context.Context, cacheDir string) (string, error) {

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// pull fetches and resets to origin (hard reset, discarding local changes).
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - opts: sync behavior options
//
// Returns:
//   - *SyncResult: sync result
//   - error: pull failure
func (g *gitTransport) pull(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error) {

	// Get current ref before pull
	oldRef, err := g.getHeadRef(ctx, cacheDir)
	if err != nil {
		// Corrupted cache, re-clone
		return g.clone(ctx, cacheDir)
	}

	// Fetch from origin (shallow)
	fetchArgs := []string{
		"fetch",
		"--depth", "1",
		"origin",
		g.branch,
	}
	if err := g.runGit(ctx, cacheDir, fetchArgs...); err != nil {
		return nil, fmt.Errorf("git fetch: %w", err)
	}

	// Hard reset to origin/branch (discards any local changes per ADR-014)
	resetArgs := []string{
		"reset",
		"--hard",
		fmt.Sprintf("origin/%s", g.branch),
	}
	if err := g.runGit(ctx, cacheDir, resetArgs...); err != nil {
		return nil, fmt.Errorf("git reset: %w", err)
	}

	// Get new ref
	newRef, err := g.getHeadRef(ctx, cacheDir)
	if err != nil {
		return nil, fmt.Errorf("getting HEAD ref after pull: %w", err)
	}

	syncedAt := time.Now()
	updated := oldRef != newRef

	// Write sync info
	if err := g.writeSyncInfo(cacheDir, newRef, syncedAt); err != nil {
		return nil, fmt.Errorf("writing sync info: %w", err)
	}

	return &SyncResult{
		Updated:   updated,
		FromRef:   oldRef,
		ToRef:     newRef,
		SyncedAt:  syncedAt,
		FromClone: false,
	}, nil
}

// resolveTag resolves a tag name to its commit SHA.
//
// Parameters:
//   - ctx: context for cancellation
//   - cacheDir: local cache directory path
//   - tag: tag name to resolve
//
// Returns:
//   - string: commit SHA for the tag
//   - error: resolution failure
func (g *gitTransport) resolveTag(ctx context.Context, cacheDir, tag string) (string, error) {

	// First ensure we have the tag
	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return "", fmt.Errorf("fetching tags: %w", err)
	}

	// Check if tag exists
	exists, err := g.HasTag(ctx, cacheDir, tag)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("tag %q not found", tag)
	}

	// Resolve tag to commit
	cmd := exec.CommandContext(ctx, "git", "rev-parse", tag+"^{}")
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resolving tag %s: %w", tag, err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runGit executes a git command.
//
// Parameters:
//   - ctx: context for cancellation
//   - dir: working directory (empty for cwd)
//   - args: git subcommand and arguments
//
// Returns:
//   - error: command failure
func (g *gitTransport) runGit(ctx context.Context, dir string, args ...string) error {

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	// Capture output for error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}

	return nil
}

// writeSyncInfo writes sync metadata to the cache directory.
//
// Parameters:
//   - cacheDir: directory containing the cached registry
//   - ref: git ref that was synced
//   - syncedAt: timestamp of the sync
//
// Returns:
//   - error: marshal or write error
func (g *gitTransport) writeSyncInfo(cacheDir, ref string, syncedAt time.Time) error {

	info := SyncInfo{
		LastSync: syncedAt,
		Ref:      ref,
		Provider: "git",
		Endpoint: g.repoURL,
	}

	return document.Write(filepath.Join(cacheDir, ".sync-info.yaml"), &info, document.WithPerm(0o644))
}

// endregion

// endregion

// fileExists checks if a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
