// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"
)

// slotDispatchProvider is a synthetic provider whose one method pairs an [ExecutableUnit]-assignable parameter with a
// value-typed one, so a single planned call exercises both sides of the slot-fill dispatch. Built as a real receiver
// type via [NewProviderReceiverType]; never announced, so the process registry stays clean.
type slotDispatchProvider struct{}

func (slotDispatchProvider) Wrap(body ExecutableUnit, note string) error { return nil }

// planInvocatorStub satisfies [PlanInvocator] for planner tests without reaching for plan.Provider (which lives
// downstream of op).
type planInvocatorStub struct {
	registry           *InvocationRegistry
	runtimeEnvironment *RuntimeEnvironment
}

func (s planInvocatorStub) InvocationRegistry() *InvocationRegistry { return s.registry }
func (s planInvocatorStub) RuntimeEnvironment() *RuntimeEnvironment { return s.runtimeEnvironment }

func TestActionPlanner_ExecutableUnitAssignableDispatch(t *testing.T) {

	receiverType, err := NewProviderReceiverType(
		reflect.TypeFor[slotDispatchProvider](),
		func(*RuntimeEnvironment) (any, error) { return nil, nil },
		RoleAction,
		map[string][]Parameter{"Wrap": {
			{Name: "body", Type: reflect.TypeFor[ExecutableUnit]()},
			{Name: "note", Type: reflect.TypeFor[string]()},
		}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	method, ok := receiverType.MethodByName("Wrap")
	if !ok {
		t.Fatal(`receiver type lacks method "Wrap"`)
	}

	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}
	invocation := &Invocation{Target: producer, Label: "file.copy#1"}

	unit, err := ActionPlanner{}.Plan(
		planInvocatorStub{registry: NewInvocationRegistry(), runtimeEnvironment: &RuntimeEnvironment{}},
		receiverType, method, []any{invocation, invocation}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("ActionPlanner.Plan: %v", err)
	}

	slots := unit.(*Node).Slots()

	// The ExecutableUnit-assignable slot receives the structural unit reference, not a value-side promise.
	body, ok := slots["body"].(ImmediateBinding)
	if !ok {
		t.Fatalf(`slot "body" = %T, want ImmediateBinding (structural unit reference)`, slots["body"])
	}
	if resolved := body.Resolve(nil, nil); resolved != any(producer) {
		t.Errorf(`slot "body" resolves to %v, want the invocation's Target`, resolved)
	}

	// The value-typed slot receives the invocation's value-side output — a PromiseBinding carrying the producer's ID.
	note, ok := slots["note"].(PromiseBinding)
	if !ok {
		t.Fatalf(`slot "note" = %T, want PromiseBinding (value-side output)`, slots["note"])
	}
	if edge := note.Edge("consumer"); edge == nil || edge.From != "producer" {
		t.Errorf(`slot "note" edge = %#v, want From "producer"`, edge)
	}
}
