// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

func TestAddOutputFlagsBindsTheCommonSet(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	addOutputFlags(cmd, &opts)

	for _, name := range []string{"filter", "jq", "output", "store"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("--%s is not bound; the common set is registered in full or not at all", name)
		}
	}
}

func TestAddOutputFlagsOutputDefaultsToJSON(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	addOutputFlags(cmd, &opts)

	flag := cmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("--output not bound")
	}
	if flag.DefValue != "json" {
		t.Errorf("--output default = %q, want %q", flag.DefValue, "json")
	}
}

func TestBuildPipelineProducesPipelineForKnownFormat(t *testing.T) {

	var buf bytes.Buffer
	pipeline, err := BuildPipeline(SinkOptions{Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	if pipeline == nil {
		t.Errorf("BuildPipeline returned nil, want *result.Pipeline")
	}
}

func TestBuildPipelineEmitsJSONForJSONFormat(t *testing.T) {

	var buf bytes.Buffer
	sink, err := BuildPipeline(SinkOptions{Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	if err := sink.Emit(map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"k": "v"`) {
		t.Errorf("output = %q, want substring \"k\": \"v\"", got)
	}
}

func TestBuildPipelineAppliesFieldFilterBeforeJQ(t *testing.T) {

	var buf bytes.Buffer
	sink, err := BuildPipeline(
		SinkOptions{
			Format:  "json",
			Filters: []string{"kind=file"},
			JQ:      ".[].name",
		},
		&buf,
	)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	in := []map[string]any{
		{"kind": "file", "name": "a"},
		{"kind": "dir", "name": "b"},
		{"kind": "file", "name": "c"},
	}
	if err := sink.Emit(in); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	got := buf.String()
	for _, want := range []string{`"a"`, `"c"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; full output:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"b"`) {
		t.Errorf("output should have filtered out b; got:\n%s", got)
	}
}

func TestBuildPipelineReportsUnknownFormat(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "xml"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown format; got nil")
	}
	if !strings.Contains(err.Error(), "unknown formatter") {
		t.Errorf("error text = %q, want substring 'unknown formatter'", err.Error())
	}
}

func TestBuildPipelineReportsFilterParseError(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "json", Filters: []string{"nope"}}, &buf)
	if err == nil {
		t.Fatal("expected field-parse error; got nil")
	}
}

func TestBuildPipelineReportsJQParseError(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "json", JQ: "((("}, &buf)
	if err == nil {
		t.Fatal("expected jq-parse error; got nil")
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
