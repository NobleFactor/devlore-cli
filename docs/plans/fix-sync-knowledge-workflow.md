# Plan: Fix sync-knowledge Workflow Binary Path Conflict

---
title: Fix sync-knowledge Workflow Binary Path Conflict
issue: https://github.com/NobleFactor/devlore-cli/issues/86
status: draft
created: 2026-02-10
updated: 2026-02-10
---

## Summary

Fix the sync-knowledge GitHub Actions workflow that fails because the `star` binary name conflicts with the new `star/` directory introduced by PR #85. The workflow builds `./star` but now `./star` resolves to a directory, not the binary.

## Goals

1. **Restore CI functionality**: sync-knowledge workflow must pass on all branches
2. **Align with conventions**: Use `bin/star` path consistent with noblefactor-ops Makefile
3. **Prevent future conflicts**: Ensure binary output paths don't conflict with project directories

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| sync-knowledge workflow | :x: Failing | `./star: Is a directory` (exit code 126) |
| star/ directory | :white_check_mark: Working | Contains project extensions per new structure |
| noblefactor-ops build | :white_check_mark: Working | Makefile builds to `bin/star` |

### Error Details

From GitHub Actions run 21889456754:

```
./star devlore-registry build knowledge \
  --domain all \
  --source_path /home/runner/work/devlore-cli/devlore-cli/devlore-cli \
  --registry_path /home/runner/work/devlore-cli/devlore-cli/devlore-registry
/home/runner/work/_temp/2d655c82-a7bb-424e-8abc-e961b30cbf6d.sh: line 1: ./star: Is a directory
##[error]Process completed with exit code 126.
```

### Root Cause

1. PR #85 introduced `star/extensions/` directory for project-local extensions
2. Workflow step `go build -o star ./cmd/star` outputs binary as `./star`
3. When workflow runs `./star devlore-registry build knowledge`, shell interprets `./star` as directory
4. Exit code 126 = "permission denied" (directory is not executable)

## Requirements

### Workflow Fix

Update `.github/workflows/sync-knowledge.yaml` to:

1. Build binary to `bin/star` instead of `star`
2. Run `./bin/star` instead of `./star`

**Before**:

```yaml
- name: Build star tool
  working-directory: noblefactor-ops
  run: go build -o star ./cmd/star

- name: Build knowledge base
  working-directory: noblefactor-ops
  run: |
    ./star devlore-registry build knowledge \
      --domain all \
      ...
```

**After**:

```yaml
- name: Build star tool
  working-directory: noblefactor-ops
  run: go build -o bin/star ./cmd/star

- name: Build knowledge base
  working-directory: noblefactor-ops
  run: |
    ./bin/star devlore-registry build knowledge \
      --domain all \
      ...
```

### Consistency Check

Verify no other workflows in devlore-cli or noblefactor-ops use `./star` directly:

| Repository | Workflow | Status |
| --- | --- | --- |
| devlore-cli | sync-knowledge.yaml | Needs fix |
| noblefactor-ops | ci.yaml | Check required |
| noblefactor-ops | Any others | Check required |

## Implementation Phases

### Phase 1: Fix sync-knowledge Workflow

- [ ] Update build step to output to `bin/star`
- [ ] Update run step to use `./bin/star`
- [ ] Commit and push to feat/star-extensions branch
- [ ] Verify PR #85 checks pass

**Files**:

- `.github/workflows/sync-knowledge.yaml` - Modify

### Phase 2: Audit Other Workflows (Optional)

- [ ] Check noblefactor-ops CI workflow for similar issues
- [ ] Update any workflows that build `./star` directly

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `.github/workflows/sync-knowledge.yaml` | Modify | Fix binary path to `bin/star` |

## Testing

1. Push fix to feat/star-extensions branch
2. Verify sync-knowledge check passes on PR #85
3. Merge PR #85 to develop
4. Verify sync-knowledge runs successfully on develop push

## Related Documents

- PR #85 - feat: add star extensions for devlore commands
- Issue #84 - Wire up devlore Starlark receiver
- noblefactor-ops Makefile - Reference for `bin/star` convention