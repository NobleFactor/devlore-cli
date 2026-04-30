// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"

	"go.starlark.net/starlark"
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

// Marshaler is implemented by Go types that take ownership of their own Go→starlark marshaling.
//
// The bridge's default Go→starlark path runs through [NewWrapper], which uses reflection over the wrapped Go
// instance's exported fields and registered methods. A Go type that needs richer or cheaper marshaling —
// collapsing internal state into a single starlark.Value, surfacing a derived representation, avoiding reflection
// on a hot path — implements [Marshaler] and the wrapper defers to it instead of running the default path.
//
// Implementing [Marshaler] is a contract: the returned [starlark.Value] is what the bridge exposes for this type,
// and no fallback to the wrapper's default behavior occurs.
type Marshaler interface {
	MarshalStarlark() (starlark.Value, error)
}

// Unmarshaler is implemented by Go types that take ownership of their own starlark→Go unmarshaling.
//
// The bridge's default starlark→Go path runs through [Unwrapper.Unwrap], which projects the wrapped Go value
// into a target type via [op.Convert]'s [op.SourceConverter] / [op.TargetConverter] cascade. A Go type that
// needs custom shape recognition — accepting multiple starlark shapes, validating invariants at the bridge
// boundary, or absorbing values that don't fit the conversion cascade — implements [Unmarshaler] and the
// wrapper defers to it.
//
// Implementing [Unmarshaler] is a contract: any error returned propagates as the bridge's unmarshaling error; no
// fallback to the wrapper's default behavior occurs.
type Unmarshaler interface {
	UnmarshalStarlark(starlark.Value) error
}