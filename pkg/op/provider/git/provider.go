// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"io"
	"os"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides git actions.
//
// Compensable Forward methods return (string, map[string]any, error):
// the resource path, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
// +devlore:access=both
type Provider struct {
	// Test hooks. Nil means use real git commands.
	cloneFn func(url, path string, output io.Writer) error
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Clone clones a repository from url into path.
// Returns compensation state with the cloned path.
//
// Parameters:
//   - url: Git repository URL to clone
//   - path: Local directory path for the clone
func (p *Provider) Clone(url, path string, output io.Writer) (string, map[string]any, error) {
	if err := p.doClone(url, path, output); err != nil {
		return "", nil, err
	}
	return path, map[string]any{"path": path}, nil
}

// CompensateClone removes the cloned directory.
func (p *Provider) CompensateClone(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	path := op.StateString(s, "path")
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

// ── Standalone Methods ───────────────────────────────────────────────

// Checkout checks out a ref in the given repository directory.
//
// Parameters:
//   - repo: Local path to the git repository
//   - ref: Branch, tag, or commit to check out
func (p *Provider) Checkout(repo, ref string, output io.Writer) (string, error) {
	cmd := exec.Command("git", "-C", repo, "checkout", ref)
	cmd.Stdout = output
	cmd.Stderr = output
	return ref, cmd.Run()
}

// Pull pulls the latest changes in the given repository directory.
//
// Parameters:
//   - repo: Local path to the git repository
func (p *Provider) Pull(repo string, output io.Writer) (string, error) {
	cmd := exec.Command("git", "-C", repo, "pull")
	cmd.Stdout = output
	cmd.Stderr = output
	return repo, cmd.Run()
}

func (p *Provider) doClone(url, path string, output io.Writer) error {
	if p.cloneFn != nil {
		return p.cloneFn(url, path, output)
	}
	cmd := exec.Command("git", "clone", url, path)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}
