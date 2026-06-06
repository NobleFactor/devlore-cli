// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"fmt"
	"time"

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

// dispatchBodyChildren dispatches `subgraph`'s children in declaration order on the per-iteration `frame`.
//
// Each child runs via [op.ActivationRecord.DispatchChild] — the single executor-owned walk — with the
// supplied `stack` so per-iteration compensations accumulate locally; iteration short-circuits on the first
// child error.
//
// Parameters:
//   - `activation`: the gather's dispatch record; supplies the child-dispatch closure into the executor walk.
//   - `ctx`: the iteration's cancellation context (scoped child of the gather's ctx).
//   - `subgraph`: the gather's bound subgraph; its children form the iterated body.
//   - `stack`: the iteration-local [op.RecoveryStack] that accumulates per-child compensations.
//   - `frame`: the per-iteration variable frame (built by [buildIterationFrame]).
//
// Returns:
//   - `any`: the last child's terminal result, or nil for zero-child bodies.
//   - `error`: non-nil on cancellation or any child's dispatch failure.
func dispatchBodyChildren(
	activation *op.ActivationRecord,
	ctx context.Context,
	subgraph *op.Subgraph,
	stack *op.RecoveryStack,
	frame map[string]op.Variable,
) (any, error) {

	var last any
	for _, child := range subgraph.Children() {
		r, err := activation.DispatchChild(ctx, child, stack, frame)
		if err != nil {
			return nil, err
		}
		last = r
	}
	return last, nil
}

// dispatchWithRetry dispatches `child` via [op.ActivationRecord.DispatchChild], retrying per its [op.RetryPolicy].
//
// Retries until `child` succeeds, the policy's MaxAttempts is exhausted, or the activation's context is cancelled.
//
// Interim implementation: reads `child.RetryPolicy()` directly (no frame-chain effective-policy walk
// yet). A nil policy means one attempt with no retry; a non-nil policy with MaxAttempts == 0 is
// treated as the explicit opt-out (one attempt, terminates any future frame-chain walk).
//
// Backoff: between attempts, `policy.ComputeDelay(prevAttempt)` is honored. The wait is interruptible
// via `activation.Context` — a cancel returns `ctx.Err()` immediately rather than completing the
// delay.
//
// Parameters:
//   - `activation`: the per-dispatch record carrying the cancellation context and the
//     child-dispatch closure.
//   - `child`: the unit to dispatch (with retry).
//   - `stack`: the subgraph-local recovery stack that the child's compensations push onto.
//
// Returns:
//   - `any`: the child's terminal result on the succeeding attempt; nil when every attempt failed.
//   - `error`: nil when the child succeeds within its retry budget; otherwise the last failure from
//     the child (or `activation.Context.Err()` if cancelled mid-backoff).
func dispatchWithRetry(activation *op.ActivationRecord, child op.ExecutableUnit, stack *op.RecoveryStack) (any, error) {

	policy := child.RetryPolicy()

	maxAttempts := 1
	if policy != nil {
		maxAttempts = policy.MaxAttempts + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {

		if attempt > 0 && policy != nil {
			delay := policy.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-activation.Context.Done():
					return nil, activation.Context.Err()
				}
			}
		}

		result, err := activation.DispatchChild(activation.Context, child, stack, activation.Variables)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, lastErr
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
