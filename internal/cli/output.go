// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/result"
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

// SinkOptions captures the populated values from [AddOutputFlags]. The struct is the input to
// [BuildSink], which composes a [result.Sink] from the flag values.
//
// Field semantics:
//
//   - Format selects the formatter: "json", "yaml", "csv", or "template". Default "json".
//   - Template carries the template body when Format == "template"; ignored otherwise.
//   - Filters is the repeatable --filter slice; each entry is a `field=value` predicate ANDed with
//     the others.
//   - JQ is the optional --jq expression. When non-empty, runs after the field-filter stage.
type SinkOptions struct {
	Format   string
	Template string
	Filters  []string
	JQ       string
}

// AddOutputFlags binds --format, --template, --filter, and --jq to opts. Call once during command
// setup, then call [BuildSink] from the cobra RunE to compose the [result.Sink].
//
// Usage:
//
//	var opts cli.SinkOptions
//	cli.AddOutputFlags(cmd, &opts)
//	cmd.RunE = func(cmd *cobra.Command, args []string) error {
//	    sink, err := cli.BuildSink(opts, cmd.OutOrStdout())
//	    if err != nil { return err }
//	    return sink.Emit(payload)
//	}
func AddOutputFlags(cmd *cobra.Command, opts *SinkOptions) {

	cmd.Flags().StringVar(&opts.Format, "format", "json",
		`Output format: json, yaml, csv, or template`)
	cmd.Flags().StringVar(&opts.Template, "template", "",
		`Template body, used when --format=template (e.g. '{{.Name}}\t{{.Version}}')`)
	cmd.Flags().StringArrayVar(&opts.Filters, "filter", nil,
		`Filter expression: field=value (repeatable, AND logic)`)
	cmd.Flags().StringVar(&opts.JQ, "jq", "",
		`jq expression applied after --filter; see github.com/itchyny/gojq`)
}

// BuildSink composes a [result.Sink] from the populated [SinkOptions] writing to w. Filters compose
// in --filter-then--jq order; the formatter is selected by [result.FormatterByName].
//
// Returns an error when the formatter name is unknown, the template body fails to parse, the field
// expressions fail to parse, or the jq expression fails to compile.
func BuildSink(opts SinkOptions, w io.Writer) (result.Sink, error) {

	formatter, err := result.FormatterByName(opts.Format, opts.Template)
	if err != nil {
		return nil, err
	}

	filter, err := result.FilterByExprs(opts.Filters, opts.JQ)
	if err != nil {
		return nil, err
	}

	return result.NewPipeline(filter, formatter, w), nil
}

// =============================================================================
// Status UI — package-global, set once at bootstrap
// =============================================================================
//
// The package-global statusUI is the canonical [status.Sink] for cli.Note /
// cli.Warn / cli.Error / cli.Failure / cli.Success / cli.Print facades. The
// same instance flows into RuntimeEnvironmentSpec.Status, so --silent and the
// program-name prefix apply uniformly across the cli facades, the runtime
// environment, providers that emit via env.Status, and starlark print().
//
// Bootstrap (cobra PersistentPreRun) reads --silent, constructs a
// status.Console with that value, and calls SetUI(ui). Tests can install a
// capture impl via SetUI and assert on the captured emissions via UI().
//
// Default before SetUI is status.Discard{} so any cli.Note call before bootstrap
// is silent rather than panicking.
var statusUI status.Sink = status.Discard{}

// SetUI installs the package-global [status.Sink] used by the cli facade
// functions ([Note], [Warn], [Error], [Failure], [Success], [Print]) and by
// the [AddSilentFlag] cobra binding. Called once during bootstrap (typically
// from a cobra PersistentPreRun).
//
// Subsequent calls replace the installed UI. Tests use this to install a
// capture implementation and read it back via [UI].
func SetUI(ui status.Sink) {
	statusUI = ui
}

// UI returns the currently installed [status.Sink]. Returns the default
// [status.Discard] when [SetUI] has not been called.
//
// Tests typically capture installed instances via type assertion:
//
//	cli.SetUI(captureUI)
//	defer cli.SetUI(status.NoOp{})
//	// ... exercise code under test ...
//	got := cli.UI().(*captureUI).Lines
func UI() status.Sink {
	return statusUI
}

// AddSilentFlag adds the --silent flag to a root command. The flag value is
// read by bootstrap (cobra PersistentPreRun) which constructs the status.Sink
// with the parsed silent value baked in via [status.NewConsole].
//
// Note that --silent is now a property of the [status.Sink] instance itself,
// applied at construction time. There is no facade-level silent gate; the UI
// honors silent or it doesn't.
func AddSilentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("silent", false,
		`Suppress all status messages (stderr)`)
}

// Note prints an informational message via the installed [status.Sink].
func Note(format string, args ...any) {
	statusUI.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message via the installed [status.Sink].
func Warn(format string, args ...any) {
	statusUI.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message via the installed [status.Sink]. Unlike
// [Failure], this does not return an error — use for non-fatal errors.
func Error(format string, args ...any) {
	statusUI.Error(fmt.Sprintf(format, args...))
}

// Failure prints an error message via the installed [status.Sink] and returns
// the wrapped error. Use when the operation cannot continue.
func Failure(format string, args ...any) error {
	return statusUI.Fail(fmt.Sprintf(format, args...))
}

// Success prints a success message via the installed [status.Sink].
func Success(format string, args ...any) {
	statusUI.Succeed(fmt.Sprintf(format, args...))
}

// Print emits raw text via the installed [status.Sink]. Used for unprefixed
// emission (e.g., starlark print() output captured at the cli level).
func Print(format string, args ...any) {
	statusUI.Print(fmt.Sprintf(format, args...))
}
