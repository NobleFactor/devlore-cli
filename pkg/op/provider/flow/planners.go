// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// actionInvocationPlanner is the slice of the session host [ChoosePlanner] needs to desugar a lambda `default=` — the
// plan provider's Plan method. Declared locally so flow names the capability without importing the plan package.
type actionInvocationPlanner interface {
	Plan(name string, args []any, kwargs map[string]any) (*op.Invocation, error)
}

// reservedSubgraphKwargs lists the kwargs SubgraphPlanner.Plan classifies specially (`body=`).
//
// `body=` populates the subgraph's children via [op.Subgraph.AddChild]. All other kwargs — including `items=` —
// flow through `subgraph.SetSlot(name, value)` and land in the unified slot map. The combinator / frame-binding
// discriminator at dispatch time is method-signature-driven: slot names matching `unit.Action().Method()`
// parameters are combinator inputs; non-matching ones are frame bindings.
var reservedSubgraphKwargs = map[string]struct{}{
	"body": {},
}

// ChoosePlanner is the specialized [op.Planner] for flow.Provider.Choose.
//
// Builds the choose subgraph's binary decision tree at plan time (phase-8 step 10): the `default=` body seals into
// the default subgraph (a leaf), each positional `*Case` contributes its when- and then-subgraphs, and the guarded
// edges wire them — whenᵢ —[op.GuardTruthy]→ thenᵢ, whenᵢ —[op.GuardFalsy]→ whenᵢ₊₁, the last falsy edge landing on
// the default. [Provider.Choose] carries no selection logic; executing the topology is the selection. Zero cases is
// defined behavior (the switch-statement precedent): the default subgraph is the only child, no guarded edges are
// emitted, and the run-all walk executes it.
type ChoosePlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Choose.
//
// Consumes the `default=` kwarg (a body — a list of invocations, the plan.subgraph construction) and the positional
// cases (each a `*Case` from `plan.case(when=, then=)`); every other kwarg lands in the subgraph's unified slot map
// as a frame binding, like [SubgraphPlanner]. Children are laid out when₀, then₀, …, default and the decision tree's
// guarded edges are emitted alongside; [op.NewSubgraph] seals the topology and [op.ValidateGraph] enforces the
// guarded-subgraph invariant at the boundaries.
//
// Parameters:
//   - `invocator`: the session host; consulted only to desugar a lambda `default=` into a function.call invocation.
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Choose.
//   - `args`: positional arguments converted starlark → Go; every entry must be a `*Case`.
//   - `kwargs`: keyword arguments converted starlark → Go; `default=` is required and reserved (a body, or a lambda
//     desugared to one), the rest are frame bindings.
//   - `annotations`: plan-time annotations applied to the subgraph at construction.
//   - `errorAction`: the failure-handler subgraph applied to the subgraph at construction, or nil.
//   - `retryPolicy`: the retry policy applied to the subgraph at construction, or nil.
//
// Returns:
//   - `op.ExecutableUnit`: the constructed choose-shaped [*op.Subgraph].
//   - `error`: non-nil when `receiverType` or `method` is nil, `default=` is missing or malformed, or a positional
//     argument is not a `*Case`.
func (ChoosePlanner) Plan(
	invocator op.PlanInvocator,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	args []any,
	kwargs map[string]any,
	annotations map[string]any,
	errorAction *op.Subgraph,
	retryPolicy *op.RetryPolicy,
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.ChoosePlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.ChoosePlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())
	action := op.NewAction(receiverType, method, actionName)

	defaultBody, present := kwargs["default"]
	if !present {
		return nil, fmt.Errorf("flow.ChoosePlanner.Plan: %s: missing required kwarg %q", actionName, "default")
	}

	// A lambda default is sugar for a one-invocation body evaluating it via function.call, mirroring plan.case's
	// desugaring (settled 2026-07-02).
	if lambda, ok := defaultBody.(*starlark.Function); ok {
		planner, capable := invocator.(actionInvocationPlanner)
		if !capable {
			return nil, fmt.Errorf(
				"flow.ChoosePlanner.Plan: %s: a lambda default requires a planning session host", actionName)
		}
		invocation, planErr := planner.Plan("function.call", []any{lambda}, nil)
		if planErr != nil {
			return nil, fmt.Errorf("flow.ChoosePlanner.Plan: default: %w", planErr)
		}
		defaultBody = invocation
	}

	defaultSubgraph, err := bodySubgraph("default", defaultBody)
	if err != nil {
		return nil, fmt.Errorf("flow.ChoosePlanner.Plan: %w", err)
	}

	cases := make([]*Case, 0, len(args))
	for i, argument := range args {
		caseValue, ok := argument.(*Case)
		if !ok {
			return nil, fmt.Errorf(
				"flow.ChoosePlanner.Plan: %s: positional argument %d is %T, want *flow.Case (from plan.case)",
				actionName, i, argument)
		}
		cases = append(cases, caseValue)
	}

	// Lay out the decision tree: when₀, then₀, …, default as children; whenᵢ —truthy→ thenᵢ and whenᵢ —falsy→ the
	// next when (the last falsy edge lands on the default leaf).
	children := make([]op.ExecutableUnit, 0, 2*len(cases)+1)
	edges := make([]op.Edge, 0, 2*len(cases))

	for i, caseValue := range cases {
		when, then := &caseValue.When, &caseValue.Then
		children = append(children, when, then)

		falsyTarget := defaultSubgraph.ID()
		if i+1 < len(cases) {
			falsyTarget = cases[i+1].When.ID()
		}

		edges = append(edges,
			op.Edge{From: when.ID(), To: then.ID(), Guard: op.GuardTruthy},
			op.Edge{From: when.ID(), To: falsyTarget, Guard: op.GuardFalsy})
	}
	children = append(children, defaultSubgraph)

	slots := make(map[string]op.Binding)
	for key, value := range kwargs {
		if key == "default" {
			continue
		}
		slots[key] = projectKwargValue(value)
	}

	spec := op.NewSubgraphSpec().
		WithID(op.GenerateNodeID(actionName)).
		WithAction(action).
		WithAnnotations(annotations).
		WithChildren(children...).
		WithEdges(edges...).
		WithErrorAction(errorAction).
		WithRetryPolicy(retryPolicy)
	for name, value := range slots {
		spec.WithSlot(name, value)
	}

	subgraph, err := op.NewSubgraph(spec)
	if err != nil {
		return nil, fmt.Errorf("flow.ChoosePlanner.Plan: %w", err)
	}

	return subgraph, nil
}

