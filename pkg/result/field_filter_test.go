// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"reflect"
	"strings"
	"testing"
)

// region Construction

func TestNewFieldFilterParsesValidExpressions(t *testing.T) {

	got, err := NewFieldFilter("name=alice", "age=30")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}
	if len(got.predicates) != 2 {
		t.Errorf("predicates count = %d, want 2", len(got.predicates))
	}
}

func TestNewFieldFilterAcceptsEmptyValues(t *testing.T) {

	got, err := NewFieldFilter("name=")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}
	if len(got.predicates) != 1 || got.predicates[0].value != "" {
		t.Errorf("predicates = %v, want [{name }]", got.predicates)
	}
}

func TestNewFieldFilterSkipsEmptyExpressions(t *testing.T) {

	got, err := NewFieldFilter("", "  ", "name=alice")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}
	if len(got.predicates) != 1 {
		t.Errorf("predicates count = %d, want 1", len(got.predicates))
	}
}

func TestNewFieldFilterErrorsOnMissingEquals(t *testing.T) {

	_, err := NewFieldFilter("name")
	if err == nil {
		t.Fatal("expected parse error; got nil")
	}
	if !strings.Contains(err.Error(), "missing '='") {
		t.Errorf("error text = %q, want substring \"missing '='\"", err.Error())
	}
}

func TestNewFieldFilterErrorsOnEmptyFieldName(t *testing.T) {

	_, err := NewFieldFilter("=value")
	if err == nil {
		t.Fatal("expected parse error; got nil")
	}
	if !strings.Contains(err.Error(), "empty field name") {
		t.Errorf("error text = %q, want substring 'empty field name'", err.Error())
	}
}

// endregion

// region Apply — slice-of-structs

func TestFieldFilterFiltersSliceOfStructs(t *testing.T) {

	type person struct {
		Name string
		Age  int
	}

	rows := []person{
		{Name: "alice", Age: 30},
		{Name: "bob", Age: 25},
		{Name: "alice", Age: 40},
	}

	f, err := NewFieldFilter("Name=alice")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []person{
		{Name: "alice", Age: 30},
		{Name: "alice", Age: 40},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestFieldFilterANDsMultiplePredicates(t *testing.T) {

	type person struct {
		Name string
		Age  int
	}

	rows := []person{
		{Name: "alice", Age: 30},
		{Name: "alice", Age: 40},
		{Name: "bob", Age: 30},
	}

	f, err := NewFieldFilter("Name=alice", "Age=30")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []person{{Name: "alice", Age: 30}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestFieldFilterHonorsCSVTagInFieldLookup(t *testing.T) {

	type row struct {
		Name string `csv:"name"`
	}

	rows := []row{{Name: "x"}, {Name: "y"}}

	f, err := NewFieldFilter("name=x")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []row{{Name: "x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestFieldFilterStringComparesNumeric(t *testing.T) {

	type row struct{ Age int }
	rows := []row{{Age: 30}, {Age: 40}}

	f, err := NewFieldFilter("Age=30")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []row{{Age: 30}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

// endregion

// region Apply — slice-of-maps

func TestFieldFilterFiltersSliceOfMaps(t *testing.T) {

	rows := []map[string]any{
		{"k": "a", "v": 1},
		{"k": "b", "v": 2},
		{"k": "a", "v": 3},
	}

	f, err := NewFieldFilter("k=a")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []map[string]any{
		{"k": "a", "v": 1},
		{"k": "a", "v": 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestFieldFilterAbsentKeyMeansNoMatch(t *testing.T) {

	rows := []map[string]any{
		{"k": "a"},
		{"other": "z"}, // no "k" key
	}

	f, err := NewFieldFilter("k=a")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []map[string]any{{"k": "a"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

// endregion

// region Apply — pass-through and edge cases

func TestFieldFilterEmptyPredicatePassesThrough(t *testing.T) {

	f, err := NewFieldFilter()
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	in := "anything goes"
	got, err := f.Apply(in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != in {
		t.Errorf("Apply = %v, want %v (pass-through)", got, in)
	}
}

func TestFieldFilterNilInputPassesThrough(t *testing.T) {

	f, err := NewFieldFilter("k=v")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != nil {
		t.Errorf("Apply(nil) = %v, want nil", got)
	}
}

func TestFieldFilterRejectsScalar(t *testing.T) {

	f, err := NewFieldFilter("k=v")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	_, err = f.Apply(42)
	if err == nil {
		t.Fatal("expected error for scalar input; got nil")
	}
	if !strings.Contains(err.Error(), "expected slice or array") {
		t.Errorf("error text = %q, want substring 'expected slice or array'", err.Error())
	}
}

func TestFieldFilterRejectsSliceOfScalars(t *testing.T) {

	f, err := NewFieldFilter("k=v")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	_, err = f.Apply([]int{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for slice of scalars; got nil")
	}
	if !strings.Contains(err.Error(), "not struct or map") {
		t.Errorf("error text = %q, want substring 'not struct or map'", err.Error())
	}
}

func TestFieldFilterEmptyResultReturnsEmptySlice(t *testing.T) {

	type row struct{ K string }
	rows := []row{{K: "a"}, {K: "b"}}

	f, err := NewFieldFilter("K=z")
	if err != nil {
		t.Fatalf("NewFieldFilter: %v", err)
	}

	got, err := f.Apply(rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	gotRows, ok := got.([]row)
	if !ok {
		t.Fatalf("Apply returned %T, want []row", got)
	}
	if len(gotRows) != 0 {
		t.Errorf("Apply returned %d rows, want 0", len(gotRows))
	}
}

// endregion
