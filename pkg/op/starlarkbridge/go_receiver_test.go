// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"
	"testing"

	"go.starlark.net/starlark"

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
