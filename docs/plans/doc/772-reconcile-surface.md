---
title: "740 and 762 disagree on writ reconcile's surface"
issue: https://github.com/NobleFactor/devlore-cli/issues/772
status: in-progress
created: 2026-09-01
updated: 2026-09-01
---

# Plan: Bring 740 and 762 into agreement on `writ reconcile`

## Summary

Two plans describe the same command differently: one records flags it will not have, and both still teach
`writ status`, a command being retired rather than modernized. This settles the surface, removes the
flags, and corrects the vocabulary everywhere it appears.

## Goals

1. **State the settled surface once**, in the plan that owns the command.
2. **Remove the flags that will not exist**, and say why the selector is deferred rather than leaving a
   gap someone fills with a guess.
3. **Correct the terminology wherever it appears.** A document that teaches `writ status` teaches a
   command that will not exist.

## The settled surface

`writ reconcile` **produces a report.** It has **no command flags.** It takes the globals —
`--output` / `-o`, `--filter`, `--jq`, `--store`, `--dry-run` — and must utilize all of them faithfully.

The selector between fix, diff, and summary behaviors is **deferred to the reconciliation epic** and is
not designed here.

### What the report carries

One JSON document, four sections:

| Section | What it is |
| --- | --- |
| `entries[]` | **the delta** — the deployed inventory classified against the live filesystem |
| `layers[]` | the registered layer tree: name, path, state (`absent`/`directory`/`link`/`broken-link`), resolved target |
| `packages[]` | the package operations writ's runs performed, as fact-of-record |
| `health` | the store's self-report: runs folded, and missing-piece findings |

Each entry is classified by one of eight states — `Linked`, `Copied`, `Missing`, `Conflict`, `Orphan`,
`Stale`, `Modified`, `ModifiedOrStale` — and carries `Repair`, naming the lifecycle command that fixes
that finding.

### Why the selector is deferred, and a caution for whoever designs it

Two of the three candidate views are **already projections of the one document**: the diff is
`--jq '.entries'` and the history is `--jq '.packages'`. Those are stage 2 of the pipeline #740 landed,
and the reserved set already carries `--jq` and `--filter` to do that selection.

Adding `--diff` and `--history` flags would give two ways to select a subset of the same document, which
will drift. Only the repair half is genuinely new, because it is a mutation rather than a projection —
and the mutation axis already has a global in `--dry-run`, so a second flag on that axis needs to say
how the two compose.

Recorded here so the reconciliation epic starts from this rather than rediscovering it.

## Current State

### Flags that will not exist

| Site | Says | Why it is wrong |
| --- | --- | --- |
| `762:51` | `writ reconcile [--drift] [--fix] [--json] [<project>...]` | none of the three will exist — `--json` is replaced by the shared `-o`, and the other two are the deferred selector under another name |
| `762:96` | "`5.1`'s 'there is no `--fix`' is retired as the stale claim it is" | overstated — reconcile will repair, but has no `--fix` and the surface is undecided |
| `5.1:12` | the same overstatement | overstated |

### Terminology: `writ status` in 740

Eight sites. Six are the command's name and change outright; two are not terminology at all:

| Site | Change | Kind |
| --- | --- | --- |
| `740:114` | the flag-inventory row | command name |
| `740:171` | "which is why `writ status` and `writ verify` cannot render yaml" | command name |
| `740:231` | "`writ status` honors one of eight formats" | command name |
| `740:237` | "`writ status -o bogus` prints the dashboard" | command name |
| `740:242` | "`writ status --store <elsewhere>`" | command name |
| `740:365` | test row 6 | command name |
| `740:212` | `cmd/writ/writ/status/report.go` | **a path** — the file on disk today |
| `740:27` | "this epic rewrites `status/report.go` before #762 moves it" | **a claim** — false, and rewritten rather than renamed |

**The path keeps both forms.** Writing `reconcile/report.go` before the rename sends a reader to a path
that does not exist; writing only `status/` teaches the retired name. It is recorded as
`cmd/writ/writ/status/report.go` → `reconcile/` under #762 — accurate now and after.

**The measurements keep their date, not their vocabulary.** Lines 231–242 record what was found on
2026-08-30, when the command was named `writ status`. The finding survives the rename; the name does not.
One dated note at the measurement block says so, rather than seven caveats scattered through the text.

## Implementation Phases

### Phase 1: 762 — the flags go (status: complete)

- [x] Delete the `[--drift] [--fix] [--json]` shape at `:51`, keeping the historical quotation that
      explains why the command was renamed to `status` and why that reason lapsed
- [x] Rewrite Requirement 2 to state the settled surface: a report, no command flags, the globals used
      faithfully, the selector deferred
- [x] Add the caution above — that diff and history are already `--jq` projections

### Phase 2: 5.1 — the `--fix` note (status: complete)

- [x] `5.1:12` brought in line: reconcile will repair, it has no `--fix`, and the surface is undecided

### Phase 3: 740 — the vocabulary corrected (status: complete)

- [x] Six command-name sites renamed: `:114`, `:171`, `:231`, `:237`, `:242`, `:365`
- [x] `:212` records both forms — the path today, and where #762 moves it
- [x] `:27` rewritten: the work lands as `writ reconcile`, so the ordering it asserts does not hold
- [x] One dated note at the measurement block, recording that the measurements predate the rename

**Files**:

- `docs/plans/feature/762-lifecycle-scopes.md` - Modify
- `docs/architecture/5.1-reconciliation.md` - Modify
- `docs/plans/feature/740-cli-output-conventions.md` - Modify

## Test Plan

No code. The claims are verifiable by search.

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | No plan records a flag `writ reconcile` will not have | search | `--drift`, `--fix` or `--json` appears as part of its surface rather than as history |
| 2 | The globals commitment is stated where the command is owned | search | 762 describes the surface without naming `-o`, `--filter`, `--jq`, `--store`, `--dry-run` |
| 3 | No document teaches `writ status` as a live command | search | `writ status` appears outside a historical quotation or a path |
| 4 | The path is findable both before and after the rename | search | `:212` names only one of the two forms |

**Not covered:** whether the deferred selector is designed correctly, because it is not designed. The
caution in Phase 1 is the only thing this plan says about it.

## Migration Path

None. Documents only.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/doc/772-reconcile-surface.md` | Create | This plan |
| `docs/plans/feature/762-lifecycle-scopes.md` | Modify | The settled surface; flags removed |
| `docs/architecture/5.1-reconciliation.md` | Modify | The `--fix` overstatement |
| `docs/plans/feature/740-cli-output-conventions.md` | Modify | Terminology, the path, the ordering claim |

## Related Documents

- [`762-lifecycle-scopes.md`](../feature/762-lifecycle-scopes.md) — thread 3, which owns the command
- [`740-cli-output-conventions.md`](../feature/740-cli-output-conventions.md) — thread 1, the flag set
- [`5.1-reconciliation.md`](../../architecture/5.1-reconciliation.md) — the design of record
- Issue [#772](https://github.com/NobleFactor/devlore-cli/issues/772)

## Open Questions

- [ ] Does the repair half become a mode of `reconcile`, or stay as the named commands each `Entry`
      already points at in its `Repair` field? `writ-deploy-family.md:136` chose the latter deliberately
      — *"no `--fix`: the repair for each finding is named instead"* — and that is what the tree
      implements today. This is bigger than flag syntax and belongs to the reconciliation epic.
