// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

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

// GitProvider implements the Provider interface using git.
// This is the demo-phase provider per ADR-014.
//
// Versioning model:
//   - Tags represent package versions (e.g., v1.0.0, v1.2.3)
//   - "latest" is a special tag tracking the latest release
//   - On main branch: "latest" resolves to the "latest" tag
//   - On other branches: "latest" resolves to HEAD (dev convenience)
type GitProvider struct {
	repoURL string
	branch  string
}

// NewGitProvider creates a new git-based registry provider.
func NewGitProvider(repoURL, branch string) *GitProvider {
	return &GitProvider{
		repoURL: repoURL,
		branch:  branch,
	}
}

// Branch returns the configured branch.
func (g *GitProvider) Branch() string {
	return g.branch
}

// RepoURL returns the configured repository URL.
func (g *GitProvider) RepoURL() string {
	return g.repoURL
}

// Name returns "git".
func (g *GitProvider) Name() string {
	return "git"
}

// Sync clones or updates the registry cache.
// Per ADR-014, uses shallow clone and hard reset (cache semantics, not workspace).
func (g *GitProvider) Sync(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error) {
	// Check if cache exists
	gitDir := filepath.Join(cacheDir, ".git")
	exists := fileExists(gitDir)

	if !exists {
		return g.clone(ctx, cacheDir)
	}

	return g.pull(ctx, cacheDir, opts)
}

// clone performs initial shallow clone.
func (g *GitProvider) clone(ctx context.Context, cacheDir string) (*SyncResult, error) {
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

// pull fetches and resets to origin (hard reset, discarding local changes).
func (g *GitProvider) pull(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error) {
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

// getHeadRef returns the current HEAD commit SHA.
func (g *GitProvider) getHeadRef(ctx context.Context, cacheDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = cacheDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// runGit executes a git command.
func (g *GitProvider) runGit(ctx context.Context, dir string, args ...string) error {
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

// writeSyncInfo writes sync metadata.
func (g *GitProvider) writeSyncInfo(cacheDir, ref string, syncedAt time.Time) error {
	info := SyncInfo{
		LastSync: syncedAt,
		Ref:      ref,
		Provider: "git",
		Endpoint: g.repoURL,
	}

	return document.Write(filepath.Join(cacheDir, ".sync-info.yaml"), &info, document.WithPerm(0o644))
}

// fileExists checks if a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// =============================================================================
// Version/Tag Support
// =============================================================================

// ListVersions returns all available version tags from the repository.
// Tags are returned in descending semver order (newest first).
// Non-semver tags are excluded.
func (g *GitProvider) ListVersions(ctx context.Context, cacheDir string) ([]string, error) {
	// Fetch tags from remote
	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	// List all tags
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
//   - "latest" on main branch → resolves to "latest" tag
//   - "latest" on other branches → resolves to HEAD
//   - Semver (e.g., "v1.0.0", "1.0.0") → resolves to that tag
//   - Empty string → same as "latest"
func (g *GitProvider) ResolveVersion(ctx context.Context, cacheDir, version string) (string, error) {
	if version == "" || version == "latest" {
		if g.branch == "main" {
			// On main: resolve "latest" tag
			return g.resolveTag(ctx, cacheDir, "latest")
		}
		// On other branches: HEAD is latest
		return "HEAD", nil
	}

	// Explicit version tag
	return g.resolveTag(ctx, cacheDir, version)
}

// CheckoutVersion checks out a specific version (tag or ref) in the cache.
// This is used when a user requests a specific package version.
func (g *GitProvider) CheckoutVersion(ctx context.Context, cacheDir, version string) error {
	ref, err := g.ResolveVersion(ctx, cacheDir, version)
	if err != nil {
		return err
	}

	if ref == "HEAD" {
		// Already at HEAD after sync
		return nil
	}

	// Fetch the specific tag if needed
	if err := g.fetchTags(ctx, cacheDir); err != nil {
		return fmt.Errorf("fetching tags: %w", err)
	}

	// Checkout the ref
	checkoutArgs := []string{"checkout", ref}
	if err := g.runGit(ctx, cacheDir, checkoutArgs...); err != nil {
		return fmt.Errorf("checkout %s: %w", ref, err)
	}

	return nil
}

// HasTag checks if a specific tag exists.
func (g *GitProvider) HasTag(ctx context.Context, cacheDir, tag string) (bool, error) {
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

// fetchTags fetches tags from the remote.
func (g *GitProvider) fetchTags(ctx context.Context, cacheDir string) error {
	args := []string{"fetch", "--tags", "--force", "origin"}
	return g.runGit(ctx, cacheDir, args...)
}

// resolveTag resolves a tag name to its commit SHA.
func (g *GitProvider) resolveTag(ctx context.Context, cacheDir, tag string) (string, error) {
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

// CurrentRef returns the current HEAD ref in the cache.
func (g *GitProvider) CurrentRef(ctx context.Context, cacheDir string) (string, error) {
	return g.getHeadRef(ctx, cacheDir)
}

// CurrentVersion returns the version tag at HEAD, or "" if HEAD is not tagged.
func (g *GitProvider) CurrentVersion(ctx context.Context, cacheDir string) (string, error) {
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
