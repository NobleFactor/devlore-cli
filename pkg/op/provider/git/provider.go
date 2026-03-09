// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"os"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	netprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
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

// Clone clones a repository from url into destination.
// Returns the cloned git.Resource and a Tombstone for compensation.
//
// Parameters:
//   - url: network resource identifying the git repository
//   - destination: file resource identifying the local clone directory
func (p *Provider) Clone(url netprov.Resource, destination file.Resource) (Resource, Tombstone, error) {
	if err := p.doClone(url.SourceURL.String(), destination.SourcePath.Abs); err != nil {
		return Resource{}, Tombstone{}, err
	}
	r := &Resource{
		URL:       url.SourceURL.String(),
		ClonePath: destination.SourcePath.Abs,
	}
	return *r, Tombstone{
		TombstoneBase: op.NewTombstoneBase(r),
		ClonedPath:    destination.SourcePath.Abs,
	}, nil
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
//   - repo: git resource identifying the local repository
//   - ref: Branch, tag, or commit to check out
func (p *Provider) Checkout(repo Resource, ref string) (Resource, error) {
	cmd := exec.Command("git", "-C", repo.ClonePath, "checkout", ref)
	cmd.Stdout = p.Context().Writer
	cmd.Stderr = p.Context().Writer
	if err := cmd.Run(); err != nil {
		return Resource{}, err
	}
	repo.Ref = ref
	return repo, nil
}

// Pull pulls the latest changes in the given repository directory.
//
// Parameters:
//   - repo: git resource identifying the local repository
func (p *Provider) Pull(repo Resource) (Resource, error) {
	cmd := exec.Command("git", "-C", repo.ClonePath, "pull")
	cmd.Stdout = p.Context().Writer
	cmd.Stderr = p.Context().Writer
	if err := cmd.Run(); err != nil {
		return Resource{}, err
	}
	return repo, nil
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
