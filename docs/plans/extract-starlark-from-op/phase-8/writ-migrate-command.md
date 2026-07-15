---
title: "writ migrate — command plan & design (step-33 slice B)"
status: IMPLEMENTED (2026-07-15, step 33 slice B) — Execute one-run restructure (slice-1 groundwork, kept) + migrate.RegisterLayer two-run registration on the common-ancestor root + session/batch convergence on Execute + cli.WriteTrace receipts in both modes; file_ops.go deleted; registration tests green
created: 2026-07-15
parent: steps/33-writ-migrate-rewrite.md
---

# writ migrate — plan & design

## Purpose

Migrate an existing environment repository (dotfiles) to a writ layer. Five phases:

1. **Analyze** — AI-assisted: auto-detect the source system (Tuckr, Stow, chezmoi, yadm, bare git, script-based),
   classify files, detect secrets (unencrypted credentials / keys, git-crypt → SOPS recommendations), derive package
   manifests from setup scripts, validate already-writ-compatible layouts. Produces the restructure graph + the
   `MigrationAnalysis`. Guarded by the `.writ-migrated` marker (refuses to re-run).
2. **Present** — `--dry-run` renders the plan (`text` / `yaml` / `json`, plus the AI `explain` narrative).
3. **Restructure** — execute the rename/copy/remove graph that reshapes content to writ conventions (`Home/`,
   `System/`, projects).
4. **Mark + receipt** — write the `.writ-migrated` marker (the human-facing rename record); persist the run's trace.
5. **Register the layer** — the source becomes the layer: `--link` (default; layer dir symlinks to the source) or
   `--move` (content moves into the layer dir), preceded by the clear-existing-layer guard (remove a symlink or
   empty dir at the layer path; refuse a non-empty dir).

Interactive mode (default on a TTY) runs a console session — analysis, AI-narrated plan, user confirmation, optional
AI-driven plan edits — and batch mode (`--non-interactive` / no TTY) runs straight through. **Both converge on the
same execution path.**

## CLI surface (kept as-is)

```
writ migrate [--link|--move] [--layer personal|team|base] [--format json|yaml|text]
             [--system <override>] [--non-interactive] [--tree-depth N] [--script-budget N] <source-directory>
```

Root flags honored: `--dry-run`, `--verbose`, and the model-provider flags (`--model`, `--model-api-key`,
`--model-endpoint`, `--model-provider`).

## Current state

**Kept (framework-agnostic, sound):** the analysis pipeline — `GatherInputs` (tree scan under depth/script budgets)
→ registry-loaded prompt → LLM → `parseRegistryLLMResponse` → `registryExecutionGraph` → `buildGraphFromRegistry` →
`planBuilder` (immediate-bound `file.move` / `mkdir` / `copy` / `remove` invocations + `DependsOn` ordering →
`AssembleDefinition`), plus `MigrationAnalysis` and its formatters. The interactive session's console flow and
AI-edit loop.

**Thrown away (the ancient pattern):**

1. The strip-mining `Execute`: filtered `file.move` nodes, peeked slot literals via the dead `op.ImmediateOf`
   (reading `path`, a slot the planned nodes do not even carry), and re-dispatched every rename through a **fresh
   one-node graph** (`file_ops.go` `Move`) — the assembled graph was never run. Rewritten in-tree (pre-ruling
   slice 1, kept): one `GraphExecutor.Run` over the whole graph, conflict precheck retained (`file.move`
   archive-overwrites, so a pre-existing target must refuse the run), marker + trace return.
2. `file_ops.go` — the one-node-graph `Mkdir` / `Move` / `Link` helpers with `Application.Flags` +
   `plan.variable` plumbing for plan-time-known values. Deleted; the layer registration is rebuilt (below).
