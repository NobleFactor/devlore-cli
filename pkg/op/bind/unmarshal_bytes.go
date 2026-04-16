// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

var byteSliceType = reflect.TypeOf([]byte(nil))

// bytesUnmarshaler projects a starlark.Bytes onto a []byte target.
type bytesUnmarshaler struct{ v starlark.Bytes }

func (u bytesUnmarshaler) Unmarshal(target reflect.Value) error {

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf([]byte(u.v)))
		return nil
	}
	if target.Type() != byteSliceType {
		return fmt.Errorf("unmarshal: cannot assign starlark.Bytes to %s", target.Type())
	}
	target.SetBytes([]byte(u.v))
	return nil
}
