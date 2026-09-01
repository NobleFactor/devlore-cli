// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

// TableFormatter renders a slice of rows as aligned columns for a human to read.
//
// It shares its column inference with the delimited formats -- [HasHeaders], `csv:"name"` tag overrides, and
// the map-key union -- so one value renders the same columns whether it is asked for as `table`, `csv`, or
// `value`. Only the presentation differs: commas, tabs, or padding.
//
// Alignment is [text/tabwriter]'s, which measures cell widths in runes rather than bytes. A hand-rolled
// `%-30s` counts bytes and so misaligns every row containing a multi-byte character -- the defect #741
// records against lore's search table.
//
// Headers are upper-cased, matching `aws`, `kubectl`, and star's own table.
//
// This is the human rendering. A spreadsheet wants `csv`; a shell pipeline wants `value`.
type TableFormatter struct {

	// MinPadding is the space between the longest cell of a column and the next column. Zero means two.
	MinPadding int
}

// Compile-time interface guard.
var _ Formatter = TableFormatter{}

// NewTableFormatter returns the aligned-column renderer.
//
// Returns:
//   - `TableFormatter`: the formatter.
func NewTableFormatter() TableFormatter { return TableFormatter{MinPadding: 2} }

// region Formatter

// Format renders value as aligned columns to w.
//
// Parameters:
//   - `value`: a slice or array of structs or maps, or a value implementing [HasHeaders].
//   - `w`: the destination.
//
// Returns:
//   - `error`: when value is not a slice or array, or a row cannot be rendered.
func (f TableFormatter) Format(value any, w io.Writer) error {

	if value == nil {
		return nil
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	// A lone record is a sequence of one (§8's S3). A scalar is one row of one column, which is what
	// `--jq '.count' -o table` should print rather than an error.
	rv = asRecords(rv)

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

	padding := f.MinPadding
	if padding <= 0 {
		padding = 2
	}

	writer := tabwriter.NewWriter(w, 0, 0, padding, ' ', 0)

	upper := make([]string, len(headers))
	for i, header := range headers {
		upper[i] = strings.ToUpper(header)
	}
	if _, err := fmt.Fprintln(writer, strings.Join(upper, "\t")); err != nil {
		return err
	}

	for i := range rv.Len() {
		row := csvRowFromElement(rv.Index(i), headers)
		for j, cell := range row {
			row[j] = escapeControlCharacters(cell)
		}
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return writer.Flush()
}

// endregion

// region Helpers

// escapeControlCharacters renders a cell's newlines, tabs, and carriage returns as their two-character
// escapes.
//
// A table pads with spaces and has no quoting, so a cell carrying its own newline terminates the line and
// splits the record across two -- silently, and for the ordinary case rather than an exotic one, since every
// shell result's `stdout` ends in a newline (#748). A tab is worse: it reaches [text/tabwriter] as a column
// break and shifts every following cell.
//
// `csv` needs none of this. RFC 4180 quoting already keeps an embedded newline inside its field, where a
// parser reads it correctly, and escaping there would corrupt a value that round-trips today. `value` needs
// none of it either: raw is raw, which is the contract `gcloud` and `aws` state for the same rendering.
//
// Parameters:
//   - `cell`: the rendered cell text.
//
// Returns:
//   - `string`: the cell with control characters escaped.
func escapeControlCharacters(cell string) string {

	if !strings.ContainsAny(cell, "\n\r\t") {
		return cell
	}

	replacer := strings.NewReplacer("\n", `\n`, "\r", `\r`, "\t", `\t`)
	return replacer.Replace(cell)
}

// endregion
