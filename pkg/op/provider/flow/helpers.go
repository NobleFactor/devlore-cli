// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// resolveBodyChildren extracts each invocation's Target from `body` and returns the resulting
// [op.ExecutableUnit] slice for a flow subgraph's children.
//
// Used by the gather/subgraph/choose planners to gather the body= kwarg into the children list that
// gets handed to [op.NewSubgraph]. Returns nil children when `body` is empty (caller passes nil to
// NewSubgraph). A singleton body — a bare `*op.Invocation` — is accepted as a one-element list
// (settled 2026-07-02), so `when=plan.file.is_dir(...)` and `when=[plan.file.is_dir(...)]` are the
// same body.
//
// Parameters:
//   - `body`: the body= kwarg value; a `[]any` of `*op.Invocation`, or a bare `*op.Invocation`.
//
// Returns:
//   - `[]op.ExecutableUnit`: the resolved children, in declaration order. Nil when `body` is empty.
//   - `error`: non-nil if `body` is not a list or invocation, or contains a non-invocation element.
func resolveBodyChildren(body any) ([]op.ExecutableUnit, error) {

	if invocation, ok := body.(*op.Invocation); ok {
		body = []any{invocation}
	}

	list, ok := body.([]any)
	if !ok {
		return nil, fmt.Errorf("flow planner: body= must be a list or a single invocation, got %T", body)
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

// gatherIterationID derives the stamped resumption identity for gather iteration `index` from the gather unit's ID.
//
// A gather runs one body-subgraph N times; each run's stamped substack is keyed by this id, so resume can skip a
// completed iteration and adopt a paused one via [op.RecoveryStack.NestedStackByUnitID].
//
// Parameters:
//   - `unit`: the gather's bound [op.ExecutableUnit] (its `*op.Subgraph`); supplies the base id.
//   - `index`: the zero-based iteration index.
//
// Returns:
//   - `string`: the per-iteration identity, `"<gatherID>#<index>"`.
func gatherIterationID(unit op.ExecutableUnit, index int) string {
	return fmt.Sprintf("%s#%d", unit.ID(), index)
}

// completeActionName is the action name flow.Complete registers under; the walk stops when a child carries it
// (Complete's early-return semantics).
const completeActionName = "flow.complete"

// walkSubgraphChildren dispatches `subgraph`'s children in declaration order on the supplied `frame`, with per-child
// retry.
//
// The single children-walk shared by [Provider.Subgraph], [Provider.Gather], and [Provider.Choose]. A subgraph
// carrying guarded edges is executed as a decision tree instead ([walkDecisionTree]) — the choose topology runs one
// root-to-leaf path. Otherwise each child runs via [op.ActivationRecord.DispatchChild] — so a child carrying an
// [op.RetryPolicy] retries uniformly regardless of which caller drove the walk, and its OnError / OnRetry handlers are
// consumed at that dispatch by the executor, invisible to this walk. On the first child whose failure stands, the walk
// short-circuits, returning the child's error. Children's compensations accumulate on the supplied `stack`.
//
// Parameters:
//   - `activation`: the dispatch record; supplies the child-dispatch closure into the executor walk.
//   - `ctx`: the cancellation context for this walk ([Provider.Subgraph] passes `activation.Context`; [Provider.Gather]
//     passes its per-iteration scoped context).
//   - `subgraph`: the bound subgraph whose children form the walked body.
//   - `stack`: the recovery stack the children's compensations push onto.
//   - `frame`: the variable frame each child dispatches under ([Provider.Subgraph] passes `activation.Variables`;
//     [Provider.Gather] passes its per-iteration frame).
//
// Returns:
//   - `any`: the last child's terminal result, or nil for zero-child bodies / on failure.
//   - `error`: non-nil on cancellation or any child's standing failure (wrapped with the child's ID).
func walkSubgraphChildren(
	activation *op.ActivationRecord,
	ctx context.Context,
	subgraph *op.Subgraph,
	stack *op.RecoveryStack,
	frame map[string]op.Variable,
) (any, error) {

	if hasConditionalEdges(subgraph) {
		return walkDecisionTree(activation, ctx, subgraph, stack, frame)
	}

	var last any

	for _, child := range subgraph.Children() {

		result, err := activation.DispatchChild(ctx, child, stack, frame)
		if err != nil {
			return nil, fmt.Errorf("child %q: %w", child.ID(), err)
		}

		last = result

		// flow.Complete is an early return from this body — like a return statement in a func: stop dispatching the
		// remaining children (they get no receipts) and yield Complete's input as the body's result. Everything
		// already done is kept; nothing unwinds — it is a success return.
		if child.ActionName() == completeActionName {
			return last, nil
		}
	}

	return last, nil
}

// walkDecisionTree executes `subgraph` as a guarded decision tree, running exactly one root-to-leaf path.
//
// The choose walk (phase-8 step 10): dispatch the root decision node, resolve its outcome from the result's truthiness
// ([branch]), follow the matching edge, repeat until a leaf; the leaf's result is the choose result. Branches not taken
// never run — the first-truthy short-circuit is the topology itself.
// A node's OnError / OnRetry handlers are consumed at its dispatch by the executor, invisible to this walk.
//
// Parameters:
//   - `activation`: the dispatch record; supplies the child-dispatch closure into the executor walk.
//   - `ctx`: the cancellation context for this walk.
//   - `subgraph`: the bound choose subgraph whose guarded edges form the tree.
//   - `stack`: the recovery stack the path's receipts land on.
//   - `frame`: the variable frame each node dispatches under.
//
// Returns:
//   - `any`: the executed leaf's result.
//   - `error`: non-nil on cancellation, a node failure, or a malformed topology.
func walkDecisionTree(
	activation *op.ActivationRecord,
	ctx context.Context,
	subgraph *op.Subgraph,
	stack *op.RecoveryStack,
	frame map[string]op.Variable,
) (any, error) {

	current, err := root(subgraph)
	if err != nil {
		return nil, err
	}

	var result any

	for current != nil {

		nodeResult, dispatchErr := activation.DispatchChild(ctx, current, stack, frame)
		if dispatchErr != nil {
			return nil, fmt.Errorf("choose node %q: %w", current.ID(), dispatchErr)
		}

		result = nodeResult

		next, branchErr := branch(subgraph, current.ID(), nodeResult)
		if branchErr != nil {
			return nil, branchErr
		}
		current = next
	}

	return result, nil
}

// root returns the decision tree's entry node — the one child no guarded edge targets.
//
// Parameters:
//   - `subgraph`: the choose subgraph; must carry guarded edges.
//
// Returns:
//   - `op.ExecutableUnit`: the root decision node.
//   - `error`: non-nil when the tree has no single root (a malformed topology that escaped validation).
func root(subgraph *op.Subgraph) (op.ExecutableUnit, error) {

	targeted := make(map[string]bool)
	for _, edge := range subgraph.Edges() {
		if edge.Guard != op.GuardNone {
			targeted[edge.To] = true
		}
	}

	var found op.ExecutableUnit
	count := 0
	for _, child := range subgraph.Children() {
		if !targeted[child.ID()] {
			found = child
			count++
		}
	}

	if count != 1 {
		return nil, fmt.Errorf("flow: choose subgraph %q: %d decision roots, want exactly 1", subgraph.ID(), count)
	}

	return found, nil
}

// branch resolves the decision node `fromID`'s next hop from its result's truthiness.
//
// A node with no outgoing guarded edges is a leaf (nil, nil). Otherwise the outcome is [op.GuardTruthy] or
// [op.GuardFalsy] by [op.IsTruthy] on `result` — the node's live result on the first run, its round-tripped result on a
// resume; re-deriving truthiness is trivial, so the outcome is never stored. Exactly one out-edge may match the outcome;
// more is a malformed topology (defense in depth behind op's guarded-edge validation).
//
// Parameters:
//   - `subgraph`: the choose subgraph whose guarded edges route the walk.
//   - `fromID`: the just-dispatched decision node's ID.
//   - `result`: the node's (live or replayed) result whose truthiness picks the branch.
//
// Returns:
//   - `op.ExecutableUnit`: the next node on the path, or nil when `fromID` is a leaf.
//   - `error`: non-nil when the matching edge count is not exactly 1 or its target is no direct child.
func branch(
	subgraph *op.Subgraph,
	fromID string,
	result any,
) (op.ExecutableUnit, error) {

	outgoing := make([]op.Edge, 0, 2)
	for _, edge := range subgraph.Edges() {
		if edge.From == fromID && edge.Guard != op.GuardNone {
			outgoing = append(outgoing, edge)
		}
	}
	if len(outgoing) == 0 {
		return nil, nil // a leaf — the walk ends here
	}

	guard := op.GuardFalsy
	if op.IsTruthy(result) {
		guard = op.GuardTruthy
	}

	var target op.ExecutableUnit
	matches := 0
	for _, edge := range outgoing {
		if edge.Guard == guard {
			matches++
			target = subgraph.ChildByID(edge.To)
		}
	}

	if matches != 1 {
		return nil, fmt.Errorf("flow: choose node %q: %d out-edges match guard %q, want exactly 1",
			fromID, matches, guard)
	}
	if target == nil {
		return nil, fmt.Errorf("flow: choose node %q: guard %q edge targets no direct child", fromID, guard)
	}

	return target, nil
}

// hasConditionalEdges reports whether `subgraph` carries any guarded edge — the discriminator that routes
// [walkSubgraphChildren] to the decision-tree walk.
//
// [op.ValidateGraph] enforces the guarded-subgraph invariant at both boundaries (plan-seal and load), so a guarded
// subgraph reaching this check is a well-formed decision tree.
//
// Parameters:
//   - `subgraph`: the bound subgraph whose edges to inspect.
//
// Returns:
//   - `bool`: true when at least one edge carries a non-[op.GuardNone] guard.
func hasConditionalEdges(subgraph *op.Subgraph) bool {

	for _, edge := range subgraph.Edges() {
		if edge.Guard != op.GuardNone {
			return true
		}
	}

	return false
}

// bodySubgraph seals `body` into a flow.subgraph-bound [*op.Subgraph] — the construction plan.subgraph(body=[...])
// uses.
//
// Shared by [NewCase] (the when- and then-subgraphs) and [ChoosePlanner] (the default subgraph): the body's
// invocations resolve to children via [resolveBodyChildren] and seal under a by-name flow.subgraph binding, resolved
// lazily at dispatch exactly like the graph root.
//
// Parameters:
//   - `role`: the error-message label for the body's position ("when", "then", "default").
//   - `body`: the body value; must be a `[]any` of `*op.Invocation`.
//
// Returns:
//   - `*op.Subgraph`: the sealed subgraph.
//   - `error`: non-nil when `body` is malformed or the subgraph cannot seal.
func bodySubgraph(role string, body any) (*op.Subgraph, error) {

	children, err := resolveBodyChildren(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", role, err)
	}

	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().
		WithID(op.GenerateNodeID("flow.subgraph")).
		WithActionNamed("flow.subgraph").
		WithChildren(children...))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", role, err)
	}

	return subgraph, nil
}

// endregion
