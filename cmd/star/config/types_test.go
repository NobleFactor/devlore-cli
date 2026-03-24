// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"reflect"
	"testing"
)

func TestGenerateConfigType_SimpleFields(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"name":    "string",
			"count":   "int",
		},
	}

	typ := generateConfigType(spec)

	// Should be a struct
	if typ.Kind() != reflect.Struct {
		t.Fatalf("generated type kind = %v, want struct", typ.Kind())
	}

	// Should have ConfigElement embedded + 3 fields
	if typ.NumField() != 4 {
		t.Errorf("NumField() = %d, want 4", typ.NumField())
	}

	// Check ConfigElement is embedded
	field, ok := typ.FieldByName("ConfigElement")
	if !ok {
		t.Error("should have embedded ConfigElement")
	}
	if !field.Anonymous {
		t.Error("ConfigElement should be anonymous (embedded)")
	}

	// Check fields exist with correct types
	testCases := []struct {
		name     string
		wantKind reflect.Kind
	}{
		{"Enabled", reflect.Bool},
		{"Name", reflect.String},
		{"Count", reflect.Int},
	}

	for _, tc := range testCases {
		field, ok := typ.FieldByName(tc.name)
		if !ok {
			t.Errorf("field %s not found", tc.name)
			continue
		}
		if field.Type.Kind() != tc.wantKind {
			t.Errorf("field %s kind = %v, want %v", tc.name, field.Type.Kind(), tc.wantKind)
		}
	}
}

func TestGenerateConfigType_SliceField(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"items": "[]string",
		},
	}

	typ := generateConfigType(spec)

	field, ok := typ.FieldByName("Items")
	if !ok {
		t.Fatal("field Items not found")
	}
	if field.Type.Kind() != reflect.Slice {
		t.Errorf("Items kind = %v, want slice", field.Type.Kind())
	}
	if field.Type.Elem().Kind() != reflect.String {
		t.Errorf("Items elem kind = %v, want string", field.Type.Elem().Kind())
	}
}

func TestGenerateConfigType_MapField(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"data": "map[string]string",
		},
	}

	typ := generateConfigType(spec)

	field, ok := typ.FieldByName("Data")
	if !ok {
		t.Fatal("field Data not found")
	}
	if field.Type.Kind() != reflect.Map {
		t.Errorf("Data kind = %v, want map", field.Type.Kind())
	}
	if field.Type.Key().Kind() != reflect.String {
		t.Errorf("Data key kind = %v, want string", field.Type.Key().Kind())
	}
	if field.Type.Elem().Kind() != reflect.String {
		t.Errorf("Data elem kind = %v, want string", field.Type.Elem().Kind())
	}
}

func TestGenerateConfigType_NestedType(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"pattern": "Pattern",
		},
		Nested: map[string]ConfigSpec{
			"Pattern": {
				Fields: map[string]string{
					"match":   "string",
					"replace": "string",
				},
			},
		},
	}

	typ := generateConfigType(spec)

	field, ok := typ.FieldByName("Pattern")
	if !ok {
		t.Fatal("field Pattern not found")
	}
	if field.Type.Kind() != reflect.Struct {
		t.Errorf("Pattern kind = %v, want struct", field.Type.Kind())
	}

	// Check nested struct has correct fields
	matchField, ok := field.Type.FieldByName("Match")
	if !ok {
		t.Error("nested Pattern should have Match field")
	}
	if matchField.Type.Kind() != reflect.String {
		t.Errorf("Match kind = %v, want string", matchField.Type.Kind())
	}
}

func TestGenerateConfigType_MapOfNestedType(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"patterns": "map[string]Pattern",
		},
		Nested: map[string]ConfigSpec{
			"Pattern": {
				Fields: map[string]string{
					"match":   "string",
					"replace": "string",
				},
			},
		},
	}

	typ := generateConfigType(spec)

	field, ok := typ.FieldByName("Patterns")
	if !ok {
		t.Fatal("field Patterns not found")
	}
	if field.Type.Kind() != reflect.Map {
		t.Fatalf("Patterns kind = %v, want map", field.Type.Kind())
	}
	if field.Type.Elem().Kind() != reflect.Struct {
		t.Errorf("Patterns elem kind = %v, want struct", field.Type.Elem().Kind())
	}
}

