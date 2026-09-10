---
title: "Three devlore-test fixtures assert text the runtime no longer produces"
issue: https://github.com/NobleFactor/devlore-cli/issues/875
status: complete
created: 2026-09-10
updated: 2026-09-10
---

# Plan: three fixtures assert stale text

## Summary

Three `data/*.star` fixtures assert text the runtime stopped producing, and each fails when run through the
CLI on `develop`. They are harvested from `devlore-cli.shell-exec-product` (a dead branch whose only value
was these uncommitted corrections) onto a fresh branch off `develop`.

| Fixture | Was | Is |
| --- | --- | --- |
| `test_imm_shell.star` | `shell.exec(...) == "echo hello"` | asserts `command`/`stdout`/`stderr`/`exit_code`, both streams, and the `"Result"` render |
| `test_flow_fatal_recovery.star` | `expect_error("fatal: abort after write")` | `expect_error("flow.failed executed: abort after write")` |
| `test_imm_file_join_variadic_error.star` | `expect_error("positional and keyword")` | `expect_error('multiple values for argument "parts"')` |

## Goals

1. The three fixtures pass when run.
2. `test_imm_shell.star` documents the real contract of `shell.exec`'s return: a `*Result` whose fields are
   individually reachable, not the command string.

## Out of scope

- **Why CI never caught this** — nothing runs the 110 non-Go-referenced fixtures. Filed as #876; it needs a
  harness gate, not a fixture edit.
- **The shell-provider rewrite** (#799). These are corrections to the *current* provider's fixtures; the
  rewrite renames `shell.exec` and will restate them. The correction stands regardless, and a false test in
  the tree should not wait on an open-ended redesign.

## Implementation Phases

### Phase 1: the plan
- [x] This document, the branch's first commit

### Phase 2: the corrections
- [x] Harvest the three corrected fixtures from `shell-exec-product`
- [x] Each passes through `devlore-test run` (verified before harvest and again after)

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/875-fixtures-assert-stale-text.md` | Create |
| `cmd/devlore-test/devloretest/data/test_imm_shell.star` | Modify |
| `cmd/devlore-test/devloretest/data/test_flow_fatal_recovery.star` | Modify |
| `cmd/devlore-test/devloretest/data/test_imm_file_join_variadic_error.star` | Modify |

## Related Documents

- Issues #875, #876; #799 (the shell-provider rewrite that will restate `test_imm_shell.star`)

## Open Questions

None.
