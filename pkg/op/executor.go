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
	hooks       *HookRegistry
	environment *RuntimeEnvironment
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
	return &GraphExecutor{environment: NewRuntimeEnvironment(context.Background(), spec)}
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

	var err error
	defer iox.Close(&err, e.environment.Root)
	graph.Rebind(e.environment)

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
	if !e.environment.DryRun {
		if err = ResolveResources(graph.Catalog); err != nil {
			graph.State = StateFailed
			return nil, err
		}
	}

	e.environment.Results = make(map[string]any)
	stack := NewRecoveryStack()

	result, err := graph.dispatch(e.environment.Context, e, stack, graph.Root, e.environment.Results, nil)

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

	ec := graph.RuntimeEnvironment()

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {

		// Apply backoff delay before retries (not before first attempt)
		if attempt > 0 && sg.Retry != nil {
			delay := sg.Retry.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-ec.Context.Done():
					return nil, ec.Context.Err()
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

	ec := node.RuntimeEnvironment()

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

	activationRecord := &ActivationRecord{Runtime: ec, SiteID: node.ID(), Context: ec.Context}
	result, complement, err := action.Do(activationRecord, slots)
	if err != nil {

		// The action got far enough to mint a complement before failing — push it onto the recovery stack so the
		// framework owns unwinding the partial side effect. A non-nil complement alongside an error means "I made
		// changes, please compensate." Actions that have nothing to undo return either a typed-nil complement
		// (PushComplement returns early) or a zero-value Receipt{} (which carries no Resource, so pushReceipt
		// bails harmlessly and no entry is appended).
		stack.PushComplement(action.FullName(), complement)

		e.hooks.FireNodeComplete(ec, node.ID(), nil, err)
		node.Status = StatusFailed

		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("%s: %w", node.Receiver, err),
		}
	}

	e.hooks.FireNodeComplete(ec, node.ID(), result, nil)

	if result != nil {
		results[node.ID()] = result
	}

	stack.PushComplement(action.FullName(), complement)
	node.Status = StatusCompleted

	return &NodeResult{NodeID: node.ID(), Status: ResultCompleted}
}

