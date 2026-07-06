---
step: 41
title: "Run-state machine — phases, aberrant running states, terminal drivers, and the trace transition journal"
status: not-started — design settled 2026-07-05; execution pending
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 41 — Run-state machine + trace transition journal

**Status:** `not-started` (charter). The state machine was settled in-session 2026-07-05; the authoritative design
lives in the [compensation-failure contract](../compensation-failure-contract.md) §"Run-state machine refinement".
This step realizes it. **Subsumes step 21's build items 1–2** (ErrorAction verdict protocol + the `Degraded`
transition): under the machine, a handler's verdict is just *which flow terminal executes inside it*, so the
protocol falls out of the terminal drivers rather than being a special mechanism.

## The machine (summary; the contract doc is authoritative)

- **The pair (settled 2026-07-05, Q2):** run state = `Phase` × `State`. Phases: `preparing` → `running` →
  `pausing`/`paused` (resumable) and `stopping`/`stopped` (the sole terminal phase). States (latching, orthogonal):
  `healthy` → `degraded` → `execution_failed` → `compensation_failed` (`execution_failed` renamed from "failed"
  for symmetry, 2026-07-05). Terminals are **derived**: `stopped × <state>` — `stopped, healthy` is completed.
- **State-flip drivers:** `degraded` ⇐ `flow.Degraded` (a gate on its input); `execution_failed` ⇐ saga-boundary
  retry exhaustion or `flow.Failed`; `compensation_failed` ⇐ a compensation action fails. Completion ⇐ last unit
  or `flow.Complete` (result = its input).
- **Three-way flip reaction (noted 2026-07-05):** each aberrant flip consults a configured reaction ∈ {continue,
  pause, stop}; defaults settle with Q3 (working baseline: degraded → continue, execution_failed → stop);
  `compensation_failed` is always stop, outside the policy.
- **Stop contract:** return the final action's result and error, plus the terminal run state (phase + state).
- **Trace transition journal:** `{Phase, State, At, UnitID, Reason}` per flip of either dimension via a single
  recording setter; the latched pair stays the O(1) answer; per-event detail stays on receipts, cross-referenced
  by `UnitID`.

## Work items

1. **`preparing`** — the pre-flight state: entered at construction, exited when the first unit dispatches. (Today
   `Run` stamps `RunStateRunning` *before* environment build and variable binding — the transition point moves.)
2. **Aberrant running states** — `running, degraded` and `running, failed` with their default/configured exits;
   the stop-on-degraded and continue-on-failed configuration acts (Q3's `--strict` generalizes to the former).
3. **Terminal drivers wired uniformly** — including `flow.Complete` as an early-exit completion gate (today it is
   a value pass-through with no control effect) and `flow.Degraded` as a typed gate the executor can recognize
   (today it returns a bare `string`, indistinguishable from `flow.Complete("...")`).
4. **Per-unit `ErrorAction` under the driver rules** — replaces the current one-shot best-effort observation hook
   (`flow/helpers.go:144-150`, `:201-206`, which dispatches the *enclosing body's* `error_action` and always
   propagates the original error) with the contract's verdict semantics, expressed through the terminals.
5. **The transition journal** — `RunStateTransition`, `Trace.Transitions`, the recording setter, serialization in
   both document formats, and answering "when/where did the state flip" in tests.
6. **The stop contract** — `Run` returns (result, error) of the final action plus the terminal state; today's
   result-discarding failure paths (`return nil, err`) align with it.
7. **Code-comment corrections** — `RunStateCompleted`'s "reached from Degraded" and `RunStateDegraded`'s framing
   update to the two-layer model (running form vs latched terminal).

## Design-question ledger (order of settlement: 2, 4, 3, 1)

1. **Q2 — representation: SETTLED 2026-07-05.** The Phase × State pair above. Consequence folded into the work
   items: the resource lifecycle type `op.State` (`resource_state.go:21`) renames to `ResourceState` to free the
   name for the run dimension.
2. **Q4 — transition scope + `flow.Complete` early exit: MECHANISM SETTLED 2026-07-05** (authoritative text: the
   contract doc §"Policy enforcement and bubble-up"). The ActivationRecord is the home of run-state info; **all**
   flow provider methods accept an activation record (the three terminals gain it); the graph executor enforces
   policy via two policies — `RetryPolicy` + the new **`TransitionPolicy`** (working name, pick pending; map
   entered-State → Reaction ∈ {Continue, Pause, Stop}; PowerShell `ErrorActionPreference` prior art, plus
   Erlang/OTP supervision trees, Step Functions Retry/Catch, Ansible, Terraform `on_failure`). Layering:
   RetryPolicy suppresses → ErrorAction adjudicates → State records (single setter, journaled) → TransitionPolicy
   reacts — atomic at one choke point. Bubble-up: every subgraph dispatch returns `(result, error, terminal
   Phase × State)`; the parent adjudicates before latching (repair absorbs `execution_failed`), latches `degraded`
   unconditionally by max-severity, journals provenance via `UnitID`. Residual sub-questions: receipts for the
   never-dispatched remainder (proposed: none — absence is the record); side effects kept on a `flow.Complete`
   exit (proposed: yes — success terminal); the `TransitionPolicy` name pick.
3. **Q3 — configuration: OPEN.** Home (graph document / run spec / application config), names, and the per-flip
   default reactions ∈ {continue, pause, stop}; re-confirm `compensation_failed` stays always-stop.
4. **Q1 — journal granularity: OPEN.** Flips-only proposed (repeat `flow.Degraded` while degraded = receipt, not
   transition).

## Sequencing

Interlocks with step 31 (per-subgraph executors: the machine describes the *root run's* state; boundaries report
upward) and step 36 (`stopped` joins the control axis). The verdict work formerly planned as step 21's items 1–2
lands here; step 21 retains the compensation-specific arc (journal persistence on `compensation failed` + restart
instructions + state-checked resume).
