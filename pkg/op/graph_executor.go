// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

var (
	// ErrNilGraph is the sentinel error returned by [NewGraphExecutor] and [ResumeExecutor] when the caller
	// passes a nil *Graph. Surfaces through the assert.NonZero precondition; declared here so callers can
	// match the specific shape via errors.Is when they need to distinguish nil-graph from other errors.
	ErrNilGraph = errors.New("expected non-nil Graph")

	// ErrPaused is the sentinel error returned by [GraphExecutor.Run] when the run halted because
	// [GraphExecutor.Pause] was called. The executor's [RunStatus.Phase] is [PhasePaused] on this exit;
	// callers can take a [*Trace] and resume later via [ResumeExecutor].
	ErrPaused = errors.New("execution paused")

	// ErrStopped is the sentinel error returned by [GraphExecutor.Run] when the run halted because
	// [GraphExecutor.Stop] was called. Unlike a pause, Run unwinds (compensating completed work) and lands
	// [PhaseStopped] — a terminal, non-resumable exit; the deliberate-halt terminal is `stopped × healthy × stopped`
	// (or `stopped × compensation_failed` when the unwind itself fails).
	ErrStopped = errors.New("execution stopped")
)

// GraphExecutor executes a planned [*Graph] under a [*RuntimeEnvironmentSpec].
//
// One executor drives one execution. [GraphExecutor.Run] builds a per-run [*RuntimeEnvironment] from the spec, clones
// the graph's planning catalog onto that environment, dispatches the graph, then tears the environment down.
// Each Run gets an independent working catalog while the graph's planning catalog stays pristine — but each Run
// requires a fresh executor; a second [GraphExecutor.Run] call on the same executor returns an error. Reexecution =
// `NewGraphExecutor(graph, spec)` again; resuming from a paused execution rebuilds the executor from a serialized
// [*Trace].
type GraphExecutor struct {

	// graph is the planned graph this executor runs. Set at construction; never replaced.
	graph *Graph

	// spec is the immutable session configuration. The Run builds a fresh [*RuntimeEnvironment] from it.
	spec *RuntimeEnvironmentSpec

	// hooks is the optional lifecycle hook registry. Installed via [GraphExecutor.SetHooks] before Run.
	hooks *HookRegistry

	// status is the executor's current [RunStatus]. Zero value is preparing × healthy; enters [PhaseRunning] at the
	// first dispatch (after the preflight — environment build + variable binding) and reaches a terminal phase
	// ([PhaseCompleted] or [PhaseStopped]) at exit.
	status RunStatus

	// transitions is the run's flips-only transition journal — appended by [GraphExecutor.Transition] on each recorded
	// change of the [Phase] or [Condition] dimension. Projected into [Trace.Transitions]; restored on resume.
	transitions []RunStatusTransition

	// stack is the per-Run [*RecoveryStack] — the audit + compensation ledger of every dispatch. Initialized at the
	// head of [GraphExecutor.Run] and held across the Run so [GraphExecutor.Trace] can project it into a serializable
	// [*Trace] at any moment (including post-Run).
	stack *RecoveryStack

	// control is the run's control plane — commands in (pause / stop), events out (phase / error). Commands issued
	// from any goroutine (via [ControlPlane.Request] / [GraphExecutor.Pause] / [GraphExecutor.Stop]) are drained at
	// each control-point ([GraphExecutor.controlPoint]); events are emitted at lifecycle transitions. A shared
	// pointer: a child executor minted by [GraphExecutor.newChildExecutor] holds the same plane, so a command issued
	// on the root run is observed at every nested subgraph executor's control-point, and every subscriber sees every
	// event. See architecture 2.7.
	control *ControlPlane

	// pendingReaction is the [TransitionPolicy] reaction recorded by [GraphExecutor.Transition] when this executor's
	// condition flips to an aberrant state — the most-severe reaction across the run's flips (continue < pause < stop).
	// The dispatch machinery consumes it at the next control point ([GraphExecutor.applyPendingReaction]), the pause-flag
	// precedent. Unlike `pauseRequested` it is NOT shared with child executors: each boundary reacts to its own flips.
	pendingReaction Reaction

	// environment is the per-Run runtime environment. Set at the head of [GraphExecutor.Run] and cleared (and closed)
	// at the tail. Nil outside a Run. Tests that exercise [GraphExecutor.bindVariables] directly mint an environment here
	// themselves rather than going through Run.
	environment *RuntimeEnvironment

	// variables is the per-Run resolved variable map. Mirrors `environment.variables` for the dispatch's
	// slot-resolution path. Cleared alongside `environment` at Run tail.
	variables map[string]Variable

	// lastVariables is the post-Run snapshot of the resolved variable map, preserved past `environment` / `variables`
	// teardown so post-Run inspection (test harnesses, observability) can read the values without holding onto the
	// runtime environment itself.
	lastVariables map[string]Variable

	// ledgerSnapshot is the resource ledger captured at Run teardown (every outcome — a paused run resumes
	// from it, a completed run's trace records the step-48 content identity), preserved past teardown so
	// [GraphExecutor.Trace] can project it into [Trace.Catalog]. [ResumeExecutor] seeds it from the trace so
	// Run's resume branch can rehydrate the live ledger.
	ledgerSnapshot *ResourceLedgerSnapshot
}

// LastVariables returns a snapshot of the resolved variable map from the most recent [GraphExecutor.Run].
//
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

// NewGraphExecutor returns an executor bound to `graph` and `spec`, in the preparing phase ([PhasePreparing]).
//
// The executor drives a single execution. [GraphExecutor.Run] builds a fresh [*RuntimeEnvironment] from
// `spec`, clones the graph's planning catalog onto it, dispatches the graph, and tears the environment down — so
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
		graph:   graph,
		spec:    spec,
		control: NewControlPlane(),
	}
}

// newChildExecutor mints a child executor that owns `childStack`, for a nested subgraph dispatch.
//
// Per the subgraph-executor-ownership model (phase-8 step 31), every subgraph executes via its own executor that owns
// its recovery stack. The child shares the parent's graph, spec, hooks, runtime environment, variable frame, and
// control plane — it does NOT rebuild the environment, clone the catalog, or rebind variables (those stay [GraphExecutor.Run]'s
// one-time top-of-tree responsibilities). The subgraph's bound action returns `childStack` as its compensator, which the
// parent carries on the dispatch's audit receipt and compensates through the action's Undo companion.
//
// The caller supplies `childStack` already chained to the enclosing stack — `newRecoveryStack(parent)` for a fresh
// dispatch, or, on resume, the subgraph's restored child stack re-parented to the enclosing stack — so a child's
// promise to an upstream producer outside this subgraph resolves up the chain via [RecoveryStack.ResultByUnitID].
//
// Parameters:
//   - `childStack`: the recovery stack the child executor owns; already parented to the enclosing stack.
//
// Returns:
//   - *GraphExecutor: the child executor, in [PhaseRunning], sharing the parent's run-scoped state.
func (e *GraphExecutor) newChildExecutor(childStack *RecoveryStack) *GraphExecutor {

	return &GraphExecutor{
		graph:         e.graph,
		spec:          e.spec,
		hooks:         e.hooks,
		status:        RunStatus{Phase: PhaseRunning},
		stack:         childStack,
		control:       e.control,
		environment:   e.environment,
		variables:     e.variables,
		lastVariables: e.lastVariables,
	}
}