// endregion

// endregion

// GatherPlanner is the specialized [op.Planner] for flow.Provider.Gather.
//
// Materializes a [*op.Subgraph] bound to flow.Gather with `body=` invocations adopted as iteration-template
// children, `items=` stamped into the unified slot map for the method's items parameter, and every other
// kwarg packed into the method's `**kwargs` sink (notably `limit=`). Stamps a sentinel `item` slot so the
// per-iteration binding established by [buildIterationFrame] masks any `plan.variable("item")` reference in
// the body from bubbling up to the session-level [op.VariableResolver]. The runtime semantics — iterating
// `items` and dispatching the adopted child subgraph N times with bounded concurrency `limit` and a fresh
// per-iteration frame — live in [Provider.Gather], not the planner.
type GatherPlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Gather.
//
// Reserves `body=` (intercepted before the param walk and adopted via [addBodyChildren], not stamped as a
// slot), then walks the method's declared parameter list and maps positional `args` and named `kwargs` onto
// the subgraph's slot map: the items parameter consumes a matching kwarg or positional, the `**kwargs` sink
// collects every unconsumed kwarg (including `limit=`), and Variable / Invocation / Promise values route
// through [projectKwargValue] before stamping. After the walk, stamps `item` as a frame-local sentinel
// (`nil` value) so child slot-references to `plan.variable("item")` are satisfied by the per-iteration frame
// rather than the bubble-up surface.
//
// Parameters:
//   - `invocator`: the session host (unused — Gather constructs its subgraph from `args` / `kwargs` alone).
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Gather.
//   - `args`: positional arguments converted starlark → Go.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed).
//   - `errorAction`: the failure-handler subgraph applied to the subgraph at construction, or nil.
//   - `retryPolicy`: the retry policy applied to the subgraph at construction, or nil.
//
// Returns:
//   - op.ExecutableUnit: the constructed gather-shaped [*op.Subgraph].
//   - `error`: non-nil when `receiverType` or `method` is nil, when `body=` is malformed, or a required
//     parameter is missing.
func (GatherPlanner) Plan(
	_ op.PlanInvocator,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	args []any,
	kwargs map[string]any,
	annotations map[string]any,
	errorAction *op.Subgraph,
	retryPolicy *op.RetryPolicy,
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.GatherPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.GatherPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())
	action := op.NewAction(receiverType, method, actionName)

	// Gather children from the body= kwarg.
	var children []op.ExecutableUnit
	if body, present := kwargs["body"]; present {
		var err error
		children, err = resolveBodyChildren(body)
		if err != nil {
			return nil, fmt.Errorf("flow.GatherPlanner.Plan: %w", err)
		}
	}

	// Gather slot bindings from positional/kwargs against the method's parameter list.
	slots := make(map[string]op.Binding)
	params := method.Parameters()
	consumed := map[string]bool{"body": true}
	positional := 0

	for _, param := range params {

		if param.Variadic {
			rest := make([]any, 0, max(0, len(args)-positional))
			for ; positional < len(args); positional++ {
				rest = append(rest, args[positional])
			}
			slots[param.Name] = op.NewImmediateBinding(rest)
			continue
		}

		if param.Kwargs {
			remaining := make(map[string]any, len(kwargs))
			for k, v := range kwargs {
				if !consumed[k] {
					remaining[k] = v
				}
			}
			slots[param.Name] = op.NewImmediateBinding(remaining)
			continue
		}

		var value any
		var present bool

		if positional < len(args) {
			value = args[positional]
			positional++
			present = true
		} else if v, ok := kwargs[param.Name]; ok {
			value = v
			consumed[param.Name] = true
			present = true
		}

		if !present {
			if param.Default != nil {
				slots[param.Name] = op.NewImmediateBinding(param.Default)
				continue
			}
			if !param.Optional {
				return nil, fmt.Errorf("flow.GatherPlanner.Plan: %s: missing required parameter %q", actionName, param.Name)
			}
			continue
		}

		slots[param.Name] = projectKwargValue(value)
	}

	// Declare `item` as a frame-local on the gather subgraph so children that reference `plan.variable("item")`
	// (the PowerShell-style `$_` per-iteration binding) are satisfied by the per-iteration frame
	// [Provider.Gather] mints rather than bubbling up to the session-level [op.VariableResolver]. The stamped
	// value is a sentinel — the actual per-iteration value is supplied by [buildIterationFrame] at dispatch.
	slots["item"] = op.NewImmediateBinding(nil)

	spec := op.NewSubgraphSpec().
		WithID(op.GenerateNodeID(actionName)).
		WithAction(action).
		WithAnnotations(annotations).
		WithChildren(children...).
		WithErrorAction(errorAction).
		WithRetryPolicy(retryPolicy)
	for name, value := range slots {
		spec.WithSlot(name, value)
	}

	subgraph, err := op.NewSubgraph(spec)
	if err != nil {
		return nil, fmt.Errorf("flow.GatherPlanner.Plan: %w", err)
	}

	return subgraph, nil
}

