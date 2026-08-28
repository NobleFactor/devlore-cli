---
step: 57
issue: https://github.com/NobleFactor/devlore-cli/issues/517
title: "Mine the workflow-rename-plan worktree before retiring it"
status: abandoned
proof_run: none — the premise was falsified before any verdict was needed
parent: ../../phase-8.md
---

# Step 57 — Mine the `workflow-rename-plan` worktree before retiring it

> **SUPERSEDED 2026-08-27 — the premise of this step is false, and [#517](https://github.com/NobleFactor/devlore-cli/issues/517)
> is closed as invalid.** Nothing in the worktree exists only there. Every item in the table below was
> either moved or deliberately removed on `develop` between 2026-08-10 and 2026-08-12 — two days
> *before* this step was chartered:
>
> | Item | What actually happened |
> | --- | --- |
> | `tools/New-OpInventory/main.go` | **Renamed** to `cmd/devlore-inventory/main.go` — `b64a4fe0` (#360) |
> | `prototype/bindgen/` | **Removed**, all 12 files — `10b4f8cd`, "chore: remove the bindgen prototype" (#386) |
> | `CODE-CONSOLIDATION-ANALYSIS.md` | **Removed** — `030a8790`, "clean repo root" (#390) |
> | `GITHUB-ISSUES.md` | **Removed** — `030a8790` (#390) |
> | `ARCHITECTURE-STATUS.md` | **Removed** — `030a8790` (#390) |
> | `draft-llm-cache-augmented-generation.md` | **Removed** — `33f7635e`, "relicense to Apache-2.0, retract SSPL-era tags" (#389) |
> | `draft-llm-long-context-prompting.md` | **Removed** — `33f7635e` (#389) |
>
> The error was method, not diligence: this step compared two working trees and read "absent from
> `develop`" as "exists nowhere else." It never consulted history. Anything wanted from the list is
> recoverable from the commits above, and the worktree is safe to retire.

**Status:** `charter` — chartered 2026-08-14. A stale worktree is holding content that exists nowhere else, and
it will be deleted at some point by someone tidying up. Read it first.

## What it is

`/Users/david-noble/Workspace/NobleFactor/devlore-cli.workflow-rename-plan` — a git worktree on the branch
`docs/workflow-rename-plan`, last written **2026-05-26**, roughly three months stale. The branch is **local
only**: it does not appear in the repository's remote branch list, so nothing but that directory is holding it.

## What is NOT worth mining

Its copy of `docs/plans/workflow-rename.md` is **older than develop's**. It still calls the leaf type
`Operation`; develop's copy (updated 2026-05-30) settled on `Step`/`Block` and added the
`Definition ▸ Node ▸ {Block, Step}` taxonomy. Develop is ahead — take nothing from that file.

## What exists only there

| Item | Size | First read |
| --- | --- | --- |
| `CODE-CONSOLIDATION-ANALYSIS.md` | 257 lines | dated 2026-03-10, scope "devlore-cli + noblefactor-ops (develop branches)" — a cross-repository duplication analysis |
| `draft-llm-long-context-prompting.md` | 270 lines | in-context learning versus fine-tuning for feeding API schemas and coding instructions |
| `GITHUB-ISSUES.md` | 238 lines | issue triage from 2026-03-10, 25 open at the time — largely historical, but its *classification* may still be useful |
| `draft-llm-cache-augmented-generation.md` | 60 lines | argues a **static domain** should use context caching rather than a vector database — directly relevant to the registry's CAG assets |
| `ARCHITECTURE-STATUS.md` | 45 lines | a roll-up index of `docs/architecture/*.status.md` |
| `prototype/bindgen/` | 12 files | a CLI-binding generator prototype: cobra extractor, schema, stubgen, codegen, `gh.yaml` example |
| `tools/New-OpInventory/main.go` | 1 file | the inventory generator — note the repository has since ruled that `inventory.gen.go` is **hand-edited** and this tool is **never run** |

## Why each needs a verdict rather than a bulk copy

- **The CAG draft** bears on a live product decision — the registry holds CAG assets, and this argues against a
  vector database for a static domain. If that reasoning is still endorsed it belongs in `docs/architecture/`;
  if it has been superseded, saying so is worth more than the file.
- **`prototype/bindgen/`** may be dead, may be the seed of something. Twelve files of generator code with tests
  is either worth `docs/sketches/` or worth deleting deliberately — not worth leaving in a worktree nobody
  opens.
- **`ARCHITECTURE-STATUS.md`** duplicates what the `*.status.md` companions already carry. Step 51 settled that
  such rosters are hand-maintained; a second index that drifts is worse than none.
- **`tools/New-OpInventory`** conflicts with a standing ruling — the inventory is edited by hand and this tool
  is never run. Mining it would reintroduce a retired path.
- **`GITHUB-ISSUES.md` and `CODE-CONSOLIDATION-ANALYSIS.md`** are five months old and describe a repository that
  has changed underneath them. Their *findings* are probably stale; their *method* may not be.

## Exit criteria

- [ ] Each item above has a recorded verdict: promote (with destination), or discard (with the reason).
- [ ] Anything promoted lands on `develop` through a normal PR, not by copying files between worktrees.
- [ ] The worktree is removed and the local branch deleted — **after** the promotions merge, never before.
- [ ] Done **before** step 56's rename window opens, so the worktree is not competing with a five-PR sweep.

## Related

- [step 56](../../phase-8.md) — the rename this worktree was created to plan.
- [`docs/plans/workflow-rename.md`](../../../workflow-rename.md) — the current plan, ahead of the worktree's copy.
