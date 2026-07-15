---
step: 33
former_step: 30
title: "writ migrate — full rewrite onto the sealed-graph executor"
status: in-progress — REWRITE PLAN drafted 2026-07-15 (user ruling: migrate/adopt were written against an ancient framework; sus out intent, discard, rewrite clean); slice-1 API unblocking landed in-tree (AssembleDefinition ×3, ImmediateOf gone, Execute onto GraphExecutor.Run + trace return, WriteTrace at 3 phantom WriteReceipt sites); slices A–D below pending approval; the deploy-family (StateView) crater is OUT of this step and needs its own charter
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 33 — writ migrate full rewrite (formerly 30)

**Status:** `not-started`. **Full rewrite, not an incremental fix** (charter set 2026-06-19; extracted here from the
phase-8 table cell, 2026-07-03 audit).

## Per-command plan & design documents (drafted 2026-07-15 — awaiting review)

Ruled 2026-07-15: slice C lies until A, B, and D land (then specced and rewritten); A, B, D are fed one at a time;
each command gets a detailed plan & design document first:

1. [writ-adopt-command.md](../writ-adopt-command.md) — slice A (gather + field projection; consumes
   [step 45](45-field-projection.md)). **LANDED 2026-07-15**: the batch machinery lives in the adopt package
   (`Collect`/`RunBatches`/`BuildGraph`), five behavioral tests green, `adopt/execute.go` and the writ-package test
   twin removed.
2. [writ-migrate-command.md](../writ-migrate-command.md) — slice B. **LANDED 2026-07-15**: one-run restructure
   (`Execute`) + two-run registration (`migrate.RegisterLayer` on the common-ancestor root) + session/batch
   convergence; `file_ops.go` deleted; registration tests green.
3. [writ-verify-command.md](../writ-verify-command.md) — no standalone command exists today (the helper serves
   reconcile); largest open-question surface, sequencing itself an open question.

## Rewrite plan (drafted 2026-07-15 — awaiting approval)

**Ruling (2026-07-15):** migrate/adopt were written against an ancient framework version. Sus out the intent, throw
away the old code, rewrite the commands clean on the current framework. Patching (the original slice-1 lane) is the
wrong approach; what slice 1 produced that coincides with the rewrite is kept, the rest of the old structure goes.

### Intent — writ migrate (what the command IS)

1. **Analyze** — AI-assisted analysis of a source repo (auto-detect Tuckr / Stow / chezmoi / yadm / bare git /
   scripts; classify files; detect secrets; manifest generation from setup scripts; recommendations). Produces the
   restructure graph + `MigrationAnalysis`. Guarded by the `.writ-migrated` marker. **Framework-agnostic — kept.**
2. **Present** — dry-run formats (text / yaml / json / AI explain). Kept (reporting reads slots via the public
   binding API).
3. **Restructure** — run the assembled rename graph **as one graph** (SAGA semantics: a failed migration rolls back).
4. **Mark + receipt** — write `.writ-migrated`; the client persists the run's trace (`cli.WriteTrace`), success or
   failure (the step-21 R4 stance).
5. **Register the layer** — layer dir symlinked to the source (`--link`, default) or the source moved into the layer
   (`--move`), with the clear-existing-layer precondition (remove a symlink / empty dir; refuse non-empty).
6. Interactive (console session, step confirmation, AI plan edits) and batch share ONE execution path.

### Intent — writ adopt

1. Enumerate items (files, or directories walked recursively; symlinks skipped), infer scope per item (Home under
   `$HOME`, else System), destination `<layer>/<scope>/<project>/<relpath>`.
2. Per file: mkdir destination dir → move the file → symlink back at the original location.
3. Dry-run narration; adoption summary; `--from-receipt` remains a not-implemented stub.

### The ancient-framework disease (why rewrite, not patch)

- **N graphs per command.** adopt builds a three-node graph + TWO runtime specs + a full plan/run cycle PER FILE;
  migrate's layer registration mints a fresh single-node graph per op (`migrate.Mkdir` / `Move` / `Link`).
- **Fake variable plumbing.** Values known at plan time are threaded through `Application.Flags` + `plan.variable`
  references because the ancient Go surface had no immediate bindings; today `plan.Provider.Plan` takes immediates
  directly (plan_builder already does).
- **Forfeited SAGA semantics.** One graph per file means a mid-batch failure leaves half-moved files with no
  rollback; the framework's whole recovery model goes unused.
- **Legacy error-prefix preservation** (`adopt.Run` / `mapAdoptError` / `firstJoinedError`) — dead weight under the
  greenfield rule.

### Rewrite shape (current idioms: one plan → one graph → one Run → one trace)

**Slice A — adopt.** One graph per scope group (Home and System have different confined roots): for each adopted
file, a mkdir → move → link chain of immediate-bound invocations, ordered within the graph; ONE
`op.GraphExecutor.Run` per group; the trace persisted via `cli.WriteTrace` even on failure; compensation rolls back a
failed batch. DELETE `adopt/execute.go` (Run / mapAdoptError / firstJoinedError) and the per-file dual-spec plumbing
in `adopt_cmd.go`; `adopt.BuildGraph` becomes batch-shaped (`env`, the item list → the group's graph). Enumeration,
scope inference, symlink-skip, and dry-run narration are intent — kept.

**Slice B — migrate execution + registration.** The analysis pipeline is untouched. `Execute` is the slice-1 rewrite
(one run over the assembled graph + conflict precheck + marker + trace return) — kept. The layer registration
(`linkToLayer` / `moveToLayer`) becomes one small immediate-bound graph (mkdir parent → link-or-move), one Run;
`file_ops.go` (the one-node-graph helpers + fake flag plumbing) is DELETED; `clearExistingLayer` stays a Go-side
precondition guard. `session.executeStep` and `runMigrateBatch` converge on `Execute` (one execution path).

**Slice C — the deploy-family crater (NOT this step, needs a ruling).** `commands.go` / `graph_builder.go` build
writ deploy/upgrade/decommission/status on the DELETED `execution.StateView` subsystem (19 + 5 references) and use
`*op.ReceiverRegistry` as a type in 7 signatures. Rebuilding "what did writ deploy" on the trace/receipt model is
its own design + charter. Until it lands (or is loudly stubbed), `cmd/writ` cannot compile even with migrate/adopt
perfect.

**Slice D — verify + close.** `make test` runs the writ family for the first time in weeks; reshape
`adopt_integration_test` / `receipt_integration_test` / `session_test` onto the new shapes; step 18 + 22 close when
the writ build is green; docs + master rows.

### Kept from the pre-ruling slice 1 (in-tree, uncommitted)

The `AssembleDefinition` renames (×3), the `immediateString` reporting helper (+ `helpers.go`), the `Execute` rewrite
onto `GraphExecutor.Run` with the trace return, the `cli.WriteTrace` persistence at the three phantom
`cli.WriteReceipt` sites, and the session/format slot fixes (`destination_path` — the old code read `path`, a slot
that does not exist on the planned move nodes). `file_ops.go`'s patched `AssembleDefinition` call is moot — the file
is deleted by slice B.

## Problem

## Problem (original charter)

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
