// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

var (
	// ErrNilGraph is the sentinel error returned by [NewGraphExecutor] and [ResumeExecutor] when the caller
	// passes a nil *Graph. Surfaces through the assert.NonZero precondition; declared here so callers can
	// match the specific shape via errors.Is when they need to distinguish nil-graph from other errors.
	ErrNilGraph = errors.New("expected non-nil Graph")

	// ErrPaused is the sentinel error returned by [GraphExecutor.Run] when the run halted because
	// [GraphExecutor.Pause] was called. The executor's [RunState] is [RunStatePaused] on this exit;
	// callers can take a [*Snapshot] and resume later via [ResumeExecutor].
	ErrPaused = errors.New("execution paused")
)

// GraphExecutor executes a planned [*Graph] under a [*RuntimeEnvironmentSpec].
//
// One executor drives one execution. [GraphExecutor.Run] builds a per-run [*RuntimeEnvironment] from the
// spec, clones the graph's planning catalog onto that env, dispatches the graph, then tears the env down.
// Each Run gets an independent working catalog while the graph's planning catalog stays pristine — but
// each Run requires a fresh executor; a second [GraphExecutor.Run] call on the same executor returns an
// error. Re-execution = `NewGraphExecutor(graph, spec)` again; resuming from a paused execution rebuilds
// the executor from a serialized snapshot.
type GraphExecutor struct {

	// graph is the planned graph this executor runs. Set at construction; never replaced.
	graph *Graph

	// spec is the immutable session configuration. The Run builds a fresh [*RuntimeEnvironment] from it.
	spec *RuntimeEnvironmentSpec

	// hooks is the optional lifecycle hook registry. Installed via [GraphExecutor.SetHooks] before Run.
	hooks *HookRegistry

	// state is the executor's top-level [RunState]. Zero value is [RunStatePending]; transitions to
	// [RunStateRunning] at the head of [GraphExecutor.Run] and reaches a terminal state
	// ([RunStateCompleted] or [RunStateFailed]) at exit.
	state RunState

	// stack is the per-Run [*RecoveryStack] — the audit + compensation ledger of every dispatch.
	// Initialized at the head of [GraphExecutor.Run] and held across the Run so [GraphExecutor.Snapshot]
	// can project it into a serializable [*Snapshot] at any moment (including post-Run).
	stack *RecoveryStack

	// pauseRequested is the pause-signal flag set by [GraphExecutor.Pause] and observed at pause-points
	// inside the dispatch chain. Atomic so [Pause] can be called from a goroutine other than the one
	// driving [GraphExecutor.Run].
	pauseRequested atomic.Bool

	// environment is the per-Run runtime environment. Set at the head of [GraphExecutor.Run] and cleared
	// (and Close'd) at the tail. Nil outside a Run. Tests that exercise [GraphExecutor.bindVariables]
	// directly mint an env here themselves rather than going through Run.
	environment *RuntimeEnvironment

	// variables is the per-Run resolved variable map. Mirrors `environment.variables` for the dispatch's
	// slot-resolution path. Cleared alongside `environment` at Run tail.
	variables map[string]Variable

	// lastVariables is the post-Run snapshot of the resolved variable map, preserved past `environment` /
	// `variables` teardown so post-Run inspection (test harnesses, observability) can read the values
	// without holding onto the runtime environment itself.
	lastVariables map[string]Variable
}

// LastVariables returns a snapshot of the resolved variable map from the most recent [GraphExecutor.Run].
// Empty before the first Run; preserved across the Run teardown. Cleared and re-populated by each Run.
//
// Returns:
//   - map[string]Variable: the resolved variables; never nil, may be empty when no parameters bubbled up.
func (e *GraphExecutor) LastVariables() map[string]Variable {

	if e.lastVariables == nil {
		return map[string]Variable{}
	}
	return e.lastVariables
}

