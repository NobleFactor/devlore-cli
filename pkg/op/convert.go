// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"go.starlark.net/starlark"
)

// GoToStarlarkValue converts a native Go value to a Starlark value.
func GoToStarlarkValue(v any) (starlark.Value, error) {
	switch val := v.(type) {
	case string:
		return starlark.String(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case bool:
		return starlark.Bool(val), nil
	case float64:
		return starlark.Float(val), nil
	case []string:
		elems := make([]starlark.Value, len(val))
		for i, s := range val {
			elems[i] = starlark.String(s)
		}
		return starlark.NewList(elems), nil
	default:
		return starlark.String(fmt.Sprintf("%v", val)), nil
	}
}

// AnyToStarlarkValue converts a Go any that carries a starlark.Value to a
// starlark.Value. Returns starlark.None if nil.
func AnyToStarlarkValue(v any) starlark.Value {
	if v == nil {
		return starlark.None
	}
	return v.(starlark.Value)
}

// StringSliceToList converts a Go []string to a Starlark list of strings.
func StringSliceToList(s []string) *starlark.List {
	elems := make([]starlark.Value, len(s))
	for i, v := range s {
		elems[i] = starlark.String(v)
	}
	return starlark.NewList(elems)
}

// StarlarkValueToGo converts a Starlark value to a native Go value.
func StarlarkValueToGo(v starlark.Value) (any, error) {
	switch val := v.(type) {
	case starlark.String:
		return string(val), nil
	case starlark.Int:
		i, _ := val.Int64()
		return int(i), nil
	case starlark.Bool:
		return bool(val), nil
	case starlark.Float:
		return float64(val), nil
	case starlark.NoneType:
		return nil, nil
	case *starlark.List:
		return StarlarkListToSlice(val)
	case *starlark.Dict:
		return StarlarkDictToMap(val)
	default:
		return nil, fmt.Errorf("unsupported Starlark type %s", v.Type())
	}
}

// StarlarkListToSlice converts a Starlark list to a Go slice.
// Returns []string if all elements are strings, []any otherwise.
func StarlarkListToSlice(list *starlark.List) (any, error) {
	n := list.Len()
	if n == 0 {
		return []string{}, nil
	}

	// Try homogeneous []string first
	allStrings := true
	for i := 0; i < n; i++ {
		if _, ok := list.Index(i).(starlark.String); !ok {
			allStrings = false
			break
		}
	}

	if allStrings {
		result := make([]string, n)
		for i := 0; i < n; i++ {
			s, ok := list.Index(i).(starlark.String)
			if !ok {
				return nil, fmt.Errorf("list element %d: expected string", i)
			}
			result[i] = string(s)
		}
		return result, nil
	}

	// Mixed types: []any
	result := make([]any, n)
	for i := 0; i < n; i++ {
		val, err := StarlarkValueToGo(list.Index(i))
		if err != nil {
			return nil, fmt.Errorf("list element %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

// StarlarkDictToMap converts a Starlark dict to a Go map[string]any.
func StarlarkDictToMap(dict *starlark.Dict) (map[string]any, error) {
	result := make(map[string]any, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("dict key must be string, got %s", item[0].Type())
		}
		val, err := StarlarkValueToGo(item[1])
		if err != nil {
			return nil, fmt.Errorf("dict key %q: %w", key, err)
		}
		result[key] = val
	}
	return result, nil
}
