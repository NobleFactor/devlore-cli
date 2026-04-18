// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"reflect"

	"go.starlark.net/starlark"
)

// tupleUnmarshaler projects a starlark.Tuple onto a Go slice target.
//
// Tuples are required to be homogeneous. A heterogeneous tuple is a
// plan-time error.
type tupleUnmarshaler struct{ v starlark.Tuple }

func (u tupleUnmarshaler) Unmarshal(target reflect.Value) error {

	indexer := func(i int) starlark.Value { return u.v[i] }
	return unmarshalSequence(len(u.v), indexer, target, true)
}
