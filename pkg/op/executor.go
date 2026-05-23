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

// GraphExecutor executes action graphs.
type GraphExecutor struct {
	hooks       *HookRegistry
	environment *RuntimeEnvironment
	variables   map[string]Variable

	// owns is true when this executor built `environment` itself (via NewGraphExecutor); false when the
	// runtime environment was borrowed from a caller (via NewGraphExecutorForEnv). Drives Close: owned
	// environments are released by the executor; borrowed environments are left for their owner to close.
	owns bool
}

// NewGraphExecutor creates an executor that owns a freshly-built execution environment derived from spec.
//
// The executor is the session owner for execution work. The owned environment's lifetime matches the
// executor's; callers `defer executor.Close()` to release it. The provided `ctx` is propagated into the
// environment's [RuntimeEnvironment.Context] field — signal handlers, timeouts, request-scoped values
// flow through to providers and subprocesses.
//
// Parameters:
//   - `ctx`: the parent context whose cancellation / values flow through `RuntimeEnvironment.Context`
//     into providers.
//   - `spec`: the execution-environment configuration.
//
// Returns:
//   - *GraphExecutor: the configured executor; call [GraphExecutor.Close] when done.
func NewGraphExecutor(ctx context.Context, spec *RuntimeEnvironmentSpec) *GraphExecutor {
	assert.NonZero("spec", spec)
	return &GraphExecutor{
		environment: NewRuntimeEnvironment(ctx, spec),
		owns:        true,
	}
}

// NewGraphExecutorForEnv creates an executor that borrows the supplied runtime environment.
//
// The borrowed-environment variant supports the in-script single-session path (D7): when starlark drives
// execution via `plan.run`, [plan.Provider.Run] constructs a borrowed executor over the planning runtime
// environment instead of building a fresh one from a spec. Preflight, dispatch, and the rest of the
// execution machinery happen inside this executor exactly as they do for the spec-owning variant — the
// only difference is that [GraphExecutor.Close] is a no-op so the surrounding `op.Plan` lifecycle stays
// responsible for closing the runtime environment.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment to borrow. Must not be nil.
//
// Returns:
//   - *GraphExecutor: the configured executor; call [GraphExecutor.Close] when done (no-op for the
//     borrowed-environment variant).
func NewGraphExecutorForEnv(runtimeEnvironment *RuntimeEnvironment) *GraphExecutor {
	assert.NonZero("runtimeEnvironment", runtimeEnvironment)
	return &GraphExecutor{
		environment: runtimeEnvironment,
		owns:        false,
	}
}

// Close releases the executor's owned runtime environment.
//
// No-op for executors that borrowed their runtime environment via [NewGraphExecutorForEnv] — those leave
// Close responsibility to the runtime environment's owner.
//
// Idempotent for owned runtime environments — delegates to [RuntimeEnvironment.Close], which runs the
// close path exactly once per runtime environment regardless of how many times it is invoked.
//
// Returns:
//   - `error`: the joined error from closing the runtime environment's owned resources, or nil on
//     success or for a borrowed-environment executor.
func (e *GraphExecutor) Close() error {
	if !e.owns {
		return nil
	}
	return e.environment.Close()
}

// Environment exposes the executor-owned runtime environment.
//
// Used by callers that need to construct or pass graph-companion objects (e.g., a [starlarkbridge.Runtime]
// sharing the executor's runtime environment). Callers must not retain the reference past the executor's
// lifetime.
//
// Returns:
//   - *RuntimeEnvironment: the executor's runtime environment.
func (e *GraphExecutor) Environment() *RuntimeEnvironment {
	return e.environment
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
// The graph root is treated as an implicit subgraph. Subgraph dispatch and its children-walking machinery are
// scheduled for Phase 5 alongside the [GraphExecutor.executeSubgraph] rewrite.
//
// Run's preflight pipeline is:
//
//  1. [GraphExecutor.bindVariables] — runs in every mode (including dry-run). Reads
//     `graph.Parameters()` and calls [VariableResolver.Resolve] against the runtime
//     environment's [application.Application] source maps. Missing-required and
//     type-mismatch errors aggregate; a non-empty result fails the run before any
//     dispatch.
//  2. [ResourceCatalog.ResolvePending] — runs in regular mode only. Drives Pending
//     catalog entries to Active or Gone; this pass touches target-machine state
//     (filesystem/network probes) so dry-run skips it.
//
// Parameters:
//   - `graph`: the execution graph to run.
//   - `variables`: caller-supplied variable bindings. Merged on top of the resolver's
//     output as the highest-priority layer; pass nil or an empty map for the common
//     case where the resolver alone produces the variable surface. Useful for tests
//     that want to inject specific variable values without going through Application
//     plumbing.
//
// Returns:
//   - `any`: the terminal node's output value, or nil if no node produced output.
//   - `error`: non-nil if preflight fails or any node or subgraph fails.
func (e *GraphExecutor) Run(graph *Graph, variables map[string]Variable) (any, error) {

	if graph.State != StatePending {
		return nil, fmt.Errorf("graph already executed (state: %s)", graph.State)
	}

	graph.Rebind(e.environment)
	defer graph.Unbind()

	if err := e.bindVariables(graph, variables); err != nil {
		graph.State = StateFailed
		return nil, err
	}

	// Pre-flight resolution pass. Drive every Pending catalog entry to Active
	// or Gone by calling r.Resolve() on each. Active and Gone entries are not
	// touched (Active is already resolved; Gone is terminal). Any Pending
	// entry whose Resolve fails transitions to Gone and contributes a
	// URI-wrapped error to the aggregated result; a non-empty result aborts
	// the run.
	//
	// This is the link-time symbol resolution pass. See the resource-management
	// architecture doc §6.5.
	//
	// Skipped in dry-run mode: dry-run validates graph structure without
	// asserting target-machine state.
	if !e.environment.Application.DryRun() {
		if errs := graph.Catalog.ResolvePending(); len(errs) > 0 {
			graph.State = StateFailed
			return nil, errors.Join(errs...)
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

// executeSubgraph is the executor entry point for an [op.Subgraph]. Body intentionally stripped —
// the prior implementation predated the four-terminal model, the frame-chain execution model
// (plan-doc D11), and the by-ID containment model (Phase 3.5). The rewrite is scheduled for
// Phase 5 (alongside plan.assemble / plan.run) so the executor matches the post-materialization
// Subgraph shape: per-dispatch [Frame] minting populated from [Subgraph.FrameBindings],
// [Subgraph.Items] propagation to the body, retry per the frame-chain [effectiveRetryPolicy]
// rule, and [errorAction] dispatch on failure.
//
// Until that rewrite lands, calling this function is a programming error — it surfaces loudly so
// any path that reaches container dispatch in Phase 4.5 fails fast with a clear pointer to the
// pending work, instead of producing silent garbage from a stale implementation.
//
// Parameters:
//   - `ctx`: ignored.
//   - `graph`: ignored.
//   - `sg`: ignored.
//   - `results`: ignored.
//   - `stack`: ignored.
//   - `overrides`: ignored.
//
// Returns:
//   - `any`: always nil.
//   - `error`: always non-nil; describes the pending Phase 5 rewrite.
func (e *GraphExecutor) executeSubgraph(_ context.Context, _ *Graph, sg *Subgraph, _ map[string]any, _ *RecoveryStack, _ map[string]SlotValue) (any, error) {
	return nil, fmt.Errorf("executeSubgraph(%q): body stripped pending Phase 5 rewrite (frame minting + FrameBindings + Items + errorAction); see plan-doc D11", sg.ID())
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
