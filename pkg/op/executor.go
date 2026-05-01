// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

var (
	ErrNilGraph = errors.New("expected non-nil Graph")
)

// GraphExecutor executes action graphs.
type GraphExecutor struct {
	hooks *HookRegistry
	spec  *RuntimeEnvironmentSpec
}

// NewGraphExecutor creates an executor with the given environment spec.
//
// Parameters:
//   - spec: the execution environment configuration.
//
// Returns:
//   - *GraphExecutor: the configured executor.
func NewGraphExecutor(spec *RuntimeEnvironmentSpec) *GraphExecutor {

	assert.NotNil("spec", spec)

	return &GraphExecutor{
		spec: spec,
	}
}

// newContext creates an execution ExecutionContext from the executor's spec.
//
// Returns:
//   - *ExecutionContext: the execution context.
func (e *GraphExecutor) newContext() *ExecutionContext {
	return e.spec.NewExecutionContext()
}

// SetHooks sets the lifecycle hook registry for this executor.
//
// Parameters:
//   - hooks: the hook registry to install.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
}

// Run executes all nodes in the graph, respecting ordering constraints.
//
// The graph root is treated as an implicit subgraph. The executor calls executeChildren on the root's children,
// applying Kahn's algorithm at each level and recursing into child subgraphs.
//
// Parameters:
//   - graph: the execution graph to run.
//
// Returns:
//   - any: the terminal node's output value, or nil if no node produced output.
//   - error: non-nil if any node or subgraph fails.
func (e *GraphExecutor) Run(graph *Graph) (any, error) {

	if graph.State != StatePending {
		return nil, fmt.Errorf("graph already executed (state: %s)", graph.State)
	}

	ctx := e.newContext()

	var err error
	defer iox.Close(&err, ctx.Root)
	graph.Rebind(ctx)

	// Pre-flight resolution pass. Iterate the catalog's discovery entries and
	// fail fast if any source resource does not exist on the target machine.
	// Shadowed entries (outputs of nodes in this graph) are skipped — their
	// producer will create them at execution time.
	//
	// This is the link-time symbol resolution pass. See the resource management
	// architecture doc §6.5.
	//
	// Skipped in dry-run mode: dry-run validates graph structure without
	// asserting target-machine state.
	if !ctx.DryRun {
		if err = ResolveResources(graph.Catalog); err != nil {
			graph.State = StateFailed
			return nil, err
		}
	}

	ctx.Results = make(map[string]any)
	stack := NewRecoveryStack()

	result, err := graph.dispatch(ctx.Context, e, stack, graph.Root, ctx.Results, nil)

	summary := graph.Summary()

	if err != nil {
		// Unwind the recovery stack in LIFO order so every action that
		// successfully completed before the failure gets its Compensate
		// companion called. The stack was populated on each successful
		// executeNode. Without this, TestCompensation-style rollback
		// never runs.
		if unwindErr := stack.Unwind(); unwindErr != nil {
			err = fmt.Errorf("%w; compensation: %w", err, unwindErr)
		}
		graph.State = StateFailed
		return nil, err
	}

	if summary.Failed() > 0 {
		graph.State = StateFailed
	} else {
		graph.State = StateExecuted
	}

	return result, nil
}

// executeChildren walks a sorted children list, dispatching each child through [Graph.dispatch].
//
// Topological roots — children with no incoming edges at this level — receive overrides. Non-root children consume
// their inputs via promises resolved from the results map, so overrides bypass them. Each child dispatches through
// graph.dispatch, reusing the caller's executor, recovery stack, and cancellation context so compensation unwinding
// and cancel propagation see the entire chain.
//
// Parameters:
//   - ctx: the cancellation context threaded from the caller.
//   - graph: the root graph (for dispatch access).
//   - children: the children to execute (declaration order).
//   - edges: ordering constraints between children at this level.
//   - results: the accumulated node results for promise resolution.
//   - stack: the recovery stack for compensation.
//   - overrides: caller-supplied slot overrides, routed to topological roots only.
//
// Returns:
//   - any: the last child's output value, or nil if no child produced output.
//   - error: non-nil if any child fails.
func (e *GraphExecutor) executeChildren(ctx context.Context, graph *Graph, children []SubgraphChild, edges []Edge, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	sorted := SortChildren(children, edges)

	hasIncoming := make(map[string]bool, len(edges))

	for _, edge := range edges {
		hasIncoming[edge.To] = true
	}

	var lastResult any

	for _, child := range sorted {

		var childOverrides map[string]SlotValue
		if !hasIncoming[child.ChildID()] {
			childOverrides = overrides
		}

		var unit ExecutableUnit

		switch {
		case child.Node != nil:
			unit = child.Node
		case child.Subgraph != nil:
			unit = child.Subgraph
		default:
			continue
		}

		childResult, err := graph.dispatch(ctx, e, stack, unit, results, childOverrides)

		if err != nil {
			return nil, err
		}

		if childResult != nil {
			lastResult = childResult
		}
	}

	return lastResult, nil
}

