// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// dictUnmarshaler projects a *starlark.Dict onto a Go map target.
//
// Keys are enforced to be uniformly K; values uniformly V. A mixed-key or
// mixed-value dict targeting a typed map[K]V fails at the first mismatch.
// Targeting an interface (any) produces map[any]any with natural Go types.
type dictUnmarshaler struct{ v *starlark.Dict }

func (u dictUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		result := make(map[any]any, u.v.Len())
		for _, item := range u.v.Items() {
			var key, value any
			ku, err := ToUnmarshaler(item[0])
			if err != nil {
				return fmt.Errorf("dict key: %w", err)
			}
			if err := ku.Unmarshal(reflect.ValueOf(&key).Elem()); err != nil {
				return fmt.Errorf("dict key: %w", err)
			}
			vu, err := ToUnmarshaler(item[1])
			if err != nil {
				return fmt.Errorf("dict value: %w", err)
			}
			if err := vu.Unmarshal(reflect.ValueOf(&value).Elem()); err != nil {
				return fmt.Errorf("dict value for key %v: %w", key, err)
			}
			result[key] = value
		}
		target.Set(reflect.ValueOf(result))
		return nil
	}

	if target.Kind() != reflect.Map {
		return fmt.Errorf("unmarshal: cannot assign starlark.Dict to %s", target.Type())
	}

	keyType := target.Type().Key()
	valType := target.Type().Elem()
	result := reflect.MakeMapWithSize(target.Type(), u.v.Len())

	for _, item := range u.v.Items() {
		keyV := reflect.New(keyType).Elem()
		ku, err := ToUnmarshaler(item[0])
		if err != nil {
			return fmt.Errorf("dict key: %w", err)
		}
		if err := ku.Unmarshal(keyV); err != nil {
			return fmt.Errorf("dict key: %w", err)
		}
		valV := reflect.New(valType).Elem()
		vu, err := ToUnmarshaler(item[1])
		if err != nil {
			return fmt.Errorf("dict value: %w", err)
		}
		if err := vu.Unmarshal(valV); err != nil {
			return fmt.Errorf("dict value for key %v: %w", keyV.Interface(), err)
		}
		result.SetMapIndex(keyV, valV)
	}

	target.Set(result)
	return nil
}
