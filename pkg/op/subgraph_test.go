// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// region TEST FIXTURES

// nodeWithSlots builds a Node whose Slots are hand-wired (bypassing Bind). The Parameter on each Slot
// carries the declared Type so type-collision tests can express incompatible declarations without going
// through the receiver-registry plumbing.
func nodeWithSlots(id string, slots ...*Slot) *Node {

	n := NewNode(id)
	n.Slots = slots
	return n
}

// stringSlot makes a Slot whose declared parameter type is string and whose value is the given SlotValue.
func stringSlot(paramName string, value SlotValue) *Slot {

	return &Slot{
		Parameter: Parameter{Name: paramName, Type: reflect.TypeFor[string]()},
		Value:     value,
	}
}

// intSlot makes a Slot whose declared parameter type is int and whose value is the given SlotValue.
func intSlot(paramName string, value SlotValue) *Slot {

	return &Slot{
		Parameter: Parameter{Name: paramName, Type: reflect.TypeFor[int]()},
		Value:     value,
	}
}

// endregion

func TestSubgraph_Parameters_SingleVariableSlot(t *testing.T) {

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n1",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
	)})

	params := sg.Parameters()
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

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n1",
		stringSlot("path", VariableValue{Name: "dest_dir"}),
		stringSlot("mode", ImmediateValue{Value: "0755"}),
		stringSlot("source", PromiseValue{NodeRef: "upstream"}),
	)})

	params := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (only VariableValue contributes)", len(params))
	}
	if params[0].Name != "dest_dir" {
		t.Errorf("Name = %q, want dest_dir", params[0].Name)
	}
}

func TestSubgraph_Parameters_DedupSameNameSameType(t *testing.T) {

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n1",
		stringSlot("path_a", VariableValue{Name: "root"}),
	)})
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n2",
		stringSlot("path_b", VariableValue{Name: "root"}),
	)})

	params := sg.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (same-name + same-type dedup)", len(params))
	}
	if params[0].Name != "root" {
		t.Errorf("Name = %q, want root", params[0].Name)
	}
}

func TestSubgraph_Parameters_TypeCollisionPanics(t *testing.T) {

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n1",
		stringSlot("a", VariableValue{Name: "x"}),
	)})
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n2",
		intSlot("b", VariableValue{Name: "x"}),
	)})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on type collision; got none")
		}
		var ae *assert.AssertionError
		if !errors.As(asError(r), &ae) {
			t.Fatalf("expected *assert.AssertionError, got %T: %v", r, r)
		}
		if !strings.Contains(ae.Message, "incompatible types") {
			t.Errorf("message %q does not mention incompatible types", ae.Message)
		}
		if !strings.Contains(ae.Message, `"x"`) {
			t.Errorf("message %q does not name the colliding variable", ae.Message)
		}
	}()

	_ = sg.Parameters()
}

func TestSubgraph_Parameters_NestedSubgraphRecursion(t *testing.T) {

	inner := NewSubgraph("inner")
	inner.AddChild(SubgraphChild{Node: nodeWithSlots("ni",
		stringSlot("path", VariableValue{Name: "from_inner"}),
	)})

	outer := NewSubgraph("outer")
	outer.AddChild(SubgraphChild{Node: nodeWithSlots("no",
		stringSlot("path", VariableValue{Name: "from_outer"}),
	)})
	outer.AddChild(SubgraphChild{Subgraph: inner})

	params := outer.Parameters()
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
	inner := NewSubgraph("inner")
	inner.AddChild(SubgraphChild{Node: nodeWithSlots("ni",
		stringSlot("a", VariableValue{Name: "shared"}),
	)})

	outer := NewSubgraph("outer")
	outer.AddChild(SubgraphChild{Node: nodeWithSlots("no",
		stringSlot("b", VariableValue{Name: "shared"}),
	)})
	outer.AddChild(SubgraphChild{Subgraph: inner})

	params := outer.Parameters()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1 (nested same-name + same-type dedup)", len(params))
	}
	if params[0].Name != "shared" {
		t.Errorf("Name = %q, want shared", params[0].Name)
	}
}

