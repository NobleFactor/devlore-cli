// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"fmt"
	"os"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// resolveBodyChildren extracts each invocation's Target from `body` and returns the resulting
// [op.ExecutableUnit] slice for a flow subgraph's children.
//
// Used by the gather/subgraph/choose planners to gather the body= kwarg into the children list that
// gets handed to [op.NewSubgraph]. Returns nil children when `body` is empty (caller passes nil to
// NewSubgraph).
//
// Parameters:
//   - `body`: the body= kwarg value; must be a []any of *op.Invocation.
//
// Returns:
//   - `[]op.ExecutableUnit`: the resolved children, in declaration order. Nil when `body` is empty.
//   - `error`: non-nil if `body` is not a list or contains a non-invocation element.
func resolveBodyChildren(body any) ([]op.ExecutableUnit, error) {

	list, ok := body.([]any)
	if !ok {
		return nil, fmt.Errorf("flow planner: body= must be a list, got %T", body)
	}

	if len(list) == 0 {
		return nil, nil
	}

	children := make([]op.ExecutableUnit, 0, len(list))
	for i, elem := range list {
		inv, ok := elem.(*op.Invocation)
		if !ok {
			return nil, fmt.Errorf("flow planner: body[%d]: expected *op.Invocation, got %T", i, elem)
		}
		children = append(children, inv.Target)
	}

	return children, nil
}

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
//   - `map[string]op.Variable`: the per-iteration frame; never nil.
func buildIterationFrame(parent map[string]op.Variable, item any) map[string]op.Variable {

	frame := make(map[string]op.Variable, len(parent)+1)
	for k, v := range parent {
		if k == "items" || k == "limit" {
			continue
		}
		frame[k] = v
	}
	frame["item"] = op.Variable{Name: "item", Value: item}
	return frame
}

// walkSubgraphChildren dispatches `subgraph`'s children in declaration order on the supplied `frame`, with per-child
// retry.
//
// The single children-walk shared by [Provider.Subgraph] and [Provider.Gather]. Each child runs via
// [op.ActivationRecord.DispatchChild] — so a child carrying an [op.RetryPolicy] retries uniformly regardless of which
// caller drove the walk — and on the first child whose retry budget is exhausted, the walk short-circuits: when
// `errorAction` is non-nil it is dispatched once (best-effort, no retry) as an observation hook before the original
// child error is returned. Children's compensations accumulate on the supplied `stack`.
//
// Parameters:
//   - `activation`: the dispatch record; supplies the child-dispatch closure into the executor walk.
//   - `ctx`: the cancellation context for this walk ([Provider.Subgraph] passes `activation.Context`; [Provider.Gather]
//     passes its per-iteration scoped context).
//   - `subgraph`: the bound subgraph whose children form the walked body.
//   - `stack`: the recovery stack the children's compensations push onto.
//   - `frame`: the variable frame each child dispatches under ([Provider.Subgraph] passes `activation.Variables`;
//     [Provider.Gather] passes its per-iteration frame).
//   - `errorAction`: the failure-observation subgraph to dispatch once on a child's exhausted-retry failure, or nil to
//     skip the observation pass ([Provider.Gather] passes nil).
//
// Returns:
//   - `any`: the last child's terminal result, or nil for zero-child bodies / on failure.
//   - `error`: non-nil on cancellation or any child's exhausted-retry failure (wrapped with the child's ID).
func walkSubgraphChildren(
	activation *op.ActivationRecord,
	ctx context.Context,
	subgraph *op.Subgraph,
	stack *op.RecoveryStack,
	frame map[string]op.Variable,
	errorAction *op.Subgraph,
) (any, error) {

	var last any

	for _, child := range subgraph.Children() {

		result, err := activation.DispatchChild(ctx, child, stack, frame)
		if err != nil {

			if errorAction != nil {
				_, dispatchErr := activation.DispatchChild(ctx, errorAction, stack, frame)
				if dispatchErr != nil {
					// Observation hook — log the dispatch failure but still surface the original child error.
					fmt.Fprintf(os.Stderr, "flow: errorAction dispatch failed: %v\n", dispatchErr)
				}
			}

			return nil, fmt.Errorf("child %q: %w", child.ID(), err)
		}

		last = result
	}

	return last, nil
}

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
// `plan.case(when=upstream_inv, then=...)` — the upstream's resolved value is looked up from the dispatch's
// [op.RecoveryStack] via [op.RecoveryStack.ResultByUnitID], keyed on the producing unit's ID. When the field
// carries a [starlark.Callable] (a lambda), the callable is invoked against the runtime environment's Thread
// and the result is unwrapped via [starlarkValueToGo]. All other shapes pass through unchanged: literals, nil,
// structs, etc. Necessary because [op.ImmediateValue.Resolve] does not recurse into nested struct fields, so a
// [Case] stashed in a slot still carries its raw deferred references.
//
// Parameters:
//   - `value`: the raw Case-field value (potentially carrying a deferred reference).
//   - `activation`: the dispatch activation; supplies the [op.RecoveryStack] whose receipts hold upstream
//     units' resolved values and the runtime environment whose Thread is used to invoke lambdas.
//
// Returns:
//   - `any`: the resolved value when a deferred reference can be looked up or invoked; `value` unchanged
//     otherwise.
func resolveDispatchedValue(value any, activation *op.ActivationRecord) any {

	if activation == nil {
		return value
	}

	switch v := value.(type) {
	case *op.Invocation:
		if v == nil || v.Target == nil || activation.Stack == nil {
			return value
		}
		if resolved, ok := activation.Stack.ResultByUnitID(v.Target.ID()); ok {
			return resolved
		}
		return value
	case *op.Promise:
		if v == nil || activation.Stack == nil {
			return value
		}
		if resolved, ok := activation.Stack.ResultByUnitID(v.Unit().ID()); ok {
			return resolved
		}
		return value
	case starlark.Callable:
		if activation.RuntimeEnvironment == nil {
			return value
		}
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
