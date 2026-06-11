// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
)

// ValidateGraph asserts the assembled graph satisfies the plan-time invariants every executable unit
// must hold before execution.
//
// Checks performed:
//
//   - Required-parameter coverage: for each [*Node], and each [*Subgraph] whose [Action] is non-nil,
//     every required parameter of the bound [Method] has a slot entry. Optional, Variadic, and Kwargs
//     parameters are exempt — Optional may be supplied or omitted; Variadic and Kwargs absorb whatever
//     is or is not supplied.
//   - Bubble-up consistency: triggers [Graph.Parameters] to drive [Subgraph.mergeBubbled] across every
//     level. Any same-named variable declared with incompatible types across child slots surfaces here
//     as one or more violations joined into the returned error.
//   - Plan-time type check: for every slot bound to a [PromiseValue], walks producer → consumer in the
//     graph, looks up the producer's declared output type ([Method.ResultType]) and the consumer's
//     slot type ([Method.ParameterByName].Type), then consults [typesAreInterconvertible] to decide
//     whether [Convert] would succeed at dispatch. Mismatches surface here as plan-time errors so
//     ill-typed promise bindings never reach execution.
//
// ValidateGraph is the single source of truth for both boundary checks:
//
//   - The planning path calls it as the final step of plan.Provider.Assemble.
//   - The wire-form load path calls it after [Graph.Rebind]'s linkActions resolves pending action
//     references through the registry. The loader (e.g., plan.Provider.Load) orders Unmarshal ->
//     Rebind -> ValidateGraph.
//
// Action-binding is a prerequisite. A loaded graph in its post-Unmarshal, pre-Rebind state carries
// unresolved action references in `pendingAction` and has no methods to validate against; calling
// ValidateGraph in that state reports every unit as having a nil action. Callers must Rebind first.
//
// Parameters:
//   - `g`: the graph to validate. A nil graph or a graph with a nil Root is treated as empty (no
//     error).
//
// Returns:
//   - `error`: an [errors.Join] of all violations found, or nil when the graph is valid. Each joined
//     entry is a single human-readable string identifying the unit and the violation; callers that
//     want structured handling can Unwrap the join.
func ValidateGraph(g *Graph) error {

	if g == nil || g.Root() == nil {
		return nil
	}

	var violations []error

	violations = checkRequiredParams(violations, g)
	violations = checkBubbleUpConsistency(violations, g)
	violations = checkPromiseTypes(violations, g)

	return errors.Join(violations...)
}

// region UNEXPORTED FUNCTIONS

// region Behaviors

// checkRequiredParams walks every node and every action-bound subgraph in g, asserting that each
// required parameter of the bound method has a slot entry. Violations are appended as standalone
// errors; the function returns the (possibly-extended) violation slice.
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `g`: the graph to walk.
//
// Returns:
//   - []error: the (possibly-extended) violation slice.
func checkRequiredParams(violations []error, g *Graph) []error {

	for _, node := range g.Nodes() {
		violations = checkUnitRequiredParams(violations, node, "node")
	}

	for _, sg := range g.Subgraphs() {
		if sg.Action() == nil {
			// A by-name subgraph (the fsroot names "flow.subgraph") has no resolved Action — and therefore no
			// method — at validate time; it resolves lazily at dispatch, so there is nothing to check here.
			continue
		}
		violations = checkUnitRequiredParams(violations, sg, "subgraph")
	}

	return violations
}

// checkUnitRequiredParams asserts that every required parameter of unit's bound method has a slot
// entry on unit.
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `unit`: the executable unit to check.
//   - `kind`: a label used in error messages — "node" or "subgraph".
//
// Returns:
//   - []error: the (possibly-extended) violation slice.
func checkUnitRequiredParams(violations []error, unit ExecutableUnit, kind string) []error {

	action := unit.Action()
	if action == nil {
		return append(violations, fmt.Errorf(
			"op.ValidateGraph: %s %q: no action bound", kind, unit.ID()))
	}

	method := action.Method()
	if method == nil {
		return append(violations, fmt.Errorf(
			"op.ValidateGraph: %s %q (action %q): action carries no method",
			kind, unit.ID(), action.Name()))
	}

	slots := unit.Slots()

	for _, param := range method.Parameters() {

		if param.Optional || param.Variadic || param.Kwargs {
			continue
		}

		if _, ok := slots[param.Name]; !ok {
			violations = append(violations, fmt.Errorf(
				"op.ValidateGraph: %s %q (action %q): required parameter %q not bound",
				kind, unit.ID(), action.Name(), param.Name))
		}
	}

	return violations
}

