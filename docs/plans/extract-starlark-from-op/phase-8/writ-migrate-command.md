---
title: "writ migrate — command plan & design (step-33 slice B)"
status: draft — awaiting review
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

## Deletions

1. `cmd/writ/writ/migrate/file_ops.go` (whole file: `Mkdir` / `Move` / `Link` / `runFileOp` /
   `buildSingleOpGraph` / `buildMigrateSpec` / the package-level `*op.Variable` set).
2. The old `Execute` body (already replaced in-tree) and `session.executeStep`'s divergent copy.

## Test plan

1. End-to-end batch migrate over a `testdata` fixture: analysis mocked/canned → restructure graph runs as one graph
   → marker written → trace persisted → layer registered (link mode) — with rollback on induced failure.
2. Conflict precheck: pre-existing rename target refuses the run before any node dispatches.
3. `--move` registration; `clearExistingLayer` guard behaviors (symlink removed, empty dir removed, non-empty
   refused).
4. Reshape `receipt_integration_test.go` + `session_test.go` onto the converged path.

## Open questions

1. **Registration-graph root confinement.** The registration touches both the source root and the layers directory
   (typically under `$XDG_DATA_HOME`). The old code anchored at `filepath.Dir(layerDir)` and then moved/linked the
   *source* — a path **outside** that confined root, which `fsroot` confinement should refuse (evidence these paths
   were never exercised). Anchor the registration graph at the deepest common ancestor of `sourceRoot` and
   `layerDir`? At `$HOME`? Unconfined?
2. **Should registration join the restructure graph** (one run for phases 3+5, full SAGA across both) or stay a
   second small graph (current design — registration failure after a successful restructure does not unwind the
   restructure)? The marker write between them argues for two runs; confirm.
3. **The AI `explain` format and the interactive AI-edit loop** — kept verbatim, or is either up for redesign in
   this pass?
