// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"os"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides git actions.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
	// Test hooks. Nil means use real git commands.
	cloneFn func(url, path string) error
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Clone clones a repository from url into path.
// Returns the cloned path and a Tombstone for compensation.
//
// Parameters:
//   - url: Git repository URL to clone
//   - path: Local directory path for the clone
func (p *Provider) Clone(url, path string) (string, Tombstone, error) {
	if err := p.doClone(url, path); err != nil {
		return "", Tombstone{}, err
	}
	return path, Tombstone{ClonedPath: path}, nil
}

// CompensateClone removes the cloned directory.
func (p *Provider) CompensateClone(state Tombstone) error {
	if state.ClonedPath == "" {
		return nil
	}
	return os.RemoveAll(state.ClonedPath)
}

// ── Standalone Methods ───────────────────────────────────────────────

// Checkout checks out a ref in the given repository directory.
//
// Parameters:
//   - repo: Local path to the git repository
//   - ref: Branch, tag, or commit to check out
func (p *Provider) Checkout(repo, ref string) (string, error) {
	cmd := exec.Command("git", "-C", repo, "checkout", ref)
	cmd.Stdout = p.Context().Writer
	cmd.Stderr = p.Context().Writer
	return ref, cmd.Run()
}

// Pull pulls the latest changes in the given repository directory.
//
// Parameters:
//   - repo: Local path to the git repository
func (p *Provider) Pull(repo string) (string, error) {
	cmd := exec.Command("git", "-C", repo, "pull")
	cmd.Stdout = p.Context().Writer
	cmd.Stderr = p.Context().Writer
	return repo, cmd.Run()
}

func (p *Provider) doClone(url, path string) error {
	if p.cloneFn != nil {
		return p.cloneFn(url, path)
	}
	cmd := exec.Command("git", "clone", url, path)
	cmd.Stdout = p.Context().Writer
	cmd.Stderr = p.Context().Writer
	return cmd.Run()
}
