// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLFormatterEncodesPrimitives(t *testing.T) {

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"int", 42},
		{"bool true", true},
		{"bool false", false},
		{"float", 3.14},
		{"slice", []string{"a", "b", "c"}},
		{"map", map[string]int{"k": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {

			var buf bytes.Buffer
			if err := (YAMLFormatter{}).Format(tc.value, &buf); err != nil {
				t.Fatalf("Format: %v", err)
			}

			// Decode the bytes back and compare structurally.
			var got any
			if err := yaml.NewDecoder(&buf).Decode(&got); err != nil {
				t.Fatalf("Decode: %v", err)
			}

			// YAML decodes maps as map[string]any (in this lib's default mode), so re-encode for
			// structural compare.
			var want any
			wantBytes, err := yaml.Marshal(tc.value)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if err := yaml.Unmarshal(wantBytes, &want); err != nil {
				t.Fatalf("re-decode: %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("round-trip = %v, want %v", got, want)
			}
		})
	}
}

func TestYAMLFormatterEncodesStruct(t *testing.T) {

	type point struct {
		X int    `yaml:"x"`
		Y int    `yaml:"y"`
		L string `yaml:"label"`
	}

	value := point{X: 1, Y: 2, L: "p"}

	var buf bytes.Buffer
	if err := (YAMLFormatter{}).Format(value, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	got := buf.String()
	// yaml.v3 quotes "y" because it's a YAML 1.1 boolean alias (yes/no). The encoded form is "y": 2,
	// unquoted on x and label.
	for _, want := range []string{"x: 1", `"y": 2`, "label: p"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %q", want, got)
		}
	}
}
