// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"reflect"
)

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

	if truthy, ok := scalarTruthy(value); ok {
		return truthy
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

// scalarTruthy reports a built-in scalar's truthiness: false for zero numbers, empty strings, and
// false itself.
//
// Parameters:
//   - `value`: the value under test.
//
// Returns:
//   - `bool`: the truthiness, when `value` is a built-in scalar.
//   - `bool`: true when `value` was a built-in scalar.
func scalarTruthy(value any) (truthy, isScalar bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		return v != "", true
	case json.Number:
		return numberTruthy(v)
	}
	return numericTruthy(value)
}

// numericTruthy is [scalarTruthy]'s case for the Go numeric types, exactly typed: a named type with a numeric
// underlying kind is not a scalar here, as it never was.
//
// Parameters:
//   - `value`: the value under test.
//
// Returns:
//   - `truthy`: whether the number is non-zero.
//   - `isScalar`: false when `value` is not one of the built-in numeric types.
func numericTruthy(value any) (truthy, isScalar bool) {
	switch v := value.(type) {
	case int:
		return v != 0, true
	case int8:
		return v != 0, true
	case int16:
		return v != 0, true
	case int32:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	case uint8:
		return v != 0, true
	case uint16:
		return v != 0, true
	case uint32:
		return v != 0, true
	case uint64:
		return v != 0, true
	case float32:
		return v != 0, true
	case float64:
		return v != 0, true
	}
	return false, false
}

// numberTruthy is [scalarTruthy]'s case for a decoder artifact.
//
// Defense in depth (#712 phase 2): the decoder should never let a json.Number reach a truthiness check, but if a
// new path does, a zero is falsy however it was decoded.
//
// Parameters:
//   - `number`: the undecoded number.
//
// Returns:
//   - `truthy`: whether the number is non-zero.
//   - `isScalar`: false when the number parses as neither an integer nor a float.
func numberTruthy(number json.Number) (truthy, isScalar bool) {

	if integer, err := number.Int64(); err == nil {
		return integer != 0, true
	}
	if float, err := number.Float64(); err == nil {
		return float != 0, true
	}

	return false, false
}
