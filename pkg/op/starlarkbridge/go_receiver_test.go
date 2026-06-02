// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region TEST FIXTURES

// metadataWrapProbe is an announced value type used to prove that an ad-hoc NewGoReceiver wrap honors registered
// MethodMetadata: Tag is announced with op.ModifierProperty (eager), Plain carries no modifier (callable).
type metadataWrapProbe struct{ value string }

// Tag is announced with op.ModifierProperty — it must project as an eager property.
func (p *metadataWrapProbe) Tag() string { return "tagged-" + p.value }

// Plain has no modifier — it must project as a callable builtin.
func (p *metadataWrapProbe) Plain() string { return "plain" }

func init() {
	op.AnnounceType(reflect.TypeFor[metadataWrapProbe](), map[string]op.MethodMetadata{
		"Tag":   {ParameterNames: []string{}, Modifiers: op.ModifierProperty},
		"Plain": {ParameterNames: []string{}},
	})
}

// endregion

// region TESTS

// TestNewGoReceiver_AdHocWrapCarriesRegisteredMetadata verifies that NewGoReceiver resolves through the announced
// registry (op.ResolveReceiverType), so an ad-hoc wrap of a registered type carries its MethodMetadata. Before the
// fix NewGoReceiver derived via reflection (op.NewReceiverType with nil metadata), which dropped Modifiers — the
// property would then project as a callable builtin instead of an eager value.
func TestNewGoReceiver_AdHocWrapCarriesRegisteredMetadata(t *testing.T) {

	receiver, err := NewGoReceiver(&metadataWrapProbe{value: "x"})
	if err != nil {
		t.Fatalf("NewGoReceiver: %v", err)
	}

	// The +property method eager-evaluates: Attr yields the call result, not the callable.
	tag, err := receiver.Attr("tag")
	if err != nil {
		t.Fatalf("Attr(tag): %v", err)
	}
	if s, ok := starlark.AsString(tag); !ok || s != "tagged-x" {
		t.Errorf("Attr(tag) = %v (%T), want eager string %q", tag, tag, "tagged-x")
	}

	// A method without the modifier stays a callable builtin.
	plain, err := receiver.Attr("plain")
	if err != nil {
		t.Fatalf("Attr(plain): %v", err)
	}
	if _, ok := plain.(*starlark.Builtin); !ok {
		t.Errorf("Attr(plain) = %T, want *starlark.Builtin", plain)
	}
}

// endregion

// region NAMED SCALAR FIXTURES

// rankProbe is a named scalar (underlying int) that declares a method — a method-bearing named scalar.
type rankProbe int

// Label makes rankProbe method-bearing (value receiver).
func (r rankProbe) Label() string { return "rank-" + strconv.Itoa(int(r)) }

// scalarHostProbe carries a method-bearing named-scalar field and a plain builtin field, to verify the projection
// split.
type scalarHostProbe struct {
	Ranked rankProbe `starlark:"ranked"`
	Plain  int       `starlark:"plain"`
}

// endregion

// region NAMED SCALAR TESTS

// TestNamedScalar_MethodsAndOrderedComparison verifies that a named scalar with methods projects through a
// goReceiver that exposes its methods and supports same-type ordered comparison (delegated to the underlying scalar
// values), with equality intact.
func TestNamedScalar_MethodsAndOrderedComparison(t *testing.T) {

	one, err := NewGoReceiver(rankProbe(1))
	if err != nil {
		t.Fatalf("NewGoReceiver(rankProbe(1)): %v", err)
	}
	two, err := NewGoReceiver(rankProbe(2))
	if err != nil {
		t.Fatalf("NewGoReceiver(rankProbe(2)): %v", err)
	}
	oneAgain, _ := NewGoReceiver(rankProbe(1))

	// The method is exposed as a callable builtin (not a +devlore:property getter) and actually dispatches on the
	// wrapped scalar value — a value-receiver method invoked through a pointer the dispatch synthesizes from the copy.
	label, err := one.Attr("label")
	if err != nil {
		t.Fatalf("Attr(label): %v", err)
	}
	builtin, ok := label.(*starlark.Builtin)
	if !ok {
		t.Fatalf("Attr(label) = %T, want *starlark.Builtin", label)
	}
	if res, err := starlark.Call(&starlark.Thread{}, builtin, nil, nil); err != nil {
		t.Fatalf("calling label(): %v", err)
	} else if s, _ := starlark.AsString(res); s != "rank-1" {
		t.Errorf("label() = %v, want %q", res, "rank-1")
	}

	cases := []struct {
		op   syntax.Token
		x, y starlark.Value
		want bool
	}{
		{syntax.LT, one, two, true},
		{syntax.GT, one, two, false},
		{syntax.LE, one, oneAgain, true},
		{syntax.GE, two, one, true},
		{syntax.EQL, one, oneAgain, true},
		{syntax.NEQ, one, two, true},
	}
	for _, c := range cases {
		got, err := starlark.CompareDepth(c.op, c.x, c.y, 1)
		if err != nil {
			t.Errorf("CompareDepth(%s): %v", c.op, err)
			continue
		}
		if got != c.want {
			t.Errorf("CompareDepth(%s) = %v, want %v", c.op, got, c.want)
		}
	}
}

// TestGoReceiver_StructRejectsOrderedComparison verifies that a struct-backed receiver still rejects ordering —
// only scalar-backed receivers gain it — while equality is unaffected.
func TestGoReceiver_StructRejectsOrderedComparison(t *testing.T) {

	a, err := NewGoReceiver(&metadataWrapProbe{value: "a"})
	if err != nil {
		t.Fatalf("NewGoReceiver: %v", err)
	}
	b, _ := NewGoReceiver(&metadataWrapProbe{value: "b"})

	if _, err := starlark.CompareDepth(syntax.LT, a, b, 1); err == nil {
		t.Errorf("struct < struct: want error, got nil")
	}
	if eq, err := starlark.CompareDepth(syntax.EQL, a, a, 1); err != nil || !eq {
		t.Errorf("struct == itself = (%v, %v), want (true, nil)", eq, err)
	}
}

// TestNamedScalar_FieldProjection verifies the projection split: a method-bearing named-scalar field projects as a
// goReceiver (Option C), while a plain builtin field projects as its starlark scalar.
func TestNamedScalar_FieldProjection(t *testing.T) {

	host, err := NewGoReceiver(&scalarHostProbe{Ranked: 5, Plain: 7})
	if err != nil {
		t.Fatalf("NewGoReceiver: %v", err)
	}

	ranked, err := host.Attr("ranked")
	if err != nil {
		t.Fatalf("Attr(ranked): %v", err)
	}
	if _, ok := ranked.(*goReceiver); !ok {
		t.Errorf("Attr(ranked) = %T, want *goReceiver (named scalar with methods wraps)", ranked)
	}

	plain, err := host.Attr("plain")
	if err != nil {
		t.Fatalf("Attr(plain): %v", err)
	}
	if _, ok := plain.(starlark.Int); !ok {
		t.Errorf("Attr(plain) = %T, want starlark.Int", plain)
	}
}

// endregion
