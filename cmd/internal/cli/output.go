// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// =============================================================================
// Exit Codes (BSD sysexits.h)
// =============================================================================

// The suite's exit codes are the BSD sysexits set, the same thirteen `Declare-BashScript` defines for the shell
// scripts beside these programs, so a status means one thing across the whole toolchain (ruled 2026-09-12).
//
// Zero and one are the two outside that set and the two most used: success, and "the command ran and the answer
// is failure" -- a verification that failed, drift that was found. Everything else says the command could not run
// as asked, and which way.
const (
	ExitOK          = 0  // Success
	ExitError       = 1  // The command ran; the answer is failure
	ExitUsage       = 64 // EX_USAGE: command line usage error
	ExitDataErr     = 65 // EX_DATAERR: data format error
	ExitNoInput     = 66 // EX_NOINPUT: cannot open input
	ExitUnavailable = 69 // EX_UNAVAILABLE: service unavailable, or a missing dependency
	ExitSoftware    = 70 // EX_SOFTWARE: internal software error
	ExitOSErr       = 71 // EX_OSERR: system error, such as a failure to fork
	ExitOSFile      = 72 // EX_OSFILE: a critical operating-system file is missing
	ExitCantCreate  = 73 // EX_CANTCREAT: cannot create an output file
	ExitIOErr       = 74 // EX_IOERR: input or output error
	ExitTempFail    = 75 // EX_TEMPFAIL: temporary failure; the user is invited to retry
	ExitProtocol    = 76 // EX_PROTOCOL: remote error in protocol
	ExitNoPerm      = 77 // EX_NOPERM: permission denied
	ExitConfig      = 78 // EX_CONFIG: configuration error, such as an unsupported platform
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

// ExitCode is the status a program exits with for an error.
//
// A coded error carries its own, from [ExitWith]. Cobra's own refusals -- an unknown flag, an unknown command, a
// wrong argument count, a flag value the command does not accept -- are usage errors and exit [ExitUsage],
// because the command could not run as asked. Everything else is [ExitError]: the command ran and the answer is
// failure.
//
// Parameters:
//   - `err`: the error a command returned, or nil.
//
// Returns:
//   - `int`: the process status.
func ExitCode(err error) int {

	if err == nil {
		return ExitOK
	}

	if coded, ok := errors.AsType[*exitError](err); ok {
		return coded.code
	}

	if isUsageError(err) {
		return ExitUsage
	}

	return ExitError
}

// isUsageError reports whether an error is cobra's own refusal of the command line.
//
// Cobra returns these as plain errors with no type to test, so the message is what there is to read. The
// alternative, wrapping every one at its source, means editing a dependency; the alternative to that is a flag
// value the shared root validates itself, which is what [AddOutputFlags] already does for `--output` and why
// that one arrives here already coded.
//
// Parameters:
//   - `err`: a non-nil error.
//
// Returns:
//   - `bool`: true when cobra refused the command line rather than a command failing.
func isUsageError(err error) bool {

	message := err.Error()

	for _, refusal := range []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"flag needs an argument",
		"invalid argument",
		"accepts ",
		"requires at least",
		"requires at most",
		"unknown help topic",

		// Cobra's three flag-group refusals (`flag_groups.go`). A group is a usage rule -- these flags
		// together, this one of them, not both -- so breaking one is a usage error like any other. Found
		// 2026-09-18 measuring `--interactive --unattended`, which exited 1 where §9 says 64; the pair has
		// been marked mutually exclusive on the shared root since long before that measurement.
		"are set they must all be set",
		"are set none of the others can be",
		"is required",
	} {
		if strings.Contains(message, refusal) {
			return true
		}
	}

	return false
}

// =============================================================================
// Output Flags
// =============================================================================

