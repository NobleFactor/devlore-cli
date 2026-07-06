---
title: "Framework SAGA failure-handling & compensation-failure contract"
status: draft
created: 2026-06-04
updated: 2026-07-04 (terminal renamed CompensationFailed; RunStateCompensationFailed + text serialization landed -- build items 3-4 done)
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
| `RecoveryStack.Unwind()` — LIFO `Compensate`, **best-effort-complete** (all entries attempted, errors joined — R3, `recovery_stack.go:181`); the executor maps its error → `RunStateCompensationFailed` (landed 2026-07-04) | ✅ | ✅ |
| `RunState{Pending,Running,Paused,Degraded,Completed,Failed,CompensationFailed}` — serialized as text ("failed" / "compensation_failed") in both document formats per the GuardResult precedent (2026-07-04) | ✅ | partial (`Degraded` still never assigned) |
| `flow.Failed` / `flow.Complete` / `flow.Degraded` terminal nodes | ✅ | ✅ |
| `ExecutableUnit.ErrorAction() *Subgraph` — per-unit failure handler | ✅ | **partial — observation hook only** (corrected 2026-07-05; the 2026-07-04 "never dispatched" note grepped only `pkg/op/*.go`): both flow walkers dispatch an error action once, best-effort, on child failure (`flow/helpers.go:144-150`, `:201-206`) — but it is the **enclosing body's** `error_action`, not the failing unit's own `ErrorAction()`, its own failure is merely logged, and the original error always propagates. The **verdict protocol** (steps 2–3 below) is unbuilt |
| `RunStateDegraded` transition | ✅ (defined) | ❌ **never assigned** (re-verified 2026-07-04) |
| Distinct terminal for compensation failure (`RunStateCompensationFailed`) | ✅ | ✅ (landed 2026-07-04: a failed unwind reaches it; two executor tests pin the Failed/CompensationFailed boundary) |
| Journal persistence on failure + restart-instruction generation | ❌ | ❌ |

## The run-outcome model — four terminals

| Terminal | Meaning | System state | Recovery stack |
|---|---|---|---|
| **Completed** | every unit clean | consistent | — |
| **Degraded** | a unit failed; its `ErrorAction` handled it (reached `flow.Degraded`) | consistent, partial | failures recorded; successes kept |
| **Failed** | a unit failed unhandled; the stack unwound **cleanly** | consistent (pre-run) | fully compensated |
| **CompensationFailed** | unhandled failure **and** unwind itself failed | **dirty** | partially compensated; journal saved |

`Completed`, `Failed`, and `CompensationFailed` (landed 2026-07-04) exist today; `Degraded` is what remains for this contract to wire.

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
- **Any `Compensate` returns an error → `CompensationFailed`.** The system is dirty — a forward action failed
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
`CompensationFailed` case only**: that trace is persisted as a restartable journal.

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
- **R2 — A failed `Compensate` MUST produce `CompensationFailed`** with the fail-loud + journal + restart-
  instructions response, uniformly across every provider. `Compensate` returns `error` precisely so this is
  detectable.
- **R3 — Unwind is best-effort-complete** — one failed compensation does not skip the rest; all compensation
  errors are aggregated and reported.
- **R4 — The journal (`Trace`) MUST be persisted on `CompensationFailed`** to enable restart.

## Provider conformance (pkg, file, service, …)

A provider's only obligations: be **best-effort** within a call (attempt every item, collect one receipt each),
return `error` when any item failed (so `ErrorAction`/unwind can act), and return a **faithful per-receipt error**
from `Compensate` when an undo fails. Providers contribute no failure-handling logic of their own. For `pkg`, the
leaf attempts all packages, returns `(receipts, error-if-any-failed)`, and never self-rolls-back — the framework
decides the consequence.

## To build

1. **Dispatch `ErrorAction`** in the executor on unit failure (R1) — currently never invoked.
2. **Transition to `RunStateDegraded`** when an `ErrorAction` reaches `flow.Degraded`; continue execution.
3. ~~**Distinct `CompensationFailed` terminal**~~ — **landed 2026-07-04**: `RunStateCompensationFailed` appended to
   the `RunState` enum; `GraphExecutor.Run` maps a non-nil `Unwind` error to it (clean unwind stays `Failed`); the
   joined error names the forward failure and every failed compensation (the fail-loud half of R2). `RunState` also
   gained text serialization ("compensation_failed") in both document formats per the GuardResult precedent. Two
   executor tests pin the boundary (`TestRun_CompensationFailure_ReachesCompensationFailed`,
   `TestRun_CleanUnwind_ReachesFailed`, `pkg/op/graph_executor_test.go`).
4. ~~**Best-effort-complete unwind** with aggregated compensation errors (R3)~~ — **landed** (`RecoveryStack.Unwind`,
   `recovery_stack.go:181`: all entries attempted LIFO, errors joined; the terminal mapping closed with item 3).
5. **Persist the journal** on `CompensationFailed` (R4) and **generate restart instructions**. R2's remaining half
   (journal + instructions) rides here; the state-checked resume from a `CompensationFailed` trace is this item's
   companion.

## Decided

- **The fourth terminal is `CompensationFailed`** — a peer `RunState` member alongside `Completed` / `Degraded` / `Failed`
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

