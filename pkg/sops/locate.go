// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"os"
	"path/filepath"
	"strings"
)

// locate returns the ordered chain of config files named `name` that govern `startDir`.
//
// The chain is each ancestor directory's `name` file — deepest first — from `startDir` up to and including `root`,
// followed by the global XDG fallback `$XDG_CONFIG_HOME/<xdgRelPath>` (default `~/.config/<xdgRelPath>`). Only files
// that exist are returned, in precedence order: the nearest in-tree file is first, the XDG fallback is last. The walk
// is bounded by `root` — directories outside it are never consulted, and the in-tree walk runs only when `startDir`
// is `root` or a descendant.
//
// This is a pure `os.Stat` walk: it borrows git's per-directory-plus-global resolution *shape* but has no git
// semantics. It must never consult git tracking or `.gitignore` — a `.sops.yaml` is commonly gitignored, and
// discovery has to find it regardless.
//
// Parameters:
//   - `root`: the upper boundary of the upward walk; the walk stops here.
//   - `startDir`: the directory whose governing config chain is wanted.
//   - `name`: the config file name collected at each directory (e.g. `.sops.yaml`).
//   - `xdgRelPath`: the global-fallback path relative to `$XDG_CONFIG_HOME`; empty skips the fallback.
//
// Returns:
//   - `[]string`: absolute paths of the existing config files, deepest in-tree first and the XDG fallback last; nil
//     when none exist.
func locate(root, startDir, name, xdgRelPath string) []string {

	var chain []string

	absRoot, rootErr := filepath.Abs(root)
	dir, dirErr := filepath.Abs(startDir)

	withinRoot := rootErr == nil && dirErr == nil &&
		(dir == absRoot || strings.HasPrefix(dir, absRoot+string(filepath.Separator)))

	if withinRoot {
		for {
			if candidate := filepath.Join(dir, name); fileExists(candidate) {
				chain = append(chain, candidate)
			}
			if dir == absRoot {
				break
			}
			dir = filepath.Dir(dir)
		}
	}

	if xdgRelPath != "" {
		if fallback := xdgConfigPath(xdgRelPath); fallback != "" && fileExists(fallback) {
			chain = append(chain, fallback)
		}
	}

	return chain
}

// fileExists reports whether `path` names an existing regular file (not a directory).
//
// Parameters:
//   - `path`: the filesystem path to test.
//
// Returns:
//   - `bool`: true when `path` exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// xdgConfigPath resolves `relPath` against the XDG config directory (`$XDG_CONFIG_HOME`, default `~/.config`).
//
// Parameters:
//   - `relPath`: the path relative to the XDG config directory.
//
// Returns:
//   - `string`: the resolved path, or empty when the home directory cannot be determined.
func xdgConfigPath(relPath string) string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, relPath)
}