// ResumeExecutor constructs a [*GraphExecutor] ready to continue dispatch from a [*Trace]'s state.
//
// The trace's [Trace.GraphChecksum] must match `graph.Checksum()` — a mismatch indicates the
// graph has changed since the pause and the trace is incompatible. On success the returned
// executor has its [RunStatus], [*RecoveryStack], and resolved variables restored from the trace;
// a subsequent [GraphExecutor.Run] continues dispatch from that point, skipping units whose UnitID
// already appears in the recovery stack with a successful receipt.
//
// Parameters:
//   - `graph`: the planned graph the trace was taken against. Must be non-nil.
//   - `spec`: the session configuration for the resumed execution. Must be non-nil.
//   - `trace`: the captured execution state. Must be non-nil and graph-compatible.
//
// Returns:
//   - *GraphExecutor: the executor ready to resume.
//   - `error`: non-nil on nil arguments or checksum mismatch.
func ResumeExecutor(graph *Graph, spec *RuntimeEnvironmentSpec, trace *Trace) (*GraphExecutor, error) {

	assert.NonZero("graph", graph)
	assert.NonZero("spec", spec)
	assert.NonZero("trace", trace)

	if graph.Checksum() != trace.GraphChecksum {
		return nil, fmt.Errorf("ResumeExecutor: graph checksum mismatch: trace=%q graph=%q",
			trace.GraphChecksum, graph.Checksum())
	}

	e := NewGraphExecutor(graph, spec)
	e.status = trace.RunStatus
	e.transitions = trace.Transitions
	e.stack = trace.Stack
	e.variables = trace.Variables
	e.lastVariables = trace.Variables
	e.ledgerSnapshot = trace.Catalog

	return e, nil
}

// region EXPORTED METHODS

// region State management

// Control returns the run's [*ControlPlane] — the surface a consumer uses to issue commands
// ([ControlPlane.Request]) and subscribe to events ([ControlPlane.Subscribe]).
//
// Returns:
//   - *ControlPlane: the run's control plane; never nil.
func (e *GraphExecutor) Control() *ControlPlane { return e.control }

// Pause requests [PhaseRunning] → [PhasePaused] at the next control-point — a thin convenience over
// [ControlPlane.Request]([ControlPause]).
//
// Pause returns immediately (fire-and-forget); the transition happens on the goroutine driving [GraphExecutor.Run]
// when it next drains the command, at which point Run returns [ErrPaused] with [GraphExecutor.RunStatus] reporting
// [PhasePaused]. If the run terminates before the control-point is reached, the request is silently dropped. A
// caller that wants the acknowledgement issues [ControlPlane.Request] directly and reads the response channel.
//
// Safe to call from any goroutine.
//
// Returns:
//   - `error`: non-nil when the executor is not in [PhaseRunning] (nothing to pause).
func (e *GraphExecutor) Pause() error {

	if e.status.Phase != PhaseRunning {
		return fmt.Errorf("Pause: executor is not running (status: %s)", e.status)
	}
	e.control.Request(ControlPause)
	return nil
}

// Stop requests a halt-and-terminate at the next control-point — a thin convenience over
// [ControlPlane.Request]([ControlStop]).
//
// Stop returns immediately (fire-and-forget); at the next control-point Run unwinds (compensating completed work)
// and lands [PhaseStopped], returning [ErrStopped]. Unlike [GraphExecutor.Pause], a stopped run is **not** resumable
// — the recovery stack is spent by the unwind. A caller that wants the acknowledgement issues [ControlPlane.Request]
// directly and reads the response channel.
//
// Safe to call from any goroutine.
//
// Returns:
//   - `error`: non-nil when the executor is not in [PhaseRunning] (nothing to stop).
func (e *GraphExecutor) Stop() error {

	if e.status.Phase != PhaseRunning {
		return fmt.Errorf("Stop: executor is not running (status: %s)", e.status)
	}
	e.control.Request(ControlStop)
	return nil
}

// SetHooks installs `hooks` as the lifecycle hook registry for every Run.
//
// Parameters:
//   - `hooks`: the hook registry to install.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
}

// ResumeUnwind drives the resumed state-checked unwind of a `stopped × compensation_failed` trace — the
// Restart half of the step-21 contract, and the one sanctioned downward condition move.
//
// Resume is an unwind, NOT a forward retry: the retained recovery journal names the candidate set, and every
// entry's Compensate re-runs against the live filesystem — the framework does not assume the operator unwound
// while clearing the blocker, it observes (an already-cleared state no-ops through the compensators' own
// tolerance; a still-failing compensation fails again). A clean unwind clears the journal and de-escalates the
// run to `stopped × execution_failed` ([ReasonUnwound], journaled) — the clean baseline from which re-running
// forward is a fresh, explicit run. A dirty unwind leaves the run at `stopped × compensation_failed` with the
// fresh diagnostics retained.
//
// The ledger rehydrate and stack re-arm mirror [GraphExecutor.Run]'s resumed branch; the two paths stay
// separate because Run resumes FORWARD execution from a paused trace, which this contract explicitly forbids
// for a compensation-failed one.
//
// Parameters:
//   - `ctx`: the context for the unwind's runtime environment.
//
// Returns:
//   - `error`: non-nil when the executor is not at `stopped × compensation_failed`, when restoring the resumed
//     state fails, or when any compensation fails again (the joined errors).
func (e *GraphExecutor) ResumeUnwind(ctx context.Context) error {

	if e.status.Phase != PhaseStopped || e.status.Condition != ConditionCompensationFailed {
		return fmt.Errorf("ResumeUnwind: executor is not at stopped × compensation_failed (status: %s)", e.status)
	}

	e.environment = NewRuntimeEnvironment(ctx, e.spec.WithCatalog(e.graph.ResourceCatalog().Clone()))
	defer func() {
		if e.environment.ResourceCatalog != nil {
			e.ledgerSnapshot = e.environment.ResourceCatalog.Snapshot()
		}
		//nolint:errcheck // diagnose-ignored-error: cleanup close; see docs/architecture/2.8-eventing-infrastructure.md
		_ = e.environment.Close()
		e.environment = nil
	}()

	if e.ledgerSnapshot != nil {
		restored, rehydrateErr := e.ledgerSnapshot.Rehydrate(e.environment)
		if rehydrateErr != nil {
			return fmt.Errorf("ResumeUnwind: rehydrate resource ledger: %w", rehydrateErr)
		}
		e.environment.ResourceCatalog = restored
	}

	if rearmErr := e.stack.rearm(e.environment); rearmErr != nil {
		return fmt.Errorf("ResumeUnwind: re-arm recovery stack: %w", rearmErr)
	}

	if err := e.stack.Unwind(e.environment); err != nil {
		e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionCompensationFailed,
			Reason: ReasonCompensationFailed, Message: err.Error()}
		return fmt.Errorf("ResumeUnwind: %w", err)
	}

	// The sanctioned de-escalation: set directly — Transition forbids downward moves by design — and journal it.
	e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionExecutionFailed, Reason: ReasonUnwound,
		Message: "resumed state-checked unwind cleared the compensation failure"}
	e.transitions = append(e.transitions, RunStatusTransition{
		Phase:     PhaseStopped,
		Condition: ConditionExecutionFailed,
		At:        time.Now(),
		Reason:    ReasonUnwound,
		Message:   "resumed state-checked unwind cleared the compensation failure",
	})

	return nil
}