// SinkOptions captures the populated values from [AddOutputFlags]. The struct is the input to
// [BuildPipeline], which composes a [result.Pipeline] from the flag values.
type SinkOptions struct {
	Format   string // bound to --output; the field names the concept, the flag names what users type
	Filters  []string
	JQ       string
	Store    string
	Paginate bool // bound to --paginate; page whatever the rendering, when stdout is a terminal
	NoPager  bool // bound to --no-pager; never page
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
// outputUsage is what a terminal shows: each rendering names itself on one line and describes itself on
// the next, so nothing depends on a column that wrapping can break.
//
// The prose orders the renderings by usefulness. Below it they are grouped by the reader each serves, the
// groups alphabetical and the names alphabetical within each, matching [result.Group] and §7 of the
// specification. A flat list of ten names answers what exists and never which one you want.
//
// The man page shows [outputUsageMan] instead -- same content, markdown, so roff lays it out as a man page
// rather than as filled prose.
const outputUsage = "Output rendering. json is the default and the native format; every other rendering " +
	"presents that JSON rather than the Go value behind it. Reach for terminal to read a report and markdown " +
	"to paste one, table or list to scan a result, yaml to read a large one, csv or json to feed a program, " +
	"template when you need a shape none of these produce, and none when you want the exit code and the side " +
	"effects alone.\n" +
	"\n" +
	"The renderings, by the reader each serves:\n" +
	"\n" +
	"Composed -- you chose the shape, in the filter stage\n" +
	"  template=BODY\n" +
	"    a Go template; when you need a shape none of the others give\n" +
	"  value\n" +
	"    raw, tab-separated, no header; when cut or awk consumes it\n" +
	"\n" +
	"Document -- a report, rather than data\n" +
	"  markdown\n" +
	"    the document as markdown; when GitHub reads it, or you paste it\n" +
	"  terminal\n" +
	"    the document rendered, with bold and italic; when you read it\n" +
	"\n" +
	"Nothing\n" +
	"  none\n" +
	"    nothing at all; when you want the exit code, not the output\n" +
	"\n" +
	"Records -- laid out for a person\n" +
	"  list\n" +
	"    one field per line; when a record is wide, or records differ\n" +
	"  table\n" +
	"    aligned columns, one row per record; when scanning many rows\n" +
	"\n" +
	"Serialized -- lossless; a library reads it back\n" +
	"  csv\n" +
	"    quoted and parseable; when a spreadsheet or a data tool reads it\n" +
	"  json\n" +
	"    the native format, nothing elided; when a script consumes it\n" +
	"  yaml\n" +
	"    the same content as json, in a shape that reads by eye"

// outputUsageMan is the same content as [outputUsage], written as markdown for the man page.
//
// cobra generates a man page by writing markdown and handing it to md2man, which turns a block quote
// into `.PP` `.RS` ... `.RE` -- an indented block roff fills on its own terms. That is the shape
// `git-clone(1)` has: the name on its line, the description indented under it, continuations staying at
// the indent. Handing roff the terminal text instead gives one filled paragraph, because roff fills
// unless told otherwise, and the layout collapses.
//
// [withManUsage] installs this on the flag for the length of generation.
const outputUsageMan = "Output rendering. json is the default and the native format; every other rendering " +
	"presents that JSON rather than the Go value behind it. Reach for terminal to read a report and markdown " +
	"to paste one, table or list to scan a result, yaml to read a large one, csv or json to feed a program, " +
	"template when you need a shape none of these produce, and none when you want the exit code and the side " +
	"effects alone.\n" +
	"\n" +
	"The renderings, by the reader each serves:\n" +
	"\n" +
	"**Composed -- you chose the shape, in the filter stage**\n" +
	"\n" +
	"**template=BODY**\n" +
	"\n" +
	"> a Go template; when you need a shape none of the others give\n" +
	"\n" +
	"**value**\n" +
	"\n" +
	"> raw, tab-separated, no header; when cut or awk consumes it\n" +
	"\n" +
	"**Document -- a report, rather than data**\n" +
	"\n" +
	"**markdown**\n" +
	"\n" +
	"> the document as markdown; when GitHub reads it, or you paste it\n" +
	"\n" +
	"**terminal**\n" +
	"\n" +
	"> the document rendered, with bold and italic; when you read it\n" +
	"\n" +
	"**Nothing**\n" +
	"\n" +
	"**none**\n" +
	"\n" +
	"> nothing at all; when you want the exit code, not the output\n" +
	"\n" +
	"**Records -- laid out for a person**\n" +
	"\n" +
	"**list**\n" +
	"\n" +
	"> one field per line; when a record is wide, or records differ\n" +
	"\n" +
	"**table**\n" +
	"\n" +
	"> aligned columns, one row per record; when scanning many rows\n" +
	"\n" +
	"**Serialized -- lossless; a library reads it back**\n" +
	"\n" +
	"**csv**\n" +
	"\n" +
	"> quoted and parseable; when a spreadsheet or a data tool reads it\n" +
	"\n" +
	"**json**\n" +
	"\n" +
	"> the native format, nothing elided; when a script consumes it\n" +
	"\n" +
	"**yaml**\n" +
	"\n" +
	"> the same content as json, in a shape that reads by eye\n"

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
			// A value the flag does not accept is a command line the program cannot run, so it exits
			// EX_USAGE like any other refusal of the arguments (ruled 2026-09-13).
			return ExitWith(ExitUsage, err)
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

		if err := closeActivePager(); err != nil {
			return err
		}

		if previousPost != nil {
			return previousPost(c, args)
		}
		return nil
	}

	// Not mutually exclusive, because git's are not: `git -p -P log` and `git -P -p log` both exit 0, and
	// `git.c` assigns `use_pager` sequentially so the last one wins. Refusing the pair would also be the only
	// usage error in the suite that cobra reports through a message [isUsageError] does not match, so it would
	// exit 1 where §9 says 64. [shouldPage] settles the pair instead: `--no-pager` wins.
	cmd.PersistentFlags().BoolVarP(&opts.Paginate, "paginate", "p", false, paginateUsage)
	cmd.PersistentFlags().BoolVarP(&opts.NoPager, "no-pager", "P", false, noPagerUsage)
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

