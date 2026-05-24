// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

var (
	ErrNilGraph = errors.New("expected non-nil Graph")
)

// GraphExecutor executes a planned [*Graph] under a [*RuntimeEnvironmentSpec].
//
// The executor binds a graph and a spec at construction; every [GraphExecutor.Run] call builds a fresh
// per-run [*RuntimeEnvironment] from the spec, Clones the graph's planning catalog onto the per-run env,
// dispatches the graph, then tears the env down. Each Run gets an independent working catalog while the
// graph's planning catalog stays pristine across "plan once, run many" reuse.
type GraphExecutor struct {

	// graph is the planned graph this executor runs. Set at construction; never replaced.
	graph *Graph

	// spec is the immutable session configuration. Every Run builds a fresh [*RuntimeEnvironment] from it.
	spec *RuntimeEnvironmentSpec

	// hooks is the optional lifecycle hook registry. Shared across Runs; installed via
	// [GraphExecutor.SetHooks].
	hooks *HookRegistry

	// environment is the per-Run runtime environment. Set at the head of [GraphExecutor.Run] and cleared
	// (and Close'd) at the tail. Nil outside a Run. Tests that exercise [GraphExecutor.bindVariables]
	// directly mint an env here themselves rather than going through Run.
	environment *RuntimeEnvironment

	// variables is the per-Run resolved variable map. Mirrors `environment.variables` for the dispatch's
	// slot-resolution path. Cleared alongside `environment` at Run tail.
	variables map[string]Variable
}

// NewGraphExecutor returns an executor bound to `graph` and `spec`.
//
// The executor holds no per-Run state. Each [GraphExecutor.Run] call builds a fresh
// [*RuntimeEnvironment] from `spec`, Clones `graph.Catalog` onto it, dispatches the graph, and tears the
// env down — so the executor itself is cheap.
//
// Re-running the same graph is still gated by [Graph.State]: a graph transitions Pending → Executed or
// Pending → Failed on a Run, and a second Run on the same graph returns an error. Callers that want
// "plan once, run many" semantics today construct one executor per Run.
//
// Parameters:
//   - `graph`: the planned graph. Must be non-nil; in [StatePending] at every Run.
//   - `spec`: the session configuration. Must be non-nil.
//
// Returns:
//   - *GraphExecutor: the configured executor.
func NewGraphExecutor(graph *Graph, spec *RuntimeEnvironmentSpec) *GraphExecutor {
	assert.NonZero("graph", graph)
	assert.NonZero("spec", spec)
	return &GraphExecutor{
		graph: graph,
		spec:  spec,
	}
}

// SetHooks installs `hooks` as the lifecycle hook registry for every Run.
//
// Parameters:
//   - `hooks`: the hook registry to install.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
}

