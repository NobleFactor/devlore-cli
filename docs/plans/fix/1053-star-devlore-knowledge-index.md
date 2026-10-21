---
title: "star devlore knowledge index takes --dry_run as a string, and ignores the global --dry-run"
issue: https://github.com/NobleFactor/devlore-cli/issues/1053
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: star devlore knowledge index reads the global --dry-run alone

Lane 40 of #949. One pull request.

## Summary

`star devlore knowledge index` declares its own `--dry_run`, a string, beside star's global `--dry-run`. Its script
reads the string (`ctx.args.get("dry_run", False)`), whose `"false"` is non-empty and so true in Starlark:
`--dry_run=false` runs dry. The global `--dry-run`, which star gives every script as `ctx.dry_run`, is ignored. Two
names for one option, against the one-way-to-read rule (ruled 2026-10-05).

## Current State

At `98c59913`:

- `star/extensions/com.noblefactor.devlore.Knowledge/extension.yaml` declares `dry_run`, `type: string`, on
  `devlore.knowledge.index`.
- `commands/index.star:224` reads `ctx.args.get("dry_run", False)`; its usage comment shows `[--dry_run=true]`.
- star sets `ctx.dry_run` from the global flag (`cmd/star/extension/command.go:81`, `DryRun` in `application.go`);
  `devlore.Actions` and `devlore.Test` read it.
- `cmd/star/extension/knowledge_index_test.go`'s `runIndex` passes `"dry_run": ""`. No test runs the command dry.

## Requirements

### Requirement 1: The global --dry-run alone

- `extension.yaml` declares no `dry_run` flag; `index.star` reads `ctx.dry_run`, and its usage comment shows
  `--dry-run`.
- A test sets star's `DryRun` and runs the index over a registry with a new file: nothing is written. Another checks
  the command declares no `dry_run` flag. Both fail at `98c59913`.
- `runIndex` passes the target alone.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1053-star-devlore-knowledge-index` and reviewed with the owner: "commit the plan
      and go", 2026-10-21

### Phase 2: The fix (Requirement 1)

- [x] The flag gone, `ctx.dry_run` read; the two tests failed at `98c59913` ("a dry run rewrote the index", "the
      command declares --dry_run beside the global --dry-run") and pass with the fix; `make test` passes. The
      generated CLI reference already listed only `--dry-run`, so it does not change

### Phase 3: Merge

- [ ] `make check-all`, finished green as the PR script runs it, before the script is called ready
- [ ] PR script written, shown, and handed over; the PR resolves #1053
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `star/extensions/com.noblefactor.devlore.Knowledge/extension.yaml` | Modify | Requirement 1 |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/index.star` | Modify | Requirement 1 |
| `cmd/star/extension/knowledge_index_test.go` | Modify | Requirement 1 |

## Open Questions

None.
