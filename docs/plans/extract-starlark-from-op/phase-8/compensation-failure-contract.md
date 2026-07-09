---
title: "Framework SAGA failure-handling & compensation-failure contract"
status: draft
created: 2026-06-04
updated: 2026-07-06 (terminal renamed again: CompensationFailed -> FailedCompensation, execution_failed -> failed_execution; 2026-07-04: RunState terminal + text serialization landed -- build items 3-4 done)
---

# Framework SAGA failure-handling & compensation-failure contract

## Summary

A uniform, framework-wide contract for what happens when a unit fails, when its failure is handled, and — the
hard case — when **compensation itself fails**. It is provider-agnostic: every provider (file, service, pkg, …)
returns faithful errors and nothing more; the executor (`GraphExecutor.Run` → `RecoveryStack.Unwind`) owns the
entire failure protocol identically for all of them.

This contract was surfaced by the `pkg.Provider` reconciler work (a partially-failed multi-package install) and the
platform Composite model, but it belongs to the executor / terminal-flow layer, not to any provider.
`platform-unification.md` and `pkg-install-reconciler.md` **reference** this contract; they do not restate it.

## Current state — scaffolded, largely unwired

| Primitive | Exists | Wired |
|---|---|---|
| `Trace` (`GraphChecksum` + `RunState` + `*RecoveryStack` + `Variables`) — the journal | ✅ | ✅ (capture) |
| `ResumeExecutor(graph, spec, trace)` — checksum-guarded restart | ✅ | ✅ |
| `RecoveryStack.Unwind()` — LIFO `Compensate`, **best-effort-complete** (all entries attempted, errors joined — R3, `recovery_stack.go:181`); the executor maps its error → `stopped × ConditionCompensationFailed` (landed 2026-07-04; re-expressed as the triplet 2026-07-07) | ✅ | ✅ |
| `RunStatus` = `{Phase, Condition, Reason}` triplet — `Phase` (`preparing` … `completed`/`stopped`) + latched `Condition` (`healthy` < `degraded` < `failed_execution` < `failed_compensation`) + prose `Reason`; the two enum dimensions serialize as their snake names per the GuardResult precedent (the type foundation landed 2026-07-08, superseding the flat enum; `run_state.go`). Identifiers read subject-verb (`ConditionExecutionFailed`) while the serialized names keep the settled `failed_execution` form | ✅ | partial (drivers / journal / policy unwired) |
| `flow.Failed` / `flow.Complete` / `flow.Degraded` terminal nodes | ✅ | ✅ (as value pass-throughs; the state-flip drivers that reach `Transition` are pending) |
| `ExecutableUnit.ErrorAction() *Subgraph` — per-unit failure handler | ✅ | **partial — observation hook only** (corrected 2026-07-05; the 2026-07-04 "never dispatched" note grepped only `pkg/op/*.go`): both flow walkers dispatch an error action once, best-effort, on child failure (`flow/helpers.go:144-150`, `:201-206`) — but it is the **enclosing body's** `error_action`, not the failing unit's own `ErrorAction()`, its own failure is merely logged, and the original error always propagates. The **verdict protocol** (steps 2–3 below) is unbuilt |
| `ConditionDegraded` transition | ✅ (defined) | ❌ **never assigned** (the `flow.Degraded` driver is pending) |
| Distinct terminal for compensation failure (`stopped × ConditionCompensationFailed`) | ✅ | ✅ (landed 2026-07-04: a failed unwind reaches it; two executor tests pin the failed_execution/failed_compensation boundary) |
| Journal persistence on failure + restart-instruction generation | ❌ | ❌ |

## The run-outcome model — four terminals

| Terminal | Meaning | System state | Recovery stack |
|---|---|---|---|
| **Completed** | every unit clean | consistent | — |
| **Degraded** | a unit failed; its `ErrorAction` handled it (reached `flow.Degraded`) | consistent, partial | failures recorded; successes kept |
| **Failed** | a unit failed unhandled; the stack unwound **cleanly** | consistent (pre-run) | fully compensated |
| **FailedCompensation** | unhandled failure **and** unwind itself failed | **dirty** | partially compensated; journal saved |

`Completed`, `Failed`, and `FailedCompensation` (landed 2026-07-04) exist today; `Degraded` is what remains for this contract to wire.