// RunStatus returns the executor's current [RunStatus] triplet.
//
// Concurrent-safe to read at any point; the field is mutated only by the goroutine driving
// [GraphExecutor.Run] (and by [GraphExecutor.Pause]'s observation of the pause flag at the next
// pause-point).
//
// Returns:
//   - `RunStatus`: the current phase × condition × reason triplet.
func (e *GraphExecutor) RunStatus() RunStatus {
	return e.status
}

// Transition records a condition flip on this executor's run status and reports whether it was accepted.
//
// The single mutator of the run-status machine and its sole choke point: every condition flip goes through here, so
// none escapes the journal. [Condition] only worsens *within a run* — a request below the current condition is rejected
// (returns a non-nil error) rather than silently clamped, which is how monotonicity is enforced under the submission
// model. The one legal downward move is the resume de-escalation — a resumed run whose state-checked unwind clears a
// `compensation_failed` trace back to `execution_failed` — and it enters through resume ([ResumeExecutor] sets the
// status directly), not through Transition; that full resumed-unwind is step 21. A request equal to the current
// condition is a no-op (flips-only: a repeat driver, e.g. a second flow.Degraded while
// already degraded, is a receipt, not a transition). [Phase] is not an argument — the executor owns phase moves
// (lifecycle events and the policy reaction). `At` is stamped internally. An aberrant flip also records the effective
// [TransitionPolicy] reaction (unit ?? graph ?? floor) as this executor's pending reaction — so the flip and its
// policy reaction are one atomic act at the one choke point.
//
// Parameters:
//   - `unitID`: the unit whose outcome drove the flip; empty for run-level events.
//   - `condition`: the [Condition] being entered — must be at or above the current condition.
//   - `reason`: the [Reason] token classifying the flip.
//   - `message`: free-text detail (typically an err.Error()), carried on the status and the journal entry.
//
// Returns:
//   - `error`: non-nil when the request would de-escalate the condition (rejected); nil when applied or a no-op.
func (e *GraphExecutor) Transition(unitID string, condition Condition, reason Reason, message string) error {

	if condition < e.status.Condition {
		return fmt.Errorf("op: transition rejected — condition may only worsen (have %s, requested %s)",
			e.status.Condition, condition)
	}

	if condition == e.status.Condition {
		return nil
	}

	e.status = RunStatus{Phase: e.status.Phase, Condition: condition, Reason: reason, Message: message}
	e.transitions = append(e.transitions, RunStatusTransition{
		Phase:     e.status.Phase,
		Condition: condition,
		At:        time.Now(),
		UnitID:    unitID,
		Reason:    reason,
		Message:   message,
	})

	// Flip and reaction are one atomic act: consult the effective policy for the entered condition and record the
	// most-severe pending reaction (continue < pause < stop), which the dispatch machinery consumes at the next
	// control point ([GraphExecutor.applyPendingReaction]).
	if reaction := e.transitionPolicyFor(unitID).ReactionFor(condition); reaction > e.pendingReaction {
		e.pendingReaction = reaction
	}

	return nil
}

// Trace projects the executor's current per-run mutable state into a serializable [*Trace].
//
// Pairs with the executor's bound [*Graph] (loaded separately via [LoadGraph]) to fully describe the
// execution. The graph identity is captured as [Trace.GraphChecksum] for resume-time verification.
// Safe to call at any point — before, during, or after [GraphExecutor.Run] — the stack and variables
// fields are nil-safe.
//
// Returns:
//   - *Trace: the captured state.
func (e *GraphExecutor) Trace() *Trace {

	ledger := e.ledgerSnapshot
	if e.environment != nil && e.environment.ResourceCatalog != nil {
		ledger = e.environment.ResourceCatalog.Snapshot()
	}

	return &Trace{
		GraphChecksum: e.graph.Checksum(),
		RunStatus:     e.status,
		Transitions:   e.transitions,
		Stack:         e.stack,
		Variables:     e.variables,
		Catalog:       ledger,
	}
}

// endregion

// region Behaviors

// Fallible actions

