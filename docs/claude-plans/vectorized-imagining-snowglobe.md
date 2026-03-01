# Plan: Edit Global CLAUDE.md — Replace Git Section

## Context
Replace all git workflow instructions with a minimal policy that prohibits Claude from executing git commands directly.

## Changes to `/Users/david-noble/.claude/CLAUDE.md`

### Remove
- **Lines 1–70**: Entire "Git Best Practices" section (includes "Git Workflow (MANDATORY)", worktree rules, branch hygiene, commit message format, etc.)

### Add (as new first section)
```
# Git

You are prohibited from creating, updating, or removing git worktrees. You cannot delete files or directories by any means. You cannot issue add, clone, commit, fetch, or push. You can generate commands. I will execute them.
```

### Keep unchanged
- Code Quality
- Communication (including the `git reset` bullet — left as-is per user request)
- Codebase Understanding
- Plan Documents

## Verification
Read the file after editing to confirm correctness.
