// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
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

// testCompensableAction implements CompensableAction for PushAction tests.
type testCompensableAction struct {
	undoFn func(Complement) error
}

func (a *testCompensableAction) Name() string        { return "test.compensable" }
func (a *testCompensableAction) Params() []Parameter { return nil }

func (a *testCompensableAction) Do(_ *ExecutionContext, _ map[string]any) (Result, Complement, error) {
	return nil, nil, nil
}

func (a *testCompensableAction) Undo(_ *ExecutionContext, state Complement) error {
	if a.undoFn != nil {
		return a.undoFn(state)
	}
	return nil
}

func TestRecoveryStack_PushAction_CompensableAction(t *testing.T) {
	s := NewRecoveryStack()
	var undone bool

	action := &testCompensableAction{
		undoFn: func(_ Complement) error { undone = true; return nil },
	}

	ctx := &ExecutionContext{}
	s.PushAction(ctx, action, "state")

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}

	if err := s.Unwind(); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}
	if !undone {
		t.Error("compensable do was not called")
	}
}

// plainAction is a non-compensable action for testing.
type plainAction struct{}

func (a *plainAction) Name() string                                                   { return "test.plain" }
func (a *plainAction) Params() []Parameter                                            { return nil }
func (a *plainAction) Do(_ *ExecutionContext, _ map[string]any) (Result, Complement, error) { return nil, nil, nil }

func TestRecoveryStack_PushAction_NonCompensable(t *testing.T) {
	s := NewRecoveryStack()

	// A plain Action (no Undo method) should be silently ignored.
	s.PushAction(&ExecutionContext{}, &plainAction{}, "state")

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (non-compensable should be ignored)", s.Len())
	}
}

func TestRecoveryStack_PushAction_FiltersErrNotCompensable(t *testing.T) {
	s := NewRecoveryStack()

	action := &testCompensableAction{
		undoFn: func(_ Complement) error { return ErrNotCompensable },
	}

	s.PushAction(&ExecutionContext{}, action, "undo")

	// Unwind should return nil — ErrNotCompensable is filtered.
	if err := s.Unwind(); err != nil {
		t.Errorf("Unwind() error = %v, want nil (ErrNotCompensable should be filtered)", err)
	}
}
