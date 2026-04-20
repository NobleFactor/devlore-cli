// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// boolUnmarshaler projects a starlark.Bool onto a bool target.
type boolUnmarshaler struct{ v starlark.Bool }

func (u boolUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(bool(u.v)))
		return nil
	}
	if target.Kind() != reflect.Bool {
		return fmt.Errorf("unmarshal: cannot assign starlark.Bool to %s", target.Type())
	}
	target.SetBool(bool(u.v))
	return nil
}
