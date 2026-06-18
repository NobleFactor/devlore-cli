// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "testing"

func TestInvocation_SlotValue_DelegatesToResultPromise(t *testing.T) {
	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}
	promise := NewPromise(producer, "out")
	invocation := &Invocation{Target: producer, Result: promise, Label: "file.copy#1"}

	got := invocation.SlotValue()

	// Invocation.SlotValue must return exactly what its Result promise returns.
	if want := promise.SlotValue(); got != want {
		t.Errorf("Invocation.SlotValue() = %#v, want delegated %#v", got, want)
	}

	// Concretely, that is a PromiseValue referencing the producer by ID.
	promiseValue, ok := got.(PromiseValue)
	if !ok {
		t.Fatalf("SlotValue() type = %T, want PromiseValue", got)
	}
	if promiseValue.UnitRef != "producer" || promiseValue.Slot != "out" {
		t.Errorf("SlotValue() = %+v, want {UnitRef: producer, Slot: out}", promiseValue)
	}
}
