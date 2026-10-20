---
title: "self uninstall can leave empty directories behind: it tries parents in random order and never retries"
issue: https://github.com/NobleFactor/devlore-cli/issues/1047
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: an uninstall removes every directory it empties, deepest first

Lane 32 of #949. One pull request.

## Summary

`cleanEmptyDirs` (`cmd/internal/cli/selfinstall.go`) collects every recorded file's parent directories into a map and
tries each with `removeIfEmpty` in the map's order. A parent tried before its child is not empty yet, is skipped, and
is never tried again, so whether `share/man`, `share/bash-completion` or `share/devlore` survive an uninstall empty
depends on Go's map iteration. Found 2026-10-19 auditing #949.

## Current State

At `720b3db6`: `cleanEmptyDirs` builds `map[string]bool` of every ancestor of every recorded path, up to the prefix, and
ranges over it once. `removeIfEmpty` removes a directory only when it reads empty. No test calls either.

## Requirements

### Requirement 1: Deepest first

`cleanEmptyDirs` tries the directories deepest first: sorted by their number of path elements, most first, and by name
within a depth, so the order is the same on every run. A directory is tried after every directory beneath it that the
manifest names, so each one the uninstall emptied is empty when its turn comes. A directory still holding anything,
recorded or not, stays.

### Requirement 2: A test that fails without it

`TestCleanEmptyDirs_RemovesEveryEmptiedDirectory` builds, in a temporary prefix, a chain six directories deep with a
recorded file at its bottom, a second branch beside it, and a directory holding a file the manifest does not record. It
removes the recorded files as the uninstall does, calls `cleanEmptyDirs`, and requires every directory of the chain
and the branch to be gone and the unrecorded file's directories to remain. With map order, the chain is removed whole
only when its six directories happen to come up deepest first, one order in 720, so the test fails without the fix;
shown before the fix lands.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1047-self-uninstall-can-leave-empty` and reviewed with the owner: approved
      2026-10-20, D1 with it ("I approve the design")

### Phase 2: The fix (Requirements 1 and 2)

- [ ] The test, failing at `720b3db6`
- [ ] Deepest first, and the test passing; `make test` green

### Phase 3: Merge

- [ ] The gate: CI's quality gate locally, `make test-race`, `make test-scenario`, all finished green before the PR
      script is called ready
- [ ] PR script written, shown, and handed over; the PR resolves #1047
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirement 1 |
| `cmd/internal/cli/selfinstall_test.go` | Modify | Requirement 2 |

## Decisions

- **D1. Sort, not retry.** Trying the directories deepest first removes every emptied one in a single pass, and the
  order is the same on every run. Retrying until a pass removes nothing would also work, but costs passes and keeps
  the order random.

## Open Questions

None.