// Run dispatches the executor's graph under a fresh per-run [*RuntimeEnvironment].
//
// At every Run:
//
//  1. Build a fresh [*RuntimeEnvironment] from the stored spec, bound to `ctx`.
//  2. Clone `graph.Catalog` onto the new environment's Catalog. The clone is independent — Resources written by
//     this Run cannot reach back into the graph's planning catalog.
//  3. Rebind the graph onto the per-run environment.
//  4. Preflight: [GraphExecutor.bindVariables] resolves the graph's parameter surface against the
//     spec's [application.Application] source maps; caller-supplied `variables` layer on top as the
//     highest-priority source.
//  5. Dispatch through [Graph.dispatch].
//  6. On error, unwind the recovery stack so every successfully-completed Action gets its Compensate
//     companion called.
//  7. Close the env (deferred); clear the transient `e.environment` and `e.variables` fields.
//
// The stop contract: the result and error come through the return; the terminal run status (phase × condition ×
// reason) is read separately via [GraphExecutor.RunStatus] (or [GraphExecutor.Trace]). The error reflects whether the
// run HALTED — a stop, an unhandled or asserted failure, or a pause — while the condition reflects the run's HEALTH,
// so a run whose [TransitionPolicy] continues past a failure returns `(result, nil)` yet reports
// `completed × execution_failed` ("ran to the end despite a failure").
//
// Parameters:
//   - `ctx`: the per-run cancellation context. Its values flow through `RuntimeEnvironment.Context` into
//     providers and subprocesses.
//   - `variables`: caller-supplied variable bindings layered on top of the resolver's output. Pass nil
//     or an empty map for the common case where the resolver alone produces the variable surface.
//
// Returns:
//   - `any`: the final dispatch's result — the terminal node's output value; nil on a failure / pause, or when no node
//     produced output.
//   - `error`: non-nil when the run halts (preflight failure, an unhandled or asserted failure that stopped the run,
//     or a pause via [ErrPaused]); nil when the run completes — including a `completed × degraded` or
//     `completed × execution_failed` outcome the policy continued through.
func (e *GraphExecutor) Run(ctx context.Context, variables map[string]Variable) (any, error) {

	resuming := e.status.Phase == PhasePaused

	if e.status.Phase != PhasePreparing && !resuming {
		return nil, fmt.Errorf("executor already used (state: %s)", e.status)
	}

	e.environment = NewRuntimeEnvironment(ctx, e.spec.WithCatalog(e.graph.ResourceCatalog().Clone()))
	defer func() {
		// Capture the resource ledger before teardown for EVERY outcome, so [GraphExecutor.Trace] (called by the host
		// after Run returns) can project it into [Trace.Catalog]: a paused run's trace becomes resumable, and a
		// completed run's trace records the as-deployed content identity — the ledger's Etag/Digest capture
		// (phase-8 step 48) — that drift attribution reads back.
		if e.environment.ResourceCatalog != nil {
			e.ledgerSnapshot = e.environment.ResourceCatalog.Snapshot()
		}
		//nolint:errcheck // diagnose-ignored-error: cleanup close; see docs/architecture/2.8-eventing-infrastructure.md
		_ = e.environment.Close()
		e.environment = nil
		e.variables = nil
	}()

	if resuming {
		if err := e.prepareResume(); err != nil {
			return nil, err
		}
	} else {
		if err := e.bindVariables(e.graph, variables); err != nil {
			e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionExecutionFailed,
				Reason: ReasonPreflightFailed, Message: "variable binding failed"}
			return nil, err
		}
		e.stack = NewRecoveryStack()
	}

	// The pre-flight resolve pass (phase-8 step 22): plan-time resources were introduced as [Pending]; now that the
	// catalog is live on the per-run environment, verify each participating entry's existence and drive the
	// [Pending] → [Active]/[Gone] transition. Runs for fresh and resumed runs alike, and in dry-run too — the probes
	// are read-only (stat-class).
	e.resolvePendingResources()

	// preparing → running: construction + environment build + variable binding (the preflight) run in
	// [PhasePreparing]; the phase enters [PhaseRunning] here, at the first dispatch. A preflight failure above lands a
	// terminal without the run ever entering running.
	e.status.Phase = PhaseRunning
	e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})

	result, err := e.dispatchWithPolicy(e.environment.Context, e.graph.Root(), e.stack, e.variables)

	if err != nil {
		// Paused execution: a pause-point inside the dispatch chain (possibly in a nested child executor) returned
		// [ErrPaused]. Move this top-level executor to [PhasePaused] so [GraphExecutor.RunStatus] and
		// [GraphExecutor.Trace] report the pause; do NOT unwind (the stack is the resume point) and do NOT move to a
		// terminal phase.
		if errors.Is(err, ErrPaused) {
			e.status.Phase = PhasePaused
			e.status.Reason = ReasonPaused
			e.status.Message = ""
			e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})
			return nil, err
		}

		// Stopped by an operator command ([ErrStopped]): unwind (compensating completed work), then land the
		// deliberate-halt terminal. This is NOT a failure — a clean unwind lands stopped × the condition the run
		// already held (healthy unless it had met trouble) × stopped; only a failed unwind is stopped ×
		// compensation_failed. Distinct from the failure terminal below (execution_failed), which a stop is not.
		if errors.Is(err, ErrStopped) {
			if unwindErr := e.stack.Unwind(e.environment); unwindErr != nil {
				e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionCompensationFailed,
					Reason: ReasonCompensationFailed, Message: "stop unwind failed: compensation error"}
				e.control.emit(ControlEvent{Kind: EventError, Status: e.status, Err: unwindErr})
				return nil, fmt.Errorf("%w; compensation: %w", err, unwindErr)
			}
			e.status = RunStatus{Phase: PhaseStopped, Condition: e.status.Condition,
				Reason: ReasonStopped, Message: "stopped by operator; stack unwound cleanly"}
			e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})
			return nil, err
		}

		return nil, e.failAndUnwind(err)
	}

	e.status.Phase = PhaseCompleted
	e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})
	e.control.emit(ControlEvent{Kind: EventResult, Status: e.status})
	return result, nil
}

// failAndUnwind lands an unhandled dispatch failure: unwind in LIFO order so every Action that
// completed before the failure gets its Compensate companion called, then pick the failure terminal.
//
// The unwind outcome selects between the two failure terminals (phase-8 step 21, the
// compensation-failure contract): a clean unwind means the system is back at its pre-run state
// (stopped × [ConditionExecutionFailed]); any failed Compensate means the system is dirty
// (stopped × [ConditionCompensationFailed]) — the joined error names the forward failure and every
// compensation that failed. A clean unwind lands stopped × the condition the failure implies,
// honoring a worse condition the run already recorded (bubbled up from the failing boundary):
// `conditionForReason` derives the terminal from the reason a dispatchFailure carries, so a
// degraded-driven stop lands stopped × degraded, not × execution_failed.
//
// Parameters:
//   - `err`: the dispatch failure.
//
// Returns:
//   - `error`: the failure (joined with compensation errors when the unwind fails).
func (e *GraphExecutor) failAndUnwind(err error) error {

	if unwindErr := e.stack.Unwind(e.environment); unwindErr != nil {
		e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionCompensationFailed,
			Reason: ReasonCompensationFailed, Message: "unwind failed: compensation error"}
		e.control.emit(ControlEvent{Kind: EventError, Status: e.status, Err: unwindErr})
		return fmt.Errorf("%w; compensation: %w", err, unwindErr)
	}

	reason := failureReason(err)
	condition := conditionForReason(reason)
	if e.status.Condition > condition {
		condition = e.status.Condition
		reason = e.status.Reason
	}
	e.status = RunStatus{Phase: PhaseStopped, Condition: condition, Reason: reason,
		Message: "unhandled failure; stack unwound cleanly"}
	e.control.emit(ControlEvent{Kind: EventError, Status: e.status, Err: err})
	return err
}

