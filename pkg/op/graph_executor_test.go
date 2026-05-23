// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"testing"
)

// --- RecoveryStack.PushComplement dispatcher ---

func TestPushComplement_Nil_NoOp(t *testing.T) {

	parent := NewRecoveryStack()

	parent.PushComplement("test.nil", nil)

	if parent.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (nil complement should be a no-op)", parent.Len())
	}
}

func TestPushComplement_NestedStack_AppendsOneEntry(t *testing.T) {

	parent := NewRecoveryStack()
	child := NewRecoveryStack()

	parent.PushComplement("test.nested", child)

	if parent.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (nested stack should add one entry)", parent.Len())
	}
}

func TestPushComplement_UnknownShape_NoOp(t *testing.T) {

	parent := NewRecoveryStack()

	// Strings are not a known complement shape; the classifier in Method.NewMethod is supposed to reject
	// these at registration. PushComplement's switch silently drops unknown shapes; this test pins that
	// behavior so a future change that adds a default branch surfaces explicitly.
	parent.PushComplement("test.unknown", "not a complement")

	if parent.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (unknown complement shape should be a no-op)", parent.Len())
	}
}