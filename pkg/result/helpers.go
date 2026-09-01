// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// region Helpers

// normalize converts a Go value into its JSON shape: maps, slices, strings, bools, [json.Number], and nil.
//
// This is stage 1 of the two-stage model (10-command-line-interface.md §8). JSON is the native format, and
// every presentation is a presentation of the JSON -- not of the Go value behind it. Running it once, at the
// head of [Pipeline.Emit], is what makes that true: the filter and the formatter see the same data, and a
// field is named by its `json:` tag everywhere rather than by its Go identifier in some renderings and its
// tag in others.
//
// Numbers stay [json.Number]. Decoding to float64 would round any integer past 2^53, which is the defect
// issue #712 records; a [json.Number] renders as the literal digits it was given. [JQFilter] converts them
// for gojq's benefit, and only there.
//
// Parameters:
//   - `value`: the result to normalize.
//
// Returns:
//   - `any`: the JSON-shaped value.
//   - `error`: a marshaling failure, naming nothing the caller can act on but never silently dropping data.
func normalize(value any) (any, error) {

	if value == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()

	var out any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}

	return out, nil
}

// asRecords returns rv as a sequence of records, wrapping a lone record in a sequence of one.
//
// S3 and S4 -- a single object -- are one row, not one cell and not an error. A scalar is not a record and
// is returned unchanged, so [DelimitedFormatter] still renders it as the single cell that `--jq '.count'`
// depends on.
//
// Parameters:
//   - `rv`: the dereferenced value.
//
// Returns:
//   - `reflect.Value`: a slice when rv was a record, and rv itself otherwise.
func asRecords(rv reflect.Value) reflect.Value {

	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Map {
		return rv
	}

	records := reflect.MakeSlice(reflect.SliceOf(rv.Type()), 1, 1)
	records.Index(0).Set(rv)
	return records
}

// endregion
