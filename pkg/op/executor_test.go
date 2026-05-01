// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"testing"
)

// --- pushComplement dispatcher ---

// pushComplementTestAction is a minimal Action stub used to drive pushComplement's dispatch.
type pushComplementTestAction struct {
	fullName string
}

func (a *pushComplementTestAction) FullName() string                                            { return a.fullName }
func (a *pushComplementTestAction) Name() string                                                { return a.fullName }
func (a *pushComplementTestAction) Params() []Parameter                                         { return nil }
func (a *pushComplementTestAction) Do(_ *RuntimeEnvironment, _ map[string]any) (Result, Complement, error) { return nil, nil, nil }

// pushComplementTestResource is a minimal Resource that satisfies the PushReceipt nil-resource and
// nil-context guards without dragging in the full ResourceBase machinery.
type pushComplementTestResource struct {
	ResourceBase
}

func (r *pushComplementTestResource) URI() string                       { return "test://pushcomplement" }
func (r *pushComplementTestResource) Resolve() error                    { return nil }
func (r *pushComplementTestResource) RuntimeEnvironment() *RuntimeEnvironment { return r.ResourceBase.RuntimeEnvironment() }

func TestPushComplement_Nil_NoOp(t *testing.T) {

	parent := NewRecoveryStack()
	action := &pushComplementTestAction{fullName: "test.nil"}

	pushComplement(parent, action, nil)

	if parent.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (nil complement should be a no-op)", parent.Len())
	}
}

func TestPushComplement_NestedStack_AppendsOneEntry(t *testing.T) {

	parent := NewRecoveryStack()
	child := NewRecoveryStack()
	action := &pushComplementTestAction{fullName: "test.nested"}

	pushComplement(parent, action, child)

	if parent.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (nested stack should add one entry)", parent.Len())
	}
}

func TestPushComplement_UnknownShape_NoOp(t *testing.T) {

	parent := NewRecoveryStack()
	action := &pushComplementTestAction{fullName: "test.unknown"}

	// Strings are not a known complement shape; the classifier in Method.NewMethod is supposed to reject
	// these at registration. pushComplement's switch silently drops unknown shapes; this test pins that
	// behavior so a future change that adds a default branch surfaces explicitly.
	pushComplement(parent, action, "not a complement")

	if parent.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (unknown complement shape should be a no-op)", parent.Len())
	}
}