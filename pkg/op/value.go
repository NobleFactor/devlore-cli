// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

// Value wraps an arbitrary Go object with an execution context.
//
// Unlike [Provider] (which has side effects and lifecycle) and [Resource] (which has identity via URI), a Value is a
// plain Go object returned from a provider method. It carries the execution context for registry access and holds the
// underlying Go value as a [reflect.Value] to avoid round-trips through any.
//
// Types should satisfy this interface by embedding [ValueBase].
type Value interface {
	Provider
	Unwrap() reflect.Value
}

// ValueBase provides a standardized implementation of the [Value] interface.
//
// It embeds [ProviderBase] for execution context access and stores the wrapped Go value as a [reflect.Value].
type ValueBase struct {
	ProviderBase
	inner reflect.Value
}

// NewValueBase creates a ValueBase wrapping the given Go value.
//
// Parameters:
//   - ctx: the execution context.
//   - v: the Go value to wrap.
//
// Returns:
//   - ValueBase: the initialized base.
func NewValueBase(ctx *RuntimeEnvironment, v reflect.Value) *ValueBase {
	return &ValueBase{
		ProviderBase: NewProviderBase(ctx),
		inner:        v,
	}
}

// Unwrap returns the underlying Go value.
func (v *ValueBase) Unwrap() reflect.Value { return v.inner }
