// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// buildIterationFrame derives a per-iteration variable frame for [Provider.Gather].
//
// Shallow-copies `parent` (parent[k] references shared without deep copy), drops the gather-internal builtin
// names `items` and `limit` from the copy so iteration bodies never see them, and binds `item` to the supplied
// iteration value. Called once per iteration goroutine — each goroutine owns its returned map, so the parallel
// dispatches are race-free.
//
// Parameters:
//   - `parent`: the gather's enclosing variable frame (typically `activation.Variables`); may be nil for a
//     top-level gather.
//   - `item`: the value to bind to the `item` variable for this iteration.
//
// Returns:
//   - `map[string]Variable`: the per-iteration frame; never nil.
func buildIterationFrame(parent map[string]Variable, item any) map[string]Variable {

	frame := make(map[string]Variable, len(parent)+1)
	for k, v := range parent {
		if k == "items" || k == "limit" {
			continue
		}
		frame[k] = v
	}
	frame["item"] = op.Variable{Name: "item", Value: item}
	return frame
}

// dispatchBodyChildren walks the gather subgraph's children in declaration order and dispatches each via
// [op.Graph.ExecuteWithStack] with the supplied per-iteration `frame` and `stack`. Returns the last child's
// result (or nil when there are no children); short-circuits on first child error.
//
// Parameters:
//   - `ctx`: the iteration's cancellation context (scoped child of the gather's ctx).
//   - `graph`: the enclosing graph; supplies the executor entry point.
//   - `subgraph`: the gather's bound subgraph; its children form the iterated body.
//   - `stack`: the iteration-local [op.RecoveryStack] that accumulates per-child compensations.
//   - `frame`: the per-iteration variable frame (built by [buildIterationFrame]).
//
// Returns:
//   - `any`: the last child's terminal result, or nil for zero-child bodies.
//   - `error`: non-nil on cancellation or any child's dispatch failure.
func dispatchBodyChildren(ctx context.Context, graph *op.Graph, subgraph *op.Subgraph, stack *op.RecoveryStack, frame map[string]Variable) (any, error) {

	var last any
	for _, child := range subgraph.Children() {
		r, err := graph.ExecuteWithStack(ctx, child, stack, frame)
		if err != nil {
			return nil, err
		}
		last = r
	}
	return last, nil
}

// Variable re-exports [op.Variable] so flow's helpers can reference the framework type without dragging the
// `op.` qualifier through every line. Identical to [op.Variable]; lives in this package solely as a typing
// shortcut for [buildIterationFrame] and adjacent helpers.
type Variable = op.Variable

// isTruthy reports whether `value` satisfies the choose dispatch's truthiness rule.
//
// Mirrors starlark.Value.Truth() semantics for native Go types so [Provider.Choose]'s sequential walk
// produces the same outcome whether the case's When was supplied as a starlark literal that projected
// through the unmarshal pipeline or as a resolved Go value:
//
//   - `bool`: true is truthy; false is falsy.
//   - integer (`int`, `int64`, `uint`, `uint64`, ...): zero is falsy; non-zero is truthy.
//   - `string`: empty is falsy; non-empty is truthy.
//   - nil: falsy.
//   - anything else (op.Resource, non-nil pointer, struct, slice, map): truthy.
//
// When a Case's When carries a deferred reference (*op.Invocation / *op.Promise / starlark.Callable),
// [resolveDispatchedValue] unwraps it to a Go-native value before isTruthy is applied — so the same
// rule governs both literal and computed conditions.
//
// Parameters:
//   - `value`: the When value from a [Case], post-[resolveDispatchedValue].
//
// Returns:
//   - `bool`: true if `value` is truthy under the choose dispatch's rule.
func isTruthy(value any) bool {

	if value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	default:
		return true
	}
}

// resolveDispatchedValue resolves a [Case] field (When or Then) at dispatch time.
//
// When the field carries an [*op.Invocation] or [*op.Promise] reference — the typical shape coming out of
// `plan.case(when=upstream_inv, then=...)` — the upstream's resolved value is looked up from
// `activation.RuntimeEnvironment.Results` by the producing unit's ID. When the field carries a
// [starlark.Callable] (a lambda), the callable is invoked against the runtime environment's Thread and the
// result is unwrapped via [starlarkValueToGo]. All other shapes pass through unchanged: literals, nil,
// structs, etc. Necessary because [op.ImmediateValue.Resolve] does not recurse into nested struct fields,
// so a [Case] stashed in a slot still carries its raw deferred references.
//
// Parameters:
//   - `value`: the raw Case-field value (potentially carrying a deferred reference).
//   - `activation`: the dispatch activation; supplies the runtime environment whose Results map holds
//     upstream invocations' resolved values and whose Thread is used to invoke lambdas.
//
// Returns:
//   - `any`: the resolved value when a deferred reference can be looked up or invoked; `value` unchanged
//     otherwise.
func resolveDispatchedValue(value any, activation *op.ActivationRecord) any {

	if activation == nil || activation.RuntimeEnvironment == nil {
		return value
	}
	results := activation.RuntimeEnvironment.Results

	switch v := value.(type) {
	case *op.Invocation:
		if v == nil || v.Target == nil || results == nil {
			return value
		}
		if resolved, ok := results[v.Target.ID()]; ok {
			return resolved
		}
		return value
	case *op.Promise:
		if v == nil || results == nil {
			return value
		}
		if resolved, ok := results[v.Unit().ID()]; ok {
			return resolved
		}
		return value
	case starlark.Callable:
		// Lambda / starlark callable used as a Case field: invoke it with no args against the runtime
		// environment's Thread. The result is unwrapped to a Go-native value so both Choose's truthiness
		// check (When) and the caller's downstream consumption (Then) see usable values. Errors during the
		// call resolve as falsy (the case won't fire).
		result, err := starlark.Call(&activation.RuntimeEnvironment.Thread, v, nil, nil)
		if err != nil {
			return false
		}
		return starlarkValueToGo(result)
	default:
		return value
	}
}

// starlarkValueToGo unwraps a [starlark.Value] returned by an invoked lambda into a Go-native value.
//
// Used by [resolveDispatchedValue] to convert a lambda's result so downstream consumers receive a usable Go
// value (`bool`, `string`, `int64`, `float64`, or nil) rather than the wrapped starlark type. Unknown types
// pass through as the original starlark.Value — [isTruthy] treats any non-nil value as truthy, preserving
// the Choose dispatch's contract.
//
// Parameters:
//   - `v`: the starlark value to unwrap.
//
// Returns:
//   - `any`: the Go-native equivalent, or `v` unchanged for types this converter does not handle.
func starlarkValueToGo(v starlark.Value) any {

	if v == nil {
		return nil
	}

	switch s := v.(type) {
	case starlark.NoneType:
		return nil
	case starlark.Bool:
		return bool(s)
	case starlark.Int:
		if i, ok := s.Int64(); ok {
			return i
		}
		return s
	case starlark.Float:
		return float64(s)
	case starlark.String:
		return string(s)
	default:
		return v
	}
}
