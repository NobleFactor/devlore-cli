// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

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
// Materializes a [*op.Subgraph] bound to flow.Choose with `default_case` and the variadic `*cases`
// stamped into the subgraph's unified slot map via [planSubgraphFromParams]. The dispatch-time
// resolution of each Case's When and Then (Invocation / Promise / Lambda) is the responsibility of
// [Provider.Choose] + [resolveDispatchedValue], not the planner.
type ChoosePlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Choose.
//
// Walks the method's declared parameter list (`default_case`, `*cases`) and maps `args` / `kwargs` to the
// subgraph's slot map. Positional args fill named parameters in declaration order; the variadic `*cases`
// parameter collects every remaining positional arg into an [op.ImmediateValue] slice slot. Named-kwarg
// passes route through [projectKwargValue] for Variable / Invocation / Promise projection.
//
// Parameters:
//   - `invocator`: the session host (unused — Choose has no body= surface).
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Choose.
//   - `args`: positional arguments converted starlark → Go. `args[0]` is `default_case`; the remainder
//     are the variadic cases (each typically a `*flow.Case` from a `plan.case(when=, then=)` call).
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed). Callers using
//     `plan.choose(default_case=..., ...)` would land entries here.
//
// Returns:
//   - op.ExecutableUnit: the constructed choose-shaped [*op.Subgraph].
//   - `error`: non-nil when `receiverType` or `method` is nil, or a required parameter is missing.
func (ChoosePlanner) Plan(
	_ op.PlanInvocator,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	args []any,
	kwargs map[string]any,
) (op.ExecutableUnit, error) {

	return planSubgraphFromParams("flow.ChoosePlanner.Plan", receiverType, method, args, kwargs)
}

// endregion

// endregion

// GatherPlanner is the specialized [op.Planner] for flow.Provider.Gather.
//
// Materializes a [*op.Subgraph] that iterates a body unit once per item with bounded concurrency.
type GatherPlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Gather.
//
// Parameters:
//   - `invocator`: gives access to the session invocation registry for resolving `do` to its body unit.
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Gather.
//   - `args`: positional arguments converted starlark → Go.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed).
//
// Returns:
//   - op.ExecutableUnit: the constructed gather-shaped [*op.Subgraph].
//   - `error`: non-nil if `items` is missing, `do` cannot be resolved, or `limit` is invalid.
func (GatherPlanner) Plan(
	_ op.PlanInvocator,
	_ op.ProviderReceiverType,
	_ *op.Method,
	_ []any,
	_ map[string]any,
) (op.ExecutableUnit, error) {

	return nil, fmt.Errorf("flow.GatherPlanner.Plan: not implemented")
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
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.SubgraphPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())

	subgraph := op.NewSubgraph(op.GenerateNodeID(actionName), op.NewAction(receiverType, method, actionName))

	if body, present := kwargs["body"]; present {
		if err := addBodyChildren(subgraph, body); err != nil {
			return nil, err
		}
	}

	// Every kwarg except `body=` lands in the unified slot map. The dispatch-time discriminator
	// (combinator input vs frame binding) is method-signature-driven.
	for key, value := range kwargs {
		if _, reserved := reservedSubgraphKwargs[key]; reserved {
			continue
		}
		subgraph.SetSlot(key, projectKwargValue(value))
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
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())

	subgraph := op.NewSubgraph(op.GenerateNodeID(actionName), op.NewAction(receiverType, method, actionName))

	for key, value := range kwargs {
		subgraph.SetSlot(key, projectKwargValue(value))
	}

	return subgraph, nil
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// addBodyChildren expects body to be a slice of *op.Invocation and AddChilds each invocation's Target to
// the supplied subgraph. AddChild stamps the child's parentID to the subgraph's ID.
//
// Parameters:
//   - `subgraph`: the subgraph being assembled.
//   - `body`: the body= kwarg value; must be a []any of *op.Invocation.
//
// Returns:
//   - `error`: non-nil if body is not a list or contains a non-invocation element.
func addBodyChildren(subgraph *op.Subgraph, body any) error {

	list, ok := body.([]any)
	if !ok {
		return fmt.Errorf("flow.SubgraphPlanner.Plan: body= must be a list, got %T", body)
	}

	for i, elem := range list {
		inv, ok := elem.(*op.Invocation)
		if !ok {
			return fmt.Errorf("flow.SubgraphPlanner.Plan: body[%d]: expected *op.Invocation, got %T", i, elem)
		}
		subgraph.AddChild(inv.Target)
	}

	return nil
}

