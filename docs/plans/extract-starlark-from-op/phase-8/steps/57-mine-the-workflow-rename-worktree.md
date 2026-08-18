---
step: 57
issue: https://github.com/NobleFactor/devlore-cli/issues/517
title: "Mine the workflow-rename-plan worktree before retiring it"
status: charter — chartered 2026-08-14; do before the rename window (step 56)
proof_run: TBD — each item below carries a keep/discard verdict, then the worktree is removed
parent: ../../phase-8.md
---

# Step 57 — Mine the `workflow-rename-plan` worktree before retiring it

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
