// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import "testing"

// region Tests

// TestDisplayWidth_ColumnsWinsAndIsNotCapped pins the order POSIX states and git, man-db and Python follow.
//
// The width a user exports is what they asked for, so it is never trimmed to [widthCap]. The terminal is not
// consulted here: under `go test` stdout is not one, which is the pipe case as well.
func TestDisplayWidth_ColumnsWinsAndIsNotCapped(t *testing.T) {

	for _, testCase := range []struct {
		columns string
		want    int
	}{
		{"60", 60},
		{"211", 211},
	} {
		t.Run(testCase.columns, func(t *testing.T) {
			t.Setenv("COLUMNS", testCase.columns)
			if got := displayWidth(); got != testCase.want {
				t.Errorf("displayWidth() = %d, want %d", got, testCase.want)
			}
		})
	}
}

// TestDisplayWidth_NonsenseColumnsFallsThrough covers a variable set to something that is not a width.
//
// Stdout is not a terminal under `go test`, so the fallback is the answer; the test asserts the value is not
// taken from the broken variable.
func TestDisplayWidth_NonsenseColumnsFallsThrough(t *testing.T) {

	for _, columns := range []string{"", "wide", "0", "-1"} {
		t.Run(columns, func(t *testing.T) {
			t.Setenv("COLUMNS", columns)
			if got := displayWidth(); got != widthFallback {
				t.Errorf("displayWidth() = %d, want the fallback %d", got, widthFallback)
			}
		})
	}
}

// TestCapWidth_BoundsTheTerminalOnly is the cap the terminal path applies.
func TestCapWidth_BoundsTheTerminalOnly(t *testing.T) {

	for _, testCase := range []struct{ width, want int }{
		{80, 80},
		{widthCap, widthCap},
		{211, widthCap},
	} {
		if got := capWidth(testCase.width); got != testCase.want {
			t.Errorf("capWidth(%d) = %d, want %d", testCase.width, got, testCase.want)
		}
	}
}

// endregion
