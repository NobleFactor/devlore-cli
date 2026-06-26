// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"
	"time"
)

// region STRUCT / TEXT CONVERSION TESTS

// convertInner and convertOuter exercise nested struct hydration from the decoded object maps a codec reloads.
type convertInner struct {
	Tag string `json:"tag"`
}

type convertOuter struct {
	Name  string         `json:"name"`
	Count int            `json:"count"`
	Inner convertInner   `json:"inner"`
	Tags  []string       `json:"tags"`
	Refs  []convertInner `json:"refs"`
}

// TestConvert_HydratesStructFromMap reconstructs a nested struct from a map[string]any, with an int from a JSON
// float64 and slices of both scalars and structs.
func TestConvert_HydratesStructFromMap(t *testing.T) {

	source := map[string]any{
		"name":  "x",
		"count": float64(3),
		"inner": map[string]any{"tag": "t"},
		"tags":  []any{"a", "b"},
		"refs":  []any{map[string]any{"tag": "r1"}, map[string]any{"tag": "r2"}},
	}

	got, err := Convert(nil, source, reflect.TypeFor[convertOuter]())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	want := convertOuter{
		Name:  "x",
		Count: 3,
		Inner: convertInner{Tag: "t"},
		Tags:  []string{"a", "b"},
		Refs:  []convertInner{{Tag: "r1"}, {Tag: "r2"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Convert = %#v, want %#v", got, want)
	}
}

// TestConvert_HydratesPointerToStruct returns a *struct when the target is a pointer.
func TestConvert_HydratesPointerToStruct(t *testing.T) {

	got, err := Convert(nil, map[string]any{"tag": "t"}, reflect.TypeFor[*convertInner]())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if want := (&convertInner{Tag: "t"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Convert = %#v, want %#v", got, want)
	}
}

// TestConvert_HydratesSliceOfStructs reconstructs []struct from []any of maps — the struct gap that blocked slices.
func TestConvert_HydratesSliceOfStructs(t *testing.T) {

	source := []any{map[string]any{"tag": "1"}, map[string]any{"tag": "2"}}

	got, err := Convert(nil, source, reflect.TypeFor[[]convertInner]())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if want := []convertInner{{Tag: "1"}, {Tag: "2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Convert = %#v, want %#v", got, want)
	}
}

// TestConvert_HydratesMapOfStructs reconstructs map[string]struct from a decoded object of objects.
func TestConvert_HydratesMapOfStructs(t *testing.T) {

	source := map[string]any{"a": map[string]any{"tag": "1"}, "b": map[string]any{"tag": "2"}}

	got, err := Convert(nil, source, reflect.TypeFor[map[string]convertInner]())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	if want := (map[string]convertInner{"a": {Tag: "1"}, "b": {Tag: "2"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Convert = %#v, want %#v", got, want)
	}
}

// TestConvert_TextUnmarshalerFromString reconstructs a time.Time from its reloaded RFC 3339 string.
func TestConvert_TextUnmarshalerFromString(t *testing.T) {

	got, err := Convert(nil, "2020-01-02T03:04:05Z", reflect.TypeFor[time.Time]())
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if gotTime, ok := got.(time.Time); !ok || !gotTime.Equal(want) {
		t.Fatalf("Convert = %v (%T), want %v", got, got, want)
	}
}

// endregion