// The pager's two switches, which are deliberately not part of the common set.
//
// The common set is §4's four flags of the result pipeline, and §14's invariants police it: a program binds
// all four on its root with this usage text, and every subcommand inherits all four. These two take no value
// and change no rendering -- they are behavioral switches, like `--dry-run` -- so putting them in the set
// would enlarge a specification concept to house a convenience. A subcommand that binds `-p` for itself is
// still caught, because [CheckNoOwnOutputFlag] reports a shorthand an ancestor already carries.
const (
	paginateUsage = `Page the result whenever stdout is a terminal, whatever its rendering`
	noPagerUsage  = `Never page the result, whatever its rendering and wherever it is going`
)

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

	// The width belongs to the environment, and [result] cannot read this package's answer to it: the
	// dependency runs the other way. So the one formatter that wraps is told here, where it is constructed.
	if terminal, wraps := formatter.(result.TerminalFormatter); wraps {
		terminal.WordWrap = displayWidth()
		formatter = terminal
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

// SilentRequested reports whether `--silent` appears in the arguments, read before cobra parses them.
//
// `--silent` is documented to take effect immediately, and a program that narrates while building its command
// tree narrates before any flag is parsed: star must load its extensions to register their commands, and
// assembling that runtime reports the module surface. Cobra cannot help at that point, so the flag is read from
// the raw arguments (#828).
//
// Parsing is cobra's own for a boolean: the bare `--silent`, and `--silent=false` to refuse it. A `--` ends the
// flags, so anything after it is an operand and is not read.
//
// Parameters:
//   - `args`: the raw arguments, normally `os.Args[1:]`.
//
// Returns:
//   - `bool`: true when the arguments ask for silence.
func SilentRequested(args []string) bool {

	for _, arg := range args {
		switch {
		case arg == "--":
			return false
		case arg == "--silent":
			return true
		case strings.HasPrefix(arg, "--silent="):
			return strings.TrimPrefix(arg, "--silent=") == "true"
		}
	}

	return false
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

// emitWriter returns where a result is written: the command's own output, or a pager in front of it.
//
// The formatter is built here only to ask its group, which is what decides whether a person is reading this
// result; [BuildPipeline] builds its own from the same name, so a bad name fails identically either way.
//
// The pager is opened once per command invocation and closed in the post-run [addOutputFlags] installs, since
// a command may Emit more than once and each result belongs on the same screen.
//
// Parameters:
//   - `writer`: where the result was bound for; [Emit] takes it from the command, because §14 allows that
//     call in [Emit] and nowhere else.
//   - `opts`: the root's options.
//   - `program`: the program's name, for the configuration section the pager reads.
//
// Returns:
//   - `io.Writer`: the destination, paged or not.
//   - `error`: the rendering name is unknown.
func emitWriter(writer io.Writer, opts SinkOptions, program string) (io.Writer, error) {

	formatter, err := result.FormatterByName(opts.Format)
	if err != nil {
		return nil, err
	}

	if !shouldPage(sink.New(writer).IsTTY(), formatter.Group(), opts.Paginate, opts.NoPager) {
		return writer, nil
	}

	if activePager == nil {
		// A pager that will not start is not worth failing a command over: the result still reaches the
		// terminal, unpaged, which is what a reader on a machine without `less` gets anyway.
		pager, err := openPager(writer, program)
		if err != nil {
			return writer, nil //nolint:nilerr // the result is worth more than the pager
		}
		activePager = pager
	}

	return activePager, nil
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

	writer, err := emitWriter(cmd.OutOrStdout(), *opts, cmd.Root().Name())
	if err != nil {
		return err
	}

	pipeline, err := BuildPipeline(*opts, writer)
	if err != nil {
		return err
	}

	return pipeline.Emit(value)
}
