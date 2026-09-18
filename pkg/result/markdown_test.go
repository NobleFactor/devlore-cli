// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"strings"
	"testing"
)

// region Tests

// TestMarkdownFormatter_AnswersEveryShape walks §8's eight shapes.
//
// The rule under test: a GFM table where the key inference yields headers, a bullet list where it does not,
// and a scalar string passed through as the document it already is.
func TestMarkdownFormatter_AnswersEveryShape(t *testing.T) {

	for _, testCase := range []struct {
		shape string
		name  string
		value any
		want  string
	}{
		{
			shape: "S1",
			name:  "a string is the document",
			value: "# Report\n\nOne line.\n",
			want:  "# Report\n\nOne line.\n",
		},
		{
			shape: "S1",
			name:  "a number is itself",
			value: 3,
			want:  "3\n",
		},
		{
			shape: "S1",
			name:  "a bool is itself",
			value: true,
			want:  "true\n",
		},
		{
			shape: "S1",
			name:  "null renders nothing",
			value: nil,
			want:  "",
		},
		{
			shape: "S2",
			name:  "an array of scalars is a bullet list",
			value: []string{"alpha", "beta"},
			want:  "- alpha\n- beta\n",
		},
		{
			shape: "S3",
			name:  "a flat object is one row",
			value: map[string]any{"name": "x", "state": "active"},
			want: "| name | state |\n" +
				"| --- | --- |\n" +
				"| x | active |\n",
		},
		{
			shape: "S4",
			name:  "a nested value is compact JSON",
			value: map[string]any{"name": "x", "health": map[string]any{"runs": 3}},
			want: "| health | name |\n" +
				"| --- | --- |\n" +
				`| {"runs":3} | x |` + "\n",
		},
		{
			shape: "S5",
			name:  "an array of flat objects is a table",
			value: []map[string]any{{"name": "x", "state": "active"}, {"name": "y", "state": "idle"}},
			want: "| name | state |\n" +
				"| --- | --- |\n" +
				"| x | active |\n" +
				"| y | idle |\n",
		},
		{
			shape: "S6",
			name:  "differing keys union, with holes",
			value: []map[string]any{{"name": "x", "state": "active"}, {"name": "y", "runs": 3}},
			want: "| name | runs | state |\n" +
				"| --- | --- | --- |\n" +
				"| x |  | active |\n" +
				"| y | 3 |  |\n",
		},
		{
			shape: "S7",
			name:  "an array of arrays is a bullet list",
			value: [][]string{{"a", "b"}, {"c", "d"}},
			want:  "- [\"a\",\"b\"]\n- [\"c\",\"d\"]\n",
		},
		{
			shape: "S8",
			name:  "an empty array renders nothing",
			value: []string{},
			want:  "",
		},
	} {
		t.Run(testCase.shape+"/"+testCase.name, func(t *testing.T) {
			if got := emit(t, "markdown", testCase.value); got != testCase.want {
				t.Errorf("markdown of %v =\n%q\nwant\n%q", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestMarkdownFormatter_ADocumentSurvivesUnchanged is the pass-through path.
//
// A command whose result is prose, `star docs starlark` among them, returns the document as one string. Every
// character of its markup must reach stdout: a heading that gained a pipe, or a table whose delimiter row was escaped, is not the
// document the command produced.
func TestMarkdownFormatter_ADocumentSurvivesUnchanged(t *testing.T) {

	document := strings.Join([]string{
		"# Schedule",
		"",
		"| Lane | Item |",
		"| --- | --- |",
		"| 1 | #840 — install leaves no registration behind |",
		"",
		"*Ruled 2026-09-14.*",
		"",
	}, "\n")

	if got := emit(t, "markdown", document); got != document {
		t.Errorf("markdown of a document =\n%q\nwant it unchanged:\n%q", got, document)
	}
}

// TestMarkdownFormatter_ADocumentGainsOneNewline pins the termination rule.
//
// Every formatter here ends its output with a newline, so a shell prompt never lands mid-line. A document
// that already ends in one must not gain a second and the blank line it would draw.
func TestMarkdownFormatter_ADocumentGainsOneNewline(t *testing.T) {

	for _, testCase := range []struct{ name, document, want string }{
		{"already terminated", "# Report\n", "# Report\n"},
		{"unterminated", "# Report", "# Report\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := emit(t, "markdown", testCase.document); got != testCase.want {
				t.Errorf("markdown = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestMarkdownFormatter_NamesTheColumnsTableNames proves the inference is shared rather than copied.
//
// One value names the same columns in the same order whether it is asked for as `markdown` or as `table`.
// `table` upper-cases its headers and `markdown` does not, which is presentation; the names and their order
// are the derivation, and those must agree.
//
// The names come from the `json:` tags because stage 1 is total: [result.Pipeline.Emit] marshals to JSON
// before any formatter runs, so a `csv:` tag reaches a formatter only when one is called directly on a Go
// value. Through the pipeline -- which is every path a user takes -- JSON has already named the fields.
func TestMarkdownFormatter_NamesTheColumnsTableNames(t *testing.T) {

	rows := []struct {
		Name    string `json:"name"`
		Skipped string `json:"-"`
		Version string `json:"version"`
	}{{"alpha", "hidden", "1.0"}}

	markdown := strings.SplitN(emit(t, "markdown", rows), "\n", 2)[0]
	table := strings.SplitN(emit(t, "table", rows), "\n", 2)[0]

	columns := strings.Split(strings.Trim(strings.TrimSpace(markdown), "|"), "|")
	for i := range columns {
		columns[i] = strings.TrimSpace(columns[i])
	}

	if want := []string{"name", "version"}; strings.Join(columns, ",") != strings.Join(want, ",") {
		t.Fatalf("markdown columns = %v, want %v (the `json:\"-\"` field is omitted)", columns, want)
	}

	offset := 0
	for _, column := range columns {
		index := strings.Index(table[offset:], strings.ToUpper(column))
		if index < 0 {
			t.Errorf("table header %q does not carry %q in order", table, strings.ToUpper(column))
			continue
		}
		offset += index + len(column)
	}
}

// TestMarkdownFormatter_ACellNeverEndsItsRow is the markdown analog of #748.
//
// A GFM row is delimited by a newline and by pipes. A cell carrying either would end the row early and
// render one record as two, or as a row with the wrong column count -- silently. Every shell result's
// `stdout` ends in a newline, so this is the ordinary case rather than an exotic one.
func TestMarkdownFormatter_ACellNeverEndsItsRow(t *testing.T) {

	got := emit(t, "markdown", map[string]any{"stdout": "one\ntwo\n", "pattern": "a|b"})

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (header, delimiter, one row): %q", len(lines), got)
	}

	if !strings.Contains(lines[2], `a\|b`) {
		t.Errorf("row = %q, want the pipe backslash-escaped", lines[2])
	}
	if !strings.Contains(lines[2], `one\ntwo\n`) {
		t.Errorf("row = %q, want the newlines rendered as their two-character escapes", lines[2])
	}
}

// endregion
