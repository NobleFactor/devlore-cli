// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

func TestVariableBinding_Resolve_WholeValue(t *testing.T) {

	binding := NewVariableBinding("target")
	variables := map[string]Variable{"target": {Name: "target", Value: "/tmp/x"}}

	if got := binding.Resolve(variables, nil); got != "/tmp/x" {
		t.Errorf("Resolve() = %v, want %q", got, "/tmp/x")
	}
}

func TestVariableBinding_Resolve_ProjectsField(t *testing.T) {

	binding := NewVariableBindingWithField("item", "source")
	variables := map[string]Variable{
		"item": {Name: "item", Value: map[string]any{"source": "/tmp/a", "dest": "/tmp/b"}},
	}

	if got := binding.Resolve(variables, nil); got != "/tmp/a" {
		t.Errorf("Resolve() = %v, want %q (the projected field)", got, "/tmp/a")
	}
}

func TestVariableBinding_Resolve_ProjectionMiss(t *testing.T) {

	binding := NewVariableBindingWithField("item", "absent")

	// An absent field resolves nil.
	variables := map[string]Variable{"item": {Name: "item", Value: map[string]any{"source": "x"}}}
	if got := binding.Resolve(variables, nil); got != nil {
		t.Errorf("Resolve() = %v, want nil for an absent field", got)
	}

	// A non-record value resolves nil.
	variables = map[string]Variable{"item": {Name: "item", Value: "not-a-record"}}
	if got := binding.Resolve(variables, nil); got != nil {
		t.Errorf("Resolve() = %v, want nil for a non-record value", got)
	}
}

func TestBindings_DocumentRoundTrip_PreservesField(t *testing.T) {

	bindings := map[string]Binding{
		"projected": NewVariableBindingWithField("item", "source"),
		"whole":     NewVariableBinding("target"),
	}

	reloaded := assembleBindings(marshalBindings(bindings))

	projected, ok := reloaded["projected"].(VariableBinding)
	if !ok {
		t.Fatalf("reloaded[projected] is %T, want VariableBinding", reloaded["projected"])
	}
	if projected.Name() != "item" || projected.Field() != "source" {
		t.Errorf("reloaded projected = (%q, %q), want (item, source)", projected.Name(), projected.Field())
	}

	whole, ok := reloaded["whole"].(VariableBinding)
	if !ok {
		t.Fatalf("reloaded[whole] is %T, want VariableBinding", reloaded["whole"])
	}
	if whole.Name() != "target" || whole.Field() != "" {
		t.Errorf("reloaded whole = (%q, %q), want (target, \"\")", whole.Name(), whole.Field())
	}
}

// TestValidateGraph_ItemProjectionOutsideGather proves the scope check (phase-8 step 45): plan.item references the
// reserved per-iteration variable, which only a gather's dispatch frame binds, so a projection of `item` outside a
// gather body is a plan error.
func TestValidateGraph_ItemProjectionOutsideGather(t *testing.T) {

	explodeAction, err := ReceiverRegistry().BuildAction("compensationCleanFixture.explode")
	if err != nil {
		t.Fatalf("BuildAction(explode): %v", err)
	}

	node, err := NewNode(NewNodeSpec().WithID("stray").WithAction(explodeAction).
		WithSlot("input", NewVariableBindingWithField("item", "source")))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(node))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	validationErr := ValidateGraph(graph)
	if validationErr == nil {
		t.Fatal("ValidateGraph() = nil, want the outside-gather violation")
	}
	if !strings.Contains(validationErr.Error(), "outside a gather body") {
		t.Errorf("ValidateGraph() = %q, want it to name the outside-gather violation", validationErr)
	}
}
