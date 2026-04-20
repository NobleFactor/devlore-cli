// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// listUnmarshaler projects a *starlark.List onto a Go slice target.
//
// Heterogeneity rule: if the list's elements are all the same starlark type,
// it marshals to []T where T matches that type. If elements differ, it
// marshals to []any.
type listUnmarshaler struct{ v *starlark.List }

func (u listUnmarshaler) Unmarshal(target reflect.Value) error {

	return unmarshalSequence(u.v.Len(), u.v.Index, target, false)
}

// unmarshalSequence is the shared implementation for list and tuple. If
// strictHomogeneous is true (tuple), a heterogeneous sequence is an error.
// For lists, a heterogeneous sequence only errors if the target is a typed
// []T; an any-target accepts heterogeneous via []any.
func unmarshalSequence(n int, indexer func(int) starlark.Value, target reflect.Value, strictHomogeneous bool) error {

	if target.Kind() == reflect.Interface {
		// Target is `any` — produce []T if homogeneous, []any otherwise.
		homogeneous, commonType := detectHomogeneity(n, indexer)
		if homogeneous && commonType != nil {
			slice := reflect.MakeSlice(reflect.SliceOf(commonType), n, n)
			for i := 0; i < n; i++ {
				u, err := ToUnmarshaler(indexer(i))
				if err != nil {
					return fmt.Errorf("index %d: %w", i, err)
				}
				if err := u.Unmarshal(slice.Index(i)); err != nil {
					return fmt.Errorf("index %d: %w", i, err)
				}
			}
			target.Set(slice)
			return nil
		}
		if strictHomogeneous {
			return fmt.Errorf("unmarshal: heterogeneous tuple cannot be represented; all elements must share one type")
		}
		anySlice := make([]any, n)
		for i := 0; i < n; i++ {
			u, err := ToUnmarshaler(indexer(i))
			if err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
			elem := reflect.ValueOf(&anySlice[i]).Elem()
			if err := u.Unmarshal(elem); err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
		}
		target.Set(reflect.ValueOf(anySlice))
		return nil
	}

	if target.Kind() != reflect.Slice {
		return fmt.Errorf("unmarshal: cannot assign sequence to %s", target.Type())
	}

	elemType := target.Type().Elem()
	slice := reflect.MakeSlice(target.Type(), n, n)
	for i := 0; i < n; i++ {
		u, err := ToUnmarshaler(indexer(i))
		if err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
		elem := reflect.New(elemType).Elem()
		if err := u.Unmarshal(elem); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
		slice.Index(i).Set(elem)
	}
	target.Set(slice)
	return nil
}

// detectHomogeneity reports whether all sequence elements share the same
// concrete starlark type. Returns the matching Go type for list-of-T
// construction (int → int, string → string, etc.) or nil if mixed.
func detectHomogeneity(n int, indexer func(int) starlark.Value) (bool, reflect.Type) {

	if n == 0 {
		return true, reflect.TypeOf((*any)(nil)).Elem()
	}
	firstType := starlarkElemGoType(indexer(0))
	if firstType == nil {
		return false, nil
	}
	for i := 1; i < n; i++ {
		if starlarkElemGoType(indexer(i)) != firstType {
			return false, nil
		}
	}
	return true, firstType
}

// starlarkElemGoType reports the natural Go type for a starlark value when
// used as a sequence element in a homogeneous context. Returns nil for
// values without a natural Go scalar target (e.g., other sequences, dicts).
func starlarkElemGoType(sv starlark.Value) reflect.Type {

	switch sv.(type) {
	case starlark.String:
		return reflect.TypeOf("")
	case starlark.Int:
		return reflect.TypeOf(int(0))
	case starlark.Bool:
		return reflect.TypeOf(false)
	case starlark.Float:
		return reflect.TypeOf(float64(0))
	case starlark.Bytes:
		return reflect.TypeOf([]byte(nil))
	}
	return nil
}
