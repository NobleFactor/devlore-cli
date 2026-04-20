// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// functionUnmarshaler passes through a *starlark.Function to a target of that exact pointer type.
//
// The canonical Go target for a starlark function is *mem.Function, but converting to that shape requires an
// ExecutionContext and a namespace (uri), neither of which an Unmarshaler has access to. The op-side Converter system
// (introduced in step 4.b) handles the ctx-aware conversion from *starlark.Function to *mem.Function after this
// unmarshaler delivers the raw function.
type functionUnmarshaler struct{ v *starlark.Function }

var starlarkFunctionType = reflect.TypeOf((*starlark.Function)(nil))

func (u functionUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(u.v))
		return nil
	}

	if target.Type() == starlarkFunctionType {
		target.Set(reflect.ValueOf(u.v))
		return nil
	}

	return fmt.Errorf("unmarshal: cannot assign starlark.Function to %s (expected *starlark.Function; use a Converter to reach *mem.Function)", target.Type())
}
