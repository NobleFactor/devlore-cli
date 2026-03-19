// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderJSON(t *testing.T) {
	data := struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{Name: "test", Version: "1.0.0"}

	var buf bytes.Buffer
	err := Render(&buf, data, Options{Format: "json"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"name": "test"`) {
		t.Errorf("expected JSON output to contain name field, got %q", out)
	}
	if !strings.Contains(out, `"version": "1.0.0"`) {
		t.Errorf("expected JSON output to contain version field, got %q", out)
	}
}

func TestRenderYAML(t *testing.T) {
	data := struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}{Name: "test", Version: "1.0.0"}

	var buf bytes.Buffer
	err := Render(&buf, data, Options{Format: "yaml"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "name: test") {
		t.Errorf("expected YAML output to contain name field, got %q", out)
	}
	if !strings.Contains(out, "version: 1.0.0") {
		t.Errorf("expected YAML output to contain version field, got %q", out)
	}
}

func TestRenderTable(t *testing.T) {
	data := []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{
		{Name: "pkg1", Version: "1.0.0"},
		{Name: "pkg2", Version: "2.0.0"},
	}

	var buf bytes.Buffer
	err := Render(&buf, data, Options{Format: "table"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(strings.ToUpper(out), "NAME") {
		t.Errorf("expected table output to contain NAME header, got %q", out)
	}
	if !strings.Contains(out, "pkg1") {
		t.Errorf("expected table output to contain 'pkg1', got %q", out)
	}
	if !strings.Contains(out, "pkg2") {
		t.Errorf("expected table output to contain 'pkg2', got %q", out)
	}
}

func TestRenderTemplate(t *testing.T) {
	data := []struct {
		Name    string
		Version string
	}{
		{Name: "test", Version: "1.0.0"},
	}

	var buf bytes.Buffer
	err := Render(&buf, data, Options{Format: "{{.Name}}:{{.Version}}"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if out != "test:1.0.0" {
		t.Errorf("expected template output 'test:1.0.0', got %q", out)
	}
}

func TestRenderWithFilter(t *testing.T) {
	data := []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{
		{Name: "foo", Version: "1.0.0"},
		{Name: "bar", Version: "2.0.0"},
		{Name: "foo", Version: "3.0.0"},
	}

	var buf bytes.Buffer
	err := Render(&buf, data, Options{Format: "json", Filter: []string{"name=foo"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "foo") {
		t.Errorf("expected filtered output to contain 'foo', got %q", out)
	}
	if strings.Contains(out, `"name": "bar"`) {
		t.Errorf("expected filtered output to NOT contain 'bar', got %q", out)
	}
}

func TestToSlice(t *testing.T) {
	single := struct{ Name string }{Name: "test"}
	result := toSlice(single)
	if len(result) != 1 {
		t.Errorf("expected single item to produce slice of length 1, got %d", len(result))
	}

	slice := []string{"a", "b", "c"}
	result2 := toSlice(slice)
	if len(result2) != 3 {
		t.Errorf("expected slice of length 3, got %d", len(result2))
	}

	result3 := toSlice(&slice)
	if len(result3) != 3 {
		t.Errorf("expected pointer to slice to produce length 3, got %d", len(result3))
	}
}

func TestGetFieldNames(t *testing.T) {
	type TestStruct struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	item := TestStruct{Name: "test", Version: "1.0"}
	fields := getFieldNames(item)

	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d: %v", len(fields), fields)
	}
}

func TestGetFieldValue(t *testing.T) {
	type TestStruct struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	item := TestStruct{Name: "test", Version: "1.0"}

	val := getFieldValue(item, "name")
	if val != "test" {
		t.Errorf("expected value 'test', got %v", val)
	}

	val2 := getFieldValue(item, "Version")
	if val2 != "1.0" {
		t.Errorf("expected value '1.0', got %v", val2)
	}

	val3 := getFieldValue(item, "nonexistent")
	if val3 != nil {
		t.Errorf("expected nil for non-existent field, got %v", val3)
	}
}

func TestMatchesFilter(t *testing.T) {
	type TestStruct struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	item := TestStruct{Name: "test", Version: "1.0"}

	tests := []struct {
		filter   string
		expected bool
	}{
		{"name=test", true},
		{"name=TEST", true}, // case insensitive
		{"name=other", false},
		{"version=1.0", true},
		{"invalid", true}, // invalid filter (no =) is skipped
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			result := matchesFilter(item, tt.filter)
			if result != tt.expected {
				t.Errorf("matchesFilter(%q) = %v, want %v", tt.filter, result, tt.expected)
			}
		})
	}
}
