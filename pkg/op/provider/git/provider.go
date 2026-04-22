// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"os"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	netprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
)

// Provider provides git actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
	// Test hooks. Nil means use real git commands.
	cloneFn func(url, path string) error
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// --- Compensable Pairs ---

// Clone clones a repository from url into destinationPath.
//
// Identity for the cloned repository is constructed by [git.NewResource].
//
// Parameters:
//   - url: network resource identifying the git repository.
//   - destinationPath: the local directory path where the repository will be cloned.
//
// Returns:
//   - *Resource: the cloned git.Resource with populated metadata.
//   - Tombstone: compensation state for removing the clone directory.
//   - error: any error from cloning.
func (p *Provider) Clone(url *netprov.Resource, destinationPath string) (*Resource, Tombstone, error) {

	destination, err := NewResource(p.ExecutionContext(), destinationPath)

	if err != nil {
		return nil, Tombstone{}, err
	}

	if err := p.doClone(url.SourceURL.String(), destination.ClonePath); err != nil {
		return nil, Tombstone{}, err
	}

	destination.URL = url.SourceURL.String()

	if err := destination.Resolve(); err != nil {
		return destination, Tombstone{}, err
	}

	return destination, Tombstone{
		TombstoneBase: op.NewTombstoneBase(destination),
		ClonedPath:    destination.ClonePath,
	}, nil
}

// CompensateClone removes the cloned directory.
func (p *Provider) CompensateClone(state Tombstone) error {
	if state.ClonedPath == "" {
		return nil
	}
	return os.RemoveAll(state.ClonedPath)
}

// --- Standalone Methods ---

// Checkout checks out a ref in the given repository directory.
//
// Parameters:
//   - repo: git resource identifying the local repository
//   - ref: Branch, tag, or commit to check out
func (p *Provider) Checkout(repo *Resource, ref string) (*Resource, error) {
	cmd := exec.Command("git", "-C", repo.ClonePath, "checkout", ref)
	cmd.Stdout = p.ExecutionContext().Writer
	cmd.Stderr = p.ExecutionContext().Writer
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	repo.Ref = ref
	return repo, nil
}

// Pull pulls the latest changes in the given repository directory.
//
// Parameters:
//   - repo: git resource identifying the local repository
func (p *Provider) Pull(repo *Resource) (*Resource, error) {
	cmd := exec.Command("git", "-C", repo.ClonePath, "pull")
	cmd.Stdout = p.ExecutionContext().Writer
	cmd.Stderr = p.ExecutionContext().Writer
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (p *Provider) doClone(url, path string) error {
	if p.cloneFn != nil {
		return p.cloneFn(url, path)
	}
	cmd := exec.Command("git", "clone", url, path)
	cmd.Stdout = p.ExecutionContext().Writer
	cmd.Stderr = p.ExecutionContext().Writer
	return cmd.Run()
}
