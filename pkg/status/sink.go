// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package status owns the side channel for human-facing user interface output — the stderr-equivalent
// stream of categorized status messages and starlark `print()` output.
//
// The package's contract is the [Sink] interface, with two ship-default implementations: [Console]
// (TTY-aware, formatted with program-name and color) and [Discard] (silent, satisfies the contract for
// tests and bootstrap-before-PreRun).
//
// Construction is immutable: callers pass `programName`, `color`, and `silent` to [NewConsole] once;
// no setters mutate the instance after construction. The same instance flows from the client's
// bootstrap (typically a cobra `PersistentPreRun`) into the runtime environment spec, into the
// runtime environment, and from there to every status emission point in the framework — `cli.Note`
// facades, `env.Status.Note` calls inside providers, starlark `Thread.Print`, and the executor.
//
// `--silent` is therefore a property of the instance, applied uniformly to every emission across
// every entry point. There is no facade-level silent gate; the Sink honors silent or it doesn't.
package status

// Sink is the side-channel user-facing output contract.
//
// The five categorized methods (Note, Warn, Error, Success, Fail) emit status messages with
// implementation-defined formatting — typically prefixed with the program name and a colored symbol
// when the implementation is TTY-aware. Print is the destination for starlark `print()` output and
// emits without the categorized-message decoration so script-printed text reads as the script wrote
// it.
//
// All methods are silent when the implementation is in silent mode (e.g., `--silent` flag).
//
// Implementations are immutable from the caller's perspective; configuration is fixed at construction
// time. Mid-run state changes (e.g., toggling silent) require constructing a new instance.
type Sink interface {

	// Error emits a non-fatal error status message.
	//
	// Distinct from [Fail], which is for fatal errors that need to propagate upward as a Go error.
	Error(msg string)

	// Fail emits a fatal error status message and returns a Go error wrapping the message. The caller typically returns
	// the error to abort the operation.
	Fail(msg string) error

	// Note emits an informational status message.
	Note(msg string)

	// Print emits raw text without categorized-message decoration.
	//
	// Used by starlark `Thread.Print` for `print()` output from scripts; the result reads as the script wrote it (no
	// [program] [symbol] prefix).
	Print(msg string)

	// Succeed emits a positive-outcome status message.
	Succeed(msg string)

	// Warn emits a warning status message.
	Warn(msg string)
}
