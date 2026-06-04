---
title: "Framework SAGA failure-handling & compensation-failure contract"
status: draft
created: 2026-06-04
updated: 2026-06-04
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
| `RecoveryStack.Unwind()` — LIFO `Compensate`; wraps a compensation error → `RunStateFailed` | ✅ | ✅ |
| `RunState{Pending,Running,Paused,Degraded,Completed,Failed}` | ✅ | partial |
| `flow.Failed` / `flow.Complete` / `flow.Degraded` terminal nodes | ✅ | ✅ |
| `ExecutableUnit.ErrorAction() *Subgraph` — per-unit failure handler | ✅ | ❌ **never dispatched** |
| `RunStateDegraded` transition | ✅ (defined) | ❌ **never assigned** |
| Distinct terminal for compensation failure | ❌ | ❌ |
| Journal persistence on failure + restart-instruction generation | ❌ | ❌ |

## The run-outcome model — four terminals

| Terminal | Meaning | System state | Recovery stack |
|---|---|---|---|
| **Completed** | every unit clean | consistent | — |
| **Degraded** | a unit failed; its `ErrorAction` handled it (reached `flow.Degraded`) | consistent, partial | failures recorded; successes kept |
| **Failed** | a unit failed unhandled; the stack unwound **cleanly** | consistent (pre-run) | fully compensated |
| **Stranded** | unhandled failure **and** unwind itself failed | **dirty** | partially compensated; journal saved |

`Completed` and `Failed` exist today. `Degraded` and `Stranded` are what this contract wires.

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
  compensation errors. (One stuck rollback cannot strand the others.)
- **All `Compensate` succeed → `Failed` (consistent).** The system is back at its pre-run state.
- **Any `Compensate` returns an error → `Stranded`.** The system is dirty — a forward action failed
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
`Stranded` case only**: that trace is persisted as a restartable journal.

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
- **R2 — A failed `Compensate` MUST produce `Stranded`** with the fail-loud + journal + restart-
  instructions response, uniformly across every provider. `Compensate` returns `error` precisely so this is
  detectable.
- **R3 — Unwind is best-effort-complete** — one failed compensation does not skip the rest; all compensation
  errors are aggregated and reported.
- **R4 — The journal (`Trace`) MUST be persisted on `Stranded`** to enable restart.

## Provider conformance (pkg, file, service, …)

A provider's only obligations: be **best-effort** within a call (attempt every item, collect one receipt each),
return `error` when any item failed (so `ErrorAction`/unwind can act), and return a **faithful per-receipt error**
from `Compensate` when an undo fails. Providers contribute no failure-handling logic of their own. For `pkg`, the
leaf attempts all packages, returns `(receipts, error-if-any-failed)`, and never self-rolls-back — the framework
decides the consequence.

## To build

1. **Dispatch `ErrorAction`** in the executor on unit failure (R1) — currently never invoked.
2. **Transition to `RunStateDegraded`** when an `ErrorAction` reaches `flow.Degraded`; continue execution.
3. **Distinct `Stranded` terminal** — a new `RunState` member, peer to `Failed`.
4. **Best-effort-complete unwind** with aggregated compensation errors (R3).
5. **Persist the journal** on `Stranded` (R4) and **generate restart instructions**.

## Decided

- **The fourth terminal is `Stranded`** — a peer `RunState` member alongside `Completed` / `Degraded` / `Failed`
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

- **`Stranded` resumes as a state-checked unwind, not a forward retry** (Q1) — re-query each resource and undo only
  what is still present (robust to operator cleanup; observe, don't assume); land at `Failed` (clean baseline); no
  auto-retry forward by default (opt-in only). Detail in *Restart*.

## Open questions

_All resolved (2026-06-04)._

## Relationships

- Pairs with `terminal-flow-control` (owns `Complete`/`Degraded`/`Failed` terminal semantics).
- Referenced by `platform-unification.md` and `pkg-install-reconciler.md` (providers conform; they do not restate).
