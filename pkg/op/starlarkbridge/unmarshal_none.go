// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import "reflect"

// noneUnmarshaler projects starlark.None onto the zero value of the target type.
type noneUnmarshaler struct{}

func (noneUnmarshaler) Unmarshal(target reflect.Value) error {

	target.Set(reflect.Zero(target.Type()))
	return nil
}
