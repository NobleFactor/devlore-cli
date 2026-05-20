// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
)

// splitReservedKwargs walks `kwargs` and partitions it into the three plan-reserved entries
// (`label`, `retry_policy`, `error_action`) and the remaining caller-supplied kwargs.
//
// Reserved-kwarg classification is plan-provider semantics — the bridge layer's job ends at
// generic starlark→Go conversion via [starlarkbridge.StarlarkToGoTyped]. The grammar:
//
//   - `label=<string>` — caller-supplied label for the invocation registry entry. Empty / absent
//     triggers [op.InvocationRegistry.AutoLabel] downstream.
//   - `retry_policy=<*op.RetryPolicy>` — resolved via StarlarkToGoTyped with target
//     reflect.TypeFor[*op.RetryPolicy](). None / absent → nil.
//   - `error_action=[invocation, ...]` — a starlark list of invocations; each element resolves
//     to *op.Invocation; the list materializes into a *op.Subgraph via
//     [subgraphFromInvocations] (same primitive that `body=` uses for `plan.subgraph`).
//
// Parameters:
//   - `env`: the runtime environment used by the conversion cascade.
//   - `kwargs`: the input kwarg tuple list.
//
// Returns:
//   - `filtered` []starlark.Tuple: kwargs with the three reserved entries removed. The input
//     slice is returned as-is when no reserved entry was present.
//   - `label` string: the supplied label, or empty.
//   - `retryPolicy` *op.RetryPolicy: the supplied retry policy, or nil.
//   - `errorAction` *op.Subgraph: the materialized error-handler subgraph, or nil.
//   - `error`: non-nil when any reserved entry has an invalid shape or fails conversion.
func splitReservedKwargs(
	env *op.RuntimeEnvironment,
	kwargs []starlark.Tuple,
) ([]starlark.Tuple, string, *op.RetryPolicy, *op.Subgraph, error) {

	var label string
	var retryPolicy *op.RetryPolicy
	var errorAction *op.Subgraph
	sawReserved := false

	for _, kv := range kwargs {

		if len(kv) != 2 {
			return nil, "", nil, nil, fmt.Errorf("kwarg tuple must have length 2, got %d", len(kv))
		}

		keyStr, ok := kv[0].(starlark.String)
		if !ok {
			return nil, "", nil, nil, fmt.Errorf("kwarg key must be a string, got %s", kv[0].Type())
		}
		key := string(keyStr)

		switch key {

		case "label":
			sawReserved = true
			s, ok := kv[1].(starlark.String)
			if !ok {
				return nil, "", nil, nil, fmt.Errorf("label= must be a string, got %s", kv[1].Type())
			}
			label = string(s)

		case "retry_policy":
			sawReserved = true
			value, err := starlarkbridge.StarlarkToGoTyped(env, kv[1], reflect.TypeFor[*op.RetryPolicy]())
			if err != nil {
				return nil, "", nil, nil, fmt.Errorf("retry_policy=: %w", err)
			}
			if value == nil {
				continue
			}
			policy, ok := value.(*op.RetryPolicy)
			if !ok {
				return nil, "", nil, nil, fmt.Errorf("retry_policy= must be *op.RetryPolicy or None, got %T", value)
			}
			retryPolicy = policy

		case "error_action":
			sawReserved = true
			subgraph, err := errorActionSubgraph(env, kv[1])
			if err != nil {
				return nil, "", nil, nil, err
			}
			errorAction = subgraph
		}
	}

	if !sawReserved {
		return kwargs, label, retryPolicy, errorAction, nil
	}

	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		keyStr, _ := kv[0].(starlark.String)
		key := string(keyStr)
		if key == "label" || key == "retry_policy" || key == "error_action" {
			continue
		}
		filtered = append(filtered, kv)
	}

	return filtered, label, retryPolicy, errorAction, nil
}

// errorActionSubgraph converts the value bound to `error_action=` into a *op.Subgraph.
//
// Accepted shapes:
//   - starlark None → nil (no error action).
//   - *starlark.List of *op.Invocation elements → *op.Subgraph via [subgraphFromInvocations].
//
// Any other shape is an error.
//
// Parameters:
//   - `env`: the runtime environment for the conversion cascade.
//   - `value`: the starlark value bound to `error_action=`.
//
// Returns:
//   - *op.Subgraph: the materialized error-handler subgraph, or nil for None.
//   - `error`: non-nil on shape errors or element-conversion failures.
func errorActionSubgraph(env *op.RuntimeEnvironment, value starlark.Value) (*op.Subgraph, error) {

	if _, isNone := value.(starlark.NoneType); isNone {
		return nil, nil
	}

	list, ok := value.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("error_action= must be a list of invocations, got %s", value.Type())
	}

	invocations := make([]*op.Invocation, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()

	invocationType := reflect.TypeFor[*op.Invocation]()

	var element starlark.Value
	for iter.Next(&element) {
		converted, err := starlarkbridge.StarlarkToGoTyped(env, element, invocationType)
		if err != nil {
			return nil, fmt.Errorf("error_action=: %w", err)
		}
		invocation, ok := converted.(*op.Invocation)
		if !ok {
			return nil, fmt.Errorf("error_action= list element must be *op.Invocation, got %T", converted)
		}
		invocations = append(invocations, invocation)
	}

	return subgraphFromInvocations("error_action", invocations), nil
}

// projectToSlotValue projects a Go value (post-[starlarkbridge.StarlarkToGoTyped] with target=any)
// into a [op.SlotValue]. Same projection that flow's planner uses for kwarg values:
//
//   - *op.Invocation → PromiseValue referencing the invocation's Target by ID.
//   - *op.Promise    → PromiseValue referencing the producing unit by ID with the producer's slot.
//   - *op.Variable   → VariableValue carrying the variable's name.
//   - anything else  → ImmediateValue wrapping the raw value.
//
// Used by [Provider.assembleBuiltin] to convert non-reserved kwargs into the
// `map[string]op.SlotValue` that [Provider.Assemble] expects for frame bindings on the graph root.
//
// Parameters:
//   - `value`: the Go value to project.
//
// Returns:
//   - op.SlotValue: the projected slot value.
func projectToSlotValue(value any) op.SlotValue {

	switch v := value.(type) {

	case *op.Invocation:
		return op.PromiseValue{UnitRef: v.Target.ID(), Slot: ""}

	case *op.Promise:
		return op.PromiseValue{UnitRef: v.Unit().ID(), Slot: v.Slot()}

	case *op.Variable:
		return op.VariableValue{Name: v.Name}

	default:
		return op.ImmediateValue{Value: value}
	}
}

// subgraphFromInvocations materializes a *op.Subgraph from a list of invocations by appending each
// invocation's Target as a child.
//
// The same primitive that drives `body=[...]` materialization in flow's SubgraphPlanner. Used by
// [errorActionSubgraph] for `error_action=[...]` so the executor's failure dispatch consumes a
// uniform *op.Subgraph shape.
//
// Parameters:
//   - `label`: the ID-generation prefix passed to [op.GenerateNodeID] (e.g., "error_action").
//   - `invocations`: the invocations whose Targets become the Subgraph's children, in order.
//
// Returns:
//   - *op.Subgraph: the assembled Subgraph.
func subgraphFromInvocations(label string, invocations []*op.Invocation) *op.Subgraph {

	subgraph := op.NewSubgraph(op.GenerateNodeID(label))
	for _, invocation := range invocations {
		subgraph.AddChild(invocation.Target)
	}
	return subgraph
}
