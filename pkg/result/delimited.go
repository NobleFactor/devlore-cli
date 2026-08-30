// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"encoding/csv"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// DelimitedFormatter renders values as delimiter-separated lines.
//
// One formatter, three attributes, three presets. gcloud reached the same shape: its `csv` and `value` are
// one renderer differing in separator, heading, and quoting, with `value` documented as "CSV with no heading
// and <TAB> separator instead of <COMMA>".
//
//   - [NewCSVFormatter]   -- comma, heading, quoted. RFC 4180, for a spreadsheet.
//   - [NewTSVFormatter]   -- tab, no heading, quoted. For `cut -f2` and `awk '{print $2}'`.
//   - [NewValueFormatter] -- tab, no heading, raw. For text `--jq` already built.
//
// Quoting is what separates the last from the middle. A machine-parseable format must quote a field
// containing its own delimiter or the row loses a boundary; a raw renderer must not, or the text a caller
// composed comes back wearing quotes it did not write.
//
// Shape drives column inference:
//
//   - If value implements [HasHeaders], its Headers() result is the column order.
//   - Otherwise, if value is a slice/array of structs, the struct fields (in declaration order, with
//     `csv:"name"` overrides and `csv:"-"` skips) are the columns.
//   - Otherwise, if value is a slice/array of maps, the union of map keys (sorted alphabetically) is
//     the columns.
//   - Otherwise the elements are scalars: one column, no header, since a scalar has no field to name.
//
// A value that is not a slice or array at all is one row. That is the shape `--jq '.count'` produces, and
// erroring on it would make the filter stage incomplete.
//
// Cell values render via fmt.Sprint, which honors fmt.Stringer and produces reasonable defaults for numbers,
// bools, time.Time, and nil. Empty input renders no bytes.
type DelimitedFormatter struct {

	// Separator is the field delimiter. The zero value is a comma.
	Separator rune

	// SuppressHeadings omits the header row. A spreadsheet wants it; a shell pipeline does not, since awk
	// and cut would each have to skip it.
	SuppressHeadings bool

	// Raw disables RFC 4180 quoting. Set for a renderer whose job is to print what it was given.
	Raw bool
}

// NewCSVFormatter returns the spreadsheet preset: comma, heading, quoted.
//
// Returns:
//   - `DelimitedFormatter`: the RFC 4180 formatter.
func NewCSVFormatter() DelimitedFormatter { return DelimitedFormatter{Separator: ','} }

// NewTSVFormatter returns the pipeline preset: tab, no heading, quoted.
//
// Tabs need no quoting for the values a row normally carries, so a line survives `cut -f2` without a parser.
// Quoting stays on so a field that does contain a tab cannot silently split the row.
//
// Returns:
//   - `DelimitedFormatter`: the tab-separated formatter.
func NewTSVFormatter() DelimitedFormatter {
	return DelimitedFormatter{Separator: '\t', SuppressHeadings: true}
}

// NewValueFormatter returns the raw preset: tab, no heading, no quoting.
//
// This is what completes the filter stage. `--jq` can build any line; every other format then imposes a
// syntax on it -- json quotes and escapes, yaml applies scalar rules, csv adds a header. This one prints it.
// `aws` ships the same rendering as `text` and `gcloud` as `value`.
//
// Returns:
//   - `DelimitedFormatter`: the raw formatter.
func NewValueFormatter() DelimitedFormatter {
	return DelimitedFormatter{Separator: '\t', SuppressHeadings: true, Raw: true}
}

// HasHeaders is the opt-in interface that overrides automatic header inference. Implementations are
// typically named-slice types whose row shape doesn't lend itself to reflection (e.g., heterogeneous
// row content keyed by symbolic names).
type HasHeaders interface {

	// Headers returns the column order to use when rendering the receiver as CSV.
	Headers() []string
}

// Compile-time interface guard.
var _ Formatter = DelimitedFormatter{}

// region Formatter

