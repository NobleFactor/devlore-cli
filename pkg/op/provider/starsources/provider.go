// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starsources holds a captured set of Starlark source files and
// provides delegation methods for indexing, stats, and analysis.
package starsources

import (
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op/provider/staranalysis"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starindex"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starstats"
)

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
// Each file is parsed once to extract both complexity and index data.
//
// +devlore:struct_param cfg=staranalysis.AnalysisConfig
func (s *Sources) Analyze(cfg staranalysis.AnalysisConfig) (*staranalysis.AnalysisReport, error) {
	return (&staranalysis.Provider{Root: s.Root}).Analyze(s.Files, cfg)
}
