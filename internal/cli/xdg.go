// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
)

// XDG Base Directory Specification paths.
// See: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html

// ConfigHome returns XDG_CONFIG_HOME or ~/.config
func ConfigHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".config")
}

// DataHome returns XDG_DATA_HOME or ~/.local/share
func DataHome() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

// CacheHome returns XDG_CACHE_HOME or ~/.cache
func CacheHome() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".cache")
}

// StateHome returns XDG_STATE_HOME or ~/.local/state
func StateHome() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state")
}

// ManPath returns the user man page directory: XDG_DATA_HOME/man/man1
func ManPath() string {
	return filepath.Join(DataHome(), "man", "man1")
}

// BashCompletionPath returns the bash completion directory.
// XDG_DATA_HOME/bash-completion/completions
func BashCompletionPath() string {
	return filepath.Join(DataHome(), "bash-completion", "completions")
}

// ZshCompletionPath returns the zsh completion directory.
// XDG_DATA_HOME/zsh/site-functions
func ZshCompletionPath() string {
	return filepath.Join(DataHome(), "zsh", "site-functions")
}

// FishCompletionPath returns the fish completion directory.
// XDG_CONFIG_HOME/fish/completions
func FishCompletionPath() string {
	return filepath.Join(ConfigHome(), "fish", "completions")
}

// Unified devlore paths for shared configuration across writ and lore.

// DevloreConfigHome returns the unified devlore config directory.
// XDG_CONFIG_HOME/devlore
func DevloreConfigHome() string {
	return filepath.Join(ConfigHome(), "devlore")
}

// DevloreCacheHome returns the unified devlore cache directory.
// XDG_CACHE_HOME/devlore
func DevloreCacheHome() string {
	return filepath.Join(CacheHome(), "devlore")
}

// DevloreDataHome returns the unified devlore data directory.
// XDG_DATA_HOME/devlore
func DevloreDataHome() string {
	return filepath.Join(DataHome(), "devlore")
}

// DevloreStateHome returns the unified devlore state directory.
// XDG_STATE_HOME/devlore
func DevloreStateHome() string {
	return filepath.Join(StateHome(), "devlore")
}
