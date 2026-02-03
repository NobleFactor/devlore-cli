// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package reconcile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckEntry_LinkedCorrectly(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create source file
	sourceFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(sourceFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink pointing to source
	targetFile := filepath.Join(targetDir, "test.txt")
	if err := os.Symlink(sourceFile, targetFile); err != nil {
		t.Fatal(err)
	}

	// Check entry
	entry := checkEntry(sourceFile, targetFile, "test.txt", "testproject", []string{"link"})

	if entry.State != StateLinked {
		t.Errorf("expected StateLinked, got %s (%s)", entry.State.Label(), entry.Message)
	}
}

func TestCheckEntry_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create source file but no symlink
	sourceFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(sourceFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	targetFile := filepath.Join(targetDir, "test.txt")

	// Check entry - target doesn't exist
	entry := checkEntry(sourceFile, targetFile, "test.txt", "testproject", []string{"link"})

	if entry.State != StateMissing {
		t.Errorf("expected StateMissing, got %s (%s)", entry.State.Label(), entry.Message)
	}
}

func TestCheckEntry_Conflict(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create source file
	sourceFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(sourceFile, []byte("source content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create regular file at target (not a symlink)
	targetFile := filepath.Join(targetDir, "test.txt")
	if err := os.WriteFile(targetFile, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Check entry - should be conflict
	entry := checkEntry(sourceFile, targetFile, "test.txt", "testproject", []string{"link"})

	if entry.State != StateConflict {
		t.Errorf("expected StateConflict, got %s (%s)", entry.State.Label(), entry.Message)
	}
}

func TestCheckEntry_Orphan(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink pointing to nonexistent file
	nonexistent := filepath.Join(tmpDir, "nonexistent.txt")
	targetFile := filepath.Join(targetDir, "test.txt")
	if err := os.Symlink(nonexistent, targetFile); err != nil {
		t.Fatal(err)
	}

	// Source also doesn't exist
	entry := checkEntry(nonexistent, targetFile, "test.txt", "testproject", []string{"link"})

	if entry.State != StateOrphan {
		t.Errorf("expected StateOrphan, got %s (%s)", entry.State.Label(), entry.Message)
	}
}

func TestCheckEntry_CopiedFile(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create source template
	sourceFile := filepath.Join(sourceDir, "config.template")
	if err := os.WriteFile(sourceFile, []byte("template content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create target file (copied, not symlinked)
	targetFile := filepath.Join(targetDir, "config")
	if err := os.WriteFile(targetFile, []byte("expanded content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Check entry with copy operation
	entry := checkEntry(sourceFile, targetFile, "config", "testproject", []string{"render", "copy"})

	if entry.State != StateCopied {
		t.Errorf("expected StateCopied, got %s (%s)", entry.State.Label(), entry.Message)
	}
}

func TestReportSummary(t *testing.T) {
	report := &Report{
		Entries: []Entry{
			{State: StateLinked},
			{State: StateLinked},
			{State: StateCopied},
			{State: StateConflict},
			{State: StateMissing},
		},
	}

	summary := report.Summary()

	if summary[StateLinked] != 2 {
		t.Errorf("expected 2 linked, got %d", summary[StateLinked])
	}
	if summary[StateCopied] != 1 {
		t.Errorf("expected 1 copied, got %d", summary[StateCopied])
	}
	if summary[StateConflict] != 1 {
		t.Errorf("expected 1 conflict, got %d", summary[StateConflict])
	}
	if summary[StateMissing] != 1 {
		t.Errorf("expected 1 missing, got %d", summary[StateMissing])
	}
}

func TestReportHasIssues(t *testing.T) {
	// Report with only linked/copied = no issues
	reportOK := &Report{
		Entries: []Entry{
			{State: StateLinked},
			{State: StateCopied},
		},
	}
	if reportOK.HasIssues() {
		t.Error("expected no issues for linked/copied entries")
	}

	// Report with conflict = has issues
	reportBad := &Report{
		Entries: []Entry{
			{State: StateLinked},
			{State: StateConflict},
		},
	}
	if !reportBad.HasIssues() {
		t.Error("expected issues for report with conflict")
	}
}
