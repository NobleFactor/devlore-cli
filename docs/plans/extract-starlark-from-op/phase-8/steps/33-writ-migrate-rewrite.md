---
step: 33
former_step: 30
title: "writ migrate — full rewrite onto the sealed-graph executor"
status: not-started — full rewrite, not an incremental fix; owns the remaining build reds in step 18's gate
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 33 — writ migrate full rewrite (formerly 30)

**Status:** `not-started`. **Full rewrite, not an incremental fix** (charter set 2026-06-19; extracted here from the
phase-8 table cell, 2026-07-03 audit).

## Problem

`cmd/writ/writ/migrate` reimplements graph execution by hand instead of running the assembled graph. `Execute`
(`migrate/execute.go`, called from `migrate_cmd.go:213`) filters the graph's `file.move` nodes into a `renameNodes`
worklist, strip-mines each node's `path`/`source` literals via `op.ImmediateOf(node.Slots()[…])`, and re-dispatches
every rename through `Move()` (`migrate/file_ops.go`), which builds its *own* single-node graph and runs that — so
the assembled graph is never executed as a graph; instead N one-node graphs are constructed and run. The
target-exists conflict precheck is hand-rolled the same way.

## The correct pattern already exists in the same package

`migrate/session.go:556-572` runs `s.graph` via `op.NewGraphExecutor(...).Run(...)` and writes `executor.Trace()` as
the receipt.

## Rewrite target

1. Collapse `Execute` onto `GraphExecutor.Run(graph, spec)` (the `session.go` path), deleting the `renameNodes` loop
   and its slot-peeking.
2. The target-exists check becomes a real preflight pass rather than literal reads.
3. The only legitimate remaining graph-slot inspection is the human-facing `.writ-migrated` marker (reporting).
4. Fold in the migrate-package `op.ImmediateOf` callers (`execute.go` / `format.go` / `session.go`) as part of the
   broader `ImmediateOf` decision.
5. Adopt the renamed `plan.Provider.*Definition` methods (`Assemble` → `AssembleDefinition`, per step 13).

## Current build-red inventory (2026-07-03 — the step-18 gate's residue this step owns)

1. `cmd/writ/writ/migrate` — `op.ImmediateOf` undefined (`execute.go`, `format.go`).
2. `cmd/writ/writ/adopt` — `planProvider.Assemble` undefined (`plan.go:73`; the `*Definition` rename).
3. `cmd/writ`, `cmd/writ/writ` — transitive.
4. `cmd/docgen` (imports `cmd/writ/writ` at `main.go:17`), `internal/e2e` — transitive.

Step 18's exit gate closes mechanically when this step and step 28 land. Closing this step also closes the last open
item of step 22 (13.0(n), the writ graph executor).
