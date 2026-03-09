// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starstats computes line and byte statistics for Starlark source files.
package starstats

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// FileStats holds line and byte statistics for a single file.
type FileStats struct {
	Path                        string
	Bytes                       int64
	LOC, SLOC, Comments, Blanks int
}

// StatsTotals aggregates statistics across all files.
type StatsTotals struct {
	FileCount                                       int
	TotalBytes                                      int64
	TotalLOC, TotalSLOC, TotalComments, TotalBlanks int
}

// Stats holds per-file and aggregate statistics.
type Stats struct {
	Files  []FileStats
	Totals StatsTotals
}

// Provider computes line and byte statistics for Starlark source files.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Root string
}

// ComputeStats computes line and byte statistics for the given files.
// If withBytes is true, byte counts are included.
// If withLOC is true, line counts (LOC, SLOC, comments, blanks) are included.
func (p *Provider) ComputeStats(files []string, withBytes, withLOC bool) (*Stats, error) {
	result := &Stats{
		Files: make([]FileStats, 0, len(files)),
	}

	for _, absPath := range files {
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}

		relPath, err := filepath.Rel(p.Root, absPath)
		if err != nil {
			relPath = absPath
		}

		fs := FileStats{Path: relPath}

		if withBytes {
			fs.Bytes = int64(len(data))
		}

		if withLOC {
			fs.LOC, fs.SLOC, fs.Comments, fs.Blanks = countLines(data)
		}

		result.Files = append(result.Files, fs)
		result.Totals.FileCount++
		result.Totals.TotalBytes += fs.Bytes
		result.Totals.TotalLOC += fs.LOC
		result.Totals.TotalSLOC += fs.SLOC
		result.Totals.TotalComments += fs.Comments
		result.Totals.TotalBlanks += fs.Blanks
	}

	return result, nil
}

// countLines classifies each line as blank, comment, or code (SLOC).
// A line is blank if it contains only whitespace.
// A line is a comment if its trimmed form starts with '#'.
// All other lines are source lines of code (SLOC).
// LOC is the total number of lines.
func countLines(data []byte) (loc, sloc, comments, blanks int) {
	if len(data) == 0 {
		return 0, 0, 0, 0
	}

	lines := bytes.Split(data, []byte("\n"))

	// If the file ends with a newline, bytes.Split produces a trailing
	// empty element that isn't a real line.
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	loc = len(lines)
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		switch {
		case trimmed == "":
			blanks++
		case strings.HasPrefix(trimmed, "#"):
			comments++
		default:
			sloc++
		}
	}
	return loc, sloc, comments, blanks
}
