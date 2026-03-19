// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/output"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"
)

// =============================================================================
// Exit Codes (BSD sysexits.h)
// =============================================================================

// Exit codes follow BSD sysexits.h conventions for portable process status.
const (
	ExitOK          = 0  // Success
	ExitError       = 1  // Generic error
	ExitUsage       = 64 // Bad CLI syntax
	ExitDataErr     = 65 // Invalid manifest/config
	ExitNoInput     = 66 // File not found
	ExitUnavailable = 69 // Registry unreachable
	ExitSoftware    = 70 // Internal error (bug)
	ExitCantCreate  = 73 // Can't create file/symlink
	ExitIOErr       = 74 // Read/write failure
	ExitNoPerm      = 77 // Permission denied
)

// =============================================================================
// Exit Code Errors
// =============================================================================
//
// Commands return errors via Cobra's RunE. To propagate a specific exit code,
// wrap the error with ExitWith:
//
//	return cli.ExitWith(cli.ExitNoInput, fmt.Errorf("file not found: %s", path))
//	return cli.ExitWith(cli.ExitNoPerm, fmt.Errorf("permission denied: %s", path))
//
// In main(), extract the code with ExitCode:
//
//	if err := cmd.Execute(); err != nil {
//	    os.Exit(cli.ExitCode(err))
//	}
//
// Plain errors (without ExitWith) default to exit code 1 (ExitError).
// Commands only need ExitWith when the distinction matters to callers
// checking $? in scripts.

// exitError wraps an error with a specific exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// ExitWith returns an error that carries a specific exit code.
func ExitWith(code int, err error) error {
	return &exitError{code: code, err: err}
}

// ExitCode extracts the exit code from an error.
// Returns the wrapped code if present, or ExitError (1) for plain errors.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if coded, ok := errors.AsType[*exitError](err); ok {
		return coded.code
	}
	return ExitError
}

// =============================================================================
// Output Flags
// =============================================================================

// AddOutputFlags adds --filter and --format flags to a command.
// The flags populate an output.Options struct for use with output.Render.
//
// Usage:
//
//	var opts output.Options
//	cli.AddOutputFlags(cmd, &opts)
func AddOutputFlags(cmd *cobra.Command, opts *output.Options) {
	cmd.Flags().StringVar(&opts.Format, "format", output.DefaultFormat,
		`Output format: json, table, yaml, or a Go template (e.g. '{{.ReceiverName}}\t{{.Version}}')`)
	cmd.Flags().StringArrayVar(&opts.Filter, "filter", nil,
		`Filter expression: field=value (repeatable, AND logic)`)
}

// =============================================================================
// Status Output Functions
// =============================================================================
//
// Thin wrappers around ui.Provider. All 136 call sites remain unchanged.
// The ui.Provider is the single implementation for both Go callers and
// Starlark immediate receivers.

// statusOutput is the package-level ui.Provider instance.
var statusOutput = &ui.Provider{
	Writer: os.Stderr,
	Color:  true,
}

// SetProgramName sets the program name used in output prefixes.
func SetProgramName(name string) {
	statusOutput.ProgramName = name
}

// SetSilent enables or disables silent mode.
func SetSilent(s bool) {
	statusOutput.Silent = s
}

// AddSilentFlag adds the --silent flag to a root command.
func AddSilentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&statusOutput.Silent, "silent", false,
		`Suppress all status messages (stderr)`)
}

// Note prints an informational message to stderr.
func Note(format string, args ...interface{}) {
	statusOutput.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message to stderr.
func Warn(format string, args ...interface{}) {
	statusOutput.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message to stderr.
// Unlike Failure, this does not return an error—use for non-fatal errors.
func Error(format string, args ...interface{}) {
	statusOutput.Error(fmt.Sprintf(format, args...))
}

// Failure prints an error message to stderr and returns an error.
// Use when the operation cannot continue.
func Failure(format string, args ...interface{}) error {
	return statusOutput.Fail(fmt.Sprintf(format, args...))
}

// Success prints a success message to stderr.
func Success(format string, args ...interface{}) {
	statusOutput.Success(fmt.Sprintf(format, args...))
}
