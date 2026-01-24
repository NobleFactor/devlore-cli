// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package status provides symlink status checking for writ deployments.
package status

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/writ/receipt"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// nodeIsDelegate returns true if the node's operations contain only "delegate".
func nodeIsDelegate(ops []string) bool {
	return len(ops) == 1 && ops[0] == "delegate"
}

// State represents the status of a deployed file.
type State int

const (
	// StateLinked means the symlink exists and points to the correct source.
	StateLinked State = iota
	// StateConflict means a file exists at the target but isn't our symlink.
	StateConflict
	// StateMissing means the source file exists but target symlink is missing.
	StateMissing
	// StateOrphan means the symlink points to a nonexistent file.
	StateOrphan
	// StateCopied means the file was copied (template/secret) and matches expected.
	StateCopied
	// StateStale means the source template/secret has changed since deployment.
	StateStale
	// StateModified means the target file was edited after deployment.
	StateModified
	// StateDriftConflict means both source and target changed since deployment.
	StateDriftConflict
)

// String returns the status indicator for display.
func (s State) String() string {
	switch s {
	case StateLinked:
		return "✓"
	case StateConflict:
		return "⚠"
	case StateMissing:
		return "✗"
	case StateOrphan:
		return "?"
	case StateCopied:
		return "✓"
	case StateStale:
		return "↑" // Source changed, needs redeploy
	case StateModified:
		return "M" // Target modified
	case StateDriftConflict:
		return "!" // Both changed
	default:
		return "?"
	}
}

// Label returns a human-readable label for the state.
func (s State) Label() string {
	switch s {
	case StateLinked:
		return "linked"
	case StateConflict:
		return "conflict"
	case StateMissing:
		return "missing"
	case StateOrphan:
		return "orphan"
	case StateCopied:
		return "copied"
	case StateStale:
		return "stale"
	case StateModified:
		return "modified"
	case StateDriftConflict:
		return "drift-conflict"
	default:
		return "unknown"
	}
}

// Entry represents the status of a single file.
type Entry struct {
	// RelTarget is the relative path from target root (e.g., .bashrc)
	RelTarget string

	// Source is the absolute path to the source file
	Source string

	// Target is the absolute path to the target file
	Target string

	// State is the current status
	State State

	// Project is the project this belongs to
	Project string

	// Operations that were/should be performed
	Operations []string

	// Message provides additional context (e.g., "points to wrong file")
	Message string

	// SourceChecksum is the expected source checksum (from receipt)
	SourceChecksum string

	// TargetChecksum is the expected target checksum (from receipt)
	TargetChecksum string
}

// Report contains the full status report.
type Report struct {
	// TargetRoot is the deployment target (e.g., $HOME)
	TargetRoot string

	// SourceRoot is the dotfiles repository path
	SourceRoot string

	// Projects checked
	Projects []string

	// Entries are the individual file statuses
	Entries []Entry

	// FromReceipt indicates status was computed from a receipt
	FromReceipt bool

	// ReceiptPath is the path to the receipt used (if any)
	ReceiptPath string
}

// Summary returns counts of each state.
func (r *Report) Summary() map[State]int {
	counts := make(map[State]int)
	for _, e := range r.Entries {
		counts[e.State]++
	}
	return counts
}

// HasIssues returns true if there are any non-linked/copied states.
func (r *Report) HasIssues() bool {
	for _, e := range r.Entries {
		if e.State != StateLinked && e.State != StateCopied {
			return true
		}
	}
	return false
}

// FromReceipt generates status by checking entries in a receipt.
// For copied files without drift detection, use this function.
func FromReceipt(rcpt *receipt.Receipt, receiptPath string) *Report {
	return FromReceiptWithDrift(rcpt, receiptPath, false)
}

// FromReceiptWithDrift generates status from a receipt with optional drift detection.
// When checkDrift is true, checksums are compared to detect source/target changes.
func FromReceiptWithDrift(rcpt *receipt.Receipt, receiptPath string, checkDrift bool) *Report {
	report := &Report{
		TargetRoot:  rcpt.Context.TargetRoot,
		SourceRoot:  rcpt.Context.SourceRoot,
		Projects:    rcpt.Context.Projects,
		FromReceipt: true,
		ReceiptPath: receiptPath,
	}

	for _, n := range rcpt.Nodes {
		if n.Status == "skipped" || n.Operation == "delegate" || n.Operation == "backup" {
			continue
		}

		var entry Entry
		ops := []string{n.Operation}
		isCopied := n.Operation != "link"
		if checkDrift && isCopied && n.SourceChecksum != "" {
			entry = checkEntryWithDrift(n.Source, n.Target, n.ID, n.Project, ops, n.SourceChecksum, n.TargetChecksum)
		} else {
			entry = checkEntry(n.Source, n.Target, n.ID, n.Project, ops)
		}
		report.Entries = append(report.Entries, entry)
	}

	return report
}

// FromBuildResult generates status by checking entries in a build result.
func FromBuildResult(br *tree.BuildResult) *Report {
	report := &Report{
		TargetRoot:  br.TargetRoot,
		SourceRoot:  br.SourceRoot,
		Projects:    br.Projects,
		FromReceipt: false,
	}

	for _, node := range br.Graph.Nodes {
		if nodeIsDelegate(node.Operations) {
			continue // Skip delegate nodes
		}
		entry := checkEntry(node.Source, node.Target, node.ID, node.Project, node.Operations)
		report.Entries = append(report.Entries, entry)
	}

	return report
}

