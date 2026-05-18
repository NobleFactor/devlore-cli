// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// containerMethods is the set of receiver-method names that the bridge dispatches as containers.
//
// For container calls, the bridge classifies reserved kwargs (body=, items=, error_action=,
// retry_policy=) directly into [op.Subgraph] fields rather than routing through the method's parameter
// list. Body children get their parentID stamped to the new subgraph's ID per plan-doc D11. The other
// three flow containers (choose, gather, wait_until) get added here as Steps 13/14/15 converge them to
// the uniform Subgraph(items []any, kwargs map[string]any) signature.
var containerMethods = map[string]bool{
	"flow.Subgraph": true,
}

// isContainerMethod reports whether actionName (e.g., "flow.Subgraph") is a registered container.
//
// Parameters:
//   - `actionName`: dotted "<provider>.<method>" name from [NodeBuilder.dispatch].
//
// Returns:
//   - `bool`: true if container; false for leaf methods.
func isContainerMethod(actionName string) bool {
	return containerMethods[actionName]
}

// dispatchContainer is the bridge entry for container method calls.
//
// Constructs an [op.Subgraph] directly (not an [op.Node]); classifies reserved kwargs into Subgraph
// fields; stamps parentID on body= children for plan-doc D11 ownership. The remaining kwargs (after
// reserved-kwarg extraction) populate [op.Subgraph.FrameBindings] as the kwarg-supplied frame
// bindings the executor uses to populate the dispatch-time Frame.
//
// Parameters:
//   - `label`: the starlark builtin label (e.g., "plan.subgraph" for the bare-root flow form).
//   - `actionName`: dotted "<provider>.<method>" name (e.g., "flow.Subgraph"), stamped on the
//     materialized op.Subgraph for execute-time lookup symmetry with op.Node.
//   - `args`: positional args from the starlark call. Containers reject positional args; use named
//     kwargs only.
//   - `kwargs`: caller-supplied keyword arguments.
//
// Returns:
//   - `starlark.Value`: the newly-registered *Invocation wrapping the constructed Subgraph.
//   - `error`: non-nil if reserved-kwarg parsing, slot conversion, or registry registration fails.
func (p *NodeBuilder) dispatchContainer(label, actionName string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if len(args) > 0 {
		return nil, fmt.Errorf("%s: positional args not supported for containers; use named kwargs", label)
	}

	bodyChildren, kwargs, err := extractBodyKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	errorAction, kwargs, err := extractErrorActionKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	retryPolicy, kwargs, err := extractRetryPolicyKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	itemsValue, kwargs, err := extractItemsKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	sg := op.NewSubgraph(op.GenerateNodeID(actionName))
	sg.Status = op.SubgraphPending

	for _, childInv := range bodyChildren {
		sg.AddChild(childInv.Target)
	}

	if errorAction != nil {
		sg.SetErrorAction(errorAction)
	}

	if retryPolicy != nil {
		sg.SetRetryPolicy(retryPolicy)
	}

	if itemsValue != nil {
		slotValue, err := starlarkValueToSlotValue(itemsValue)
		if err != nil {
			return nil, fmt.Errorf("%s: items: %w", label, err)
		}
		sg.Items = slotValue
	}

	if len(kwargs) > 0 {
		sg.FrameBindings = make(map[string]op.SlotValue, len(kwargs))
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			slotValue, err := starlarkValueToSlotValue(kv[1])
			if err != nil {
				return nil, fmt.Errorf("%s: frame binding %s: %w", label, key, err)
			}
			sg.FrameBindings[key] = slotValue
		}
	}

	inv := &Invocation{Label: p.registry.AutoLabel(label), Target: sg, Promise: nil}
	if err := p.registry.Register(inv.Label, inv); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	return inv, nil
}

