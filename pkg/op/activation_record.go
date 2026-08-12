// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
)

// ActivationRecord serves as the data record specific to action invocations.
//
// It is passed as the initial argument injected by the framework into provider methods during every [Action.Do] and
// [CompensableAction.Undo] call. The framework constructs one [ActivationRecord] per dispatch and passes it to the
// provider method as the first parameter.
//
// [Provider] methods read shared session state via [ActivationRecord.RuntimeEnvironment], the dispatching unit via
// [ActivationRecord.CallerID], the graph via [ActivationRecord.Graph], and a `stdlib` `context.Context` for
// cancellation-aware operations via [ActivationRecord.Context].
//
// Each goroutine-driven dispatch holds its own [ActivationRecord]; pointer fields on `RuntimeEnvironment` (Catalog,
// Status, RecoverySite, Registry, etc.) share underlying instances with their own internal synchronization. Concurrent
// dispatches cannot race on per-call fields because they hold different records.
//
// CallerID identifies the caller in every dispatch mode (step 30): the dispatching unit's id under graph
// dispatch, a deterministic `file:line:col` call-site under starlark immediate-mode dispatch, and "" when no
// caller identity exists (test fixtures, CLI runners). Graph stays optional independently — the old Graph/Unit
// both-nil-or-both-set pairing invariant dissolved with the rename.
//
// Context is the per-dispatch cancellation context. It defaults to `RuntimeEnvironment.Context` at construction.
// Combinators (subgraph, choose, gather, and wait_until) derive a scoped child context with `context.WithCancel(
// activation.Context)` and assign it back so per-iteration cancellation reaches the nested provider methods. Provider
// methods don't act on the context for their own logic. They thread it into the stdlib / 3rd-party dependencies they
// call (e.g., `exec.CommandContext`, `http.NewRequestWithContext`), which use Go's standard context convention to abort
// on cancellation. To signal cancellation from a provider's own body, return an error wrapping `context.Context.Err()`.
//
// Lifecycle: created by the executor (or a non-graph dispatcher) before dispatch; consumed during the dispatch;
// discarded afterward. No persistent identity, no registry — each record is unique to one invocation.
type ActivationRecord struct {

	// RuntimeEnvironment is the session-scope execution environment. Always set during dispatch. Shared across every
	// concurrent dispatch in the same session; never mutated mid-execution.
	RuntimeEnvironment *RuntimeEnvironment

	// Context is the cancellation-aware context for this dispatch. Defaults to `RuntimeEnvironment.Context`;
	// combinators may assign a scoped child context to tighten the cancellation boundary for their nested dispatches.
	Context context.Context

	// Graph is the operation graph this activation belongs to. Non-nil during graph dispatch; nil for non-graph
	// dispatchers. Providers that traverse the graph (e.g., [flow.Provider] for `choose` / `gather` / `wait_until` /
	// `subgraph`) read this field; when `nil` they have no graph to walk.
	Graph *Graph

	// Stack is the recovery stack the current dispatch's receipt pushes onto and that [PromiseBinding.Resolve] queries
	// via [RecoveryStack.ResultByUnitID] for upstream unit results. Stamped by the executor when constructing the
	// activation; nil during non-graph dispatch.
	Stack *RecoveryStack

	// CallerID identifies the caller of the dispatched method (step 30). Graph dispatch: the dispatching unit's
	// id (a unit is a graph-encoded call to a provider method). Starlark dispatch: the script call-site as
	// `file:line:col` (a .star line is a script-encoded call to the same method). Empty when no caller identity
	// exists.
	//
	// [ResourceCatalog.GetOrCreate] takes it as the producer stamp on interned Resources, so a .star-produced
	// resource's ProducerID() reads like "mkfile.star:42:8" — its origin, visible in a debugger. Method bodies
	// that need the dispatching unit OBJECT (the flow combinators walking their subgraphs) resolve it via
	// `activation.Graph.ResolveExecutable(activation.CallerID)` — graph dispatch always has the graph in scope.
	CallerID string

	// Variables is the per-call variable frame in scope for this dispatch. Stamped by the executor just before
	// [Action.Do] is invoked. Carries the session-resolved variables ([VariableResolver] output) at top-level; per-call
	// frames (e.g., gather's per-iteration `item` binding) supersede it on nested dispatches.
	//
	// Concurrent dispatches each hold their own [ActivationRecord], so per-iteration frames built by combinators
	// (gather, future map / fold) are race-free by construction — each goroutine owns its activation and the variables
	// map referenced from it.
	Variables map[string]Variable

	// Slots holds this dispatch's resolved slot values — the output of [ExecutableUnit.ResolveSlots] keyed by the
	// parameter name. Stamped by the executor (or non-graph dispatcher) just before [Action.Do] is invoked, consumed by
	// [Method.Invoke] when mapping slot entries to typed Go arguments via reflection, then implicitly discarded when
	// the activation goes out of scope at dispatch tail.
	//
	// Conceptually transient: a binding-to-argument transform that lives only between resolve and call. It rides on
	// the activation rather than as a separate parameter, so the dispatch context is one bundle (alongside Variables,
	// Stack, Context, CallerID, Graph) rather than half-on-the-activation, half-in-a-parameter.
	Slots map[string]any

	// executor is the boundary that owns this dispatch — stamped by the executor when it builds the record (a node
	// dispatch, or a subgraph's own child executor). It stays private: a dispatched provider reaches the run-status
	// machine only through [ActivationRecord.RunStatus] (read) and [ActivationRecord.Transition] (the sole mutator),
	// never the executor itself. Nil during non-graph dispatch (the starlark immediate-mode bridge, test fixtures,
	// CLI runners).
	executor *GraphExecutor
}

