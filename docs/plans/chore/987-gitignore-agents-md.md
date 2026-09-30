---
title: "AGENTS.md is ignored at the repository root"
issue: https://github.com/NobleFactor/devlore-cli/issues/987
status: approved
created: 2026-09-30
updated: 2026-09-30
---

# Plan: AGENTS.md is ignored at the repository root

## Summary

Lane 20 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916). The owner added `/AGENTS.md` to
`.gitignore` in the main clone on 2026-09-29 and ruled it committed at once, through the full process. `AGENTS.md`
is a Codex instruction file, a copy of `CLAUDE.md`, that keeps reappearing at the root; nothing here reads it.

## Goals

1. **`.gitignore` carries `/AGENTS.md`** as its first line, the owner's change as made.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `.gitignore` on develop | ❌ | no entry for `AGENTS.md` |
| the main clone | ⚠️ | the owner's line, uncommitted; also an uncommitted formatting-only `README.md` change, out of scope and the owner's |

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed with the owner 2026-09-30.

### Phase 2: The change

- [ ] `/AGENTS.md` at the top of `.gitignore`; CI's gate green on the merged tree.

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/chore/987-gitignore-agents-md.md` | Create: this plan |
| `.gitignore` | Modify: `/AGENTS.md` first |
