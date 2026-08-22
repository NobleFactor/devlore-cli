// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"reflect"
)

// ErrNotCompensable signals that "Do" acknowledges rollback but cannot undo its effect.
//
// The executor logs a warning and continues unwinding.
var ErrNotCompensable = errors.New("action is not compensable")

// ActionName is the short dotted action label — `<receiver>.<snake_method>`, e.g. "file.write_text".
//
// It is the script-facing vocabulary: the name a `.star` line invokes, the value [Action.Name] reports, and the key
// [receiverRegistry.BuildAction] / [RuntimeEnvironment.ActionByName] / plan.Provider.Plan resolve. Codegen emits one
// typed constant per action method into the provider package root (`file.WriteText`), so Go callers get completion
// and a typo is a compile error instead of a runtime lookup failure.
//
// The fully-qualified type identity — [Method.ActionName] / [Action.FullName], the `<pkg-path>.<receiver>.<method>`
// receipt-stamp form — is a different concept (reflect's package-path identity) and deliberately stays `string`.
type ActionName string

// Action is a pure, infallible value transformer.
//
// Do returns (result, nil, nil). An [Action] has no side effects and cannot fail.
type Action interface {
	FullName() string
	Name() ActionName
	Method() *Method
	Params() []Parameter
	Do(activationRecord *ActivationRecord) (Result, Compensator, error)
}

// FallibleAction has side effects and can fail.
//
// Do returns (result, nil, error).
type FallibleAction interface {
	Action
}

// CompensableAction has side effects, can fail, and can be undone.
//
// Do returns (result, compensator, error).
type CompensableAction interface {
	Action
	Undo(activationRecord *ActivationRecord, compensator Compensator) error
}

// Compensator reverses its own effects during saga rollback — the Composite "component" of the recovery tree.
//
// A leaf receipt (via [ReceiptBase]) compensates by invoking its compensating action; a [*RecoveryStack] compensates
// by unwinding its children LIFO. Every compensable Do returns a Compensator as its second result (nil for
// non-compensable actions), which the executor invokes during rollback.
type Compensator interface {

	// Compensate reverses the effects captured when this compensator was produced, using `runtimeEnvironment` to
	// resolve and dispatch the reversal.
	Compensate(runtimeEnvironment *RuntimeEnvironment) error
}

// Parameter describes a single parameter accepted by an Action's Do method.
//
// Parameter is the runtime-typed form of a parameter token produced by codegen. The parameter token (e.g.,
// "destination_path", "mode?", "mode?=0o666", "*parts", "**kwargs") is cracked at the announce boundary by
// parseParameterToken, which populates every field below. Downstream consumers — Method.Invoke, slot-fill in the
// starlark bridge, error reporting — read these fields directly and never re-parse the token.
//
// Field invariants:
//   - Name is the bare parameter name with no decoration (no leading "*"/"**", no trailing "?", no "=value"
//     suffix). It is the canonical key for slots[Name] lookups and for kwarg matching.
//   - Type is the Go reflect.Type the dispatch site projects values into via op.Convert.
//   - Optional is true when the caller may omit this slot. Set by parseParameterToken in three cases: the
//     parameter token carries "?", or the parameter is Variadic, or the parameter is Kwargs. Variadic and Kwargs are
//     inherently "zero or more" — the caller may always omit positional overflow or extra keyword args — so
//     Optional being true for them lets consumers ask one question ("may caller omit this slot?") without
//     special-casing the Variadic / Kwargs flags. If Default is non-nil, slot-fill substitutes it.
//   - Variadic is true for tokens with a leading single "*". The Go method declares the parameter as a slice;
//     the dispatch site collects positional overflow into it.
//   - Kwargs is true for tokens with a leading "**". The Go method declares the parameter as map[string]any;
//     the dispatch site collects unknown keyword arguments into it.
//   - Default holds a Go-native value assignable to Type (or nil iff the parameter has no default). The dynamic
//     type inside the any box always matches Type exactly — parseDefaultExpression widens the parsed primitive
//     to Type's named form (e.g., os.FileMode(0o666), not uint32(0o666)). Default is never a starlark.Value and
//     never a raw string at the runtime layer.
//
// Variadic and Kwargs cannot carry an explicit "?" or "=value" at the token level — the grammar rejects
// "*parts?" and "**kwargs?=foo" at parse time. Their Optional bit is set by parseParameterToken on the basis
// of the marker alone, and Default is always nil for them.
type Parameter struct {
	Name     string
	Type     reflect.Type
	Optional bool
	Variadic bool
	Kwargs   bool
	Default  any
}

// Result is data that flows to downstream nodes via edges (e.g., file content, a rendered template, a query result).
//
// The executor stores this keyed by node ID and resolves promise slots from stored Results before calling downstream
// Do.
type Result = any
