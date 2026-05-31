---
title: "Rename op → workflow with Definition/Node/Block/Step taxonomy"
issue: TBD
status: draft
created: 2026-05-26
updated: 2026-05-30
---

# Plan: Rename op → workflow

## Summary

Rename the `op` package to `workflow` and adopt a coherent flow-control vocabulary across its core types. `ExecutableUnit` becomes `Node` — the abstract vertex — and its two variants become `Step` (the leaf, was `Node`) and `Block` (the composite, was `Subgraph`). The durable execution record `RecoveryStack` becomes `Ledger`. The container/driver pair `Graph` / `GraphExecutor` becomes `Definition` / `Executor`, making the intent-vs-reality dichotomy explicit at the type level.

The family reads as **`Definition ▸ Node ▸ {Block, Step}`**: a Definition is a DAG of Nodes; every Node is either a `Block` (a container whose action is structural — `flow.Provider.Gather`, `choose`, `each` — and whose responsibility is flow control over its `Children`) or a `Step` (a leaf whose action wraps a concrete domain method — `file.Provider.Copy` — with its `Do` and compensating `Undo`).

This is a pure rename project. No behavior changes, no signature changes, no scope creep beyond the listed renames.

## Goals

1. **Honest package name.** `workflow` accurately names what the package contains (build-once, run-many executions with pause/restart/reconciliation and a durable audit trail). `op` is opaque shorthand.
2. **Coherent type taxonomy.** `Node` is the abstract vertex; `Step` (leaf) and `Block` (composite) are its variants. `Edge` connects Nodes. The hierarchy reads cleanly from container to leaf, and `Block`/`Step` borrow program-structure vocabulary that reads natively to a DevOps engineer.
3. **Intent ↔ reality at the type level.** `Definition` (intent) ↔ `Ledger` (reality), with `Executor` driving the transition. Three nouns describe the whole system.
4. **No behavioral or signature changes.** Each phase is a pure rename so reviewers can verify by inspection.

## Prerequisites

This work cannot start until the in-flight upstream PR (`refactor/extract-starlark-from-op.phase-8`) merges to `develop`. Starting earlier creates intractable merge conflicts in `pkg/op/`.

The plan stays in `draft` status until then. Branch creation, GitHub issue creation, and the first phase begin only after develop is current.

## Current State (audited 2026-05-26)

| Element | Current | Location | Notes |
|---|---|---|---|
| Package | `pkg/op/` | — | 62 .go files at root |
| Subpackages | `provider/`, `starlarkbridge/`, `inventory/`, `sops/` | — | 76, 5, 2+1.gen.go, 13 files |
| Abstract vertex | `ExecutableUnit` (interface) | `executable_unit.go:29` | → `Node` |
| Leaf variant | `Node` (struct) | `graph.go:380` | has `Layer` + `Origin` fields → `Step` |
| Composite variant | `Subgraph` (struct) | `subgraph.go:28` | contains `Children []SubgraphChild` → `Block` |
| Durable record | `RecoveryStack` (struct) | `recovery_stack.go:16` | → `Ledger` |
| Recovery anchor | `RecoverySite` (struct) | `recovery_site.go:32` | name stays — see Out of Scope |
| Container | `Graph` (struct) | `graph.go:40` | → `Definition` |
| Runtime driver | `GraphExecutor` (struct) | `graph_executor.go:32` | → `Executor` |
| Connection | `Edge` (struct) | `graph.go:368` | planning-time |
| Slot value | `SlotValue` (interface) | `slot.go:14` | no concrete `Slot` type exists |
| Durable entry | `Receipt` (interface) | `receipt.go:20` | |
| Importing files outside `pkg/op/` | ~200 | spans `cmd/`, `internal/` | |
| Qualified `op.X` references | ~2,499 | non-test + test combined | |
| Generated files in scope | 29 `.gen.go` | `pkg/op/provider/*/gen/` + `pkg/op/inventory/` | regenerate via `make build` |
| Name collisions for new names | `Step` (cross-package only) | repo-wide | `Block`, `Ledger`, `Workflow`, `Definition`, `Executor` are all unused elsewhere. `Step` has no language-level clash: the only existing type is `console.Step` in `internal/console` (a TUI session step, different package); the `pkg/op` "Step" hits are comment prose ("Step 3 fires…"), not identifiers. |

## Taxonomy Target

```
workflow
├─ Definition             root container       (was Graph)
│  ├─ Node                abstract vertex      (was ExecutableUnit; interface)
│  │   ├─ Step            leaf variant         (was Node — wraps a domain method, Do/Undo)
│  │   └─ Block           composite variant    (was Subgraph — structural action over Children)
│  └─ Edge                planning-time connection
├─ Ledger                 durable execution record   (was RecoveryStack)
│  ├─ Receipt             durable entry              (unchanged)
│  └─ Ledger              nested sub-ledgers
├─ Executor               runtime driver       (was GraphExecutor)
└─ RecoverySite           recovery anchor      (unchanged — name is accurate as-is)
```

