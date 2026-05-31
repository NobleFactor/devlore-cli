// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"strings"
	"testing"
)

// region TEST FIXTURES

// slotSpec carries the parameter declaration and bound [SlotValue] for one slot on a synthesized Node.
// Slot-type information now lives on the bound [Action]'s [Method] (per step 15 collapse); this spec
// lets tests express typed parameter declarations without going through the receiver-registry plumbing.
type slotSpec struct {
	name  string
	typ   reflect.Type
	value SlotValue
}

// nodeWithSlots builds a Node bound to a synthesized [Action] whose [Method] declares the slot
// parameters. Each entry in slots becomes both a [Parameter] on the fake method (for
// [Node.Parameters] type lookup) and a [SetSlot] call on the node.
//
// Parameters:
//   - `id`: the node identifier.
//   - slots: the typed slot specifications.
//
// Returns:
//   - *Node: the constructed node, bound to a synthesized action.
func nodeWithSlots(id string, slots ...slotSpec) *Node {

	params := make([]Parameter, len(slots))
	for i, s := range slots {
		params[i] = Parameter{Name: s.name, Type: s.typ}
	}

	n := NewNode(id, &action{method: &Method{parameters: params}}, nil)
	for _, s := range slots {
		n.setSlot(s.name, s.value)
	}
	return n
}

// stringSlot makes a [slotSpec] for a `string`-typed parameter bound to value.
func stringSlot(name string, value SlotValue) slotSpec {

	return slotSpec{name: name, typ: reflect.TypeFor[string](), value: value}
}

// stubSubgraph constructs a Subgraph bound to a stub action so the [NewSubgraph] action invariant
// holds. The stub action has no method; any attempt to dispatch this subgraph via action.Do would
// panic — appropriate for tests that exercise containment / bubble-up / parent-stamping behavior
// without invoking the executor.
func stubSubgraph(id string) *Subgraph {

	sg, err := NewSubgraph(id, &action{name: "stub"}, nil, nil, nil, nil, nil)
	if err != nil {
		panic("stubSubgraph: " + err.Error())
	}
	return sg
}

// intSlot makes a [slotSpec] for an `int`-typed parameter bound to value.
func intSlot(name string, value SlotValue) slotSpec {

	return slotSpec{name: name, typ: reflect.TypeFor[int](), value: value}
}

// endregion

func TestSubgraph_Parameters_SingleVariableSlot(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n1",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1", len(params))
	}
	if params[0].Name != "dest_dir" {
		t.Errorf("Name = %q, want dest_dir (variable name, not method param name)", params[0].Name)
	}
	if params[0].Type != reflect.TypeFor[string]() {
		t.Errorf("Type = %v, want string", params[0].Type)
	}
}

func TestSubgraph_Parameters_ImmediateAndPromise_DoNotContribute(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n1",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
		stringSlot("mode", ImmediateValue{Value: "0755"}),
		stringSlot("source", PromiseValue{UnitRef: "upstream"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (only VariableValue contributes)", len(params))
	}
	if params[0].Name != "dest_dir" {
		t.Errorf("Name = %q, want dest_dir", params[0].Name)
	}
}

func TestSubgraph_Parameters_DedupSameNameSameType(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n1",
		stringSlot("path_a", VariableValue{Name: "root"}),
	))
	sg.addChild(nodeWithSlots("n2",
		stringSlot("path_b", VariableValue{Name: "root"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (same-name + same-type dedup)", len(params))
	}
	if params[0].Name != "root" {
		t.Errorf("Name = %q, want root", params[0].Name)
	}
}

func TestSubgraph_Parameters_TypeCollision_ReturnsError(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n1",
		stringSlot("a", VariableValue{Name: "x"}),
	))
	sg.addChild(nodeWithSlots("n2",
		intSlot("b", VariableValue{Name: "x"}),
	))

	_, err := sg.Parameters()
	if err == nil {
		t.Fatal("expected error on type collision; got nil")
	}
	if !strings.Contains(err.Error(), "incompatible types") {
		t.Errorf("error %q does not mention incompatible types", err)
	}
	if !strings.Contains(err.Error(), `"x"`) {
		t.Errorf("error %q does not name the colliding variable", err)
	}
}

