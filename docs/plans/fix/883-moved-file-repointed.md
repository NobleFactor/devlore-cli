---
title: "Lane 3: a file that moved between layers is re-pointed under stop -- an occupant is what the record wrote"
issue: https://github.com/NobleFactor/devlore-cli/issues/883
status: complete
created: 2026-09-23
updated: 2026-09-23
---

# Plan: Lane 3 of the writ lifecycle schedule

## Summary

Lane 3 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), PR A, on the branch lanes 1 and 2 committed
to. Ruled 2026-09-23: **an occupant is writ's own when it matches what the record wrote** -- the literal link
target for a link, the content digest for a copy -- and the source is never resolved to decide it. Today pre-flight
under `stop` resolves the link on disk and resolves the recorded source and compares the two; when the source moved
to another layer the link dangles, the resolution fails, and writ refuses its own link as foreign. That is the
event #883 records (twelve `git-*` links after personal#180) and what the Linux VM did on 2026-09-22 a minute after a
pull.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 883

Bug, feature #463 (writ command surface), taken into #916 as lane 3; waits on #922, landed at `217f52fb`: the record
pre-flight reads is the lifetime being replaced.

## Goals

- [ ] Pre-flight under `stop` recognizes a dangling link that reads exactly as the record wrote it, and the deploy
      re-points it.
- [ ] Pre-flight and `reconcile` decide "is this what the record wrote" by one predicate.

## Current State

Read 2026-09-23 at `b1b96dc6`.

| Component | Status | Notes |
| --- | --- | --- |
| `deploy.occupantIsOurs` (`deploy.go:361`) | ❌ | link: `EvalSymlinks(target) == EvalSymlinks(source)`; either failing reads as foreign. Copy: digest as recorded |
| `reconcile.classifyLink` (`reconcile.go:140`, #923) | ✅ | reads the link's literal endpoint, absolutizes it against the link's directory, and compares it to the recorded source -- the predicate this lane needs, written once already |
| `readback.Entry` | ⚠️ | carries `Target`, `Source`, `Action`, `RecordedDigest`; no method says whether the occupant is as recorded |
| the layer-journey scenario, step 3.3 (`scenario_layer_journey_test.go:1150`) | ⚠️ | redeploys after the move with `--conflict=replace`, because under `stop` the moved links were refused; step 3.6 then proves the default policy accepts writ's own links |

## Requirements

### Requirement 1: one predicate

`readback.Entry` gains `AsRecorded() bool`: for a link, the target is a symlink whose literal endpoint, absolutized
against the target's directory, equals the recorded source; for a copy, the target's content digest equals the
recorded target digest. Neither side is resolved. `occupantIsOurs` becomes that method; `classifyLink` and
`classifyCopied` use it for their `changed` decision, so the two commands cannot disagree about what "ours" means.

### Requirement 2: the scenario says so

Step 3.3 redeploys under the default policy and asserts the moved links are re-pointed without `--conflict`; step
3.6 keeps proving a redeploy over its own links. A dangling link whose target the new plan does not name at all is
left where it is: deploy replaces the record, not the disk.

### Requirement 3: the words

The pre-flight paragraph in `10-command-line-interface.md` states the predicate in one sentence; #883's body gets
the ruling and the landing.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-23 ("AsRecorded() is named appropriately. it is the right
      predicate").

### Phase 2: The predicate

- [x] `readback.Entry.AsRecorded`; `occupantIsOurs` gone, pre-flight and the two classifiers on the predicate;
      `TestAsRecorded_Link` and `TestAsRecorded_Copy` beside it (dangling link, link elsewhere, non-link, copy as
      recorded, edited, no digest); `make test` green.

### Phase 3: The scenario

- [x] Requirement 2: step 3.3 redeploys under the default policy; `make test-scenario` exit 0, 8 passes.

### Phase 4: The words

- [x] Requirement 3: the guide's "Conflict resolution" says what an occupant of writ's own is; `5.1` names the
      predicate under "Reconcile's words"; the status file notes it. (10-cli has no pre-flight passage; the guide
      is where `--conflict` is explained.)

### Phase 5: Gates, metrics, and the VM sequence

- [x] `make check` exit 0 (`build/e2e/883-make-check.log`); `make test-scenario` exit 0, 8 passes, step 3.3 under
      the default policy. Coverage: total 63.1%; `readback` 82.4%, `reconcile` 86.1%, `deploy` 78.8%; `AsRecorded`
      87.5%, `preflightConflicts` 95.2%, `classifyCopied` 100%, `classifyLink` 63.6%. Complexity: `AsRecorded` 8,
      `classifyLink` 6, `classifyCopied` 7. Size: `reconcile.go` 245 (was 278), `deploy.go` 395 (was 424),
      `readback.go` 512.
- [x] Both VMs, snapshotted first (Linux `{3d35f680...}`, Windows `{5bbf98e5...}`); logs `build/e2e/883-e2e-ud24.log`,
      `883-e2e-wd11.log`. On both: a clean deploy; the source of one base link moved to another suffix directory in
      the writ-owned clone, the link dangling and `reconcile` saying `dangling`; a deploy under `stop`
      (`--allow-dirty`, since an uncommitted move dirties the clone and that gate -- #852's -- runs first, so the
      plan is HEAD's and still names the old path) **accepted the dangling link as writ's own and ran** (Linux 167
      files, Windows 71), where before #883 it refused; the move undone and a plain deploy restored `linked` on
      every entry. The re-pointing itself is proven by the layer-journey scenario's committed move, step 3.3.

### Phase 6: Closure

- [x] #883 carries the ruling and the landing; committed on `feature/922-lifetime-fold`; this plan `complete` at PR A.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | a dangling link that reads as recorded is ours; one pointing elsewhere is not | unit, `deploy` | the source is resolved |
| 2 | a copy as recorded is ours; an edited copy is not | unit, `deploy` | the digest is not compared |
| 3 | the journey's move redeploys under `stop` | scenario | the refusal returns |
| 4 | both VMs, the move sequence | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/883-moved-file-repointed.md` | Create: this plan |
| `cmd/writ/writ/readback/readback.go`, a test | Modify: `Entry.AsRecorded` |
| `cmd/writ/writ/deploy/deploy.go`, a test | Modify: `occupantIsOurs` on the predicate |
| `cmd/writ/writ/reconcile/reconcile.go` | Modify: the classifiers on the predicate |
| `cmd/writ/scenario_layer_journey_test.go` | Modify: step 3.3 |
| `docs/architecture/10-command-line-interface.md`, its status file | Modify: Requirement 3 |

## Open questions

None. The ruling covers the case; the leftover-on-disk consequence is recorded under #923's open question and is the
reconciliation epic's.

## Related Documents

- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule; this plan is lane 3
- [#883](https://github.com/NobleFactor/devlore-cli/issues/883) -- the bug
- [922-lifetime-fold.md](../feature/922-lifetime-fold.md) -- lane 1, the same branch
- [923-reconcile-words.md](../feature/923-reconcile-words.md) -- lane 2, the same branch
