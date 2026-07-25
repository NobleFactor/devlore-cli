// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
)

// dispatchBuiltinBody returns a [starlark.Builtin] body that routes one plan-mode call through [Provider.invocation].
//
// Shared by Tier-1 dispatch ([adapter.Attr], one builtin per sub-namespace method) and Tier-2 dispatch
// ([Provider.buildPromotedBuiltins], one builtin per root-provider method promoted to the flat `plan.*` namespace).
//
// Flow inside the closure:
//
//  1. Split reserved kwargs (label / retry_policy / on_error) via [splitReservedKwargs].
//  2. Convert positional args to Go via per-arg [starlarkbridge.StarlarkToGoTyped] with target `any`.
//  3. Convert the remaining (non-reserved) kwargs to Go the same way.
//  4. Call [Provider.invocation] with the resolved args / kwargs / reserved-kwarg payload.
//  5. Wrap the resulting [*op.Invocation] via [starlarkbridge.NewGoReceiver] so it presents to starlark
//     with the same attribute surface other receivers do.
//
// Parameters:
//   - `provider`: the plan.Provider owning the invocation registry and runtime environment.
//   - `receiverType`: the receiver type whose method `methodName` lives on.
//   - `methodName`: the Go method name (CamelCase) to dispatch through [Provider.invocation].
//   - `actionName`: the qualified action name used in error messages (e.g., `"file.write_text"` for Tier-1,
//     `"flow.complete"` for Tier-2).
//
// Returns:
//   - func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error): the
//     starlark.Builtin closure body that wraps the dispatch flow above.
func dispatchBuiltinBody(
	provider *Provider,
	receiverType op.ProviderReceiverType,
	methodName, actionName string,
) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {

	return func(
		_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {

		env := provider.RuntimeEnvironment()

		filtered, label, retryPolicy, onError, onRetry, transitionPolicy, err := splitReservedKwargs(env, kwargs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", actionName, err)
		}

		anyType := reflect.TypeFor[any]()

		goArgs := make([]any, len(args))
		for i, sv := range args {
			value, err := starlarkbridge.StarlarkToGoTyped(env, sv, anyType)
			if err != nil {
				return nil, fmt.Errorf("%s: arg %d: %w", actionName, i, err)
			}
			goArgs[i] = value
		}

		goKwargs := make(map[string]any, len(filtered))
		for _, kv := range filtered {
			keyStr, _ := kv[0].(starlark.String)
			value, err := starlarkbridge.StarlarkToGoTyped(env, kv[1], anyType)
			if err != nil {
				return nil, fmt.Errorf("%s: kwarg %q: %w", actionName, string(keyStr), err)
			}
			goKwargs[string(keyStr)] = value
		}

		invocation, err := provider.invocation(
			receiverType,
			methodName,
			goArgs,
			goKwargs,
			retryPolicy,
			onError,
			onRetry,
			transitionPolicy,
			label,
		)
		if err != nil {
			return nil, err
		}

		return starlarkbridge.NewGoReceiver(invocation)
	}
}

// onErrorSubgraph converts the value bound to `on_error=` into a *op.Subgraph.
//
// Accepted shapes:
//   - starlark None → nil (no error action).
//   - *starlark.List of *op.Invocation elements → *op.Subgraph via [subgraphFromInvocations].
//
// Any other shape is an error.
//
// Parameters:
//   - `env`: the runtime environment for the conversion cascade.
//   - `value`: the starlark value bound to `on_error=`.
//
// Returns:
//   - *op.Subgraph: the materialized error-handler subgraph, or nil for None.
//   - `error`: non-nil on shape errors or element-conversion failures.
func onErrorSubgraph(env *op.RuntimeEnvironment, value starlark.Value) (*op.Subgraph, error) {

	if _, isNone := value.(starlark.NoneType); isNone {
		return nil, nil
	}

	list, ok := value.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("on_error= must be a list of invocations, got %s", value.Type())
	}

	invocations := make([]*op.Invocation, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()

	invocationType := reflect.TypeFor[*op.Invocation]()

	var element starlark.Value
	for iter.Next(&element) {
		converted, err := starlarkbridge.StarlarkToGoTyped(env, element, invocationType)
		if err != nil {
			return nil, fmt.Errorf("on_error=: %w", err)
		}
		invocation, ok := converted.(*op.Invocation)
		if !ok {
			return nil, fmt.Errorf("on_error= list element must be *op.Invocation, got %T", converted)
		}
		invocations = append(invocations, invocation)
	}

	return subgraphFromInvocations(env, "on_error", invocations)
}

// onRetrySubgraph converts the value bound to `on_retry=` into a *op.Subgraph.
//
// Accepted shapes:
//   - starlark None → nil (no retry handler).
//   - *starlark.List of *op.Invocation elements → *op.Subgraph via [subgraphFromInvocations].
//
// Any other shape is an error.
//
// Parameters:
//   - `env`: the runtime environment for the conversion cascade.
//   - `value`: the starlark value bound to `on_retry=`.
//
// Returns:
//   - *op.Subgraph: the materialized retry-handler subgraph, or nil for None.
//   - `error`: non-nil on shape errors or element-conversion failures.
func onRetrySubgraph(env *op.RuntimeEnvironment, value starlark.Value) (*op.Subgraph, error) {

	if _, isNone := value.(starlark.NoneType); isNone {
		return nil, nil
	}

	list, ok := value.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("on_retry= must be a list of invocations, got %s", value.Type())
	}

	invocations := make([]*op.Invocation, 0, list.Len())
	iter := list.Iterate()
	defer iter.Done()

	invocationType := reflect.TypeFor[*op.Invocation]()

	var element starlark.Value
	for iter.Next(&element) {
		converted, err := starlarkbridge.StarlarkToGoTyped(env, element, invocationType)
		if err != nil {
			return nil, fmt.Errorf("on_retry=: %w", err)
		}
		invocation, ok := converted.(*op.Invocation)
		if !ok {
			return nil, fmt.Errorf("on_retry= list element must be *op.Invocation, got %T", converted)
		}
		invocations = append(invocations, invocation)
	}

	return subgraphFromInvocations(env, "on_retry", invocations)
}