// Run dispatches the executor's graph under a fresh per-run [*RuntimeEnvironment].
//
// At every Run:
//
//  1. Build a fresh [*RuntimeEnvironment] from the stored spec, bound to `ctx`.
//  2. Clone `graph.Catalog` onto the new env's Catalog. The clone is independent — Resources written by
//     this Run cannot reach back into the graph's planning catalog.
//  3. Rebind the graph onto the per-run env.
//  4. Preflight: [GraphExecutor.bindVariables] resolves the graph's parameter surface against the
//     spec's [application.Application] source maps; caller-supplied `variables` layer on top as the
//     highest-priority source.
//  5. Dispatch through [Graph.dispatch].
//  6. On error, unwind the recovery stack so every successfully-completed Action gets its Compensate
//     companion called.
//  7. Close the env (deferred); clear the transient `e.environment` and `e.variables` fields.
//
// Parameters:
//   - `ctx`: the per-run cancellation context. Its values flow through `RuntimeEnvironment.Context` into
//     providers and subprocesses.
//   - `variables`: caller-supplied variable bindings layered on top of the resolver's output. Pass nil
//     or an empty map for the common case where the resolver alone produces the variable surface.
//
// Returns:
//   - `any`: the terminal node's output value, or nil if no node produced output.
//   - `error`: non-nil if preflight fails or any node or subgraph fails.
func (e *GraphExecutor) Run(ctx context.Context, variables map[string]Variable) (any, error) {

	if e.graph.State != StatePending {
		return nil, fmt.Errorf("graph already executed (state: %s)", e.graph.State)
	}

	e.environment = NewRuntimeEnvironment(ctx, e.spec)
	e.environment.Catalog = e.graph.Catalog.Clone()
	defer func() {
		_ = e.environment.Close()
		e.environment = nil
		e.variables = nil
	}()

	e.graph.Rebind(e.environment)
	defer e.graph.Unbind()

	if err := e.bindVariables(e.graph, variables); err != nil {
		e.graph.State = StateFailed
		return nil, err
	}

	e.environment.Results = make(map[string]any)
	stack := NewRecoveryStack()

	result, err := e.graph.dispatch(e.environment.Context, e, stack, e.graph.Root, e.environment.Results, nil)

	summary := e.graph.Summary()

	if err != nil {
		// Unwind the recovery stack in LIFO order so every action that
		// successfully completed before the failure gets its Compensate
		// companion called. The stack was populated on each successful
		// executeNode. Without this, TestCompensation-style rollback
		// never runs.
		if unwindErr := stack.Unwind(); unwindErr != nil {
			err = fmt.Errorf("%w; compensation: %w", err, unwindErr)
		}
		e.graph.State = StateFailed
		return nil, err
	}

	if summary.Failed() > 0 {
		e.graph.State = StateFailed
	} else {
		e.graph.State = StateExecuted
	}

	return result, nil
}

// bindVariables runs the binding-layer preflight pass.
//
// Walks `graph.Parameters()` (the bubble-up variable surface), drives the runtime environment's
// [VariableResolver] against its [application.Application] source maps, and merges the resolved variables
// into `RuntimeEnvironment.variables` and `e.variables` ready for slot resolution at dispatch time.
// Caller-supplied `callerVariables` are layered on top as the highest-priority source — useful for tests
// injecting specific bindings without going through Application plumbing.
//
// Variable resolution is pure (reads in-memory Application maps and process environment variables; no
// filesystem or network probes), so the pass runs in both regular and dry-run modes. This is intentional:
// dry-run output that renders slot values needs them resolved.
//
// `graph` is passed explicitly (rather than read from `e.graph`) so unit tests can exercise the preflight
// pass against arbitrary graphs without rebuilding the executor.
//
// Parameters:
//   - `graph`: the bound execution graph; `graph.Parameters()` drives the resolver's parameter input.
//   - `callerVariables`: optional caller-supplied bindings that override resolver output.
//
// Returns:
//   - `error`: nil on success; on failure, the joined aggregated error from the resolver (missing-required
//     and type-mismatch entries).
func (e *GraphExecutor) bindVariables(graph *Graph, callerVariables map[string]Variable) error {

	params, paramErr := graph.Parameters()
	if paramErr != nil {
		return paramErr
	}

	resolver := e.environment.variableResolver
	if errs := resolver.Resolve(e.environment, params); len(errs) > 0 {
		return errors.Join(errs...)
	}

	if e.environment.variables == nil {
		e.environment.variables = make(map[string]Variable, len(params)+len(callerVariables))
	}

	for name, v := range resolver.Variables() {
		e.environment.variables[name] = v
	}

	for name, v := range callerVariables {
		e.environment.variables[name] = v
	}

	e.variables = e.environment.variables

	return nil
}

