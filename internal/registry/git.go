// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package registry

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GitProvider implements the Provider interface using git.
// This is the demo-phase provider per ADR-014.
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
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
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

	data, err := yaml.Marshal(&info)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(cacheDir, ".sync-info.yaml"), data, 0644)
}

// fileExists checks if a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