// dispatchWithPolicy dispatches `unit` through this executor, retrying per the unit's [RetryPolicy].
//
// The single per-unit dispatch primitive, shared by [GraphExecutor.Run] (which dispatches the root) and
// [ActivationRecord.DispatchChild] (which dispatches a subgraph's children) — so retry is honored uniformly for every
// unit. The effective policy comes from [GraphExecutor.retryPolicyFor] (the step-35 tri-state: explicit wins, a nested
// subgraph defaults to `policies.retry`, everything else to none): a nil result yields one attempt, a non-nil policy
// yields `MaxAttempts + 1`. Between attempts `policy.ComputeDelay(prevAttempt)` backoff is honored via an interruptible
// wait — a cancel mid-backoff returns `ctx.Err()` immediately. A failed attempt that is [ErrPaused] or under a canceled
// context returns immediately without burning further budget.
//
// When a [RetryPolicy] leaves another attempt pending, the unit's [OnRetry] handler (when set) gates it: a truthy
// verdict continues the loop, a falsy verdict vetoes it, and a handler that itself errors or panics ends it
// ([ReasonHandlerFailed]). [OnRetry] never fires without a [RetryPolicy] — there is no retry to gate.
//
// At the terminal failure — retry budget exhausted, or the loop vetoed by [OnRetry] — the failure falls to the unit's
// [OnError] handler via [GraphExecutor.resolveFailure]: a truthy verdict absorbs it (the handler's return becomes the
// unit's result, no flip), a falsy verdict lets the failure stand ([ReasonActionFailed] on exhaustion,
// [ReasonRetryVetoed] on a veto), and a handler that errors or panics fails as [ReasonHandlerFailed]. A standing
// failure surfaces through a [dispatchFailure] carrying the flip [Reason] to the boundary unwind.
//
// Parameters:
//   - `ctx`: the cancellation context for the dispatch and its backoff waits.
//   - `unit`: the unit to dispatch (with retry).
//   - `stack`: the recovery stack the dispatch's compensations push onto.
//   - `variables`: the variable frame in scope for the dispatch.
//
// Returns:
//   - `any`: the unit's result on the succeeding attempt, or an [OnError] handler's return when it absorbs a failure;
//     nil when the failure stands.
//   - `error`: non-nil when the unit's failure stands (not absorbed by [OnError]), or the unit is paused / canceled.
func (e *GraphExecutor) dispatchWithPolicy(
	ctx context.Context,
	unit ExecutableUnit,
	stack *RecoveryStack,
	variables map[string]Variable,
) (any, error) {

	policy := e.retryPolicyFor(unit)
	maxAttempts := 1

	if policy != nil {
		maxAttempts = policy.MaxAttempts + 1
	}

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {

		if err := waitRetryDelay(ctx, policy, attempt); err != nil {
			return nil, err
		}

		result, err := unit.Execute(ctx, e, stack, variables)
		if err == nil {
			// The unit succeeded — but it may have driven a condition flip (a flow.Degraded / flow.Failed driver)
			// whose policy reaction is now pending. Consume it before returning the result.
			return e.applyPendingReaction(result)
		}

		lastErr = err

		if errors.Is(err, ErrPaused) || ctx.Err() != nil {
			return nil, err
		}

		// A hard failure — a framework-dispatch error, or a policy-driven stop (a reaction Stop, including flow.Failed) —
		// bypasses retry and OnError: it is a structural or deliberate assertion, not an incidental failure to absorb.
		if isHardFailure(err) {
			return nil, err
		}

		// A retry is pending only when another attempt remains. OnRetry (when set) gates it: a truthy verdict keeps
		// retrying, a falsy verdict vetoes the loop (the failure then falls to OnError with the retry_vetoed base
		// reason), and a handler that itself fails ends it as handler_failed.
		if attempt+1 < maxAttempts {
			retry, gateResult, gateErr := e.gateRetry(ctx, unit, variables, lastErr)
			if !retry {
				return gateResult, gateErr
			}
		}
	}

	// The retry budget is exhausted; the failure falls to OnError with the objective action_failed base reason.
	return e.resolveFailure(ctx, unit, variables, ReasonActionFailed, lastErr)
}

// waitRetryDelay sleeps the policy's computed backoff before a retry attempt, honoring cancellation.
//
// Parameters:
//   - `ctx`: the cancellation context.
//   - `policy`: the unit's retry policy; nil (or the first attempt) waits nothing.
//   - `attempt`: the zero-based attempt about to run.
//
// Returns:
//   - `error`: the context's error when canceled mid-wait.
func waitRetryDelay(ctx context.Context, policy *RetryPolicy, attempt int) error {

	if attempt == 0 || policy == nil {
		return nil
	}

	delay := policy.ComputeDelay(attempt - 1)
	if delay <= 0 {
		return nil
	}

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// gateRetry consults the unit's OnRetry handler between attempts: no handler (or a truthy verdict)
// keeps retrying; a falsy verdict vetoes the loop and resolves the failure with the retry_vetoed
// base reason; a handler that itself fails ends the dispatch as handler_failed.
//
// Parameters:
//   - `ctx`: the cancellation context.
//   - `unit`: the unit under dispatch.
//   - `variables`: the variable frame in scope.
//   - `lastErr`: the attempt's failure, resolved on a veto.
//
// Returns:
//   - `bool`: true to keep retrying.
//   - `any`: the veto resolution's result, when not retrying.
//   - `error`: the veto resolution's (or handler's) error, when not retrying.
func (e *GraphExecutor) gateRetry(
	ctx context.Context, unit ExecutableUnit, variables map[string]Variable, lastErr error,
) (retry bool, result any, err error) {

	handler := unit.OnRetry()
	if handler == nil {
		return true, nil, nil
	}

	verdict, handlerErr := e.dispatchHandler(ctx, handler, variables)
	if handlerErr != nil {
		return false, nil, &dispatchFailure{reason: ReasonHandlerFailed, cause: handlerErr}
	}

	if !IsTruthy(verdict) {
		result, err := e.resolveFailure(ctx, unit, variables, ReasonRetryVetoed, lastErr)
		return false, result, err
	}

	return true, nil, nil
}

// prepareResume rebuilds the resumed run's state on the fresh environment: [ResumeExecutor] already
// restored e.stack (the recovery tree) and e.variables (the resolved frame) from the trace.
//
// The variables re-publish onto the fresh environment; the saved resource ledger rehydrates so the
// recovery stack's receipt id references resolve against the same generations the pre-pause run
// produced (a fresh clone would lose superseded shadow generations); and the stack re-arms concrete
// receipts against the rehydrated ledger, so a resumed-then-failed run can roll back the pre-pause
// work. A failure lands the preflight-failed terminal.
//
// Returns:
//   - `error`: non-nil when rehydration or re-arming fails.
func (e *GraphExecutor) prepareResume() error {

	e.environment.variables = e.variables

	if e.ledgerSnapshot != nil {
		restored, rehydrateErr := e.ledgerSnapshot.Rehydrate(e.environment)
		if rehydrateErr != nil {
			e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionExecutionFailed,
				Reason: ReasonPreflightFailed, Message: "resource ledger rehydrate failed"}
			return fmt.Errorf("Run: rehydrate resource ledger: %w", rehydrateErr)
		}
		e.environment.ResourceCatalog = restored
	}

	if rearmErr := e.stack.rearm(e.environment); rearmErr != nil {
		e.status = RunStatus{Phase: PhaseStopped, Condition: ConditionExecutionFailed,
			Reason: ReasonPreflightFailed, Message: "recovery stack re-arm failed"}
		return fmt.Errorf("Run: re-arm recovery stack: %w", rearmErr)
	}

	return nil
}

// dispatchHandler runs a failure-handler subgraph ([OnRetry] / [OnError]) on a fresh [RecoveryStack] and returns its
// result, recovering a panic into an error.
//
// The fresh stack is the §6.4 receipts-don't-leak rule: a handler settles its own compensation and its receipts never
// join the parent `stack`, so a veto or absorb leaves the parent saga's ledger untouched. On the handler's own failure
// its fresh stack unwinds before the error propagates, so a partially-applied handler cleans up after itself.
//
// Parameters:
//   - `ctx`: the cancellation context for the handler dispatch.
//   - `handler`: the handler subgraph to run.
//   - `variables`: the variable frame in scope for the handler — the same frame the failed unit saw.
//
// Returns:
//   - `any`: the handler subgraph's result, read for truthiness by the caller.
//   - `error`: non-nil when the handler errors or panics.
func (e *GraphExecutor) dispatchHandler(
	ctx context.Context,
	handler *Subgraph,
	variables map[string]Variable,
) (result any, err error) {

	handlerStack := NewRecoveryStack()

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %v", recovered)
		}
		if err != nil {
			//nolint:errcheck // diagnose-ignored-error: error wins; see docs/architecture/2.8-eventing-infrastructure.md
			_ = handlerStack.Unwind(e.environment)
		}
	}()

	return handler.Execute(ctx, e, handlerStack, variables)
}

