// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starcode provides Starlark source file capture with glob pattern matching, .gitignore awareness, and optional
// .bzl file inclusion.
package starcode

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/staranalysis"
	"github.com/NobleFactor/devlore-cli/cmd/star/provider/starindex"
	"github.com/NobleFactor/devlore-cli/cmd/star/provider/starstats"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	ignore "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gitignore"
)

// Provider performs Starlark source capture.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Root string
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	if ctx.Root != nil {
		p.Root = ctx.Root.Name()
	}
	return p
}

// Capture collects Starlark source files matching the given pattern.
//
// If gitignore is true, files excluded by .gitignore rules are skipped. If includeBzl is true, .bzl files are included
// alongside .star files.
//
// Parameters:
//   - pattern: the glob pattern to match (supports **).
//   - gitignore: when true, skip files excluded by .gitignore rules.
//   - includeBzl: when true, include .bzl files alongside .star files.
//
// Returns:
//   - *Sources: the captured file set with root and sorted paths.
//   - error: non-nil if the root directory cannot be resolved or the walk fails.
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
		files, err = p.captureRecursive(absRoot, pattern, gitignore, includeBzl)
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
//
// Returns:
//   - []string: relative paths for all captured files.
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
//
// Returns:
//   - int: the file count.
func (s *Sources) Count() int {
	return len(s.Files)
}

// Index parses all captured files and extracts functions, loads, and globals.
// If withDocstrings is true, function docstrings are extracted.
// If withGlobals is true, top-level assignments are captured.
//
// +devlore:defaults withDocstrings=true,withGlobals=true
//
// Parameters:
//   - withDocstrings: when true, extract function docstrings.
//   - withGlobals: when true, capture top-level assignments.
//
// Returns:
//   - *starindex.Index: the parsed index with functions, loads, and globals.
//   - error: non-nil if parsing fails.
func (s *Sources) Index(withDocstrings, withGlobals bool) (*starindex.Index, error) {
	return (&starindex.Provider{Root: s.Root}).IndexFiles(s.Files, withDocstrings, withGlobals)
}

// Stats computes line and byte statistics for all captured files.
// If withBytes is true, byte counts are included.
// If withLOC is true, line counts (LOC, SLOC, comments, blanks) are included.
//
// +devlore:defaults withBytes=true,withLOC=true
//
// Parameters:
//   - withBytes: when true, include byte counts.
//   - withLOC: when true, include line counts (LOC, SLOC, comments, blanks).
//
// Returns:
//   - *starstats.Stats: the computed statistics.
//   - error: non-nil if stat computation fails.
func (s *Sources) Stats(withBytes, withLOC bool) (*starstats.Stats, error) {
	return (&starstats.Provider{Root: s.Root}).ComputeStats(s.Files, withBytes, withLOC)
}

// Analyze performs a combined analysis of all captured files.
//
// +devlore:struct_param cfg=staranalysis.AnalysisConfig
//
// Parameters:
//   - cfg: the analysis configuration.
//
// Returns:
//   - *staranalysis.AnalysisReport: the combined analysis report.
//   - error: non-nil if analysis fails.
func (s *Sources) Analyze(cfg staranalysis.AnalysisConfig) (*staranalysis.AnalysisReport, error) {
	return (&staranalysis.Provider{Root: s.Root}).Analyze(s.Files, cfg)
}

// captureRecursive walks the tree using file.ReceiverType.WalkTree and matches
// files against the glob pattern.
//
// Parameters:
//   - absRoot: the absolute root directory to walk.
//   - pattern: the glob pattern to match (may contain **).
//   - honorGitignore: when true, skip files excluded by .gitignore.
//   - includeBzl: when true, include .bzl files alongside .star files.
//
// Returns:
//   - []string: absolute paths of matched files.
//   - error: non-nil if the walk fails.
func (p *Provider) captureRecursive(absRoot, pattern string, honorGitignore, includeBzl bool) ([]string, error) {
	var files []string

	visitor := file.Reducer(func(initial any, resource *file.Resource, relPath string, _ *op.RecoveryStack) (any, error) {
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

	fp := file.NewProvider(p.ExecutionContext())
	_, _, err := fp.WalkTree(&file.Resource{SourcePath: op.NewPath("", absRoot)}, visitor, honorGitignore)

	if err != nil {
		return nil, err
	}

	return files, nil
}

// captureFlat uses filepath.Glob for non-recursive patterns.
//
// Parameters:
//   - absRoot: the absolute root directory.
//   - pattern: the glob pattern to match.
//   - tracker: optional gitignore tracker (nil to disable filtering).
//   - includeBzl: when true, include .bzl files alongside .star files.
//
// Returns:
//   - []string: absolute paths of matched files.
//   - error: non-nil if the glob fails.
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
//
// Parameters:
//   - path: the file path to check.
//   - includeBzl: when true, .bzl files are also accepted.
//
// Returns:
//   - bool: true if the file is a Starlark source file.
func isStarlarkFile(path string, includeBzl bool) bool {
	ext := filepath.Ext(path)
	if ext == ".star" {
		return true
	}
	return includeBzl && ext == ".bzl"
}

// flattenDoubleStar converts "**/*.star" to a pattern usable with filepath.Match
// by stripping the "**/" prefix. This is a simplistic approach;
// matchRecursivePattern handles the more general case.
//
// Parameters:
//   - pattern: the glob pattern containing **.
//
// Returns:
//   - string: the flattened pattern.
func flattenDoubleStar(pattern string) string {
	// filepath.Match doesn't support **; strip the ** prefix and match only the base pattern portion.
	return strings.ReplaceAll(pattern, "**/", "")
}

// matchRecursivePattern checks if relPath matches a ** glob pattern by
// matching the suffix portion after ** against the path's suffix.
//
// Parameters:
//   - pattern: the glob pattern containing **.
//   - relPath: the relative path to test.
//
// Returns:
//   - bool: true if the path matches the pattern suffix.
//   - error: non-nil if filepath.Match fails.
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
