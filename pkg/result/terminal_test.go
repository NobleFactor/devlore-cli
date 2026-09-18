// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// region Tests

// TestTerminalFormatter_ConsumesEveryMarker is what separates this rendering from `markdown` and from glamour's
// own `notty` style, both of which keep the markers.
func TestTerminalFormatter_ConsumesEveryMarker(t *testing.T) {

	document := "# Title\n\n## Section\n\nLanes **five** through *seven*, see `terminal`, not ~~plain~~.\n"
	visible := escapeSequence.ReplaceAllString(emit(t, "terminal", document), "")

	for _, marker := range []string{"#", "**", "*seven*", "`", "~~"} {
		if strings.Contains(visible, marker) {
			t.Errorf("rendered text still carries the marker %q:\n%s", marker, visible)
		}
	}

	for _, word := range []string{"Title", "Section", "five", "seven", "terminal", "plain"} {
		if !strings.Contains(visible, word) {
			t.Errorf("rendered text lost %q:\n%s", word, visible)
		}
	}
}

// TestTerminalFormatter_MarkersBecomeAttributes proves the consumed markers came back as styling, and that
// nothing asked the destination whether it is a terminal.
//
// The destination here is a buffer, which is not a terminal. A renderer that probed would see that and emit no
// escape codes at all; finding bold, italic, and underline in the buffer is the proof that it did not.
func TestTerminalFormatter_MarkersBecomeAttributes(t *testing.T) {

	rendered := emit(t, "terminal", "# Title\n\nLanes **five** through *seven*.\n")

	for _, testCase := range []struct {
		word string
		want []string
	}{
		{"Title", []string{"1", "3", "4"}},
		{"five", []string{"1"}},
		{"seven", []string{"3"}},
	} {
		parameters := parametersBefore(rendered, testCase.word)
		for _, parameter := range testCase.want {
			if !slices.Contains(parameters, parameter) {
				t.Errorf("%q is styled with %v, want it to include SGR %s:\n%q",
					testCase.word, parameters, parameter, rendered)
			}
		}
	}
}

// TestTerminalFormatter_NoColorKeepsAttributes pins no-color.org's rule as §10 records it.
//
// The variable suppresses color and nothing else, so bold and italic survive. The code block is the case that
// would slip: chroma highlights it with escape codes of its own, and only glamour's check of the color profile
// keeps chroma out of it.
func TestTerminalFormatter_NoColorKeepsAttributes(t *testing.T) {

	t.Setenv("NO_COLOR", "1")

	document := "# Title\n\nLanes **five** through *seven*, see `terminal`.\n\n```go\nfunc main() {}\n```\n"
	rendered := emit(t, "terminal", document)

	if parameters := parametersBefore(rendered, "five"); !slices.Contains(parameters, "1") {
		t.Errorf("%q is styled with %v, want bold to survive NO_COLOR:\n%q", "five", parameters, rendered)
	}
	if parameters := parametersBefore(rendered, "seven"); !slices.Contains(parameters, "3") {
		t.Errorf("%q is styled with %v, want italic to survive NO_COLOR:\n%q", "seven", parameters, rendered)
	}

	for _, sequence := range escapeSequence.FindAllString(rendered, -1) {
		body := strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), "m")
		for _, parameter := range strings.Split(body, ";") {
			if isColorParameter(parameter) {
				t.Errorf("NO_COLOR is set and the output carries the color sequence %q:\n%q", sequence, rendered)
			}
		}
	}
}

// TestTerminalFormatter_RendersData proves the chain composes: records become a markdown table, and the table is
// drawn with box characters rather than pipes.
func TestTerminalFormatter_RendersData(t *testing.T) {

	rendered := emit(t, "terminal", []map[string]any{{"lane": 5, "item": "#826"}, {"lane": 6, "item": "#895"}})
	visible := escapeSequence.ReplaceAllString(rendered, "")

	for _, want := range []string{"item", "lane", "#826", "#895", "│"} {
		if !strings.Contains(visible, want) {
			t.Errorf("rendered table is missing %q:\n%s", want, visible)
		}
	}

	if strings.Contains(visible, "| --- |") {
		t.Errorf("rendered table still carries the markdown delimiter row:\n%s", visible)
	}
}

