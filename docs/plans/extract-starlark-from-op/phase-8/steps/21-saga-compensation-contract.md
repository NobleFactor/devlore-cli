---
step: 21
former_step: 18.6
title: "SAGA failure-handling & compensation-failure contract"
status: draft — contract stated; realization not started
proof_run: n/a (draft)
parent: ../../phase-8.md
---

# Step 21 — SAGA failure-handling & compensation-failure contract (formerly 18.6)

**Status:** `draft`. The contract is stated; its realization is unscheduled. Cross-cutting: it governs the deploy's
failure semantics for every graph.

## The contract (as drafted)

1. **Four run terminals:** `Completed` / `Degraded` / `Failed` / `Stranded`.
2. **Error actions MUST run** — a declared `error_action` is not best-effort.
3. **A failed `Compensate` → `Stranded`:** fail loud, journal the `Trace`, and support restart from it. Compensation
   failure is never swallowed into a `Failed` terminal.

## Design and history

The draft lives in [phase-8/compensation-failure-contract.md](../compensation-failure-contract.md). The pre-renumber
lineage called this "step 21.6" in [3.4-platform-package-managers.status.md](../../../architecture/3.4-platform-package-managers.status.md)
(rewritten to the current numbering in the same audit, group 4).

## Note

The name collision with the historical "graph-immutability seal" (`phase-8/graph-immutability.md`) is coincidental:
that work predates the current table; its citations were rewritten name-based in the 2026-07-03 audit.
