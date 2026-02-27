// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package gitignore provides gitignore-aware file filtering using go-git's gitignore package. It implements a native
// Go stack-based tracker that supports the full Git ignore hierarchy: global gitignore, .git/info/exclude, and
// per-directory .gitignore.
package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Tracker manages a stack of gitignore pattern sources and provides path matching.
//
// The stack is ordered from the least specific (global ignore) to the most specific (deepest subdirectory .gitignore).
// The deepest matching rule wins.
type Tracker struct {
	root  string
	stack []PatternSource
	dirs  []string // directory that owns each stack entry ("" for base layers)
}

// PatternSource pairs gitignore patterns with the file they were loaded from.
type PatternSource struct {
	Path     string
	Patterns []gitignore.Pattern
}

// NewTracker creates a Tracker rooted at the given directory.
//
// The Tracker created loads three base layers: global gitignore (core.excludesfile), .git/info/exclude, and the
// root .gitignore.
//
// Parameters:
//   - root: Absolute path to the root directory of the git repository
//
// Returns:
//   - *Tracker: The initialized Tracker
func NewTracker(root string) (*Tracker, error) {

	absRoot, err := filepath.Abs(root)

	if err != nil {
		return nil, err
	}

	t := &Tracker{root: absRoot}

	// Layer 0: Global ignore (core.excludesfile or XDG fallback)

	if globalPath := resolveGlobalIgnore(); globalPath != "" {
		if patterns := loadPatterns(globalPath, nil); len(patterns) > 0 {
			t.stack = append(t.stack, PatternSource{
				Path:     globalPath,
				Patterns: patterns,
			})
			t.dirs = append(t.dirs, "")
		}
	}

	// Layer 1: .git/info/exclude

	excludePath := filepath.Join(absRoot, ".git", "info", "exclude")

	if patterns := loadPatterns(excludePath, nil); len(patterns) > 0 {
		t.stack = append(t.stack, PatternSource{
			Path:     excludePath,
			Patterns: patterns,
		})
		t.dirs = append(t.dirs, "")
	}

	// Layer 2: Root .gitignore

	rootGI := filepath.Join(absRoot, ".gitignore")

	if patterns := loadPatterns(rootGI, nil); len(patterns) > 0 {
		t.stack = append(t.stack, PatternSource{
			Path:     rootGI,
			Patterns: patterns,
		})
		t.dirs = append(t.dirs, "")
	}

	return t, nil
}

// Root returns the absolute root directory of this tracker.
func (t *Tracker) Root() string {
	return t.root
}

// IsIgnored checks whether a relative path (from root) should be ignored.
//
// The most specific (deepest) matching rule wins. Within each layer, the last matching pattern takes precedence.
//
// Parameters:
//   - path: Relative path to check (e.g. "src/main.cpp")
//   - isDir: True if the path is a directory (e.g. "src/main.cpp" is a directory, not a file)
//
// Returns: true if the path is ignored, false if the path is not ignored.
func (t *Tracker) IsIgnored(path string, isDir bool) (ignored bool, source string) {

	segments := strings.Split(filepath.ToSlash(path), "/")

	// Check from the most specific (top of stack) to the least specific. Within each layer, check patterns in
	// reverse (last match wins).

	for i := len(t.stack) - 1; i >= 0; i-- {
		for j := len(t.stack[i].Patterns) - 1; j >= 0; j-- {
			result := t.stack[i].Patterns[j].Match(segments, isDir)
			switch result {
			case gitignore.Exclude:
				return true, t.stack[i].Path
			case gitignore.Include:
				return false, t.stack[i].Path
			}
		}
	}
	return false, ""
}

