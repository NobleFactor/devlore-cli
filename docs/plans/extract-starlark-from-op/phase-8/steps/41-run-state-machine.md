---
step: 41
title: "Run-state machine — phases, aberrant running states, terminal drivers, and the trace transition journal"
status: in-progress — foundation + PoliciesConfig + journal + Transition choke point landed/committed 2026-07-08; serialized Condition rename + the failure-handling reconciliation 2026-07-09; behavioral work items 6-19 pending
proof_run: 2026-07-08 (type foundation: pkg/op + all providers + provider/plan green; FAIL set unchanged from baseline)
parent: ../../phase-8.md
---

# Step 41 — Run-state machine + trace transition journal

**Status:** `in-progress` (foundation landed 2026-07-07). The state machine was settled in-session 2026-07-05; the
authoritative design lives in the [compensation-failure contract](../compensation-failure-contract.md) §"Run-state
machine refinement". This step realizes it. **Subsumes step 21's build items 1–2** (OnError verdict protocol +
the `Degraded` transition): under the machine, a handler's verdict is just *which flow terminal executes inside it*,
so the protocol falls out of the terminal drivers rather than being a special mechanism.

## Progress (2026-07-09)

**2026-07-09 — the failure-handling reconciliation + the serialized rename.** A design draft reviewed blind to these
docs was reconciled to our vocabulary and settled the remaining behavioral design: `Reason` becomes a typed
closed-vocabulary token and today's prose `Reason` becomes `Message`; `Transition` loses its Phase argument (the
executor owns Phase moves from lifecycle + the policy reaction) and returns a reject `error` (monotonicity enforced by
arbitration; the `OnError` absorption defers the pending flip); the error handler becomes a verdict-rendering `OnError` (renamed from `ErrorAction`) with a new `OnRetry`
(truthiness verdicts, absorption, `handler_failed` symmetry); `flow.Failed` is a hard condition assertion that mirrors
`flow.Degraded`, un-caught by `OnError`, the policy driving the stop. The compensation-failure decision (no forward
continuation; stop or pause) is now documented in §2.2 with the Garcia-Molina & Salem attribution. The serialized
`Condition` names were renamed `failed_execution` → `execution_failed` and `failed_compensation` →
`compensation_failed` (aligning serialized with identifier word order; item 11, landed). The flow terminal drivers are
a green uncommitted checkpoint (item 6). The reconciled items are 8–15 in the work list below.

**Type layer landed + committed 2026-07-09.** Items 9–10 and the bulk of 12–13 are implemented and green: the
`Reason` typed enum, `RunStatus` / `RunStatusTransition` grown to `{Phase, Condition, Reason, Message}`, and
`Transition` reworked to `(unitID, condition, reason, message) error` — no Phase argument, a downward request overruled
with an error (monotonicity by arbitration), the flow drivers + activation surface on the new signature, and the
orphaned `transitionPolicy()` helper removed. Pending: item 13's `flow.Failed` mirroring, item 14 (`OnError` /
`OnRetry` + the absorption deferral), item 15 (`transition_policy=`), and item 8 (the reconciled doc).

**Item 15 landed 2026-07-10.** `transition_policy=` is threaded through the full plan chain alongside `retry_policy=`:
`ExecutableUnit` (field, accessor, setter, spec, `WithTransitionPolicy`), the `Planner` interface + `ActionPlanner` +
the four flow planners, `plan.Provider.invocation` / `AssembleDefinition` + `splitReservedKwargs`, and
node/subgraph/graph construction + serialization (`transition:` beside `retry:`). The gen descriptor auto-regenerated
(`transition_policy?` on `plan.assemble`). Green (pkg/op + plan + flow + devloretest + providers pass; FAIL set
unchanged). Inert until items 16–17 consume the reaction. Remaining reconciliation: item 13 (`flow.Failed` mirroring,
gated on 16–17) and item 14 (`OnError`/`OnRetry` + absorption).

