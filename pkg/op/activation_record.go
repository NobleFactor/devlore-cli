// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
)

// ActivationRecord is the per-invocation data record threaded through every [Action.Do] call (and
// every [CompensableAction.Undo] call) as the framework-injected first argument to provider methods.
//
// The framework constructs one [ActivationRecord] per dispatch and passes it to the provider method as
// the first parameter. Provider methods read shared session state via [ActivationRecord.RuntimeEnvironment],
// the dispatching unit via [ActivationRecord.Unit], the graph via [ActivationRecord.Graph], and a stdlib
// `context.Context` for cancellation-aware operations via [ActivationRecord.Context].
//
// Each goroutine-driven dispatch holds its own [ActivationRecord]; pointer fields on `RuntimeEnvironment`
// (Catalog, Status, RecoverySite, Registry, etc.) share underlying instances with their own internal
// synchronization. Concurrent dispatches cannot race on per-call fields because they hold different
// records.
//
// Graph and Unit are coupled — both nil during non-graph dispatch (the starlark immediate-mode bridge,
// test fixtures, CLI runners), and both non-nil during graph dispatch. The intermediate states (Graph
// set without Unit, or Unit set without Graph) are not legal under this design; the constructor
// documents the invariant but does not enforce it in the type.
//
// Context is the per-dispatch cancellation context. It defaults to `RuntimeEnvironment.Context` at
// construction; combinators (gather, future choose / wait_until) derive a scoped child context with
// `context.WithCancel(activation.Context)` and assign it back so per-iteration cancellation reaches
// the nested provider methods. Provider methods don't act on the context for their own logic — they
// thread it into the stdlib / 3rd-party dependencies they call (e.g., `exec.CommandContext`,
// `http.NewRequestWithContext`), which use Go's standard context convention to abort on cancellation.
// To signal cancellation from a provider's own body, return an error wrapping `ctx.Err()`.
//
// Lifecycle: created by the executor (or a non-graph dispatcher) before dispatch; consumed during
// the dispatch; discarded afterward. No persistent identity, no registry — each record is unique to
// one invocation.
type ActivationRecord struct {

	// Context is the cancellation-aware context for this dispatch. Defaults to
	// `RuntimeEnvironment.Context`; combinators may assign a scoped child context to tighten the
	// cancellation boundary for their nested dispatches.
	Context context.Context

	// Graph is the operation graph this activation belongs to. Non-nil during graph dispatch; nil
	// for non-graph dispatchers. Providers that traverse the graph (e.g., [flow.Provider] for
	// choose / gather / wait_until / subgraph) read this field; when nil they have no graph to walk.
	Graph *Graph

	// Unit is the executable unit being dispatched — *Node for node dispatches, *Subgraph for
	// subgraph dispatches. Non-nil during graph dispatch; nil for non-graph dispatchers. Coupled
	// with Graph: both nil or both non-nil.
	//
	// Method bodies that need the dispatching subgraph (e.g., [flow.Provider.Subgraph] walking its
	// children) type-assert:
	//
	//   sg, ok := activation.Unit.(*Subgraph)
	//
	// [ResourceCatalog.GetOrCreate] reads `Unit.ID()` as the producer stamp on interned Resources.
	// When Unit is nil (non-graph dispatch) the catalog interns the Resource with an empty producer
	// stamp — bridge / test / CLI Resources are reachable by URI but carry no lineage edge.
	Unit ExecutableUnit

	// RuntimeEnvironment is the session-scope execution environment. Always set during dispatch.
	// Shared across every concurrent dispatch in the same session; never mutated mid-execution.
	RuntimeEnvironment *RuntimeEnvironment

	// Variables is the per-call variable frame in scope for this dispatch. Stamped by the executor
	// just before [Action.Do] is invoked. Carries the session-resolved variables ([VariableResolver]
	// output) at top-level; per-call frames (e.g., gather's per-iteration `item` binding) supersede
	// it on nested dispatches.
	//
	// Concurrent dispatches each hold their own [ActivationRecord], so per-iteration frames built
	// by combinators (gather, future map / fold) are race-free by construction — each goroutine
	// owns its activation and the variables map referenced from it.
	Variables map[string]Variable

	// dispatchChild forwards a child dispatch through the parent [GraphExecutor], preserving
	// observability hooks and the parent run's results map. Installed by
	// [GraphExecutor.executeSubgraph] on the bound-action path so the dispatched flow-method body
	// (typically [flow.Provider.Subgraph]) can walk [Subgraph.Children] without reaching into the
	// executor type. The flow method supplies the [RecoveryStack] per call so compensations
	// accumulate at the subgraph-local saga boundary; it also supplies the `variables` frame for
	// the child dispatch — typically `activation.Variables` to inherit the current frame, or a
	// per-iteration frame for combinators that rebind variables (gather binds `item`).
	//
	// Nil during non-graph dispatch (the starlark immediate-mode bridge, test fixtures, CLI runners)
	// and during structural-container dispatch (the executor walks children itself for `action == nil`
	// subgraphs).
	dispatchChild func(ctx context.Context, child ExecutableUnit, stack *RecoveryStack, variables map[string]Variable) (any, error)
}

