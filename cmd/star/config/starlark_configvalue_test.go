// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"testing"

	"go.starlark.net/starlark"
)

// ConfigValueTestStruct is a test struct for ConfigValue tests
type ConfigValueTestStruct struct {
	ConfigElement
	Enabled     bool
	Name        string
	Count       int
	Score       float64
	Items       []string
	SkipModTidy bool
}

func TestConfigValue_String(t *testing.T) {
	s := &ConfigValueTestStruct{Name: "test"}
	cv := WrapAsStarlarkValue(s)

	got := cv.String()
	if got == "" {
		t.Error("String() should not be empty")
	}
	if got == "config(nil)" {
		t.Error("String() should not be 'config(nil)' for valid struct")
	}
}

func TestConfigValue_String_Nil(t *testing.T) {
	cv := WrapAsStarlarkValue(nil)
	if got := cv.String(); got != "config(nil)" {
		t.Errorf("String() = %q, want 'config(nil)'", got)
	}
}

func TestConfigValue_Type(t *testing.T) {
	cv := WrapAsStarlarkValue(&ConfigValueTestStruct{})
	if got := cv.Type(); got != "config" {
		t.Errorf("Type() = %q, want 'config'", got)
	}
}

func TestConfigValue_Truth(t *testing.T) {
	cv := WrapAsStarlarkValue(&ConfigValueTestStruct{})
	if got := cv.Truth(); got != starlark.True {
		t.Errorf("Truth() = %v, want True", got)
	}
}

func TestConfigValue_Hash(t *testing.T) {
	cv := WrapAsStarlarkValue(&ConfigValueTestStruct{})
	_, err := cv.Hash()
	if err == nil {
		t.Error("Hash() should return error (config is not hashable)")
	}
}

func TestConfigValue_Attr_Bool(t *testing.T) {
	s := &ConfigValueTestStruct{Enabled: true}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("enabled")
	if err != nil {
		t.Fatalf("Attr('enabled') error: %v", err)
	}
	if got, ok := val.(starlark.Bool); !ok || got != starlark.True {
		t.Errorf("Attr('enabled') = %v, want True", val)
	}
}

func TestConfigValue_Attr_String(t *testing.T) {
	s := &ConfigValueTestStruct{Name: "test"}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("name")
	if err != nil {
		t.Fatalf("Attr('name') error: %v", err)
	}
	if got, ok := val.(starlark.String); !ok || string(got) != "test" {
		t.Errorf("Attr('name') = %v, want 'test'", val)
	}
}

func TestConfigValue_Attr_Int(t *testing.T) {
	s := &ConfigValueTestStruct{Count: 42}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("count")
	if err != nil {
		t.Fatalf("Attr('count') error: %v", err)
	}
	if got, ok := val.(starlark.Int); !ok {
		t.Errorf("Attr('count') = %v (%T), want Int", val, val)
	} else {
		i, _ := got.Int64()
		if i != 42 {
			t.Errorf("Attr('count') = %d, want 42", i)
		}
	}
}

func TestConfigValue_Attr_Float(t *testing.T) {
	s := &ConfigValueTestStruct{Score: 3.14}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("score")
	if err != nil {
		t.Fatalf("Attr('score') error: %v", err)
	}
	if got, ok := val.(starlark.Float); !ok || float64(got) != 3.14 {
		t.Errorf("Attr('score') = %v, want 3.14", val)
	}
}

func TestConfigValue_Attr_Slice(t *testing.T) {
	s := &ConfigValueTestStruct{Items: []string{"a", "b", "c"}}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("items")
	if err != nil {
		t.Fatalf("Attr('items') error: %v", err)
	}
	list, ok := val.(*starlark.List)
	if !ok {
		t.Fatalf("Attr('items') = %T, want *starlark.List", val)
	}
	if list.Len() != 3 {
		t.Errorf("items len = %d, want 3", list.Len())
	}
}

func TestConfigValue_Attr_SnakeCase(t *testing.T) {
	s := &ConfigValueTestStruct{SkipModTidy: true}
	cv := WrapAsStarlarkValue(s)

	val, err := cv.Attr("skip_mod_tidy")
	if err != nil {
		t.Fatalf("Attr('skip_mod_tidy') error: %v", err)
	}
	if got, ok := val.(starlark.Bool); !ok || got != starlark.True {
		t.Errorf("Attr('skip_mod_tidy') = %v, want True", val)
	}
}

func TestConfigValue_Attr_NotFound(t *testing.T) {
	s := &ConfigValueTestStruct{}
	cv := WrapAsStarlarkValue(s)

	_, err := cv.Attr("missing")
	if err == nil {
		t.Error("Attr('missing') should return error")
	}
	// Should be NoSuchAttrError
	if _, ok := err.(starlark.NoSuchAttrError); !ok {
		t.Errorf("error type = %T, want NoSuchAttrError", err)
	}
}

