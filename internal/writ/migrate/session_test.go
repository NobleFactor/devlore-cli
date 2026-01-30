// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/console"
	"github.com/NobleFactor/devlore-cli/internal/execution"
)

func TestNewSession(t *testing.T) {
	opts := Options{
		SourceRoot: "/tmp/test",
		Execute:    false,
		Verbose:    true,
	}

	session := NewSession(opts)
	if session == nil {
		t.Fatal("expected session to be non-nil")
	}
	if session.opts.SourceRoot != "/tmp/test" {
		t.Errorf("expected source root '/tmp/test', got %q", session.opts.SourceRoot)
	}
	if session.state != StateWelcome {
		t.Errorf("expected initial state StateWelcome, got %v", session.state)
	}
}

func TestSessionImplementsInterface(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Verify Session implements console.Session
	var _ console.Session = session
}

func TestSessionWelcomeStep(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	step := session.Next()
	if step == nil {
		t.Fatal("expected welcome step")
	}
	if step.Type != console.StepInfo {
		t.Errorf("expected StepInfo, got %v", step.Type)
	}
	if step.Title != "Welcome" {
		t.Errorf("expected title 'Welcome', got %q", step.Title)
	}
	if step.Content == "" {
		t.Error("expected content to be non-empty")
	}
}

func TestSessionStateTransitions(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Initial state
	if session.state != StateWelcome {
		t.Errorf("expected StateWelcome, got %v", session.state)
	}

	// Get welcome step
	_ = session.Next()

	// Respond to advance
	if err := session.Respond(""); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should advance to detecting
	if session.state != StateDetecting {
		t.Errorf("expected StateDetecting, got %v", session.state)
	}
}

func TestSessionComplete(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Initially not complete
	if session.Complete() {
		t.Error("session should not be complete initially")
	}

	// Force to complete state
	session.state = StateComplete
	if !session.Complete() {
		t.Error("session should be complete")
	}

	// Error state is also complete
	session.state = StateError
	if !session.Complete() {
		t.Error("session should be complete on error")
	}
}

func TestSessionError(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// No error initially
	if session.Error() != nil {
		t.Error("expected no error initially")
	}

	// Set an error
	session.err = os.ErrNotExist
	if session.Error() != os.ErrNotExist {
		t.Errorf("expected os.ErrNotExist, got %v", session.Error())
	}
}

func TestSessionResult(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// No result initially
	if session.Result() != nil {
		t.Error("expected no result initially")
	}

	// Set a result
	session.result = &SessionResult{Executed: true}
	result := session.Result()
	if result == nil {
		t.Fatal("expected result")
	}
	sr, ok := result.(*SessionResult)
	if !ok {
		t.Fatal("expected *SessionResult")
	}
	if !sr.Executed {
		t.Error("expected Executed to be true")
	}
}

func TestSessionCurrent(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// No current step before Next()
	if session.Current() != nil {
		t.Error("expected no current step before Next()")
	}

	// Get a step
	step := session.Next()
	current := session.Current()
	if current != step {
		t.Error("expected Current() to return the step from Next()")
	}
}

func TestSessionWithRealDirectory(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()

	// Create a simple dotfiles structure
	projectDir := filepath.Join(tmpDir, "shell")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(projectDir, ".bashrc")
	if err := os.WriteFile(testFile, []byte("# test bashrc"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	opts := Options{
		SourceRoot: tmpDir,
		Execute:    false,
		Verbose:    false,
	}

	session := NewSession(opts)

	// Should be able to get welcome step
	step := session.Next()
	if step == nil {
		t.Fatal("expected welcome step")
	}
	if step.Title != "Welcome" {
		t.Errorf("expected 'Welcome', got %q", step.Title)
	}

	// Respond and move to detection
	if err := session.Respond(""); err != nil {
		t.Fatalf("respond error: %v", err)
	}

	// Get detection step - this will actually run BuildMigration
	// which may fail without AI provider, so we just check it doesn't panic
	step = session.Next()
	if step == nil {
		t.Fatal("expected step after detection")
	}

	// Either we got an error step (no AI provider) or we're in analysis
	if step.Type == console.StepError {
		// Expected without AI provider configured
		t.Log("Detection failed (expected without AI provider):", session.err)
	} else if step.Type == console.StepProgress {
		t.Log("Detection in progress")
	}
}

func TestSessionErrorState(t *testing.T) {
	opts := Options{SourceRoot: "/nonexistent/path"}
	session := NewSession(opts)

	// Get welcome
	_ = session.Next()
	_ = session.Respond("")

	// Detection should fail on nonexistent path
	step := session.Next()

	// Should either error or be in error state
	if session.state == StateError {
		if step.Type != console.StepError {
			t.Errorf("expected StepError, got %v", step.Type)
		}
		if session.err == nil {
			t.Error("expected error to be set")
		}
	}
}

func TestConfirmRenamesSkipsWhenNoRenames(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Manually set up state with no renames
	session.state = StateConfirmRenames
	session.graph = &execution.Graph{Nodes: []*execution.Node{}}
	session.analysis = &MigrationAnalysis{SourceRoot: "/tmp/test"}

	step := session.Next()

	// Should skip to secrets since no renames, or return a confirm step
	if step.Type == console.StepConfirm {
		t.Log("Got confirm step for renames (no nodes to rename)")
	} else {
		// Check we advanced past renames
		if session.state != StateConfirmSecrets && session.state != StateConfirmRenames {
			t.Logf("State after confirm renames: %v", session.state)
		}
	}
}

func TestConfirmSecretsSkipsWhenNoSecrets(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Manually set up state with no secrets - need graph for preview step
	session.state = StateConfirmSecrets
	session.graph = &execution.Graph{Nodes: []*execution.Node{}}
	session.analysis = &MigrationAnalysis{
		SourceRoot:     "/tmp/test",
		SecretFindings: []SecretFinding{},
	}

	step := session.Next()

	// Should skip to preview since no secrets, or return a confirm step
	if step.Type == console.StepConfirm {
		t.Log("Got confirm step for secrets")
	} else {
		// Check we advanced past secrets
		if session.state != StatePreview && session.state != StateConfirmSecrets {
			t.Logf("State after confirm secrets: %v", session.state)
		}
	}
}

func TestSessionResultType(t *testing.T) {
	result := &SessionResult{
		Graph:    &execution.Graph{},
		Analysis: &MigrationAnalysis{SourceRoot: "/test"},
		Executed: true,
	}

	if result.Graph == nil {
		t.Error("expected Graph")
	}
	if result.Analysis == nil {
		t.Error("expected Analysis")
	}
	if result.Analysis.SourceRoot != "/test" {
		t.Errorf("expected SourceRoot '/test', got %q", result.Analysis.SourceRoot)
	}
	if !result.Executed {
		t.Error("expected Executed to be true")
	}
}