// Format renders value as delimiter-separated values to w.
//
// Returns an error if value is not a slice/array of structs or maps and does not implement
// [HasHeaders].
func (f DelimitedFormatter) Format(value any, w io.Writer) error {

	if value == nil {
		return nil
	}

	rv := indirect(reflect.ValueOf(value))
	if !rv.IsValid() {
		return nil
	}

	// A value that is not a sequence is one row. `--jq '.count'` produces this, and refusing it would make
	// the filter stage incomplete.
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return f.emit(w, nil, [][]string{{csvCellValue(rv)}})
	}

	// A byte slice is one value, not a sequence of numbers.
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return f.emit(w, nil, [][]string{{string(rv.Bytes())}})
	}

	if rv.Len() == 0 {
		return nil
	}

	headers, headersFromValue := csvHeadersFromValue(value)
	if !headersFromValue {
		headers = csvHeadersFromElements(rv)
	}

	rows := make([][]string, 0, rv.Len())
	for i := range rv.Len() {
		rows = append(rows, csvRowFromElement(rv.Index(i), headers))
	}

	return f.emit(w, headers, rows)
}

// emit writes the header and rows, quoted or raw.
//
// Parameters:
//   - `w`: the destination.
//   - `headers`: the column names, or nil when the rows are scalars.
//   - `rows`: the rendered cells.
//
// Returns:
//   - `error`: any write error.
func (f DelimitedFormatter) emit(w io.Writer, headers []string, rows [][]string) error {

	separator := f.Separator
	if separator == 0 {
		separator = ','
	}

	if f.Raw {
		for _, row := range rows {
			if _, err := fmt.Fprintln(w, strings.Join(row, string(separator))); err != nil {
				return err
			}
		}
		return nil
	}

	writer := csv.NewWriter(w)
	writer.Comma = separator

	if !f.SuppressHeadings && len(headers) > 0 {
		if err := writer.Write(headers); err != nil {
			return err
		}
	}

	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

// endregion

// region Header inference

// csvHeadersFromValue returns the headers when value implements [HasHeaders]. The second return
// reports whether the interface was satisfied.
func csvHeadersFromValue(value any) ([]string, bool) {

	if hh, ok := value.(HasHeaders); ok {
		return hh.Headers(), true
	}
	return nil, false
}

// csvHeadersFromElements infers headers from the first element's shape. Slice-of-structs uses field
// declaration order; slice-of-maps uses the union of keys across every element, sorted alphabetically.
func csvHeadersFromElements(rv reflect.Value) []string {

	first := indirect(rv.Index(0))
	switch first.Kind() {

	case reflect.Struct:
		return csvHeadersFromStruct(first.Type())

	case reflect.Map:
		return csvHeadersFromMaps(rv)

	default:
		// Scalars have no field to name, so there is no header row.
		return nil
	}
}

// csvHeadersFromStruct returns the exported field names of t in declaration order. Fields tagged
// `csv:"name"` use name; fields tagged `csv:"-"` are omitted; unexported fields are skipped silently.
// Embedded struct fields are promoted per [reflect.VisibleFields].
func csvHeadersFromStruct(t reflect.Type) []string {

	visible := reflect.VisibleFields(t)
	headers := make([]string, 0, len(visible))
	for _, field := range visible {
		if !field.IsExported() {
			continue
		}
		name, skip := csvFieldName(field)
		if skip {
			continue
		}
		headers = append(headers, name)
	}
	return headers
}

// csvFieldName returns the column name for a struct field and whether the field should be skipped.
// The `csv:"name"` tag overrides field.Name; the `csv:"-"` tag skips the field entirely.
func csvFieldName(field reflect.StructField) (name string, skip bool) {

	tag := field.Tag.Get("csv")
	switch tag {
	case "":
		return field.Name, false
	case "-":
		return "", true
	default:
		return tag, false
	}
}

// csvHeadersFromMaps returns the sorted union of keys across every map element of rv. Every element
// must be a map; mixed-type slices error at the element-iteration step.
func csvHeadersFromMaps(rv reflect.Value) []string {

	seen := make(map[string]struct{})
	for i := range rv.Len() {
		element := indirect(rv.Index(i))
		if element.Kind() != reflect.Map {
			continue
		}
		for _, key := range element.MapKeys() {
			seen[fmt.Sprint(key.Interface())] = struct{}{}
		}
	}

	headers := make([]string, 0, len(seen))
	for key := range seen {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	return headers
}

// endregion

// region Row rendering

// csvRowFromElement renders a single element (struct or map) as a row in headers order. Missing
// fields/keys render as "" — the empty string. Anything that is not struct or map errors loudly.
func csvRowFromElement(rv reflect.Value, headers []string) []string {

	rv = indirect(rv)
	switch rv.Kind() {

	case reflect.Struct:
		return csvRowFromStruct(rv, headers)

	case reflect.Map:
		return csvRowFromMap(rv, headers)

	default:
		// A scalar element is a single-field row.
		return []string{csvCellValue(rv)}
	}
}

// csvRowFromStruct renders a struct's exported fields in headers order. Fields not present in headers
// are skipped; headers not present on the struct render as "".
func csvRowFromStruct(rv reflect.Value, headers []string) []string {

	cells := make([]string, len(headers))
	fields := csvStructFieldByName(rv.Type())
	for i, header := range headers {
		index, ok := fields[header]
		if !ok {
			continue
		}
		cells[i] = csvCellValue(rv.FieldByIndex(index))
	}
	return cells
}

// csvStructFieldByName returns a name→fieldIndex map honoring `csv:"name"` and `csv:"-"` tags.
// Index is the [reflect.StructField.Index] slice — pass it to [reflect.Value.FieldByIndex] to handle
// promoted embedded fields. The map is rebuilt per row, which is fine for human-scale row counts; if
// profiling shows it as a hot spot, hoist via a sync.Map keyed by reflect.Type.
func csvStructFieldByName(t reflect.Type) map[string][]int {

	visible := reflect.VisibleFields(t)
	out := make(map[string][]int, len(visible))
	for _, field := range visible {
		if !field.IsExported() {
			continue
		}
		name, skip := csvFieldName(field)
		if skip {
			continue
		}
		out[name] = field.Index
	}
	return out
}

// csvRowFromMap renders a map's values in headers order. Headers not present in the map render as "".
func csvRowFromMap(rv reflect.Value, headers []string) []string {

	cells := make([]string, len(headers))
	for i, header := range headers {
		// reflect.Map lookups need the index value to match the key type. We accept any map whose key
		// type is comparable and convertible from a string for direct lookup, and fall back to a
		// linear scan for non-string-keyed maps.
		cells[i] = csvCellValue(csvMapLookup(rv, header))
	}
	return cells
}

// csvMapLookup returns the value at header in rv. For string-keyed maps this is a direct lookup; for
// other key types it does a linear scan comparing fmt.Sprint(key) to header. Returns the zero
// reflect.Value if the key is absent.
func csvMapLookup(rv reflect.Value, header string) reflect.Value {

	keyType := rv.Type().Key()
	if keyType.Kind() == reflect.String {
		key := reflect.New(keyType).Elem()
		key.SetString(header)
		return rv.MapIndex(key)
	}

	for _, key := range rv.MapKeys() {
		if fmt.Sprint(key.Interface()) == header {
			return rv.MapIndex(key)
		}
	}
	return reflect.Value{}
}

// csvCellValue renders rv as a string for a CSV cell. Invalid (zero) reflect.Values render as "".
// Pointers are dereferenced; nil pointers render as "". Everything else delegates to fmt.Sprint,
// which honors fmt.Stringer for custom types.
func csvCellValue(rv reflect.Value) string {

	if !rv.IsValid() {
		return ""
	}
	rv = indirect(rv)
	if !rv.IsValid() {
		return ""
	}
	return fmt.Sprint(rv.Interface())
}

// endregion

// region Helpers

// indirect dereferences pointers and unwraps interfaces, returning the underlying concrete value.
// Returns the zero reflect.Value for nil pointers and nil interfaces.
func indirect(rv reflect.Value) reflect.Value {

	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) {
		if rv.IsNil() {
			return reflect.Value{}
		}
		rv = rv.Elem()
	}
	return rv
}

// endregion
