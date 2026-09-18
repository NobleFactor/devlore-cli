// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// The widths that answer when the environment does not, in columns.
const (

	// widthCap bounds a width taken from the terminal. A modern window is often 200 columns and more, and
	// prose set that wide is hard to read -- glow caps its own rendering at 120 for the same reason. A width
	// the user states in COLUMNS is not capped: they said what they want.
	widthCap = 120

	// widthFallback answers when neither COLUMNS nor the terminal does.
	//
	// Chosen over pflag's zero, which means "do not wrap at all": a pipe or a CI log has no width to report,
	// and an unwrapped line there is a wall of text rather than a deliberate choice.
	widthFallback = 100
)

// region Helpers

// displayWidth reports the column count text should wrap to.
//
// One answer serves help text and the `terminal` rendering, so the two cannot disagree about how wide the
// screen is.
//
// COLUMNS wins when it is set and sane, uncapped: a user who exports it has said what they want, and it is the
// only answer available when stdout is a pipe. POSIX says the same -- the variable "overrides the number of
// columns in the terminal window size and any terminal-width information implied by TERM" -- and `git`,
// `man-db` and Python's `shutil.get_terminal_size` all consult it first. The terminal is asked next, capped by
// [widthCap], and [widthFallback] answers when neither does.
//
// Returns:
//   - `int`: the wrap width in columns.
func displayWidth() int {

	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}

	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return capWidth(width)
	}

	return widthFallback
}

// capWidth bounds a width the terminal reported.
//
// Parameters:
//   - `width`: the terminal's column count.
//
// Returns:
//   - `int`: the width, or [widthCap] when it exceeds it.
func capWidth(width int) int {

	if width > widthCap {
		return widthCap
	}
	return width
}

// endregion
