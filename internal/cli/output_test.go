// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitError", ExitError, 1},
		{"ExitUsage", ExitUsage, 64},
		{"ExitDataErr", ExitDataErr, 65},
		{"ExitNoInput", ExitNoInput, 66},
		{"ExitUnavailable", ExitUnavailable, 69},
		{"ExitSoftware", ExitSoftware, 70},
		{"ExitCantCreate", ExitCantCreate, 73},
		{"ExitIOErr", ExitIOErr, 74},
		{"ExitNoPerm", ExitNoPerm, 77},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("expected %s = %d, got %d", tt.name, tt.expected, tt.code)
			}
		})
	}
}

func TestExitWith(t *testing.T) {
	baseErr := errors.New("file not found")
	err := ExitWith(ExitNoInput, baseErr)

	if err == nil {
		t.Fatal("expected error to be non-nil")
	}

	// Should preserve the original error message
	if err.Error() != "file not found" {
		t.Errorf("expected error message 'file not found', got %q", err.Error())
	}

	// Should unwrap to base error
	if !errors.Is(err, baseErr) {
		t.Error("expected wrapped error to unwrap to base error")
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil error", nil, ExitOK},
		{"plain error", errors.New("generic error"), ExitError},
		{"ExitNoInput wrapped", ExitWith(ExitNoInput, errors.New("not found")), ExitNoInput},
		{"ExitNoPerm wrapped", ExitWith(ExitNoPerm, errors.New("permission denied")), ExitNoPerm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := ExitCode(tt.err)
			if code != tt.expected {
				t.Errorf("expected exit code %d, got %d", tt.expected, code)
			}
		})
	}
}

func TestOutputFlagsDefaults(t *testing.T) {
	if DefaultFormat != "json" {
		t.Errorf("expected DefaultFormat 'json', got %q", DefaultFormat)
	}
}

func TestAddOutputFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var flags OutputFlags
	AddOutputFlags(cmd, &flags)

	// Check format flag
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag to be added")
	}
	if formatFlag.DefValue != "json" {
		t.Errorf("expected --format default 'json', got %q", formatFlag.DefValue)
	}

	// Check filter flag
	filterFlag := cmd.Flags().Lookup("filter")
	if filterFlag == nil {
		t.Fatal("expected --filter flag to be added")
	}
}

func TestAddMutationFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var flags MutationFlags
	AddMutationFlags(cmd, &flags)

	// Check passthru flag
	passthruFlag := cmd.Flags().Lookup("passthru")
	if passthruFlag == nil {
		t.Fatal("expected --passthru flag to be added")
	}

	// Check format flag
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag to be added")
	}
}

func TestRenderJSON(t *testing.T) {
	data := struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{Name: "test", Version: "1.0.0"}

	var buf bytes.Buffer
	err := Render(&buf, data, OutputFlags{Format: "json"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name": "test"`) {
		t.Errorf("expected JSON output to contain name field, got %q", output)
	}
	if !strings.Contains(output, `"version": "1.0.0"`) {
		t.Errorf("expected JSON output to contain version field, got %q", output)
	}
}

func TestRenderYAML(t *testing.T) {
	data := struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}{Name: "test", Version: "1.0.0"}

	var buf bytes.Buffer
	err := Render(&buf, data, OutputFlags{Format: "yaml"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name: test") {
		t.Errorf("expected YAML output to contain name field, got %q", output)
	}
	if !strings.Contains(output, "version: 1.0.0") {
		t.Errorf("expected YAML output to contain version field, got %q", output)
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
	err := Render(&buf, data, OutputFlags{Format: "table"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	output := buf.String()
	// Check for header
	if !strings.Contains(strings.ToUpper(output), "NAME") {
		t.Errorf("expected table output to contain NAME header, got %q", output)
	}
	// Check for data
	if !strings.Contains(output, "pkg1") {
		t.Errorf("expected table output to contain 'pkg1', got %q", output)
	}
	if !strings.Contains(output, "pkg2") {
		t.Errorf("expected table output to contain 'pkg2', got %q", output)
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
	err := Render(&buf, data, OutputFlags{Format: "{{.Name}}:{{.Version}}"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "test:1.0.0" {
		t.Errorf("expected template output 'test:1.0.0', got %q", output)
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
	err := Render(&buf, data, OutputFlags{Format: "json", Filter: []string{"name=foo"}})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	output := buf.String()
	// Should contain foo entries
	if !strings.Contains(output, "foo") {
		t.Errorf("expected filtered output to contain 'foo', got %q", output)
	}
	// Should not contain bar (filtered out)
	if strings.Contains(output, `"name": "bar"`) {
		t.Errorf("expected filtered output to NOT contain 'bar', got %q", output)
	}
}

func TestRenderMutation(t *testing.T) {
	data := struct {
		Changed bool `json:"changed"`
	}{Changed: true}

	// With passthru=false, should output nothing
	var buf1 bytes.Buffer
	err := RenderMutation(&buf1, data, MutationFlags{Passthru: false})
	if err != nil {
		t.Fatalf("RenderMutation failed: %v", err)
	}
	if buf1.Len() > 0 {
		t.Errorf("expected no output with passthru=false, got %q", buf1.String())
	}

	// With passthru=true, should output
	var buf2 bytes.Buffer
	err = RenderMutation(&buf2, data, MutationFlags{Passthru: true, Format: "json"})
	if err != nil {
		t.Fatalf("RenderMutation failed: %v", err)
	}
	if buf2.Len() == 0 {
		t.Error("expected output with passthru=true")
	}
}

func TestSetProgramName(t *testing.T) {
	original := programName
	defer func() { programName = original }()

	SetProgramName("testprog")
	if programName != "testprog" {
		t.Errorf("expected programName 'testprog', got %q", programName)
	}
}

func TestSetSilent(t *testing.T) {
	original := silent
	defer func() { silent = original }()

	SetSilent(true)
	if !silent {
		t.Error("expected silent to be true")
	}

	SetSilent(false)
	if silent {
		t.Error("expected silent to be false")
	}
}

func TestFailureReturnsError(t *testing.T) {
	err := Failure("test error: %s", "detail")
	if err == nil {
		t.Fatal("expected Failure to return error")
	}

	expected := "test error: detail"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

func TestToSlice(t *testing.T) {
	// Single item should be wrapped
	single := struct{ Name string }{Name: "test"}
	result := toSlice(single)
	if len(result) != 1 {
		t.Errorf("expected single item to produce slice of length 1, got %d", len(result))
	}

	// Slice should be preserved
	slice := []string{"a", "b", "c"}
	result2 := toSlice(slice)
	if len(result2) != 3 {
		t.Errorf("expected slice of length 3, got %d", len(result2))
	}

	// Pointer to slice should work
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

	// Should have 2 fields
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

	// Get by json tag
	val := getFieldValue(item, "name")
	if val != "test" {
		t.Errorf("expected value 'test', got %v", val)
	}

	// Get by field name
	val2 := getFieldValue(item, "Version")
	if val2 != "1.0" {
		t.Errorf("expected value '1.0', got %v", val2)
	}

	// Non-existent field
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