## Protocol — unit failure

When a unit's `Execute` returns an error, the executor proceeds in this fixed order:

1. **Retry** — if the unit carries a `RetryPolicy`, exhaust it first.
2. **Error action — MUST run.** If the unit has an `ErrorAction` subgraph, the executor **must** dispatch it.
   This is a hard guarantee, not best-effort: the failure handler is the consumer's declared control point, so it
   cannot be skipped, short-circuited, or dropped under load. The handler receives the failure context (the error
   and the unit's receipts).
3. **The handler's terminal is the verdict** — determined by which `flow` terminal the `ErrorAction` subgraph
   reaches:
   - **`flow.Degraded(...)`** → the run transitions to `RunStateDegraded` and **execution continues**. The failed
     unit's partial successes are **kept** (not unwound) and its failures are recorded on the `RecoveryStack`. This
     is how a consumer opts a node into best-effort semantics — *put a `flow.Degraded` node in the error action.*
   - **`flow.Complete(output)`** → the failure was *repaired* (e.g. an alternative installed); the run continues
     **clean** (no degrade).
   - **`flow.Failed(...)`**, or the handler errors → **unhandled** → fall to step 4.
4. **No `ErrorAction`, or the handler did not resolve the failure** → the failure is unrecoverable → unwind (next
   section).

Because atomic-vs-best-effort is decided entirely by whether an `ErrorAction` (with `flow.Degraded`) is attached,
it is a **per-node consumer choice**, not a global mode. Omit the handler → unhandled failure → unwind → atomic
rollback. Attach a `flow.Degraded` handler → kept successes + `Degraded` + continue.

## Protocol — unwind, and compensation failure (the core)

An unhandled failure unwinds the `RecoveryStack` in LIFO order, calling each completed action's `Compensate`.

- **Unwind is best-effort-complete.** A `Compensate` that returns an error MUST NOT abort the rest of the unwind.
  The executor records that compensation's failure, continues unwinding the remaining entries, and aggregates all
  compensation errors. (One stuck rollback cannot block the others.)
- **All `Compensate` succeed → `Failed` (consistent).** The system is back at its pre-run state.
- **Any `Compensate` returns an error → `FailedCompensation`.** The system is dirty — a forward action failed
  *and* its undo failed. This terminal is categorically worse than `Failed` and MUST be handled distinctly and
  loudly. The contract is always the same three things:
  1. **Fail loudly.** Surface a distinct terminal (not lumped with clean `Failed`); the error names every
     compensation that failed and why. A silent or generic failure here is a contract violation.
  2. **Save the journal.** Persist the `Trace` — `GraphChecksum`, the terminal state, the `RecoveryStack` with
     per-entry compensation outcomes (compensated / failed / not-yet-reached), and the variables — so the run is
     restartable.
  3. **Emit restart instructions.** Generate troubleshooting + the exact resume command, derived from the trace:
     which node failed, which `Compensate` failed and why, and how to resume once the operator clears the blocker.

## Restart

`ResumeExecutor(graph, spec, trace)` already restores an executor from a `Trace`, refusing a checksum-mismatched
graph. This contract lifts the current "a `Failed` trace is archival, not runnable" restriction **for the
`FailedCompensation` case only**: that trace is persisted as a restartable journal.

Resume is a **state-checked unwind**, not a forward retry. The `RecoveryStack` names the candidate set (units the
run touched and compensations not yet succeeded); the resumed unwind **re-queries each resource's actual state
before acting** and undoes only what is still present — so it is robust to whatever the operator did manually while
clearing the blocker (already-cleared state ⟹ that compensation no-ops). The framework does **not** assume the
operator unwound; it observes. A clean resumed unwind lands the run at `Failed` (clean baseline); it does **not**
auto-retry forward by default (the original forward failure may be unaddressed) — re-running forward is a fresh,
explicit run, with auto-retry-forward available only as an opt-in.

## Hard requirements

- **R1 — Error actions MUST run.** On any unit failure, the unit's `ErrorAction` subgraph is dispatched. Never
  skipped.
- **R2 — A failed `Compensate` MUST produce `FailedCompensation`** with the fail-loud + journal + restart-
  instructions response, uniformly across every provider. `Compensate` returns `error` precisely so this is
  detectable.
- **R3 — Unwind is best-effort-complete** — one failed compensation does not skip the rest; all compensation
  errors are aggregated and reported.
- **R4 — The journal (`Trace`) MUST be persisted on `FailedCompensation`** to enable restart.

## Provider conformance (pkg, file, service, …)

A provider's only obligations: be **best-effort** within a call (attempt every item, collect one receipt each),
return `error` when any item failed (so `ErrorAction`/unwind can act), and return a **faithful per-receipt error**
from `Compensate` when an undo fails. Providers contribute no failure-handling logic of their own. For `pkg`, the
leaf attempts all packages, returns `(receipts, error-if-any-failed)`, and never self-rolls-back — the framework
decides the consequence.

## To build

1. **Dispatch `ErrorAction`** in the executor on unit failure (R1) — currently never invoked.
2. **Transition to `RunStateDegraded`** when an `ErrorAction` reaches `flow.Degraded`; continue execution.
3. ~~**Distinct `FailedCompensation` terminal**~~ — **landed 2026-07-04**: `RunStateFailedCompensation` appended to
   the `RunState` enum; `GraphExecutor.Run` maps a non-nil `Unwind` error to it (clean unwind stays `Failed`); the
   joined error names the forward failure and every failed compensation (the fail-loud half of R2). `RunState` also
   gained text serialization ("failed_compensation") in both document formats per the GuardResult precedent. Two
   executor tests pin the boundary (`TestRun_CompensationFailure_ReachesFailedCompensation`,
   `TestRun_CleanUnwind_ReachesFailed`, `pkg/op/graph_executor_test.go`).
4. ~~**Best-effort-complete unwind** with aggregated compensation errors (R3)~~ — **landed** (`RecoveryStack.Unwind`,
   `recovery_stack.go:181`: all entries attempted LIFO, errors joined; the terminal mapping closed with item 3).
5. **Persist the journal** on `FailedCompensation` (R4) and **generate restart instructions**. R2's remaining half
   (journal + instructions) rides here; the state-checked resume from a `FailedCompensation` trace is this item's
   companion.

## Decided

- **The fourth terminal is `FailedCompensation`** — a peer `RunState` member alongside `Completed` / `Degraded` / `Failed`
  (not a flag on `Failed`). It marks an unhandled failure whose unwind also failed: the system is half-changed and
  needs manual intervention before restart.
- **`Degraded` continues; dependents fail on their own** (Q2) — there is no dependency-aware skipping. `Degraded`
  behaves like PowerShell's `$ErrorActionPreference = 'Continue'`: the failure is recorded, execution proceeds, and
  a downstream unit that needs a failed output fails when *it* runs, then its own error action degrades or
  escalates it. Because failure is **per-package**, this is precise — consumers of a failed package fail while
  consumers of its succeeded siblings proceed; node-level skipping would over-prune.

- **A `Degraded` run exits `0` but is made loud** (Q3) — the author *chose* to degrade (they wrote `flow.Degraded`,
  not `flow.Failed`), and that choice is both the "continue, don't halt" decision and the harm assessment (high-harm
  operations use `flow.Failed` → `Failed` → non-zero). So forward movement is preserved by default — following tasks
  and `&&` chains proceed. "Not ignored" is decoupled from the exit code: a distinct, prominent end-of-run
  **Degraded summary** (which units/packages degraded and why) plus the journaled `Trace`, and the machine-readable
  terminal state `Degraded` on the result so callers branch on *state*, not exit code (the CI warning-annotation
  pattern — non-blocking but visible). An operator whose pipeline must halt on degradation opts in with a strict
  mode (e.g. `--strict`), mapping `Degraded → non-zero`.

- **`FailedCompensation` resumes as a state-checked unwind, not a forward retry** (Q1) — re-query each resource and undo only
  what is still present (robust to operator cleanup; observe, don't assume); land at `Failed` (clean baseline); no
  auto-retry forward by default (opt-in only). Detail in *Restart*.

## Open questions

_All resolved (2026-06-04)._

## Run-state machine — settled 2026-07-05, refined in-session (realization: step 41)

The run state is a **pair of orthogonal dimensions** (settled 2026-07-05, superseding the flat `RunState` enum;
the four-terminal model above is its `stopped` row). Realization chartered as
[step 41](steps/41-run-state-machine.md), which **subsumes build items 1–2 above** (the verdict protocol falls out
of the terminal drivers).

**`Phase` — where the run is:** `preparing` (construction + pre-flight: variable binding, environment build,
catalog clone) → `running` (from the first unit dispatch) → `pausing` → `paused` (resumable; resumes to `running`),
and **two terminal phases** (settled 2026-07-06): **`completed`** — the natural end (the final unit executes, or
`flow.Complete` executes; the result is Complete's input) — and `stopping` → **`stopped`** — the commanded or
policy-driven end (stop command, cancellation, or a `TransitionPolicy` Stop reaction). The transitional forms
(`pausing`, `stopping`) carry the command-requested-but-not-yet-observed gap the control plane needs (step 36).
**Completing is not a State transition** — it means the run is done and State remains exactly as latched: healthy,
degraded, or failed, that's where the run ends.

**`Condition` — the run's health, latching, orthogonal to Phase:** `healthy` → `degraded` → `failed_execution` →
`failed_compensation`. (`failed_execution` named for symmetry with `failed_compensation` — renamed from "failed"
2026-07-05.) The run status is the **triplet `RunStatus{Phase, Condition, Reason}`** (renamed from the pair
`RunState{Phase, State}` 2026-07-07 to kill the `RunState.State` stutter; `Reason` is the prose driver of the latest
move, carried for informative logs). **Go constants (settled 2026-07-06; the health dimension renamed 2026-07-07):**
the snake names above are the serialized forms; the health identifiers read subject-verb for call-site readability —
`ConditionHealthy` / `ConditionDegraded` / `ConditionExecutionFailed` / `ConditionCompensationFailed` (so identifier
and serialized word order deliberately differ: `ConditionExecutionFailed` ⇄ `failed_execution`). Phase constants
follow the same pattern: `PhasePreparing` … `PhaseCompleted`, `PhaseStopped`. The Go name `op.State` was occupied by
the resource lifecycle state (`resource_state.go:21`); that type renamed to `ResourceState` (the type foundation) — the run
health dimension is `Condition`, not `State`.

**Terminals are derived, not enumerated:** the grid is `{completed, stopped} × State`. Notable cells:
`completed × healthy` — the clean run · `completed × degraded` — ran to the end, degraded along the way ·
`completed × failed_execution` — ran to the end despite a failure (exactly what a Continue reaction on
`failed_execution` produces) · `stopped × healthy` — canceled/stopped cleanly before finishing (no longer colliding
with completion) · `stopped × failed_execution` — the default stop-on-failure end · `stopped × failed_compensation`
— compensation failed. `failed_compensation` pairs only with `stopped` (always stop, no configuration).

**State-flip drivers:** `degraded` ⇐ `flow.Degraded` executes (a gate on its input); `failed_execution` ⇐ a saga
boundary exhausts its retry policy, or `flow.Failed` executes; `failed_compensation` ⇐ a compensation action fails.
Completion is a **Phase** event, never a State flip: the last unit executes, or `flow.Complete` executes — the
result is Complete's input (anything) — and State stays as latched.

**Transition-trigger inventory (2026-07-06, additions under discussion).** Confirmed: initial `healthy` at
construction; action error on the final retry → `failed_execution`; `flow.Failed` → `failed_execution`;
compensation error (any unwind) → `failed_compensation`; `flow.Degraded` → `degraded`; final-unit success /
`flow.Complete` → Phase `completed`, State unchanged (settled 2026-07-06 — resolves the former
cancel-vs-completed collision via the two terminal phases). All four additions below **confirmed 2026-07-07** (to be
wired in step 41):

1. **Bubble-up latching** — the parent's State flips from a child subgraph's returned terminal (degraded latches
   unconditionally; a child's `failed_execution` flips the parent only after ErrorAction adjudication).
2. **Pre-flight errors during `preparing`** → `failed_execution` (variable binding, catalog rehydrate/re-arm).
3. **Framework dispatch errors that are not action returns** → `failed_execution` (action-name resolution failure,
   malformed decision topology at runtime).
4. **The resume de-escalation** — the one legal downward transition: resuming a `failed_compensation` trace whose
   state-checked unwind completes cleanly lands `failed_execution` (the latch rule reads "monotonic within a
   run").

**Flip reaction is a three-way policy (noted 2026-07-05):** on each aberrant flip the run **continues, pauses, or
stops** by an act of configuration — defaults to be settled in the configuration discussion (open question 3;
the earlier defaults stand as the working baseline: degraded → continue, failed_execution → stop).
`failed_compensation` stays outside the policy: **always stop, no configuration** (re-confirm at question 3).

**Stop contract:** the run returns the final action's result and error, plus the terminal run state (phase +
state).

**Verdict unification:** an error-action handler expresses its verdict by *which flow terminal executes inside it*
— `flow.Degraded` degrades the run, `flow.Complete` repairs, `flow.Failed` fails — the same driver rules as
anywhere else in the graph. Protocol steps 2–3 above stop being a special mechanism.

**Trace transition journal:** the `Trace` gains a transition journal — `{Phase, Condition, At time.Time,
UnitID string, Reason string}` per flip of either dimension, written by a single recording setter so no flip goes
unjournaled; the
latched triplet stays as the O(1) answer. "When did the run flip to degraded?" and "where did it flip to
failed_execution?" become direct reads; per-event detail (every degradation, every failure) stays on the receipts,
cross-referenced by `UnitID` (ReceiptBase's UUIDv7 transaction IDs already carry issue time). **Flips-only —
settled 2026-07-06 (Q1):** the journal records actual state changes; a second `flow.Degraded` while already
degraded is a receipt, not a transition.

## Policy enforcement and bubble-up — Q4 mechanism settled 2026-07-05

**Home: the ActivationRecord.** The run-state info rides the per-dispatch frame the framework already injects
first (`firstParamIsActivation`); the record already carries the child executor's stack, and
`ActivationRecord.DispatchChild` already owns the RetryPolicy loop — the state cell rides the same conduit. **All
flow provider methods accept an activation record.** Signature consequence: the combinators (`Subgraph`, `Gather`,
`WaitUntil`) already do; the three terminals do not (`Complete(output)`, `Degraded(format, args, kwargs)`,
`Failed(format, args, kwargs)`) — they gain it, which is what makes them state-flip drivers instead of value
pass-throughs (step 41 work item).

**The graph executor enforces policy; providers never do** (consistent with *Provider conformance* above). Two
policies of interest: `RetryPolicy` and a new transition policy — working name **`TransitionPolicy`** (user pick
pending; the placeholder was `StateTransitionPolicy`; anything around "ErrorAction" is out — that word is taken by
the handler subgraph). Shape: a map from entered State to `Reaction`, `Reaction ∈ {Continue, Pause, Stop}`.

**Prior art.** PowerShell `$ErrorActionPreference` (the prompt for this design): its `Ignore` / `SilentlyContinue`
collapse into our `Continue` because observability here is unconditional (receipts + journal always record; "not
ignored" is decoupled from control flow per Q3's exit-0-but-loud decision); its `Inquire` is subsumed by `Pause`
(inquire is a synchronous prompt; pause is a resumable held state inspected through the control plane, step 36);
PowerShell *workflows*' `Suspend` is the closest precedent for pause — and it only existed in their workflow
engine. Beyond PowerShell: **Erlang/OTP supervision trees** (the strongest bubble-up analog — each supervisor
absorbs or escalates child failures per its own strategy, escalation walks the tree with per-level policy); **AWS
Step Functions** per-state `Retry`/`Catch` (exactly the RetryPolicy + handler split); **Ansible**
(`ignore_errors`, `any_errors_fatal`, `block/rescue/always`); **Terraform** provisioner `on_failure =
continue|fail`; **GitHub Actions** `continue-on-error` + `if: failure()/always()`; older lineage: Make `-k`, shell
`set -e`, SQL*Plus `WHENEVER SQLERROR CONTINUE|EXIT`, JCL `COND=`.

**The interaction — four mechanisms, strictly layered, one job each:**

1. **RetryPolicy suppresses** (innermost; already wired at `DispatchChild`): a failure cured by retry never
   becomes anything — no flip, no policy consultation; attempt history lives on the receipt.
2. **ErrorAction adjudicates**: on an exhausted failure the executor dispatches the unit's handler; the verdict is
   which flow terminal executes inside it — `flow.Complete` → repaired (handler's output stands as the unit's
   result, no flip); `flow.Degraded` → State flips `degraded`; `flow.Failed` / handler error / no handler →
   `failed_execution`.
3. **Condition records**: the flip lands on the owning subgraph executor's Phase × Condition cell through the single
   recording setter, which writes the journal entry.
4. **TransitionPolicy reacts** (outermost; executor-enforced): the same setter consults the policy for the entered
   state — `Continue` keeps walking; `Pause` runs the existing pause machinery (Phase → `pausing` → `paused`,
   resumable); `Stop` runs Phase → `stopping`, unwinds per this contract, lands `stopped × state`.
   `failed_compensation` stays outside the policy: always stop.

Flip and reaction share one choke point, so they are atomic and journaled together — no flip escapes the journal
or the policy.

**Bubble-up data flow (corrected 2026-07-06 — the executor tree is the channel).** Phase × Condition never travels
through method returns: the dispatch chain has provider-shaped signatures end to end (`Action.Do` returns
`(Result, Complement, error)`, `unit.Execute` returns `(any, error)`, a compensable provider method returns
`(product, receipt, error)` — and the flow combinators ARE provider methods), so there is no slot for a status triplet
and none is added. Instead, the read mirrors how the host learns the run's terminal state today (`Run` returns
`(any, error)`; the host reads `executor.RunStatus()` afterward), one level down:

1. **The parent already holds the child's executor** — `Subgraph.Execute` creates it
   (`executor.newChildExecutor(childStack)`) before dispatching the body; under step 31's model the child
   executor's Phase × Condition cell is the authoritative record of how that boundary ended.
2. **Dispatch returns `(result, err)` exactly as today; the parent then reads the child executor's latched
   terminal Phase × Condition through the handle it already holds** and runs layers 1–4 at *its* level: adjudicates
   before latching (its RetryPolicy on the child unit, the child unit's ErrorAction, its own TransitionPolicy — a
   repair verdict absorbs the child's `failed_execution`, the parent never latches it), latches degradation
   unconditionally by max-severity (`healthy < degraded < failed_execution < failed_compensation`; a mark, not
   control flow — "dependents fail on their own", Q2 decision above), and journals provenance (the parent's
   transition entry names the child subgraph in `UnitID` with a bubbled-from reason).
3. **The serialized form rides the receipt.** The subgraph's audit receipt on the parent stack — which already
   carries the child stack — additionally records the child's terminal Phase × Condition. Live control flow reads the
   executor handle; the durable record (trace-as-tree, resume) reads the receipt. Two consumers, each from its
   natural surface.

**The single recording setter is `Transition`, on the executor** — the choke point where the flip, the journal
entry, and the TransitionPolicy consultation are one atomic act:

```go
// Transition latches this executor's run status to (phase, condition, reason), appends the journal entry, and
// returns the TransitionPolicy reaction for the entered condition.
//
// The single choke point of the run-state machine: every flip of either dimension goes through here, so no flip
// escapes the journal or the policy. `At` is stamped internally; a non-aberrant transition (phase movement,
// healthy condition) returns ReactionContinue. Condition moves are monotonic within a run (the latch); the sole
// legal downward move is the resume de-escalation, which enters through resume, not through Transition.
//
// Parameters:
//   - `unitID`: the unit whose outcome drove the flip; empty for run-level events (a pause command, pre-flight).
//   - `phase`: the [Phase] being entered (pass the current phase when only Condition flips).
//   - `condition`: the [Condition] being entered (pass the current condition when only Phase flips).
//   - `reason`: the driver, as prose — "flow.degraded executed", "retry budget exhausted (3 attempts)",
//     "compensate failed: …", "pause requested".
//
// Returns:
//   - `Reaction`: the configured reaction for the entered condition — executor-side call sites act on it directly
//     (continue the walk, initiate pausing, initiate stopping); provider-side drivers discard it (the executor
//     records the pending reaction at this choke point and its dispatch machinery acts at the next control
//     point — the pause-flag precedent).
func (e *GraphExecutor) Transition(unitID string, phase Phase, condition Condition, reason string) Reaction
```

Flip drivers reach it through the frame (the Q4 ruling): the ActivationRecord carries run-state info *downward* —
flow methods and the drivers call their **own boundary's** `Transition` via the record's `Transition` delegate (the
executor stays private on the record). What was
considered and rejected as a side channel is the *upward* direction: a child writing its verdict into its
activation frame for the parent to fish out would invert adjudication ownership and leave the read point ambiguous.
The executor handle has neither problem: valid exactly when dispatch returns, and it is the parent's own object.
Downward through the frame to `Transition`; upward only by the parent reading the child executor it created.

**The worked driver — `flow.Provider.Degraded` (settled 2026-07-06):**

```go
// pkg/op/provider/flow — the Degraded driver under step 41 (gains the activation record per Q4):
func (p *Provider) Degraded(
    activationRecord *op.ActivationRecord,
    format string,
    args []any,
    kwargs map[string]any,
) string {

    rendered := op.RenderError(format, args, kwargs)

    // The condition flip: a Condition-only transition on the OWNING boundary, reached through the activation's
    // Transition delegate (the executor itself is never exposed). Phase passes through unchanged.
    activationRecord.Transition(
        activationRecord.Unit.ID(),                  // who — this flow.degraded node
        activationRecord.RunStatus().Phase,          // phase — unchanged (Condition-only flip)
        op.ConditionDegraded,                        // condition — the flip
        "flow.degraded executed: "+rendered.Error(), // why — the rendered gate message
    )

    return rendered.Error()
}
```

Three things the call embodies: (1) **the boundary is reachable through the frame** — `ActivationRecord` gains a
`Transition(...)` method (the sole mutator, delegating to the boundary executor's `Transition`) and a `RunStatus()`
accessor (a read-only copy of the current phase × condition). The executor itself stays **private** on the record, so
a dispatched provider's entire run-status surface is read (`RunStatus`) plus the sanctioned mutate (`Transition`) —
nothing else. Both resolve against the boundary's own executor — the one `Node.Execute` / `Subgraph.Execute` stamp
when they build the record; (2) **the returned `Reaction` is deliberately discarded** — providers never
enforce policy, so the driver doesn't act on continue/pause/stop; `Transition` records the pending reaction and
the executor's dispatch machinery observes it at the next control point (the pause-flag precedent), while
executor-side call sites (retry exhaustion, compensation failure, bubble-up latching) act on the return directly;
(3) **the unit ID is the `flow.degraded` node itself**, so the journal's "where did the run flip to degraded"
points at the gate that fired, and that node's receipt carries the full context. `flow.Failed` and `flow.Complete`
follow the same shape (`ConditionExecutionFailed` for Failed; a Phase-only move for Complete — completion never
flips Condition).

The recursion is uniform — each subgraph executor is a supervision node: adjudicate what your children's executors
report, latch what can't be absorbed, consult your own TransitionPolicy, and let your creator read your latched
state in turn. The root's version is the host's `Run` + `RunStatus()` read; the stop contract's
`(result, error, terminal state)` is conceptual — result and error via the return, the terminal triplet via the
executor.

## TransitionPolicy — Q3 settled 2026-07-06

**The name is `TransitionPolicy`** (settled by use). **The default reactions (the builtin floor):**

| Condition flip | Floor | Rationale |
|---|---|---|
| `ConditionDegraded` | **continue** (in the degraded condition) | the author chose to degrade |
| `ConditionExecutionFailed` | **stop** — at the saga boundary, with an error return to the parent | the floor must suit unattended execution (writ/lore headless, devlore-test in CI); stop delivers the consistent pre-run state |
| `ConditionCompensationFailed` | **stop** — at the saga boundary, with an error return to the parent | same, plus journal + restart instructions |

**Pause is the attended-mode override** for both failure states — its value is preserving the failure scene (no
unwind for `failed_execution`; the dirty residue held for inspection after the best-effort unwind for
`failed_compensation`); its cost is an attendant. The config layering expresses it naturally: `base` says stop; an
interactive application or a `dev` profile flips `failed_execution: pause` in its own layer.

**`failed_compensation` re-enters the policy** (revising the earlier always-stop ruling) with one hard constraint:
**`continue` is never legal for `failed_compensation`** — you cannot walk on past a dirty unwind. The section's
`Validate()` enforces it. Pause there applies *after* the best-effort unwind completes (R3 is never interrupted);
both stop and pause keep the journal.

**Stop is boundary-local; pause is run-global.** Stop at a boundary unwinds that boundary's stack, lands it
`stopped × <condition>`, and returns `(nil, error, terminal status)` to the parent — where bubble-up adjudication runs
(the parent's retry, the unit's ErrorAction, the parent's own TransitionPolicy). The run ends only if the failure
escalates unabsorbed through every ancestor. Pause parks the whole run, resumable.

**Error reporting:** on stop, the boundary's error (wrapped with the boundary identity; compensation errors joined)
travels the `(result, error, terminal Phase × Condition)` triple — the state rides machine-readably, the error stays
prose; the journal answers when/where, receipts carry detail. On a failure-driven pause the return is
`errors.Join(ErrPaused, cause)` — the host sees "resumable" and "why" — and the journal entry
(`{Phase: paused, Condition: failed_execution, UnitID, Reason}`) is the authoritative record, so a failure-pause never
masquerades as an operator pause.

**Configuration home (Q3's structural half):** one op-owned section roots every executor-enforced policy,
following the `runtime` section precedent (`RuntimeEnvironmentConfig` — builtin floor, announced at init, read via
`Application.Config`):

```go
// pkg/op — announced at init() with its builtin floor.
type PoliciesConfig struct {
    devconfig.SectionBase                   // path: "policies"
    Retry      RetryPolicy      `yaml:"retry"`      // the DEFAULT policy for subgraph combinators (step 35)
    Transition TransitionPolicy `yaml:"transition"`
}

type TransitionPolicy struct {
    Degraded           Reaction `yaml:"degraded"`            // floor: continue
    ExecutionFailed    Reaction `yaml:"failed_execution"`    // floor: stop (Go field reads subject-verb; yaml keeps failed_execution)
    CompensationFailed Reaction `yaml:"failed_compensation"` // floor: stop; "continue" rejected by Validate
}

type Reaction int // ReactionContinue / ReactionPause / ReactionStop — serialized "continue" / "pause" / "stop"
```

The configured `Retry` is a plain `op.RetryPolicy` (not a separate defaults type). **Retry-policy assignment is the
step-35 tri-state:** *none* — explicit `MaxAttempts: 0`, fail immediately; *default* — unset/nil, resolving
per-type: a **subgraph combinator** resolves to the configured `policies.retry`; **every other executable unit
resolves to none** (no retry — fail fast, the boundary decides); *specific* — an explicit policy on the unit wins
outright.

Across the layers, an application's policy configuration defines the defaults for any graph it executes
(`base < profiles.<active> < applications.<app>`, e.g. `applications.lore.policies.transition.failed_execution:
pause` for an interactive app). **Plan-time read/override** rides the proven reserved-kwarg machinery:
`transition_policy=` becomes the sibling of `retry_policy=` — legal on any `plan.*` call (unit-level) and on
`plan.assemble_definition` (graph-level) — stamped onto the unit, serialized in the graph document beside `retry`.
Unset means inherit. **Resolution at execution, per boundary, at the flip choke point:**

```
unit.TransitionPolicy  ??  nearest ancestor's  ??  graph's  ??  Application.Config "policies"  ??  builtin floor
```

**`flow.Complete` is an early return — settled 2026-07-06 (Q4 residuals):** `flow.Complete` should be viewed, and
behaves, as an **early return from a subgraph combinator — like a `return` statement in a func**. It ends the body
it executes in with its input as that body's result (at the root: run completion; in an error action: the repair
verdict; in a gather iteration: that iteration completes). As with a function return: units never dispatched get
**no receipts** (absence is the record; the journal's phase entry explains why the walk ended), and everything the
body already did is **kept** — it is a success return, nothing unwinds.

**Still open:** none — the four proposed transition-trigger additions (bubble-up latching, preparing-phase errors,
framework dispatch errors, the resume de-escalation) were **confirmed 2026-07-07** and are wired in step 41.

## Relationships

- Pairs with `terminal-flow-control` (owns `Complete`/`Degraded`/`Failed` terminal semantics).
- Referenced by `platform-unification.md` and `pkg-install-reconciler.md` (providers conform; they do not restate).
- Realization of the state machine + journal: [step 41](steps/41-run-state-machine.md); retry semantics: steps 31
  (boundary mechanics) and 35 (tri-state defaults); `stopped`: step 36.