// resolveFailure fires the unit's [OnError] handler (when set) at a terminal failure and renders its verdict.
//
// `baseReason` is why the retry loop ended — [ReasonActionFailed] on exhaustion, [ReasonRetryVetoed] on an [OnRetry]
// veto. With no [OnError] handler the failure stands (via [standingFailure]). With one, its return renders the verdict:
//   - truthy ⇒ **absorb**: the pending flip is rejected (never taken, so strict monotonicity is untouched — absorption
//     is a climb-not-taken), and the handler's return becomes the unit's result. No error propagates, so the boundary
//     unwind never fires;
//   - falsy ⇒ the failure stands (via [standingFailure]);
//   - error / panic ⇒ [ReasonHandlerFailed].
//
// The handler runs through [GraphExecutor.dispatchHandler] on a fresh [RecoveryStack] (§6.4 receipts-don't-leak). A
// standing failure returns a [dispatchFailure] carrying the flip [Reason] to the boundary unwind.
//
// Parameters:
//   - `ctx`: the cancellation context for the handler dispatch.
//   - `unit`: the failed unit whose [OnError] handler resolves the failure.
//   - `variables`: the variable frame in scope for the handler.
//   - `baseReason`: the reason to stamp when this unit originates the failure — action_failed or retry_vetoed; a
//     reason a deeper unit already stamped is preserved.
//   - `cause`: the underlying failure the handler adjudicates.
//
// Returns:
//   - `any`: the handler's return on an absorb; nil when the failure stands.
//   - `error`: nil on an absorb; otherwise a [dispatchFailure] carrying `baseReason` (stands) or handler_failed.
func (e *GraphExecutor) resolveFailure(
	ctx context.Context,
	unit ExecutableUnit,
	variables map[string]Variable,
	baseReason Reason,
	cause error,
) (any, error) {

	handler := unit.OnError()
	if handler == nil {
		return nil, standingFailure(baseReason, cause)
	}

	result, handlerErr := e.dispatchHandler(ctx, handler, variables)
	if handlerErr != nil {
		return nil, &dispatchFailure{reason: ReasonHandlerFailed, cause: handlerErr}
	}

	if !IsTruthy(result) {
		return nil, standingFailure(baseReason, cause)
	}

	return result, nil
}

// applyPendingReaction translates this executor's pending [TransitionPolicy] reaction into a dispatch outcome after a
// unit that succeeded but drove a condition flip (a flow.Degraded / flow.Failed driver): Continue yields the result;
// Pause returns an [ErrPaused] join so the run parks run-globally; Stop returns a [dispatchFailure] carrying the
// recorded reason so the boundary unwinds and lands stopped × the recorded condition.
//
// The pause-flag precedent: the reaction is recorded at the [GraphExecutor.Transition] choke point and consumed here,
// the next control point, rather than acted on inside the provider driver (providers never enforce policy).
//
// Parameters:
//   - `result`: the succeeding unit's result, returned unchanged when the reaction is Continue.
//
// Returns:
//   - `any`: `result` on Continue; nil otherwise.
//   - `error`: nil on Continue; an [ErrPaused] join on Pause; a [dispatchFailure] on Stop.
func (e *GraphExecutor) applyPendingReaction(result any) (any, error) {

	switch e.pendingReaction {
	case ReactionStop:
		return nil, &dispatchFailure{reason: e.status.Reason, cause: errors.New(e.status.Message), bypassHandler: true}
	case ReactionPause:
		return nil, errors.Join(ErrPaused, errors.New(e.status.Message))
	default:
		return result, nil
	}
}

// transitionPolicyFor resolves the effective [TransitionPolicy] for a flip driven by `unitID`: the unit's own policy,
// then the graph-level (root) policy, then the builtin floor. The ancestor and app-config layers land later.
//
// Parameters:
//   - `unitID`: the unit whose flip is being reacted to; empty (a run-level event) resolves to graph ?? floor.
//
// Returns:
//   - `TransitionPolicy`: the resolved policy — never a zero value, as the floor backstops every lookup.
func (e *GraphExecutor) transitionPolicyFor(unitID string) TransitionPolicy {

	if unit, ok := e.graph.unitsByID[unitID]; ok && unit != nil {
		if policy := unit.TransitionPolicy(); policy != nil {
			return *policy
		}
	}

	if policy := e.graph.Root().TransitionPolicy(); policy != nil {
		return *policy
	}

	return NewPoliciesConfig().Transition
}

// retryPolicyFor resolves the effective [RetryPolicy] for dispatching `unit` — the step-35 tri-state.
//
// An explicit policy on the unit always wins (`MaxAttempts:0` is a deliberate no-retry). Absent one (nil), the
// default is narrow: only a **structural nested subgraph** — a pure saga boundary bound to `flow.subgraph`, with an
// enclosing scope to roll back up to — inherits the configured `policies.retry` (the builtin floor today; the file /
// env / cli layers land with the config loader, mirroring [GraphExecutor.transitionPolicyFor]). Everything else
// resolves to **none**: the graph root (also `flow.subgraph`, but there is no "up" to roll back to); the flow
// combinators `gather` / `choose` / `wait_until` (which carry their own deliberate failure protocols — `wait_until`
// already polls, so an outer retry would retry its retry); a node (construction stamps an explicit `MaxAttempts:0`,
// so it reaches the explicit-wins branch); and every non-subgraph unit.
//
// Parameters:
//   - `unit`: the unit about to be dispatched.
//
// Returns:
//   - `*RetryPolicy`: the resolved policy, or nil for no retry (one attempt).
func (e *GraphExecutor) retryPolicyFor(unit ExecutableUnit) *RetryPolicy {

	if policy := unit.RetryPolicy(); policy != nil {
		return policy
	}

	// "flow.subgraph" is the structural-subgraph action (op names it as a string — op cannot import the flow
	// provider for the flow.Subgraph const; see graph.go's root construction). The combinators bind flow.gather /
	// flow.choose / flow.wait_until and are deliberately excluded.
	if subgraph, isSubgraph := unit.(*Subgraph); isSubgraph &&
		unit != e.graph.Root() && subgraph.boundActionName() == "flow.subgraph" {
		retry := NewPoliciesConfig().Retry
		return &retry
	}

	return nil
}

