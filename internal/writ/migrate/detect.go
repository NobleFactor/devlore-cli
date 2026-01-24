// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"path/filepath"
	"strings"
)

// SourceSystem identifies the dotfile management approach used in the source repository.
type SourceSystem string

const (
	SystemTuckr       SourceSystem = "tuckr"
	SystemStow        SourceSystem = "stow"
	SystemChezmoi     SourceSystem = "chezmoi"
	SystemYadm        SourceSystem = "yadm"
	SystemBareGit     SourceSystem = "bare-git"
	SystemScriptBased SourceSystem = "script-based"
	SystemUnknown     SourceSystem = "unknown"
)

// Detect identifies the source system used in the given directory.
// It checks filesystem signals in priority order and returns the first match.
func Detect(root string) (SourceSystem, error) {
	// 1. Hooks.toml anywhere → tuckr
	if found, _ := findFile(root, "Hooks.toml"); found {
		return SystemTuckr, nil
	}

	// 2. .stow-local-ignore → stow
	if exists(filepath.Join(root, ".stow-local-ignore")) {
		return SystemStow, nil
	}

	// 3. dot_ prefixed dirs → chezmoi
	if hasDotUnderscoreDirs(root) {
		return SystemChezmoi, nil
	}

	// 4. ## in filenames → yadm
	if hasYadmTemplates(root) {
		return SystemYadm, nil
	}

	// 5. Bare git (HEAD/objects/refs at root) → bare-git
	if isBareGit(root) {
		return SystemBareGit, nil
	}

	// 6. <project>-<Platform> directory pattern with known platforms → script-based
	if hasProjectPlatformDirs(root) {
		return SystemScriptBased, nil
	}

	return SystemUnknown, nil
}

// knownPlatforms are the platform values recognized in directory names.
var knownPlatforms = map[string]bool{
	"Darwin":  true,
	"Linux":   true,
	"Unix":    true,
	"Windows": true,
	"Debian":  true,
	"Ubuntu":  true,
	"Arch":    true,
}

// findFile searches recursively for a file with the given name.
func findFile(root, name string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.Name() == name {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// exists checks if a path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hasDotUnderscoreDirs checks for chezmoi-style dot_ prefixed directories.
func hasDotUnderscoreDirs(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "dot_") {
			return true
		}
	}
	return false
}

// hasYadmTemplates checks for yadm-style ## template markers in filenames.
func hasYadmTemplates(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "##") {
			return true
		}
	}
	return false
}

// isBareGit checks if the root looks like a bare git repository.
func isBareGit(root string) bool {
	return exists(filepath.Join(root, "HEAD")) &&
		exists(filepath.Join(root, "objects")) &&
		exists(filepath.Join(root, "refs"))
}

// hasProjectPlatformDirs checks for <project>-<Platform> directory naming.
// Returns true if at least two directories match the pattern.
func hasProjectPlatformDirs(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, platform := parseProjectPlatform(e.Name()); platform != "" {
			count++
			if count >= 2 {
				return true
			}
		}
	}
	return false
}

// parseProjectPlatform splits a directory name on the last dash that precedes
// a known platform value. Returns project and platform (empty if no platform suffix).
func parseProjectPlatform(name string) (project, platform string) {
	// Try splitting from the right on each dash
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			candidate := name[i+1:]
			if knownPlatforms[candidate] {
				return name[:i], candidate
			}
		}
	}
	return name, ""
}