// checkEntry checks the status of a single file.
func checkEntry(source, target, relTarget, project string, operations []string) Entry {
	entry := Entry{
		RelTarget:  relTarget,
		Source:     source,
		Target:     target,
		Project:    project,
		Operations: operations,
	}

	// Check if target exists
	targetInfo, err := os.Lstat(target)
	if os.IsNotExist(err) {
		// Target doesn't exist
		if _, srcErr := os.Stat(source); srcErr == nil {
			entry.State = StateMissing
			entry.Message = "symlink not created"
		} else {
			entry.State = StateOrphan
			entry.Message = "source file deleted"
		}
		return entry
	}
	if err != nil {
		entry.State = StateConflict
		entry.Message = err.Error()
		return entry
	}

	// Determine expected operation type
	isLink := len(operations) == 1 && operations[0] == "link"

	if isLink {
		// Should be a symlink
		if targetInfo.Mode()&os.ModeSymlink == 0 {
			entry.State = StateConflict
			entry.Message = "file exists, not a symlink"
			return entry
		}

		// Check symlink target
		linkTarget, err := os.Readlink(target)
		if err != nil {
			entry.State = StateConflict
			entry.Message = "cannot read symlink"
			return entry
		}

		// Resolve relative symlinks
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(target), linkTarget)
		}
		linkTarget = filepath.Clean(linkTarget)

		// Check if symlink points to our source
		if linkTarget == source {
			// Verify the source actually exists
			if _, err := os.Stat(source); os.IsNotExist(err) {
				entry.State = StateOrphan
				entry.Message = "source file deleted"
				return entry
			}
			entry.State = StateLinked
			return entry
		}

		// Symlink points elsewhere - check if it's a dangling symlink
		if _, err := os.Stat(linkTarget); os.IsNotExist(err) {
			entry.State = StateOrphan
			entry.Message = "symlink points to nonexistent file"
			return entry
		}

		entry.State = StateConflict
		entry.Message = "symlink points to " + linkTarget
		return entry
	}

	// Should be a copied file (template, secret, etc.)
	if targetInfo.Mode()&os.ModeSymlink != 0 {
		entry.State = StateConflict
		entry.Message = "expected file, found symlink"
		return entry
	}

	// File exists, was copied
	entry.State = StateCopied
	return entry
}

// checkEntryWithDrift checks status of a copied file using checksums.
func checkEntryWithDrift(source, target, relTarget, project string, operations []string, expectedSourceChecksum, expectedTargetChecksum string) Entry {
	entry := Entry{
		RelTarget:      relTarget,
		Source:         source,
		Target:         target,
		Project:        project,
		Operations:     operations,
		SourceChecksum: expectedSourceChecksum,
		TargetChecksum: expectedTargetChecksum,
	}

	// Check if target exists
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if _, srcErr := os.Stat(source); srcErr == nil {
			entry.State = StateMissing
			entry.Message = "file not deployed"
		} else {
			entry.State = StateOrphan
			entry.Message = "source file deleted"
		}
		return entry
	}

	// Compute current checksums
	currentSourceChecksum := receipt.ChecksumFile(source)
	currentTargetChecksum := receipt.ChecksumFile(target)

	sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != expectedSourceChecksum
	targetChanged := currentTargetChecksum != "" && currentTargetChecksum != expectedTargetChecksum

	switch {
	case sourceChanged && targetChanged:
		entry.State = StateDriftConflict
		entry.Message = "both source and target changed"
	case sourceChanged:
		entry.State = StateStale
		entry.Message = "source changed, redeploy needed"
	case targetChanged:
		entry.State = StateModified
		entry.Message = "target modified locally"
	default:
		entry.State = StateCopied
	}

	return entry
}

// ScanTarget scans the target directory for writ-managed symlinks.
// This works without a receipt by looking for symlinks that point into sourceRoot.
func ScanTarget(targetRoot, sourceRoot string) *Report {
	report := &Report{
		TargetRoot:  targetRoot,
		SourceRoot:  sourceRoot,
		FromReceipt: false,
	}

	projectSet := make(map[string]bool)

	// Walk the target directory looking for symlinks
	_ = filepath.Walk(targetRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories (except at top level)
		if info.IsDir() {
			base := filepath.Base(path)
			if path != targetRoot && strings.HasPrefix(base, ".") && base != ".config" && base != ".local" {
				// Skip most hidden dirs, but walk .config and .local
				return filepath.SkipDir
			}
			return nil
		}

		// Only check symlinks
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		// Check where symlink points
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil
		}

		// Resolve relative symlinks
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
		}
		linkTarget = filepath.Clean(linkTarget)

		// Check if it points into our source root
		if !strings.HasPrefix(linkTarget, sourceRoot) {
			return nil // Not our symlink
		}

		// Extract project from path
		relSource := strings.TrimPrefix(linkTarget, sourceRoot+"/")
		parts := strings.SplitN(relSource, "/", 3) // Home/project/...
		project := ""
		if len(parts) >= 2 {
			project = strings.Split(parts[1], ".")[0] // Strip suffix like .Darwin
			projectSet[project] = true
		}

		relTarget, _ := filepath.Rel(targetRoot, path)

		entry := Entry{
			RelTarget:  relTarget,
			Source:     linkTarget,
			Target:     path,
			Project:    project,
			Operations: []string{"link"},
		}

		// Check if source exists
		if _, err := os.Stat(linkTarget); os.IsNotExist(err) {
			entry.State = StateOrphan
			entry.Message = "source file deleted"
		} else {
			entry.State = StateLinked
		}

		report.Entries = append(report.Entries, entry)
		return nil
	})

	// Convert project set to slice
	for p := range projectSet {
		report.Projects = append(report.Projects, p)
	}

	return report
}