func TestSubgraph_Parameters_SortedByName(t *testing.T) {

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n",
		stringSlot("p1", VariableValue{Name: "zebra"}),
		stringSlot("p2", VariableValue{Name: "alpha"}),
		stringSlot("p3", VariableValue{Name: "mango"}),
	)})

	params := sg.Parameters()
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

func TestSubgraph_AddChild_StampsParent_Node(t *testing.T) {

	sg := NewSubgraph("sg")
	n := NewNode("n1")
	if n.Parent() != nil {
		t.Fatalf("fresh Node.Parent() = %v, want nil", n.Parent())
	}

	sg.AddChild(SubgraphChild{Node: n})

	if n.Parent() != sg {
		t.Errorf("Node.Parent() not stamped: got %v, want %v", n.Parent(), sg)
	}
}

func TestSubgraph_AddChild_StampsParent_Subgraph(t *testing.T) {

	outer := NewSubgraph("outer")
	inner := NewSubgraph("inner")
	if inner.Parent() != nil {
		t.Fatalf("fresh Subgraph.Parent() = %v, want nil", inner.Parent())
	}

	outer.AddChild(SubgraphChild{Subgraph: inner})

	if inner.Parent() != outer {
		t.Errorf("Subgraph.Parent() not stamped: got %v, want %v", inner.Parent(), outer)
	}
}

func TestSubgraph_AddChild_NestedOwnership(t *testing.T) {

	outer := NewSubgraph("outer")
	middle := NewSubgraph("middle")
	inner := NewSubgraph("inner")
	leaf := NewNode("leaf")

	inner.AddChild(SubgraphChild{Node: leaf})
	middle.AddChild(SubgraphChild{Subgraph: inner})
	outer.AddChild(SubgraphChild{Subgraph: middle})

	// Walk up: leaf → inner → middle → outer → nil
	if leaf.Parent() != inner {
		t.Errorf("leaf.Parent() = %v, want inner", leaf.Parent())
	}
	if inner.Parent() != middle {
		t.Errorf("inner.Parent() = %v, want middle", inner.Parent())
	}
	if middle.Parent() != outer {
		t.Errorf("middle.Parent() = %v, want outer", middle.Parent())
	}
	if outer.Parent() != nil {
		t.Errorf("outer.Parent() = %v, want nil (root of this tree)", outer.Parent())
	}
}

func TestGraph_AddNodeAndAddSubgraph_StampParent(t *testing.T) {

	g := NewGraph()

	n := NewNode("n")
	g.AddNode(n)
	if n.Parent() != g.Root {
		t.Errorf("AddNode: parent = %v, want g.Root", n.Parent())
	}

	sg := NewSubgraph("sg")
	g.AddSubgraph(sg)
	if sg.Parent() != g.Root {
		t.Errorf("AddSubgraph: parent = %v, want g.Root", sg.Parent())
	}

	if g.Root.Parent() != nil {
		t.Errorf("Root.Parent() = %v, want nil (graph root)", g.Root.Parent())
	}
}

func TestSubgraph_Parameters_EmptySubgraph(t *testing.T) {

	sg := NewSubgraph("empty")
	params := sg.Parameters()
	if len(params) != 0 {
		t.Errorf("empty subgraph: len = %d, want 0", len(params))
	}
}

func TestSubgraph_Parameters_NodeWithNoVariableSlots(t *testing.T) {

	sg := NewSubgraph("sg")
	sg.AddChild(SubgraphChild{Node: nodeWithSlots("n",
		stringSlot("a", ImmediateValue{Value: "x"}),
		stringSlot("b", PromiseValue{NodeRef: "upstream"}),
	)})

	params := sg.Parameters()
	if len(params) != 0 {
		t.Errorf("len = %d, want 0 (no VariableValue slots)", len(params))
	}
}

// asError coerces a recovered panic value to an error so errors.As can walk it. The package's assert.raise
// panics with *AssertionError, which is an error, so the panic value is type-equivalent.
func asError(v any) error {

	if e, ok := v.(error); ok {
		return e
	}
	return nil
}
