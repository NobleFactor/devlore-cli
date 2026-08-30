// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
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
// [BuildPipeline], which composes a [result.Pipeline] from the flag values.
type SinkOptions struct {
	Format  string // bound to --output; the field names the concept, the flag names what users type
	Filters []string
	JQ      string
	Store   string
}

// AddOutputFlags binds the common set -- --filter, --jq, --output/-o, and --store -- to opts.
//
// Bound to PersistentFlags, so one call on a program's root command covers every subcommand. All four
// in-scope programs register the whole set: a user who learns `-o yaml` on one types it on the next without
// checking. See docs/architecture/10-command-line-interface.md.
//
// Call once during root setup, then call [BuildPipeline] from a command's RunE to compose the
// [result.Pipeline].
//
// Parameters:
//   - `cmd`: the command to bind to, normally a program's root.
//   - `opts`: the struct the flag values populate.
func AddOutputFlags(cmd *cobra.Command, opts *SinkOptions) {

	cmd.PersistentFlags().StringVarP(&opts.Format, "output", "o", "json",
		`Output rendering: json, yaml, table, csv, value (raw, pairs with --jq), template=BODY, or none`)
	cmd.PersistentFlags().StringArrayVar(&opts.Filters, "filter", nil,
		`Filter expression: field=value (repeatable, AND logic)`)
	cmd.PersistentFlags().StringVar(&opts.JQ, "jq", "",
		`jq expression applied after --filter; see github.com/itchyny/gojq`)
	cmd.PersistentFlags().StringVar(&opts.Store, "store", "",
		`Execution store root, holding definitions and traces (default: the XDG state path)`)
}

// BuildPipeline composes a [result.Pipeline] from the populated [SinkOptions] writing through w.
// Filters compose in --filter-then--jq order; the formatter is selected by [result.FormatterByName].
// The writer is wrapped in a [sink.Sink] via [sink.New] internally.
//
// Returns an error when the formatter name is unknown, the field
// expressions fail to parse, or the jq expression fails to compile.
func BuildPipeline(opts SinkOptions, w io.Writer) (*result.Pipeline, error) {

	formatter, err := result.FormatterByName(opts.Format)
	if err != nil {
		return nil, err
	}

	filter, err := result.FilterByExprs(opts.Filters, opts.JQ)
	if err != nil {
		return nil, err
	}

	return result.NewPipeline(filter, formatter, sink.New(w)), nil
}

// =============================================================================
// Narrator — package-global, set once at bootstrap
// =============================================================================
//
// The package-global narrator is the canonical [*status.Narrator] for cli.Note / cli.Warn /
// cli.Error / cli.Failure / cli.Success / cli.Print facades. The same instance flows into every
// runtime environment — WithStatus(cli.UI()) at each RuntimeEnvironmentSpec construction; star
// installs it on its long-lived environment at bootstrap — so --silent and the program-name
// prefix apply uniformly across the cli facades, the runtime environment, providers that emit
// via env.Status, and starlark print().
//
// Bootstrap (cobra PersistentPreRun) reads --silent and forks: silent → wrap [sink.Discard],
// otherwise → wrap [sink.Stderr]. Both forks call [SetUI] with the constructed Narrator.
//
// Default before SetUI is a Narrator wrapping [sink.Discard], so any cli.Note call before
// bootstrap is silent rather than panicking.
var narrator = status.NewNarrator("", sink.Discard())

// SetUI installs the package-global narrator used by the cli facade functions ([Note], [Warn],
// [Error], [Failure], [Success], [Print]).
//
// Subsequent calls replace the installed narrator.
func SetUI(n *status.Narrator) {
	narrator = n
}

// UI returns the currently installed narrator.
func UI() *status.Narrator {
	return narrator
}

// AddSilentFlag adds the --silent flag to a root command. The flag value is read by bootstrap
// (cobra PersistentPreRun) which forks construction of the narrator: silent → [sink.Discard],
// otherwise → [sink.Stderr].
func AddSilentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("silent", false,
		`Suppress all status messages (stderr)`)
}

// Note prints an informational message via the installed narrator.
func Note(format string, args ...any) {
	narrator.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message via the installed narrator.
func Warn(format string, args ...any) {
	narrator.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message via the installed narrator. Unlike [Failure], this does not return
// an error — use for non-fatal errors.
func Error(format string, args ...any) {
	narrator.Error(fmt.Sprintf(format, args...))
}

// Failure prints an error message via the installed narrator and returns the wrapped error. Use
// when the operation cannot continue.
func Failure(format string, args ...any) error {
	return narrator.Fail(fmt.Sprintf(format, args...))
}

// Success prints a success message via the installed narrator.
func Success(format string, args ...any) {
	narrator.Succeed(fmt.Sprintf(format, args...))
}

// Print emits raw text via the installed narrator.
func Print(format string, args ...any) {
	narrator.Print(fmt.Sprintf(format, args...))
}