// planSubgraphFromParams maps `args` + `kwargs` to a fresh [*op.Subgraph]'s slot map via `method.Parameters()`.
//
// Uses the same precedence / variadic / kwargs-sink rules as [op.ActionPlanner.Plan]: positional args fill
// named parameters in declaration order; the variadic `*<name>` parameter collects every remaining
// positional arg into a `[]any` wrapped as [op.ImmediateValue]; the `**<name>` sink (if declared) collects
// every unconsumed kwarg into a `map[string]any` wrapped as [op.ImmediateValue]; Variable / Invocation /
// Promise values route through [projectKwargValue] before stamping. Missing required parameters return an
// error.
//
// Used by container planners (ChoosePlanner / GatherPlanner) that need positional-args support.
// SubgraphPlanner uses its own body=-aware variant; WaitUntilPlanner uses a simpler kwargs-only iteration.
//
// Parameters:
//   - `prefix`: error-message prefix identifying the calling planner (e.g., `"flow.ChoosePlanner.Plan"`).
//   - `receiverType`: the method's receiver type.
//   - `method`: the method descriptor.
//   - `args`: positional args from the starlark call (already converted to Go).
//   - `kwargs`: named kwargs from the starlark call (already converted to Go, reserved entries removed).
//
// Returns:
//   - op.ExecutableUnit: the constructed [*op.Subgraph].
//   - `error`: non-nil when `receiverType` / `method` is nil, or a required parameter is missing.
func planSubgraphFromParams(
	prefix string,
	receiverType op.ProviderReceiverType,
	method *op.Method,
	args []any,
	kwargs map[string]any,
) (op.ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("%s: nil receiverType", prefix)
	}
	if method == nil {
		return nil, fmt.Errorf("%s: nil method", prefix)
	}

	actionName := receiverType.Name() + "." + op.CamelToSnake(method.Name())

	subgraph := op.NewSubgraph(op.GenerateNodeID(actionName), op.NewAction(receiverType, method, actionName))

	params := method.Parameters()
	consumed := make(map[string]bool, len(kwargs))
	positional := 0

	for _, param := range params {

		if param.Variadic {
			rest := make([]any, 0, max(0, len(args)-positional))
			for ; positional < len(args); positional++ {
				rest = append(rest, args[positional])
			}
			subgraph.SetSlot(param.Name, op.ImmediateValue{Value: rest})
			continue
		}

		if param.Kwargs {
			remaining := make(map[string]any, len(kwargs))
			for k, v := range kwargs {
				if !consumed[k] {
					remaining[k] = v
				}
			}
			subgraph.SetSlot(param.Name, op.ImmediateValue{Value: remaining})
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
				subgraph.SetSlot(param.Name, op.ImmediateValue{Value: param.Default})
				continue
			}
			if !param.Optional {
				return nil, fmt.Errorf("%s: %s: missing required parameter %q", prefix, actionName, param.Name)
			}
			continue
		}

		subgraph.SetSlot(param.Name, projectKwargValue(value))
	}

	return subgraph, nil
}

// projectKwargValue wraps a Go-side kwarg value into a [op.SlotValue] for storage in the subgraph's slot map.
//
// Variable references become VariableValue; invocation handles become PromiseValue pointing at the producer;
// everything else is wrapped as ImmediateValue.
//
// Parameters:
//   - `value`: the converted Go value of the kwarg.
//
// Returns:
//   - op.SlotValue: the wrapped slot value.
func projectKwargValue(value any) op.SlotValue {

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

// endregion
