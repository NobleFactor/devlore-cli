// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

// region Types

// ListFormatter renders each record as one field per line, keys aligned within the record.
//
// This is the rendering for a result that is wide, heterogeneous, or both -- where `table` is unreadable
// and `json` is punctuation a reader has to see past:
//
//	name  : x
//	state : active
//
//	name     : y
//	runs     : 3
//	findings : ["a","b"]
//
// It shares key derivation with [DelimitedFormatter] and [TableFormatter] -- `csv:"name"` tags, struct
// declaration order, sorted map keys -- and diverges on one point: the delimited formats derive ONE column
// set as the union across every record, where this gives each record its own keys. That is what makes it
// right for a heterogeneous stream, where a union renders mostly holes.
//
// Keys are padded within a record rather than across the stream, so a heterogeneous stream does not pay for
// its widest key everywhere. The separator is " : " with the colon aligned, deliberately not "key: value",
// which reads as YAML when `-o yaml` is one flag away and means something else.
type ListFormatter struct{}

// endregion

// region Constructors

// NewListFormatter returns the one-field-per-line formatter.
//
// Returns:
//   - `ListFormatter`: the formatter.
func NewListFormatter() ListFormatter { return ListFormatter{} }

// endregion

// region Methods

// Format writes value as records of aligned `key : value` lines separated by blank lines.
//
// Parameters:
//   - `value`: the result to render.
//   - `w`: the destination.
//
// Returns:
//   - `error`: a write failure, or nil.
func (f ListFormatter) Format(value any, w io.Writer) error {

	if value == nil {
		return nil
	}

	rv := indirect(reflect.ValueOf(value))
	if !rv.IsValid() {
		return nil
	}

	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return f.writeRecord(w, rv, false)
	}

	// A byte slice is one value, not a sequence of numbers -- the same carve-out the delimited formats make.
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		_, err := fmt.Fprintln(w, string(rv.Bytes()))
		return err
	}

	for i := 0; i < rv.Len(); i++ {
		if err := f.writeRecord(w, indirect(rv.Index(i)), i > 0); err != nil {
			return err
		}
	}

	return nil
}

// endregion

// region Helpers

// writeRecord writes one record, preceded by a blank line when it is not the first.
//
// Parameters:
//   - `w`: the destination.
//   - `rv`: the dereferenced record.
//   - `separate`: whether to emit the blank line that divides this record from the previous one.
//
// Returns:
//   - `error`: a write failure, or nil.
func (f ListFormatter) writeRecord(w io.Writer, rv reflect.Value, separate bool) error {

	if separate {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	if !rv.IsValid() {
		_, err := fmt.Fprintln(w)
		return err
	}

	keys, values := listFields(rv)

	// A scalar or a sequence carries no keys of its own: it is the value alone, which is what makes
	// `--jq '.name' -o list` print a name rather than a line of punctuation.
	if keys == nil {
		for _, value := range values {
			if _, err := fmt.Fprintln(w, value); err != nil {
				return err
			}
		}
		return nil
	}

	width := 0
	for _, key := range keys {
		if n := len([]rune(key)); n > width {
			width = n
		}
	}

	for i, key := range keys {
		padding := strings.Repeat(" ", width-len([]rune(key)))
		if _, err := fmt.Fprintf(w, "%s%s : %s\n", key, padding, values[i]); err != nil {
			return err
		}
	}

	return nil
}

// listFields returns a record's keys and rendered values, or nil keys when the value carries none.
//
// Key derivation is the delimited formats': [csvHeadersFromStruct] for a struct, honoring `csv:` tags and
// declaration order, and sorted keys for a map. What differs is scope -- these are THIS record's keys, not
// the union across the stream, which is what makes a heterogeneous stream render densely.
//
// A scalar, or a sequence, has no keys and returns its rendered values alone.
//
// Parameters:
//   - `rv`: the dereferenced record.
//
// Returns:
//   - `[]string`: the keys, or nil when the record has none.
//   - `[]string`: the rendered values, aligned with the keys when those are present.
func listFields(rv reflect.Value) (keys, values []string) {

	switch rv.Kind() {

	case reflect.Struct:
		keys = csvHeadersFromStruct(rv.Type())
		return keys, csvRowFromStruct(rv, keys)

	case reflect.Map:
		keys = listMapKeys(rv)
		values = make([]string, len(keys))
		for i, key := range keys {
			values[i] = csvCellValue(csvMapLookup(rv, key))
		}
		return keys, values

	case reflect.Slice, reflect.Array:
		for i := range rv.Len() {
			values = append(values, csvCellValue(rv.Index(i)))
		}
		return nil, values

	default:
		return nil, []string{csvCellValue(rv)}
	}
}

// listMapKeys returns one map's keys, sorted.
//
// The delimited formats' [csvHeadersFromMaps] unions keys across a whole slice; this is the single-record
// counterpart, and sorts by the same rule so a map renders its fields in the same order under every
// presentation.
//
// Parameters:
//   - `rv`: the map.
//
// Returns:
//   - `[]string`: the keys, sorted.
func listMapKeys(rv reflect.Value) []string {

	keys := make([]string, 0, rv.Len())
	for _, key := range rv.MapKeys() {
		keys = append(keys, fmt.Sprint(key.Interface()))
	}
	sort.Strings(keys)
	return keys
}

// endregion
