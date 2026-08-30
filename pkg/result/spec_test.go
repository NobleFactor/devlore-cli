// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region NAME=ARGUMENT

// TestFormatSpec_BareNamesUnchanged pins that adding the argument form did not disturb plain names.
func TestFormatSpec_BareNamesUnchanged(t *testing.T) {

	for _, name := range []string{"csv", "json", "none", "table", "value", "yaml"} {
		if _, err := result.FormatterByName(name); err != nil {
			t.Errorf("FormatterByName(%q): %v", name, err)
		}
	}
}

// TestFormatSpec_SplitsOnTheFirstEquals is the parse rule.
//
// A template body containing '=' must survive intact; splitting on the last, or on every, '=' would corrupt
// it. The body below is only valid if the split took the first one.
func TestFormatSpec_SplitsOnTheFirstEquals(t *testing.T) {

	formatter, err := result.FormatterByName(`template={{if eq .Name "a=b"}}yes{{end}}`)
	if err != nil {
		t.Fatalf("FormatterByName: %v", err)
	}

	var buffer bytes.Buffer
	if err := formatter.Format(struct{ Name string }{Name: "a=b"}, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if buffer.String() != "yes" {
		t.Errorf("rendered %q, want %q -- the argument lost its '='", buffer.String(), "yes")
	}
}

// TestFormatSpec_ArgumentErrors covers the three ways a spec can be malformed.
func TestFormatSpec_ArgumentErrors(t *testing.T) {

	for _, testCase := range []struct{ spec, wants string }{
		{"template", "needs a body"},
		{"template=", "empty argument"},
		{"json=oops", "takes no argument"},
		{"xml", "unknown formatter"},
	} {
		t.Run(testCase.spec, func(t *testing.T) {

			_, err := result.FormatterByName(testCase.spec)
			if err == nil {
				t.Fatalf("FormatterByName(%q) succeeded, want an error", testCase.spec)
			}
			if !strings.Contains(err.Error(), testCase.wants) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), testCase.wants)
			}
		})
	}
}

// endregion

// region value

// TestValueFormatter_PrintsStringsAsWritten is the property the format exists for.
//
// `--jq` can build any line; every other format then imposes a syntax on it. json would quote and escape
// this, csv would add a header. This is what completes the filter stage.
func TestValueFormatter_PrintsStringsAsWritten(t *testing.T) {

	var buffer bytes.Buffer
	built := []any{"~/.zshrc is current", "~/.vimrc is drifted"}

	if err := result.NewValueFormatter().Format(built, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	want := "~/.zshrc is current\n~/.vimrc is drifted\n"
	if buffer.String() != want {
		t.Errorf("rendered %q, want %q", buffer.String(), want)
	}
	if strings.Contains(buffer.String(), `"`) {
		t.Error("value added quoting; it must print what it was given")
	}
}

// TestValueFormatter_JoinsRowFieldsWithTabs covers the multi-field row, matching `gcloud value`.
func TestValueFormatter_JoinsRowFieldsWithTabs(t *testing.T) {

	var buffer bytes.Buffer
	rows := []struct {
		Name    string
		Version string
	}{{"alpha", "1.0"}, {"beta", "2.0"}}

	if err := result.NewValueFormatter().Format(rows, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2: %q", len(lines), buffer.String())
	}
	if lines[0] != "alpha\t1.0" {
		t.Errorf("first line = %q, want %q", lines[0], "alpha\t1.0")
	}
}

// TestValueFormatter_ScalarIsOneLine covers the single-value case a `--jq '.name'` produces.
func TestValueFormatter_ScalarIsOneLine(t *testing.T) {

	var buffer bytes.Buffer
	if err := result.NewValueFormatter().Format("alpha", &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if buffer.String() != "alpha\n" {
		t.Errorf("rendered %q, want %q", buffer.String(), "alpha\n")
	}
}

// endregion
