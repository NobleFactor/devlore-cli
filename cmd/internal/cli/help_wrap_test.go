// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"
)

// region Tests

// TestWrapUsage_HangsUnderTheTextItContinues is why this wrapper exists rather than pflag's.
//
// pflag indents every continuation to the flag's description column, having one indent level and no notion
// of structure. A two-column usage -- `--output`'s renderings, each a name and a sentence -- collapses the
// moment it wraps, and the column is exactly what made the list scannable.
func TestWrapUsage_HangsUnderTheTextItContinues(t *testing.T) {

	const line = "      csv            quoted and parseable; when a spreadsheet or a data tool reads it"

	got := strings.Split(strings.TrimRight(wrapUsage(line, 50), "\n"), "\n")

	if len(got) < 2 {
		t.Fatalf("did not wrap at 50:\n%s", strings.Join(got, "\n"))
	}
	for i, l := range got {
		if len([]rune(l)) > 50 {
			t.Errorf("line %d is %d columns, want <= 50: %q", i, len([]rune(l)), l)
		}
	}

	// "      csv            " is 21 columns: six of indent, three of name, twelve of padding.
	for i, l := range got[1:] {
		if !strings.HasPrefix(l, strings.Repeat(" ", 21)) {
			t.Errorf("continuation %d does not hang under the sentence: %q", i, l)
		}
		if strings.HasPrefix(l, strings.Repeat(" ", 22)) {
			t.Errorf("continuation %d is over-indented: %q", i, l)
		}
	}
}

// TestWrapUsage_ProseHangsAtItsIndent covers a line with no name column.
func TestWrapUsage_ProseHangsAtItsIndent(t *testing.T) {

	const line = "      Output rendering. json is the default and the native format, and the rest present it."

	got := strings.Split(strings.TrimRight(wrapUsage(line, 40), "\n"), "\n")

	if len(got) < 2 {
		t.Fatalf("did not wrap at 40:\n%s", strings.Join(got, "\n"))
	}
	for i, l := range got[1:] {
		if !strings.HasPrefix(l, "      ") || strings.HasPrefix(l, "       ") {
			t.Errorf("continuation %d does not hang at the leading indent: %q", i, l)
		}
	}
}

// TestWrapUsage_LeavesShortLinesAlone pins that wrapping is not reformatting.
func TestWrapUsage_LeavesShortLinesAlone(t *testing.T) {

	const line = "  -o, --output string   Output rendering."

	if got := wrapUsage(line, 100); got != line+"\n" {
		t.Errorf("a line inside the width was altered:\n%q", got)
	}
}

// TestWrapUsage_ZeroWidthIsPflagsMeaning pins the one value that means "do not wrap".
//
// It is pflag's own convention, and the reason help text never wrapped: cobra's template calls FlagUsages,
// which is FlagUsagesWrapped(0).
func TestWrapUsage_ZeroWidthIsPflagsMeaning(t *testing.T) {

	const line = "      a line comfortably longer than any width this test would otherwise impose upon it"

	if got := wrapUsage(line, 0); got != line {
		t.Errorf("width 0 wrapped anyway:\n%q", got)
	}
}

// TestWrapUsage_ANarrowWidthGivesTextTheWholeLine covers the floor.
//
// Honoring a hanging indent inside a narrow terminal leaves a sliver too thin to read, so the line falls
// back to its leading indent and spends the width on words instead.
func TestWrapUsage_ANarrowWidthGivesTextTheWholeLine(t *testing.T) {

	const line = "      csv            quoted and parseable; when a spreadsheet or a data tool reads it"

	got := strings.Split(strings.TrimRight(wrapUsage(line, 34), "\n"), "\n")

	for i, l := range got[1:] {
		if strings.HasPrefix(l, strings.Repeat(" ", 21)) {
			t.Errorf("continuation %d still hangs at the name column, leaving %d columns of text: %q",
				i, 34-21, l)
		}
	}
}

// endregion
