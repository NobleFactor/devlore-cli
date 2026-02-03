// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
)

// SystemGit implements system.git.* bindings for git queries.
// These are immediate queries - they execute during analysis, not deferred.
type SystemGit struct{}

// NewSystemGit creates a new SystemGit.
func NewSystemGit() *SystemGit {
	return &SystemGit{}
}

// Starlark Value interface
func (g *SystemGit) String() string        { return "system.git" }
func (g *SystemGit) Type() string          { return "system.git" }
func (g *SystemGit) Freeze()               {}
func (g *SystemGit) Truth() starlark.Bool  { return true }
func (g *SystemGit) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: system.git") }

// Starlark HasAttrs interface
func (g *SystemGit) Attr(name string) (starlark.Value, error) {
	switch name {
	case "installed":
		return starlark.NewBuiltin("system.git.installed", g.installed), nil
	case "version":
		return starlark.NewBuiltin("system.git.version", g.version), nil
	case "repo_root":
		return starlark.NewBuiltin("system.git.repo_root", g.repoRoot), nil
	case "current_branch":
		return starlark.NewBuiltin("system.git.current_branch", g.currentBranch), nil
	case "is_clean":
		return starlark.NewBuiltin("system.git.is_clean", g.isClean), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("system.git has no attribute %q", name))
	}
}

func (g *SystemGit) AttrNames() []string {
	return []string{"current_branch", "installed", "is_clean", "repo_root", "version"}
}

// installed checks if git is installed.
// Usage: system.git.installed()
//
// Returns: True if git is available in PATH
func (g *SystemGit) installed(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("git")
	return starlark.Bool(err == nil), nil
}

// version returns the installed git version.
// Usage: system.git.version()
//
// Returns: Git version string (e.g., "2.43.0"), or empty if not installed
func (g *SystemGit) version(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	parts := strings.Fields(string(output))
	if len(parts) >= 3 {
		return starlark.String(parts[2]), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// repoRoot returns the root directory of the current git repository.
// Usage: system.git.repo_root()
//
// Returns: Path to repository root, or empty string if not in a repo
func (g *SystemGit) repoRoot(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// currentBranch returns the current git branch name.
// Usage: system.git.current_branch()
//
// Returns: Current branch name, or empty string if not in a repo
func (g *SystemGit) currentBranch(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// isClean checks if the working directory has no uncommitted changes.
// Usage: system.git.is_clean()
//
// Returns: True if there are no uncommitted changes
func (g *SystemGit) isClean(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(len(strings.TrimSpace(string(output))) == 0), nil
}
