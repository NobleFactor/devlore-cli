// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestGoToStarlarkValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantType string
		wantStr  string
	}{
		{"string", "hello", "string", `"hello"`},
		{"int", 42, "int", "42"},
		{"int64", int64(999), "int", "999"},
		{"bool_true", true, "bool", "True"},
		{"bool_false", false, "bool", "False"},
		{"float64", 3.14, "float", "3.14"},
		{"string_slice", []string{"a", "b"}, "list", `["a", "b"]`},
		{"empty_string_slice", []string{}, "list", "[]"},
		{"unknown_type_struct", struct{ X int }{7}, "string", `"{7}"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GoToStarlarkValue(tt.input)
			if err != nil {
				t.Fatalf("GoToStarlarkValue(%v) returned error: %v", tt.input, err)
			}
			if got.Type() != tt.wantType {
				t.Errorf("GoToStarlarkValue(%v).ProviderType() = %q, want %q", tt.input, got.Type(), tt.wantType)
			}
			if got.String() != tt.wantStr {
				t.Errorf("GoToStarlarkValue(%v).String() = %q, want %q", tt.input, got.String(), tt.wantStr)
			}
		})
	}
}

func TestStarlarkValueToGo_String(t *testing.T) {
	val, err := StarlarkValueToGo(starlark.String("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}
}

func TestStarlarkValueToGo_Int(t *testing.T) {
	val, err := StarlarkValueToGo(starlark.MakeInt(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	i, ok := val.(int)
	if !ok {
		t.Fatalf("expected int, got %T", val)
	}
	if i != 42 {
		t.Errorf("got %d, want %d", i, 42)
	}
}

func TestStarlarkValueToGo_Bool(t *testing.T) {
	tests := []struct {
		name string
		in   starlark.Bool
		want bool
	}{
		{"true", starlark.True, true},
		{"false", starlark.False, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := StarlarkValueToGo(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			b, ok := val.(bool)
			if !ok {
				t.Fatalf("expected bool, got %T", val)
			}
			if b != tt.want {
				t.Errorf("got %v, want %v", b, tt.want)
			}
		})
	}
}

func TestStarlarkValueToGo_Float(t *testing.T) {
	val, err := StarlarkValueToGo(starlark.Float(2.718))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, ok := val.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", val)
	}
	if f != 2.718 {
		t.Errorf("got %f, want %f", f, 2.718)
	}
}

func TestStarlarkValueToGo_NoneType(t *testing.T) {
	val, err := StarlarkValueToGo(starlark.None)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestStarlarkValueToGo_List(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.String("b"),
	})
	val, err := StarlarkValueToGo(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ss, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", val)
	}
	if len(ss) != 2 || ss[0] != "a" || ss[1] != "b" {
		t.Errorf("got %v, want [a b]", ss)
	}
}

func TestStarlarkValueToGo_Dict(t *testing.T) {
	dict := starlark.NewDict(1)
	if err := dict.SetKey(starlark.String("key"), starlark.MakeInt(10)); err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}
	val, err := StarlarkValueToGo(dict)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", val)
	}
	if m["key"] != 10 {
		t.Errorf("got m[key] = %v, want 10", m["key"])
	}
}

func TestStarlarkValueToGo_Unsupported(t *testing.T) {
	// starlark.NewBuiltin is a type not handled by StarlarkValueToGo
	builtin := starlark.NewBuiltin("dummy",
		func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.None, nil
		},
	)
	_, err := StarlarkValueToGo(builtin)
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
}

func TestStarlarkListToSlice_HomogeneousStrings(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("x"),
		starlark.String("y"),
		starlark.String("z"),
	})
	val, err := StarlarkListToSlice(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ss, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", val)
	}
	if len(ss) != 3 || ss[0] != "x" || ss[1] != "y" || ss[2] != "z" {
		t.Errorf("got %v, want [x y z]", ss)
	}
}

func TestStarlarkListToSlice_MixedTypes(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.MakeInt(1),
		starlark.Bool(true),
	})
	val, err := StarlarkListToSlice(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slice, ok := val.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", val)
	}
	if len(slice) != 3 {
		t.Fatalf("got len %d, want 3", len(slice))
	}
	if s, ok := slice[0].(string); !ok || s != "a" {
		t.Errorf("element 0: got %v, want string(a)", slice[0])
	}
	if i, ok := slice[1].(int); !ok || i != 1 {
		t.Errorf("element 1: got %v, want int(1)", slice[1])
	}
	if b, ok := slice[2].(bool); !ok || b != true {
		t.Errorf("element 2: got %v, want bool(true)", slice[2])
	}
}

func TestStarlarkListToSlice_Empty(t *testing.T) {
	list := starlark.NewList(nil)
	val, err := StarlarkListToSlice(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ss, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string for empty list, got %T", val)
	}
	if len(ss) != 0 {
		t.Errorf("got len %d, want 0", len(ss))
	}
}

func TestStarlarkDictToMap_StringKeys(t *testing.T) {
	dict := starlark.NewDict(2)
	if err := dict.SetKey(starlark.String("name"), starlark.String("test")); err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}
	if err := dict.SetKey(starlark.String("count"), starlark.MakeInt(5)); err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}

	m, err := StarlarkDictToMap(dict)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("m[name] = %v, want test", m["name"])
	}
	if m["count"] != 5 {
		t.Errorf("m[count] = %v, want 5", m["count"])
	}
}

func TestStarlarkDictToMap_NonStringKey(t *testing.T) {
	dict := starlark.NewDict(1)
	if err := dict.SetKey(starlark.MakeInt(42), starlark.String("val")); err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}

	_, err := StarlarkDictToMap(dict)
	if err == nil {
		t.Fatal("expected error for non-string key, got nil")
	}
}

func TestStarlarkDictToMap_Empty(t *testing.T) {
	dict := starlark.NewDict(0)
	m, err := StarlarkDictToMap(dict)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("got len %d, want 0", len(m))
	}
}