// executeSubgraph runs a subgraph with retry logic, recursively executing its children.
//
// The subgraph does not derive its own cancellation scope — it propagates the caller's ctx down to its children.
// External cancel (root) and ancestor-gather cancel both reach the children via ctx inheritance; executeNode's
// entry-time ctx.Err() check picks them up at the next node boundary.
//
// Parameters:
//   - ctx: the cancellation context threaded from the caller.
//   - graph: the root graph (for dispatch access and compensation lookup).
//   - sg: the subgraph to execute.
//   - results: the accumulated node results for promise resolution.
//   - stack: the recovery stack for compensation.
//   - overrides: caller-supplied slot overrides, routed to topological roots within the subgraph.
//
// Returns:
//   - any: the last child's output value within the subgraph, or nil.
//   - error: non-nil if the subgraph fails after all retry attempts.
func (e *GraphExecutor) executeSubgraph(ctx context.Context, graph *Graph, sg *Subgraph, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	maxAttempts := 1

	if sg.Retry != nil {
		maxAttempts += sg.Retry.MaxAttempts
	}

	ec := graph.ExecutionContext()

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {

		// Apply backoff delay before retries (not before first attempt)
		if attempt > 0 && sg.Retry != nil {
			delay := sg.Retry.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}
		}

		// Reset inner node statuses for retry
		if attempt > 0 {
			resetSubgraphNodes(sg)
		}

		e.hooks.FireSubgraphStart(ec, sg.ID())

		childResult, innerErr := e.executeChildren(ctx, graph, sg.Children, sg.Edges, results, stack, overrides)

		e.hooks.FireSubgraphComplete(ec, sg.ID(), innerErr)

		attemptRecord := Attempt{
			Number:    attempt + 1,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if innerErr == nil {
			attemptRecord.Status = "completed"
			sg.Attempts = append(sg.Attempts, attemptRecord)
			sg.Status = SubgraphCompleted
			return childResult, nil
		}

		attemptRecord.Status = "failed"
		attemptRecord.Error = innerErr.Error()
		sg.Attempts = append(sg.Attempts, attemptRecord)
		lastErr = innerErr
	}

	return nil, lastErr
}

// resetSubgraphNodes resets all node statuses within a subgraph back to pending for retry.
// Walks the subgraph tree recursively.
func resetSubgraphNodes(sg *Subgraph) {
	for _, c := range sg.Children {
		if c.Node != nil {
			c.Node.Status = StatusPending
			c.Node.Error = ""
			c.Node.Timestamp = ""
		}
		if c.Subgraph != nil {
			resetSubgraphNodes(c.Subgraph)
		}
	}
}

