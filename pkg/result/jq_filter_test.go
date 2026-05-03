// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"reflect"
	"strings"
	"testing"
)

// region Construction

func TestNewJQFilterParsesValidExpression(t *testing.T) {

	got, err := NewJQFilter(".name")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}
	if got == nil {
		t.Fatal("NewJQFilter returned nil filter")
	}
}

func TestNewJQFilterReportsParseError(t *testing.T) {

	_, err := NewJQFilter("(((")
	if err == nil {
		t.Fatal("expected parse error for malformed jq; got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error text = %q, want substring 'parse'", err.Error())
	}
}

// endregion

// region Apply — happy paths

func TestJQFilterExtractsField(t *testing.T) {

	f, err := NewJQFilter(".name")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	got, err := f.Apply(map[string]any{"name": "alice", "age": 30})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "alice" {
		t.Errorf("Apply = %v, want %q", got, "alice")
	}
}

func TestJQFilterSelectsArrayElements(t *testing.T) {

	f, err := NewJQFilter(`.[] | select(.kind == "file")`)
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	in := []map[string]any{
		{"kind": "file", "name": "a"},
		{"kind": "dir", "name": "b"},
		{"kind": "file", "name": "c"},
	}

	got, err := f.Apply(in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	results, ok := got.([]any)
	if !ok {
		t.Fatalf("Apply returned %T, want []any", got)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for i, want := range []string{"a", "c"} {
		row, ok := results[i].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] is %T, want map[string]any", i, results[i])
		}
		if row["name"] != want {
			t.Errorf("results[%d].name = %v, want %q", i, row["name"], want)
		}
	}
}

func TestJQFilterAppliesToStructInputViaJSONTags(t *testing.T) {

	type person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	f, err := NewJQFilter(".age")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	got, err := f.Apply(person{Name: "alice", Age: 30})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Numbers come out as int64 (UseNumber + Int64 conversion).
	if got != int64(30) {
		t.Errorf("Apply = %v (%T), want int64(30)", got, got)
	}
}

func TestJQFilterEmptyResultReturnsNil(t *testing.T) {

	f, err := NewJQFilter(`.[] | select(.k == "z")`)
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	got, err := f.Apply([]map[string]any{{"k": "a"}, {"k": "b"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != nil {
		t.Errorf("Apply = %v, want nil", got)
	}
}

func TestJQFilterMultiResultReturnsSlice(t *testing.T) {

	f, err := NewJQFilter(".[]")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	got, err := f.Apply([]any{1, 2, 3})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []any{int64(1), int64(2), int64(3)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

// endregion

// region Apply — execution errors

func TestJQFilterReportsExecutionError(t *testing.T) {

	// Indexing into a string with a string key is a jq runtime error.
	f, err := NewJQFilter(`.foo`)
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	_, err = f.Apply(42)
	if err == nil {
		t.Fatal("expected execution error for non-object indexing; got nil")
	}
	if !strings.Contains(err.Error(), "execute") {
		t.Errorf("error text = %q, want substring 'execute'", err.Error())
	}
}

// endregion

// region Apply — pass-through

func TestJQFilterIdentityPassesThroughInputShape(t *testing.T) {

	f, err := NewJQFilter(".")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	in := map[string]any{"k": "v"}
	got, err := f.Apply(in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !reflect.DeepEqual(got, in) {
		t.Errorf("identity Apply = %v, want %v", got, in)
	}
}

func TestJQFilterAppliesToNil(t *testing.T) {

	f, err := NewJQFilter(".")
	if err != nil {
		t.Fatalf("NewJQFilter: %v", err)
	}

	got, err := f.Apply(nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != nil {
		t.Errorf("Apply(nil) = %v, want nil", got)
	}
}

// endregion
