---
title: "star writes an empty list in a command's result as null"
issue: https://github.com/NobleFactor/devlore-cli/issues/1215
status: complete
created: 2026-11-08
updated: 2026-11-08
---

# Plan: every list in a star command's result is a list, `[]` when it is empty

Step 14 of issue tracking, iteration 1 (NobleFactor/noblefactor-engprod#346), ahead of
NobleFactor/noblefactor-engprod#379 and #436, which it blocks. One pull request.

## Summary

A star command that returns an empty list prints `null` where the list belongs, at every depth. `goValue`
(`cmd/star/extension/result.go`) converts a Starlark list or tuple through `goSlice`, which starts from `var out []any`;
a list with no elements leaves it a nil slice, and JSON writes a nil slice as `null`. Anything that loops over the list
then fails: jq exits 5, "Cannot iterate over null". `goSlice` starts from an empty slice instead, so an empty list is
`[]` in every rendering, and a test fails if it is ever nil again.

## Rulings

- 2026-11-08, the owner: star returns an empty list in place of null everywhere. A list is always a list, `[]` when it
  is empty; a key that holds one value, and holds none, is null. Nothing in a result uses null on a list to mean
  anything but none, and every star command's output changes with it.
- 2026-11-08, the owner: the fix is done now, as step 14 of the schedule, ahead of the two tasks it blocks.

## Current State

At `be7042d6`:

| Where | An empty list becomes | Notes |
| --- | --- | --- |
| `goSlice`, `cmd/star/extension/result.go` | nil, written as `null` | The defect: `var out []any` |
| `naturalSequence`, `pkg/op/starlarkbridge/converter.go` | `[]` | `make([]any, 0, n)`: a provider's arguments are right already |
| `FieldFilter.Apply`, `pkg/result/field_filter.go` | `[]` | `reflect.MakeSlice(…, 0, n)`: `--filter` matching nothing gives `[]` once its input is a list |
| `JQFilter.Apply`, `pkg/result/jq_filter.go` | not a list | A program that yields no value returns nil: no list, and not this defect |
| Tests | none | Nothing tests `goValue` |

## Requirements

### Requirement 1: an empty list is an empty slice

`goSlice` returns an empty, non-nil slice for an empty list or tuple, at any depth, so that `-o json` prints `[]`, a
dict holding one prints `{"home": []}`, and `None` still prints `null`.

### Requirement 2: a test that fails when it comes back

`cmd/star/extension/result_test.go` converts each of those values as a command's result is converted, encodes it as
JSON, and compares the text; it fails when `goSlice` returns nil again.

## Implementation Phases

### Phase 1: the plan

- [x] This plan committed on `fix/1215-empty-list-not-null`

### Phase 2: the test, first

- [x] `cmd/star/extension/result_test.go`, failing on the empty list, the empty tuple and the nested cases before the
      fix

### Phase 3: the fix

- [x] `goSlice` starts from `make([]any, 0, n)`; `goValue`'s doc comment says an empty list is an empty slice

### Phase 4: proved and merged

- [x] `make check` passes; a star built from the branch runs NobleFactor/noblefactor-engprod's `tests/Test-StarGitHub`
      in #436's worktree, and its cases 3e to 8 pass
- [x] The pull request script written, shown and handed over; it closes #1215

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | An empty list, an empty tuple, an empty list in a dict and in a list encode as `[]` | unit | `goSlice` returns nil for no elements |
| 2 | `None` still encodes as `null` | unit | the fix turns None into a list |
| 3 | NobleFactor/noblefactor-engprod's extension prints `"home": []` and the sweep's empty lists | `tests/Test-StarGitHub`, with a star from this branch | the defect is back |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/fix/1215-empty-list-not-null.md` | Create | This plan |
| `cmd/star/extension/result.go` | Modify | `goSlice` from an empty slice |
| `cmd/star/extension/result_test.go` | Create | The test |

## Related Documents

- [10-command-line-interface.md](../../architecture/10-command-line-interface.md), §8, the pipeline the result enters
- NobleFactor/noblefactor-engprod#436's plan, `docs/plans/feature/436-triage-sweep.md`, where the defect was found

## Open Questions

None.