// TestTerminalFormatter_NoLineEndsInPadding pins the trim.
//
// glamour pads every line out to the wrap width, so a reader copying a report out of the terminal collects
// trailing spaces and a diff of two renderings is noise. Styling must survive the trim: a style dropped with the
// padding would bleed onto the next line.
func TestTerminalFormatter_NoLineEndsInPadding(t *testing.T) {

	rendered := emit(t, "terminal", "# Title\n\nLanes **five** through *seven*.\n\n- alpha\n- beta\n")

	for number, line := range strings.Split(rendered, "\n") {
		visible := escapeSequence.ReplaceAllString(line, "")
		if visible != strings.TrimRight(visible, " \t") {
			t.Errorf("line %d ends in padding: %q", number+1, line)
		}
	}

	if parameters := parametersBefore(rendered, "five"); !slices.Contains(parameters, "1") {
		t.Errorf("%q is styled with %v, want bold to survive the trim:\n%q", "five", parameters, rendered)
	}
	for _, word := range []string{"Title", "five", "seven", "alpha", "beta"} {
		if !strings.Contains(escapeSequence.ReplaceAllString(rendered, ""), word) {
			t.Errorf("the trim lost %q:\n%q", word, rendered)
		}
	}
}

// TestTerminalFormatter_EmptyRendersNothing is §8's S8: nothing, not an empty document with its padding.
func TestTerminalFormatter_EmptyRendersNothing(t *testing.T) {

	for _, value := range []any{nil, []string{}} {
		if got := emit(t, "terminal", value); got != "" {
			t.Errorf("terminal of %v = %q, want nothing", value, got)
		}
	}
}

// TestTerminalFormatter_AWindowsPathKeepsItsBackslashes is the defect this rendering had on Windows.
//
// Measured on `danoble-wd11-3.local`, 2026-09-18: `--output markdown` printed
// `C:\Users\david-noble\.local\share` and `--output terminal` printed `C:\Users\david-noble.local\share`.
// The chain is the cause -- `markdown` is source, and goldmark reads a backslash as an escape -- so the cell
// has to carry its backslashes doubled for the second reader.
func TestTerminalFormatter_AWindowsPathKeepsItsBackslashes(t *testing.T) {

	const root = `C:\Users\david-noble\.local\share\devlore`

	rendered := escapeSequence.ReplaceAllString(emit(t, "terminal", map[string]any{"root": root}), "")

	if !strings.Contains(rendered, root) {
		t.Errorf("the path lost a backslash:\n%s\nwant it to contain %q", rendered, root)
	}
}

// endregion

// region Helpers

// escapeSequence matches one SGR escape sequence.
var escapeSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// isColorParameter reports whether an SGR parameter sets a foreground or background color.
//
// 30 to 38 and 90 to 97 are foreground, 40 to 48 and 100 to 107 background; 38 and 48 introduce the 256-color and
// true-color forms. 39 and 49 restore the defaults and are not colors.
//
// Parameters:
//   - `parameter`: one SGR parameter.
//
// Returns:
//   - `bool`: true when the parameter sets a color.
func isColorParameter(parameter string) bool {

	value, err := strconv.Atoi(parameter)
	if err != nil {
		return false
	}

	return (value >= 30 && value <= 38) || (value >= 40 && value <= 48) ||
		(value >= 90 && value <= 97) || (value >= 100 && value <= 107)
}

// parametersBefore returns the parameters of every SGR sequence between the previous reset and the first
// occurrence of word.
//
// glamour styles text a token at a time, so a word is preceded by the sequences that style it, and those follow
// the reset that ended the token before.
//
// Parameters:
//   - `rendered`: the rendered output.
//   - `word`: the text whose styling is wanted.
//
// Returns:
//   - `[]string`: the SGR parameters in force for word; nil when word is absent.
func parametersBefore(rendered, word string) []string {

	index := strings.Index(rendered, word)
	if index < 0 {
		return nil
	}

	var parameters []string
	for _, sequence := range escapeSequence.FindAllString(rendered[:index], -1) {
		body := strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), "m")
		if body == "" || body == "0" {
			parameters = nil
			continue
		}
		parameters = append(parameters, strings.Split(body, ";")...)
	}

	return parameters
}

// endregion