// extractBodyKwarg pulls the "body" kwarg and parses its value as a list of *Invocation.
//
// The body= kwarg is reserved across all container methods: its elements become
// [op.Subgraph.Children] with their parentID stamped to the constructed subgraph's ID. Accepted
// value shapes for body=:
//
//   - `*starlark.List` where every element is *Invocation. Returns the slice of invocations.
//   - `starlark.NoneType`. Returns nil; treated as "no children."
//   - Anything else. Returns a descriptive error.
//
// Parameters:
//   - `kwargs`: the caller-supplied keyword arguments.
//
// Returns:
//   - `[]*Invocation`: the unwrapped body children, or nil if body= was absent or None.
//   - `[]starlark.Tuple`: kwargs with the "body" entry removed.
//   - `error`: non-nil if the body= value is of an unexpected type or contains non-Invocation elements.
func extractBodyKwarg(kwargs []starlark.Tuple) ([]*Invocation, []starlark.Tuple, error) {

	for i, kv := range kwargs {

		key, _ := starlark.AsString(kv[0])
		if key != "body" {
			continue
		}

		var children []*Invocation

		switch v := kv[1].(type) {

		case *starlark.List:

			children = make([]*Invocation, 0, v.Len())
			for j := 0; j < v.Len(); j++ {
				inv, ok := v.Index(j).(*Invocation)
				if !ok {
					return nil, nil, fmt.Errorf("body[%d]: expected Invocation, got %s", j, v.Index(j).Type())
				}
				children = append(children, inv)
			}

		case starlark.NoneType:

			// explicit None — treated as no body

		default:

			return nil, nil, fmt.Errorf("body: expected list of invocations, got %s", kv[1].Type())
		}

		filtered := make([]starlark.Tuple, 0, len(kwargs)-1)
		filtered = append(filtered, kwargs[:i]...)
		filtered = append(filtered, kwargs[i+1:]...)

		return children, filtered, nil
	}

	return nil, kwargs, nil
}

// extractErrorActionKwarg pulls the "error_action" kwarg and parses its value as an [op.ExecutableUnit].
//
// Accepted value shapes:
//
//   - `*Invocation`. Returns the Invocation's Target as the ExecutableUnit.
//   - `starlark.NoneType`. Returns nil; treated as "no error action" (executor falls back to
//     flow.Provider.Failed sentinel at dispatch time).
//   - Anything else. Returns a descriptive error.
//
// Parameters:
//   - `kwargs`: the caller-supplied keyword arguments.
//
// Returns:
//   - `op.ExecutableUnit`: the error-handling unit, or nil if error_action= was absent or None.
//   - `[]starlark.Tuple`: kwargs with the "error_action" entry removed.
//   - `error`: non-nil if the error_action= value is of an unexpected type.
func extractErrorActionKwarg(kwargs []starlark.Tuple) (op.ExecutableUnit, []starlark.Tuple, error) {

	for i, kv := range kwargs {

		key, _ := starlark.AsString(kv[0])
		if key != "error_action" {
			continue
		}

		var unit op.ExecutableUnit

		switch v := kv[1].(type) {

		case *Invocation:

			unit = v.Target

		case starlark.NoneType:

			// explicit None — treated as no error action

		default:

			return nil, nil, fmt.Errorf("error_action: expected Invocation, got %s", kv[1].Type())
		}

		filtered := make([]starlark.Tuple, 0, len(kwargs)-1)
		filtered = append(filtered, kwargs[:i]...)
		filtered = append(filtered, kwargs[i+1:]...)

		return unit, filtered, nil
	}

	return nil, kwargs, nil
}

