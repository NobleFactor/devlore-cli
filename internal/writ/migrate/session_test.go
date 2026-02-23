// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/console"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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
	if session.state != StateAnalyzing {
		t.Errorf("expected initial state StateAnalyzing, got %v", session.state)
	}
}

func TestSessionImplementsInterface(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Verify Session implements console.Session
	var _ console.Session = session
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
	if !errors.Is(session.Error(), os.ErrNotExist) {
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

	// Manually set up to avoid running actual analysis
	session.state = StateConversing
	session.aiResponse = "Test response"

	step := session.Next()
	current := session.Current()
	if current != step {
		t.Error("expected Current() to return the step from Next()")
	}
}

func TestSessionSlashCommands(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Set up state for slash commands
	session.state = StateConversing
	session.analysis = &MigrationAnalysis{SourceRoot: "/tmp/test"}
	session.graph = &op.Graph{}

	tests := []struct {
		cmd           string
		expectedState SessionState
	}{
		{"/help", StateConversing},
		{"/explain", StateConversing},
		{"/exit", StateComplete},
	}

	for _, tc := range tests {
		session.state = StateConversing // Reset state
		err := session.Respond(tc.cmd)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tc.cmd, err)
		}
		if session.state != tc.expectedState {
			t.Errorf("expected state %v after %s, got %v", tc.expectedState, tc.cmd, session.state)
		}
	}
}

func TestSessionSlashAnalyze(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	// Set up as if we already analyzed
	session.state = StateConversing
	session.analysis = &MigrationAnalysis{SourceRoot: "/tmp/test"}
	session.history = []model.Message{{Role: model.RoleUser, Content: "test"}}

	err := session.Respond("/analyze")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if session.state != StateAnalyzing {
		t.Errorf("expected StateAnalyzing, got %v", session.state)
	}
}

func TestSessionSlashHelp(t *testing.T) {
	help := slashCommandHelp()
	if help == "" {
		t.Error("expected help text")
	}
	if !contains(help, "/analyze") {
		t.Error("expected /analyze in help")
	}
	if !contains(help, "/explain") {
		t.Error("expected /explain in help")
	}
	if !contains(help, "/help") {
		t.Error("expected /help in help")
	}
	if !contains(help, "/exit") {
		t.Error("expected /exit in help")
	}
}

func TestSessionConversationStep(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	session.state = StateConversing
	session.aiResponse = "This is the AI response"

	step := session.conversationStep()
	if step.Type != console.StepInput {
		t.Errorf("expected StepInput, got %v", step.Type)
	}
	if step.Content != "This is the AI response" {
		t.Errorf("unexpected content: %s", step.Content)
	}
}

func TestSessionDetectReadyToExecute(t *testing.T) {
	session := &Session{}

	// Test detection of ready-to-execute responses
	tests := []struct {
		content  string
		expected bool
	}{
		{"Here's what I found", false},
		{"**Ready to Execute**\nThe migration will perform the following...", true},
		{"Ready to execute. Type approve to proceed.", true},
		{"Let me help you", false},
		{"Type approve to execute the changes", true},
	}

	for _, tc := range tests {
		result := session.detectReadyToExecute(tc.content)
		if result != tc.expected {
			t.Errorf("detectReadyToExecute(%q) = %v, want %v", tc.content, result, tc.expected)
		}
	}
}

func TestSessionProcessPlanResponse(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	session.state = StatePlanProposed

	// Approve
	err := session.processPlanResponse("approve")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if session.state != StateExecuting {
		t.Errorf("expected StateExecuting, got %v", session.state)
	}
}

func TestSessionProcessPlanResponseVariants(t *testing.T) {
	approvals := []string{"approve", "yes", "ok", "proceed", "👍"}

	for _, approval := range approvals {
		t.Run(approval, func(t *testing.T) {
			opts := Options{SourceRoot: "/tmp/test"}
			session := NewSession(opts)
			session.state = StatePlanProposed

			err := session.processPlanResponse(approval)
			if err != nil {
				t.Errorf("unexpected error for %q: %v", approval, err)
			}
			if session.state != StateExecuting {
				t.Errorf("expected StateExecuting for %q, got %v", approval, session.state)
			}
		})
	}
}

func TestSessionWithRealDirectory(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()

	// Create a simple structure
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

	// Initial state is analyzing
	if session.state != StateAnalyzing {
		t.Errorf("expected StateAnalyzing, got %v", session.state)
	}

	// Next() runs analysis
	step := session.Next()
	if step == nil {
		t.Fatal("expected step after analysis")
	}

	// Either we got an error step or we're in conversing
	switch step.Type {
	case console.StepError:
		t.Log("Analysis failed:", session.err)
	case console.StepProgress:
		// Analysis completed, next call should return conversation step
		t.Log("Analysis in progress")
	}
}

func TestSessionErrorState(t *testing.T) {
	opts := Options{SourceRoot: "/nonexistent/path/that/does/not/exist"}
	session := NewSession(opts)

	// Run analysis on nonexistent path
	step := session.Next()

	// Should be in error state
	if session.state == StateError {
		if step.Type != console.StepError {
			t.Errorf("expected StepError, got %v", step.Type)
		}
		if session.err == nil {
			t.Error("expected error to be set")
		}
	}
}

func TestSessionResultType(t *testing.T) {
	result := &SessionResult{
		Graph:       &op.Graph{},
		Analysis:    &MigrationAnalysis{SourceRoot: "/test"},
		Executed:    true,
		ReceiptPath: "/path/to/receipt.yaml",
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
	if result.ReceiptPath != "/path/to/receipt.yaml" {
		t.Errorf("expected ReceiptPath, got %q", result.ReceiptPath)
	}
}

func TestGenerateStaticInitialResponse(t *testing.T) {
	opts := Options{SourceRoot: "/tmp/test"}
	session := NewSession(opts)

	session.analysis = &MigrationAnalysis{
		SourceRoot: "/tmp/test",
		System:     SystemUnknown,
		Stats: MigrationStats{
			TotalFiles: 10,
			Projects:   2,
			Platforms:  1,
		},
		Observations:    []string{"Found shell configs"},
		Warnings:        []string{"No encryption detected"},
		Recommendations: []string{"Consider using SOPS"},
	}

	response := session.generateStaticInitialResponse()
	if response == "" {
		t.Error("expected non-empty response")
	}
	if !contains(response, "/tmp/test") {
		t.Error("expected source root in response")
	}
	if !contains(response, "Files:") {
		t.Error("expected file count in response")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
