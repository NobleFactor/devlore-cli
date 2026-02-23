// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"fmt"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testUndoAction implements CompensableAction with controllable Undo behavior.
type testUndoAction struct {
	name   string
	undoFn func(state UndoState) error
}

func (a *testUndoAction) Name() string { return a.name }
func (a *testUndoAction) Do(_ *Context, _ map[string]any) (Result, UndoState, error) {
	return nil, nil, nil
}
func (a *testUndoAction) Undo(state UndoState) error {
	if a.undoFn != nil {
		return a.undoFn(state)
	}
	return nil
}

func TestRecoveryStackPushLen(t *testing.T) {
	s := &RecoveryStack{}

	if s.Len() != 0 {
		t.Errorf("expected empty stack, got len %d", s.Len())
	}

	s.Push(RecoveryEntry{Node: &op.Node{ID: "prepare", Action: op.StubAction("test")}})
	s.Push(RecoveryEntry{Node: &op.Node{ID: "install", Action: op.StubAction("test")}})

	if s.Len() != 2 {
		t.Errorf("expected len 2, got %d", s.Len())
	}
}

func TestRecoveryStackUnwindLIFO(t *testing.T) {
	s := &RecoveryStack{}
	var order []string

	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "prepare", Action: &testUndoAction{
			name: "prepare",
			undoFn: func(_ UndoState) error {
				order = append(order, "prepare")
				return nil
			},
		}},
	})
	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "install", Action: &testUndoAction{
			name: "install",
			undoFn: func(_ UndoState) error {
				order = append(order, "install")
				return nil
			},
		}},
	})
	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "provision", Action: &testUndoAction{
			name: "provision",
			undoFn: func(_ UndoState) error {
				order = append(order, "provision")
				return nil
			},
		}},
	})

	ctx := &Context{Context: context.Background()}
	errs := s.Unwind(ctx)

	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	// LIFO: provision, install, prepare
	expected := []string{"provision", "install", "prepare"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d compensations, got %d", len(expected), len(order))
	}
	for i, name := range expected {
		if order[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, order[i])
		}
	}

	// Stack should be empty after unwind
	if s.Len() != 0 {
		t.Errorf("expected stack empty after unwind, got len %d", s.Len())
	}
}

func TestRecoveryStackUnwindCompensateError(t *testing.T) {
	s := &RecoveryStack{}

	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "prepare", Action: &testUndoAction{
			name: "prepare",
		}},
	})
	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "install", Action: &testUndoAction{
			name: "install",
			undoFn: func(_ UndoState) error {
				return fmt.Errorf("compensate install failed")
			},
		}},
	})
	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "provision", Action: &testUndoAction{
			name: "provision",
		}},
	})

	ctx := &Context{Context: context.Background()}
	errs := s.Unwind(ctx)

	// Should have 1 error but all three were executed
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Error() != "compensate install failed" {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestRecoveryStackUnwindNilCompensate(t *testing.T) {
	s := &RecoveryStack{}

	s.Push(RecoveryEntry{
		Node: &op.Node{ID: "prepare", Action: nil}, // No action — should be skipped
	})

	ctx := &Context{Context: context.Background()}
	errs := s.Unwind(ctx)

	if len(errs) != 0 {
		t.Errorf("expected no errors for nil action, got %v", errs)
	}
}

func TestRecoveryStackUnwindEmpty(t *testing.T) {
	s := &RecoveryStack{}

	ctx := &Context{Context: context.Background()}
	errs := s.Unwind(ctx)

	if len(errs) != 0 {
		t.Errorf("expected no errors for empty stack, got %v", errs)
	}
}

func TestRecoveryStackEntries(t *testing.T) {
	s := &RecoveryStack{}

	s.Push(RecoveryEntry{Node: &op.Node{ID: "a", Action: op.StubAction("first")}})
	s.Push(RecoveryEntry{Node: &op.Node{ID: "b", Action: op.StubAction("second")}})

	entries := s.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Node.ActionName() != "first" {
		t.Errorf("expected first entry action name 'first', got %q", entries[0].Node.ActionName())
	}
	if entries[1].Node.ActionName() != "second" {
		t.Errorf("expected second entry action name 'second', got %q", entries[1].Node.ActionName())
	}

	// Entries is a slice copy — appending to it does not affect the stack.
	_ = append(entries, RecoveryEntry{}) //nolint:ineffassign // verifying Entries() returns a copy
	original := s.Entries()
	if len(original) != 2 {
		t.Error("Entries() did not return a copy")
	}
}
