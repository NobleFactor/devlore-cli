// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// RecoveryEntry represents a completed phase and its compensating action.
// The executor pushes entries as phases complete successfully, and pops
// them in LIFO order during rollback.
type RecoveryEntry struct {
	// PhaseID is the unique identifier of the completed phase.
	PhaseID string

	// PhaseName is the human-readable phase name.
	PhaseName string

	// Compensate is the function to execute for rollback.
	// It receives the execution context and returns any error.
	Compensate func(ctx *Context) error

	// State holds the execution metadata captured during the forward action.
	// Passed to the compensating action so it knows what to undo.
	State map[string]any
}

// RecoveryStack is a LIFO stack of recovery entries.
// The executor pushes entries as phases complete and unwinds
// (pops + executes compensating actions) on failure.
type RecoveryStack struct {
	entries []RecoveryEntry
}

// Push adds a recovery entry to the top of the stack.
func (s *RecoveryStack) Push(entry RecoveryEntry) {
	s.entries = append(s.entries, entry)
}

// Len returns the number of entries on the stack.
func (s *RecoveryStack) Len() int {
	return len(s.entries)
}

// Unwind executes all compensating actions in LIFO order.
// Returns a slice of errors from compensating actions that failed.
// Compensating action failures do not stop the unwind — all entries
// are processed regardless of individual failures.
func (s *RecoveryStack) Unwind(ctx *Context) []error {
	var errs []error

	// Pop from top (most recent) to bottom (oldest)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.Compensate == nil {
			continue
		}
		if err := entry.Compensate(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	// Clear the stack
	s.entries = nil

	return errs
}

// Entries returns a copy of the stack entries (bottom to top).
// Used for inspection and serialization.
func (s *RecoveryStack) Entries() []RecoveryEntry {
	result := make([]RecoveryEntry, len(s.entries))
	copy(result, s.entries)
	return result
}
