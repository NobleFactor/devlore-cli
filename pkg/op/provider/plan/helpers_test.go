// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

func TestProjectToBinding_Dispatch(t *testing.T) {

	producer, err := op.NewNode(op.NewNodeSpec().WithID("producer").WithActionNamed(file.Mkdir))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}
	invocation := &op.Invocation{Target: producer, Label: "file.mkdir#1"}

	// *op.Invocation → PromiseBinding referencing the invocation's Target by ID.
	got := projectToBinding(invocation)
	if want := op.NewPromiseBinding("producer"); got != op.Binding(want) {
		t.Errorf("projectToBinding(*op.Invocation) = %#v, want %#v", got, want)
	}

	// *op.Variable → VariableBinding carrying the variable's name.
	got = projectToBinding(&op.Variable{Name: "region"})
	if want := op.NewVariableBinding("region"); got != op.Binding(want) {
		t.Errorf("projectToBinding(*op.Variable) = %#v, want %#v", got, want)
	}

	// Anything else → ImmediateBinding wrapping the raw value.
	immediate, ok := projectToBinding(42).(op.ImmediateBinding)
	if !ok {
		t.Fatalf("projectToBinding(42) = %T, want op.ImmediateBinding", projectToBinding(42))
	}
	if value := immediate.Resolve(nil, nil); value != 42 {
		t.Errorf("ImmediateBinding.Resolve() = %v, want 42", value)
	}
}