func TestGetOrCreateType_Caching(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
		},
	}

	typ1 := getOrCreateType("test.cache", spec)
	typ2 := getOrCreateType("test.cache", spec)

	// Should return the same type (pointer equality)
	if typ1 != typ2 {
		t.Error("getOrCreateType should return cached type")
	}
}

func TestNewConfigInstance_Defaults(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"name":    "string",
			"count":   "int",
		},
	}
	defaults := map[string]interface{}{
		"enabled": true,
		"name":    "test",
		"count":   42,
	}

	typ := generateConfigType(spec)
	instance := newConfigInstance(typ, defaults)

	// Get values via reflection
	rv := reflect.ValueOf(instance).Elem()

	if got := rv.FieldByName("Enabled").Bool(); got != true {
		t.Errorf("Enabled = %v, want true", got)
	}
	if got := rv.FieldByName("Name").String(); got != "test" {
		t.Errorf("Name = %q, want 'test'", got)
	}
	if got := rv.FieldByName("Count").Int(); got != 42 {
		t.Errorf("Count = %d, want 42", got)
	}
}

func TestNewConfigInstance_SliceDefault(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"items": "[]string",
		},
	}
	defaults := map[string]interface{}{
		"items": []string{"a", "b", "c"},
	}

	typ := generateConfigType(spec)
	instance := newConfigInstance(typ, defaults)

	rv := reflect.ValueOf(instance).Elem()
	items := rv.FieldByName("Items")

	if items.Len() != 3 {
		t.Errorf("Items len = %d, want 3", items.Len())
	}
	if items.Index(0).String() != "a" {
		t.Errorf("Items[0] = %q, want 'a'", items.Index(0).String())
	}
}

func TestNewConfigInstance_MapDefault(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"data": "map[string]string",
		},
	}
	defaults := map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	typ := generateConfigType(spec)
	instance := newConfigInstance(typ, defaults)

	rv := reflect.ValueOf(instance).Elem()
	data := rv.FieldByName("Data")

	if data.Len() != 2 {
		t.Errorf("Data len = %d, want 2", data.Len())
	}
}

func TestNewConfigInstance_NestedStructDefault(t *testing.T) {
	spec := ConfigSpec{
		Fields: map[string]string{
			"patterns": "map[string]Pattern",
		},
		Nested: map[string]ConfigSpec{
			"Pattern": {
				Fields: map[string]string{
					"match":   "string",
					"replace": "string",
				},
			},
		},
	}
	defaults := map[string]interface{}{
		"patterns": map[string]interface{}{
			"*.go": map[string]interface{}{
				"match":   "// Copyright",
				"replace": "// SPDX",
			},
		},
	}

	typ := generateConfigType(spec)
	instance := newConfigInstance(typ, defaults)

	rv := reflect.ValueOf(instance).Elem()
	patterns := rv.FieldByName("Patterns")

	if patterns.Len() != 1 {
		t.Fatalf("Patterns len = %d, want 1", patterns.Len())
	}

	// Get the pattern for "*.go"
	goPattern := patterns.MapIndex(reflect.ValueOf("*.go"))
	if !goPattern.IsValid() {
		t.Fatal("patterns['*.go'] not found")
	}

	match := goPattern.FieldByName("Match").String()
	if match != "// Copyright" {
		t.Errorf("Match = %q, want '// Copyright'", match)
	}
}

func TestResolveType_Primitives(t *testing.T) {
	tests := []struct {
		typeName string
		wantKind reflect.Kind
	}{
		{"bool", reflect.Bool},
		{"string", reflect.String},
		{"int", reflect.Int},
		{"float64", reflect.Float64},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			typ := resolveType(tt.typeName, nil)
			if typ.Kind() != tt.wantKind {
				t.Errorf("resolveType(%q) kind = %v, want %v", tt.typeName, typ.Kind(), tt.wantKind)
			}
		})
	}
}

func TestResolveType_Unknown(t *testing.T) {
	typ := resolveType("UnknownType", nil)
	// Should fall back to interface{}
	if typ.Kind() != reflect.Interface {
		t.Errorf("resolveType(UnknownType) kind = %v, want interface", typ.Kind())
	}
}
