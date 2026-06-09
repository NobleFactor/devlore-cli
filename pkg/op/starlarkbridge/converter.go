// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// region EXPORTED FUNCTIONS

// region Conversion

// StarlarkToGoTyped converts a [starlark.Value] into a value of the declared Go target type.
//
// The cascade: starlark None short-circuits to nil; otherwise [converter.toGo] produces an `any` value in its natural
// Go shape, then [op.Convert] routes through registered converters and Resource constructors to land on `target`. The
// environment's [op.ReceiverRegistry] is consulted for Resource construction.
//
// This is the public façade over [converter]; external callers convert through it rather than naming the type.
//
// Parameters:
//   - `env`: the runtime environment the [converter] is built from; its registry is consulted by [op.Convert].
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

	c := converter{environment: env}

	intermediate, err := c.toGo(sv, reflect.TypeFor[any]())
	if err != nil {
		return nil, err
	}

	return op.Convert(c.environment, intermediate, target)
}

// endregion

// endregion

// region SUPPORTING TYPES

// converter performs starlark → Go value conversion.
//
// It carries the [op.RuntimeEnvironment] the conversion needs — the environment's [op.ReceiverRegistry] drives
// [op.Convert] and resource construction. A [Runtime] owns one converter and hands it to every [goReceiver] it builds;
// ad-hoc wraps that convert nothing hold a zero converter.
type converter struct {
	environment *op.RuntimeEnvironment
}

// region UNEXPORTED METHODS

// region Conversion

// toGoInto recursively converts a Starlark receiver into a [reflect.Value] target.
func (c converter) toGoInto(sv starlark.Value, rv reflect.Value) error {

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
		val, err := c.toNaturalGo(sv)
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
			return c.toGoSlice(iter, rv)
		}
		return fmt.Errorf("expected list, got %s", sv.Type())

	case reflect.Map:
		if dict, ok := sv.(*starlark.Dict); ok {
			return c.toGoMap(dict, rv)
		}
		return fmt.Errorf("expected dict, got %s", sv.Type())

	case reflect.Struct:
		return c.toGoStruct(sv, rv)

	default:
		return fmt.Errorf("unsupported conversion: %s to %s", sv.Type(), rv.Type())
	}
	return nil
}

// toGoMap converts a [starlark.Dict] into a typed Go map via reflection.
func (c converter) toGoMap(dict *starlark.Dict, rv reflect.Value) error {

	m := reflect.MakeMapWithSize(rv.Type(), dict.Len())
	keyType := rv.Type().Key()
	valType := rv.Type().Elem()

	for _, item := range dict.Items() {

		key := reflect.New(keyType).Elem()

		if err := c.toGoInto(item[0], key); err != nil {
			return fmt.Errorf("dict key: %w", err)
		}

		val := reflect.New(valType).Elem()

		if err := c.toGoInto(item[1], val); err != nil {
			return fmt.Errorf("dict value for key %v: %w", key.Interface(), err)
		}

		m.SetMapIndex(key, val)
	}

	rv.Set(m)
	return nil
}

// toGoSlice converts a [starlark.Iterable] into a typed Go slice via reflection.
func (c converter) toGoSlice(sv starlark.Iterable, rv reflect.Value) error {

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

		if err := c.toGoInto(x, target); err != nil {
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
func (c converter) toGoStruct(sv starlark.Value, rv reflect.Value) error {

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

		if err := c.toGoInto(val, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}

	return nil
}

// toGo converts a [starlark.Value] into a fresh Go value of the target type.
func (c converter) toGo(sv starlark.Value, target reflect.Type) (any, error) {

	rv := reflect.New(target).Elem()

	if err := c.toGoInto(sv, rv); err != nil {
		return nil, err
	}

	return rv.Interface(), nil
}

// toNaturalGo is the bridge's central starlark → Go translation: hand it any [starlark.Value], get back the natural Go
// value or an error.
//
// Primitives (None, String, Int, Bool, Float, Bytes) map to their Go equivalents. Containers recurse through
// [converter.toNaturalGo] per element: List, Tuple, and Set yield a []any; Dict yields a map[string]any and so requires
// string keys — a non-string key is an error here, not a silent stringify, matching JSON's string-only object-key
// model. Wrapped Go values — anything implementing the bridge's [Projector] interface (notably [*goReceiver] over a
// registered Go instance, plus [*op.Promise]) — are asked to project to `any`.
//
// The fall-through returns any remaining starlark type as-is — notably a *starlark.Function, which the planner resolves
// to its content resource via the registry's source key ([op.ReceiverRegistry.ConstructorForSource]), keeping this
// bridge free of any provider. Downstream [op.Convert] with a typed target handles target-aware projection.
func (c converter) toNaturalGo(sv starlark.Value) (any, error) {

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
			nat, err := c.toNaturalGo(x)
			if err != nil {
				return nil, err
			}
			res = append(res, nat)
		}

		return res, nil

	case *starlark.Dict:

		res := make(map[string]any, v.Len()) // Optimized: Pre-allocates map buckets.

		for _, item := range v.Items() {

			// JSON objects are string-keyed and Go's encoding/json rejects any map whose key type is not
			// string/integer/TextMarshaler — so a starlark dict projects to map[string]any, requiring string keys.
			// AsString accepts only starlark.String; any other key type is a hard error here rather than a cryptic
			// encode failure downstream.

			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("dict key: expected string, got %s", item[0].Type())
			}

			val, err := c.toNaturalGo(item[1])
			if err != nil {
				return nil, err
			}

			res[key] = val
		}

		return res, nil

	case Projector:
		return v.Project(reflect.TypeFor[any]())
	}

	// Fall-through: a starlark type with no natural Go form here. Returned as-is; downstream op.Convert with a typed
	// target handles target-aware projection.
	return sv, nil
}

// endregion

// endregion

// endregion
