---
title: "Get-EpicReport renders chores"
issue: https://github.com/NobleFactor/devlore-cli/issues/809
status: complete
created: 2026-09-04
updated: 2026-09-04
---

# Plan: Get-EpicReport renders chores

## Summary

`Get-EpicReport` walks `epic -> feature -> {task | bug}` and has no tier for `chore`, so an epic whose
children are chores renders empty. `Epic:Process` in `noblefactor-ops` holds eleven issues and the
report showed one row: the epic itself. Two edits to the jq selector fix it.

## Goals

1. **A chore is a child of its epic** — selected alongside tasks and bugs, since it carries
   `Epic:<Name>` like every other issue.
2. **No regression** for epics that have no chores.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `kindOf` | ✅ Working | already returns `"chore"` |
| Audit `faults` | ✅ Working | already accepts `chore` as a kind |
| `isChild` selector | ❌ Missing | `tagged("task") or isBug` — chores never selected |
| Unparented annotation | ✅ Correct | "unfiled, no parent feature" is right for a chore too |

The omission is in one definition. Everything downstream already handles the kind.

## Requirements

### The selector admits chores

```jq
def isChild: tagged("task") or isBug or tagged("chore");
```

This matches `docs/issue-standards.md` in `noblefactor-ops`, which states that a chore carries
`Epic:Process`, has no parent feature, and that "under `Epic:Process` there is no feature tier; the
chores hang directly from the epic". The script implemented the first half and dropped the second.

### An absent parent feature stays a fault, for every kind

A first draft of this change excused a chore's missing parent feature, on the strength of
`issue-standards.md`: "under `Epic:Process` there is no feature tier; the chores hang directly from
the epic."

**Ruled 2026-09-04, against that clause:** *an epic with no features is not an epic — it is an
ill-defined issue, or an incomplete one.* A chore therefore files under a feature like any other
tier-three issue, and one that names no feature is unfiled and should say so.

So the annotation is left exactly as it was. The only change is the selector.

`Epic:Process` (#142) is the worked example: nine chores, zero features. With this change the report
names it — eleven rows, two features, and eight chores each marked `unfiled, no parent feature`.
Before the change it rendered one row and looked healthy. **The defect was that the report could not
show the problem, not that the problem was a rendering artefact.**

This leaves `noblefactor-ops/docs/issue-standards.md` contradicting the ruling. Amending it, and
decomposing #142 into features, are separate work in that repository.

## Implementation Phases

### Phase 1: The two edits

- [x] Widen `isChild` to admit `chore`
- [x] Leave the unparented annotation alone — a chore with no feature is unfiled
- [x] `shfmt -d -i 4 -ci` clean
- [x] `shellcheck -x --severity=warning` clean
- [x] `.github/scripts/shell-lint.sh` reports `ok ./scripts/Get-EpicReport`

**Files**:

- `scripts/Get-EpicReport` - Modify

### Phase 2: Verification

- [x] `noblefactor-ops` `Epic:Process` renders 11 rows, was 1
- [x] Its two features render as features; its eight chores read `unfiled, no parent feature`
- [x] `devlore-cli` `Epic:ResourceModel` output byte-identical before and after

## Known limitation, not addressed here

**A chore in this repository still does not appear in any report.** Chores here carry `Epic:Process`,
but the epic *issue* lives once, in `noblefactor-ops#142` — so `--epic Process` run here finds no epic
to anchor the section and renders nothing. #809 itself is invisible to the report that #809 fixes.

That is the single-repository limitation, not the chore-rendering defect, and it is the same one that
prevents a thread spanning two repositories from being reported. It belongs to
`noblefactor-ops#140`, where the report becomes a star extension and the repository selector is
designed.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `scripts/Get-EpicReport` | Modify | Two jq edits |
| `docs/plans/chore/809-report-renders-chores.md` | Create | This plan |

## Related Documents

- Issue #809 - this chore
- `NobleFactor/noblefactor-ops#142` - `Epic:Process`, the epic this repository's chores belong to
- `NobleFactor/noblefactor-ops#140` - the star extension that replaces this script; carries the
  chore-rendering requirement forward so it is not reinherited
- #797 - deletes this script when that extension ships
- `NobleFactor/noblefactor-ops:docs/issue-standards.md` - states that chores hang directly from the
  epic with no feature tier. The 2026-09-04 ruling contradicts that clause; amending it is separate
  work in that repository.
