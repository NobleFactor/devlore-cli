// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"testing"

	"go.starlark.net/starlark"
)

// TestExtractOptionsKwarg_Absent verifies that kwargs without an "options" key pass through unchanged.
func TestExtractOptionsKwarg_Absent(t *testing.T) {

	kwargs := []starlark.Tuple{
		{starlark.String("label"), starlark.String("x")},
		{starlark.String("count"), starlark.MakeInt(3)},
	}

	opts, filtered, err := extractOptionsKwarg(kwargs)

	if err != nil {
		t.Fatalf("extractOptionsKwarg: %v", err)
	}
	if opts != nil {
		t.Errorf("opts = %v, want nil", opts)
	}
	if len(filtered) != len(kwargs) {
		t.Errorf("filtered length = %d, want %d", len(filtered), len(kwargs))
	}
}

// TestExtractOptionsKwarg_WrapperUnwrap verifies that a *goReceiver around *Options is unwrapped.
func TestExtractOptionsKwarg_WrapperUnwrap(t *testing.T) {

	options := &Options{Label: "my-label"}

	r := &goReceiver{instance: options}

	kwargs := []starlark.Tuple{
		{starlark.String("foo"), starlark.String("x")},
		{starlark.String("options"), r},
		{starlark.String("bar"), starlark.MakeInt(3)},
	}

	opts, filtered, err := extractOptionsKwarg(kwargs)

	if err != nil {
		t.Fatalf("extractOptionsKwarg: %v", err)
	}
	if opts != options {
		t.Errorf("opts = %p, want %p", opts, options)
	}
	if opts.Label != "my-label" {
		t.Errorf("opts.Label = %q, want %q", opts.Label, "my-label")
	}
	if len(filtered) != 2 {
		t.Fatalf("filtered length = %d, want 2", len(filtered))
	}
	if key, _ := starlark.AsString(filtered[0][0]); key != "foo" {
		t.Errorf("filtered[0] key = %q, want foo", key)
	}
	if key, _ := starlark.AsString(filtered[1][0]); key != "bar" {
		t.Errorf("filtered[1] key = %q, want bar", key)
	}
}

// TestExtractOptionsKwarg_None verifies explicit None is treated as "no options."
func TestExtractOptionsKwarg_None(t *testing.T) {

	kwargs := []starlark.Tuple{
		{starlark.String("options"), starlark.None},
	}

	opts, filtered, err := extractOptionsKwarg(kwargs)

	if err != nil {
		t.Fatalf("extractOptionsKwarg: %v", err)
	}
	if opts != nil {
		t.Errorf("opts = %v, want nil", opts)
	}
	if len(filtered) != 0 {
		t.Errorf("filtered length = %d, want 0", len(filtered))
	}
}

// TestExtractOptionsKwarg_WrongType verifies a non-*Options value produces an error.
func TestExtractOptionsKwarg_WrongType(t *testing.T) {

	kwargs := []starlark.Tuple{
		{starlark.String("options"), starlark.String("not an options")},
	}

	_, _, err := extractOptionsKwarg(kwargs)

	if err == nil {
		t.Fatal("extractOptionsKwarg: expected error, got nil")
	}
}

// TestExtractOptionsKwarg_WrongWrapperInstance verifies a *goReceiver around a non-*Options Go value errors.
func TestExtractOptionsKwarg_WrongWrapperInstance(t *testing.T) {

	type notOptions struct{ X int }

	v := &goReceiver{instance: &notOptions{X: 42}}

	kwargs := []starlark.Tuple{
		{starlark.String("options"), v},
	}

	_, _, err := extractOptionsKwarg(kwargs)

	if err == nil {
		t.Fatal("extractOptionsKwarg: expected error, got nil")
	}
}