// NewActivationRecord constructs an [*ActivationRecord] for one dispatch. Graph and Unit must be
// either both nil (non-graph dispatch) or both non-nil (graph dispatch); the intermediate states
// are not legal under this design.
//
// [Context] is initialized to `runtimeEnvironment.Context`. Combinator-scoped callers (gather and
// similar) assign a derived child context to [ActivationRecord.Context] after construction to
// narrow the cancellation boundary for their nested dispatches.
//
// Parameters:
//   - `graph`: the graph this dispatch belongs to, or nil for non-graph dispatch.
//   - `unit`: the executable unit being dispatched, or nil for non-graph dispatch. Must be non-nil
//     iff `graph` is non-nil.
//   - `runtimeEnvironment`: the session-scope execution environment.
//
// Returns:
//   - *ActivationRecord: the constructed activation.
func NewActivationRecord(graph *Graph, unit ExecutableUnit, runtimeEnvironment *RuntimeEnvironment) *ActivationRecord {

	var ctx context.Context
	if runtimeEnvironment != nil {
		ctx = runtimeEnvironment.Context
	}
	return &ActivationRecord{
		Context:            ctx,
		Graph:              graph,
		Unit:               unit,
		RuntimeEnvironment: runtimeEnvironment,
	}
}

// DispatchChild forwards a child dispatch through the parent [GraphExecutor].
//
// Available only from a bound subgraph's flow-method body — the executor installs the underlying
// closure when it dispatches the bound subgraph via [Action.Do]. Calling DispatchChild outside that
// context (non-graph dispatch, structural-container dispatch) returns an error.
//
// The caller supplies the [RecoveryStack] so compensations from this child dispatch land in the
// caller's saga boundary, and the `variables` frame for the child dispatch — typically
// `a.Variables` to inherit the current frame, or a per-iteration frame for combinators that
// rebind variables (gather binds `item` per iteration).
//
// Parameters:
//   - `ctx`: the cancellation context for the child dispatch — typically `a.Context` or a scoped
//     child derived via `context.WithCancel`.
//   - `child`: the unit to dispatch.
//   - `stack`: the recovery stack child compensations push onto.
//   - `variables`: the variable frame in scope for the child dispatch.
//
// Returns:
//   - `any`: the child's terminal result.
//   - `error`: non-nil if the child fails or DispatchChild is invoked outside a bound-subgraph
//     dispatch.
func (a *ActivationRecord) DispatchChild(ctx context.Context, child ExecutableUnit, stack *RecoveryStack, variables map[string]Variable) (any, error) {

	if a.dispatchChild == nil {
		return nil, fmt.Errorf("ActivationRecord.DispatchChild: not available outside a bound-subgraph dispatch")
	}
	return a.dispatchChild(ctx, child, stack, variables)
}