3. The phantom `cli.WriteReceipt` call — replaced by `cli.WriteTrace(trace)` (client-owned persistence, step-21 R4;
   the failed run's journal survives).
4. `session.executeStep`'s divergent execution copy — converges on `Execute`.

## Target design

1. **Restructure** (landed shape): `Execute(ctx, graph, analysis) (*op.Trace, error)` — empty-graph short-circuit;
   conflict precheck over `file.move` targets (reads `destination_path` immediates via the reporting helper); one
   `GraphExecutor.Run` under a root confined at the source; post-run per-rename report; `.writ-migrated` marker
   (rename list read from the graph — the one legitimate reporting inspection); trace returned for the caller to
   persist.
2. **Layer registration as one graph.** `--link`: `file.mkdir(<layers-parent>)` → `file.link(source=<sourceRoot>,
   target_path=<layerDir>)`. `--move`: `file.mkdir` → `file.move(source=<sourceRoot>,
   destination_path=<layerDir>)`. Immediate-bound, planned via `plan.Provider`, one Run, trace persisted. The
   `clearExistingLayer` guard stays Go-side (a precondition, not a deployable action).
3. **One execution path**: `runMigrateBatch` and the interactive session's execute step both call `Execute` and then
   the registration graph; receipts via `cli.WriteTrace` in both modes.

## Deletions — done 2026-07-15

1. `cmd/writ/writ/migrate/file_ops.go` (whole file: `Mkdir` / `Move` / `Link` / `runFileOp` /
   `buildSingleOpGraph` / `buildMigrateSpec` / the package-level `*op.Variable` set).
2. The old `Execute` body (replaced by the one-run shape) and `session.executeStep`'s divergent copy (converged on
   `Execute`; its repo-local `.writ-migrate-receipt.json` write is replaced by `cli.WriteTrace` like batch mode).
3. `clearExistingLayer` / `linkToLayer` / `moveToLayer` in `migrate_cmd.go` — the guard moved into the migrate
   package beside `RegisterLayer`; both the batch and interactive call sites now register through it.

## Landed shape (2026-07-15)

`migrate.RegisterLayer(ctx, sourceRoot, layerDir, useMove, verbose)` — the Go-side `clearExistingLayer` guard, then
one immediate-bound graph (`file.mkdir(<layers-parent>)` → `file.link` or `file.move`) planned via `plan.Provider`
under a root confined at `commonAncestor(sourceRoot, layerDir)`, one run, trace persisted win-or-lose. Tests:
`commonAncestor` units, the four guard behaviors, link mode (layer symlinks to the source, content readable through
it, source untouched), move mode (content moved, source gone), and the occupied-layer refusal.

## Test plan

1. End-to-end batch migrate over a `testdata` fixture: analysis mocked/canned → restructure graph runs as one graph
   → marker written → trace persisted → layer registered (link mode) — with rollback on induced failure.
2. Conflict precheck: pre-existing rename target refuses the run before any node dispatches.
3. `--move` registration; `clearExistingLayer` guard behaviors (symlink removed, empty dir removed, non-empty
   refused).
4. Reshape `receipt_integration_test.go` + `session_test.go` onto the converged path.

## Settled (2026-07-15)

1. **Registration-graph root confinement: the deepest common ancestor** of the resolved `cli.WritLayersDir()`-based
   `layerDir` and the absolute `sourceRoot` — computed over actual paths, never an assumed `$HOME` (a relocated
   `XDG_DATA_HOME` plus a home source needs an ancestor above both). Typical case confines at `$HOME`; degradation
   toward `/` happens only when the two trees genuinely span that far. (The old code anchored at
   `filepath.Dir(layerDir)` and then moved/linked the source — outside its own confined root; those paths were
   never exercised.) Context ruled in: the layer tree is `<layers>/{base,team,personal}/{Home,System}/<project>/…`
   with `personal` possibly being the link-mode symlink to the source repo itself.
2. **Two runs.** Registration stays its own graph after the restructure + marker. Rationale: a failed registration
   must NOT unwind the completed restructure (independently valuable; retryable alone); confinement stays tight per
   phase (restructure at `sourceRoot`, registration at the common ancestor); the marker write and the
   `clearExistingLayer` guard sit naturally between the runs; two traces for two operations.
3. **The AI surfaces are kept verbatim** — `FormatMigrationExplain` and the interactive AI-edit loop
   (`applyGraphModifications` + graph re-derivation) are analysis/presentation-layer and framework-agnostic;
   redesigning them would be product work smuggled into a framework migration. The only adjacent change is the
   execution seam: `session.executeStep` converges on `Execute`.