func TestSubgraph_Parameters_NestedSubgraphRecursion(t *testing.T) {

	inner := stubSubgraph("inner")
	inner.addChild(nodeWithSlots("ni",
		stringSlot("path", VariableValue{Name: "from_inner"}),
	))

	outer := stubSubgraph("outer")
	outer.addChild(nodeWithSlots("no",
		stringSlot("path", VariableValue{Name: "from_outer"}),
	))
	outer.addChild(inner)

	params, _ := outer.Parameters()
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2 (outer + inner contributions)", len(params))
	}

	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
	}
	for _, want := range []string{"from_inner", "from_outer"} {
		if !names[want] {
			t.Errorf("missing %q from bubble-up surface (have %v)", want, names)
		}
	}
}

func TestSubgraph_Parameters_NestedSubgraphDedup(t *testing.T) {

	// inner and outer both contribute "shared" with same type → dedup to one.
	inner := stubSubgraph("inner")
	inner.addChild(nodeWithSlots("ni",
		stringSlot("a", VariableValue{Name: "shared"}),
	))

	outer := stubSubgraph("outer")
	outer.addChild(nodeWithSlots("no",
		stringSlot("b", VariableValue{Name: "shared"}),
	))
	outer.addChild(inner)

	params, _ := outer.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (nested same-name + same-type dedup)", len(params))
	}
	if params[0].Name != "shared" {
		t.Errorf("Name = %q, want shared", params[0].Name)
	}
}

func TestSubgraph_Parameters_SortedByName(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n",
		stringSlot("p1", VariableValue{Name: "zebra"}),
		stringSlot("p2", VariableValue{Name: "alpha"}),
		stringSlot("p3", VariableValue{Name: "mango"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 3 {
		t.Fatalf("len = %d, want 3", len(params))
	}
	want := []string{"alpha", "mango", "zebra"}
	for i, w := range want {
		if params[i].Name != w {
			t.Errorf("params[%d].Name = %q, want %q (stable order)", i, params[i].Name, w)
		}
	}
}

func TestSubgraph_Parameters_FrameBindings_FullyBoundReturnsEmpty(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
	))
	sg.setSlot("dest_dir", ImmediateValue{Value: "/tmp/x"})

	params, _ := sg.Parameters()
	if len(params) != 0 {
		t.Errorf("len = %d, want 0 (dest_dir is bound locally)", len(params))
	}
}

func TestSubgraph_Parameters_FrameBindings_PartialBindingFiltersOnlyMatching(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n",
		stringSlot("p1", VariableValue{Name: "alpha"}),
		stringSlot("p2", VariableValue{Name: "beta"}),
	))
	sg.setSlot("alpha", ImmediateValue{Value: "hello"})

	params, _ := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len = %d, want 1 (alpha bound; beta exposed)", len(params))
	}
	if params[0].Name != "beta" {
		t.Errorf("Name = %q, want beta (alpha was filtered out)", params[0].Name)
	}
}

func TestSubgraph_Parameters_FrameBindings_NestedHidesFromOuter(t *testing.T) {

	// Inner subgraph references "secret" and binds it locally via a frame-binding slot; "secret" must NOT bubble up to
	// outer. Outer references "shared" only via its own child node; that variable stays exposed.

	inner := stubSubgraph("inner")
	inner.addChild(nodeWithSlots("inner_node",
		stringSlot("p", VariableValue{Name: "secret"}),
	))
	inner.setSlot("secret", ImmediateValue{Value: "hidden"})

	outer := stubSubgraph("outer")
	outer.addChild(inner)
	outer.addChild(nodeWithSlots("outer_node",
		stringSlot("q", VariableValue{Name: "shared"}),
	))

	params, _ := outer.Parameters()
	if len(params) != 1 {
		t.Fatalf("len = %d, want 1 (inner's secret is filtered at its own level; only shared bubbles up)", len(params))
	}
	if params[0].Name != "shared" {
		t.Errorf("Name = %q, want shared", params[0].Name)
	}
}

func TestSubgraph_Parameters_FrameBindings_EmptyMapIsNoOp(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len = %d, want 1 (no frame bindings on subgraph does not filter)", len(params))
	}
	if params[0].Name != "dest_dir" {
		t.Errorf("Name = %q, want dest_dir", params[0].Name)
	}
}

