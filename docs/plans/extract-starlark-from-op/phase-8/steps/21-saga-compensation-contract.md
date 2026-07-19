---
step: 21
former_step: 18.6
title: "SAGA failure-handling & compensation-failure contract"
status: COMPLETE 2026-07-18 — the final item landed: the resumed pure state-checked unwind (GraphExecutor.ResumeUnwind + ReasonUnwound); the audit confirmed it had NOT been built (Run refuses non-paused resumes; Transition's own doc deferred it here), and it now exists with three pinning tests; the stale FailedCompensation test names swept
proof_run: 2026-07-04
parent: ../../phase-8.md
---

# Step 21 — SAGA failure-handling & compensation-failure contract (formerly 18.6)

**Status:** COMPLETE 2026-07-18 — every item realized, including the final one: the resumed pure state-checked
unwind (`GraphExecutor.ResumeUnwind`, `ReasonUnwound`). See the Landed section below.
terminal + best-effort unwind (2026-07-04), the `OnError` verdict dispatch, the `ConditionDegraded` transition, and the
state-checked resume all landed (the last three with step 41, now complete). Item 5's framework-side journal work landed
2026-07-13: the framework reports what it knows — the source of the problem and the compensation diagnostics — durably
on the `Trace`; the client owns persistence and presentation (out of step 21). Cross-cutting: it governs the deploy's
failure semantics for every graph.

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
  (`pkg/op/graph_executor_test.go:158`/`:203`) pin the terminal boundary end-to-end through announced compensable
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

**Landed 2026-07-13 (build item 5, framework side) — the failure-time behavior.** Scope settled along the
framework/client boundary. **The client owns persistence and presentation:** `internal/cli.WriteTrace` already
writes the `Trace` under the receipts directory, and rendering restart instructions (troubleshooting text, the
resume command) is a consumer surface. **The framework owns reporting what it knows** — the source of the problem
and the compensation diagnostics — durably on the `Trace`, so a client that persists it holds a faithful journal
and a client that presents restart instructions can name which node failed and which `Compensate` failed and why.
CLI wiring is out of step 21.

**The defect this fixed:** before this landed, the journal was destroyed exactly at the compensation-failure terminal.
`RecoveryStack.Unwind` collected each compensation failure only into the joined return error, then unconditionally
cleared `s.entries`; `GraphExecutor.Trace()` reads `e.stack` directly, so a trace captured after the failed unwind had
an **empty stack** — the failing receipt and every compensation outcome gone, the only surviving report the unstructured
joined error string `Run` returns. That contradicted §5.2's "every compensation outcome stays on the receipts" and R4
("persist the `Trace` … the `RecoveryStack` with per-entry compensation outcomes").

**What landed:**

1. **Retain the journal on a failed unwind** (`recovery_stack.go`, `RecoveryStack.Unwind`). The stack clears only on a
   clean unwind (every compensation succeeded → `ConditionExecutionFailed`, nothing to journal); any compensation
   failure retains the entries, so `Trace().Stack` carries the failing receipt (source) plus the compensated/failed
   entries (diagnostics).
2. **Record each compensation outcome on its receipt** (`receipt.go`). `ReceiptBase` gained `compensationError`,
   serialized as `compensation_error` on `ReceiptData` (`omitempty`) and restored uniformly by `ReceiptBase.Restore` /
   `RestoreEncoded`; `RecoveryStack.Unwind` sets it when a leaf receipt's `Compensate` fails. It is distinct from the
   forward `status` (`status` = the forward call failed; `compensation_error` = its undo failed). The two hand-building
   providers (`file`, `pkg`) thread the field through their `RestoreEncoded`; every other receipt inherits it. A nested
   substack's dirtiness rides its own retained failed children.
3. **The source falls out of #1** — the failing node's receipt (`Err()` set, nil compensator so `Unwind` skips it as an
   audit entry) is retained, naming which node failed and why.

**Tests:** `TestRun_CompensationFailure_ReachesFailedCompensation` (extended) asserts the retained stack carries the
forward error and the `CompensationError`; `TestRun_CleanUnwind_ReachesFailed` (extended) asserts a clean unwind still
empties the stack; `TestRecoveryStack_Unwind_RetainsJournalOnFailure` pins the retain-vs-clear rule; and
`TestRecoveryStack_CompensationError_RoundTrips` proves `compensation_error` survives a trace save/load in both json and
yaml.

The contract's third per-entry category — **not-yet-reached** — cannot occur at the stack level: the unwind is
best-effort-complete (R3), so every entry is attempted (compensated or failed). It is a derivable *journal* category
(planned nodes that never dispatched, which `Trace.Summarize` already tallies as `skipped`) — a client concern, not a
framework entry-state.

**Out of scope (settled 2026-07-13):** the `internal/cli.WriteTrace` invocation, restart-instruction rendering, the
resume command, and any CLI run-path wiring — all client-owned.

**Landed 2026-07-18 (the final item — the resumed pure unwind).** The flagged audit ran and found the path NOT
built: `ResumeExecutor` restored state but `Run` refuses any non-paused resume, and `Transition`'s doc explicitly
deferred the de-escalation here. Now built as **`GraphExecutor.ResumeUnwind(ctx)`** — the Restart contract
realized: it requires `stopped × compensation_failed`, restores the resumed state (ledger rehydrate + stack
re-arm, mirroring Run's resumed branch — the paths stay separate because Run resumes FORWARD execution, which
this contract forbids for a compensation-failed trace), and re-runs the retained journal's compensations against
the live filesystem — the framework observes rather than assumes (operator-cleared state no-ops through the
compensators' own tolerance). A clean unwind clears the journal and performs the one sanctioned downward
condition move — `stopped × execution_failed` under the new **`ReasonUnwound`** token, journaled in
`Trace.Transitions`; a dirty unwind stays at `compensation_failed` with fresh diagnostics retained. Pinned by
`TestResumeUnwind_{CleanSecondPass_DeEscalates,StillDirty_StaysCompensationFailed,RefusesWrongState}` (the first
via a new flaky fixture whose compensation fails once then succeeds — the operator-cleared-the-blocker shape).
The stale test names also swept: `ReachesFailedCompensation` → `ReachesCompensationFailed`,
`CleanUnwind_ReachesFailed` → `CleanUnwind_ReachesExecutionFailed`.

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