// endregion

// endregion

// SubgraphPlanner is the specialized [op.Planner] for flow.Provider.Subgraph.
//
// Classifies the call's kwargs into two partitions: `body=` children (added via [op.Subgraph.AddChild],
// which stamps each child's parent ID) and everything else (stamped into the subgraph's unified slot map
// via [op.Subgraph.SetSlot]). The dispatch-time discriminator between combinator inputs and frame bindings
// is method-signature-driven, not planner-side.
type SubgraphPlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Subgraph.
//
// Parameters:
//   - `invocator`: the session host (unused today; future kwarg-classification rules may consult it).
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Subgraph.
//   - `args`: positional arguments; unused — flow.Subgraph has no positional surface today.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed); `body=` becomes
//     children, every other entry becomes a slot value.
//   - `errorAction`: the failure-handler subgraph applied to the subgraph at construction, or nil.
//   - `retryPolicy`: the retry policy applied to the subgraph at construction, or nil.
//
// Returns:
//   - op.ExecutableUnit: the constructed [*op.Subgraph] with classified kwargs applied.
//   - `error`: non-nil if `body=` is not a list, contains a non-invocation element, or `items=` is
//     malformed.
func (SubgraphPlanner) Plan(
	_ op.PlanInvocator,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	_ []any,
	kwargs map[string]any,
	annotations map[string]any,
	errorAction *op.Subgraph,
	retryPolicy *op.RetryPolicy,
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())
	action := op.NewAction(receiverType, method, actionName)

	// Gather children from the body= kwarg.
	var children []op.ExecutableUnit
	if body, present := kwargs["body"]; present {
		var err error
		children, err = resolveBodyChildren(body)
		if err != nil {
			return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: %w", err)
		}
	}

	// Every kwarg except `body=` lands in the unified slot map. The dispatch-time discriminator
	// (combinator input vs frame binding) is method-signature-driven.
	slots := make(map[string]op.Binding)
	for key, value := range kwargs {
		if _, reserved := reservedSubgraphKwargs[key]; reserved {
			continue
		}
		slots[key] = projectKwargValue(value)
	}

	// Default the `items` slot to an empty list when the caller doesn't supply one. [flow.Provider.Subgraph]
	// treats `len(items) == 0` as "dispatch children sequentially with no per-iteration scope," which is the
	// common `plan.subgraph(body=[...])` case. Without this default, [op.ValidateGraph] would reject the
	// subgraph as "required parameter `items` not bound" even though the method handles the zero case
	// correctly at dispatch.
	if _, present := slots["items"]; !present {
		slots["items"] = op.NewImmediateBinding([]any{})
	}

	spec := op.NewSubgraphSpec().
		WithID(op.GenerateNodeID(actionName)).
		WithAction(action).
		WithAnnotations(annotations).
		WithChildren(children...).
		WithErrorAction(errorAction).
		WithRetryPolicy(retryPolicy)
	for name, value := range slots {
		spec.WithSlot(name, value)
	}

	subgraph, err := op.NewSubgraph(spec)
	if err != nil {
		return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: %w", err)
	}

	return subgraph, nil
}

