// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// ValueFormatter writes values as written: no quoting, no document syntax, no header.
//
// It is what completes the filter stage. `--jq` can build any string -- `.entries[] | "\(.target) is
// \(.state)"` -- but every other formatter then imposes a syntax on it: json quotes and escapes it, yaml
// applies its own scalar rules, csv adds a header. This one prints it.
//
// A slice or array emits one element per line. Anything else emits one line. Elements render through
// fmt.Sprint, which honors fmt.Stringer, so a resource prints as its URI and a time as its formatted form.
//
// `aws` ships the same rendering as `text` and `gcloud` as `value`; both exist for the same reason, and
// gcloud documents its as "CSV with no heading and <TAB> separator", which is this once a row has one field.
type ValueFormatter struct {

	// Separator joins the fields of a multi-field row. The zero value is a tab, matching `gcloud value`.
	Separator string
}

// Compile-time interface guard.
var _ Formatter = ValueFormatter{}

// NewValueFormatter returns the raw renderer, tab-separated between fields of one row.
//
// Returns:
//   - `ValueFormatter`: the formatter.
func NewValueFormatter() ValueFormatter { return ValueFormatter{Separator: "\t"} }

// region Formatter

// Format writes value to w with no syntax of its own.
//
// Parameters:
//   - `value`: the value to render.
//   - `w`: the destination.
//
// Returns:
//   - `error`: any write error.
func (f ValueFormatter) Format(value any, w io.Writer) error {

	if value == nil {
		return nil
	}

	separator := f.Separator
	if separator == "" {
		separator = "\t"
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	// A string is a single line even though it is not a slice of runes to this formatter.
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		_, err := fmt.Fprintln(w, string(rv.Bytes()))
		return err
	}

	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		_, err := fmt.Fprintln(w, valueLine(value, separator))
		return err
	}

	for i := range rv.Len() {
		if _, err := fmt.Fprintln(w, valueLine(rv.Index(i).Interface(), separator)); err != nil {
			return err
		}
	}

	return nil
}

// endregion

// region Helpers

// valueLine renders one row: a scalar as itself, a multi-field row with its fields joined.
//
// Parameters:
//   - `row`: the row to render.
//   - `separator`: the string between fields of a multi-field row.
//
// Returns:
//   - `string`: the rendered line.
func valueLine(row any, separator string) string {

	rv := reflect.ValueOf(row)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		fields := make([]string, 0, rv.Len())
		for i := range rv.Len() {
			fields = append(fields, fmt.Sprint(rv.Index(i).Interface()))
		}
		return strings.Join(fields, separator)

	case reflect.Struct:
		fields := make([]string, 0, rv.NumField())
		for i := range rv.NumField() {
			if rv.Type().Field(i).IsExported() {
				fields = append(fields, fmt.Sprint(rv.Field(i).Interface()))
			}
		}
		return strings.Join(fields, separator)
	}

	return fmt.Sprint(row)
}

// endregion
