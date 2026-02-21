// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"errors"
	"fmt"
)

// RecoveryEntry represents a successfully executed undoable node and the
// state needed to undo it. The executor pushes one entry per completed
// node that implements CompensableAction.
type RecoveryEntry struct {
	// Node is the executed node (carries the CompensableAction action for Undo).
	Node *Node

	// UndoState is the state captured by Do, passed to Undo during rollback.
	UndoState UndoState
}

// RecoveryStack is a LIFO stack of recovery entries.
// The executor pushes entries as nodes complete and unwinds
// (pops + executes Undo) on failure.
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

// Unwind executes Undo on all entries in LIFO order.
// Each entry's node slots are resolved from the results map before calling Undo.
// Undo failures do not stop the unwind — all entries are processed.
// Non-compensable actions (NotCompensableError) are logged and skipped.
func (s *RecoveryStack) Unwind(ctx *Context, results map[string]any, proxyCtx ...map[string]any) []error {
	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		undoable, ok := entry.Node.Action.(CompensableAction)
		if !ok {
			continue
		}
		slots := entry.Node.ResolvedSlots(results, proxyCtx...)
		if err := undoable.Undo(ctx, slots, entry.UndoState); err != nil {
			if errors.Is(err, NotCompensableError) {
				if ctx.Writer != nil {
					fmt.Fprintf(ctx.Writer, "  [warn] %s: not compensable, skipping\n", undoable.Name())
				}
				continue
			}
			errs = append(errs, err)
		}
	}

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
