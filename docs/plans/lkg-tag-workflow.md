---
title: "LKG Tag Workflow"
issue: TBD
status: draft
created: 2026-03-19
updated: 2026-03-19
---

# Plan: LKG Tag Workflow

## Summary

Add a manually-triggered GitHub Actions workflow that lets admins mark any merged PR as the Last Known Good (LKG) build. After merging a PR to `develop` (or any branch), an admin runs `gh workflow run lkg-tag.yaml -f pr=<number>` or triggers it from the Actions tab. The workflow resolves the PR's merge commit and base branch, then creates (or moves) an annotated tag `<base-branch>/lkg` at that commit. For example, tagging PR #256 (merged into `develop`) moves the `develop/lkg` tag to PR #256's merge commit.

## Goals

1. **Manual LKG tagging**: Allow an admin to mark any commit as the LKG build via the GitHub Actions UI
2. **Branch-scoped convention**: Use `<branch>/lkg` naming (e.g., `develop/lkg`, `main/lkg`) so each branch can have its own independent LKG marker
3. **Moving tag**: Each invocation replaces the previous LKG tag for that branch, so there is always exactly one LKG per branch
4. **Annotated tags**: Tags carry metadata (who tagged, when, message) for audit trail
5. **Admin-only access**: Only repository admins can trigger the workflow

## Current State

| Component        | Status  | Notes                                              |
| ---------------- | ------- | -------------------------------------------------- |
| Release workflow | Failing | Creates `v0.1.0-dev.*` prereleases on develop push |
| LKG tagging      | Missing | No mechanism to mark a build as known-good         |

## Requirements

### Workflow: `lkg-tag.yaml`

A `workflow_dispatch` workflow with one input:

| Input | Required | Default | Description             |
| ----- | -------- | ------- | ----------------------- |
| `pr`  | yes      | —       | PR number to tag as LKG |

**Behavior**:

1. Check that the triggering user has admin role; fail otherwise
2. Fetch the PR via GitHub API; fail if the PR is not merged
3. Resolve the merge commit SHA and the base branch from the PR metadata
4. Derive the tag name as `<base-branch>/lkg` (e.g., PR merged into `develop` produces `develop/lkg`)
5. Force-create an annotated tag at the merge commit, with a message recording the PR number, title, who triggered it, and the merge SHA
6. Push the tag with `--force` (moves existing tag)

**Permissions**: `contents: write` (required to push tags)

**Example usage from CLI**:

```bash
# Tag PR #256's merge commit as develop/lkg
gh workflow run lkg-tag.yaml -f pr=256
```

## Implementation Phases

### Phase 1: Workflow file

- [ ] Create `.github/workflows/lkg-tag.yaml`
- [ ] Validate PR is merged, resolve merge commit and base branch
- [ ] Check triggering user is admin
- [ ] Force-create annotated `<base-branch>/lkg` tag at merge commit and push

**Files**:

- `.github/workflows/lkg-tag.yaml` - Create

## Files to Create/Modify

| File                             | Action | Purpose                                    |
| -------------------------------- | ------ | ------------------------------------------ |
| `.github/workflows/lkg-tag.yaml` | Create | workflow_dispatch workflow for LKG tagging |

## Open Questions

None.
