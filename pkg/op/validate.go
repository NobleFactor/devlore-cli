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

	if g == nil || g.Root == nil {
		return nil
	}

	var violations []error

	violations = checkRequiredParams(violations, g)
	violations = checkBubbleUpConsistency(violations, g)

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
			// Unbound subgraphs (containers, the root) have no method to validate against — their
			// role is structural (parent / scope), not dispatch-bearing.
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

// endregion

// endregion
