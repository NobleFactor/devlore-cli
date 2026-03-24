// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"testing"
)

type AccessorTestStruct struct {
	ConfigElement
	Enabled     bool
	Name        string
	Count       int
	Score       float64
	Items       []string
	Data        map[string]interface{}
	SkipModTidy bool // Tests snake_case conversion
	Nested      NestedStruct
}

type NestedStruct struct {
	Value string
}

func newTestAccessor() *ConfigAccessor {
	s := &AccessorTestStruct{
		Enabled:     true,
		Name:        "test",
		Count:       42,
		Score:       3.14,
		Items:       []string{"a", "b", "c"},
		Data:        map[string]interface{}{"key": "value"},
		SkipModTidy: true,
		Nested:      NestedStruct{Value: "nested"},
	}
	return NewAccessor(s)
}

func TestNewAccessor_Pointer(t *testing.T) {
	s := &AccessorTestStruct{Name: "test"}
	acc := NewAccessor(s)
	if !acc.IsValid() {
		t.Error("accessor should be valid for pointer")
	}
}

func TestNewAccessor_Value(t *testing.T) {
	s := AccessorTestStruct{Name: "test"}
	acc := NewAccessor(s)
	if !acc.IsValid() {
		t.Error("accessor should be valid for value")
	}
}

func TestConfigAccessor_Bool(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.Bool("enabled"); got != true {
		t.Errorf("Bool('enabled') = %v, want true", got)
	}

	// Non-existent field
	if got := acc.Bool("missing"); got != false {
		t.Errorf("Bool('missing') = %v, want false", got)
	}

	// Wrong type field
	if got := acc.Bool("name"); got != false {
		t.Errorf("Bool('name') = %v, want false", got)
	}
}

func TestConfigAccessor_BoolOr(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.BoolOr("missing", true); got != true {
		t.Errorf("BoolOr('missing', true) = %v, want true", got)
	}
}

func TestConfigAccessor_String(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.String("name"); got != "test" {
		t.Errorf("String('name') = %q, want 'test'", got)
	}

	// Non-existent field
	if got := acc.String("missing"); got != "" {
		t.Errorf("String('missing') = %q, want ''", got)
	}
}

func TestConfigAccessor_StringOr(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.StringOr("missing", "default"); got != "default" {
		t.Errorf("StringOr('missing', 'default') = %q, want 'default'", got)
	}

	// Empty string field should return default
	s := &AccessorTestStruct{Name: ""}
	acc2 := NewAccessor(s)
	if got := acc2.StringOr("name", "default"); got != "default" {
		t.Errorf("StringOr('name', 'default') for empty = %q, want 'default'", got)
	}
}

func TestConfigAccessor_Int(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.Int("count"); got != 42 {
		t.Errorf("Int('count') = %d, want 42", got)
	}

	// Non-existent field
	if got := acc.Int("missing"); got != 0 {
		t.Errorf("Int('missing') = %d, want 0", got)
	}
}

func TestConfigAccessor_IntOr(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.IntOr("missing", 99); got != 99 {
		t.Errorf("IntOr('missing', 99) = %d, want 99", got)
	}
}

func TestConfigAccessor_Float(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.Float("score"); got != 3.14 {
		t.Errorf("Float('score') = %f, want 3.14", got)
	}

	// Non-existent field
	if got := acc.Float("missing"); got != 0 {
		t.Errorf("Float('missing') = %f, want 0", got)
	}
}

func TestConfigAccessor_StringSlice(t *testing.T) {
	acc := newTestAccessor()

	got := acc.StringSlice("items")
	if len(got) != 3 {
		t.Fatalf("StringSlice('items') len = %d, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("StringSlice('items') = %v, want [a b c]", got)
	}

	// Non-existent field
	if got := acc.StringSlice("missing"); got != nil {
		t.Errorf("StringSlice('missing') = %v, want nil", got)
	}
}

func TestConfigAccessor_StringSliceOr(t *testing.T) {
	acc := newTestAccessor()

	defaultVal := []string{"x", "y"}
	if got := acc.StringSliceOr("missing", defaultVal); len(got) != 2 {
		t.Errorf("StringSliceOr('missing', default) = %v, want %v", got, defaultVal)
	}
}

func TestConfigAccessor_Map(t *testing.T) {
	acc := newTestAccessor()

	got := acc.Map("data")
	if got == nil {
		t.Fatal("Map('data') = nil, want map")
	}
	if got["key"] != "value" {
		t.Errorf("Map('data')['key'] = %v, want 'value'", got["key"])
	}

	// Non-existent field
	if got := acc.Map("missing"); got != nil {
		t.Errorf("Map('missing') = %v, want nil", got)
	}
}

func TestConfigAccessor_Struct(t *testing.T) {
	acc := newTestAccessor()

	nested := acc.Struct("nested")
	if !nested.IsValid() {
		t.Fatal("Struct('nested') should be valid")
	}
	if got := nested.String("value"); got != "nested" {
		t.Errorf("nested.String('value') = %q, want 'nested'", got)
	}

	// Non-existent field
	invalid := acc.Struct("missing")
	if invalid.IsValid() {
		t.Error("Struct('missing') should be invalid")
	}
}

func TestConfigAccessor_Get(t *testing.T) {
	acc := newTestAccessor()

	if got := acc.Get("name"); got != "test" {
		t.Errorf("Get('name') = %v, want 'test'", got)
	}
	if got := acc.Get("count"); got != 42 {
		t.Errorf("Get('count') = %v, want 42", got)
	}
	if got := acc.Get("missing"); got != nil {
		t.Errorf("Get('missing') = %v, want nil", got)
	}
}

func TestConfigAccessor_Has(t *testing.T) {
	acc := newTestAccessor()

	if !acc.Has("name") {
		t.Error("Has('name') should be true")
	}
	if acc.Has("missing") {
		t.Error("Has('missing') should be false")
	}
}

func TestConfigAccessor_Fields(t *testing.T) {
	acc := newTestAccessor()

	fields := acc.Fields()
	if len(fields) == 0 {
		t.Fatal("Fields() should return field names")
	}

	// Check some expected fields are present
	fieldMap := make(map[string]bool)
	for _, f := range fields {
		fieldMap[f] = true
	}

	expected := []string{"enabled", "name", "count", "score", "items", "data", "skip_mod_tidy", "nested"}
	for _, exp := range expected {
		if !fieldMap[exp] {
			t.Errorf("Fields() should contain %q", exp)
		}
	}

	// ConfigElement should not be in the list
	if fieldMap["config_element"] {
		t.Error("Fields() should not contain 'config_element'")
	}
}

func TestConfigAccessor_SnakeCaseConversion(t *testing.T) {
	acc := newTestAccessor()

	// Field is SkipModTidy, should be accessible as skip_mod_tidy
	if got := acc.Bool("skip_mod_tidy"); got != true {
		t.Errorf("Bool('skip_mod_tidy') = %v, want true", got)
	}
}

func TestConfigAccessor_InvalidAccessor(t *testing.T) {
	acc := &ConfigAccessor{} // empty, invalid

	if acc.IsValid() {
		t.Error("empty accessor should be invalid")
	}
	if acc.Bool("any") != false {
		t.Error("Bool on invalid should return false")
	}
	if acc.String("any") != "" {
		t.Error("String on invalid should return ''")
	}
	if acc.Fields() != nil {
		t.Error("Fields on invalid should return nil")
	}
}