// RunStatus returns a copy of the owning boundary's current [RunStatus] triplet.
//
// Read-only: the returned value is a copy, so a caller cannot change the run status through it — the only mutator is
// [ActivationRecord.Transition]. During non-graph dispatch (no executor) the zero triplet (preparing × healthy) is
// returned.
//
// Returns:
//   - `RunStatus`: the boundary executor's current status, or the zero value when there is no executor.
func (a *ActivationRecord) RunStatus() RunStatus {

	if a.executor == nil {
		return RunStatus{}
	}

	return a.executor.RunStatus()
}

// Transition submits a condition flip to the owning boundary's run status through the executor's single choke point.
//
// The one path by which a dispatched provider (a flow terminal driver) changes the run status; the executor is never
// exposed, so this and [ActivationRecord.RunStatus] are the entire run-status surface a provider sees. The submission
// is arbitrated: a request that would de-escalate the [Condition] is rejected with a non-nil error (monotonicity),
// while a worsening or same-condition request is applied or is a no-op. Phase is not an argument — the executor owns
// phase moves. A no-op returning nil during non-graph dispatch.
//
// Parameters:
//   - `condition`: the [Condition] being entered — must be at or above the current condition.
//   - `reason`: the [Reason] token classifying the flip.
//   - `message`: free-text detail, typically an err.Error().
//
// Returns:
//   - `error`: non-nil when the request would de-escalate the condition (rejected); nil when applied or a no-op.
func (a *ActivationRecord) Transition(condition Condition, reason Reason, message string) error {

	if a.executor == nil {
		return nil
	}

	return a.executor.Transition(a.CallerID, condition, reason, message)
}

// NewActivationRecord constructs an [*ActivationRecord] for one dispatch.
//
// The caller id is "" for non-graph, non-starlark dispatch; Graph is independently optional; the old pairing states
// are not legal under this design. [Context] is initialized to `runtimeEnvironment.Context`. Combinator-scoped callers
// (gather and similar) assign a derived child context to [ActivationRecord.Context] after construction to narrow the
// cancellation boundary for their nested dispatches.
//
// Parameters:
//   - `graph`: the graph this dispatch belongs to, or nil for non-graph dispatch.
//   - `unit`: the executable unit being dispatched, or nil for non-graph dispatch. Must be non-nil iff `graph` is
//     non-nil.
//   - `runtimeEnvironment`: the session-scope execution environment.
//
// Returns:
//   - *ActivationRecord: the constructed activation.
func NewActivationRecord(graph *Graph, callerID string, runtimeEnvironment *RuntimeEnvironment) *ActivationRecord {

	var ctx context.Context

	if runtimeEnvironment != nil {
		ctx = runtimeEnvironment.Context
	}

	return &ActivationRecord{
		RuntimeEnvironment: runtimeEnvironment,
		Context:            ctx,
		Graph:              graph,
		CallerID:           callerID,
	}
}

// DispatchChild dispatches a child through the owning [GraphExecutor], retrying per the child's [RetryPolicy].
//
// Available only from a bound subgraph's flow-method body — the executor stamps itself on the activation when it
// dispatches the bound subgraph via [Action.Do]. Calling DispatchChild outside that context (non-graph dispatch)
// returns an error.
//
// A thin forwarder to [GraphExecutor.dispatchWithPolicy] — the shared per-unit dispatch primitive that also drives the
// root from [GraphExecutor.Run] — so retry (and, in the failure-protocol seam, the OnRetry / OnError handlers) lives in
// one place, uniform for every unit and invisible to the flow method. See [GraphExecutor.dispatchWithPolicy] for the
// retry-budget / backoff / pause-cancel semantics.
//
// The caller supplies the [RecoveryStack] so compensations from this child dispatch land in the caller's saga boundary,
// and the `variables` frame for the child dispatch — typically `a.Variables` to inherit the current frame, or a
// per-iteration frame for combinators that rebind variables (gather binds `item` per iteration).
//
// Parameters:
//   - `ctx`: the cancellation context for the child dispatch and its backoff waits — typically `a.Context` or a scoped
//     child derived via `context.WithCancel`.
//   - `child`: the unit to dispatch (with retry).
//   - `stack`: the recovery stack child compensations push onto.
//   - `variables`: the variable frame in scope for the child dispatch.
//
// Returns:
//   - `any`: the child's terminal result on the succeeding attempt; nil when every attempt failed.
//   - `error`: non-nil if the child fails its retry budget, is paused / canceled, or DispatchChild is invoked outside
//     a bound-subgraph dispatch.
func (a *ActivationRecord) DispatchChild(
	ctx context.Context,
	child ExecutableUnit,
	stack *RecoveryStack,
	variables map[string]Variable,
) (any, error) {

	if a.executor == nil {
		return nil, fmt.Errorf("ActivationRecord.DispatchChild: not available outside a bound-subgraph dispatch")
	}

	return a.executor.dispatchWithPolicy(ctx, child, stack, variables)
}
