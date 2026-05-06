// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package status owns the side-channel for human-facing user-interface output — the categorized
// narration stream that accompanies a tool's primary work.
//
// The package's surface is the [Narrator] concrete type, which wraps a [pkg/sink.Sink] with the
// six categorized emission methods (Note, Warn, Error, Succeed, Fail, Print) and the program-name
// prefix. Color decoration is keyed off the sink's [pkg/sink.Sink.IsTTY] query — TTYs get ANSI
// color codes around the category symbol; non-TTYs get plain bytes.
//
// Narrator emissions form a progress arc that conventionally concludes with [Narrator.Succeed]
// (positive resolution) or [Narrator.Fail] (negative resolution). The narration is event-stream
// in shape but story-shaped in semantic — successive Notes and Warns build context for the
// terminal Succeed/Fail.
//
// Construction is immutable: callers pass program name + sink to [NewNarrator] once; no setters
// mutate the instance after construction. Silence is selected by passing [pkg/sink.Discard] as the
// sink, not by toggling a Narrator field.
package status

import (
	"errors"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/sink"
)

// ANSI color codes used by Narrator to decorate categorized status messages.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorGray   = "\033[37m"
)

// Status symbols rendered before the message text.
const (
	symbolNote    = "+"
	symbolWarn    = "△"
	symbolError   = "✖"
	symbolSuccess = "✔"
)

// Narrator is the categorized narration wrapper. Methods emit "[<programName>] [<colored symbol>]
// <msg>\n" through the configured [sink.Sink]; [Narrator.Print] emits the raw message followed by a
// newline, with no decoration.
//
// All fields are unexported and set at construction by [NewNarrator]; the value is immutable from
// the caller's perspective. Mid-run reconfiguration (writer redirect, program rename) requires
// constructing a new Narrator and replacing the runtime environment's Status reference.
//
// Color decoration is keyed off [sink.Sink.IsTTY] at construction — true → wrap symbols in ANSI
// codes, false → plain bytes. To suppress all output, construct with [sink.Discard] as the sink.
type Narrator struct {
	programName string
	sink        sink.Sink
	color       bool
}

// NewNarrator constructs an immutable [Narrator] writing through the supplied sink.
//
// Parameters:
//   - programName: name shown in the "[program]" prefix on categorized messages (e.g., "lore").
//   - s:           the [sink.Sink] to write through. Must not be nil. Pass [sink.Discard] to
//     suppress all narration; pass [sink.Stderr] for the standard cli case.
//
// Returns:
//   - *Narrator: the constructed value, ready to install on a runtime environment spec.
func NewNarrator(programName string, s sink.Sink) *Narrator {
	return &Narrator{
		programName: programName,
		sink:        s,
		color:       s.IsTTY(),
	}
}

// region EXPORTED METHODS

// Error emits a non-fatal error status message in red.
//
// Parameters:
//   - msg: the error message to emit.
func (n *Narrator) Error(msg string) {
	n.emit(colorRed, symbolError, msg)
}

// Fail emits a fatal error status message in red and returns a Go error wrapping the message.
//
// Parameters:
//   - msg: the fatal-error message to emit.
//
// Returns:
//   - error: a non-nil error wrapping msg. Callers typically return this to abort the operation.
func (n *Narrator) Fail(msg string) error {
	n.emit(colorRed, symbolError, msg)
	return errors.New(msg)
}

// Note emits an informational status message in gray.
//
// Parameters:
//   - msg: the informational message to emit.
func (n *Narrator) Note(msg string) {
	n.emit(colorGray, symbolNote, msg)
}

// Print emits raw text followed by a newline.
//
// No decoration; intended for starlark `print()` output where the script's exact bytes are what the
// user expects.
//
// Parameters:
//   - msg: the raw text to emit.
func (n *Narrator) Print(msg string) {
	_, _ = fmt.Fprintln(n.sink, msg) //nolint:errcheck // narration is best-effort
}

// Succeed emits a positive-outcome status message in green.
//
// Parameters:
//   - msg: the success message to emit.
func (n *Narrator) Succeed(msg string) {
	n.emit(colorGreen, symbolSuccess, msg)
}

// Warn emits a warning status message in yellow.
//
// Parameters:
//   - msg: the warning message to emit.
func (n *Narrator) Warn(msg string) {
	n.emit(colorYellow, symbolWarn, msg)
}

// endregion

// region UNEXPORTED METHODS

// emit writes one decorated status line through the sink.
//
// Format is "[<programName>] [<colored symbol>] <msg>\n"; when color is false the colorize wrap is
// a no-op.
//
// Parameters:
//   - color:  ANSI color code to wrap the symbol in (when color is enabled).
//   - symbol: the category symbol.
//   - msg:    the message body.
func (n *Narrator) emit(color, symbol, msg string) {
	_, _ = fmt.Fprintf(n.sink, "[%s] [%s] %s\n", n.programName, n.colorize(color, symbol), msg) //nolint:errcheck // narration is best-effort
}

// colorize wraps text in ANSI color codes when color is enabled; otherwise returns text unchanged.
//
// Parameters:
//   - color: the ANSI color code to wrap with.
//   - text:  the text to wrap.
//
// Returns:
//   - string: text wrapped in ANSI codes iff n.color is true.
func (n *Narrator) colorize(color, text string) string {
	if !n.color {
		return text
	}
	return color + text + colorReset
}

// endregion
