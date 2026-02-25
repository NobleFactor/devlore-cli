// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package ignore provides gitignore-aware file filtering using go-git's
// gitignore package. It implements a native Go stack-based tracker that
// supports the full Git ignore hierarchy: global ignore, .git/info/exclude,
// and per-directory .gitignore.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	gitcfg "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// PatternSource pairs gitignore patterns with the file they were loaded from.
type PatternSource struct {
	Path     string
	Patterns []gitignore.Pattern
}

// Tracker manages a stack of gitignore pattern sources and provides path
// matching. The stack is ordered from least specific (global ignore) to
// most specific (deepest subdirectory .gitignore). The deepest matching
// rule wins.
type Tracker struct {
	root  string
	stack []PatternSource
	dirs  []string // directory that owns each stack entry ("" for base layers)
}

// NewTracker creates a Tracker rooted at the given directory. It loads three
// base layers: global ignore (core.excludesfile), .git/info/exclude, and
// the root .gitignore.
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
// Returns the ignore status and the source file path that caused the match.
// The most specific (deepest) matching rule wins. Within each layer,
// the last matching pattern takes precedence.
func (t *Tracker) IsIgnored(path string, isDir bool) (ignored bool, source string) {
	segments := strings.Split(filepath.ToSlash(path), "/")

	// Check from most specific (top of stack) to least specific.
	// Within each layer, check patterns in reverse (last match wins).
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

// Push loads the .gitignore from the given directory (relative to root) onto
// the stack. It automatically pops entries from sibling or cousin directories
// that are no longer ancestors of dir.
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
// domain is the path segments of the directory containing the file
// (nil for root-level files).
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
// It reads core.excludesfile from ~/.gitconfig, falling back to
// ${XDG_CONFIG_HOME}/git/ignore or ~/.config/git/ignore.
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
func readGitConfigExcludes(home string) string {
	// Check GIT_CONFIG_GLOBAL env var first, then ~/.gitconfig
	var paths []string
	if env := os.Getenv("GIT_CONFIG_GLOBAL"); env != "" {
		paths = append(paths, env)
	}
	paths = append(paths, filepath.Join(home, ".gitconfig"))

	for _, path := range paths {
		f, err := os.Open(filepath.Clean(path)) //nolint:gosec // path from user's git config
		if err != nil {
			continue
		}

		cfg := gitcfg.New()
		if err := gitcfg.NewDecoder(f).Decode(cfg); err != nil {
			f.Close()
			continue
		}
		f.Close()

		excludes := cfg.Section("core").Option("excludesfile")
		if excludes == "" {
			continue
		}

		// Expand ~ prefix
		if strings.HasPrefix(excludes, "~/") {
			excludes = filepath.Join(home, excludes[2:])
		}
		return excludes
	}
	return ""
}