// endregion

// endregion

// WaitUntilPlanner is the specialized [op.Planner] for flow.Provider.WaitUntil.
//
// Materializes a [*op.Subgraph] that polls a predicate against a target until truthy or until timeout.
type WaitUntilPlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.WaitUntil.
//
// Constructs a single [*op.Subgraph] bound to flow.WaitUntil and stamps every kwarg into the subgraph's slot
// map via [op.Subgraph.SetSlot]. The polling cadence (target, predicate, timeout, interval) lives entirely in
// the slot map; the runtime semantics — polling until the predicate evaluates truthy or the timeout elapses —
// are the flow.Subgraph dispatch path's job, not the planner's.
//
// Parameters:
//   - `invocator`: the session host (unused — flow.WaitUntil has no body to resolve through the registry).
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for WaitUntil.
//   - `args`: positional arguments; unused — flow.WaitUntil is kwargs-driven today.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed); typically `target=`,
//     `predicate=`, `timeout=`, `interval=`. Each is stamped into the subgraph's slot map.
//   - `errorAction`: the failure-handler subgraph applied to the subgraph at construction, or nil.
//   - `retryPolicy`: the retry policy applied to the subgraph at construction, or nil.
//
// Returns:
//   - op.ExecutableUnit: the constructed wait-until-shaped [*op.Subgraph].
//   - `error`: non-nil when `receiverType` or `method` is nil.
func (WaitUntilPlanner) Plan(
	_ op.PlanInvocator,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	_ []any,
	kwargs map[string]any,
	annotations map[string]any,
	errorAction *op.Subgraph,
	retryPolicy *op.RetryPolicy,
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())
	action := op.NewAction(receiverType, method, actionName)

	slots := make(map[string]op.Binding, len(kwargs))
	for key, value := range kwargs {
		slots[key] = projectKwargValue(value)
	}

	spec := op.NewSubgraphSpec().
		WithID(op.GenerateNodeID(actionName)).
		WithAction(action).
		WithAnnotations(annotations).
		WithErrorAction(errorAction).
		WithRetryPolicy(retryPolicy)
	for name, value := range slots {
		spec.WithSlot(name, value)
	}

	subgraph, err := op.NewSubgraph(spec)
	if err != nil {
		return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: %w", err)
	}

	return subgraph, nil
}

// endregion

// endregion

// region HELPER FUNCTIONS

// projectKwargValue wraps a Go-side kwarg value into a [op.Binding] for storage in the subgraph's slot map.
//
// Variable references become VariableBinding; invocation handles become PromiseBinding pointing at the producer;
// everything else is wrapped as ImmediateBinding.
//
// Parameters:
//   - `value`: the converted Go value of the kwarg.
//
// Returns:
//   - op.Binding: the projected binding.
func projectKwargValue(value any) op.Binding {

	switch v := value.(type) {
	case *op.Invocation:
		return op.NewPromiseBinding(v.Target.ID())
	case *op.Variable:
		return op.NewVariableBinding(v.Name)
	default:
		return op.NewImmediateBinding(value)
	}
}
