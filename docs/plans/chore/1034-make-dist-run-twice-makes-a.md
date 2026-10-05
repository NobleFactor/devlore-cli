---
title: "make dist empties dist/ before it builds"
issue: https://github.com/NobleFactor/devlore-cli/issues/1034
status: draft
created: 2026-10-04
updated: 2026-10-04
---

# Plan: make dist empties dist/ before it builds

Lane 25 of #949, ruled 2026-10-04: "Let's log it as a chore and fix the problem now. make dist should ensure the dist
dir is empty or gone beforehand." This pull request also closes lane 24's plan, whose last box waited on the merge.

## Issue 1034

At `6f600d68`, `checksums` (`Makefile:609-611`) hashes every `dist/devlore-cli_$(VERSION)_*.*` into
`dist/devlore-cli_$(VERSION)_checksums.txt`. On a second `make dist` in one checkout, the pattern also matches the
checksums file the first run wrote; the shell empties it before `shasum` reads it, so the new file lists itself with
the hash of an empty file. Archives an earlier run built for another `PLATFORM` stay in `dist/` too, and are listed
with the new ones. The release job checks out clean, so no published release is affected.

## Requirements

### Requirement 1: Every make dist starts from an empty dist/

- `dist-all` removes `dist/` before it creates it, after the host build that proves the version stamp, so every
  archive and the checksums file in `dist/` are this run's.
- `make dist` run twice makes the same set of files, and its checksums file has one line for each archive this run
  built and nothing else.
- `dist-clean` stays, for removing `dist/` without building.

### Requirement 2: Lane 24's plan closes

- `docs/plans/fix/1029-installers-tell-the-user-to.md`: its last box is ticked, citing the live install of 2026-10-05
  through the published one-liner (`v0.1.0-dev.20261005021254`, build `6f600d68`: "Checksum verified",
  `Already registered: base team personal`, no skipped line), and its status becomes `complete`.

## Implementation Phases

### Phase 1: Commit the plan

- [ ] This plan committed on `chore/1034-make-dist-run-twice-makes-a`, and reviewed with the owner

### Phase 2: The fix (Requirement 1)

- [ ] `dist-all` empties `dist/`. Proven: with a stale archive and checksums file left in `dist/`,
      `make dist PLATFORM=<host>` twice leaves one archive and a checksums file with its one line, the same both runs

### Phase 3: The documents (Requirement 2)

- [ ] Lane 24's plan

### Phase 4: Merge

- [ ] PR script written, shown, and handed over. The PR resolves #1034
- [ ] After the merge, the Release run publishes a checksums file with one line for each of the six archives

## Out of Scope

- **A test for the Makefile.** The proof is the two runs above, in the plan and the pull request; the Makefile has no
  test harness, and building one is not this chore.

## Decisions

- **D1. The removal is in `dist-all`, not a `dist-clean` prerequisite of `dist`.** Make need not build prerequisites in
  the order they are written once `-j` is in play, so `dist: dist-clean dist-all checksums` could clean after building.
  Inside `dist-all`'s recipe it runs in order, after the host build and before anything is written to `dist/`.
