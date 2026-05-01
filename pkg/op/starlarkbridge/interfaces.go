// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"
)

// Projector is implemented by starlark.Values that know how to project themselves into a Go target type.
//
// Projector is the bridge's contract for "given a target type, give me a Go value of that type." The wrapper
// implements it by running [op.Convert]'s cascade on its wrapped Go instance. Plan-time references (Promise,
// Invocation) implement it by selecting one of their legal handles based on the target type — for example,
// Promise projects to *Promise, op.PromiseValue, or itself-as-interface depending on what the caller wants.
//
// NodeBuilder.fillSlot type-asserts against this interface to detect any starlark.Value with a Go-side projection
// path, regardless of mechanism. The interface is the contract; concrete types choose how to satisfy it.
type Projector interface {
	Project(target reflect.Type) (any, error)
}