// frameworkFailure records a framework-dispatch flip on this executor and returns the error to propagate.
//
// A framework-dispatch failure — no action bound, action-name resolution failure, malformed decision topology — is a
// structural error, not an action's error return. It is journaled here as a [ConditionExecutionFailed] flip
// ([ReasonFrameworkFailed]); the returned [dispatchFailure] carries that reason to the boundary and marks the error so
// the dispatch machinery bypasses retry and OnError (a structural error is not an incidental failure to absorb).
//
// Parameters:
//   - `unitID`: the unit whose dispatch failed structurally.
//   - `cause`: the underlying framework error.
//
// Returns:
//   - `error`: a [dispatchFailure] carrying [ReasonFrameworkFailed].
func (e *GraphExecutor) frameworkFailure(unitID string, cause error) error {

	assert.NoError("transition to framework-failed",
		e.Transition(unitID, ConditionExecutionFailed, ReasonFrameworkFailed, cause.Error()))
	return &dispatchFailure{reason: ReasonFrameworkFailed, cause: cause, bypassHandler: true}
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

// resolvePendingResources drives the discovery-side Pending → Active/Gone transition at pre-flight (phase-8 step 22).
//
// Plan-time resources are introduced into the graph's catalog as [Pending] — deliberately unresolved — and are
// expected to resolve here, once the catalog is cloned onto the per-run [RuntimeEnvironment] and the executor owns it.
// Each [Pending] entry whose type participates in existence verification (see [participatesInExistenceVerification] —
// the staged rollout's gate) is verified through [ResourceCatalog.VerifyExistence], which reads [Resource.Exists] and
// applies the catalog-owned transition.
//
// The pass marks and never fails the run: a [Gone] resource is a recorded fact, not a stop — planning and running
// operations against a not-yet-existent path is legitimate (`file.exists` on a missing file answers false; a producer
// revives a [Gone] URI via shadow), and consumers of a genuinely missing resource fail at their own dispatch (the Q2
// "dependents fail on their own" precedent). The probes are read-only (stat-class), so the pass also runs in dry-run.
func (e *GraphExecutor) resolvePendingResources() {

	catalog := e.environment.ResourceCatalog
	if catalog == nil {
		return
	}

	for _, resource := range catalog.pendingEntries() {
		if !participatesInExistenceVerification(resource) {
			continue
		}
		// Mark, don't fail: VerifyExistence records Gone on a missing resource; the error is informational here.
		//nolint:errcheck // diagnose-ignored-error: records Gone; see docs/architecture/2.8-eventing-infrastructure.md
		_ = catalog.VerifyExistence(resource)
	}
}

// Actions

// controlPoint drains one pending control-plane command before a unit dispatch and reacts to it — the commands-in
// half of the plane, polled by the dispatch chain at each unit.
//
// It is the `switch` the design replaces the pause flag with: a drained [ControlPause] moves the phase to
// [PhasePaused] and returns [ErrPaused] (the caller stops without dispatching further; the stack is preserved as the
// resume point); a [ControlStop] moves the phase to [PhaseStopping] and returns [ErrStopped] (Run then unwinds and
// terminates); an unserved command (e.g. [ControlStep] in this slice) is answered with an error and dispatch
// proceeds. Each command is answered on its own response channel, and a phase move is emitted as an event. No
// pending command returns nil and dispatch continues.
//
// Returns:
//   - `error`: [ErrPaused] on a pause, [ErrStopped] on a stop, or nil to continue dispatch.
func (e *GraphExecutor) controlPoint() error {

	request, pending := e.control.poll()
	if !pending {
		return nil
	}

	switch request.cmd {

	case ControlPause:
		e.status.Phase = PhasePaused
		e.status.Reason = ReasonPaused
		e.status.Message = "at a control-point"
		request.response <- ControlResponse{Status: e.status}
		e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})
		return ErrPaused

	case ControlStop:
		e.status.Phase = PhaseStopping
		e.status.Reason = ReasonStopped
		e.status.Message = "stop requested at a control-point"
		request.response <- ControlResponse{Status: e.status}
		e.control.emit(ControlEvent{Kind: EventPhaseChanged, Status: e.status})
		return ErrStopped

	default:
		request.response <- ControlResponse{Err: fmt.Errorf("control: unsupported command %s", request.cmd)}
		return nil
	}
}

// pushAuditReceipt builds, stamps, and pushes a receipt at a dispatch exit.
//
// A [*RecoveryStack] compensator (a combinator, or a batch action like file.WalkTree / archive.Extract) is stamped
// with the unit's identity + outcome and nested directly ([RecoveryStack.PushNested]) — it is a [RecoveryStack] in the
// tree, rolled back by its own [RecoveryStack.Unwind], with no wrapping receipt and no named companion. Otherwise a
// [Receipt] compensator becomes the audit-trail entry, and a nil compensator gets a fresh bare [*ReceiptBase] entry.
//
// When `action` is non-nil — the dispatch actually ran — [Receipt.Commit] stamps the unit ID, action names,
// result, compensator, and error in one call; `slots` is stamped separately. When `action` is nil (a
// cancellation, pause, or no-Action-bound exit that never dispatched), the receipt is pushed bare with no commit.
//
// Parameters:
//   - `unit`: the dispatching [ExecutableUnit]; supplies the unit ID and action names to [Receipt.Commit].
//   - `stack`: the recovery stack the receipt pushes onto.
//   - `slots`: the resolved slot snapshot at dispatch time.
//   - `result`: the dispatch's return value, or nil for failure / void.
//   - `compensator`: the action's compensator return — Receipt, *RecoveryStack, or nil.
//   - `dispatchErr`: the dispatch error, or nil on success.
//   - `action`: the dispatched [Action], or nil for an audit-only exit that never dispatched (cancellation, pause,
//     or an unbound unit). Nil suppresses the commit — a unit carries an action even when canceled, so this flag,
//     not `unit.Action()`, signals whether the dispatch ran.
//
// Returns:
//   - `error`: non-nil when [Receipt.Commit] fails (its only failure: minting the transaction id). The receipt is
//     still pushed for the audit trail, but its result and compensator were never recorded, so the caller must fail
//     the dispatch rather than report success over an untracked side effect.
func (e *GraphExecutor) pushAuditReceipt(
	unit ExecutableUnit,
	stack *RecoveryStack,
	slots map[string]any,
	result any,
	compensator Compensator,
	dispatchErr error,
	action Action,
) error {

	// A *RecoveryStack compensator (a combinator or batch action) is stamped with the unit's identity/outcome and
	// nested directly, so it is a RecoveryStack in the tree — compensated by its own Unwind, needing no wrapping
	// receipt or named companion (subgraph-direct-push, phase-8 step 42 slice 3a).
	if childStack, ok := compensator.(*RecoveryStack); ok {
		childStack.Stamp(unit.ID(), result, dispatchErr)
		stack.PushNested(childStack)
		return nil
	}

	var receipt Receipt

	if c, ok := compensator.(Receipt); ok {
		receipt = c
	} else {
		receipt = &ReceiptBase{}
	}

	receipt.SetSlots(slots)

	var commitErr error

	if action != nil {
		// A minimal activation carries the caller identity Commit stamps (step 30): the unit's id as the caller,
		// plus the graph so Commit can resolve the unit object for its action-name stamping.
		commitErr = receipt.Commit(
			&ActivationRecord{Graph: e.graph, CallerID: unit.ID()}, result, compensator, dispatchErr)
	}

	stack.Push(receipt)

	if commitErr != nil {
		return fmt.Errorf("commit audit receipt for %s: %w", unit.ID(), commitErr)
	}

	return nil
}

