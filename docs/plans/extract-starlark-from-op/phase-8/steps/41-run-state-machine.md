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

- **Execution axis:** `preparing` (construction + pre-flight) → `running` (from the first unit dispatch) →
  optionally the aberrant running states — `running, degraded` (default **continue**; stop by configuration) and
  `running, failed` (default **stop**; continue by configuration) → terminal.
- **Terminal drivers:** `completed` ⇐ last unit or `flow.Complete` (result = its input); `degraded` ⇐ run stops
  while degraded (`flow.Degraded` is the gate); `failed` ⇐ saga-boundary retry exhaustion or `flow.Failed`;
  `compensation failed` ⇐ any compensation action fails — always stop, no configuration.
- **Stop contract:** return the final action's result and error, plus the terminal run state.
- **Control axis:** `paused` (resumable; preserves position and aberrance) and `stopped` (step 36; terminal
  unwind; a failed stop-unwind lands `compensation failed`).
- **Trace transition journal:** `Trace.Transitions []RunStateTransition{To, At, UnitID, Reason}` — one entry per
  flip via a single recording setter; `Trace.State` stays the latch; per-event detail stays on receipts,
  cross-referenced by `UnitID`.

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

## Open design items (settle at pickup)

1. **Flips-only journal** — proposed 2026-07-05 (a repeat `flow.Degraded` while already degraded is a receipt, not
   a transition); confirmation pending.
2. **Enum representation** — distinct members for the aberrant running states (`RunStateRunningDegraded`, …) vs. a
   (phase × aberrance) pair; and the `RunStatePending` → `preparing` rename. Text serialization (landed
   2026-07-04) makes either append-safe.
3. **Configuration home** — where stop-on-degraded / continue-on-failed live (graph spec, run spec, application
   config) and their names.
4. **`flow.Complete` early-exit semantics** — what "skipped remainder" means for receipts, promises, and the
   resume guard.

## Sequencing

Interlocks with step 31 (per-subgraph executors: the machine describes the *root run's* state; boundaries report
upward) and step 36 (`stopped` joins the control axis). The verdict work formerly planned as step 21's items 1–2
lands here; step 21 retains the compensation-specific arc (journal persistence on `compensation failed` + restart
instructions + state-checked resume).
