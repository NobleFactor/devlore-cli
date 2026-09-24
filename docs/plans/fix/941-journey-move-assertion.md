---
title: "The layer journey's post-move assertion names the moved-away targets, not ~/local"
issue: https://github.com/NobleFactor/devlore-cli/issues/941
status: complete
created: 2026-09-24
updated: 2026-09-24
---

# Plan: fix the Windows scenario regression from PR A

## Summary

PR A ([#940](https://github.com/NobleFactor/devlore-cli/pull/940)) merged with the two Windows scenario jobs red.
Lane 1's rewrite of the layer journey's step 3.4/3.5 asserts that, after the redeploy replaces the record, nothing
under `~/local` is in the record. On Windows the fixture's later commit still deploys `common.Windows/local/bin/w.ps1`
under `~/local`, so that link is rightly in the new record and the sweep calls a correct record stale. The Unix
fixtures keep nothing under `~/local` after the move, which is why only Windows failed. The assertion must name the
targets the move took away -- the set the step already computes -- and nothing wider.

**Corrected 2026-09-24 while fixing:** that set is `orphans`, the links left dangling by the redeploy and removed by
hand, not `expected`. `expected` is every link whose *source* moved, and the helper's source moved while its target
stayed deployed, re-pointed at the base layer; iterating `expected` failed on Unix at the helper.

**This plan covers this one fix and nothing else.**

## Issue 941

Bug, epic WritDeployment, feature #762; found by PR #940's CI on 2026-09-24. Develop is red on `scenario
(windows-amd64)` and `scenario (windows-arm64)` until it lands.

## Goals

- [x] Both Windows scenario jobs green on develop (the PR's CI).
- [x] The assertion names the moved-away targets (the orphans), and would still catch a stale record.

## Current State

Read 2026-09-24 at `ec1d2c0a`.

| Component | Status | Notes |
| --- | --- | --- |
| `scenario_layer_journey_test.go` step 3.4/3.5 (`:1186`) | ✅ fixed | after the hand cleanup of the old `~/local` links, every reconcile entry under `~/local` fails the step |
| the same step, earlier (`:1160`) | ✅ | `expected` is the set of targets that dangled after the move: exactly the moved-away files; `oldTree` counts those under `~/local` |
| CI | ❌ | the two Windows scenario legs red on develop since `ec1d2c0a`; `quality-gate` green |

## Requirements

### Requirement 1: the assertion

After the hand cleanup, for each path in `orphans`: no reconcile state names it. The `missing == 0` check stays. A
target the later commit still deploys, re-pointed in place (the helper) or kept under `~/local` (Windows' `w.ps1`),
is `linked` and untouched by the assertion.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered (go, 2026-09-24).

### Phase 2: The fix

- [x] The loop iterates `orphans`; `make check` and `make test-scenario` green here (darwin/arm64, 2026-09-24); the Windows proof is CI's.

### Phase 3: Closure

- [x] The pull request, synced with develop first, `Closes #941`; both Windows scenario jobs green is the merge's evidence; this plan `complete` with it.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the journey's step 3.5 passes on Unix | `make test-scenario` | the loop is wrong |
| 2 | the journey's step 3.5 passes on Windows | CI, `scenario (windows-*)` | a kept `~/local` target is swept |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/941-journey-move-assertion.md` | Create: this plan |
| `cmd/writ/scenario_layer_journey_test.go` | Modify: step 3.4/3.5's loop |

## Open questions

None.

## Related Documents

- [#941](https://github.com/NobleFactor/devlore-cli/issues/941) -- the bug; [#922](https://github.com/NobleFactor/devlore-cli/issues/922) -- where the assertion came from
- [922-lifetime-fold.md](../feature/922-lifetime-fold.md) -- lane 1's plan
