// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestNewReceiver(t *testing.T) {
	r := NewReceiver("test_ns")
	if r.name != "test_ns" {
		t.Errorf("NewReceiver name = %q, want %q", r.name, "test_ns")
	}
}

func TestReceiverString(t *testing.T) {
	r := NewReceiver("my.namespace")
	if got := r.String(); got != "my.namespace" {
		t.Errorf("String() = %q, want %q", got, "my.namespace")
	}
}

func TestReceiverType(t *testing.T) {
	r := NewReceiver("pkg.receiver")
	if got := r.Type(); got != "pkg.receiver" {
		t.Errorf("Type() = %q, want %q", got, "pkg.receiver")
	}
}

func TestReceiverTruth(t *testing.T) {
	r := NewReceiver("any")
	if got := r.Truth(); got != starlark.True {
		t.Errorf("Truth() = %v, want True", got)
	}
}

func TestReceiverHash(t *testing.T) {
	r := NewReceiver("unhashable")
	_, err := r.Hash()
	if err == nil {
		t.Fatal("Hash() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unhashable type: unhashable") {
		t.Errorf("Hash() error = %q, want message containing %q", err.Error(), "unhashable type: unhashable")
	}
}

func TestReceiverFreeze(t *testing.T) {
	r := NewReceiver("freezable")
	// Freeze should not panic
	r.Freeze()
}

func TestListToStringSlice_AllStrings(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("alpha"),
		starlark.String("beta"),
		starlark.String("gamma"),
	})
	got := ListToStringSlice(list)
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("element %d = %q, want %q", i, got[i], s)
		}
	}
}

func TestListToStringSlice_MixedTypes(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("hello"),
		starlark.MakeInt(42),
		starlark.Bool(true),
	})
	got := ListToStringSlice(list)
	// Non-string elements produce empty string via AsString failure
	if got[0] != "hello" {
		t.Errorf("element 0 = %q, want %q", got[0], "hello")
	}
	if got[1] != "" {
		t.Errorf("element 1 = %q, want empty (int cannot be AsString)", got[1])
	}
	if got[2] != "" {
		t.Errorf("element 2 = %q, want empty (bool cannot be AsString)", got[2])
	}
}

func TestListToStringSlice_Empty(t *testing.T) {
	list := starlark.NewList(nil)
	got := ListToStringSlice(list)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestMakeAttr(t *testing.T) {
	called := false
	fn := func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		called = true
		return starlark.None, nil
	}
	attr := MakeAttr("do_thing", fn)
	builtin, ok := attr.(*starlark.Builtin)
	if !ok {
		t.Fatalf("MakeAttr returned %T, want *starlark.Builtin", attr)
	}
	if builtin.Name() != "do_thing" {
		t.Errorf("builtin.Name() = %q, want %q", builtin.Name(), "do_thing")
	}
	// Invoke the builtin to verify the function is wired up
	_, err := builtin.CallInternal(nil, nil, nil)
	if err != nil {
		t.Fatalf("calling builtin: %v", err)
	}
	if !called {
		t.Error("underlying function was not called")
	}
}

func TestNoSuchAttrError(t *testing.T) {
	err := NoSuchAttrError("file", "unknown_method")
	want := "file has no .unknown_method attribute"
	if err.Error() != want {
		t.Errorf("NoSuchAttrError() = %q, want %q", err.Error(), want)
	}
}

func TestNoSuchAttrError_DifferentNames(t *testing.T) {
	tests := []struct {
		receiver string
		attr     string
		want     string
	}{
		{"shell", "run", "shell has no .run attribute"},
		{"net", "download", "net has no .download attribute"},
		{"template", "render", "template has no .render attribute"},
	}
	for _, tt := range tests {
		t.Run(tt.receiver+"_"+tt.attr, func(t *testing.T) {
			err := NoSuchAttrError(tt.receiver, tt.attr)
			if err.Error() != tt.want {
				t.Errorf("got %q, want %q", err.Error(), tt.want)
			}
		})
	}
}
