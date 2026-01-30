// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package console provides an interactive terminal UI for guided workflows.
// It uses Bubble Tea for the TUI framework and supports session-based
// state machines that present information and collect user responses.
package console

// Session defines the interface for interactive workflows.
// A Session is a state machine that presents content and collects responses.
type Session interface {
	// Next advances the session and returns the next step to display.
	// Returns nil when the session is complete.
	Next() *Step

	// Respond processes the user's response to the current step.
	// The response format depends on the step type:
	//   - Confirm: "yes" or "no"
	//   - Select: index of selected option (as string)
	//   - Input: the text entered
	Respond(response string) error

	// Current returns the current step, or nil if not started.
	Current() *Step

	// Complete returns true if the session has finished.
	Complete() bool

	// Result returns the session's final result after completion.
	// The type depends on the session implementation.
	Result() any

	// Error returns any error that terminated the session.
	Error() error
}

// StepType identifies the kind of user interaction expected.
type StepType int

const (
	// StepInfo displays information without requiring a response.
	StepInfo StepType = iota

	// StepConfirm asks for yes/no confirmation.
	StepConfirm

	// StepSelect presents options for the user to choose from.
	StepSelect

	// StepInput requests free-form text input.
	StepInput

	// StepProgress shows progress during a long-running operation.
	StepProgress

	// StepComplete signals the session has finished successfully.
	StepComplete

	// StepError signals the session terminated with an error.
	StepError
)

// Step represents a single interaction in the session.
type Step struct {
	// Type determines how the step is rendered and what input is expected.
	Type StepType

	// Title is a short heading for the step.
	Title string

	// Content is the main body, rendered as markdown.
	Content string

	// Options are the choices for StepSelect.
	Options []Option

	// Default is the default value for StepInput or StepConfirm.
	Default string

	// Progress is the completion percentage (0-100) for StepProgress.
	Progress int

	// Error contains the error for StepError.
	Error error
}

// Option represents a selectable choice.
type Option struct {
	// Label is the display text.
	Label string

	// Description provides additional context.
	Description string

	// Value is the underlying value returned when selected.
	Value string
}
