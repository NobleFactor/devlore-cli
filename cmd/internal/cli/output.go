// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"io"
	"sync"

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

// restoreStoreRoot undoes the root's `--store` selection when the command tree finishes. A package-level
// value because [SetStoreRoot]'s own root is one: the pair is scoped to a single command invocation, which
// is the only thing a cobra process runs.
var restoreStoreRoot func()

// outputUsage documents --output.
//
// The prose is one logical line, joined across source lines, so [wrapUsage] reflows it to the terminal
// rather than to a width guessed here. The rendering list is one line each, because those carry a name
// column that wrapping hangs under -- a shape pflag's own wrapper flattens.
//
// The prose orders the renderings by usefulness; the list is alphabetical, matching
// [result.FormatterByName]'s error and §7 of the specification. Everyday use wants to find a name.
const outputUsage = "Output rendering. json is the default and the native format; every other rendering " +
	"presents that JSON rather than the Go value behind it. Reach for yaml to read a large result, table " +
	"or list to scan one, csv or value to feed another program, template when you need a shape none of " +
	"these produce, and none when you want the exit code and the side effects alone.\n" +
	"\n" +
	"The renderings, alphabetically, each closing with what it is for:\n" +
	"csv            quoted and parseable; when a spreadsheet or a data tool reads it\n" +
	"json           the native format, nothing elided; when a script consumes it\n" +
	"list           one field per line; when a record is wide, or records differ\n" +
	"none           nothing at all; when you want the exit code, not the output\n" +
	"table          aligned columns, one row per record; when scanning many rows\n" +
	"template=BODY  a Go template; when you need a shape none of the others give\n" +
	"value          raw, tab-separated, no header; when cut or awk consumes it\n" +
	"yaml           the same content as json; when reading a large result by eye"

// addOutputFlags binds the common set -- --filter, --jq, --output/-o, and --store -- to opts.
//
// Bound to PersistentFlags, so one call on a program's root command covers every subcommand. All four
// in-scope programs register the whole set: a user who learns `-o yaml` on one types it on the next without
// checking. See docs/architecture/10-command-line-interface.md.
//
// Call once during root setup, then call [BuildPipeline] from a command's RunE to compose the
// [result.Pipeline].
//
// Binding also WIRES the two flags whose meaning does not depend on a command rendering anything, so
// registering the set and honoring it cannot come apart:
//
//   - `--output` is validated. A command that never reaches [BuildPipeline] used to accept any string,
//     because [result.FormatterByName] is the only place the value is checked -- `writ reconcile -o bogus`
//     printed its report and exited 0 (#754).
//   - `--store` is resolved. It used to be read by nothing outside `devlore-test`, so `writ reconcile --store
//     <elsewhere>` folded runs from the DEFAULT store and reported the result as compliance (#753).
//
// Both were one defect wearing two faces: a flag registered on a root that no leaf consumed. Cobra
// advertised the whole set on every command's help while one command honored it, and a flag that is present
// and inert is worse than an absent one -- an absent flag errors, and the user tries something else.
//
// The store selection is undone when the command tree finishes. Leaving it set would be harmless in a
// process that runs one command and exits, and corrupting in a test binary that runs many: the root is a
// package-level value, so the first command passing `--store` would silently relocate every command after
// it. A caller needing a narrower scope still calls [SetStoreRoot] itself, as `devlore-test` does per run.
//
// Parameters:
//   - `cmd`: the command to bind to, normally a program's root.
//   - `opts`: the struct the flag values populate.
func addOutputFlags(cmd *cobra.Command, opts *SinkOptions) {

	previous := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {

		if previous != nil {
			if err := previous(c, args); err != nil {
				return err
			}
		}

		if _, err := result.FormatterByName(opts.Format); err != nil {
			return err
		}

		if opts.Store != "" {
			restore, err := SetStoreRoot(opts.Store)
			if err != nil {
				return err
			}
			restoreStoreRoot = restore
		}

		return nil
	}

	previousPost := cmd.PersistentPostRunE
	cmd.PersistentPostRunE = func(c *cobra.Command, args []string) error {

		if restoreStoreRoot != nil {
			restoreStoreRoot()
			restoreStoreRoot = nil
		}

		if previousPost != nil {
			return previousPost(c, args)
		}
		return nil
	}

	cmd.PersistentFlags().StringVarP(&opts.Format, "output", "o", "json", commonSetUsage["output"])
	cmd.PersistentFlags().StringArrayVar(&opts.Filters, "filter", nil, commonSetUsage["filter"])
	cmd.PersistentFlags().StringVar(&opts.JQ, "jq", "", commonSetUsage["jq"])
	cmd.PersistentFlags().StringVar(&opts.Store, "store", "", commonSetUsage["store"])
}

// commonSetUsage is the usage text of each flag of the common set, as the shared root binds it. Held
// here so [CheckSharedSetOnRoot] can tell the shared set from a hand-rolled one carrying the same names.
var commonSetUsage = map[string]string{
	"filter": `Filter expression: field=value (repeatable, AND logic)`,
	"jq":     `jq expression applied after --filter; see github.com/itchyny/gojq`,
	"output": outputUsage,
	"store":  `Execution store root, holding definitions and traces (default: the XDG state path)`,
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

// rootOptions holds, per root built by [NewRootCmd], the options its common set binds. One process runs one
// command tree, but a test builds several roots, so the record is per root rather than per package.
var (
	rootOptions   = map[*cobra.Command]*SinkOptions{}
	rootOptionsMu sync.Mutex
)

// registerRootOptions records the options a root's common set binds, for [Emit] to find.
//
// Parameters:
//   - `root`: the root command.
//   - `opts`: the options its flags write into.
func registerRootOptions(root *cobra.Command, opts *SinkOptions) {
	rootOptionsMu.Lock()
	defer rootOptionsMu.Unlock()
	rootOptions[root] = opts
}

// Emit renders a command's result to stdout through the shared pipeline, with the options the command's
// root binds: `--output` selects the rendering, `--filter` and `--jq` narrow it. It is the one render path
// for every program on the shared root and for the shared commands alike (10-command-line-interface.md §8;
// ruled 2026-09-03).
//
// Parameters:
//   - `cmd`: the running command; its root's options and its output writer are used.
//   - `value`: the result.
//
// Returns:
//   - `error`: the root was not built by [NewRootCmd], the pipeline cannot be built, or the value cannot
//     be rendered.
func Emit(cmd *cobra.Command, value any) error {

	rootOptionsMu.Lock()
	opts, ok := rootOptions[cmd.Root()]
	rootOptionsMu.Unlock()
	if !ok {
		return fmt.Errorf("%s: the root was not built by cli.NewRootCmd, so it carries no common set to render with", cmd.CommandPath())
	}

	pipeline, err := BuildPipeline(*opts, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	return pipeline.Emit(value)
}
