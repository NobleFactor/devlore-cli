// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		name string
		s    State
		want string
	}{
		{"linked", StateLinked, "✓ Linked"},
		{"copied", StateCopied, "✓ Copied"},
		{"conflict", StateConflict, "⚠ Conflict"},
		{"missing", StateMissing, "✗ Missing"},
		{"orphan", StateOrphan, "? Orphan"},
		{"stale", StateStale, "↑ Stale"},
		{"modified", StateModified, "M Modified"},
		{"drift_conflict", StateDriftConflict, "! Conflict"},
		{"unknown", State(99), "? Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestStateLabel(t *testing.T) {
	tests := []struct {
		name string
		s    State
		want string
	}{
		{"linked", StateLinked, "linked"},
		{"copied", StateCopied, "copied"},
		{"conflict", StateConflict, "conflict"},
		{"missing", StateMissing, "missing"},
		{"orphan", StateOrphan, "orphan"},
		{"stale", StateStale, "stale"},
		{"modified", StateModified, "modified"},
		{"drift_conflict", StateDriftConflict, "drift_conflict"},
		{"unknown", State(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Label(); got != tt.want {
				t.Errorf("State(%d).Label() = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestReportSummary(t *testing.T) {
	r := &Report{
		Entries: []Entry{
			{State: StateLinked},
			{State: StateLinked},
			{State: StateMissing},
			{State: StateConflict},
			{State: StateLinked},
			{State: StateCopied},
			{State: StateMissing},
		},
	}

	got := r.Summary()
	want := map[State]int{
		StateLinked:   3,
		StateMissing:  2,
		StateConflict: 1,
		StateCopied:   1,
	}

	for state, count := range want {
		if got[state] != count {
			t.Errorf("Summary()[%s] = %d, want %d", state.Label(), got[state], count)
		}
	}

	if len(got) != len(want) {
		t.Errorf("Summary() has %d states, want %d", len(got), len(want))
	}
}

func TestReportSummaryEmpty(t *testing.T) {
	r := &Report{}

	got := r.Summary()
	if len(got) != 0 {
		t.Errorf("Summary() on empty report = %v, want empty map", got)
	}
}

func TestFromBuildResult(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up source files for the symlink to point at.
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sourceFile := filepath.Join(sourceDir, "config.sh")
	if err := os.WriteFile(sourceFile, []byte("#!/bin/bash"), 0o644); err != nil {
		t.Fatal(err)
	}

	wrongSource := filepath.Join(sourceDir, "other.sh")
	if err := os.WriteFile(wrongSource, []byte("#!/bin/zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Target: correct symlink
	correctLink := filepath.Join(targetDir, "correct-link")
	if err := os.Symlink(sourceFile, correctLink); err != nil {
		t.Fatal(err)
	}

	// Target: wrong symlink (points to wrong source)
	wrongLink := filepath.Join(targetDir, "wrong-link")
	if err := os.Symlink(wrongSource, wrongLink); err != nil {
		t.Fatal(err)
	}

	// Target: regular file (copied)
	copiedFile := filepath.Join(targetDir, "copied-file")
	if err := os.WriteFile(copiedFile, []byte("copied content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Target: missing file (no file created at this path)
	missingFile := filepath.Join(targetDir, "missing-file")

	result := &tree.BuildResult{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Files: []*tree.FileEntry{
			{ID: "correct-link", Source: sourceFile, Target: correctLink, Project: "devtools"},
			{ID: "wrong-link", Source: sourceFile, Target: wrongLink, Project: "devtools"},
			{ID: "copied-file", Source: sourceFile, Target: copiedFile, Project: "devtools"},
			{ID: "missing-file", Source: sourceFile, Target: missingFile, Project: "devtools"},
		},
	}

	report := FromBuildResult(result)

	if report.TargetRoot != targetDir {
		t.Errorf("TargetRoot = %q, want %q", report.TargetRoot, targetDir)
	}
	if report.SourceRoot != sourceDir {
		t.Errorf("SourceRoot = %q, want %q", report.SourceRoot, sourceDir)
	}

	if len(report.Entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(report.Entries))
	}

	wantStates := []struct {
		id    string
		state State
	}{
		{"correct-link", StateLinked},
		{"wrong-link", StateConflict},
		{"copied-file", StateCopied},
		{"missing-file", StateMissing},
	}

	for i, ws := range wantStates {
		entry := report.Entries[i]
		if entry.RelTarget != ws.id {
			t.Errorf("entry[%d].RelTarget = %q, want %q", i, entry.RelTarget, ws.id)
		}
		if entry.State != ws.state {
			t.Errorf("entry[%d] %q: State = %s, want %s", i, ws.id, entry.State, ws.state)
		}
	}

	// Verify project extraction
	if len(report.Projects) != 1 || report.Projects[0] != "devtools" {
		t.Errorf("Projects = %v, want [devtools]", report.Projects)
	}
}

func TestFromBuildResultEmpty(t *testing.T) {
	result := &tree.BuildResult{
		SourceRoot: "/nonexistent/source",
		TargetRoot: "/nonexistent/target",
	}

	report := FromBuildResult(result)

	if len(report.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(report.Entries))
	}
	if len(report.Projects) != 0 {
		t.Errorf("got %d projects, want 0", len(report.Projects))
	}
}

func TestScanTarget(t *testing.T) {
	tmpDir := t.TempDir()

	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Existing source file
	sourceFile := filepath.Join(sourceDir, "config.sh")
	if err := os.WriteFile(sourceFile, []byte("#!/bin/bash"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Removed source file (create then delete so the symlink is dangling)
	removedSource := filepath.Join(sourceDir, "removed.sh")
	if err := os.WriteFile(removedSource, []byte("gone"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Symlink pointing to existing source → StateLinked
	linkedTarget := filepath.Join(targetDir, "config.sh")
	if err := os.Symlink(sourceFile, linkedTarget); err != nil {
		t.Fatal(err)
	}

	// Symlink pointing to removed source → StateOrphan
	orphanTarget := filepath.Join(targetDir, "removed.sh")
	if err := os.Symlink(removedSource, orphanTarget); err != nil {
		t.Fatal(err)
	}

	// Now remove the source file to make the symlink orphaned
	if err := os.Remove(removedSource); err != nil {
		t.Fatal(err)
	}

	report := ScanTarget(targetDir, sourceDir)

	if report.TargetRoot != targetDir {
		t.Errorf("TargetRoot = %q, want %q", report.TargetRoot, targetDir)
	}
	if report.SourceRoot != sourceDir {
		t.Errorf("SourceRoot = %q, want %q", report.SourceRoot, sourceDir)
	}

	if len(report.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(report.Entries))
	}

	// Build a map for reliable lookup regardless of walk order
	entryByRel := make(map[string]Entry)
	for _, e := range report.Entries {
		entryByRel[e.RelTarget] = e
	}

	linked, ok := entryByRel["config.sh"]
	if !ok {
		t.Fatal("missing entry for config.sh")
	}
	if linked.State != StateLinked {
		t.Errorf("config.sh: State = %s, want %s", linked.State, StateLinked)
	}

	orphan, ok := entryByRel["removed.sh"]
	if !ok {
		t.Fatal("missing entry for removed.sh")
	}
	if orphan.State != StateOrphan {
		t.Errorf("removed.sh: State = %s, want %s", orphan.State, StateOrphan)
	}
	if orphan.Message != "source file deleted" {
		t.Errorf("removed.sh: Message = %q, want %q", orphan.Message, "source file deleted")
	}
}

func TestScanTargetEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	targetDir := filepath.Join(tmpDir, "target")
	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	report := ScanTarget(targetDir, sourceDir)

	if len(report.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(report.Entries))
	}
}

func TestScanTargetNonSymlinksSkipped(t *testing.T) {
	tmpDir := t.TempDir()

	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create regular files in target — these should be skipped
	if err := os.WriteFile(filepath.Join(targetDir, "regular.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory — should also be skipped
	if err := os.MkdirAll(filepath.Join(targetDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "subdir", "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := ScanTarget(targetDir, sourceDir)

	if len(report.Entries) != 0 {
		t.Errorf("got %d entries, want 0 (non-symlinks should be skipped)", len(report.Entries))
	}
}
