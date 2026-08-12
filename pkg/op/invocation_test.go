// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"
)

func TestInvocation_Binding(t *testing.T) {
	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}
	invocation := &Invocation{Target: producer, Label: "file.copy#1"}

	got := invocation.Binding()

	// Invocation.Binding returns a PromiseBinding referencing the producer by ID.
	if want := NewPromiseBinding("producer"); got != want {
		t.Errorf("Invocation.Binding() = %#v, want %#v", got, want)
	}
	if edge := got.Edge("consumer"); edge == nil || edge.From != "producer" || edge.To != "consumer" {
		t.Errorf("Binding().Edge(%q) = %#v, want &{From:producer To:consumer}", "consumer", edge)
	}
}

func TestInvocation_Detached_NoGraphReference(t *testing.T) {

	// Structural half of the detachment contract (phase-8 D5): neither the plan-time handle nor its value-side
	// binding declares graph-typed state — the producer→consumer relationship travels as a unit ID until
	// plan.assemble_definition materializes the edge.
	graphType := reflect.TypeFor[Graph]()

	for _, subject := range []reflect.Type{reflect.TypeFor[Invocation](), reflect.TypeFor[PromiseBinding]()} {
		for i := range subject.NumField() {
			field := subject.Field(i)
			if field.Type == graphType || (field.Type.Kind() == reflect.Pointer && field.Type.Elem() == graphType) {
				t.Errorf("%s field %q is graph-typed (%s); plan-time values must stay detached",
					subject.Name(), field.Name, field.Type)
			}
		}
	}

	// Behavioral half: resolution goes through the recovery stack, never a graph — with no stack there is nothing
	// to consult.
	if resolved := NewPromiseBinding("producer").Resolve(nil, nil); resolved != nil {
		t.Errorf("PromiseBinding.Resolve(nil, nil) = %v, want nil (no stack, nothing to consult)", resolved)
	}
}
