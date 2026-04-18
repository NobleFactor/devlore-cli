// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// setUnmarshaler projects a *starlark.Set onto a Go map[T]struct{} target.
//
// Homogeneity is required: all set members must share one starlark type,
// and the Go target's key type must match. A heterogeneous set is a
// plan-time error (Go map key semantics diverge from starlark set
// equality for mixed types).
type setUnmarshaler struct{ v *starlark.Set }

func (u setUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() != reflect.Map {
		return fmt.Errorf("unmarshal: cannot assign starlark.Set to %s (expected map[T]struct{})", target.Type())
	}
	if target.Type().Elem().Kind() != reflect.Struct || target.Type().Elem().NumField() != 0 {
		return fmt.Errorf("unmarshal: starlark.Set target must be map[T]struct{}, got %s", target.Type())
	}

	n := u.v.Len()
	if n == 0 {
		target.Set(reflect.MakeMapWithSize(target.Type(), 0))
		return nil
	}

	// Homogeneity check: all elements must be the same starlark type.
	iter := u.v.Iterate()
	defer iter.Done()

	var firstType reflect.Type
	var elem starlark.Value
	keyType := target.Type().Key()
	result := reflect.MakeMapWithSize(target.Type(), n)

	for iter.Next(&elem) {
		elemType := starlarkElemGoType(elem)
		if elemType == nil {
			return fmt.Errorf("unmarshal: starlark.Set element of type %s is not representable as a Go map key", elem.Type())
		}
		if firstType == nil {
			firstType = elemType
			if firstType != keyType {
				return fmt.Errorf("unmarshal: starlark.Set of %s cannot target %s", elem.Type(), target.Type())
			}
		} else if elemType != firstType {
			return fmt.Errorf("unmarshal: heterogeneous starlark.Set cannot be represented in Go; all elements must share one type")
		}

		keyV := reflect.New(keyType).Elem()
		eu, err := ToUnmarshaler(elem)
		if err != nil {
			return fmt.Errorf("set element: %w", err)
		}
		if err := eu.Unmarshal(keyV); err != nil {
			return fmt.Errorf("set element: %w", err)
		}
		result.SetMapIndex(keyV, reflect.Zero(target.Type().Elem()))
	}

	target.Set(result)
	return nil
}
