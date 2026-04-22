// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

// isTruthy reports whether value satisfies the choose dispatch's truthiness rule (phase-8 step 13).
//
// Mirrors starlark.Value.Truth() semantics for native Go types so [Provider.Choose]'s sequential walk produces
// the same outcome whether the case's When was supplied as a starlark literal that projected through the
// unmarshal pipeline or as a resolved Go value:
//
//   - bool: true is truthy; false is falsy.
//   - integer (int, int64, uint, uint64, etc.): zero is falsy; non-zero is truthy.
//   - string: empty is falsy; non-empty is truthy.
//   - nil: falsy.
//   - anything else (op.Resource, non-nil pointer, struct, slice, map): truthy.
//
// When a case's When is an *starlarkbridge.Invocation reference, the executor (step 16) dispatches the When
// invocation, resolves its value, and applies isTruthy to the resolved value — so the same rule governs both
// literal and computed conditions.
//
// Parameters:
//   - value: the When value from a [Case].
//
// Returns:
//   - bool: true if value is truthy under the choose dispatch's rule.
func isTruthy(value any) bool {

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
	default:
		return true
	}
}
