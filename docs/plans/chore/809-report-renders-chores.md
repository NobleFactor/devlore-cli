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
2. **A chore's missing parent feature is not a fault** — for a task it is; for a chore it is the
   design, and the report should not annotate it.
3. **No regression** for epics that have no chores.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `kindOf` | ✅ Working | already returns `"chore"` |
| Audit `faults` | ✅ Working | already accepts `chore` as a kind |
| `isChild` selector | ❌ Missing | `tagged("task") or isBug` — chores never selected |
| Unparented annotation | ❌ Wrong | calls a chore's absent feature "unfiled, no parent feature" |

The omission is in one definition. Everything downstream already handles the kind.

## Requirements

### The selector admits chores

```jq
def isChild: tagged("task") or isBug or tagged("chore");
```

This matches `docs/issue-standards.md` in `noblefactor-ops`, which states that a chore carries
`Epic:Process`, has no parent feature, and that "under `Epic:Process` there is no feature tier; the
chores hang directly from the epic". The script implemented the first half and dropped the second.

### An absent parent feature is annotated by kind

```jq
def unparented:
  if tagged("chore") then ""
  else "unfiled, no parent feature" end;
```

Applied at both `row(...)` call sites — the json branch and the markdown branch. A chore's Comment
column then reads `chore` alone, rather than repeating a non-fault on every row.

## Implementation Phases

### Phase 1: The two edits

- [x] Widen `isChild` to admit `chore`
- [x] Add `unparented`; replace the literal at both call sites
- [x] `shfmt -d -i 4 -ci` clean
- [x] `shellcheck -x --severity=warning` clean
- [x] `.github/scripts/shell-lint.sh` reports `ok ./scripts/Get-EpicReport`

**Files**:

- `scripts/Get-EpicReport` - Modify

### Phase 2: Verification

- [x] `noblefactor-ops` `Epic:Process` renders 11 rows, was 1
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
- `NobleFactor/noblefactor-ops:docs/issue-standards.md` - the scheme this restores agreement with