## Rename Mapping

| # | From | To | Kind | Status |
|---|---|---|---|---|
| 1 | `Node` (type) | `Step` | type rename | confirmed |
| 2 | `ExecutableUnit` (type) | `Node` | type rename + file rename | confirmed |
| 3 | `RecoveryStack` (type) | `Ledger` | type rename + file rename | confirmed |
| 4 | `Graph` + `GraphExecutor` + `Subgraph` (types) | `Definition` + `Executor` + `Block` | container/composite rename + file renames | confirmed |
| 5 | package `op` | package `workflow` | package + directory move | confirmed |

Sequencing is collision-safe: Phase 1 must precede Phase 2 (frees the name `Node`). All other phases are independent.

## Implementation Phases

Each phase ships as its own PR. Each PR merges before the next starts (per the "never accumulate PRs" rule). Suggested sub-branch naming follows the existing pattern: `refactor/workflow-rename.phase-1`, `.phase-2`, etc.

### Phase 1: Node → Step

Single concept rename of the leaf struct. Frees the name `Node` for Phase 2.

**Targets:**
- `pkg/op/graph.go:380` — struct declaration carrying `Layer` + `Origin` fields
- Internal symbols that ripple: `NewNode → NewStep`, `GenerateNodeID → GenerateStepID`, `NodeID → StepID`, `node`-named locals
- All consumer references `op.Node` → `op.Step`

**Files:**
- `pkg/op/graph.go` — modify
- ~200 importing files outside `pkg/op/` — modify qualified references

### Phase 2: ExecutableUnit → Node

Single symbol rename. Safe because Phase 1 freed the name. `Node` becomes the abstract interface satisfied by both `Step` and `Block`.

**Targets:**
- `pkg/op/executable_unit.go:29` — interface declaration
- `executableUnitType` (reflect.Type cache, `planner.go:16`) → `nodeType`
- All consumer references `op.ExecutableUnit` → `op.Node`

**Files:**
- `pkg/op/executable_unit.go` → `pkg/op/node.go` (rename)
- Consumer files — modify qualified references

### Phase 3: RecoveryStack → Ledger

Single symbol rename. Independent of Phases 1 and 2.

**Targets:**
- `pkg/op/recovery_stack.go:16` — struct declaration
- All consumer references `op.RecoveryStack` → `op.Ledger`
- Private helpers (e.g., `recoveryEntry`) renamed to match where it improves clarity

**Files:**
- `pkg/op/recovery_stack.go` → `pkg/op/ledger.go` (rename)
- Consumer files — modify qualified references

### Phase 4: Graph → Definition + GraphExecutor → Executor + Subgraph → Block

Container/composite rename. These move together: `GraphExecutor` stops making sense once `Graph` is gone, and `Subgraph` is the composite `Node` variant whose vocabulary belongs with `Definition`/`Step`. A `Step`'s failure handler is a `Block` — the `errorAction *Subgraph` parameters become `errorAction *Block`.

**Targets:**
- `pkg/op/graph.go:40` — `Graph` struct declaration
- `pkg/op/graph_executor.go:32` — `GraphExecutor` struct declaration
- `pkg/op/subgraph.go:28` — `Subgraph` struct declaration (and `SubgraphChild` → `BlockChild` where it improves clarity)
- `Planner.Plan` / `ActionPlanner.Plan` signatures: `errorAction *Subgraph` → `*Block` (`planner.go:97,165`)
- All consumer references `op.Graph` → `op.Definition`, `op.GraphExecutor` → `op.Executor`, `op.Subgraph` → `op.Block`

**Files:**
- `pkg/op/graph.go` → `pkg/op/definition.go` (rename — the `Graph`/`Definition` struct dominates the file)
- `pkg/op/graph_executor.go` → `pkg/op/executor.go` (rename)
- `pkg/op/subgraph.go` → `pkg/op/block.go` (rename)
- Consumer files — modify qualified references (the second-largest blast radius after Phase 5)

### Phase 5: package op → workflow

Largest blast radius. Use JetBrains "Move package" to update all importers and qualified references atomically.

**Targets:**
- Directory move: `pkg/op/` → `pkg/workflow/`
- Subpackages move with the directory: `pkg/workflow/{provider,starlarkbridge,inventory,sops}/`
- All ~200 importing files: import path `github.com/.../pkg/op` → `github.com/.../pkg/workflow`
- All ~2,499 qualified references: `op.X` → `workflow.X`
- 29 `.gen.go` files regenerated via `make build`

