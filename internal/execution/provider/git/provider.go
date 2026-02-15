// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"io"
	"os/exec"
)

// Provider provides git operations.
type Provider struct{}

// Clone clones a repository from url into path.
func (p *Provider) Clone(url, path string, output io.Writer) error {
	cmd := exec.Command("git", "clone", url, path)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
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
