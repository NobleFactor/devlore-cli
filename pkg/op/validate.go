// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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
//   - Plan-time type check: for every slot bound to a [PromiseBinding], walks producer → consumer in the
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
	violations = checkEdges(violations, g)
	violations = checkItemProjectionScope(violations, g)

	return errors.Join(violations...)
}

// checkItemProjectionScope flags item-field projections outside a gather body (phase-8 step 45).
//
// plan.item(field) references the reserved per-iteration variable `item`, which only a gather's dispatch frame binds
// ([flow] buildIterationFrame); a projection of `item` anywhere outside a gather body can never resolve, so it is a
// plan error, not a nil at dispatch. The walk descends the containment tree from the root, marking every subtree
// under a `flow.gather`-bound subgraph as in-scope.
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `g`: the graph to walk.
//
// Returns:
//   - `[]error`: the (possibly-extended) violation slice.
func checkItemProjectionScope(violations []error, g *Graph) []error {

	var walk func(units []ExecutableUnit, inGather bool)
	walk = func(units []ExecutableUnit, inGather bool) {
		for _, unit := range units {
			if !inGather {
				for slot, value := range unit.Slots() {
					binding, ok := value.(VariableBinding)
					if !ok || binding.Name() != "item" || binding.Field() == "" {
						continue
					}
					violations = append(violations, fmt.Errorf(
						"unit %q slot %q: plan.item(%q) outside a gather body — the iteration variable is only bound by a gather's per-iteration frame",
						unit.ID(), slot, binding.Field()))
				}
			}
			if subgraph, ok := unit.(*Subgraph); ok {
				walk(subgraph.Children(), inGather || boundActionName(subgraph) == "flow.gather")
			}
		}
	}
	walk(g.Root().Children(), boundActionName(g.Root()) == "flow.gather")

	return violations
}

// checkEdges validates every subgraph's edge set — endpoints, acyclicity, and the guarded-subgraph invariant.
//
// The plan-seal half of the two-boundary contract ([assembleGraph] runs the same [Subgraph.validateEdges] at load), so
// a malformed decision tree or an edge cycle is rejected before a graph is sealed, and again when a document is loaded
// (phase-8 step 10).
//
// Parameters:
//   - `violations`: the accumulating violation slice.
//   - `g`: the graph to walk.
//
// Returns:
//   - `[]error`: the (possibly-extended) violation slice.
func checkEdges(violations []error, g *Graph) []error {

	root := g.Root()

	if err := root.validateEdges(); err != nil {
		violations = append(violations, err)
	}
	for _, sg := range root.descendantSubgraphs() {
		if err := sg.validateEdges(); err != nil {
			violations = append(violations, err)
		}
	}

	return violations
}

// region HELPER FUNCTIONS

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
			// A by-name subgraph (the root names "flow.subgraph") has no resolved Action — and therefore no
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

// checkPromiseTypes is the plan-time type-check pass over the graph's [PromiseBinding] slot bindings.
//
// For every slot whose [Binding] is a [PromiseBinding], looks up the producing unit by
// [PromiseBinding.Edge], derives its declared result type via [Method.ResultType], and compares to the
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

			promise, ok := slotValue.(PromiseBinding)
			if !ok {
				continue
			}

			if violation := checkPromiseSlot(units, id, slotName, promise, consumerMethod); violation != nil {
				violations = append(violations, violation)
			}
		}
	}

	return violations
}

// checkPromiseSlot type-checks one promise-bound slot against its producer's declared result type.
//
// A producer absent from the unit table is itself a violation; missing type metadata on either side
// (an unbound action, an untyped parameter) passes — the required-params pass owns those complaints.
//
// Parameters:
//   - `units`: the graph's unit table.
//   - `id`: the consuming unit's ID.
//   - `slotName`: the slot under check.
//   - `promise`: the slot's promise binding.
//   - `consumerMethod`: the consuming unit's method.
//
// Returns:
//   - `error`: the violation, or nil.
func checkPromiseSlot(
	units map[string]ExecutableUnit, id, slotName string, promise PromiseBinding, consumerMethod *Method,
) error {

	edge := promise.Edge(id)
	producer, present := units[edge.From]
	if !present {
		return fmt.Errorf(
			"op.ValidateGraph: unit %q slot %q: producer %q not found in graph",
			id, slotName, edge.From)
	}

	producerAction := producer.Action()
	if producerAction == nil {
		return nil
	}
	producerMethod := producerAction.Method()
	if producerMethod == nil {
		return nil
	}

	sourceType := producerMethod.ResultType()
	if sourceType == nil {
		return nil
	}

	param, paramPresent := consumerMethod.ParameterByName(slotName)
	if !paramPresent {
		return nil
	}
	targetType := param.Type
	if targetType == nil {
		return nil
	}

	if typesAreInterconvertible(sourceType, targetType) {
		return nil
	}

	return fmt.Errorf(
		"op.ValidateGraph: unit %q slot %q: cannot bind %q output (%s) to declared type %s",
		id, slotName, edge.From, sourceType, targetType)
}

// indexUnitsByID flattens the graph's nodes and resolved-action subgraphs into a single ID → unit map for
// [PromiseBinding.Edge] lookups. Subgraphs with no resolved Action at validate time (the root, which binds
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

// boundActionName returns the short dotted action name a unit is bound to — from the bound [Action] when one is
// resolved, else the by-name binding — or "" for an unbound unit.
//
// Parameters:
//   - `unit`: the unit to name.
//
// Returns:
//   - `ActionName`: the short action name (e.g. "flow.gather"), or "".
func boundActionName(unit ExecutableUnit) ActionName {

	if action := unit.Action(); action != nil {
		return action.Name()
	}
	return unit.ActionName()
}
