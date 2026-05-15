// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"fmt"

	"go.starlark.net/starlark"
)

// equalValues compares two values that may have different Go types but should compare equal at the user
// level (e.g., int64 vs int when both originated as an integer literal). Integer-typed values are normalized
// to int64 via [toInt64] before comparison; everything else falls back to ==.
//
// Parameters:
//   - a: the first value.
//   - b: the second value.
//
// Returns:
//   - bool: true when the values are equal under user-level semantics.
func equalValues(a, b any) bool {

	if a == nil || b == nil {
		return a == b
	}
	if a == b {
		return true
	}
	if ai, ok := toInt64(a); ok {
		if bi, ok := toInt64(b); ok {
			return ai == bi
		}
	}
	return false
}

// starlarkDictToGoMap converts a starlark dict (keys must be strings) to a map[string]any. Values are
// shallow-converted from starlark to Go via [starlarkValueToGo].
//
// Parameters:
//   - d: the starlark dict to convert.
//
// Returns:
//   - map[string]any: the converted Go map.
//   - error: non-nil if any dict key is not a starlark string.
func starlarkDictToGoMap(d *starlark.Dict) (map[string]any, error) {

	out := make(map[string]any, d.Len())
	for _, k := range d.Keys() {
		ks, ok := k.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings, got %s", k.Type())
		}
		v, _, err := d.Get(k)
		if err != nil {
			return nil, fmt.Errorf("reading key %q: %w", string(ks), err)
		}
		out[string(ks)] = starlarkValueToGo(v)
	}
	return out, nil
}

// starlarkValueToGo unwraps a starlark scalar to its Go counterpart for storage in the binding source maps.
// Lists, dicts, and other complex types pass through as the [starlark.Value]; Phase 2's real resolver work
// will refine handling for non-scalar source values.
//
// Parameters:
//   - v: the starlark value to unwrap.
//
// Returns:
//   - any: the corresponding Go value (string, bool, int64, float64, nil for None), or the original
//     starlark.Value when the type is non-scalar.
func starlarkValueToGo(v starlark.Value) any {

	switch x := v.(type) {
	case starlark.String:
		return string(x)
	case starlark.Bool:
		return bool(x)
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			return i
		}
		return v
	case starlark.Float:
		return float64(x)
	case starlark.NoneType:
		return nil
	}
	return v
}

// toInt64 normalizes integer-like values to int64 for comparison.
//
// Parameters:
//   - v: the value to normalize.
//
// Returns:
//   - int64: the normalized integer; zero when v is not an integer type.
//   - bool: true when v was an integer type and the normalization succeeded.
func toInt64(v any) (int64, bool) {

	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return int64(x), true
	}
	return 0, false
}