// NewGraphExecutor returns an executor bound to `graph` and `spec`, in [RunStatePending].
//
// The executor drives a single execution. [GraphExecutor.Run] builds a fresh [*RuntimeEnvironment] from
// `spec`, clones the graph's planning catalog onto it, dispatches the graph, and tears the env down — so
// the executor itself is cheap. Re-running the same graph means constructing a new executor;
// [GraphExecutor.Run] rejects a second call against the same executor.
//
// Parameters:
//   - `graph`: the planned graph. Must be non-nil.
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

// region EXPORTED METHODS

// region State management

// SetHooks installs `hooks` as the lifecycle hook registry for every Run.
//
// Parameters:
//   - `hooks`: the hook registry to install.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
}

// Snapshot projects the executor's current per-run mutable state into a serializable [*Snapshot].
//
// Pairs with the executor's bound [*Graph] (loaded separately via [LoadGraph]) to fully describe the
// execution. The graph identity is captured as [Snapshot.GraphChecksum] for resume-time verification.
// Safe to call at any point — before, during, or after [GraphExecutor.Run] — the stack and variables
// fields are nil-safe.
//
// Returns:
//   - *Snapshot: the captured state.
func (e *GraphExecutor) Snapshot() *Snapshot {

	return &Snapshot{
		GraphChecksum: e.graph.Checksum(),
		State:         e.state,
		Stack:         e.stack,
		Variables:     e.variables,
	}
}

// State returns the executor's current [RunState].
//
// Concurrent-safe to read at any point; the field is mutated only by the goroutine driving
// [GraphExecutor.Run] (and by [GraphExecutor.Pause]'s observation of the pause flag at the next
// pause-point).
//
// Returns:
//   - `RunState`: the current state.
func (e *GraphExecutor) State() RunState {
	return e.state
}

// Pause signals the executor to transition [RunStateRunning] → [RunStatePaused] at the next
// pause-point in the dispatch chain.
//
// Pause returns immediately. The actual transition happens on the goroutine driving
// [GraphExecutor.Run] when it next observes the pause flag — at which point Run returns
// [ErrPaused] with [GraphExecutor.State] reporting [RunStatePaused]. If the run terminates
// (Completed or Failed) before the pause-point is reached, the pause request is silently dropped
// and the executor lands in the corresponding terminal state.
//
// Safe to call from a goroutine other than the one driving Run; the pause flag is atomic.
//
// Returns:
//   - `error`: non-nil when the executor is not in [RunStateRunning] (nothing to pause).
func (e *GraphExecutor) Pause() error {

	if e.state != RunStateRunning {
		return fmt.Errorf("Pause: executor is not running (state: %s)", e.state)
	}
	e.pauseRequested.Store(true)
	return nil
}

// pausePointObserved is the pause-point hook invoked by the dispatch chain before each unit
// dispatch. When the pause flag is set, it transitions state to [RunStatePaused] and returns true;
// the caller then unwinds without dispatching further. When the flag is not set, it returns false
// and dispatch proceeds.
//
// Returns:
//   - `bool`: true when a pause has been requested and the executor has transitioned to
//     [RunStatePaused]; false otherwise.
func (e *GraphExecutor) pausePointObserved() bool {

	if !e.pauseRequested.Load() {
		return false
	}
	e.state = RunStatePaused
	return true
}

// ResumeExecutor constructs a [*GraphExecutor] from a `(graph, spec, snapshot)` triple and prepares
// it to continue dispatch from the [*Snapshot]'s state.
//
// The snapshot's [Snapshot.GraphChecksum] must match `graph.Checksum()` — a mismatch indicates the
// graph has changed since the pause and the snapshot is incompatible. On success the returned
// executor has its [RunState], [*RecoveryStack], and resolved variables restored from the snapshot;
// a subsequent [GraphExecutor.Run] continues dispatch from that point, skipping units whose UnitID
// already appears in the recovery stack with a successful receipt.
//
// Parameters:
//   - `graph`: the planned graph the snapshot was taken against. Must be non-nil.
//   - `spec`: the session configuration for the resumed execution. Must be non-nil.
//   - `snapshot`: the captured execution state. Must be non-nil and graph-compatible.
//
// Returns:
//   - *GraphExecutor: the executor ready to resume.
//   - `error`: non-nil on nil arguments or checksum mismatch.
func ResumeExecutor(graph *Graph, spec *RuntimeEnvironmentSpec, snapshot *Snapshot) (*GraphExecutor, error) {

	assert.NonZero("graph", graph)
	assert.NonZero("spec", spec)
	assert.NonZero("snapshot", snapshot)

	if graph.Checksum() != snapshot.GraphChecksum {
		return nil, fmt.Errorf("ResumeExecutor: graph checksum mismatch: snapshot=%q graph=%q",
			snapshot.GraphChecksum, graph.Checksum())
	}

	e := NewGraphExecutor(graph, spec)
	e.state = snapshot.State
	e.stack = snapshot.Stack
	e.variables = snapshot.Variables
	e.lastVariables = snapshot.Variables

	return e, nil
}

