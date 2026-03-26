// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func Test_newReceiver(t *testing.T) {
	r := newReceiver("test_ns")
	if r.name != "test_ns" {
		t.Errorf("newReceiver name = %q, want %q", r.name, "test_ns")
	}
}

func TestReceiverString(t *testing.T) {
	r := newReceiver("my.namespace")
	if got := r.String(); got != "my.namespace" {
		t.Errorf("String() = %q, want %q", got, "my.namespace")
	}
}

func TestReceiverType(t *testing.T) {
	r := newReceiver("pkg.receiver")
	if got := r.Type(); got != "pkg.receiver" {
		t.Errorf("ProviderType() = %q, want %q", got, "pkg.receiver")
	}
}

func TestReceiverTruth(t *testing.T) {
	r := newReceiver("any")
	if got := r.Truth(); got != starlark.True {
		t.Errorf("Truth() = %v, want True", got)
	}
}

func TestReceiverHash(t *testing.T) {
	r := newReceiver("unhashable")
	_, err := r.Hash()
	if err == nil {
		t.Fatal("Hash() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unhashable type: unhashable") {
		t.Errorf("Hash() error = %q, want message containing %q", err.Error(), "unhashable type: unhashable")
	}
}

func TestReceiverFreeze(t *testing.T) {
	r := newReceiver("freezable")
	// Freeze should not panic
	r.Freeze()
}

func TestBuiltinFunc(t *testing.T) {
	called := false
	fn := func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		called = true
		return starlark.None, nil
	}
	builtin := starlark.NewBuiltin("do_thing", fn)
	if builtin.Name() != "do_thing" {
		t.Errorf("builtin.ReceiverName() = %q, want %q", builtin.Name(), "do_thing")
	}
	// Invoke the builtin to verify the function is wired up.
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
