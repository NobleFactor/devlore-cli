// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "errors"

// errDrifted indicates that a reconcile check detected external modification,
// making compensation unsafe. The entry is skipped during Unwind.
var errDrifted = errors.New("state has drifted: compensation unsafe")

// recoveryEntry captures a single compensable operation with its undo and reconcile state.
type recoveryEntry struct {
	compensate     func(any) error
	reconcile      func(any) (bool, error)
	undoState      any
	reconcileState any
}

// RecoveryStack accumulates compensable operations in LIFO order. On Unwind, each entry is
// reconciled (if a reconcile function was provided) and then compensated in reverse order.
type RecoveryStack struct {
	entries []recoveryEntry
}

// NewRecoveryStack creates an empty RecoveryStack.
func NewRecoveryStack() *RecoveryStack {
	return &RecoveryStack{}
}

// Do invokes a closure and, if it succeeds, pushes a recovery entry onto the stack.
// If invoke returns an error, the stack is unchanged and the error is returned without unwinding.
func (s *RecoveryStack) Do(
	invoke func() (undoState any, reconcileState any, err error),
	compensate func(any) error,
	reconcile func(any) (bool, error),
) error {
	undoState, reconcileState, err := invoke()
	if err != nil {
		return err
	}
	s.Push(compensate, reconcile, undoState, reconcileState)
	return nil
}

// PushAction pushes a recovery entry onto the stack when complement is non-nil.
// A nil complement signals that the action produced no undo state, so there is
// nothing to compensate and the call is a no-op.
func (s *RecoveryStack) PushAction(ctx *Context, action Action, complement any) {
	if complement == nil {
		return
	}
	type undoer interface {
		Undo(ctx *Context, complement Complement) error
	}
	u, ok := action.(undoer)
	if !ok {
		return
	}
	s.Push(
		func(state any) error {
			err := u.Undo(ctx, state)
			if errors.Is(err, ErrNotCompensable) {
				return nil
			}
			return err
		},
		nil, complement, nil,
	)
}

// Push manually adds a recovery entry to the stack.
func (s *RecoveryStack) Push(
	compensate func(any) error,
	reconcile func(any) (bool, error),
	undoState any,
	reconcileState any,
) {
	s.entries = append(s.entries, recoveryEntry{
		compensate:     compensate,
		reconcile:      reconcile,
		undoState:      undoState,
		reconcileState: reconcileState,
	})
}

// Unwind compensates all entries in LIFO order. For each entry:
//  1. If reconcile is non-nil, it is called first. If reconcile returns false (drifted),
//     the entry is skipped and an ErrDrifted is collected.
//  2. If reconcile is nil or returns true (safe), compensate is called.
//
// All entries are attempted regardless of individual failures. Errors are joined via errors.Join.
func (s *RecoveryStack) Unwind() error {
	var errs []error
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]

		if entry.reconcile != nil {
			safe, reconcileErr := entry.reconcile(entry.reconcileState)
			if reconcileErr != nil {
				errs = append(errs, reconcileErr)
				continue
			}
			if !safe {
				errs = append(errs, errDrifted)
				continue
			}
		}

		if entry.compensate != nil {
			if err := entry.compensate(entry.undoState); err != nil {
				errs = append(errs, err)
			}
		}
	}
	s.entries = nil
	return errors.Join(errs...)
}

// Discard drops all entries without unwinding.
func (s *RecoveryStack) Discard() {
	s.entries = nil
}

// Len returns the number of entries on the stack.
func (s *RecoveryStack) Len() int {
	return len(s.entries)
}
