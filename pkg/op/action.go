// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"reflect"
)

// ErrNotCompensable signals that "Do" acknowledges rollback but cannot undo its effect.
//
// The executor logs a warning and continues unwinding.
var ErrNotCompensable = errors.New("action is not compensable")

// Result is data that flows to downstream nodes via edges (e.g., file content, a rendered template, a query result).
//
// The executor stores this keyed by node ID and resolves promise slots from stored Results before calling downstream
// Do.
type Result = any

// Complement is the state captured by Do and passed to Undo during saga rollback.
//
// Each "Do" defines its own state shape. Actions with no rollback return nil from Do; their Undo ignores the state
// parameter.
type Complement = any

// Parameter describes a single parameter accepted by an Action's Do method.
//
// Parameter is the runtime-typed form of a wire parameter token produced by codegen. The wire token (e.g.,
// "destination_path", "mode?", "mode?=0o666", "*parts", "**kwargs") is cracked at the announce boundary by
// parseParameterToken, which populates every field below. Downstream consumers — Method.Invoke, slot-fill in the
// starlark bridge, error reporting — read these fields directly and never re-parse the token.
//
// Field invariants:
//   - Name is the bare parameter name with no decoration (no leading "*"/"**", no trailing "?", no "=value"
//     suffix). It is the canonical key for slots[Name] lookups and for kwarg matching.
//   - Type is the Go reflect.Type the dispatch site projects values into via op.Convert.
//   - Optional is true for tokens carrying the "?" marker. The slot may be left empty by the caller; if Default
//     is non-nil, slot-fill substitutes it.
//   - Variadic is true for tokens with a leading single "*". The Go method declares the parameter as a slice; the
//     dispatch site collects positional overflow into it.
//   - Kwargs is true for tokens with a leading "**". The Go method declares the parameter as map[string]any; the
//     dispatch site collects unknown keyword arguments into it.
//   - Default holds a Go-native value assignable to Type (or nil iff the parameter has no default). The dynamic
//     type inside the any box always matches Type exactly — parseDefaultExpression widens the parsed primitive
//     to Type's named form (e.g., os.FileMode(0o666), not uint32(0o666)). Default is never a starlark.Value and
//     never a raw string at the runtime layer.
//
// Variadic and Kwargs are mutually exclusive with Optional and Default — the wire grammar rejects "*parts?" and
// "**kwargs?=foo" at parse time.
type Parameter struct {
	Name     string
	Type     reflect.Type
	Optional bool
	Variadic bool
	Kwargs   bool
	Default  any
}

// Action is a pure, infallible value transformer. No side effects, cannot fail.
//
// Do returns (result, nil, nil).
type Action interface {
	FullName() string
	Name() string
	Params() []Parameter
	Do(ctx *RuntimeEnvironment, slots map[string]any) (Result, Complement, error)
}

// FallibleAction has side effects and can fail.
//
// Do returns (result, nil, error).
type FallibleAction interface {
	Action
}

// CompensableAction has side effects, can fail, and can be undone.
//
// Do returns (result, complement, error).
type CompensableAction interface {
	Action
	Undo(ctx *RuntimeEnvironment, complement Complement) error
}
