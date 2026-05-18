// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// region Helpers

func makeContainerKwarg(key string, value starlark.Value) starlark.Tuple {
	return starlark.Tuple{starlark.String(key), value}
}

func makeInvocationWithID(t *testing.T, id, receiver string) *Invocation {

	t.Helper()

	node := op.NewNode(id)
	node.Receiver = receiver

	return &Invocation{
		Label:   id + "#1",
		Target:  node,
		Promise: NewPromise(node, ""),
	}
}

// endregion

// region extractBodyKwarg

func TestExtractBodyKwarg_AbsentReturnsNil(t *testing.T) {

	kwargs := []starlark.Tuple{
		makeContainerKwarg("x", starlark.MakeInt(10)),
	}

	children, remaining, err := extractBodyKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractBodyKwarg: %v", err)
	}

	if children != nil {
		t.Errorf("children = %v, want nil", children)
	}

	if len(remaining) != 1 {
		t.Errorf("remaining kwargs len = %d, want 1", len(remaining))
	}
}

func TestExtractBodyKwarg_ListOfInvocations(t *testing.T) {

	a := makeInvocationWithID(t, "a", "file.write_text")
	b := makeInvocationWithID(t, "b", "file.move")

	list := starlark.NewList([]starlark.Value{a, b})
	kwargs := []starlark.Tuple{
		makeContainerKwarg("body", list),
		makeContainerKwarg("x", starlark.MakeInt(42)),
	}

	children, remaining, err := extractBodyKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractBodyKwarg: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("children len = %d, want 2", len(children))
	}

	if children[0].Target.ID() != "a" {
		t.Errorf("children[0].Target.ID() = %q, want %q", children[0].Target.ID(), "a")
	}

	if children[1].Target.ID() != "b" {
		t.Errorf("children[1].Target.ID() = %q, want %q", children[1].Target.ID(), "b")
	}

	if len(remaining) != 1 {
		t.Fatalf("remaining kwargs len = %d, want 1 (x= preserved)", len(remaining))
	}

	if key, _ := starlark.AsString(remaining[0][0]); key != "x" {
		t.Errorf("remaining kwarg key = %q, want %q", key, "x")
	}
}

func TestExtractBodyKwarg_NoneAcceptedAsEmpty(t *testing.T) {

	kwargs := []starlark.Tuple{
		makeContainerKwarg("body", starlark.None),
	}

	children, remaining, err := extractBodyKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractBodyKwarg: %v", err)
	}

	if children != nil {
		t.Errorf("children = %v, want nil (None)", children)
	}

	if len(remaining) != 0 {
		t.Errorf("remaining kwargs len = %d, want 0 (body= removed)", len(remaining))
	}
}

func TestExtractBodyKwarg_NonListErrors(t *testing.T) {

	kwargs := []starlark.Tuple{
		makeContainerKwarg("body", starlark.String("not a list")),
	}

	_, _, err := extractBodyKwarg(kwargs)
	if err == nil {
		t.Fatal("expected error for non-list body=")
	}

	if !strings.Contains(err.Error(), "list of invocations") {
		t.Errorf("error %q does not mention 'list of invocations'", err.Error())
	}
}

func TestExtractBodyKwarg_NonInvocationElementErrors(t *testing.T) {

	list := starlark.NewList([]starlark.Value{starlark.String("not an invocation")})
	kwargs := []starlark.Tuple{makeContainerKwarg("body", list)}

	_, _, err := extractBodyKwarg(kwargs)
	if err == nil {
		t.Fatal("expected error for non-Invocation element")
	}

	if !strings.Contains(err.Error(), "Invocation") {
		t.Errorf("error %q does not mention 'Invocation'", err.Error())
	}
}

// endregion

// region extractErrorActionKwarg

func TestExtractErrorActionKwarg_AbsentReturnsNil(t *testing.T) {

	kwargs := []starlark.Tuple{
		makeContainerKwarg("x", starlark.MakeInt(10)),
	}

	unit, remaining, err := extractErrorActionKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractErrorActionKwarg: %v", err)
	}

	if unit != nil {
		t.Errorf("unit = %v, want nil", unit)
	}

	if len(remaining) != 1 {
		t.Errorf("remaining kwargs len = %d, want 1", len(remaining))
	}
}

func TestExtractErrorActionKwarg_NodeAutoWrapsIntoSingleChildSubgraph(t *testing.T) {

	cleanup := makeInvocationWithID(t, "cleanup", "file.remove")

	kwargs := []starlark.Tuple{
		makeContainerKwarg("error_action", cleanup),
	}

	handler, remaining, err := extractErrorActionKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractErrorActionKwarg: %v", err)
	}

	if handler == nil {
		t.Fatal("handler = nil, want auto-wrapped Subgraph around the Node")
	}

	children := handler.Children()
	if len(children) != 1 {
		t.Fatalf("handler.Children() len = %d, want 1 (the wrapped Node)", len(children))
	}

	if children[0].ID() != "cleanup" {
		t.Errorf("wrapped child ID = %q, want %q", children[0].ID(), "cleanup")
	}

	if len(remaining) != 0 {
		t.Errorf("remaining kwargs len = %d, want 0", len(remaining))
	}
}

func TestExtractErrorActionKwarg_SubgraphPassesThroughUnwrapped(t *testing.T) {

	sg := op.NewSubgraph("recovery")
	sg.Name = "recovery"
	sg.Status = op.SubgraphPending

	inv := &Invocation{
		Label:  "recovery#1",
		Target: sg,
	}

	kwargs := []starlark.Tuple{
		makeContainerKwarg("error_action", inv),
	}

	handler, _, err := extractErrorActionKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractErrorActionKwarg: %v", err)
	}

	if handler != sg {
		t.Errorf("handler = %v, want the original Subgraph pointer (no wrap)", handler)
	}
}

// endregion

// region extractRetryPolicyKwarg

func TestExtractRetryPolicyKwarg_AbsentReturnsNil(t *testing.T) {

	kwargs := []starlark.Tuple{
		makeContainerKwarg("x", starlark.MakeInt(10)),
	}

	policy, remaining, err := extractRetryPolicyKwarg(kwargs)
	if err != nil {
		t.Fatalf("extractRetryPolicyKwarg: %v", err)
	}

	if policy != nil {
		t.Errorf("policy = %v, want nil", policy)
	}

	if len(remaining) != 1 {
		t.Errorf("remaining kwargs len = %d, want 1", len(remaining))
	}
}

// endregion

// region starlarkValueToSlotValue

func TestStarlarkValueToSlotValue_InvocationProducesPromiseValue(t *testing.T) {

	inv := makeInvocationWithID(t, "src", "file.read_text")

	sv, err := starlarkValueToSlotValue(inv)
	if err != nil {
		t.Fatalf("starlarkValueToSlotValue: %v", err)
	}

	pv, ok := sv.(op.PromiseValue)
	if !ok {
		t.Fatalf("got %T, want op.PromiseValue", sv)
	}

	if pv.NodeRef != "src" {
		t.Errorf("PromiseValue.NodeRef = %q, want %q", pv.NodeRef, "src")
	}
}

func TestStarlarkValueToSlotValue_LiteralProducesImmediateValue(t *testing.T) {

	sv, err := starlarkValueToSlotValue(starlark.MakeInt(42))
	if err != nil {
		t.Fatalf("starlarkValueToSlotValue: %v", err)
	}

	if _, ok := sv.(op.ImmediateValue); !ok {
		t.Fatalf("got %T, want op.ImmediateValue", sv)
	}
}

// endregion