// executeSubgraph dispatches a [*Subgraph] through the same Action.Do path [GraphExecutor.executeNode]
// uses, with two entry shapes:
//
//  1. Structural container (`sg.Action() == nil`). The graph root takes this path; any other unbound
//     subgraph is structural-only too. The executor walks `sg.Children()` directly via
//     [Graph.dispatch]; child Nodes route through executeNode, nested Subgraphs route back through
//     executeSubgraph. Container output is nil — the meaningful results flow from the children's
//     entries in `results`.
//  2. Bound subgraph (`sg.Action() != nil`). flow.Subgraph / flow.Gather / flow.Choose /
//     flow.WaitUntil all reach this path. The subgraph's own slots are resolved (matching the
//     bound method's parameter list); the activation is built with the subgraph as `Unit`; the
//     action's `Do` is invoked. The flow method's body orchestrates the children walk + any
//     per-iteration semantics (retry, errorAction, frame minting). The flow.Provider.Subgraph
//     implementation is filled in by step 18.e; until then bound subgraphs dispatch successfully
//     but their children are silently skipped.
//
// Cooperative cancellation check at entry mirrors [executeNode]: `ctx.Err()` is observed before
// any dispatch, so subgraph dispatch bails on root/external cancel or any ancestor combinator's
// scoped cancel.
//
// Audit-receipt push happens at every exit (cancelled, action error, success) — same shape as
// executeNode, stamped with the subgraph's ID. Subgraph hooks ([HookRegistry.FireSubgraphStart] /
// [HookRegistry.FireSubgraphComplete]) fire around the dispatch.
//
// Parameters:
//   - `ctx`: the cancellation context threaded from [Graph.dispatch].
//   - `graph`: the enclosing graph (passed to [NewActivationRecord]).
//   - `sg`: the subgraph to dispatch.
//   - `results`: the accumulated unit results for promise resolution; the subgraph's terminal
//     result is keyed by its ID on success.
//   - `stack`: the recovery stack child compensations push onto.
//   - `overrides`: caller-supplied slot overrides for this subgraph, or nil.
//
// Returns:
//   - `any`: the subgraph's terminal result, or nil for structural-container dispatches and for
//     bound dispatches whose action produces no output.
//   - `error`: non-nil on cancellation, on a structural-container child-walk failure, or on a
//     bound action's failure.
func (e *GraphExecutor) executeSubgraph(ctx context.Context, graph *Graph, sg *Subgraph, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	runtimeEnvironment := e.environment
	subgraphID := sg.ID()

	// pushAuditReceipt mirrors [executeNode]'s pushAuditReceipt: stamps a receipt at a dispatch exit,
	// promoting a Receipt complement to the audit entry or building a fresh *ReceiptBase otherwise,
	// and pushing nested *RecoveryStack complements alongside.
	pushAuditReceipt := func(status Status, slots map[string]any, result any, complement any, dispatchErr error, actionFullName string) {

		var receipt Receipt

		if c, ok := complement.(Receipt); ok {
			receipt = c
		} else {
			receipt = &ReceiptBase{}
			if c, ok := complement.(*RecoveryStack); ok {
				stack.PushNested(c)
			}
		}

		receipt.SetStatus(status)
		receipt.SetErr(dispatchErr)
		receipt.SetResult(result)
		receipt.SetSlots(slots)
		receipt.SetUnitID(subgraphID)
		receipt.SetComplement(complement)

		if actionFullName != "" {
			_ = receipt.Commit(actionFullName)
		}

		_ = stack.Push(receipt)
	}

	// Exit 1: context cancelled before dispatch begins.
	if err := ctx.Err(); err != nil {
		pushAuditReceipt(StatusFailed, nil, nil, nil, err, "")
		return nil, fmt.Errorf("subgraph %s: %w", subgraphID, err)
	}

	action := sg.Action()

	// Structural-container path: no bound action. Walk children directly via [Graph.dispatch].
	if action == nil {

		e.hooks.FireSubgraphStart(runtimeEnvironment, subgraphID)

		for _, child := range sg.Children() {
			if _, err := graph.dispatch(ctx, e, stack, child, results, nil); err != nil {
				pushAuditReceipt(StatusFailed, nil, nil, nil, err, "")
				e.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, err)
				return nil, fmt.Errorf("subgraph %s: child %s: %w", subgraphID, child.ID(), err)
			}
		}

		pushAuditReceipt(StatusCompleted, nil, nil, nil, nil, "")
		e.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, nil)
		return nil, nil
	}

	// Bound-action path: dispatch via Action.Do — same as Node.
	slots := sg.ResolveSlots(e.variables, results, overrides)
	runtimeEnvironment.Results = results
	e.hooks.FireSubgraphStart(runtimeEnvironment, subgraphID)

	activationRecord := NewActivationRecord(graph, sg, runtimeEnvironment)
	activationRecord.Context = ctx
	activationRecord.dispatchChild = func(childCtx context.Context, child ExecutableUnit, subStack *RecoveryStack, ov map[string]SlotValue) (any, error) {
		return graph.dispatch(childCtx, e, subStack, child, results, ov)
	}
	result, complement, err := action.Do(activationRecord, slots)

	// Exit 2: Do returned an error.
	if err != nil {
		pushAuditReceipt(StatusFailed, slots, nil, complement, err, action.FullName())
		e.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, err)
		return nil, fmt.Errorf("subgraph %s: %s: %w", subgraphID, action.Name(), err)
	}

	// Exit 3: successful dispatch.
	if result != nil {
		results[subgraphID] = result
	}
	pushAuditReceipt(StatusCompleted, slots, result, complement, nil, action.FullName())
	e.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, nil)

	return result, nil
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
func (e *GraphExecutor) executeNode(ctx context.Context, graph *Graph, node *Node, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) *NodeResult {

	runtimeEnvironment := e.environment
	nodeID := node.ID()

	// pushAuditReceipt builds, stamps, and pushes a receipt at a dispatch exit.
	//
	// If complement is a Receipt, that receipt becomes the audit-trail entry (stamped with the
	// dispatch's status / err / result / slots / unitID). If complement is a *RecoveryStack
	// (multi-output compensable), it's pushed nested AND a fresh audit-only *ReceiptBase is pushed
	// alongside. Otherwise a fresh audit-only *ReceiptBase is pushed.
	pushAuditReceipt := func(status Status, slots map[string]any, result any, complement any, dispatchErr error, actionFullName string) {

		var receipt Receipt

		if c, ok := complement.(Receipt); ok {
			receipt = c
		} else {
			receipt = &ReceiptBase{}
			if c, ok := complement.(*RecoveryStack); ok {
				stack.PushNested(c)
			}
		}

		receipt.SetStatus(status)
		receipt.SetErr(dispatchErr)
		receipt.SetResult(result)
		receipt.SetSlots(slots)
		receipt.SetUnitID(nodeID)
		receipt.SetComplement(complement)

		if actionFullName != "" {
			_ = receipt.Commit(actionFullName)
		}

		_ = stack.Push(receipt)
	}

	// Exit 1: context cancelled before dispatch begins.
	if err := ctx.Err(); err != nil {
		pushAuditReceipt(StatusFailed, nil, nil, nil, err, "")
		return &NodeResult{
			NodeID: nodeID,
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s: %w", nodeID, err),
		}
	}

	// Resolve the action via the bound base accessor. Every writer binds the Action at construction
	// time (step 14 migration); a nil Action here is a programming error.
	action := node.Action()
	if action == nil {
		err := fmt.Errorf("node %s: no Action bound", nodeID)
		pushAuditReceipt(StatusFailed, nil, nil, nil, err, "")
		return &NodeResult{
			NodeID: nodeID,
			Status: ResultFailed,
			Error:  err,
		}
	}

	slots := node.ResolveSlots(e.variables, results, overrides)
	runtimeEnvironment.Results = results
	e.hooks.FireNodeStart(runtimeEnvironment, nodeID, slots)

	activationRecord := NewActivationRecord(graph, node, runtimeEnvironment)
	activationRecord.Context = ctx
	result, complement, err := action.Do(activationRecord, slots)

	// Exit 2: Do returned an error.
	if err != nil {
		pushAuditReceipt(StatusFailed, slots, nil, complement, err, action.FullName())
		e.hooks.FireNodeComplete(runtimeEnvironment, nodeID, nil, err)
		return &NodeResult{
			NodeID: nodeID,
			Status: ResultFailed,
			Error:  fmt.Errorf("%s: %w", action.Name(), err),
		}
	}

	// Exit 3: successful dispatch.
	if result != nil {
		results[nodeID] = result
	}
	pushAuditReceipt(StatusCompleted, slots, result, complement, nil, action.FullName())
	e.hooks.FireNodeComplete(runtimeEnvironment, nodeID, result, nil)

	return &NodeResult{NodeID: nodeID, Status: ResultCompleted}
}
