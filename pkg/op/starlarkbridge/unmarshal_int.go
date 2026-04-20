// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// intUnmarshaler projects a starlark.Int onto a signed/unsigned Go integer or float target.
type intUnmarshaler struct{ v starlark.Int }

func (u intUnmarshaler) Unmarshal(target reflect.Value) error {

	kind := target.Kind()

	switch kind {

	case reflect.Interface:
		i, ok := u.v.Int64()
		if !ok {
			return fmt.Errorf("unmarshal: int value out of range")
		}
		target.Set(reflect.ValueOf(int(i)))
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := u.v.Int64()
		if !ok {
			return fmt.Errorf("unmarshal: int value out of range")
		}
		if target.OverflowInt(i) {
			return fmt.Errorf("unmarshal: int value %d overflows %s", i, target.Type())
		}
		target.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, ok := u.v.Uint64()
		if !ok {
			return fmt.Errorf("unmarshal: uint value out of range")
		}
		if target.OverflowUint(i) {
			return fmt.Errorf("unmarshal: uint value %d overflows %s", i, target.Type())
		}
		target.SetUint(i)
		return nil

	case reflect.Float32, reflect.Float64:
		target.SetFloat(float64(u.v.Float()))
		return nil
	}

	return fmt.Errorf("unmarshal: cannot assign starlark.Int to %s", target.Type())
}
