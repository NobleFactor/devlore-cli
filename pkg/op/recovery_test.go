// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRecoveryStack_Do_PushesOnSuccess(t *testing.T) {
	s := NewRecoveryStack()

	err := s.Do(
		func() (any, any, error) { return "undo-state", nil, nil },
		func(state any) error { return nil },
		nil,
	)

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
}

func TestRecoveryStack_Do_ReturnsErrorWithoutPushing(t *testing.T) {
	s := NewRecoveryStack()
	invokeErr := errors.New("invoke failed")

	err := s.Do(
		func() (any, any, error) { return nil, nil, invokeErr },
		func(state any) error { return nil },
		nil,
	)

	if !errors.Is(err, invokeErr) {
		t.Fatalf("Do() error = %v, want %v", err, invokeErr)
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (nothing pushed on failure)", s.Len())
	}
}

func TestRecoveryStack_Unwind_LIFO(t *testing.T) {
	s := NewRecoveryStack()
	var order []int

	for i := range 3 {
		i := i
		s.Push(
			func(any) error { order = append(order, i); return nil },
			nil, i, nil,
		)
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

func TestRecoveryStack_Unwind_SkipsDrifted(t *testing.T) {
	s := NewRecoveryStack()
	compensated := false

	s.Push(
		func(any) error { compensated = true; return nil },
		func(any) (bool, error) { return false, nil }, // drifted
		"undo", "reconcile",
	)

	err := s.Unwind()
	if !errors.Is(err, errDrifted) {
		t.Fatalf("Unwind() error = %v, want errDrifted", err)
	}
	if compensated {
		t.Error("compensate was called despite drift detection")
	}
}

func TestRecoveryStack_Unwind_BestEffort(t *testing.T) {
	s := NewRecoveryStack()
	var compensated []int

	// Entry 0: succeeds
	s.Push(
		func(any) error { compensated = append(compensated, 0); return nil },
		nil, nil, nil,
	)
	// Entry 1: fails
	s.Push(
		func(any) error { return errors.New("compensate-1 failed") },
		nil, nil, nil,
	)
	// Entry 2: succeeds
	s.Push(
		func(any) error { compensated = append(compensated, 2); return nil },
		nil, nil, nil,
	)

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

func TestRecoveryStack_Unwind_NilReconcile(t *testing.T) {
	s := NewRecoveryStack()
	compensated := false

	s.Push(
		func(any) error { compensated = true; return nil },
		nil, // nil reconcile = always safe
		"undo", nil,
	)

	if err := s.Unwind(); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}
	if !compensated {
		t.Error("compensate was not called with nil reconcile")
	}
}

func TestRecoveryStack_Discard(t *testing.T) {
	s := NewRecoveryStack()
	compensated := false

	s.Push(
		func(any) error { compensated = true; return nil },
		nil, nil, nil,
	)

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

	s.Push(func(any) error { return nil }, nil, nil, nil)

	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}

	s.Push(func(any) error { return nil }, nil, nil, nil)

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
	child := NewRecoveryStack()

	var childCompensated bool
	child.Push(func(any) error { childCompensated = true; return nil }, nil, nil, nil)

	parent.PushNested(child)

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

func TestRecoveryStack_MarshalJSON_ClosureOnlyEntry_Errors(t *testing.T) {
	s := NewRecoveryStack()

	s.Push(func(any) error { return nil }, nil, nil, nil)

	if _, err := json.Marshal(s); err == nil {
		t.Fatal("MarshalJSON() error = nil, want error for closure-only entry")
	}
}
