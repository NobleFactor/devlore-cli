// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starcode provides Starlark source file capture with glob pattern matching,
// .gitignore awareness, and optional .bzl file inclusion.
package starcode

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	ignore "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gitignore"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/staranalysis"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starindex"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starstats"
)

// Provider performs Starlark source capture.
//
// +devlore:access=immediate
// +devlore:bind Root=WorkDir
type Provider struct {
	op.ProviderBase
	Root string
}

// Capture collects Starlark source files matching the given pattern.
// If gitignore is true, files excluded by .gitignore rules are skipped.
// If includeBzl is true, .bzl files are included alongside .star files.
//
// +devlore:defaults gitignore=true,includeBzl=true
func (p *Provider) Capture(pattern string, gitignore, includeBzl bool) (*Sources, error) {
	root := p.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var files []string

	if strings.Contains(pattern, "**") {
		files, err = captureRecursive(absRoot, pattern, gitignore, includeBzl)
	} else {
		var tracker *ignore.Tracker
		if gitignore {
			tracker, err = ignore.NewTracker(absRoot)
			if err != nil {
				return nil, err
			}
		}
		files, err = captureFlat(absRoot, pattern, tracker, includeBzl)
	}
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return &Sources{Root: absRoot, Files: files}, nil
}

// Sources holds a captured set of Starlark source files.
type Sources struct {
	Root  string   // absolute root directory
	Files []string // absolute paths, sorted
}

// Paths returns the file paths relative to Root.
func (s *Sources) Paths() []string {
	paths := make([]string, len(s.Files))
	for i, f := range s.Files {
		rel, err := filepath.Rel(s.Root, f)
		if err != nil {
			paths[i] = f
		} else {
			paths[i] = rel
		}
	}
	return paths
}

// Count returns the number of captured files.
func (s *Sources) Count() int {
	return len(s.Files)
}

// Index parses all captured files and extracts functions, loads, and globals.
// If withDocstrings is true, function docstrings are extracted.
// If withGlobals is true, top-level assignments are captured.
//
// +devlore:defaults withDocstrings=true,withGlobals=true
func (s *Sources) Index(withDocstrings, withGlobals bool) (*starindex.Index, error) {
	return (&starindex.Provider{Root: s.Root}).IndexFiles(s.Files, withDocstrings, withGlobals)
}

// Stats computes line and byte statistics for all captured files.
// If withBytes is true, byte counts are included.
// If withLOC is true, line counts (LOC, SLOC, comments, blanks) are included.
//
// +devlore:defaults withBytes=true,withLOC=true
func (s *Sources) Stats(withBytes, withLOC bool) (*starstats.Stats, error) {
	return (&starstats.Provider{Root: s.Root}).ComputeStats(s.Files, withBytes, withLOC)
}

// Analyze performs a combined analysis of all captured files.
//
// +devlore:struct_param cfg=staranalysis.AnalysisConfig
func (s *Sources) Analyze(cfg staranalysis.AnalysisConfig) (*staranalysis.AnalysisReport, error) {
	return (&staranalysis.Provider{Root: s.Root}).Analyze(s.Files, cfg)
}

// captureRecursive walks the tree using file.Provider.WalkTree and matches files against the glob pattern.
func captureRecursive(absRoot, pattern string, honorGitignore, includeBzl bool) ([]string, error) {
	var files []string

	visitor := file.Reducer(func(initial any, resource file.Resource, relPath string, _ *op.RecoveryStack) (any, error) {
		if resource.Mode.IsDir() {
			return initial, nil
		}
		if !isStarlarkFile(relPath, includeBzl) {
			return initial, nil
		}
		matched, err := filepath.Match(flattenDoubleStar(pattern), relPath)
		if err != nil {
			// Try matching just the filename against the pattern's base
			matched, err = matchRecursivePattern(pattern, relPath)
			if err != nil {
				return initial, err
			}
		}
		if matched {
			files = append(files, filepath.Join(absRoot, relPath))
		}
		return initial, nil
	})

	_, _, err := (&file.Provider{}).WalkTree(file.Resource{SourcePath: file.SourcePath{Abs: absRoot}}, visitor, honorGitignore)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// captureFlat uses filepath.Glob for non-recursive patterns.
func captureFlat(absRoot, pattern string, tracker *ignore.Tracker, includeBzl bool) ([]string, error) {
	fullPattern := filepath.Join(absRoot, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, absPath := range matches {
		relPath, err := filepath.Rel(absRoot, absPath)
		if err != nil {
			continue
		}

		if !isStarlarkFile(relPath, includeBzl) {
			continue
		}

		if tracker != nil {
			ignored, _ := tracker.IsIgnored(relPath, false)
			if ignored {
				continue
			}
		}

		files = append(files, absPath)
	}

	return files, nil
}

// isStarlarkFile returns true if the path has a .star extension (or .bzl if includeBzl is true).
func isStarlarkFile(path string, includeBzl bool) bool {
	ext := filepath.Ext(path)
	if ext == ".star" {
		return true
	}
	return includeBzl && ext == ".bzl"
}

// flattenDoubleStar converts "**/*.star" to a pattern usable with filepath.Match by replacing "**/" with any prefix match.
// This is a simplistic approach; matchRecursivePattern handles the more general case.
func flattenDoubleStar(pattern string) string {
	// filepath.Match doesn't support **; strip the ** prefix and match only the base pattern portion.
	return strings.ReplaceAll(pattern, "**/", "")
}

// matchRecursivePattern checks if relPath matches a ** glob pattern by matching the suffix portion after ** against
// the path's suffix.
func matchRecursivePattern(pattern, relPath string) (bool, error) {
	// Split on ** and match the suffix
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return false, nil
	}

	suffix := strings.TrimPrefix(parts[1], "/")
	if suffix == "" {
		return true, nil
	}

	// Check if the file's base name or relative suffix matches
	return filepath.Match(suffix, filepath.Base(relPath))
}