// checkBubbleUpConsistency triggers [Graph.Parameters] to force [Subgraph.mergeBubbled] across the
// entire graph. The returned error, when non-nil, is an [errors.Join] of every collision detected.
// Unwrapping the join (when supported by the underlying type) splices each collision into the outer
// violation list so they surface as top-level entries; otherwise the error is appended as-is.
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `g`: the graph to walk.
//
// Returns:
//   - []error: the (possibly-extended) violation slice.
func checkBubbleUpConsistency(violations []error, g *Graph) []error {

	_, err := g.Parameters()
	if err == nil {
		return violations
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return append(violations, joined.Unwrap()...)
	}

	return append(violations, err)
}

// checkPromiseTypes is the plan-time type-check pass over the graph's [PromiseValue] slot bindings.
//
// For every slot whose [SlotValue] is a [PromiseValue], looks up the producing unit by
// [PromiseValue.UnitRef], derives its declared result type via [Method.ResultType], and compares to the
// consumer slot's [Parameter.Type] (looked up via [Method.ParameterByName] for the consumer's bound
// method). The comparison runs through [typesAreInterconvertible] — the same convertibility relation
// [Convert] consults at slot-fill time — so plan-time and dispatch-time agree on the contract.
//
// Each mismatch is appended to `violations`; orphan and bubble-up errors aggregate alongside in the same
// envelope ValidateGraph's caller receives. Slots whose producer / consumer / parameter cannot be
// resolved (missing method, missing parameter, nil types) skip silently — the required-params pass
// and the bubble-up pass already catch the structural issues that would cause those lookups to fail.
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `g`: the graph to walk.
//
// Returns:
//   - []error: the (possibly-extended) violation slice.
func checkPromiseTypes(violations []error, g *Graph) []error {

	units := indexUnitsByID(g)

	for id, unit := range units {

		action := unit.Action()
		if action == nil {
			continue
		}
		consumerMethod := action.Method()
		if consumerMethod == nil {
			continue
		}

		for slotName, slotValue := range unit.Slots() {

			promise, ok := slotValue.(PromiseValue)
			if !ok {
				continue
			}

			producer, present := units[promise.UnitRef]
			if !present {
				violations = append(violations, fmt.Errorf(
					"op.ValidateGraph: unit %q slot %q: producer %q not found in graph",
					id, slotName, promise.UnitRef))
				continue
			}

			producerAction := producer.Action()
			if producerAction == nil {
				continue
			}
			producerMethod := producerAction.Method()
			if producerMethod == nil {
				continue
			}

			sourceType := producerMethod.ResultType()
			if sourceType == nil {
				continue
			}

			param, paramPresent := consumerMethod.ParameterByName(slotName)
			if !paramPresent {
				continue
			}
			targetType := param.Type
			if targetType == nil {
				continue
			}

			if typesAreInterconvertible(sourceType, targetType) {
				continue
			}

			violations = append(violations, fmt.Errorf(
				"op.ValidateGraph: unit %q slot %q: cannot bind %q output (%s) to declared type %s",
				id, slotName, promise.UnitRef, sourceType, targetType))
		}
	}

	return violations
}

// indexUnitsByID flattens the graph's nodes and resolved-action subgraphs into a single ID → unit map for
// [PromiseValue.UnitRef] lookups. Subgraphs with no resolved Action at validate time (the fsroot, which binds
// "flow.subgraph" by name and resolves it lazily at dispatch) are excluded — Promise references never target them.
//
// Parameters:
//   - `g`: the graph to walk.
//
// Returns:
//   - map[string]ExecutableUnit: every node by ID plus every action-bound subgraph by ID.
func indexUnitsByID(g *Graph) map[string]ExecutableUnit {

	units := make(map[string]ExecutableUnit)

	for _, node := range g.Nodes() {
		units[node.ID()] = node
	}

	for _, subgraph := range g.Subgraphs() {
		if subgraph.Action() == nil {
			continue
		}
		units[subgraph.ID()] = subgraph
	}

	return units
}

// endregion

// endregion
