// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package star

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// goValue converts a script's return value into the JSON-shaped Go value the output pipeline renders:
// dicts with string keys become maps, lists and tuples become slices, scalars become their Go
// counterparts, and None becomes nil. Anything else -- a function, a struct with a custom type, a
// dict keyed by something other than strings -- has no JSON shape and is refused by name, so a script
// learns at once what it may return.
//
// Parameters:
//   - `value`: the value the script returned.
//
// Returns:
//   - `any`: the JSON-shaped Go value; nil for None.
//   - `error`: non-nil when the value, or anything inside it, has no JSON shape.
func goValue(value starlark.Value) (any, error) {

	switch v := value.(type) {
	case nil, starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Int:
		if i, ok := v.Int64(); ok {
			return i, nil
		}
		return v.BigInt(), nil
	case starlark.Float:
		return float64(v), nil
	case starlark.String:
		return string(v), nil
	case *starlark.List:
		return goSlice(v)
	case starlark.Tuple:
		return goSlice(v)
	case *starlark.Dict:
		return goMap(v.Items())
	case *starlarkstruct.Struct:
		fields := starlark.StringDict{}
		v.ToStringDict(fields)
		items := make([]starlark.Tuple, 0, len(fields))
		for name, field := range fields {
			items = append(items, starlark.Tuple{starlark.String(name), field})
		}
		return goMap(items)
	default:
		return nil, fmt.Errorf("a %s cannot be a command result; return a dict, list, string, number, bool or None", value.Type())
	}
}

// goSlice converts every element of the iterable.
//
// Parameters:
//   - `iterable`: the list or tuple.
//
// Returns:
//   - `any`: the elements as a slice.
//   - `error`: the first element that has no JSON shape.
func goSlice(iterable starlark.Iterable) (any, error) {

	var out []any
	iter := iterable.Iterate()
	defer iter.Done()

	var element starlark.Value
	for iter.Next(&element) {
		converted, err := goValue(element)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}

	return out, nil
}

// goMap converts key/value pairs whose keys are strings.
//
// Parameters:
//   - `items`: the pairs, each a two-element tuple.
//
// Returns:
//   - `any`: the pairs as a map.
//   - `error`: a key that is not a string, or a value that has no JSON shape.
func goMap(items []starlark.Tuple) (any, error) {

	out := make(map[string]any, len(items))
	for _, item := range items {
		key, ok := item[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("a dict keyed by %s cannot be a command result; keys must be strings", item[0].Type())
		}
		converted, err := goValue(item[1])
		if err != nil {
			return nil, err
		}
		out[string(key)] = converted
	}

	return out, nil
}
