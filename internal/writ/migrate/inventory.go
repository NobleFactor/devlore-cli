// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// InventoryEntry represents a single file discovered during inventory.
type InventoryEntry struct {
	RelPath      string // Relative path from source root
	AbsPath      string // Absolute path
	Project      string // Parsed from directory name
	Platform     string // Parsed from directory name (empty if base)
	Class        FileClass
	IsExecutable bool
	SizeBytes    int64
	Observations []string
}

// DirectoryMapping represents a rename from legacy to writ naming.
type DirectoryMapping struct {
	SourceDir string // e.g., "all-Darwin"
	TargetDir string // e.g., "all.Darwin"
	Project   string // e.g., "all"
	Platform  string // e.g., "Darwin"
}

// Inventory walks the source root and builds a list of all files with their
// project and platform assignments.
func Inventory(root string) ([]InventoryEntry, error) {
	root = filepath.Clean(root)
	var entries []InventoryEntry

	topDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, topDir := range topDirs {
		if !topDir.IsDir() {
			continue
		}
		if strings.HasPrefix(topDir.Name(), ".") {
			continue
		}

		project, platform := parseProjectPlatform(topDir.Name())

		dirPath := filepath.Join(root, topDir.Name())
		err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if d.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(root, path)
			info, err := d.Info()
			if err != nil {
				return nil
			}

			entry := InventoryEntry{
				RelPath:      relPath,
				AbsPath:      path,
				Project:      project,
				Platform:     platform,
				IsExecutable: info.Mode()&0111 != 0,
				SizeBytes:    info.Size(),
			}
			entries = append(entries, entry)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries, nil
}

// BuildMappings generates the directory rename mappings for all directories
// that use the <project>-<Platform> convention.
func BuildMappings(root string) ([]DirectoryMapping, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var mappings []DirectoryMapping
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		project, platform := parseProjectPlatform(e.Name())
		if platform == "" {
			continue // no platform suffix, no rename needed
		}
		mappings = append(mappings, DirectoryMapping{
			SourceDir: e.Name(),
			TargetDir: project + "." + platform,
			Project:   project,
			Platform:  platform,
		})
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].SourceDir < mappings[j].SourceDir
	})

	return mappings, nil
}

// UniqueProjects returns a sorted list of unique project names from entries.
func UniqueProjects(entries []InventoryEntry) []string {
	seen := make(map[string]bool)
	for _, e := range entries {
		seen[e.Project] = true
	}
	var projects []string
	for p := range seen {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	return projects
}

// UniquePlatforms returns a sorted list of unique platform values from entries.
func UniquePlatforms(entries []InventoryEntry) []string {
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.Platform != "" {
			seen[e.Platform] = true
		}
	}
	var platforms []string
	for p := range seen {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	return platforms
}
