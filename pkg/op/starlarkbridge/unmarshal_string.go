// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// stringUnmarshaler projects a starlark.String onto a string target.
type stringUnmarshaler struct{ v starlark.String }

func (u stringUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(string(u.v)))
		return nil
	}
	if target.Kind() != reflect.String {
		return fmt.Errorf("unmarshal: cannot assign starlark.String to %s", target.Type())
	}
	target.SetString(string(u.v))
	return nil
}
