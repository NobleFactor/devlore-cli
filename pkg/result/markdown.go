// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// MarkdownFormatter renders a value as a markdown document.
//
// It shares its column inference with [TableFormatter] and the delimited formats -- [HasHeaders],
// `csv:"name"` tag overrides, and the map-key union -- so one value names the same columns whether it is
// asked for as `markdown`, `table`, `csv`, or `value`. Only the presentation differs.
//
// Two rules decide the shape, and between them they answer §8's eight:
//
//   - **A scalar string passes through verbatim.** A command whose result is prose returns the document
//     itself, and a document is not a datum to be laid out. `star docs starlark` is one.
//   - **Otherwise: a GFM table where the inference yields headers, a bullet list where it does not.**
//     GitHub-flavored markdown has no headerless table -- the delimiter row is required -- and synthesizing
//     column names would name fields the data does not have.
//
// Headers keep the case the JSON gives them, unlike [TableFormatter], which upper-cases. These are the names
// the `json:` tags declare and the names the Starlark surface shows a customer; a markdown document is read
// by people and by GitHub, and neither wants them shouted.
//
// A non-scalar cell renders as compact JSON, at any depth, never truncated -- the rule §8 states for every
// presentation that lays data out.
type MarkdownFormatter struct{}

// Compile-time interface guard.
var _ Formatter = MarkdownFormatter{}

// NewMarkdownFormatter returns the markdown document renderer.
//
// Returns:
//   - `MarkdownFormatter`: the formatter.
func NewMarkdownFormatter() MarkdownFormatter { return MarkdownFormatter{} }

// region Formatter

// Format renders value as a markdown document to w.
//
// Parameters:
//   - `value`: any normalized value; a string is the document itself.
//   - `w`: the destination.
//
// Returns:
//   - `error`: any write error.
func (f MarkdownFormatter) Format(value any, w io.Writer) error {

	if value == nil {
		return nil
	}

	rv := indirect(reflect.ValueOf(value))
	if !rv.IsValid() {
		return nil
	}

	// The document itself. `json.Number` is a string kind and is not one, so the test is on the type.
	if rv.Kind() == reflect.String && rv.Type() != jsonNumberType {
		return writeDocument(w, rv.String())
	}

	// A lone record is a sequence of one (§8's S3).
	rv = asRecords(rv)

	// Still not a sequence: a scalar, and a scalar is the whole document.
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		_, err := fmt.Fprintln(w, escapeControlCharacters(csvCellValue(rv)))
		return err
	}

	if rv.Len() == 0 {
		return nil
	}

	headers, headersFromValue := csvHeadersFromValue(rv.Interface())
	if !headersFromValue {
		headers = csvHeadersFromElements(rv)
	}

	if len(headers) == 0 {
		return writeBulletList(rv, w)
	}

	return writeTable(rv, headers, w)
}

// endregion

// region Helpers

// jsonNumberType distinguishes a decoded number from a string. [normalize] decodes with `UseNumber`, so
// every number arrives as a [json.Number], whose kind is [reflect.String].
var jsonNumberType = reflect.TypeOf(json.Number(""))

// writeDocument writes a scalar string as the document it already is, terminated by exactly one newline.
//
// Termination rather than verbatim bytes: every formatter in this package ends its output with a newline, a
// shell prompt that lands mid-line is a defect, and a document that already ends in one must not gain a
// blank line for being emitted twice.
//
// Parameters:
//   - `w`: the destination.
//   - `document`: the markdown source.
//
// Returns:
//   - `error`: any write error.
func writeDocument(w io.Writer, document string) error {

	if strings.HasSuffix(document, "\n") {
		_, err := io.WriteString(w, document)
		return err
	}

	_, err := fmt.Fprintln(w, document)
	return err
}

