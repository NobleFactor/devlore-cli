// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package console

import (
	"testing"
)

// mockSession implements Session for testing.
type mockSession struct {
	steps    []*Step
	index    int
	response string
	result   any
	err      error
}

func (m *mockSession) Next() *Step {
	if m.index >= len(m.steps) {
		return nil
	}
	step := m.steps[m.index]
	m.index++
	return step
}

func (m *mockSession) Respond(response string) error {
	m.response = response
	return nil
}

func (m *mockSession) Current() *Step {
	if m.index == 0 || m.index > len(m.steps) {
		return nil
	}
	return m.steps[m.index-1]
}

func (m *mockSession) Complete() bool {
	return m.index >= len(m.steps)
}

func (m *mockSession) Result() any {
	return m.result
}

func (m *mockSession) Error() error {
	return m.err
}

func TestStepTypes(t *testing.T) {
	tests := []struct {
		stepType StepType
		name     string
	}{
		{StepInfo, "info"},
		{StepConfirm, "confirm"},
		{StepSelect, "select"},
		{StepInput, "input"},
		{StepProgress, "progress"},
		{StepComplete, "complete"},
		{StepError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &Step{Type: tt.stepType, Title: tt.name}
			if step.Type != tt.stepType {
				t.Errorf("expected step type %v, got %v", tt.stepType, step.Type)
			}
		})
	}
}

func TestOption(t *testing.T) {
	opt := Option{
		Label:       "Test Option",
		Description: "A test option",
		Value:       "test",
	}

	if opt.Label != "Test Option" {
		t.Errorf("expected label 'Test Option', got %q", opt.Label)
	}
	if opt.Value != "test" {
		t.Errorf("expected value 'test', got %q", opt.Value)
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Primary == "" {
		t.Error("expected primary color to be set")
	}
	if theme.Error == "" {
		t.Error("expected error color to be set")
	}
	if theme.Success == "" {
		t.Error("expected success color to be set")
	}
}

func TestDefaultStyles(t *testing.T) {
	styles := DefaultStyles()

	if styles == nil {
		t.Fatal("expected styles to be non-nil")
	}

	// Verify key styles are initialized
	_ = styles.Title.Render("test")
	_ = styles.Error.Render("test")
	_ = styles.Success.Render("test")
}

func TestNewConsole(t *testing.T) {
	con := New()
	if con == nil {
		t.Fatal("expected console to be non-nil")
	}
	if con.styles == nil {
		t.Error("expected styles to be initialized")
	}
}

func TestConsoleHelpers(t *testing.T) {
	con := New()

	// These should not panic
	con.Success("test success")
	con.Warning("test warning")
	con.Error("test error")
	con.Info("test info")
}

func TestMockSession(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Step 1"},
			{Type: StepConfirm, Title: "Step 2"},
			{Type: StepComplete, Title: "Done"},
		},
		result: "test result",
	}

	// Initially not complete
	if session.Complete() {
		t.Error("session should not be complete initially")
	}

	// Advance through steps
	step1 := session.Next()
	if step1.Title != "Step 1" {
		t.Errorf("expected 'Step 1', got %q", step1.Title)
	}

	step2 := session.Next()
	if step2.Title != "Step 2" {
		t.Errorf("expected 'Step 2', got %q", step2.Title)
	}

	// Respond to step
	if err := session.Respond("yes"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if session.response != "yes" {
		t.Errorf("expected response 'yes', got %q", session.response)
	}

	step3 := session.Next()
	if step3.Title != "Done" {
		t.Errorf("expected 'Done', got %q", step3.Title)
	}

	// Now complete
	if !session.Complete() {
		t.Error("session should be complete")
	}

	// Result available
	if session.Result() != "test result" {
		t.Errorf("expected 'test result', got %v", session.Result())
	}
}

func TestNewModel(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test", Content: "Hello"},
		},
	}

	model := NewModel(session)
	if model == nil {
		t.Fatal("expected model to be non-nil")
	}
	if model.session != session {
		t.Error("expected session to be set")
	}
	if model.styles == nil {
		t.Error("expected styles to be initialized")
	}
}

func TestModelInit(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test", Content: "Hello"},
		},
	}

	model := NewModel(session)
	cmd := model.Init()

	// Should have a command (batch of render + input init)
	if cmd == nil {
		t.Error("expected init to return a command")
	}

	// Step should be set
	if model.step == nil {
		t.Error("expected step to be set after Init")
	}
}

func TestModelView(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test Title", Content: "Test content"},
		},
	}

	model := NewModel(session)
	_ = model.Init()

	view := model.View()
	if view == "" {
		t.Error("expected view to have content")
	}
}
