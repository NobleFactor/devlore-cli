# Plan: Fix sync-knowledge Workflow

---
title: Fix sync-knowledge Workflow
issue: https://github.com/NobleFactor/devlore-cli/issues/86
status: in-progress
created: 2026-02-10
updated: 2026-02-10
---

## Summary

Fix the sync-knowledge GitHub Actions workflow that fails after PR #85 introduced the `star/extensions/` directory structure. Two issues must be addressed: binary path conflicts and extension discovery.

## Goals

1. **Restore CI functionality**: sync-knowledge workflow must pass on all branches
2. **Proper extension discovery**: Star must find devlore-registry extensions
3. **Path forward**: Establish pattern for workflows using star with project-specific extensions

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Binary path | :white_check_mark: Fixed | Changed to `bin/star` |
| Extension discovery | :x: Failing | Star can't find devlore-registry extensions |
| star release | :x: Missing | No downloadable release with extensions |

### Error Progression

**Error 1** (run 21889456754):
```
./star: Is a directory
##[error]Process completed with exit code 126.
```
Root cause: `star/` directory conflicts with `./star` binary output.

**Error 2** (run 21889716548):
```
Error: unknown command "devlore-registry" for "star"
```
Root cause: Star built from noblefactor-ops doesn't include devlore-registry extensions. Those extensions now live in `devlore-cli/star/extensions/`.

## Root Cause Analysis

The workflow architecture has a fundamental issue:

1. **Before PR #85**: `devlore-registry` commands were in `noblefactor-ops/ops/` directory
2. **After PR #85**: Commands moved to `devlore-cli/star/extensions/` as proper extensions
3. **Problem**: Workflow builds star from noblefactor-ops but needs extensions from devlore-cli

Star discovers extensions from:
1. `./star/extensions/` (current working directory)
2. Walk up parent directories
3. `~/.local/share/star/extensions/` (user-installed)

When running from noblefactor-ops directory, star cannot find devlore-cli extensions.

## Solution Options

### Option A: Run from devlore-cli directory (Quick Fix)

Change workflow to run star from devlore-cli directory:

```yaml
- name: Build knowledge base
  working-directory: devlore-cli  # Changed from noblefactor-ops
  run: |
    ${{ github.workspace }}/noblefactor-ops/bin/star devlore-registry build knowledge \
      --domain all \
      --source_path ${{ github.workspace }}/devlore-cli \
      --registry_path ${{ github.workspace }}/devlore-registry
```

**Pros**: Simple, works immediately
**Cons**: Requires building star from source every time

### Option B: Use Star Release (Proper Solution)

Download a star release binary that includes extensions:

```yaml
- name: Download star
  run: |
    curl -sSL https://github.com/NobleFactor/noblefactor-ops/releases/latest/download/star-linux-amd64.tar.gz | tar xz
    chmod +x star

- name: Install devlore extensions
  run: |
    cp -r devlore-cli/star/extensions/* ~/.local/share/star/extensions/

- name: Build knowledge base
  run: |
    ./star devlore-registry build knowledge ...
```

**Pros**: Faster CI, uses tested release
**Cons**: Requires star release infrastructure (not yet implemented)

### Option C: Build devlore-cli's own star

Have devlore-cli build its own star binary with extensions bundled:

```yaml
- name: Build devlore star
  working-directory: devlore-cli
  run: |
    # Would require devlore-cli to have star build capability
    go build -o bin/star ./cmd/star
```

**Pros**: Self-contained
**Cons**: Duplicates star build logic, harder to maintain

## Implementation

### Phase 1: Quick Fix (This PR)

Apply Option A to unblock the PR:

- [x] Fix binary path to `bin/star`
- [ ] Change working directory to `devlore-cli`
- [ ] Use absolute path to star binary
- [ ] Verify PR #85 checks pass

**Files**:
- `.github/workflows/sync-knowledge.yaml` - Modify

### Phase 2: Star Release Infrastructure (Future)

Implement Option B as the proper long-term solution:

- [ ] Create star release workflow in noblefactor-ops
- [ ] Publish releases with binaries for linux/darwin x amd64/arm64
- [ ] Update sync-knowledge to download release instead of building
- [ ] Consider bundling common extensions in release

**Depends on**: noblefactor-ops release infrastructure

## Files to Create/Modify

| File | Action | Phase | Purpose |
| --- | --- | --- | --- |
| `.github/workflows/sync-knowledge.yaml` | Modify | 1 | Fix paths and working directory |
| `noblefactor-ops/.github/workflows/release.yaml` | Create | 2 | Star release automation |

## Testing

1. Push fix to feat/star-extensions branch
2. Verify sync-knowledge check passes on PR #85
3. Merge PR #85 to develop
4. Verify sync-knowledge runs successfully on develop push

## Related Documents

- PR #85 - feat: add star extensions for devlore commands
- Issue #84 - Wire up devlore Starlark receiver
- noblefactor-ops Makefile - Reference for `bin/star` convention
- docs/plans/star-release-install.md (noblefactor-ops) - Star release infrastructure