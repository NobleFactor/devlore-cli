// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package status

import (
	"errors"
	"fmt"
	"io"
)

// ANSI color codes used by Console to decorate categorized status messages.
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

// Console is the TTY-aware [UI] implementation. Categorized methods emit
// "[<programName>] [<colored symbol>] <msg>\n" to the configured writer; [Console.Print] emits the
// raw message followed by a newline, with no decoration.
//
// All fields are unexported and set at construction by [NewConsole]; the value is immutable from the
// caller's perspective. Mid-run silent toggling, color toggling, etc. require constructing a new
// Console and replacing the runtime environment's Status reference.
type Console struct {
	writer      io.Writer
	programName string
	color       bool
	silent      bool
}

// Compile-time interface guard.
var _ UI = (*Console)(nil)

// NewConsole constructs an immutable [Console] writing to w.
//
// Parameters:
//   - w: output destination, typically [os.Stderr]. Must not be nil.
//   - programName: name shown in the "[program]" prefix on categorized messages (e.g., "lore").
//   - color: when true, ANSI color codes wrap the status symbol; when false, symbols emit as plain
//     bytes. Callers typically derive this from a TTY check on w.
//   - silent: when true, every method (including [Console.Print] and [Console.Fail]) emits nothing;
//     [Console.Fail] still returns a non-nil error so callers can propagate the failure.
//
// Returns:
//   - *Console: the constructed value, ready to install on a runtime environment spec.
func NewConsole(w io.Writer, programName string, color, silent bool) *Console {
	return &Console{
		writer:      w,
		programName: programName,
		color:       color,
		silent:      silent,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Note emits an informational status message in gray.
func (c *Console) Note(msg string) {

	if c.silent {
		return
	}
	c.emit(colorGray, symbolNote, msg)
}

// Warn emits a warning status message in yellow.
func (c *Console) Warn(msg string) {

	if c.silent {
		return
	}
	c.emit(colorYellow, symbolWarn, msg)
}

// Error emits a non-fatal error status message in red.
func (c *Console) Error(msg string) {

	if c.silent {
		return
	}
	c.emit(colorRed, symbolError, msg)
}

// Success emits a positive-outcome status message in green.
func (c *Console) Success(msg string) {

	if c.silent {
		return
	}
	c.emit(colorGreen, symbolSuccess, msg)
}

// Fail emits a fatal error status message in red and returns a Go error wrapping the message. Silent
// mode suppresses the emission but still returns the error.
func (c *Console) Fail(msg string) error {

	if !c.silent {
		c.emit(colorRed, symbolError, msg)
	}
	return errors.New(msg)
}

// Print emits raw text followed by a newline. No decoration; intended for starlark `print()` output
// where the script's exact bytes are what the user expects.
func (c *Console) Print(msg string) {

	if c.silent {
		return
	}
	_, _ = fmt.Fprintln(c.writer, msg) //nolint:errcheck // status output is best-effort
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// emit writes one decorated status line to the writer. Format is
// "[<programName>] [<colored symbol>] <msg>\n"; when color is false the colorize wrap is a no-op.
func (c *Console) emit(color, symbol, msg string) {
	_, _ = fmt.Fprintf(c.writer, "[%s] [%s] %s\n", c.programName, c.colorize(color, symbol), msg) //nolint:errcheck // status output is best-effort
}

// colorize wraps text in ANSI color codes when color is enabled; otherwise returns text unchanged.
func (c *Console) colorize(color, text string) string {

	if !c.color {
		return text
	}
	return color + text + colorReset
}

// endregion

// endregion