// extractRetryPolicyKwarg pulls the "retry_policy" kwarg and parses its value as a [*op.RetryPolicy].
//
// Accepted value shapes:
//
//   - `*goReceiver` around a `*op.RetryPolicy` (produced by a future plan.retry_policy(...) builtin or
//     a Go-side constructor projected through the bridge). Returns the *op.RetryPolicy.
//   - `starlark.NoneType`. Returns nil; the executor falls back per the D11 frame-chain inheritance rule.
//   - Anything else. Returns a descriptive error.
//
// Parameters:
//   - `kwargs`: the caller-supplied keyword arguments.
//
// Returns:
//   - `*op.RetryPolicy`: the parsed policy, or nil if retry_policy= was absent or None.
//   - `[]starlark.Tuple`: kwargs with the "retry_policy" entry removed.
//   - `error`: non-nil if the retry_policy= value is of an unexpected type.
func extractRetryPolicyKwarg(kwargs []starlark.Tuple) (*op.RetryPolicy, []starlark.Tuple, error) {

	for i, kv := range kwargs {

		key, _ := starlark.AsString(kv[0])
		if key != "retry_policy" {
			continue
		}

		var policy *op.RetryPolicy

		switch v := kv[1].(type) {

		case *goReceiver:

			rp, ok := v.instance.(*op.RetryPolicy)
			if !ok {
				return nil, nil, fmt.Errorf("retry_policy: expected *op.RetryPolicy, got starlark receiver around %T", v.instance)
			}

			policy = rp

		case starlark.NoneType:

			// explicit None — treated as no retry policy

		default:

			return nil, nil, fmt.Errorf("retry_policy: expected *op.RetryPolicy, got %s", kv[1].Type())
		}

		filtered := make([]starlark.Tuple, 0, len(kwargs)-1)
		filtered = append(filtered, kwargs[:i]...)
		filtered = append(filtered, kwargs[i+1:]...)

		return policy, filtered, nil
	}

	return nil, kwargs, nil
}

// extractItemsKwarg pulls the "items" kwarg and returns its raw [starlark.Value] for later conversion.
//
// Unlike body=/error_action=/retry_policy=, items= is type-agnostic at extract time — the value can be a
// concrete list, a Variable reference, or a Promise. Conversion to [op.SlotValue] happens at the caller
// (see [starlarkValueToSlotValue]).
//
// Parameters:
//   - `kwargs`: the caller-supplied keyword arguments.
//
// Returns:
//   - `starlark.Value`: the items= value, or nil if absent.
//   - `[]starlark.Tuple`: kwargs with the "items" entry removed.
//   - `error`: always nil. Reserved for future type-pre-validation.
func extractItemsKwarg(kwargs []starlark.Tuple) (starlark.Value, []starlark.Tuple, error) {

	for i, kv := range kwargs {

		key, _ := starlark.AsString(kv[0])
		if key != "items" {
			continue
		}

		filtered := make([]starlark.Tuple, 0, len(kwargs)-1)
		filtered = append(filtered, kwargs[:i]...)
		filtered = append(filtered, kwargs[i+1:]...)

		return kv[1], filtered, nil
	}

	return nil, kwargs, nil
}

// starlarkValueToSlotValue converts a [starlark.Value] into an [op.SlotValue] without requiring a target
// type or a hosting [op.Slot]. Mirrors the dispatch surface of [NodeBuilder.fillSlot] but returns the
// SlotValue directly so container paths can stash it on [op.Subgraph.Items] or
// [op.Subgraph.FrameBindings] without inventing a placeholder slot.
//
// Conversion table:
//
//   - `*Invocation` → [op.PromiseValue]{NodeRef: inv.Target.ID(), Slot: ""} (the invocation's terminal
//     output).
//   - `*goReceiver` around `*op.Variable` → [op.VariableValue]{Name: v.Name} (the plan.variable("name")
//     reference).
//   - Anything else → [op.ImmediateValue]{Value: sv} (raw starlark value; consumers downstream are
//     responsible for projecting to the required Go type. A future iteration may route literals through
//     [toGo] when the container's expected type is known).
//
// Parameters:
//   - `sv`: the starlark value to convert.
//
// Returns:
//   - `op.SlotValue`: the conversion result.
//   - `error`: always nil for now. Reserved for future type-validation paths.
func starlarkValueToSlotValue(sv starlark.Value) (op.SlotValue, error) {

	if inv, ok := sv.(*Invocation); ok {
		return op.PromiseValue{NodeRef: inv.Target.ID(), Slot: ""}, nil
	}

	if gr, ok := sv.(*goReceiver); ok {
		if v, ok := gr.instance.(*op.Variable); ok {
			return op.VariableValue{Name: v.Name}, nil
		}
	}

	return op.ImmediateValue{Value: sv}, nil
}
