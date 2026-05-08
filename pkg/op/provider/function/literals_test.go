// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestFormatLiteral_None(t *testing.T) {
	got, err := FormatLiteral(starlark.None)
	if err != nil {
		t.Fatal(err)
	}
	if got != "None" {
		t.Errorf("got %q, want %q", got, "None")
	}
}

func TestFormatLiteral_Bool(t *testing.T) {
	for _, tc := range []struct {
		val  starlark.Bool
		want string
	}{
		{starlark.True, "True"},
		{starlark.False, "False"},
	} {
		got, err := FormatLiteral(tc.val)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("FormatLiteral(%v) = %q, want %q", tc.val, got, tc.want)
		}
	}
}

func TestFormatLiteral_Int(t *testing.T) {
	got, err := FormatLiteral(starlark.MakeInt(42))
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

func TestFormatLiteral_Float(t *testing.T) {
	got, err := FormatLiteral(starlark.Float(3.14))
	if err != nil {
		t.Fatal(err)
	}
	if got != "3.14" {
		t.Errorf("got %q, want %q", got, "3.14")
	}
}

func TestFormatLiteral_String(t *testing.T) {
	got, err := FormatLiteral(starlark.String("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if got != `"hello world"` {
		t.Errorf("got %q, want %q", got, `"hello world"`)
	}
}

func TestFormatLiteral_StringEscaping(t *testing.T) {
	got, err := FormatLiteral(starlark.String("line1\nline2\ttab\\back\"quote"))
	if err != nil {
		t.Fatal(err)
	}
	want := `"line1\nline2\ttab\\back\"quote"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_EmptyList(t *testing.T) {
	l := starlark.NewList(nil)
	got, err := FormatLiteral(l)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[]" {
		t.Errorf("got %q, want %q", got, "[]")
	}
}

func TestFormatLiteral_List(t *testing.T) {
	l := starlark.NewList([]starlark.Value{
		starlark.MakeInt(1),
		starlark.String("two"),
		starlark.True,
	})
	got, err := FormatLiteral(l)
	if err != nil {
		t.Fatal(err)
	}
	want := `[1, "two", True]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_EmptyDict(t *testing.T) {
	d := starlark.NewDict(0)
	got, err := FormatLiteral(d)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{}" {
		t.Errorf("got %q, want %q", got, "{}")
	}
}

func TestFormatLiteral_Dict(t *testing.T) {
	d := starlark.NewDict(2)
	_ = d.SetKey(starlark.String("a"), starlark.MakeInt(1))
	_ = d.SetKey(starlark.String("b"), starlark.MakeInt(2))
	got, err := FormatLiteral(d)
	if err != nil {
		t.Fatal(err)
	}
	// Dict iteration order is insertion order in Starlark.
	want := `{"a": 1, "b": 2}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_Tuple(t *testing.T) {
	tup := starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)}
	got, err := FormatLiteral(tup)
	if err != nil {
		t.Fatal(err)
	}
	if got != "(1, 2)" {
		t.Errorf("got %q, want %q", got, "(1, 2)")
	}
}

func TestFormatLiteral_SingleElementTuple(t *testing.T) {
	tup := starlark.Tuple{starlark.MakeInt(1)}
	got, err := FormatLiteral(tup)
	if err != nil {
		t.Fatal(err)
	}
	if got != "(1,)" {
		t.Errorf("got %q, want %q", got, "(1,)")
	}
}

func TestFormatLiteral_NestedStructure(t *testing.T) {
	inner := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)})
	d := starlark.NewDict(1)
	_ = d.SetKey(starlark.String("items"), inner)
	got, err := FormatLiteral(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"items": [1, 2]}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_Struct(t *testing.T) {
	s := starlarkstruct.FromStringDict(starlark.String("resource"), starlark.StringDict{
		"uri":         starlark.String("scheme://example"),
		"source_path": starlark.String("/tmp/foo"),
	})
	got, err := FormatLiteral(s)
	if err != nil {
		t.Fatal(err)
	}
	// Keys are sorted alphabetically.
	want := `{"source_path": "/tmp/foo", "uri": "scheme://example"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_NestedStruct(t *testing.T) {
	inner := starlarkstruct.FromStringDict(starlark.String("resource_base"), starlark.StringDict{
		"uri": starlark.String("scheme://example"),
		"id":  starlark.String("abc123"),
	})
	outer := starlarkstruct.FromStringDict(starlark.String("resource"), starlark.StringDict{
		"resource_base": inner,
		"data":          starlark.String("content"),
	})
	got, err := FormatLiteral(outer)
	if err != nil {
		t.Fatal(err)
	}
	// Outer keys sorted, inner struct recursively serialized as dict.
	want := `{"data": "content", "resource_base": {"id": "abc123", "uri": "scheme://example"}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLiteral_EmptyStruct(t *testing.T) {
	s := starlarkstruct.FromStringDict(starlark.String("empty"), starlark.StringDict{})
	got, err := FormatLiteral(s)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{}" {
		t.Errorf("got %q, want %q", got, "{}")
	}
}

func TestFormatLiteral_UnsupportedType(t *testing.T) {
	s := starlark.NewSet(0)
	_, err := FormatLiteral(s)
	if err == nil {
		t.Fatal("expected error for set type")
	}
}
