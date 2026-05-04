// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/output"
	"github.com/NobleFactor/devlore-cli/pkg/status"
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
// Status UI — package-global, set once at bootstrap
// =============================================================================
//
// The package-global statusUI is the canonical [status.UI] for cli.Note /
// cli.Warn / cli.Error / cli.Failure / cli.Success / cli.Print facades. The
// same instance flows into RuntimeEnvironmentSpec.Status, so --silent and the
// program-name prefix apply uniformly across the cli facades, the runtime
// environment, providers that emit via env.Status, and starlark print().
//
// Bootstrap (cobra PersistentPreRun) reads --silent, constructs a
// status.Console with that value, and calls SetUI(ui). Tests can install a
// capture impl via SetUI and assert on the captured emissions via UI().
//
// Default before SetUI is status.NoOp{} so any cli.Note call before bootstrap
// is silent rather than panicking.
var statusUI status.UI = status.NoOp{}

// SetUI installs the package-global [status.UI] used by the cli facade
// functions ([Note], [Warn], [Error], [Failure], [Success], [Print]) and by
// the [AddSilentFlag] cobra binding. Called once during bootstrap (typically
// from a cobra PersistentPreRun).
//
// Subsequent calls replace the installed UI. Tests use this to install a
// capture implementation and read it back via [UI].
func SetUI(ui status.UI) {
	statusUI = ui
}

// UI returns the currently installed [status.UI]. Returns the default
// [status.NoOp] when [SetUI] has not been called.
//
// Tests typically capture installed instances via type assertion:
//
//	cli.SetUI(captureUI)
//	defer cli.SetUI(status.NoOp{})
//	// ... exercise code under test ...
//	got := cli.UI().(*captureUI).Lines
func UI() status.UI {
	return statusUI
}

// AddSilentFlag adds the --silent flag to a root command. The flag value is
// read by bootstrap (cobra PersistentPreRun) which constructs the status.UI
// with the parsed silent value baked in via [status.NewConsole].
//
// Note that --silent is now a property of the [status.UI] instance itself,
// applied at construction time. There is no facade-level silent gate; the UI
// honors silent or it doesn't.
func AddSilentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("silent", false,
		`Suppress all status messages (stderr)`)
}

// Note prints an informational message via the installed [status.UI].
func Note(format string, args ...any) {
	statusUI.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message via the installed [status.UI].
func Warn(format string, args ...any) {
	statusUI.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message via the installed [status.UI]. Unlike
// [Failure], this does not return an error — use for non-fatal errors.
func Error(format string, args ...any) {
	statusUI.Error(fmt.Sprintf(format, args...))
}

// Failure prints an error message via the installed [status.UI] and returns
// the wrapped error. Use when the operation cannot continue.
func Failure(format string, args ...any) error {
	return statusUI.Fail(fmt.Sprintf(format, args...))
}

// Success prints a success message via the installed [status.UI].
func Success(format string, args ...any) {
	statusUI.Succeed(fmt.Sprintf(format, args...))
}

// Print emits raw text via the installed [status.UI]. Used for unprefixed
// emission (e.g., starlark print() output captured at the cli level).
func Print(format string, args ...any) {
	statusUI.Print(fmt.Sprintf(format, args...))
}
