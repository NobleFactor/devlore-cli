// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package config provides unified configuration for star commands.
// Configuration is loaded from a hierarchy of config.yaml files:
//  1. ${GIT_TOPLEVEL}/star/config.yaml (project - highest priority)
//  2. ${XDG_CONFIG_HOME}/star/config.yaml (user defaults)
//  3. Extension defaults (from extension.yaml files)
package config

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-git/v5"
)

// gitWorkspaceRoot caches the git repository root path.
// Empty string means not in a git repo (or git not available).
var (
	gitWorkspaceRoot     string
	gitWorkspaceRootOnce sync.Once
	gitWorkspaceRootSet  bool // true if explicitly set (for testing)
)

// initGitWorkspaceRoot finds the git repository root once using go-git.
// Returns empty string if not in a git repo.
func initGitWorkspaceRoot() string {
	gitWorkspaceRootOnce.Do(func() {
		if gitWorkspaceRootSet {
			return // Already set by SetGitWorkspaceRoot
		}

		// Start from current directory and search up for .git
		cwd, err := os.Getwd()
		if err != nil {
			gitWorkspaceRoot = ""
			return
		}

		// PlainOpenWithOptions with DetectDotGit walks up the directory tree
		repo, err := git.PlainOpenWithOptions(cwd, &git.PlainOpenOptions{
			DetectDotGit: true,
		})
		if err != nil {
			gitWorkspaceRoot = ""
			return
		}

		// Get the worktree to find the fsroot path
		wt, err := repo.Worktree()
		if err != nil {
			gitWorkspaceRoot = ""
			return
		}

		gitWorkspaceRoot = wt.Filesystem.Root()
	})
	return gitWorkspaceRoot
}

// GitWorkspaceRoot returns the cached git repository root.
// Returns empty string if not in a git repo.
func GitWorkspaceRoot() string {
	return initGitWorkspaceRoot()
}

// SetGitWorkspaceRoot sets the git workspace root for testing.
// Do ResetGitWorkspaceRoot to restore normal behavior.
func SetGitWorkspaceRoot(path string) {
	gitWorkspaceRoot = path
	gitWorkspaceRootSet = true
	gitWorkspaceRootOnce.Do(func() {}) // Mark as done
}

// ResetGitWorkspaceRoot resets the git workspace root cache.
// The next call to GitWorkspaceRoot will re-detect from git.
func ResetGitWorkspaceRoot() {
	gitWorkspaceRoot = ""
	gitWorkspaceRootSet = false
	gitWorkspaceRootOnce = sync.Once{}
}

// ConfigSource describes a configuration file location and whether it exists.
type ConfigSource struct {
	Path   string
	Exists bool
}

// userConfigPath returns the path to the user's config.yaml.
func userConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "star", "config.yaml")
}

// projectConfigPath returns the path to the project's config.yaml.
// Returns empty string if not in a git repository.
func projectConfigPath() string {
	root := GitWorkspaceRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "star", "config.yaml")
}