// executeNode resolves slots, dispatches the action, stores the result, and pushes a recovery entry.
//
// Entry begins with a cooperative cancellation check: reading ctx.Err() catches both root/external cancel (the tool's
// signal handler closing the root context) and any ancestor combinator's scoped cancel (e.g., a gather that called its
// own cancel after the first iteration failure) via ctx inheritance through the dispatch chain. A cancelled check
// returns a failed NodeResult before the action runs.
//
// Parameters:
//   - ctx: the cancellation context threaded from dispatch; checked at entry.
//   - node: the node to execute.
//   - results: the accumulated results for promise resolution.
//   - stack: the recovery stack the node's compensation pushes onto.
//   - overrides: caller-supplied slot overrides for this node, if any.
//
// Returns:
//   - *NodeResult: the execution outcome, including any cancellation or action error.
func (e *GraphExecutor) executeNode(ctx context.Context, node *Node, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) *NodeResult {

	if err := ctx.Err(); err != nil {
		node.Status = StatusFailed
		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s: %w", node.ID(), err),
		}
	}

	ec := node.ExecutionContext()

	action, err := node.Action()

	if err != nil {
		node.Status = StatusFailed
		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s: %w", node.ID(), err),
		}
	}

	slots := node.ResolveSlots(ec, results, overrides)

	ec.Results = results

	e.hooks.FireNodeStart(ec, node.ID(), slots)

	result, complement, err := action.Do(ec, slots)

	if err != nil {

		// The action got far enough to mint a complement before failing — push it onto the recovery stack so
		// the framework owns unwinding the partial side effect. Mirrors the post-dispatch catalog-failure
		// path below: a non-nil complement alongside an error means "I made changes, please compensate."
		// Actions that have nothing to undo return either a typed-nil complement (the switch in
		// pushComplement returns early) or a zero-value Receipt{} (which carries no Resource, so
		// RecoveryStack.PushReceipt bails harmlessly and no entry is appended).
		pushComplement(stack, action, complement)

		e.hooks.FireNodeComplete(ec, node.ID(), nil, err)
		node.Status = StatusFailed

		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("%s: %w", node.Receiver, err),
		}
	}

	// Post-dispatch catalog reconciliation. See docs/architecture/4-resource-management.md §6.5, §6.8.
	//
	// If the method returned a Resource, the catalog records the resolved version so downstream nodes,
	// receipts, and compensation can observe it.
	//
	//   - Plan-time-shadowed case: the Planned companion ran during planning and the catalog already has
	//     a pending entry owned by this node. Transition fills the pending entry's metadata in place, so
	//     every outstanding pointer to the pending resource sees the populated fields.
	//   - Monadic case: no Planned companion (or the companion returned KnownAtExecution), so no pending
	//     entry exists. Shadow the real result now under this node's origin.
	//
	// A Shadow/Transition failure means the node's side effect has already happened; we push the action
	// onto the recovery stack before surfacing the catalog error so compensation can unwind it.
	if resource, isResource := result.(Resource); isResource && resource != nil && !IsKnownAtExecution(resource) {
		if node.graph.Catalog == nil {
			node.graph.Catalog = NewResourceCatalog()
		}
		catalog := node.graph.Catalog

		var catErr error
		if pendingID := catalog.Current(resource.URI()); pendingID != "" {
			if pending, ok := catalog.Lookup(pendingID); ok && pending.resourceBase().producerID == node.ID() {
				catErr = catalog.Transition(resource, node.ID())
			} else {
				_, catErr = catalog.Shadow(resource, node.ID())
			}
		} else {
			_, catErr = catalog.Shadow(resource, node.ID())
		}

		if catErr != nil {
			pushComplement(stack, action, complement)
			e.hooks.FireNodeComplete(ec, node.ID(), nil, catErr)
			node.Status = StatusFailed
			return &NodeResult{
				NodeID: node.ID(),
				Status: ResultFailed,
				Error:  fmt.Errorf("%s: post-dispatch catalog: %w", node.Receiver, catErr),
			}
		}
	}

	e.hooks.FireNodeComplete(ec, node.ID(), result, nil)

	if result != nil {
		results[node.ID()] = result
	}

	pushComplement(stack, action, complement)
	node.Status = StatusCompleted

	return &NodeResult{
		NodeID: node.ID(),
		Status: ResultCompleted,
	}
}

// pushComplement dispatches a node's complement onto the parent recovery stack by shape.
//
// The classifier in [Method.NewMethod] guarantees one of three shapes for any compensable action: nil (the
// action ran but produced no undo state), a [Receipt] (single-output compensable), or a [*RecoveryStack]
// (multi-output compensable whose receipts [Method.Invoke] has already wrapped into a sub-stack). Any other
// shape is unreachable by construction.
//
// Parameters:
//   - parent: the parent recovery stack receiving the entry.
//   - action: the [Action] whose complement is being pushed; supplies [Action.FullName] for receipt-bearing
//     entries.
//   - complement: the complement value returned by [Method.Invoke].
func pushComplement(parent *RecoveryStack, action Action, complement any) {

	switch v := complement.(type) {
	case nil:
		return
	case Receipt:
		_ = parent.PushReceipt(v, action.FullName())
	case *RecoveryStack:
		parent.PushNested(v)
	}
}
