// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

// IsTruthy reports whether `value` is truthy under Python / Starlark truth semantics.
//
// Mirrors starlark.Value.Truth() over the Go-native values the converter produces, so every truthiness test — a
// decision node's [GuardResult], a flow.wait_until poll, an [OnRetry] / [OnError] handler verdict — evaluates the
// same way whether the tested value was projected from a Starlark value or produced as a resolved Go value:
//
//   - nil — and any typed-nil pointer, function, or channel — is falsy.
//   - `bool`: false is falsy; true is truthy.
//   - numbers (every integer width, `float32`, `float64`): zero is falsy; non-zero is truthy.
//   - `string`, slices, arrays, maps: empty is falsy; non-empty is truthy.
//   - structs: the zero value is falsy; anything else is truthy.
//   - anything else ([Resource], non-nil pointers): truthy.
//
// Parameters:
//   - `value`: the value whose truthiness routes the caller — a decision node's result, a poll result, or a handler's
//     return.
//
// Returns:
//   - `bool`: true if `value` is truthy under the rules above.
func IsTruthy(value any) bool {

	if value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	}

	switch reflected := reflect.ValueOf(value); reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice:
		return reflected.Len() > 0
	case reflect.Chan, reflect.Func, reflect.Pointer, reflect.UnsafePointer:
		return !reflected.IsNil()
	case reflect.Struct:
		return !reflected.IsZero()
	default:
		return true
	}
}
