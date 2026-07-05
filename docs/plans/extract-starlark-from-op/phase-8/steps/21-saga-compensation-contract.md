---
step: 21
former_step: 18.6
title: "SAGA failure-handling & compensation-failure contract"
status: in-progress — CompensationFailed terminal landed 2026-07-04; ErrorAction dispatch, Degraded transition, journal persistence open
proof_run: 2026-07-04
parent: ../../phase-8.md
---

# Step 21 — SAGA failure-handling & compensation-failure contract (formerly 18.6)

**Status:** `in-progress`. The contract is settled; its realization is underway — the fourth terminal is real as of
2026-07-04. Cross-cutting: it governs the deploy's failure semantics for every graph.

## The contract

1. **Four run terminals:** `Completed` / `Degraded` / `Failed` / `CompensationFailed`.
2. **Error actions MUST run** — a declared `error_action` is not best-effort.
3. **A failed `Compensate` → `CompensationFailed`:** fail loud, journal the `Trace`, and support restart from it.
   Compensation failure is never swallowed into a `Failed` terminal.

## Realization state (2026-07-04)

**Landed (build items 3–4):**

- `RunStateCompensationFailed` (`pkg/op/run_state.go`) — appended after `RunStateFailed`; `GraphExecutor.Run` maps a
  non-nil `RecoveryStack.Unwind` error to it, while a clean unwind keeps `RunStateFailed`. The joined Run error names
  the forward failure and every failed compensation (the fail-loud half of R2). The unwind loop itself is
  best-effort-complete (R3).
- `RunState` text serialization — traces now carry `"state": "compensation_failed"` (JSON) / `state:
  compensation_failed` (YAML) instead of a bare integer, per the GuardResult document-form precedent
  (`MarshalText`/`UnmarshalText` + the yaml.v3 companions).
- Tests: `TestRun_CompensationFailure_ReachesCompensationFailed` and `TestRun_CleanUnwind_ReachesFailed`
  (`pkg/op/graph_executor_test.go`) pin the terminal boundary end-to-end through announced compensable fixtures whose
  `CompensateProduce` errors / succeeds; `run_state_test.go` round-trips every state through both document formats.

**Subsumed by step 41 (2026-07-05):** build items 1–2 — `ErrorAction` verdict dispatch (R1) and the
`RunStateDegraded` transition — land with the run-state machine
([41-run-state-machine.md](41-run-state-machine.md)): a handler's verdict is which flow terminal executes inside it,
so the protocol falls out of the terminal drivers. (Correction 2026-07-05: an error action IS dispatched today, as a
one-shot best-effort observation hook at the enclosing body — `flow/helpers.go:144-150`, `:201-206`; the 2026-07-04
"never dispatched" note grepped too narrowly. The verdict semantics are what's unbuilt.)

**Open here (build item 5):** journal persistence on `CompensationFailed` (R4) + restart instructions + the
state-checked resume.

## Design and history

The contract lives in [phase-8/compensation-failure-contract.md](../compensation-failure-contract.md), including the
per-primitive wiring table and the remaining build list. The fourth terminal was renamed `Stranded` →
`CompensationFailed` on 2026-07-04, before any code carried the old name. The pre-renumber lineage called this
"step 21.6" in [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(rewritten to the current numbering in the 2026-07-03 audit, group 4).

## Note

The name collision with the historical "graph-immutability seal" (`phase-8/graph-immutability.md`) is coincidental:
that work predates the current table; its citations were rewritten name-based in the 2026-07-03 audit.
