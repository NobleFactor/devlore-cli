// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region EXPORTED FUNCTIONS

// region Conversion

// StarlarkToGoTyped converts a [starlark.Value] into a value of the declared Go target type.
//
// The cascade: starlark None short-circuits to nil; otherwise [toGo] produces an `any` value in its
// natural Go shape, then [op.Convert] routes through registered converters and Resource constructors to
// land on `target`. The env's [op.RuntimeEnvironment.Registry] is consulted for Resource construction.
//
// Parameters:
//   - `env`: the runtime environment whose registry is consulted by [op.Convert].
//   - `sv`: the starlark value to convert.
//   - `target`: the declared Go target type.
//
// Returns:
//   - `any`: the converted Go value (nil for starlark None).
//   - `error`: non-nil if conversion fails.
func StarlarkToGoTyped(env *op.RuntimeEnvironment, sv starlark.Value, target reflect.Type) (any, error) {

	if _, ok := sv.(starlark.NoneType); ok {
		return nil, nil
	}

	intermediate, err := toGo(sv, reflect.TypeFor[any]())
	if err != nil {
		return nil, err
	}

	return op.Convert(env, intermediate, target)
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// region Conversion

// toGoInto recursively converts a Starlark receiver into a [reflect.Value] target.
func toGoInto(sv starlark.Value, rv reflect.Value) error {

	if sv == nil || sv == starlark.None {
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

	// 1. Dereference and Allocate Pointers.

	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}

	// 2. Handle Interface Targets via natural projection.

	if rv.Kind() == reflect.Interface {
		val, err := toNaturalGo(sv)
		if err != nil {
			return err
		}
		rv.Set(reflect.ValueOf(val))
		return nil
	}

	// 3. Direct Assignment (for custom Go receivers).

	if reflect.TypeOf(sv).AssignableTo(rv.Type()) {
		rv.Set(reflect.ValueOf(sv))
		return nil
	}

	// 4. Concrete Type Logic.

	switch rv.Kind() {
	case reflect.String:
		s, ok := starlark.AsString(sv)
		if !ok {
			return fmt.Errorf("expected string, got %s", sv.Type())
		}
		rv.SetString(s)

	case reflect.Bool:
		if b, ok := sv.(starlark.Bool); ok {
			rv.SetBool(bool(b))
		} else {
			return fmt.Errorf("expected bool, got %s", sv.Type())
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if si, ok := sv.(starlark.Int); ok {
			i, ok := si.Int64()
			if !ok {
				return fmt.Errorf("int out of range")
			}
			rv.SetInt(i)
		} else {
			return fmt.Errorf("expected int, got %s", sv.Type())
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if si, ok := sv.(starlark.Int); ok {
			u, ok := si.Uint64()
			if !ok {
				return fmt.Errorf("uint out of range")
			}
			rv.SetUint(u)
		} else {
			return fmt.Errorf("expected int, got %s", sv.Type())
		}

	case reflect.Float32, reflect.Float64:
		f, ok := starlark.AsFloat(sv)
		if !ok {
			return fmt.Errorf("expected float or int, got %s", sv.Type())
		}
		rv.SetFloat(f)

	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			if b, ok := sv.(starlark.Bytes); ok {
				rv.SetBytes([]byte(b))
				return nil
			}
			return fmt.Errorf("expected bytes, got %s", sv.Type())
		}
		if iter, ok := sv.(starlark.Iterable); ok {
			return toGoSlice(iter, rv)
		}
		return fmt.Errorf("expected list, got %s", sv.Type())

	case reflect.Map:
		if dict, ok := sv.(*starlark.Dict); ok {
			return toGoMap(dict, rv)
		}
		return fmt.Errorf("expected dict, got %s", sv.Type())

	case reflect.Struct:
		return toGoStruct(sv, rv)

	default:
		return fmt.Errorf("unsupported conversion: %s to %s", sv.Type(), rv.Type())
	}
	return nil
}

// toGoMap converts a [starlark.Dict] into a typed Go map via reflection.
func toGoMap(dict *starlark.Dict, rv reflect.Value) error {

	m := reflect.MakeMapWithSize(rv.Type(), dict.Len())
	keyType := rv.Type().Key()
	valType := rv.Type().Elem()

	for _, item := range dict.Items() {

		key := reflect.New(keyType).Elem()

		if err := toGoInto(item[0], key); err != nil {
			return fmt.Errorf("dict key: %w", err)
		}

		val := reflect.New(valType).Elem()

		if err := toGoInto(item[1], val); err != nil {
			return fmt.Errorf("dict value for key %v: %w", key.Interface(), err)
		}

		m.SetMapIndex(key, val)
	}

	rv.Set(m)
	return nil
}

// toGoSlice converts a [starlark.Iterable] into a typed Go slice via reflection.
func toGoSlice(sv starlark.Iterable, rv reflect.Value) error {

	n := max(starlark.Len(sv), 0)

	sliceType := rv.Type()
	elemType := sliceType.Elem()
	newSlice := reflect.MakeSlice(sliceType, n, n)

	iter := sv.Iterate()
	defer iter.Done()

	var x starlark.Value
	i := 0

	for iter.Next(&x) {

		if i >= newSlice.Len() {
			newSlice = reflect.Append(newSlice, reflect.Zero(elemType))
		}

		target := newSlice.Index(i)

		if err := toGoInto(x, target); err != nil {
			return fmt.Errorf("list index %d: %w", i, err)
		}

		i++
	}

	if i < newSlice.Len() {
		newSlice = newSlice.Slice(0, i)
	}

	rv.Set(newSlice)
	return nil
}

// toGoStruct converts a [starlark.Dict] or [starlark.HasAttrs] into a typed Go struct via reflection.
func toGoStruct(sv starlark.Value, rv reflect.Value) error {

	info := getTypeInfo(rv.Type())
	if info == nil {
		return fmt.Errorf("cannot convert %s to non-struct %s", sv.Type(), rv.Type())
	}

	var lookup func(string) (starlark.Value, error)

	switch v := sv.(type) {
	case *starlark.Dict:
		lookup = func(name string) (starlark.Value, error) {
			val, found, err := v.Get(starlark.String(name))
			if !found {
				return nil, nil
			}
			return val, err
		}
	case starlark.HasAttrs:
		lookup = v.Attr
	default:
		return fmt.Errorf("expected struct or dict, got %s", sv.Type())
	}

	for _, fi := range info.fields {

		val, err := lookup(fi.starName)
		if err != nil || val == nil {
			continue
		}

		if err := toGoInto(val, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}

	return nil
}

// toGo converts a [starlark.Value] into a fresh Go value of the target type.
func toGo(sv starlark.Value, target reflect.Type) (any, error) {

	rv := reflect.New(target).Elem()

	if err := toGoInto(sv, rv); err != nil {
		return nil, err
	}

	return rv.Interface(), nil
}

// toNaturalGo maps a Starlark value to its natural Go representation.
func toNaturalGo(sv starlark.Value) (any, error) {

	switch v := sv.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.String:
		return string(v), nil
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("int out of range")
		}
		return i, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Float:
		return float64(v), nil
	case starlark.Bytes:
		return []byte(v), nil

	case *starlark.List, starlark.Tuple, *starlark.Set:

		n := max(starlark.Len(v), 0)

		res := make([]any, 0, n) // Optimized: Allocates capacity but stays empty for append.
		iter := v.(starlark.Iterable).Iterate()
		defer iter.Done()

		var x starlark.Value
		for iter.Next(&x) {
			nat, err := toNaturalGo(x)
			if err != nil {
				return nil, err
			}
			res = append(res, nat)
		}

		return res, nil

	case *starlark.Dict:

		res := make(map[any]any, v.Len()) // Optimized: Pre-allocates map buckets.

		for _, item := range v.Items() {

			k, err := toNaturalGo(item[0])
			if err != nil {
				return nil, err
			}

			val, err := toNaturalGo(item[1])
			if err != nil {
				return nil, err
			}

			res[k] = val
		}

		return res, nil

	default:
		return sv, nil
	}
}

// endregion

// endregion
