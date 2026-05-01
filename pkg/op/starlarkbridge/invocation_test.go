// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// --- Helpers ---

func makeInvocation(t *testing.T, label string) *Invocation {

	t.Helper()

	node := op.NewNode("test-node-1")
	node.Receiver = "file.write_text"

	return &Invocation{
		Label:   label,
		Target:  node,
		Promise: NewPromise(node, ""),
	}
}

// --- starlark.Value surface ---

func TestInvocation_Type(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	if got := inv.Type(); got != "Invocation" {
		t.Errorf("Type() = %q, want %q", got, "Invocation")
	}
}

func TestInvocation_String(t *testing.T) {

	inv := makeInvocation(t, "file.write_text#1")

	want := "Invocation(file.write_text#1)"

	if got := inv.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestInvocation_Truth(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	if got := inv.Truth(); got != starlark.True {
		t.Errorf("Truth() = %v, want True", got)
	}
}

func TestInvocation_Hash_Unhashable(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	if _, err := inv.Hash(); err == nil {
		t.Error("Hash() error = nil, want non-nil (Invocation is unhashable)")
	}
}

func TestInvocation_Freeze_NoOp(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	inv.Freeze() // should not panic
}

// --- starlark.HasAttrs surface (delegates to Result) ---

func TestInvocation_Attr_DelegatesToPromise(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	attr, err := inv.Attr("node_id")
	if err != nil {
		t.Fatalf("Attr(node_id): %v", err)
	}
	if s, ok := attr.(starlark.String); !ok || string(s) != "test-node-1" {
		t.Errorf("Attr(node_id) = %v, want starlark.String(%q)", attr, "test-node-1")
	}
}

func TestInvocation_AttrNames_DelegatesToPromise(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	names := inv.AttrNames()

	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}

	for _, want := range []string{"node_id", "retry", "slot"} {
		if !have[want] {
			t.Errorf("AttrNames() missing %q; got %v", want, names)
		}
	}
}

// --- Project ---
//
// FillSlot is covered indirectly — it delegates to Promise.FillSlot, which relies on Node.SetSlot (requires a
// method-bound consumer node, established by full dispatch in integration). The delegation itself is a one-line
// pass-through not worth a unit-level harness.

func TestInvocation_Project_ToInvocationPointer(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	got, err := inv.Project(reflect.TypeFor[*Invocation]())
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got != inv {
		t.Errorf("got = %p, want %p", got, inv)
	}
}

func TestInvocation_Project_ToPromisePointer(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	got, err := inv.Project(reflect.TypeFor[*Promise]())
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got != inv.Promise {
		t.Errorf("got = %p, want %p", got, inv.Promise)
	}
}

func TestInvocation_Project_ToPromiseValue(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	got, err := inv.Project(reflect.TypeFor[op.PromiseValue]())
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	pv, ok := got.(op.PromiseValue)
	if !ok {
		t.Fatalf("got = %T, want op.PromiseValue", got)
	}
	if pv.NodeRef != "test-node-1" {
		t.Errorf("pv.NodeRef = %q, want %q", pv.NodeRef, "test-node-1")
	}
}

func TestInvocation_Project_UnsupportedTarget(t *testing.T) {

	inv := makeInvocation(t, "test#1")

	if _, err := inv.Project(reflect.TypeFor[string]()); err == nil {
		t.Error("Project(string target) error = nil, want error")
	}
}
