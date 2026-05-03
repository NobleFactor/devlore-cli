// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestJSONFormatterEncodesPrimitives(t *testing.T) {

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"int", 42},
		{"bool true", true},
		{"bool false", false},
		{"float", 3.14},
		{"nil", nil},
		{"slice", []string{"a", "b", "c"}},
		{"map", map[string]int{"k": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {

			var buf bytes.Buffer
			if err := (JSONFormatter{}).Format(tc.value, &buf); err != nil {
				t.Fatalf("Format: %v", err)
			}

			// Decode the bytes back and compare structurally.
			var got any
			if err := json.NewDecoder(&buf).Decode(&got); err != nil {
				t.Fatalf("Decode: %v", err)
			}

			// JSON decodes numbers as float64. Re-encode tc.value the same way for comparison.
			var want any
			wantBytes, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if err := json.Unmarshal(wantBytes, &want); err != nil {
				t.Fatalf("re-decode: %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("round-trip = %v, want %v", got, want)
			}
		})
	}
}

func TestJSONFormatterEncodesStruct(t *testing.T) {

	type point struct {
		X int    `json:"x"`
		Y int    `json:"y"`
		L string `json:"label"`
	}

	value := point{X: 1, Y: 2, L: "p"}

	var buf bytes.Buffer
	if err := (JSONFormatter{}).Format(value, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	got := buf.String()
	for _, want := range []string{`"x": 1`, `"y": 2`, `"label": "p"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %q", want, got)
		}
	}
}

func TestJSONFormatterUsesIndent(t *testing.T) {

	var buf bytes.Buffer
	if err := (JSONFormatter{}).Format(map[string]int{"k": 1}, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	got := buf.String()
	// Two-space indent on nested fields produces "\n  " before the key.
	if !strings.Contains(got, "\n  \"k\"") {
		t.Errorf("expected two-space indent; got %q", got)
	}
}
