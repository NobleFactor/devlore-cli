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

	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return fmt.Errorf("result.TableFormatter: expected slice or array, got %T", value)
	}

	if rv.Len() == 0 {
		return nil
	}

	headers, headersFromValue := csvHeadersFromValue(value)
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
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return writer.Flush()
}

// endregion
