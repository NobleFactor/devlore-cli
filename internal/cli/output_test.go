// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/output"
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

func TestAddOutputFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var opts output.Options
	AddOutputFlags(cmd, &opts)

	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("expected --format flag to be added")
	}
	if formatFlag.DefValue != "json" {
		t.Errorf("expected --format default 'json', got %q", formatFlag.DefValue)
	}

	filterFlag := cmd.Flags().Lookup("filter")
	if filterFlag == nil {
		t.Fatal("expected --filter flag to be added")
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