**Item 14 design settled 2026-07-10 — the failure-protocol seam.** `OnError`/`OnRetry` will live in a shared
executor-level `dispatchWithPolicy` primitive that both `Run` (root) and `DispatchChild` (children) call. The dispatch
mechanism is already unified via `Subgraph.Execute` — `Run` dispatches the root through the same `Subgraph.Execute` as
any nested subgraph; only the retry-loop wrapper is missing at the `Run` seam — so the hoist is small and dissolves the
root-inert caveat (the root's policies become live) and keeps the protocol invisible to providers. Absorption is an
implicit deferral (flip-at-unwind; an absorb stops propagation, so `execution_failed` never fires). Three slices:
rename + plumbing + the hoist; the `OnRetry` hook; the `OnError` verdict.

**Slice 1a landed 2026-07-10 — the `dispatchWithPolicy` hoist (behavior-preserving).** The retry loop moved from
`ActivationRecord.DispatchChild` into the shared `GraphExecutor.dispatchWithPolicy(unit, stack, variables)`; `Run`
dispatches the root through it (`e.dispatchWithPolicy(ctx, e.graph.Root(), …)`) and `DispatchChild` became a thin
forwarder (`a.executor.dispatchWithPolicy(…)`); the `dispatchChild` closure + field are removed. The root now carries
the retry wrapper — no behavior change for a nil root policy (one attempt, as before), and the root-inert caveat is
dissolved. Green (pkg/op + providers + devloretest; FAIL set unchanged). Next: the `ErrorAction` → `OnError` rename +
`OnRetry` plumbing (1b/1c).

**Slice 1b landed 2026-07-10 — the `ErrorAction` → `OnError` rename.** The failure handler is renamed throughout
(`OnError()` accessor, `setOnError`, the `onError` field/params, `WithOnError`, the `on_error=` reserved kwarg, the
plan chain, the four flow planners, and the failure-design docs — keeping the PowerShell `ErrorActionPreference` prior
art). Name-only; the observation-only behavior is unchanged (the verdict lands in slice 3). The gen descriptor
regenerated (`on_error?`). Green (pkg/op + providers + devloretest; FAIL set unchanged). Next: `OnRetry` plumbing (1c).

**Slice 2 landed 2026-07-11 — the `OnRetry` per-attempt hook.** `dispatchWithPolicy` now consults `OnRetry` after a
failed attempt *when a retry is pending* (`attempt+1 < maxAttempts` and the unit has an `OnRetry` handler): a truthy
verdict keeps retrying, a falsy verdict vetoes the loop, and a handler that itself errors or panics ends it. The
handler runs through a new `GraphExecutor.dispatchHandler` on a **fresh `RecoveryStack`** (§6.4 receipts-don't-leak —
it settles its own compensation, its receipts never join the parent stack; a panic is recovered into an error). The
verdict/handler reason reaches the boundary via a new internal `dispatchFailure` (in `graph_executor.go`'s SUPPORTING
TYPES) that wraps the cause and carries the flip `Reason`; the `Run` boundary unwind reads it through `failureReason`,
defaulting to `action_failed`. So a veto now lands `stopped × execution_failed × retry_vetoed`, a handler failure
`… × handler_failed`. Truthiness moved to the canonical **`op.IsTruthy`** (relocated from flow, exported; flow's
guard/wait_until call sites delegate; `guard_result.go`'s dangling `[IsTruthy]` ref now resolves). Tests: the veto
path proven end-to-end (`TestRun_OnRetryVeto_StampsRetryVetoed`), the reason-carrying mechanism proven through `%w`
wrapping (`TestFailureReason_*`), and the `IsTruthy` suite moved to `pkg/op`. Not yet wired: a veto (and exhaustion)
does not *yet* trigger `OnError` — that is slice 3. Green (pkg/op + providers + flow; FAIL set unchanged). Next:
slice 3 — the `OnError` verdict/absorption + walk-hook deletion.

**Slice 1c landed 2026-07-10 — the `OnRetry` handler plumbing.** `OnRetry` is added as a second `*Subgraph` handler
mirroring `OnError` exactly, threaded end-to-end through the plan chain: the `ExecutableUnit` interface (`OnRetry()` /
`setOnRetry`, the `onRetry` field, the accessor + parentID-stamping setter, the spec `OnRetry` field + `WithOnRetry`);
`NodeSpec` / `SubgraphSpec` / `GraphSpec` `WithOnRetry` + the construction stamps (`setOnRetry(spec.OnRetry)`);
`ActionPlanner.Plan` + the four flow planners (the `onRetry` param after `onError`, the `.WithOnRetry(...)` stamp, the
Plan-doc bullet); the `on_retry=` reserved kwarg (`splitReservedKwargs` → `onRetrySubgraph` → `subgraphFromInvocations`,
`AssembleDefinition` → `.WithOnRetry(onRetrySg)`, the `p.invocation` seam). The gen descriptor regenerated (`on_retry?`).
Behavior is still observation-only — the per-attempt hook is wired in slice 2, the veto/verdict in slice 3. Green
(pkg/op + providers + devloretest; FAIL set unchanged). Next: the `OnRetry` per-attempt hook in `dispatchWithPolicy` (2).

**Landed + committed 2026-07-08 — the type foundation** (no behavior change; the flat `RunState` enum is re-expressed
as the triplet): `op.State` → `ResourceState` (frees the name; the run health dimension is `Condition`, not `State`).
`run_state.go` defines `Phase` (`PhasePreparing` … `PhaseCompleted`), `Condition` (`ConditionHealthy` <
`ConditionDegraded` < `ConditionExecutionFailed` < `ConditionCompensationFailed`, severity-ordered; identifiers read
subject-verb and the serialized names are the matching snake forms `execution_failed`), and `RunStatus` as the run-status
triplet `{Phase, Condition, Reason}` (`Reason` = the prose driver of the latest move, for informative logs). Both enum
dimensions carry text/YAML marshaling. `Trace.State` → `Trace.RunStatus` (serialized `run_status: {phase, condition,
reason}`); `GraphExecutor.State()` → `RunStatus()`. The executor carries the triplet via faithful direct assignment
(old `Failed` → stopped × `execution_failed`; `FailedCompensation` → stopped × `compensation_failed`; completion moves
Phase only, preserving the `Condition` as it stood; each terminal stamps a `Reason`). `pkg/op`, all providers, and
`provider/plan` green; the FAIL set is unchanged from baseline. All four transition-trigger additions are confirmed
(wire all four: bubble-up, preparing-phase errors, framework dispatch errors, resume de-escalation).

**Landed (the wrap, 2026-07-08):** the op-owned `PoliciesConfig` section (path `policies`, `init()`-announced at its
builtin floor, `RuntimeEnvironmentConfig` precedent) + `TransitionPolicy` + `Reaction {continue/pause/stop}` +
`Validate` + the `PoliciesFrom` accessor; the flips-only transition journal (`RunStatusTransition`,
`Trace.Transitions`, projected into `Trace()` and restored by `ResumeExecutor`); and the
`Transition(unitID, phase, condition, reason) Reaction` choke point — the run-status machine's sole mutator
(monotonic only-worsens `Condition`, flips-only journal append, floor policy reaction). The activation exposes a **narrow
surface**: `RunStatus()` (a read-only value copy) + `Transition()` (delegating to the boundary executor); the
`executor` stays **private** on the record (no `Executor()` accessor), so a dispatched provider sees only read + the
sanctioned mutate.

**Pending — the behavioral wiring:** the flow terminal drivers (`Complete` early-return, `Degraded`/`Failed` as typed
condition-flip drivers reaching `Transition` through the activation) + the OnError verdict protocol (replacing the
`flow/helpers.go` observation hook); the executor's preparing→running move + bubble-up (parent reads the child
executor's status triplet, adjudicates, takes the worst by max-severity); the four transition triggers; the
`transition_policy=` reserved kwarg; and the stop contract. The three flow terminals gain a framework-injected
`*op.ActivationRecord` first parameter — the reflection-detected `firstParamIsActivation` pattern the combinators
already use (`method.go:111`), stripped from the user-visible params — so the Starlark surface, announced params, and
codegen are unchanged.

**This behavioral work is the next task** (sequencing agreed 2026-07-08, ahead of the rest of step 21 and step 22):
it also closes **step 21's build items 1–2** — `OnError` verdict dispatch (R1) and the `ConditionDegraded`
transition — which step 21 subsumes into this step. Step 21's remaining item 5 (journal persistence + restart
instructions + state-checked resume) and step 22 follow.

**Deferred consumer:** `cmd/writ/writ/migrate/receipt_integration_test.go` still cites `op.RunStateCompleted` /
`trace.State`. That package is already build-broken (`op.ImmediateOf` / `plan.Provider.Assemble`) and is step 33's
rewrite; not edited here.

## The machine (summary; the contract doc is authoritative)

- **The pair (settled 2026-07-05, Q2):** run state = `Phase` × `State`. Phases: `preparing` → `running` →
  `pausing`/`paused` (resumable), and **two terminal phases** (settled 2026-07-06): `completed` (natural end —
  final unit or `flow.Complete`) and `stopping`/`stopped` (commanded or policy-driven end). States (only-worsening,  orthogonal): `healthy` → `degraded` → `execution_failed` → `compensation_failed` (rename chain: failed →
  execution_failed 2026-07-05 → execution_failed 2026-07-06). Terminals are **derived**: `{completed, stopped} ×
  State` — `completed × execution_failed` is the continue-on-failure end; `stopped × healthy` is a clean cancel;
  `compensation_failed` pairs only with `stopped`.
- **State-flip drivers:** `degraded` ⇐ `flow.Degraded` (a gate on its input); `execution_failed` ⇐ saga-boundary
  retry exhaustion or `flow.Failed`; `compensation_failed` ⇐ a compensation action fails. Completion is a Phase
  event, never a State flip — State ends as it stood. Proposed trigger additions (contract doc, confirmation
  pending): bubble-up, preparing-phase errors, framework dispatch errors, the resume de-escalation.
- **Three-way flip reaction (noted 2026-07-05):** each aberrant flip consults a configured reaction ∈ {continue,
  pause, stop}; defaults settle with Q3 (working baseline: degraded → continue, execution_failed → stop);
  `compensation_failed` is always stop, outside the policy.
- **Stop contract:** return the final action's result and error, plus the terminal run status (phase + condition).
- **Trace transition journal:** `{Phase, Condition, At, UnitID, Reason}` per flip of either dimension via a single
  recording setter; the status stays the O(1) answer; per-event detail stays on receipts, cross-referenced
  by `UnitID`.

## Work items

The failure-handling reconciliation (2026-07-09, against a design draft reviewed blind to these docs) folded items
8–15 into the step; the serialized `Condition` rename is item 11. Status: ✅ committed, 🟡 coded (uncommitted), ⬜
pending.

**Landed + committed**

1. ✅ **Type foundation** — `ResourceState` rename; `Phase` / `Condition` / `RunStatus` types + text/YAML marshaling;
   `Trace.RunStatus`; `RunStatus()` accessor.
2. ✅ **`PoliciesConfig`** — the op-owned section + `TransitionPolicy` + `Reaction` + `Validate` + `PoliciesFrom`.
3. ✅ **The transition journal** — `RunStatusTransition`, `Trace.Transitions`, the recording setter, projected into
   `Trace()` and restored by `ResumeExecutor`.
4. ✅ **The `Transition` choke point + narrow activation surface** — sole mutator; `RunStatus()` + `Transition()` on
   the record, executor private. (The signature is reworked by item 12.)
5. ✅ **Code-comment corrections** — the flat `RunState` comments rewritten to the triplet with the type foundation.

**Committed 2026-07-09**

6. ✅ **Flow terminal drivers** — the three terminals gain the framework-injected `*op.ActivationRecord` first param;
   `Complete` early-return (the walk stops on a `flow.complete` child); `Degraded` / `Failed` drive `Transition` on
   the reshaped signature (item 13 refines `flow.Failed` to mirror `flow.Degraded`).
7. ✅ **Compensation-failure decision doc** — §2.2 "Compensation Failure Has No Forward Continuation" + the
   Garcia-Molina & Salem attribution.

**Pending — the failure-handling reconciliation**

8. ⬜ **Place the reconciled failure-handling doc** (`docs/architecture/2.4-failure-handling.md`) — the reviewed draft
   aligned to our vocabulary + these decisions; §2.2's machine section and the contract doc shrink to pointers.
9. ✅ **`Reason` typed enum** (landed 2026-07-09) — a snake-serialized closed vocabulary, two families (health:
   `action_failed`, `compensation_failed`, `retry_vetoed`, `handler_failed`, `absorbed`, `degraded`, `failed`,
   `preflight_failed`; lifecycle: `started`, `completed`, `stopped`, `paused`); the policy dispatches on `Condition`,
   so the vocabulary stays small.
10. ✅ **`RunStatus` → `{Phase, Condition, Reason, Message}`** (landed 2026-07-09) — the prose `Reason` became
    `Message`; the typed `Reason` slotted in. `RunStatusTransition` grew the same two fields.
11. ✅ **Serialized `Condition` rename** — `failed_execution` → `execution_failed`, `failed_compensation` →
    `compensation_failed` across code + docs (landed 2026-07-09; the identifier ⇄ serialized word order now aligns).
12. 🟡 **`Transition` rework** — drop the Phase argument (done — the executor owns Phase moves), `reason string` →
    typed `Reason` + `message string` (done), return `error` (done — a downward request is overruled, monotonicity by
    arbitration). The `OnError` absorption deferral rides with item 14.
13. 🟡 **Drivers rework** — `Degraded` / `Failed` on the new signature with typed reasons (done); `flow.Failed`
    mirroring `flow.Degraded` (dropping its error-return short-circuit so the policy drives the stop) is pending. The
    objective default (a bare error → `{execution_failed, action_failed}`) is already in the executor.
14. 🟡 **`OnError` / `OnRetry` — the failure-protocol seam** — hoist the retry loop out of
    `ActivationRecord.DispatchChild` into a shared executor-level `dispatchWithPolicy(unit, stack, variables)` that both
    `Run` (root) and `DispatchChild` (children) call. The dispatch mechanism is already shared (`Subgraph.Execute` —
    `Run` dispatches the root through it like any nested subgraph); only the policy wrapper was not — so the hoist is
    small and **dissolves the root-inert caveat** (the root's `RetryPolicy` / `OnError` / `OnRetry` become live), keeping
    the protocol invisible to providers. Then: `error_action=` → `on_error=`, add `on_retry=`; `OnRetry` per attempt
    (truthy ⇒ retry, falsy ⇒ veto `retry_vetoed`, error ⇒ `handler_failed`); `OnError` on exhaustion (truthy ⇒ absorb —
    its return becomes the result, `absorbed`; falsy ⇒ fail `action_failed`; error ⇒ `handler_failed`). The deferral is
    **implicit**: the `execution_failed` flip only fires at the boundary unwind when an error propagates, so an absorb
    (which stops propagation) is a climb-not-taken — no explicit pending-flip register. The walk's observation hook
    (`flow/helpers.go:148–154`) is deleted; handlers dispatch on a **fresh `RecoveryStack`** (§6.4 receipts-don't-leak).
    Slices: (1) ✅ rename + `OnRetry` plumbing + the `dispatchWithPolicy` hoist (1a hoist, 1b `OnError` rename, 1c
    `OnRetry` plumbing — all landed 2026-07-10); (2) ✅ the `OnRetry` hook (veto/handler_failed via `dispatchFailure`;
    `op.IsTruthy` relocation — landed 2026-07-11); (3) ⬜ the `OnError` verdict + walk cleanup.
15. ✅ **`transition_policy=` reserved kwarg** (landed 2026-07-10) — the sibling to `retry_policy=`, threaded through
    the plan chain (`splitReservedKwargs` → `invocation` / `AssembleDefinition` → `Plan` → spec → unit / subgraph /
    graph), unit- and graph-level, serialized beside `retry` (`transition:`). Authorable at plan time; inert until
    reaction consumption (items 16–17).

**Pending — behavioral wiring**

16. ⬜ **`preparing` → `running` move** — entered at construction, exited on first dispatch (today `Run` stamps
    `PhaseRunning` before environment build + variable binding — the transition point moves).
17. ⬜ **Bubble-up + the four triggers** — the parent reads the child executor's terminal triplet and adjudicates by
    max-severity; the four flips (bubble-up, preparing-phase errors, framework-dispatch errors, resume de-escalation).
18. ⬜ **The stop contract** — `Run` returns `(result, error)` of the final action plus the terminal run status;
    today's result-discarding failure paths (`return nil, err`) align with it.
19. ⬜ **Cleanup + verify**.

## Design-question ledger (order of settlement: 2, 4, 3, 1)

1. **Q2 — representation: SETTLED 2026-07-05.** The Phase × Condition × Reason triplet (`RunStatus`) above. Go constants (settled 2026-07-06; health dimension
   renamed to `Condition` 2026-07-07): `ConditionHealthy` / `ConditionDegraded` / `ConditionExecutionFailed` /
   `ConditionCompensationFailed` — identifiers read subject-verb and the serialized names are the matching snake forms; Phase constants follow the same pattern (`PhasePreparing` … `PhaseCompleted`,
   `PhaseStopped`). Consequence folded into the work items: the resource lifecycle type `op.State`
   (`resource_state.go:21`) renames to `ResourceState` to free the name; the run health dimension is `Condition`.
2. **Q4 — transition scope + `flow.Complete` early exit: MECHANISM SETTLED 2026-07-05** (authoritative text: the
   contract doc §"Policy enforcement and bubble-up"). The ActivationRecord is the home of run-state info; **all**
   flow provider methods accept an activation record (the three terminals gain it); the graph executor enforces
   policy via two policies — `RetryPolicy` + the new **`TransitionPolicy`** (working name, pick pending; map
   entered-Condition → Reaction ∈ {Continue, Pause, Stop}; PowerShell `ErrorActionPreference` prior art, plus
   Erlang/OTP supervision trees, Step Functions Retry/Catch, Ansible, Terraform `on_failure`). Layering:
   RetryPolicy suppresses → OnError adjudicates → Condition records (single setter, journaled) → TransitionPolicy
   reacts — atomic at one choke point. Bubble-up (corrected 2026-07-06): **the executor tree is the channel** —
   Phase × Condition never travels through method returns (the dispatch chain is provider-shaped end to end; a
   compensable method returns `(product, receipt, error)`); dispatch returns `(result, err)` and the parent reads
   the child executor's terminal triplet as it ended through the handle it created (`newChildExecutor`), mirroring the
   host's `Run` + `RunStatus()` read at the root; the subgraph's audit receipt records the child's terminal triplet for
   the serialized trace; an ActivationRecord transition method was rejected as a side channel (the record carries
   state info downward only). The parent adjudicates before recording (repair absorbs `execution_failed`), takes
   `degraded` unconditionally by max-severity, journals provenance via `UnitID`. Residuals SETTLED 2026-07-06:
   `flow.Complete` is an **early return from a subgraph combinator — like a `return` statement in a func**; the
   never-dispatched remainder gets no receipts (absence is the record), and everything the body already did is
   kept (a success return unwinds nothing).
3. **Q3 — configuration: SETTLED 2026-07-06** (authoritative text: the contract doc §"TransitionPolicy — Q3
   settled"). Name: **`TransitionPolicy`**. Floor: degraded → continue; execution_failed → stop;
   compensation_failed → stop — pause is the attended-mode override for both failure states (layered in via
   profile/app config); `compensation_failed` re-enters the policy with **continue-illegal** (`Validate()`
   enforces). Stop is boundary-local (unwind + `(nil, error, terminal state)` to the parent, bubble-up
   adjudication); pause is run-global (failure-pause returns `errors.Join(ErrPaused, cause)`; the journal entry is
   authoritative). Home: the op-owned `PoliciesConfig` section (path `policies`, `runtime`-section precedent) —
   `Retry op.RetryPolicy` (the default for subgraph combinators; step-35 tri-state: none = explicit
   `MaxAttempts:0`, default = nil → configured `policies.retry` for subgraphs / none for all other units,
   specific = explicit policy wins) + `Transition TransitionPolicy`. Plan-time: `transition_policy=` reserved
   kwarg beside `retry_policy=`, unit- and graph-level, serialized beside `retry`. Execution resolution:
   unit ?? ancestor ?? graph ?? app config ?? floor.
4. **Q1 — journal granularity: SETTLED 2026-07-06.** Flips-only: the journal records actual state changes; a
   repeat `flow.Degraded` while degraded is a receipt, not a transition.

## Sequencing

Interlocks with step 31 (per-subgraph executors: the machine describes the *root run's* state; boundaries report
upward) and step 36 (`stopped` joins the control axis). The verdict work formerly planned as step 21's items 1–2
lands here; step 21 retains the compensation-specific arc (journal persistence on `compensation failed` + restart
instructions + state-checked resume).
