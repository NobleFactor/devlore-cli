// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// Unmarshaler is a starlark value that can populate a Go reflect.Value target
// from itself. Polymorphism is on the starlark side — the value knows how to
// project itself into a Go target type.
//
// Built-in starlark types (String, Int, Bool, etc.) do not satisfy this
// interface directly; they are wrapped by starlarkbridge-side adapters at the boundary.
// Types we own (*Promise, *receiver) implement Unmarshal as first-class
// methods.
type Unmarshaler interface {
	Unmarshal(target reflect.Value) error
}

// ToUnmarshaler adapts a raw starlark.Value into an Unmarshaler. This is the
// single boundary where starlark's interface-valued world is reduced to the
// concrete wrappers that drive the rest of the unmarshaling pipeline.
//
// The type switch lives here and nowhere else.
func ToUnmarshaler(sv starlark.Value) (Unmarshaler, error) {

	if sv == nil {
		return noneUnmarshaler{}, nil
	}

	switch v := sv.(type) {

	case Unmarshaler:
		return v, nil

	case starlark.NoneType:
		return noneUnmarshaler{}, nil

	case starlark.Bool:
		return boolUnmarshaler{v}, nil

	case starlark.Int:
		return intUnmarshaler{v}, nil

	case starlark.Float:
		return floatUnmarshaler{v}, nil

	case starlark.String:
		return stringUnmarshaler{v}, nil

	case starlark.Bytes:
		return bytesUnmarshaler{v}, nil

	case *starlark.List:
		return listUnmarshaler{v}, nil

	case starlark.Tuple:
		return tupleUnmarshaler{v}, nil

	case *starlark.Dict:
		return dictUnmarshaler{v}, nil

	case *starlark.Set:
		return setUnmarshaler{v}, nil

	case *starlark.Function:
		return functionUnmarshaler{v}, nil
	}

	return nil, fmt.Errorf("unmarshal: unsupported starlark type %s", sv.Type())
}