// projectToBinding projects a Go value into an [op.Binding] (PromiseBinding / VariableBinding / ImmediateBinding).
//
// The Go value is the post-[starlarkbridge.StarlarkToGoTyped] form (target=any). The projection:
//
//   - *op.Invocation → PromiseBinding referencing the invocation's Target by ID.
//   - *op.Variable   → VariableBinding carrying the variable's name.
//   - anything else  → ImmediateBinding wrapping the raw value.
//
// Used by [Provider.AssembleDefinition] to convert the kwarg sink (`map[string]any` from `**frame_bindings`) into the
// `map[string]op.Binding` shape `graph.Root.FrameBindings` expects.
//
// Parameters:
//   - `value`: the Go value to project.
//
// Returns:
//   - op.Binding: the projected binding.
func projectToBinding(value any) op.Binding {

	switch v := value.(type) {

	case *op.Invocation:
		return op.NewPromiseBinding(v.Target.ID())

	case *op.Variable:
		return op.NewVariableBindingWithField(v.Name, v.Field)

	default:
		return op.NewImmediateBinding(value)
	}
}

// splitReservedKwargs partitions `kwargs` into the five plan-reserved entries and the caller-supplied remainder.
//
// Reserved-kwarg classification is plan-provider semantics — the bridge layer's job ends at generic starlark→Go
// conversion via [starlarkbridge.StarlarkToGoTyped]. The grammar:
//
//   - `label=<string>` — caller-supplied label for the invocation registry entry. Empty / absent triggers
//     [op.InvocationRegistry.AutoLabel] downstream.
//   - `retry_policy=<*op.RetryPolicy>` — resolved via StarlarkToGoTyped with target
//     reflect.TypeFor[*op.RetryPolicy](). None / absent → nil.
//   - `transition_policy=<*op.TransitionPolicy>` — resolved via StarlarkToGoTyped with target
//     reflect.TypeFor[*op.TransitionPolicy](). None / absent → nil.
//   - `on_error=[invocation, ...]` — a starlark list of invocations; each element resolves to *op.Invocation;
//     the list materializes into a *op.Subgraph via [subgraphFromInvocations] (same primitive that `body=` uses
//     for `plan.subgraph`).
//   - `on_retry=[invocation, ...]` — same shape as `on_error=`; materializes into the per-attempt retry-handler
//     *op.Subgraph.
//
// Parameters:
//   - `env`: the runtime environment used by the conversion cascade.
//   - `kwargs`: the input kwarg tuple list.
//
// Returns:
//   - []starlark.Tuple: kwargs with the five reserved entries removed. The input slice is returned as-is when no
//     reserved entry was present.
//   - `string`: the supplied label, or empty.
//   - *op.RetryPolicy: the supplied retry policy, or nil.
//   - *op.Subgraph: the materialized error-handler subgraph, or nil.
//   - *op.Subgraph: the materialized retry-handler subgraph, or nil.
//   - *op.TransitionPolicy: the supplied transition policy, or nil.
//   - `error`: non-nil when any reserved entry has an invalid shape or fails conversion.
func splitReservedKwargs(
	env *op.RuntimeEnvironment,
	kwargs []starlark.Tuple,
) ([]starlark.Tuple, string, *op.RetryPolicy, *op.Subgraph, *op.Subgraph, *op.TransitionPolicy, error) {

	var label string
	var retryPolicy *op.RetryPolicy
	var onError *op.Subgraph
	var onRetry *op.Subgraph
	var transitionPolicy *op.TransitionPolicy
	sawReserved := false

	for _, kv := range kwargs {

		if len(kv) != 2 {
			return nil, "", nil, nil, nil, nil, fmt.Errorf("kwarg tuple must have length 2, got %d", len(kv))
		}

		keyStr, ok := kv[0].(starlark.String)
		if !ok {
			return nil, "", nil, nil, nil, nil, fmt.Errorf("kwarg key must be a string, got %s", kv[0].Type())
		}
		key := string(keyStr)

		switch key {

		case "label":
			sawReserved = true
			s, ok := kv[1].(starlark.String)
			if !ok {
				return nil, "", nil, nil, nil, nil, fmt.Errorf("label= must be a string, got %s", kv[1].Type())
			}
			label = string(s)

		case "retry_policy":
			sawReserved = true
			value, err := starlarkbridge.StarlarkToGoTyped(env, kv[1], reflect.TypeFor[*op.RetryPolicy]())
			if err != nil {
				return nil, "", nil, nil, nil, nil, fmt.Errorf("retry_policy=: %w", err)
			}
			if value == nil {
				continue
			}
			policy, ok := value.(*op.RetryPolicy)
			if !ok {
				return nil, "", nil, nil, nil, nil, fmt.Errorf("retry_policy= must be *op.RetryPolicy or None, got %T", value)
			}
			retryPolicy = policy

		case "transition_policy":
			sawReserved = true
			value, err := starlarkbridge.StarlarkToGoTyped(env, kv[1], reflect.TypeFor[*op.TransitionPolicy]())
			if err != nil {
				return nil, "", nil, nil, nil, nil, fmt.Errorf("transition_policy=: %w", err)
			}
			if value == nil {
				continue
			}
			policy, ok := value.(*op.TransitionPolicy)
			if !ok {
				return nil, "", nil, nil, nil, nil, fmt.Errorf("transition_policy= must be *op.TransitionPolicy or None, got %T", value)
			}
			transitionPolicy = policy

		case "on_error":
			sawReserved = true
			subgraph, err := onErrorSubgraph(env, kv[1])
			if err != nil {
				return nil, "", nil, nil, nil, nil, err
			}
			onError = subgraph

		case "on_retry":
			sawReserved = true
			subgraph, err := onRetrySubgraph(env, kv[1])
			if err != nil {
				return nil, "", nil, nil, nil, nil, err
			}
			onRetry = subgraph
		}
	}

	if !sawReserved {
		return kwargs, label, retryPolicy, onError, onRetry, transitionPolicy, nil
	}

	filtered := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		keyStr, _ := kv[0].(starlark.String)
		key := string(keyStr)
		if key == "label" || key == "retry_policy" || key == "on_error" || key == "on_retry" || key == "transition_policy" {
			continue
		}
		filtered = append(filtered, kv)
	}

	return filtered, label, retryPolicy, onError, onRetry, transitionPolicy, nil
}

