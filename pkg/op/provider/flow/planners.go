// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// reservedSubgraphKwargs are the keys SubgraphPlanner.Plan classifies specially: body= populates the
// subgraph's children; items= populates its iteration domain. Every other kwarg lands in FrameBindings.
var reservedSubgraphKwargs = map[string]struct{}{
	"body":  {},
	"items": {},
}

// ChoosePlanner is the specialized [op.Planner] for flow.Provider.Choose.
//
// Materializes a [*op.Subgraph] whose children encode the case-fold semantics of `plan.choose`. The
// variadic `*cases` parameter is unpacked into the subgraph as case children; `default_case` is stamped
// as the subgraph's default branch.
type ChoosePlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Plan implements [op.Planner] for flow.Provider.Choose.
//
// Parameters:
//   - `invocator`: the session host.
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for Choose.
//   - `args`: positional arguments; `args[0]` is the default case, the remainder are the variadic cases.
//   - `kwargs`: keyword arguments (reserved entries removed); Choose takes none today.
//
// Returns:
//   - op.ExecutableUnit: the constructed choose-shaped [*op.Subgraph].
//   - `error`: non-nil if the cases list is malformed or a branch fails to project.
func (ChoosePlanner) Plan(
	_ op.PlanInvocator,
	_ op.ProviderReceiverType,
	_ *op.Method,
	_ []any,
	_ map[string]any,
) (op.ExecutableUnit, error) {

	return nil, fmt.Errorf("flow.ChoosePlanner.Plan: not implemented")
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
// Classifies the call's kwargs into three partitions: body= children (added via [op.Subgraph.AddChild],
// which stamps each child's parent ID), items= iteration domain (stamped on the subgraph's Items slot),
// and frame-binding kwargs (everything else, stamped into FrameBindings).
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
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed); contains `body=`,
//     `items=`, and any frame-binding entries.
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

	subgraph := op.NewSubgraph(op.GenerateNodeID(actionName))

	if body, present := kwargs["body"]; present {
		if err := addBodyChildren(subgraph, body); err != nil {
			return nil, err
		}
	}

	if items, present := kwargs["items"]; present {
		subgraph.Items = projectKwargValue(items)
	}

	for key, value := range kwargs {
		if _, reserved := reservedSubgraphKwargs[key]; reserved {
			continue
		}
		if subgraph.FrameBindings == nil {
			subgraph.FrameBindings = make(map[string]op.SlotValue, len(kwargs))
		}
		subgraph.FrameBindings[key] = projectKwargValue(value)
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
// Parameters:
//   - `invocator`: unused.
//   - `receiverType`: the flow planning provider.
//   - `method`: the registered descriptor for WaitUntil.
//   - `args`: positional arguments converted starlark → Go.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries removed).
//
// Returns:
//   - op.ExecutableUnit: the constructed wait-until-shaped [*op.Subgraph].
//   - `error`: non-nil if the predicate cannot be resolved or cadence parameters are invalid.
func (WaitUntilPlanner) Plan(
	_ op.PlanInvocator,
	_ op.ProviderReceiverType,
	_ *op.Method,
	_ []any,
	_ map[string]any,
) (op.ExecutableUnit, error) {

	return nil, fmt.Errorf("flow.WaitUntilPlanner.Plan: not implemented")
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

// projectKwargValue wraps a Go-side kwarg value into a [op.SlotValue] for storage in Items or
// FrameBindings. Variable references become VariableValue; invocation handles become PromiseValue
// pointing at the producer; everything else is wrapped as ImmediateValue.
//
// Parameters:
//   - `value`: the converted Go value of the kwarg.
//
// Returns:
//   - op.SlotValue: the wrapped slot value.
func projectKwargValue(value any) op.SlotValue {

	switch v := value.(type) {
	case *op.Invocation:
		return op.PromiseValue{NodeRef: v.Target.ID(), Slot: ""}
	case *op.Promise:
		return op.PromiseValue{NodeRef: v.Node().ID(), Slot: v.Slot()}
	case *op.Variable:
		return op.VariableValue{Name: v.Name}
	default:
		return op.ImmediateValue{Value: value}
	}
}

// endregion