**Files:** every Go file in the repo that imports or references `op`.

**Starlarkbridge constraint:** I never edit `pkg/workflow/starlarkbridge/` contents directly. Required edits there are staged at the new path for the user's inspection.

## Per-Phase Address-Breaks Pattern

Every phase follows the same loop:

1. **You rename in JetBrains** (`Refactor → Rename` for symbols; `Refactor → Move` for the package).
2. **`make check`** runs build, vet, lint, complexity, tests.
3. **You share the failure output** (paste, or push to a branch I can read).
4. **I propose targeted fixes** with file:line precision. Per the no-consumer-edits-without-consult rule, I surface fixes; you apply or approve before I apply.
5. **`make check` passes** → PR opens, reviews, merges → next phase starts.

Phase 5 adds one step before `make check`: run `make build` first so `.gen.go` files regenerate against the new package path.

## Verification (per phase)

- [ ] `make build` passes
- [ ] `make check` passes (build, vet, lint, shell-lint, complexity, test)
- [ ] No new lint warnings
- [ ] `rg --word-regexp '\bOldName\b' -- '*.go'` returns zero hits (excluding `.gen.go` if regeneration is pending)
- [ ] Doc comments referencing the old name addressed in the docs audit (Phase 5 close-out)

## Documentation Audit (after final phase merges)

After all phases land:

1. **`docs/architecture/**/*.md`** — every file mentioning `op`, `Node`, `ExecutableUnit`, `Subgraph`, `RecoveryStack`, `Graph`, `GraphExecutor` updated to the new vocabulary.
2. **`docs/plans/**/*.md`** — every plan referencing old names updated. Completed plans documenting historical state may stay (they are a record); in-flight plans must update.
3. **Doc comments in `pkg/workflow/**/*.go`** — every comment mentioning old names updated. Per the standards-apply-to-generated-and-tests rule, this includes test files; generated files update on the next codegen pass.
4. **Test names and table-case labels** — every `t.Run("op_...")` or similar renamed.
5. **`CLAUDE.md` and root README** — any references updated.

Method: I grep each old name → enumerate locations → produce one batched edit per file (per the JetBrains focus-loss preference) → you review the diff per file.

## Out of Scope (deferred)

- **`RecoverySite` rename.** The name is accurate as-is (a persistent stash from which content can be recovered). Family-coherence with `Ledger` is not a reason to rename.
- **Materializing a concrete `Slot` type.** Currently only `SlotValue` interface exists. Promoting `Slot` to a real type is a design change, not a rename — separate effort.
- **Other vocabulary cleanup.** Anything not in the rename mapping above.

## Open Questions

- [ ] **Q2: Subpackage edit bans beyond `starlarkbridge/`?** `provider/`, `inventory/`, `sops/` move with the directory in Phase 5. The starlarkbridge ban means I never edit its contents directly. Same treatment for the others, or are they edit-eligible?
- [ ] **Q3: GitHub issue.** Per the standard workflow, an issue tracks this plan. Issue number goes in frontmatter once created (post-prerequisite-merge).
- [ ] **Q4: Plan template adherence.** This plan deviates slightly from `docs/plans/TEMPLATE.md` (richer phase descriptions; no Migration Path section since rename has no user-facing migration). Confirm acceptable.

## Files to Create/Modify

| File / Path | Action | Phase |
|---|---|---|
| `pkg/op/graph.go` | Modify (Node → Step type) | 1 |
| `pkg/op/executable_unit.go` → `pkg/op/node.go` | Rename + modify | 2 |
| `pkg/op/recovery_stack.go` → `pkg/op/ledger.go` | Rename + modify | 3 |
| `pkg/op/graph.go` → `definition.go`, `graph_executor.go` → `executor.go`, `subgraph.go` → `block.go` | Modify + rename | 4 |
| `pkg/op/` → `pkg/workflow/` | Directory move | 5 |
| ~200 importing files | Modify imports + qualified refs | each phase touches some; Phase 5 touches all |
| 29 `.gen.go` files | Regenerate via `make build` | 5 |
| `docs/architecture/**/*.md` | Modify vocabulary | post-final-phase |
| `docs/plans/**/*.md` (in-flight) | Modify vocabulary | post-final-phase |
| `CLAUDE.md`, root README | Modify references | post-final-phase |

## Related Documents

- [`docs/plans/extract-starlark-from-op.md`](./extract-starlark-from-op.md) — predecessor refactor that establishes much of the structure being renamed
- [`docs/architecture/`](../architecture/) — architecture docs requiring update post-rename