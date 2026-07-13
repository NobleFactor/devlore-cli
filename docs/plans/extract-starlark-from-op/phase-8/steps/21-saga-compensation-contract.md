---
step: 21
former_step: 18.6
title: "SAGA failure-handling & compensation-failure contract"
status: in-progress — items 1–4 landed (the stopped × ConditionCompensationFailed terminal + best-effort unwind, 2026-07-04; the OnError verdict dispatch + ConditionDegraded transition landed with step 41, now complete); the state-checked resume (ResumeExecutor) and the transition-journal data structure also landed; the only open work is item 5's failure-time behavior — auto-persist the Trace on stopped × ConditionCompensationFailed + generate restart instructions
proof_run: 2026-07-04
parent: ../../phase-8.md
---

# Step 21 — SAGA failure-handling & compensation-failure contract (formerly 18.6)

**Status:** `in-progress`. The contract is settled and nearly fully realized: the `stopped × ConditionCompensationFailed`
terminal + best-effort unwind (2026-07-04), the `OnError` verdict dispatch, the `ConditionDegraded` transition, and the
state-checked resume all landed (the last three with step 41, now complete). The only open work is item 5's failure-time
behavior. Cross-cutting: it governs the deploy's failure semantics for every graph.

## The contract

1. **Four run terminals** (now the derived `{completed, stopped} × Condition` cells per step 41): `completed × healthy`,
   `completed × degraded`, `stopped × ConditionExecutionFailed`, `stopped × ConditionCompensationFailed`.
2. **Error actions MUST run** — a declared `error_action` is not best-effort.
3. **A failed `Compensate` → `stopped × ConditionCompensationFailed`:** fail loud, journal the `Trace`, and support
   restart from it. Compensation failure is never swallowed into a `ConditionExecutionFailed` terminal.

## Realization state (2026-07-13)

**Landed (build items 3–4):**

- The `stopped × ConditionCompensationFailed` terminal (`pkg/op/run_state.go`) — `GraphExecutor.Run` maps a non-nil
  `RecoveryStack.Unwind` error to it (`graph_executor.go:444`), while a clean unwind keeps `stopped ×
  ConditionExecutionFailed`. The joined Run error names the forward failure and every failed compensation (the fail-loud
  half of R2). The unwind loop itself is best-effort-complete (R3). (Step 41's foundation re-expressed the old flat
  terminals as the `RunStatus{Phase, Condition, Reason}` triplet, so terminals are now derived as `stopped × condition`;
  the mapping is otherwise unchanged.)
- `RunStatus` / `Condition` serialization — traces carry `run_status: {phase: stopped, condition: compensation_failed}`
  in both document formats: the `Condition` dimension serializes over the settled snake names via `MarshalText` /
  `MarshalYAML` (+ the yaml.v3 companions), per the GuardResult document-form precedent.
- Tests: `TestRun_CompensationFailure_ReachesFailedCompensation` and `TestRun_CleanUnwind_ReachesFailed`
  (`pkg/op/graph_executor_test.go:158`/`:179`) pin the terminal boundary end-to-end through announced compensable
  fixtures whose `CompensateProduce` errors / succeeds (asserting the `Phase` × `Condition` pair); `run_state_test.go`
  round-trips every `Phase` and `Condition` through both document formats. (The `ReachesFailedCompensation` test name
  still carries the dead "FailedCompensation" term — the terminal is `stopped × ConditionCompensationFailed`.)

**Landed with step 41 (build items 1–2):** the `OnError` verdict dispatch (R1) and the `ConditionDegraded` transition
landed with the run-state machine ([41-run-state-machine.md](41-run-state-machine.md), now **complete**).
`GraphExecutor.resolveFailure` (`graph_executor.go:598`) fires the failed unit's `OnError` handler and renders its
verdict — a truthy return absorbs the failure (becomes the unit's result, no flip), a falsy one lets it stand;
`flow.Provider.Degraded` (`flow/provider.go:620`) flips `ConditionDegraded` via `Transition`.

**Landed (state-checked resume):** `ResumeExecutor` restarts from a persisted trace (checksum-guarded), and the resume
de-escalation is a state-checked unwind — a resumed `stopped × ConditionCompensationFailed` trace whose unwind clears
cleanly lands `stopped × ConditionExecutionFailed` (`graph_executor.go:224`/`:250`). The transition-journal **data
structure** is also done: `Trace.Transitions []RunStatusTransition` serializes in both formats (`trace.go:32`).

**Open here (build item 5) — the failure-time behavior:** two pieces remain, both derived from the already-built trace:

1. **Persist the `Trace` on `stopped × ConditionCompensationFailed`.** The executor exposes `.Trace()`
   (`graph_executor.go:307`), and the trace serializes + `ResumeExecutor` restarts from it — but nothing
   **auto-persists on compensation failure** (no `document.Write` on the failure path); it is left to the caller.
2. **Generate restart instructions.** On `stopped × ConditionCompensationFailed`, emit troubleshooting + the exact
   resume command derived from the trace (which node failed, which `Compensate` failed and why). **Absent** — no
   restart-instruction generation exists, confirmed by the contract's own wiring table
   ([compensation-failure-contract.md](../compensation-failure-contract.md): "Journal persistence on failure +
   restart-instruction generation — ❌ ❌").

## Design and history

The contract lives in [phase-8/compensation-failure-contract.md](../compensation-failure-contract.md), including the
per-primitive wiring table and the remaining build list. The terminal's rename chain: `Stranded` → `CompensationFailed`
(2026-07-04) → `FailedCompensation` (2026-07-06) → **`ConditionCompensationFailed`** (2026-07-07, step 41's
`RunStatus{Phase, Condition}` triplet — the current name, serialized `compensation_failed`). "FailedCompensation" is dead
in code (one leftover test name aside); it survives only in not-yet-swept docs. The pre-renumber lineage called this
"step 21.6" in [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(rewritten to the current numbering in the 2026-07-03 audit, group 4).

## Note

The name collision with the historical "graph-immutability seal" (`phase-8/graph-immutability.md`) is coincidental:
that work predates the current table; its citations were rewritten name-based in the 2026-07-03 audit.
