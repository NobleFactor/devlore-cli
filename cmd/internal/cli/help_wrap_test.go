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

// TestWrapLong_ProseReflowsToTheWidth is #759's premise: a paragraph an author wrapped at 80 reflows to the
// terminal, at 60 and at 100 alike, with no line over the width.
func TestWrapLong_ProseReflowsToTheWidth(t *testing.T) {

	const long = "Report deployed state: what should be present, where it should have come from, and\n" +
		"what's missing or different.\n" +
		"\n" +
		"The report is derived from the store (the run index plus the persisted graphs and\n" +
		"traces) -- never from a directory scan -- and has four sections: the registered layer\n" +
		"tree, the deployed inventory classified against the live filesystem, the package\n" +
		"operations writ's runs performed, and store health."

	for _, width := range []int{60, 100} {
		got := strings.Split(wrapLong(long, width), "\n")
		for i, l := range got {
			if len([]rune(l)) > width {
				t.Errorf("width %d: line %d is %d columns: %q", width, i, len([]rune(l)), l)
			}
		}
		blank := 0
		for _, l := range got {
			if l == "" {
				blank++
			}
		}
		if blank != 1 {
			t.Errorf("width %d: %d blank lines, want the one that separates the paragraphs:\n%s", width, blank, strings.Join(got, "\n"))
		}
	}
	// The author broke the first line at 82 columns; at 100 the paragraph packs past it.
	if first := strings.SplitN(wrapLong(long, 100), "\n", 2)[0]; len([]rune(first)) <= 82 {
		t.Errorf("at 100 the first paragraph did not reflow past the authored break: %q", first)
	}
}

// TestWrapLong_StructureKeepsItsBreaks is why #755 left `Long` alone: a policy ladder is one entry per line, and
// stays so at any width, indent intact.
func TestWrapLong_StructureKeepsItsBreaks(t *testing.T) {

	const long = "What each outcome does to the exit status is the signing policy ladder:\n" +
		"\n" +
		"  ignore           No verification at all\n" +
		"  report           (default) Report every outcome; never fail\n" +
		"  reject           Reject anything that is not valid"

	got := strings.Split(wrapLong(long, 100), "\n")
	want := strings.Split(long, "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d changed:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// TestWrapLong_AnOverLongStructureLineHangsUnderItsColumn: a table row too wide for the terminal wraps as a
// usage line does, its continuation under the sentence, not under the name.
func TestWrapLong_AnOverLongStructureLineHangsUnderItsColumn(t *testing.T) {

	const long = "  modified-or-stale  Differs, but the run predates recorded content identity: attribution indeterminate"

	got := strings.Split(wrapLong(long, 60), "\n")
	if len(got) < 2 {
		t.Fatalf("did not wrap at 60: %q", got)
	}
	for i, l := range got {
		if len([]rune(l)) > 60 {
			t.Errorf("line %d is %d columns: %q", i, len([]rune(l)), l)
		}
	}
	if !strings.HasPrefix(got[1], strings.Repeat(" ", 21)) || strings.HasPrefix(got[1], strings.Repeat(" ", 22)) {
		t.Errorf("continuation does not hang under the sentence column (21): %q", got[1])
	}
}

// TestWrapLong_ZeroWidthIsUntouched mirrors pflag's meaning of zero: no wrapping at all.
func TestWrapLong_ZeroWidthIsUntouched(t *testing.T) {

	const long = "a line the author wrapped\nand its continuation"
	if got := wrapLong(long, 0); got != long {
		t.Errorf("width 0 changed the text: %q", got)
	}
}

// TestWrapLong_MeasuresRunes: a multi-byte character counts as one column, so the wrap lands where it looks.
func TestWrapLong_MeasuresRunes(t *testing.T) {

	long := strings.Repeat("é ", 25) + "fin"
	for i, l := range strings.Split(wrapLong(long, 20), "\n") {
		if len([]rune(l)) > 20 {
			t.Errorf("line %d is %d columns: %q", i, len([]rune(l)), l)
		}
	}
}

// TestWrapLong_TrailingWhitespaceGoes: every line is right-trimmed, so nothing an author left survives.
func TestWrapLong_TrailingWhitespaceGoes(t *testing.T) {

	for _, l := range strings.Split(wrapLong("prose with a trailing space \n  structure too \n", 80), "\n") {
		if strings.TrimRight(l, " ") != l {
			t.Errorf("trailing whitespace survived: %q", l)
		}
	}
}

// endregion
