// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"io"
	"os"
	"os/exec"
)

// Provider provides git operations.
//
// Compensable Forward methods return (map[string]any, error).
// The map is the compensation receipt — opaque to the executor,
// meaningful only to the corresponding Compensate* Backward method.
type Provider struct {
	// Test hooks. Nil means use real git commands.
	cloneFn func(url, path string, output io.Writer) error
}

// Clone clones a repository from url into path.
// Returns compensation state with the cloned path.
func (p *Provider) Clone(url, path string, output io.Writer) (map[string]any, error) {
	if err := p.doClone(url, path, output); err != nil {
		return nil, err
	}
	return map[string]any{"path": path}, nil
}

// CompensateClone removes the cloned directory.
func (p *Provider) CompensateClone(state map[string]any) error {
	path, _ := state["path"].(string)
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

// Checkout checks out a ref in the given repository directory.
func (p *Provider) Checkout(repo, ref string, output io.Writer) error {
	cmd := exec.Command("git", "-C", repo, "checkout", ref)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// Pull pulls the latest changes in the given repository directory.
func (p *Provider) Pull(repo string, output io.Writer) error {
	cmd := exec.Command("git", "-C", repo, "pull")
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
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
