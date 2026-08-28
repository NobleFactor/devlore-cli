// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"strings"
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
		NewProviderFlags(SurfaceWorkflow, PlacementQualified),
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
		receiverType, method, []any{invocation, invocation}, nil, nil, nil, nil, nil, nil)
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

// mintProbeProvider is a synthetic provider whose one method takes a parameter typed by a resource
// INTERFACE, so the mint designation and its refusal can be exercised without a real provider. Never
// announced, so the process registry stays clean.
type mintProbeProvider struct{}

func (mintProbeProvider) Take(claim Resource) error { return nil }

// mintedResource is the concrete type a designation points at — the shape `file.AnyKind` has, minus the
// filesystem.
type mintedResource struct {
	ResourceBase
}

func (r *mintedResource) Addressing() AddressingMode { return AddressingLocation }
func (r *mintedResource) Exists() bool               { return true }
func (r *mintedResource) Resolve() error             { return nil }

// planStringIntoResourceInterface plans `Take("some/path")` and returns the planning error, if any.
func planStringIntoResourceInterface(t *testing.T) error {

	t.Helper()

	receiverType, err := NewProviderReceiverType(
		reflect.TypeFor[mintProbeProvider](),
		func(*RuntimeEnvironment) (any, error) { return nil, nil },
		NewProviderFlags(SurfaceWorkflow, PlacementQualified),
		map[string][]Parameter{"Take": {{Name: "claim", Type: reflect.TypeFor[Resource]()}}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	method, ok := receiverType.MethodByName("Take")
	if !ok {
		t.Fatal(`receiver type lacks method "Take"`)
	}

	_, err = ActionPlanner{}.Plan(
		planInvocatorStub{registry: NewInvocationRegistry(), runtimeEnvironment: &RuntimeEnvironment{}},
		receiverType, method, []any{"some/path"}, nil, nil, nil, nil, nil, nil)

	return err
}

// TestActionPlanner_AnUndesignatedResourceInterfaceRefusesAnAuthoredString pins the narrowed refusal
// (4-resource-management.md §5.7 rule 6, amended 2026-08-23): a claim asserts a kind, an interface
// asserts none, and an interface that designates no claim type has nothing to assert it on the author's
// behalf — so the string is refused rather than guessed at.
func TestActionPlanner_AnUndesignatedResourceInterfaceRefusesAnAuthoredString(t *testing.T) {

	err := planStringIntoResourceInterface(t)
	if err == nil {
		t.Fatal("planning succeeded; an undesignated resource interface must refuse an authored string")
	}
	if !strings.Contains(err.Error(), "designates") {
		t.Errorf("refusal %q does not name the missing designation", err)
	}
}

// TestActionPlanner_ADesignatedResourceInterfaceMintsTheClaim pins the other half: once the interface
// designates a claim type, the same authored string binds — the substitution happens ahead of the
// grammar and the conversion, so the slot fills exactly as an explicitly kinded parameter would.
func TestActionPlanner_ADesignatedResourceInterfaceMintsTheClaim(t *testing.T) {

	RegisterResourceMint(reflect.TypeFor[Resource](), reflect.TypeFor[*mintedResource]())
	t.Cleanup(func() {
		resourceMintsMu.Lock()
		delete(resourceMints, reflect.TypeFor[Resource]())
		resourceMintsMu.Unlock()
	})

	if err := planStringIntoResourceInterface(t); err == nil {
		t.Fatal("expected the designated mint type to be attempted")
	} else if strings.Contains(err.Error(), "designates") {
		t.Errorf("still reporting a missing designation after one was registered: %v", err)
	}
}