func TestConfigValue_Attr_Nil(t *testing.T) {
	cv := WrapAsStarlarkValue(nil)

	_, err := cv.Attr("anything")
	if err == nil {
		t.Error("Attr on nil should return error")
	}
}

func TestConfigValue_AttrNames(t *testing.T) {
	s := &ConfigValueTestStruct{}
	cv := WrapAsStarlarkValue(s)

	names := cv.AttrNames()
	if len(names) == 0 {
		t.Fatal("AttrNames() should return field names")
	}

	// Convert to map for easier checking
	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}

	expected := []string{"enabled", "name", "count", "score", "items", "skip_mod_tidy"}
	for _, exp := range expected {
		if !nameMap[exp] {
			t.Errorf("AttrNames() should contain %q", exp)
		}
	}
}

func TestConfigValue_AttrNames_Nil(t *testing.T) {
	cv := WrapAsStarlarkValue(nil)
	names := cv.AttrNames()
	if names != nil {
		t.Errorf("AttrNames() on nil = %v, want nil", names)
	}
}

func TestConfigValue_Attr_WithChildren(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	// Create a hierarchy with ConfigElement children
	cfg := newExtensionsConfig("test.yaml")
	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
		},
		Defaults: map[string]interface{}{
			"enabled": true,
		},
	}
	cfg.registerExtension("lint.go", spec)

	cv := WrapAsStarlarkValue(cfg)

	// Should be able to access "lint" child
	lintVal, err := cv.Attr("lint")
	if err != nil {
		t.Fatalf("Attr('lint') error: %v", err)
	}

	// lint should also be a ConfigValue
	lintCV, ok := lintVal.(*ConfigValue)
	if !ok {
		t.Fatalf("lint value type = %T, want *ConfigValue", lintVal)
	}

	// Should be able to access "go" from lint
	goVal, err := lintCV.Attr("go")
	if err != nil {
		t.Fatalf("lint.Attr('go') error: %v", err)
	}

	goCV, ok := goVal.(*ConfigValue)
	if !ok {
		t.Fatalf("go value type = %T, want *ConfigValue", goVal)
	}

	// Should be able to access "enabled" from go
	enabledVal, err := goCV.Attr("enabled")
	if err != nil {
		t.Fatalf("go.Attr('enabled') error: %v", err)
	}
	if got, ok := enabledVal.(starlark.Bool); !ok || got != starlark.True {
		t.Errorf("enabled = %v, want True", enabledVal)
	}
}

func TestGoToStarlarkReflect_Nil(t *testing.T) {
	val, err := goToStarlarkReflect(nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if val != starlark.None {
		t.Errorf("got %v, want None", val)
	}
}

func TestGoToStarlarkReflect_Primitives(t *testing.T) {
	tests := []struct {
		input interface{}
		check func(starlark.Value) bool
	}{
		{true, func(v starlark.Value) bool { b, ok := v.(starlark.Bool); return ok && b == starlark.True }},
		{false, func(v starlark.Value) bool { b, ok := v.(starlark.Bool); return ok && b == starlark.False }},
		{42, func(v starlark.Value) bool { i, ok := v.(starlark.Int); return ok && i.String() == "42" }},
		{3.14, func(v starlark.Value) bool { f, ok := v.(starlark.Float); return ok && float64(f) == 3.14 }},
		{"test", func(v starlark.Value) bool { s, ok := v.(starlark.String); return ok && string(s) == "test" }},
	}

	for _, tt := range tests {
		val, err := goToStarlarkReflect(tt.input)
		if err != nil {
			t.Errorf("goToStarlarkReflect(%v) error: %v", tt.input, err)
			continue
		}
		if !tt.check(val) {
			t.Errorf("goToStarlarkReflect(%v) = %v (%T), check failed", tt.input, val, val)
		}
	}
}

func TestMapToStarlarkReflect(t *testing.T) {
	m := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	val, err := goToStarlarkReflect(m)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	dict, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("got %T, want *starlark.Dict", val)
	}
	if dict.Len() != 2 {
		t.Errorf("dict len = %d, want 2", dict.Len())
	}
}

func TestSliceToStarlark(t *testing.T) {
	s := []int{1, 2, 3}

	val, err := goToStarlarkReflect(s)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	list, ok := val.(*starlark.List)
	if !ok {
		t.Fatalf("got %T, want *starlark.List", val)
	}
	if list.Len() != 3 {
		t.Errorf("list len = %d, want 3", list.Len())
	}
}
