// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package segment

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MatchDirectories finds all matching directories in sourceRoot for the given projects.
// It returns results sorted by project name, then by specificity (most specific first).
func MatchDirectories(sourceRoot string, projects []string, segs Segments) ([]MatchResult, error) {
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return nil, err
	}

	// Build set of requested projects for fast lookup
	projectSet := make(map[string]bool)
	for _, p := range projects {
		projectSet[p] = true
	}

	var results []MatchResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirname := entry.Name()

		// Skip hidden directories
		if strings.HasPrefix(dirname, ".") {
			continue
		}

		project, suffixes := ParseDirName(dirname)

		// Check if this project was requested
		if !projectSet[project] {
			continue
		}

		// Check if suffixes match current segments
		if !segs.Match(dirname) {
			continue
		}

		results = append(results, MatchResult{
			Path:     filepath.Join(sourceRoot, dirname),
			Project:  project,
			Suffixes: suffixes,
		})
	}

	// Sort: by project name, then by specificity (descending)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Project != results[j].Project {
			return results[i].Project < results[j].Project
		}
		return results[i].Specificity() > results[j].Specificity()
	})

	return results, nil
}

// MatchAllProjects finds all matching directories for all projects in sourceRoot.
// It returns results sorted by project name, then by specificity (most specific first).
func MatchAllProjects(sourceRoot string, segs Segments) ([]MatchResult, error) {
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return nil, err
	}

	var results []MatchResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirname := entry.Name()

		// Skip hidden directories
		if strings.HasPrefix(dirname, ".") {
			continue
		}

		project, suffixes := ParseDirName(dirname)

		// Check if suffixes match current segments
		if !segs.Match(dirname) {
			continue
		}

		results = append(results, MatchResult{
			Path:     filepath.Join(sourceRoot, dirname),
			Project:  project,
			Suffixes: suffixes,
		})
	}

	// Sort: by project name, then by specificity (descending)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Project != results[j].Project {
			return results[i].Project < results[j].Project
		}
		return results[i].Specificity() > results[j].Specificity()
	})

	return results, nil
}

// GroupByProject groups match results by project name.
// Within each group, directories are sorted by specificity (most specific first).
func GroupByProject(results []MatchResult) map[string][]MatchResult {
	groups := make(map[string][]MatchResult)
	for _, r := range results {
		groups[r.Project] = append(groups[r.Project], r)
	}
	return groups
}

// ProjectNames returns the unique project names from match results in sorted order.
func ProjectNames(results []MatchResult) []string {
	seen := make(map[string]bool)
	var names []string
	for _, r := range results {
		if !seen[r.Project] {
			seen[r.Project] = true
			names = append(names, r.Project)
		}
	}
	sort.Strings(names)
	return names
}