// joinAuditStamp pushes the dispatch-exit audit receipt for a failing exit and joins any stamp failure onto it.
//
// The failure-exit companion of [GraphExecutor.pushAuditReceipt]: the exit already carries `err`, so a commit
// failure (its only failure: minting the transaction id) rides the same returned error rather than masking the
// exit's cause. Success exits call [GraphExecutor.pushAuditReceipt] directly and fail the unit on a stamp failure.
//
// Parameters:
//   - `unit`: the dispatching [ExecutableUnit].
//   - `stack`: the recovery stack the receipt pushes onto.
//   - `slots`: the resolved slot snapshot, or nil when the exit precedes slot resolution.
//   - `compensator`: the action's compensator return, or nil when the dispatch never ran.
//   - `err`: the failing exit's error. Must not be nil.
//   - `action`: the dispatched [Action], or nil for an exit that never dispatched.
//
// Returns:
//   - `error`: `err`, with any stamp failure joined via [errors.Join].
func (e *GraphExecutor) joinAuditStamp(
	unit ExecutableUnit,
	stack *RecoveryStack,
	slots map[string]any,
	compensator Compensator,
	err error,
	action Action,
) error {

	if stampErr := e.pushAuditReceipt(unit, stack, slots, nil, compensator, err, action); stampErr != nil {
		return errors.Join(err, stampErr)
	}

	return err
}

// endregion

// endregion

// region SUPPORTING TYPES

// dispatchFailure carries the [Reason] token a failed dispatch should stamp onto the run status at the boundary
// unwind, overriding the objective [ReasonActionFailed] default. It wraps the underlying cause so `errors.Is` /
// `errors.As` and `%w` unwrapping still reach the original error.
//
// An [OnRetry] veto wraps the action's failure as [ReasonRetryVetoed]; a handler that itself errors or panics wraps
// its own failure as [ReasonHandlerFailed]. A propagating error that carries no dispatchFailure falls back to
// [ReasonActionFailed] — the objective "the action failed" reason.
//
// `bypassHandler` marks a hard failure — a framework-dispatch error, or a policy-driven stop (a reaction Stop, which
// includes flow.Failed's execution_failed assertion). These are not incidental action failures, so they bypass retry
// and OnError rather than being adjudicated ([isHardFailure]). An ordinary action failure leaves it false.
type dispatchFailure struct {
	reason        Reason
	cause         error
	bypassHandler bool
}

// Error returns the wrapped cause's message.
//
// Returns:
//   - `string`: the wrapped cause's `Error()`.
func (f *dispatchFailure) Error() string {
	return f.cause.Error()
}

// Unwrap returns the wrapped cause so `errors.Is` / `errors.As` traverse to the original error.
//
// Returns:
//   - `error`: the wrapped cause.
func (f *dispatchFailure) Unwrap() error {
	return f.cause
}

// failureReason returns the [Reason] a failed dispatch carries, or [ReasonActionFailed] when `err` carries none.
//
// Parameters:
//   - `err`: the error propagating to the boundary unwind.
//
// Returns:
//   - `Reason`: the carried reason, or [ReasonActionFailed] as the objective default.
func failureReason(err error) Reason {

	var failure *dispatchFailure
	if errors.As(err, &failure) {
		return failure.reason
	}

	return ReasonActionFailed
}

// standingFailure returns the error to propagate when a unit's failure stands (no [OnError], or [OnError] falsy).
//
// The reason is stamped once, at the unit that originates the failure. A `cause` that already carries a
// [dispatchFailure] came from a deeper unit and keeps its reason (returned unchanged, so the full wrapped chain and its
// inner reason survive to the boundary — an ancestor's exhaustion never overwrites a child's retry_vetoed); a `cause`
// that carries none is stamped with `baseReason`.
//
// Parameters:
//   - `baseReason`: the reason to stamp when `cause` carries none — action_failed or retry_vetoed.
//   - `cause`: the underlying failure.
//
// Returns:
//   - `error`: `cause` unchanged when it already carries a [dispatchFailure]; otherwise a [dispatchFailure] wrapping
//     it.
func standingFailure(baseReason Reason, cause error) error {

	var existing *dispatchFailure
	if errors.As(cause, &existing) {
		return cause
	}

	return &dispatchFailure{reason: baseReason, cause: cause}
}

// conditionForReason maps a failure [Reason] to the [Condition] it implies, so the boundary unwind lands the right
// terminal condition from the reason a [dispatchFailure] carries.
//
// Parameters:
//   - `reason`: the failure reason.
//
// Returns:
//   - `Condition`: [ConditionDegraded] for a degraded reason, [ConditionCompensationFailed] for a compensation
//     failure, else [ConditionExecutionFailed] — the objective forward-failure default.
func conditionForReason(reason Reason) Condition {

	switch reason {
	case ReasonDegraded:
		return ConditionDegraded
	case ReasonCompensationFailed:
		return ConditionCompensationFailed
	default:
		return ConditionExecutionFailed
	}
}

// isHardFailure reports whether `err` carries a hard-failure [dispatchFailure] — a framework-dispatch error or a
// policy-driven stop (a reaction Stop, including flow.Failed) — that the dispatch machinery bypasses retry and OnError
// for, rather than adjudicating it as an incidental action failure.
//
// Parameters:
//   - `err`: the error to inspect.
//
// Returns:
//   - `bool`: true when `err` (or a wrapped error) is a hard failure.
func isHardFailure(err error) bool {

	var failure *dispatchFailure
	return errors.As(err, &failure) && failure.bypassHandler
}

// existenceVerifiableTypes is the staged-rollout gate for the pre-flight resolve pass (phase-8 step 22), keyed by the
// canonical type id ([Resource.ResourceType]).
//
// Only resource types with real [Resource.Resolve] / [Resource.Exists] implementations participate — the
// [ResourceBase] defaults are loud [assert.Unimplemented] stubs, so the pass must not reach a not-yet-migrated type.
// Each per-type step adds its type as it implements the contract; the gate dissolves once all nine resource-bearing
// providers participate.
var existenceVerifiableTypes = map[string]struct{}{
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file.Regular":      {},
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file.Directory":    {},
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file.SymbolicLink": {},
}

// participatesInExistenceVerification reports whether `resource`'s type is enrolled in the pre-flight resolve pass.
//
// The canonical type id is deliberately read through the sealed base accessor rather than the [Resource.ResourceType]
// interface method: the base field is minted at construction and cannot be shadowed by a concrete type, so the gate
// reads the unforgeable source.
//
// Parameters:
//   - `resource`: the cataloged resource under consideration.
//
// Returns:
//   - `bool`: true when the resource's canonical type id is in [existenceVerifiableTypes].
func participatesInExistenceVerification(resource Resource) bool {

	_, ok := existenceVerifiableTypes[resource.resourceBase().ResourceType()]
	return ok
}

// endregion
