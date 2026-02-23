// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package reconcile implements drift detection and repair for writ deployments.
package reconcile

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// State represents the reconciliation state of a deployed entry.
type State int

const (
	// StateLinked means the symlink exists and points correctly.
	StateLinked State = iota
	// StateCopied means the file was copied and exists.
	StateCopied
	// StateConflict means the target exists but doesn't match expectations.
	StateConflict
	// StateMissing means the expected target doesn't exist.
	StateMissing
	// StateOrphan means the symlink target no longer exists.
	StateOrphan
	// StateStale means the source changed since deployment.
	StateStale
	// StateModified means the target was locally modified.
	StateModified
	// StateDriftConflict means both source and target changed.
	StateDriftConflict
)

// String returns the status indicator for display.
func (s State) String() string {
	switch s {
	case StateLinked:
		return "✓ Linked"
	case StateCopied:
		return "✓ Copied"
	case StateConflict:
		return "⚠ Conflict"
	case StateMissing:
		return "✗ Missing"
	case StateOrphan:
		return "? Orphan"
	case StateStale:
		return "↑ Stale"
	case StateModified:
		return "M Modified"
	case StateDriftConflict:
		return "! Conflict"
	default:
		return "? Unknown"
	}
}

// Label returns a machine-readable label for JSON output.
func (s State) Label() string {
	switch s {
	case StateLinked:
		return "linked"
	case StateCopied:
		return "copied"
	case StateConflict:
		return "conflict"
	case StateMissing:
		return "missing"
	case StateOrphan:
		return "orphan"
	case StateStale:
		return "stale"
	case StateModified:
		return "modified"
	case StateDriftConflict:
		return "drift_conflict"
	default:
		return "unknown"
	}
}

// Entry represents a single file in the reconcile report.
type Entry struct {
	RelTarget string
	Source    string
	Target   string
	Project  string
	Action   string
	State    State
	Message  string
}

// Report contains the reconciliation results.
type Report struct {
	TargetRoot  string
	SourceRoot  string
	Projects    []string
	FromReceipt bool
	ReceiptPath string
	Entries     []Entry
}

// Summary returns counts by state.
func (r *Report) Summary() map[State]int {
	counts := make(map[State]int)
	for _, e := range r.Entries {
		counts[e.State]++
	}
	return counts
}

// FromBuildResult creates a report from a tree build result by scanning
// the target directory for each expected file.
func FromBuildResult(result *tree.BuildResult) *Report {
	report := &Report{
		TargetRoot: result.TargetRoot,
		SourceRoot: result.SourceRoot,
	}

	projects := make(map[string]bool)
	for _, f := range result.Files {
		if f.Project != "" {
			projects[f.Project] = true
		}

		entry := Entry{
			RelTarget: f.ID,
			Source:    f.Source,
			Target:   f.Target,
			Project:  f.Project,
		}

		info, err := os.Lstat(f.Target)
		switch {
		case os.IsNotExist(err):
			entry.State = StateMissing
			entry.Message = "not deployed"
		case err != nil:
			entry.State = StateConflict
			entry.Message = err.Error()
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, readErr := os.Readlink(f.Target)
			switch {
			case readErr != nil:
				entry.State = StateConflict
				entry.Message = "cannot read symlink"
			case filepath.Clean(linkTarget) == filepath.Clean(f.Source):
				entry.State = StateLinked
			default:
				entry.State = StateConflict
				entry.Message = "symlink points to " + linkTarget
			}
		default:
			entry.State = StateCopied
		}

		report.Entries = append(report.Entries, entry)
	}

	for p := range projects {
		report.Projects = append(report.Projects, p)
	}

	return report
}

// ScanTarget scans the target directory for symlinks pointing to source.
func ScanTarget(targetRoot, sourceRoot string) *Report {
	report := &Report{
		TargetRoot: targetRoot,
		SourceRoot: sourceRoot,
	}

	err := filepath.Walk(targetRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentional: skip unreadable entries during walk
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil //nolint:nilerr // intentional: skip unreadable symlinks during walk
		}

		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
		}
		linkTarget = filepath.Clean(linkTarget)

		if !strings.HasPrefix(linkTarget, sourceRoot) {
			return nil
		}

		relTarget, relErr := filepath.Rel(targetRoot, path)
		if relErr != nil {
			relTarget = path // use absolute path as fallback
		}
		entry := Entry{
			RelTarget: relTarget,
			Source:    linkTarget,
			Target:   path,
			Action:   "file.link",
		}

		if _, err := os.Stat(linkTarget); os.IsNotExist(err) {
			entry.State = StateOrphan
			entry.Message = "source file deleted"
		} else {
			entry.State = StateLinked
		}

		report.Entries = append(report.Entries, entry)
		return nil
	})
	if err != nil {
		report.Entries = append(report.Entries, Entry{
			RelTarget: targetRoot,
			State:     StateConflict,
			Message:   "walk error: " + err.Error(),
		})
	}

	return report
}
