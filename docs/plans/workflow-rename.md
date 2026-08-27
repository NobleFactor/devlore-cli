---
title: "Rename op → workflow with Definition/Node/Block/Step taxonomy"
issue: 451
status: ready
created: 2026-05-26
updated: 2026-08-27
---

# Plan: Rename op → workflow

> **Re-audited 2026-08-27** under [#716](https://github.com/NobleFactor/devlore-cli/issues/716). The original
> prerequisite is satisfied, the Current State reflects the tree, and §Appendix: Name Mapping now gives a
> reviewer something to inspect a diff against — which goal 4 promises and the plan previously lacked.
>
> Writing the appendix caught an error already in the plan: Phase 1 listed
> `GenerateNodeID → GenerateStepID`, which is wrong. See §Appendix.
>
> [#715](https://github.com/NobleFactor/devlore-cli/issues/715) renames the provider roles and belongs in the
> same window; see §Related Rename.

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

~~This work cannot start until the in-flight upstream PR (`refactor/extract-starlark-from-op.phase-8`) merges to `develop`.~~ **Satisfied.** That branch is gone from the remote and phase 8 has landed.

The real prerequisite now is [#716](https://github.com/NobleFactor/devlore-cli/issues/716): the plan's own
facts are three months stale, and a phase started from them would be scoped against a package 74% smaller than
the one that exists.

## Current State (audited 2026-08-27)

| Element | Current | Location | Notes |
|---|---|---|---|
| Package | `pkg/op/` | — | 108 .go files at root |
| Subpackages | `claimcheck/`, `inventory/`, `provider/`, `server/`, `starlarkbridge/` | — | 3, 3, 238, 3, 8 files |
| Abstract vertex | `ExecutableUnit` (interface) | `executable_unit.go:39` | → `Node` |
| Leaf variant | `Node` (struct) | `node.go:19` | moved out of `graph.go` since the last audit → `Step` |
| Composite variant | `Subgraph` (struct) | `subgraph.go:32` | → `Block` |
| Durable record | `RecoveryStack` (struct) | `recovery_stack.go:37` | → `Ledger` |
| Recovery anchor | `RecoverySite` (struct) | `recovery_site.go:33` | name stays — see Out of Scope |
| Container | `Graph` (struct) | `graph.go:50` | → `Definition` |
| Runtime driver | `GraphExecutor` (struct) | `graph_executor.go:44` | → `Executor` |
| Connection | `Edge` (struct) | `graph.go:1079` | planning-time; name stays |
| Slot binding | `Binding` (interface) | `binding.go:21` | 3 variants; name stays |
| Durable entry | `Receipt` (interface) | `receipt.go:22` | name stays |
| Serialized forms | `graphData`, `nodeData`, `subgraphData` | `graph.go`, `node.go` | follow their subjects |
| Files importing `pkg/op` outside it | 130 | spans `cmd/`, `internal/`, `pkg/` | |
| Qualified `op.X` references | 3,880 | non-test + test combined | |
| Generated files in scope | 48 `.gen.go` | `provider/*/gen/`, `inventory/` | regenerate via `make build` |
| Name collisions | `Step` only | repo-wide | targets declared nowhere; `Step` only as `console.Step` |

### Subpackage disposition

Neither `claimcheck/` nor `server/` existed when the phases were written.

**`claimcheck/`** — the build gate that holds `+devlore:claim=` directives to their call graphs. Nearly
rename-inert: it hardcodes only `pkg/assert` and `pkg/fsroot` as trust boundaries, and its test loads the
whole module by pattern (`github.com/NobleFactor/devlore-cli/...`), which the rename does not change. Moves
with the directory in Phase 5. One doc-comment example names `"./pkg/op/provider/..."` and needs updating.

**`server/`** — the HTTP/2 wire listener bridging a run's `ControlPlane` to a remote consumer. It uses
`op.ControlPlane`, `op.RunStatus`, and the `op.Control*` family, **none of which this rename touches**, so it
is affected by Phase 5 alone. Moves with the directory.

Both are edit-eligible on the same terms as `provider/` and `inventory/` — see Q2.

### Additions since the last audit, checked for rename impact

Method claims (`MethodClaims`, `ClaimDeterministic`), the run-state machine (`RunStatus`, `Phase`,
`Condition`), the control plane (`ControlPlane`, `ControlCommand`, `ControlEvent`), sealed provider resources,
and the conversion rules (`losesMeaning`, `readAgainstField`, `explainRefusal`) name claims, control, and
conversion — **no graph-structure vocabulary**, and none collides with a target name.

The exception is `reportGraphReach`, added by #677, which the appendix covers with the rest.

### What moved between audits

Recorded so the scale of drift is visible: this plan sat untouched for three months while the package it
renames grew substantially, and a phase scoped from the old numbers would have been badly wrong.

| Element | Audited 2026-05-26 | Audited 2026-08-27 | |
| --- | --- | --- | --- |
| `pkg/op/` root `.go` files | 62 | **108** | +74% |
| `provider/` files | 76 | **238** | +213% |
| `starlarkbridge/` files | 5 | **8** | +60% |
| Generated `.gen.go` | 29 | **48** | +66% |
| Qualified `op.X` references | ~2,499 | **3,880** | +55% |
| Files importing `pkg/op` | ~200 | 130 | methodology may differ |

**The subpackage list is wrong in both directions.** Audited as `provider/`, `starlarkbridge/`, `inventory/`,
`sops/`. Today: `claimcheck/`, `inventory/`, `provider/`, `server/`, `starlarkbridge/` — `sops/` has left, and
`claimcheck/` and `server/` arrived afterwards. Every line reference in the table above has shifted.

**What still holds.** All five renames remain well-formed, and the collision claim survives: `Step` is declared
only in `internal/console`, while `Block`, `Ledger`, `Definition`, `Executor`, and `Workflow` are declared
nowhere. Phase 1 before Phase 2 is still the only ordering constraint.

**New evidence for the target.** `plan.Provider` now exposes `AssembleDefinition`, `LoadDefinition`, and
`SaveDefinition` — all returning `*op.Graph` — and scripts read `plan.assemble_definition(...)`. The API
already calls a graph a *definition* while the Go type is still `Graph`. The rename closes a gap that has
widened on its own since the audit, rather than imposing a new word.

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

### Appendix: Name Mapping (audited 2026-08-27)

Goal 4 promises "each phase is a pure rename so reviewers can verify by inspection." This is the reference a
reviewer inspects against. It covers the **exported, non-test surface of `pkg/op`**; unexported and test
identifiers follow the same rules mechanically, and the hazards below apply to them equally.

#### The rules

1. `Graph` → `Definition`, `GraphExecutor` → `Executor`, `Subgraph` → `Block`, `RecoveryStack` → `Ledger`.
2. **`Node` is ambiguous by construction.** It maps to `Step` *and* is the target of `ExecutableUnit`, so it
   appears on both sides. For every identifier containing it, ask **which sense it carries**: the leaf becomes
   `Step`; the abstract vertex keeps `Node`.
3. An identifier naming no renamed concept keeps its name even when its receiver type changes.
4. **Not every `Node` is ours** — see §Do not rename.

#### Types

| Current | New | |
| --- | --- | --- |
| `Graph` | `Definition` | |
| `GraphExecutor` | `Executor` | |
| `GraphSpec` | `DefinitionSpec` | |
| `Subgraph` | `Block` | |
| `SubgraphSpec` | `BlockSpec` | |
| `RecoveryStack` | `Ledger` | |
| `ExecutableUnit` | `Node` | abstract vertex; frees nothing until Phase 1 vacates `Node` |
| `ExecutableUnitSpec` | `NodeSpec` | **collides with today's `NodeSpec`** until Phase 1 renames it |
| `Node` | `Step` | leaf |
| `NodeSpec` | `StepSpec` | must precede the `ExecutableUnitSpec` rename |

#### Constructors and package functions

| Current | New |
| --- | --- |
| `NewGraph` | `NewDefinition` |
| `NewGraphSpec` | `NewDefinitionSpec` |
| `NewGraphExecutor` | `NewExecutor` |
| `NewSubgraph` | `NewBlock` |
| `NewSubgraphSpec` | `NewBlockSpec` |
| `NewNode` | `NewStep` |
| `NewNodeSpec` | `NewStepSpec` |
| `NewRecoveryStack` | `NewLedger` |
| `NewChildRecoveryStack` | `NewChildLedger` |
| `LoadGraph` | `LoadDefinition` |
| `SaveGraph` | `SaveDefinition` |
| `SerializeGraphs` | `SerializeDefinitions` |
| `ValidateGraph` | `ValidateDefinition` |

**`LoadGraph` → `LoadDefinition` is worth noting**: `plan.Provider` already has a method of that name, in a
different package, returning `*op.Graph`. After the rename the two agree — `plan.Provider.LoadDefinition`
returns a `*workflow.Definition` by calling `workflow.LoadDefinition`. Not a collision; a convergence.

#### Methods whose own name changes

| Current | New | Why |
| --- | --- | --- |
| `Graph.Subgraphs` | `Definition.Blocks` | names the composite |
| `Graph.SubgraphByID` | `Definition.BlockByID` | |
| `Graph.Nodes` | `Definition.Steps` | returns leaves |
| `Subgraph.Subgraphs` | `Block.Blocks` | |

#### Hooks

| Current | New | Why |
| --- | --- | --- |
| `FireSubgraphStart` / `FireSubgraphComplete` | `FireBlockStart` / `FireBlockComplete` | composite |
| `OnSubgraphStart` / `OnSubgraphComplete` | `OnBlockStart` / `OnBlockComplete` | |
| `FireNodeStart` / `FireNodeComplete` | `FireStepStart` / `FireStepComplete` | fired per LEAF dispatch |
| `OnNodeStart` / `OnNodeComplete` | `OnStepStart` / `OnStepComplete` | |

#### Keeps its name — verify by ABSENCE from the diff

A table of only the changes cannot distinguish "correctly unchanged" from "missed". These must not move:

| Identifier | Why it stays |
| --- | --- |
| `GenerateNodeID` | Mints ids for the abstract vertex, not the leaf — see below the table |
| `Edge` | planning-time connection; names no renamed concept |
| `Receipt` | durable entry; unchanged per the taxonomy |
| `RecoverySite` | name is accurate as-is — see Out of Scope |
| `Binding` and its variants | slot binding; names no renamed concept |
| `Graph.Checksum`, `.Edges`, `.Kind`, `.Origin`, `.Root`, … | receiver changes; the name itself is unaffected |

**`GenerateNodeID` in detail**, because Phase 1 originally had it wrong. `pkg/op/provider/flow/helpers.go`
calls it as:

```go
GenerateNodeID(string(Subgraph))
```

It mints ids for composites as well as leaves, so it names the **abstract vertex** — which under the new
taxonomy is `Node`, the name it already has. A mechanical `Node → Step` sweep renames it `GenerateStepID` and
quietly makes it wrong: an identifier claiming to mint leaf ids while minting them for `Block`s too. Phase 1's
target list said exactly that until this appendix was written.

#### Do not rename — NOT ours

`pkg/op/default_eval.go` and `deferred_default.go` use `text/template/parse`, whose vocabulary collides with
ours:

```go
action, ok := tree.Root.Nodes[0].(*parse.ActionNode)
```

`parse.ActionNode`, `parse.PipeNode`, `parse.StringNode`, `parse.IdentifierNode`, `parse.NumberNode`,
`parse.BoolNode`, `parse.FieldNode`, `parse.CommandNode` — **and `.Nodes`, a field on a stdlib type.** A
mechanical sweep over the word `Node` rewrites `tree.Root.Nodes` and fails to compile. Phase 1 must exclude
these by qualifier, not by name.

#### Files

| Current | New |
| --- | --- |
| `graph.go` | `definition.go` |
| `graph_executor.go` | `executor.go` |
| `subgraph.go` | `block.go` |
| `node.go` | `step.go` |
| `executable_unit.go` | `node.go` — **only after `node.go` vacates** |
| `recovery_stack.go` | `ledger.go` |
| `nodeid.go` | keeps its name, following `GenerateNodeID` |

Test files follow their subject: `graph_executor_test.go` → `executor_test.go`, `subgraph_test.go` →
`block_test.go`, and so on. `graph_number_fidelity_test.go` and `graph_format_identity_test.go` name the
Definition, so both take `definition_`.

#### Counts, for scoping

| Word | Exported identifiers containing it |
| --- | --- |
| `Graph` | 53 |
| `Subgraph` | 37 |
| `RecoveryStack` | 18 |
| `ExecutableUnit` | 3 |

23 of those are exported and non-test — the surface tabled above. The remainder are unexported helpers and
test fixtures, which follow the same rules.

## Implementation Phases

Each phase ships as its own PR. Each PR merges before the next starts (per the "never accumulate PRs" rule). Suggested sub-branch naming follows the existing pattern: `refactor/workflow-rename.phase-1`, `.phase-2`, etc.

### Phase 1: Node → Step

Single concept rename of the leaf struct. Frees the name `Node` for Phase 2.

**Targets:**
- `pkg/op/graph.go:380` — struct declaration carrying `Layer` + `Origin` fields
- Internal symbols that ripple: `NewNode → NewStep`, `NodeSpec → StepSpec`, `node`-named locals REFERRING TO LEAVES
- **`GenerateNodeID` KEEPS its name** — corrected 2026-08-27. This list previously said `GenerateNodeID → GenerateStepID`, `NodeID → StepID`. That is wrong: `flow/helpers.go` calls it as `GenerateNodeID(string(Subgraph))`, so it mints ids for composites as well as leaves. It names the ABSTRACT VERTEX, which under the new taxonomy is `Node`. See the appendix.
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

## Related Rename: the provider roles

`+devlore:surface=graph` produces `RoleAction` and `+devlore:surface=module` produces `RoleModule` — two
vocabularies for one idea, connected only by a switch in `generate.star`. An author writes `graph`, the
generated code says `Action`.

This rename forces the issue: `Graph` becomes `Definition`, so `surface=graph` will name a type that no longer
exists, and `module` is starlark's word for a namespace rather than a term in the new taxonomy. **Neither
current word survives.**

Belongs in this window rather than before or after it — doing it separately means renaming twice, and doing it
after means shipping a constant named for the type it replaced. Tracked as
[#715](https://github.com/NobleFactor/devlore-cli/issues/715), blocked on #716.

## Open Questions

- [x] **Q2: Subpackage edit bans.** ~~`provider/`, `inventory/`, `sops/` move with the directory in Phase 5.~~
  Restated 2026-08-27: `sops/` has left the package, and `claimcheck/` and `server/` have arrived. The
  starlarkbridge edit ban has since been lifted, so all five subpackages are edit-eligible on the same terms.
  Dispositions are in §Subpackage disposition.
- [x] **Q3: GitHub issue.** Epic [#451](https://github.com/NobleFactor/devlore-cli/issues/451), with
  [#716](https://github.com/NobleFactor/devlore-cli/issues/716) (this re-audit) and
  [#715](https://github.com/NobleFactor/devlore-cli/issues/715) (the role rename) beneath it.
- [ ] **Q4: Plan template adherence.** `docs/plans/TEMPLATE.md` gained a **Test Plan** section on 2026-08-27,
  which this plan does not have. For a pure rename the test plan is "every existing test passes unchanged" —
  which §Verification (per phase) already states, and which is the whole assertion. Confirm that section
  satisfies the requirement rather than adding a redundant one.
- [ ] **Q5: Documentation churn.** Added 2026-08-27. §Documentation Audit predates
  `3.2-projected-provider-api.md`, `3.6-method-classification.md`, `4.5-fsroot-variants.md`, and the provider
  catalog, all of which now describe `pkg/op` in prose a pure rename churns. In scope for the audit phase, or
  a follow-up? Stating it beats discovering it mid-phase.

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