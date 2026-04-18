// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// floatUnmarshaler projects a starlark.Float onto a Go float target.
type floatUnmarshaler struct{ v starlark.Float }

func (u floatUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(float64(u.v)))
		return nil
	}
	if target.Kind() != reflect.Float32 && target.Kind() != reflect.Float64 {
		return fmt.Errorf("unmarshal: cannot assign starlark.Float to %s", target.Type())
	}
	target.SetFloat(float64(u.v))
	return nil
}
