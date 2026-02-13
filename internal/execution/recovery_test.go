// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"fmt"
	"testing"
)

func TestRecoveryStackPushLen(t *testing.T) {
	s := &RecoveryStack{}

	if s.Len() != 0 {
		t.Errorf("expected empty stack, got len %d", s.Len())
	}

	s.Push(RecoveryEntry{PhaseID: "phase.prepare", PhaseName: "prepare"})
	s.Push(RecoveryEntry{PhaseID: "phase.install", PhaseName: "install"})

	if s.Len() != 2 {
		t.Errorf("expected len 2, got %d", s.Len())
	}
}

func TestRecoveryStackUnwindLIFO(t *testing.T) {
	s := &RecoveryStack{}
	var order []string

	s.Push(RecoveryEntry{
		PhaseID:   "phase.prepare",
		PhaseName: "prepare",
		Compensate: func(ctx *Context) error {
			order = append(order, "prepare")
			return nil
		},
	})
	s.Push(RecoveryEntry{
		PhaseID:   "phase.install",
		PhaseName: "install",
		Compensate: func(ctx *Context) error {
			order = append(order, "install")
			return nil
		},
	})
	s.Push(RecoveryEntry{
		PhaseID:   "phase.provision",
		PhaseName: "provision",
		Compensate: func(ctx *Context) error {
			order = append(order, "provision")
			return nil
		},
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
		PhaseID:   "phase.prepare",
		PhaseName: "prepare",
		Compensate: func(ctx *Context) error {
			return nil
		},
	})
	s.Push(RecoveryEntry{
		PhaseID:   "phase.install",
		PhaseName: "install",
		Compensate: func(ctx *Context) error {
			return fmt.Errorf("compensate install failed")
		},
	})
	s.Push(RecoveryEntry{
		PhaseID:   "phase.provision",
		PhaseName: "provision",
		Compensate: func(ctx *Context) error {
			return nil
		},
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
		PhaseID:    "phase.prepare",
		PhaseName:  "prepare",
		Compensate: nil, // No compensating action
	})

	ctx := &Context{Context: context.Background()}
	errs := s.Unwind(ctx)

	if len(errs) != 0 {
		t.Errorf("expected no errors for nil compensate, got %v", errs)
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

	s.Push(RecoveryEntry{PhaseID: "a", PhaseName: "first"})
	s.Push(RecoveryEntry{PhaseID: "b", PhaseName: "second"})

	entries := s.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].PhaseID != "a" {
		t.Errorf("expected first entry 'a', got %q", entries[0].PhaseID)
	}
	if entries[1].PhaseID != "b" {
		t.Errorf("expected second entry 'b', got %q", entries[1].PhaseID)
	}

	// Entries is a copy — original stack should be unaffected
	entries[0].PhaseID = "modified"
	original := s.Entries()
	if original[0].PhaseID != "a" {
		t.Error("Entries() did not return a copy")
	}
}
