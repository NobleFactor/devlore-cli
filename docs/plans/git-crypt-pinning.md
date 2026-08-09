---
title: "Git-Crypt Pinning"
status: complete
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Git-Crypt Pinning

## Summary

Caught live by the Personal-repo dogfooding cutover: writ's snapshot pinning
(`git worktree add`) fails on an unlocked git-crypt repository — the checkout's smudge
filter reads `$GIT_DIR/git-crypt` for its key, but a worktree's private git-dir never
holds it (the key lives in the COMMON dir). Any git-crypt layer broke pinning, on every
registration path. Reproduced and workaround validated by hand (git-crypt 0.8.0,
git 2.55) before implementing.

## Fix

`gitWorktreeAdd` becomes three steps: `worktree add --no-checkout --detach`, then
`linkGitCryptKeys` (symlink the common dir's `git-crypt` into the worktree's private
git-dir; a repository without git-crypt is a no-op, and a reused worktree skips the
existing link), then `checkout --force <hash>`. Non-crypt repositories see identical
behavior through a different sequence.

## Proven by

The live cutover itself: 75 files deployed from the git-crypt personal layer (secrets
plaintext end to end), `writ status` 75/75, zero new broken links against the pre-cutover
baseline. CI cannot exercise git-crypt (not on runners); the scenario's non-crypt legs
prove the resequenced path.