// Push loads the .gitignore from the given directory (relative to root) onto the stack.
//
// It automatically pops entries from sibling or cousin directories that are no longer ancestors of dir.
//
// Parameters:
//   - dir: Relative path to the directory containing the .gitignore file (e.g. "src/main.cpp")
func (t *Tracker) Push(dir string) {

	dirSlash := filepath.ToSlash(dir)

	// Pop entries that are not ancestors of dir

	for len(t.dirs) > 0 {
		top := t.dirs[len(t.dirs)-1]
		if top == "" {
			break // base layers are never popped
		}
		topSlash := filepath.ToSlash(top)
		if dirSlash == topSlash || strings.HasPrefix(dirSlash, topSlash+"/") {
			break // top is ancestor of (or equal to) dir
		}
		t.stack = t.stack[:len(t.stack)-1]
		t.dirs = t.dirs[:len(t.dirs)-1]
	}

	// Don't push if this directory is already on top

	if len(t.dirs) > 0 && t.dirs[len(t.dirs)-1] == dir {
		return
	}

	giPath := filepath.Join(t.root, dir, ".gitignore")
	domain := strings.Split(dirSlash, "/")

	if patterns := loadPatterns(giPath, domain); len(patterns) > 0 {
		t.stack = append(t.stack, PatternSource{
			Path:     giPath,
			Patterns: patterns,
		})
		t.dirs = append(t.dirs, dir)
	}
}

// loadPatterns reads a gitignore file and returns parsed patterns.
//
// Parameters:
//   - domain: the path segments of the directory containing the file (nil for root-level files).
//
// Returns:
//   - The parsed patterns from the gitignore file
func loadPatterns(path string, domain []string) []gitignore.Pattern {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	return patterns
}

// resolveGlobalIgnore finds the global gitignore file path.
//
// It reads core.excludesfile from ~/.gitconfig, falling back to ${XDG_CONFIG_HOME}/git/ignore or ~/.config/git/ignore.
// If the path is not a valid path, it returns an empty string.
//
// Returns: The global gitignore file path or an empty string if not found
func resolveGlobalIgnore() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Try reading core.excludesfile from git config
	if excludes := readGitConfigExcludes(home); excludes != "" {
		if _, err := os.Stat(excludes); err == nil {
			return excludes
		}
	}

	// Fall back to XDG standard path
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	globalIgnore := filepath.Clean(filepath.Join(xdgConfig, "git", "ignore"))
	if _, err := os.Stat(globalIgnore); err == nil { //nolint:gosec // path from user's XDG config
		return globalIgnore
	}

	return ""
}

// readGitConfigExcludes reads core.excludesfile from git config files.
//
// Parameters:
//   - home: The home directory of the user
//
// Returns: The path of the file if it is found in the global git configs--following XDG and Git priority standards--
// otherwise an empty string.
func readGitConfigExcludes(home string) string {

	var paths []string

	// 1. Highest Priority: GIT_CONFIG_GLOBAL override

	if env := os.Getenv("GIT_CONFIG_GLOBAL"); env != "" {
		paths = append(paths, env)
	} else {

		// 2. XDG Priority: Check XDG_CONFIG_HOME or default ~/.config/git/config

		xdgConfig := os.Getenv("XDG_CONFIG_HOME")

		if xdgConfig != "" {
			paths = append(paths, filepath.Join(xdgConfig, "git", "config"))
		} else if home != "" {
			paths = append(paths, filepath.Join(home, ".config", "git", "config"))
		}

		// 3. Legacy Priority: ~/.gitconfig

		if home != "" {
			paths = append(paths, filepath.Join(home, ".gitconfig"))
		}
	}

	for _, path := range paths {

		f, err := os.Open(path)

		if err != nil {
			continue
		}

		cfg := config.New()
		decoder := config.NewDecoder(f)

		if err := decoder.Decode(cfg); err != nil {
			f.Close()
			continue
		}

		f.Close()

		excludes := cfg.Section("core").Option("excludesfile")

		if excludes != "" {
			return expandGitPath(excludes, home)
		}
	}

	return ""
}

// expandGitPath handles the ~ prefix for paths found within config files.
//
// Parameters:
//   - path: The path to expand
//   - home: The home directory of the user
//
// Returns: The expanded path with ~ prefix replaced by the home directory
func expandGitPath(path, home string) string {
	if home != "" && strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