func TestSubgraph_AddChild_StampsParent_Node(t *testing.T) {

	sg := stubSubgraph("sg")
	n := NewNode("n1", &action{name: "stub"}, nil)

	if n.ParentID() != "" {
		t.Fatalf("fresh Node.ParentID() = %q, want empty", n.ParentID())
	}

	sg.addChild(n)

	if n.ParentID() != sg.ID() {
		t.Errorf("Node.ParentID() not stamped: got %q, want %q", n.ParentID(), sg.ID())
	}
}

func TestSubgraph_AddChild_StampsParent_Subgraph(t *testing.T) {

	outer := stubSubgraph("outer")
	inner := stubSubgraph("inner")

	if inner.ParentID() != "" {
		t.Fatalf("fresh Subgraph.ParentID() = %q, want empty", inner.ParentID())
	}

	outer.addChild(inner)

	if inner.ParentID() != outer.ID() {
		t.Errorf("Subgraph.ParentID() not stamped: got %q, want %q", inner.ParentID(), outer.ID())
	}
}

func TestSubgraph_AddChild_NestedOwnership(t *testing.T) {

	outer := stubSubgraph("outer")
	middle := stubSubgraph("middle")
	inner := stubSubgraph("inner")
	leaf := NewNode("leaf", &action{name: "stub"}, nil)

	inner.addChild(leaf)
	middle.addChild(inner)
	outer.addChild(middle)

	// Walk up the parent-ID chain: leaf → inner → middle → outer → "" (root of this tree).

	if leaf.ParentID() != inner.ID() {
		t.Errorf("leaf.ParentID() = %q, want %q", leaf.ParentID(), inner.ID())
	}

	if inner.ParentID() != middle.ID() {
		t.Errorf("inner.ParentID() = %q, want %q", inner.ParentID(), middle.ID())
	}

	if middle.ParentID() != outer.ID() {
		t.Errorf("middle.ParentID() = %q, want %q", middle.ParentID(), outer.ID())
	}

	if outer.ParentID() != "" {
		t.Errorf("outer.ParentID() = %q, want empty (root of this tree)", outer.ParentID())
	}
}

func TestSubgraph_SetErrorAction_StampsParent_Subgraph(t *testing.T) {

	outer := stubSubgraph("outer")
	handler := stubSubgraph("on-failure")

	outer.setErrorAction(handler)

	if handler.ParentID() != outer.ID() {
		t.Errorf("handler.ParentID() = %q, want %q", handler.ParentID(), outer.ID())
	}
	if outer.ErrorAction() != handler {
		t.Errorf("ErrorAction() did not return the handler")
	}
}

func TestSubgraph_SetErrorAction_Nil_ClearsWithoutStamping(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.setErrorAction(nil)

	if sg.ErrorAction() != nil {
		t.Errorf("ErrorAction() = %v, want nil after SetErrorAction(nil)", sg.ErrorAction())
	}
}

func TestSubgraph_SetErrorAction_RejectsConflictingReparent(t *testing.T) {

	first := stubSubgraph("first")
	second := stubSubgraph("second")
	handler := stubSubgraph("on-failure")

	first.setErrorAction(handler)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on conflicting re-parent; got none")
		}
	}()

	second.setErrorAction(handler) // stampParent panics: already parented to "first".
}

func TestSubgraph_Parameters_EmptySubgraph(t *testing.T) {

	sg := stubSubgraph("empty")
	params, _ := sg.Parameters()
	if len(params) != 0 {
		t.Errorf("empty subgraph: len = %d, want 0", len(params))
	}
}

func TestSubgraph_Parameters_NodeWithNoVariableSlots(t *testing.T) {

	sg := stubSubgraph("sg")
	sg.addChild(nodeWithSlots("n",
		stringSlot("a", ImmediateValue{Value: "x"}),
		stringSlot("b", PromiseValue{UnitRef: "upstream"}),
	))

	params, _ := sg.Parameters()
	if len(params) != 0 {
		t.Errorf("len = %d, want 0 (no VariableValue slots)", len(params))
	}
}

