// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRecoveryStack_Unwind_LIFO(t *testing.T) {
	s := NewRecoveryStack()
	var order []int

	for i := range 3 {
		child := NewRecoveryStack()
		child.PushNested(tagStack(i, func(v int) { order = append(order, v) }))
		s.PushNested(child)
	}

	if err := s.Unwind(); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 compensations, got %d", len(order))
	}
	// LIFO: 2, 1, 0
	for i, want := range []int{2, 1, 0} {
		if order[i] != want {
			t.Errorf("order[%d] = %d, want %d", i, order[i], want)
		}
	}

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after Unwind", s.Len())
	}
}

func TestRecoveryStack_Unwind_BestEffort(t *testing.T) {
	s := NewRecoveryStack()
	var compensated []int

	// Entry 0: succeeds
	s.PushNested(tagStack(0, func(v int) { compensated = append(compensated, v) }))
	// Entry 1: fails
	s.PushNested(failStack(errors.New("compensate-1 failed")))
	// Entry 2: succeeds
	s.PushNested(tagStack(2, func(v int) { compensated = append(compensated, v) }))

	err := s.Unwind()
	if err == nil {
		t.Fatal("Unwind() should return error when a compensation fails")
	}

	// Entry 1 failed, but entries 0 and 2 should still have been compensated (LIFO: 2, 0).
	if len(compensated) != 2 {
		t.Fatalf("expected 2 successful compensations, got %d: %v", len(compensated), compensated)
	}
	if compensated[0] != 2 || compensated[1] != 0 {
		t.Errorf("compensated = %v, want [2, 0]", compensated)
	}
}

func TestRecoveryStack_Discard(t *testing.T) {
	s := NewRecoveryStack()
	compensated := false

	s.PushNested(tagStack(0, func(int) { compensated = true }))

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}

	s.Discard()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after Discard", s.Len())
	}
	if compensated {
		t.Error("compensate was called after Discard (should not unwind)")
	}
}

func TestRecoveryStack_Len(t *testing.T) {
	s := NewRecoveryStack()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 for empty stack", s.Len())
	}

	s.PushNested(NewRecoveryStack())

	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}

	s.PushNested(NewRecoveryStack())

	if s.Len() != 2 {
		t.Errorf("Len() = %d, want 2", s.Len())
	}
}

func TestRecoveryStack_PushNested_AppendsOneEntry(t *testing.T) {
	parent := NewRecoveryStack()
	child := NewRecoveryStack()

	parent.PushNested(child)

	if parent.Len() != 1 {
		t.Errorf("parent.Len() = %d, want 1", parent.Len())
	}
}

func TestRecoveryStack_PushNested_NilIsNoOp(t *testing.T) {
	parent := NewRecoveryStack()

	parent.PushNested(nil)

	if parent.Len() != 0 {
		t.Errorf("parent.Len() = %d, want 0 (nil sub should be no-op)", parent.Len())
	}
}

func TestRecoveryStack_PushNested_UnwindRecurses(t *testing.T) {
	parent := NewRecoveryStack()
	childCompensated := false

	parent.PushNested(tagStack(0, func(int) { childCompensated = true }))

	if err := parent.Unwind(); err != nil {
		t.Fatalf("parent.Unwind() error = %v", err)
	}

	if !childCompensated {
		t.Error("child compensate was not invoked by parent.Unwind()")
	}
}

func TestRecoveryStack_MarshalJSON_Empty(t *testing.T) {
	s := NewRecoveryStack()

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := `{"entries":[]}`
	if string(data) != want {
		t.Errorf("MarshalJSON = %q, want %q", string(data), want)
	}
}

func TestRecoveryStack_MarshalJSON_NestedSub(t *testing.T) {
	parent := NewRecoveryStack()
	child := NewRecoveryStack()
	parent.PushNested(child)

	data, err := json.Marshal(parent)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := `{"entries":[{"sub":{"entries":[]}}]}`
	if string(data) != want {
		t.Errorf("MarshalJSON = %q, want %q", string(data), want)
	}
}

// tagStack builds a one-entry nested stack whose compensate closure calls record with tag.
func tagStack(tag int, record func(int)) *RecoveryStack {
	inner := NewRecoveryStack()
	leaf := NewRecoveryStack()
	leaf.entries = append(leaf.entries, recoveryEntry{
		compensate: func(any) error { record(tag); return nil },
	})
	inner.PushNested(leaf)
	return inner
}

// failStack builds a one-entry nested stack whose compensate closure returns err.
func failStack(err error) *RecoveryStack {
	inner := NewRecoveryStack()
	leaf := NewRecoveryStack()
	leaf.entries = append(leaf.entries, recoveryEntry{
		compensate: func(any) error { return err },
	})
	inner.PushNested(leaf)
	return inner
}