- **`CompensationFailed` resumes as a state-checked unwind, not a forward retry** (Q1) — re-query each resource and undo only
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
catalog clone) → `running` (from the first unit dispatch) → `pausing` → `paused` (resumable; resumes to `running`)
and `stopping` → `stopped` (the sole terminal phase). The transitional forms (`pausing`, `stopping`) carry the
command-requested-but-not-yet-observed gap the control plane needs (step 36).

**`State` — the run's health, latching, orthogonal to Phase:** `healthy` → `degraded` → `execution_failed` →
`compensation_failed`. (`execution_failed` named for symmetry with `compensation_failed` — renamed from "failed"
2026-07-05.) `RunState` becomes **Phase and State**. The Go name `op.State` is currently occupied by the resource
lifecycle state (`resource_state.go:21`); that type renames to `ResourceState` (its file already says so) — folded
into step 41.

**Terminals are derived, not enumerated:** `stopped × healthy` = completed · `stopped × degraded` = degraded ·
`stopped × execution_failed` = execution failed · `stopped × compensation_failed` = compensation failed.

**State-flip drivers:** `degraded` ⇐ `flow.Degraded` executes (a gate on its input); `execution_failed` ⇐ a saga
boundary exhausts its retry policy, or `flow.Failed` executes; `compensation_failed` ⇐ a compensation action fails.
Completion: the last unit executes, or `flow.Complete` executes — the result is Complete's input (anything).

**Flip reaction is a three-way policy (noted 2026-07-05):** on each aberrant flip the run **continues, pauses, or
stops** by an act of configuration — defaults to be settled in the configuration discussion (open question 3;
the earlier defaults stand as the working baseline: degraded → continue, execution_failed → stop).
`compensation_failed` stays outside the policy: **always stop, no configuration** (re-confirm at question 3).

**Stop contract:** the run returns the final action's result and error, plus the terminal run state (phase +
state).

**Verdict unification:** an error-action handler expresses its verdict by *which flow terminal executes inside it*
— `flow.Degraded` degrades the run, `flow.Complete` repairs, `flow.Failed` fails — the same driver rules as
anywhere else in the graph. Protocol steps 2–3 above stop being a special mechanism.

**Trace transition journal:** the `Trace` gains a transition journal — `{Phase, State, At time.Time, UnitID string,
Reason string}` per flip of either dimension, written by a single recording setter so no flip goes unjournaled; the
latched pair stays as the O(1) answer. "When did the run flip to degraded?" and "where did it flip to
execution_failed?" become direct reads; per-event detail (every degradation, every failure) stays on the receipts,
cross-referenced by `UnitID` (ReceiptBase's UUIDv7 transaction IDs already carry issue time). *Proposed,
confirmation pending (open question 1): flips-only in the journal — a second `flow.Degraded` while already degraded
is a receipt, not a transition.*

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
   `execution_failed`.
3. **State records**: the flip lands on the owning subgraph executor's Phase × State cell through the single
   recording setter, which writes the journal entry.
4. **TransitionPolicy reacts** (outermost; executor-enforced): the same setter consults the policy for the entered
   state — `Continue` keeps walking; `Pause` runs the existing pause machinery (Phase → `pausing` → `paused`,
   resumable); `Stop` runs Phase → `stopping`, unwinds per this contract, lands `stopped × state`.
   `compensation_failed` stays outside the policy: always stop.

Flip and reaction share one choke point, so they are atomic and journaled together — no flip escapes the journal
or the policy.

**Bubble-up data flow.** The stop contract IS the bubble-up datum: every subgraph dispatch returns
`(result, error, terminal Phase × State)` to its parent's walk. The parent:

1. **Adjudicates before latching** — a child terminal of `stopped × execution_failed` arrives as a failure at the
   parent's walk, which runs layers 1–4 at *its* level (its RetryPolicy on the child unit, the child unit's
   ErrorAction, its own TransitionPolicy). A repair verdict absorbs the child's failure — the parent never latches
   it.
2. **Latches degradation unconditionally** — a child ending `stopped × degraded` succeeded; nothing to adjudicate;
   the parent's State latches by max-severity (`healthy < degraded < execution_failed < compensation_failed`).
   Degradation propagates as a mark, not as control flow ("dependents fail on their own", Q2 decision above).
3. **Journals provenance** — the parent's transition entry names the child subgraph in `UnitID` with a
   bubbled-from reason; step 31's trace-as-tree-of-subtraces means each level journals its own flips and the root
   journal reads as the run's story.

The recursion is uniform — each subgraph executor is a supervision node: adjudicate what your children hand you,
latch what can't be absorbed, consult your own TransitionPolicy, hand `(result, error, state)` upward. The root's
handoff is the run's return to the host.

**Still open:** question 3 — where `TransitionPolicy` is authored, its names, and the per-flip default reactions
(the RetryPolicy tri-state shape, step 35, is the candidate to evaluate); question 1 — journal granularity
(flips-only proposed); the `TransitionPolicy` name pick; and Q4's residual sub-questions (`flow.Complete`
early-exit receipts for the never-dispatched remainder — proposed none; side effects kept — proposed yes).

## Relationships

- Pairs with `terminal-flow-control` (owns `Complete`/`Degraded`/`Failed` terminal semantics).
- Referenced by `platform-unification.md` and `pkg-install-reconciler.md` (providers conform; they do not restate).
- Realization of the state machine + journal: [step 41](steps/41-run-state-machine.md); retry semantics: steps 31
  (boundary mechanics) and 35 (tri-state defaults); `stopped`: step 36.