// subgraphFromInvocations materializes a *op.Subgraph from a list of invocations, bound to flow.subgraph.
//
// Appends each invocation's Target as a child of the new Subgraph. The Subgraph is bound to the canonical
// flow.subgraph action so it dispatches as a plain container at execute time. Same primitive that drives
// `body=[...]` materialization in flow's SubgraphPlanner. Used by [onErrorSubgraph] for `on_error=[...]`
// so the executor's failure dispatch consumes a uniform *op.Subgraph shape.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves flow.subgraph to its bound action.
//   - `label`: the ID-generation prefix passed to [op.GenerateNodeID] (e.g., `"on_error"`).
//   - `invocations`: the invocations whose Targets become the Subgraph's children, in order.
//
// Returns:
//   - *op.Subgraph: the assembled Subgraph.
//   - `error`: non-nil if the flow.subgraph action cannot be resolved through env's registry.
func subgraphFromInvocations(
	env *op.RuntimeEnvironment, label string, invocations []*op.Invocation,
) (*op.Subgraph, error) {

	action, err := op.ReceiverRegistry().BuildAction(flow.Subgraph)
	if err != nil {
		return nil, fmt.Errorf("subgraphFromInvocations: %w", err)
	}

	children := make([]op.ExecutableUnit, 0, len(invocations))
	for _, invocation := range invocations {
		children = append(children, invocation.Target)
	}

	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().
		WithID(op.GenerateNodeID(label)).
		WithAction(action).
		WithChildren(children...))
	if err != nil {
		return nil, fmt.Errorf("subgraphFromInvocations: %w", err)
	}
	return subgraph, nil
}
