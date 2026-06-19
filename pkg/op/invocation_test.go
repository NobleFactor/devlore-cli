// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "testing"

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
	if got.ProducerID() != "producer" {
		t.Errorf("Binding().ProducerID() = %q, want %q", got.ProducerID(), "producer")
	}
}