// endregion

// region Behaviors

// Fallible actions

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

	if e.state != RunStatePending {
		return nil, fmt.Errorf("executor already used (state: %s)", e.state)
	}
	e.state = RunStateRunning

	e.environment = NewRuntimeEnvironment(ctx, e.spec)
	e.environment.Catalog = e.graph.ResourceCatalog().Clone()
	defer func() {
		_ = e.environment.Close()
		e.environment = nil
		e.variables = nil
	}()

	if err := e.bindVariables(e.graph, variables); err != nil {
		e.state = RunStateFailed
		return nil, err
	}

	e.stack = NewRecoveryStack()

	result, err := e.graph.Root().Execute(e.environment.Context, e, e.stack, e.variables)

	if err != nil {
		// Paused execution: state was already set to [RunStatePaused] by [pausePointObserved]; do
		// NOT unwind (the stack is the resume point) and do NOT transition to Failed.
		if errors.Is(err, ErrPaused) {
			return nil, err
		}
		// Unwind in LIFO order so every Action that completed before the failure gets its Compensate
		// companion called; without this, TestCompensation-style rollback never runs.
		if unwindErr := e.stack.Unwind(); unwindErr != nil {
			err = fmt.Errorf("%w; compensation: %w", err, unwindErr)
		}
		e.state = RunStateFailed
		return nil, err
	}

	e.state = RunStateCompleted
	return result, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

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
	e.lastVariables = e.environment.variables

	return nil
}

// pushAuditReceipt builds, stamps, and pushes a receipt at a dispatch exit.
//
// If `complement` is a [Receipt], that receipt becomes the audit-trail entry (stamped with the
// dispatch's err / result / slots / unitID). If `complement` is a [*RecoveryStack] (multi-output
// compensable), it's pushed nested AND a fresh audit-only [*ReceiptBase] is pushed alongside.
// Otherwise a fresh audit-only [*ReceiptBase] is pushed.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] of the dispatching unit; stamped onto the receipt.
//   - `stack`: the recovery stack the receipt pushes onto.
//   - `slots`: the resolved slot snapshot at dispatch time.
//   - `result`: the dispatch's return value, or nil for failure / void.
//   - `complement`: the action's complement return — Receipt, *RecoveryStack, or nil.
//   - `dispatchErr`: the dispatch error, or nil on success.
//   - `actionFullName`: the canonical action name for [Receipt.Commit], or "" for audit-only receipts
//     without a stamped action.
func (e *GraphExecutor) pushAuditReceipt(
	unitID string,
	stack *RecoveryStack,
	slots map[string]any,
	result any,
	complement any,
	dispatchErr error,
	actionFullName string,
) {

	var receipt Receipt

	if c, ok := complement.(Receipt); ok {
		receipt = c
	} else {
		receipt = &ReceiptBase{}
		if c, ok := complement.(*RecoveryStack); ok {
			stack.PushNested(c)
		}
	}

	receipt.SetErr(dispatchErr)
	receipt.SetResult(result)
	receipt.SetSlots(slots)
	receipt.SetUnitID(unitID)
	receipt.SetComplement(complement)

	if actionFullName != "" {
		_ = receipt.Commit(actionFullName)
	}

	_ = stack.Push(receipt)
}

// endregion

// endregion