// writeBulletList writes one bullet per record, for the shapes whose inference yields no headers: an array
// of scalars, and an array of arrays.
//
// Parameters:
//   - `rv`: the sequence of records.
//   - `w`: the destination.
//
// Returns:
//   - `error`: any write error.
func writeBulletList(rv reflect.Value, w io.Writer) error {

	for i := range rv.Len() {
		cells := csvRowFromElement(rv.Index(i), nil)
		for j, cell := range cells {
			cells[j] = escapeControlCharacters(escapeMarkdownText(cell))
		}
		if _, err := fmt.Fprintf(w, "- %s\n", strings.Join(cells, ", ")); err != nil {
			return err
		}
	}

	return nil
}

// writeTable writes a GitHub-flavored markdown table: a header row, the delimiter row GFM requires, and one
// row per record.
//
// Parameters:
//   - `rv`: the sequence of records.
//   - `headers`: the column names, in order.
//   - `w`: the destination.
//
// Returns:
//   - `error`: any write error.
func writeTable(rv reflect.Value, headers []string, w io.Writer) error {

	if err := writeTableRow(w, headers); err != nil {
		return err
	}

	delimiters := make([]string, len(headers))
	for i := range delimiters {
		delimiters[i] = "---"
	}
	if _, err := fmt.Fprintf(w, "| %s |\n", strings.Join(delimiters, " | ")); err != nil {
		return err
	}

	for i := range rv.Len() {
		if err := writeTableRow(w, csvRowFromElement(rv.Index(i), headers)); err != nil {
			return err
		}
	}

	return nil
}

// writeTableRow writes one pipe-delimited row with its cells escaped.
//
// Parameters:
//   - `w`: the destination.
//   - `cells`: the row's cells, already in column order.
//
// Returns:
//   - `error`: any write error.
func writeTableRow(w io.Writer, cells []string) error {

	escaped := make([]string, len(cells))
	for i, cell := range cells {
		escaped[i] = escapeTableCell(cell)
	}

	_, err := fmt.Fprintf(w, "| %s |\n", strings.Join(escaped, " | "))
	return err
}

// escapeTableCell makes a cell safe inside a GFM table row.
//
// A row is delimited by newlines and by pipes, so a cell carrying either would end the row early and split
// the record across two -- silently, and for the ordinary case rather than an exotic one, since a compact
// JSON cell holding a shell result carries both. Newlines, tabs, and carriage returns become their
// two-character escapes, as they do in `table`; a pipe is backslash-escaped, which is GFM's own mechanism
// and renders as a pipe.
//
// Parameters:
//   - `cell`: the rendered cell text.
//
// Returns:
//   - `string`: the cell, safe between two pipes.
func escapeTableCell(cell string) string {

	return strings.ReplaceAll(escapeControlCharacters(escapeMarkdownText(cell)), "|", `\|`)
}

// escapeMarkdownText makes literal text survive being read as markdown.
//
// A backslash is markdown's escape character, so `C:\Users\me\.local` read back as markdown is
// `C:\Users\me.local` -- the backslash before the dot is consumed and the path is silently wrong. `markdown`
// is source: it is pasted into an issue, and [TerminalFormatter] parses it. Both readers apply the rule, so
// the text has to carry its own backslashes doubled.
//
// Only the backslash is doubled here. The other metacharacters -- asterisk, underscore, backtick -- are
// markup a reader may well have meant, and a cell is data rather than prose; a Windows path is the case that
// occurs, measured on `danoble-wd11-3.local` on 2026-09-18.
//
// This runs BEFORE [escapeControlCharacters], never after: that function writes a newline as the two
// characters `\` and `n`, and doubling its backslash afterwards would render `\\n` and put a literal
// backslash in the reader's cell.
//
// Parameters:
//   - `text`: the rendered text.
//
// Returns:
//   - `string`: the text with its backslashes doubled.
func escapeMarkdownText(text string) string {

	return strings.ReplaceAll(text, `\`, `\\`)
}

// endregion
