---
title: "Lane 1: deploy replaces the record -- the fold is bounded to the current lifetime"
issue: https://github.com/NobleFactor/devlore-cli/issues/922
status: active
created: 2026-09-23
updated: 2026-09-23
---

# Plan: Lane 1 of the writ lifecycle schedule

## Summary

Lane 1 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), PR A. Deploy **replaces** the record
(#913). Each deploy is one lifetime -- ruled 2026-09-23: **one `writ deploy` invocation, across every scope it
runs** -- whose receipts stack and fold naturally through the upgrades and reconciliations that follow, until a later
deploy replaces it or a decommission ends it. Today `readback.Fold` folds every receipt the machine has ever written
into one record, so a file that left the checkout stays in the record forever. This lane gives the store a
first-class lifetime, makes deploy open one, makes the other operations write into the current one, and bounds the
fold to it.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 922

Task, epic WritDeployment, feature #762. Found 2026-09-22 on `danoble-ud24-1.local`: 52 records reported `orphan`,
then `missing`, after a layer reorganization, because the fold reads every lifetime the machine has ever had.

## Goals

- [ ] The store knows what a lifetime is, which one is current, and which traces belong to it.
- [ ] `writ deploy` opens a lifetime; `upgrade`, `reconcile` and `adopt` write into the current one; the fold reads
      the current one and nothing else. (`decommission` ending one is #929's; this lane leaves the seam.)
- [ ] A file the previous lifetime deployed and this one does not is not in the record, on both VMs.

## Current State

Read 2026-09-23 at `a835739d`.

| Component | Status | Notes |
| --- | --- | --- |
| `cli.IndexEntry` (`cmd/internal/cli/index.go`) | ⚠️ | `At`, `Event` (graph or trace), `Tool`, `Scope`, `GraphChecksum`, `TraceFile`; nothing says which invocation wrote it |
| the run index doctrine (`index.go:20`) | ✅ | "a detection hint, never a source of truth": documents absent from the index still count |
| `cli.WriteGraph` (`store.go:125`) | ✅ | content-addressed by checksum, idempotent: the same plan is one document however many invocations run it -- so a graph cannot carry a per-invocation mark |
| `cli.WriteTrace` (`store.go:180`) | ⚠️ | one trace per graph run, named by UTC time under the graph's directory; appends the index line; carries no invocation mark |
| `readback.Fold` (`readback.go:143`) | ❌ | every writ run in the index plus every trace on disk, sorted by time, folded onto one map, newest per target; `removingActions` subtract |
| `deploy.runAll` (`deploy.go:147`) | ⚠️ | one graph per scope, run in order; nothing groups the scopes' traces as one invocation |
| `deploy.preflightConflicts` (`deploy.go:275`) | ⚠️ | reads the fold as the record of what writ left; under `stop` refuses occupants it does not recognize |
| writers of runs | — | `deploy` (5 sites), `upgrade` (2), `decommission` (4), `adopt` (2), `secret encrypt` (4), `migrate` (5): every one goes through `WriteGraph`/`WriteTrace` and lands in the same fold |
| `reconcile` | ⚠️ | reads the fold; its words are #923's |

## Requirements

### Requirement 1: the lifetime is a store document

A lifetime is a document in the store, `lifetimes/<id>.yaml`, with a UUIDv7 id minted by the `writ deploy`
invocation that opens it: `id`, `opened` (UTC), `state` (`current`, `replaced`, `ended`), `closed` (UTC, when not
current), and `runs`, the list of `{graph_checksum, trace_file, operation}` in write order. `lifetimes/current`
names the current one. The index stays a detection hint; the lifetime document is the truth about membership, so a
trace on disk that no lifetime names belongs to none and is not folded. A store with no `lifetimes/` directory has
no current lifetime (#756's not-found).

### Requirement 2: deploy opens a lifetime; the others write into it

`writ deploy` mints the id after pre-flight passes and before the first scope runs; every scope's trace is appended
to it as it is written; when the first trace is written the previous current lifetime becomes `replaced` and the new
one `current`. `upgrade`, `reconcile` (once #924 writes receipts) and `adopt` append their traces to the current
lifetime and refuse with not-found (66) when there is none. `decommission` appends its traces and ends the
lifetime -- the ending is #929's; this lane makes it append. `secret encrypt` and `migrate` are not deployment
records and join no lifetime.

### Requirement 3: the fold reads the current lifetime

`readback.Fold` reads `lifetimes/current`, then that lifetime's runs in write order, and folds those and only those.
The signature and the `Inventory` it returns do not change, so pre-flight and `reconcile` change nothing. A missing
current lifetime is `os.ErrNotExist`, which pre-flight already treats as an empty inventory and `reconcile` as
not-found.

### Requirement 4: what a partial deploy leaves current

A deploy whose later scope fails has written earlier scopes' traces: the new lifetime is current with what it wrote
(fail-forward, as `decommission` already runs), and the receipt says which scopes ran. Deploy replaces; a deploy that
half-ran replaced half, and the record says so. Ruled 2026-09-23.

### Requirement 5: the words

`5.1-reconciliation.md` "What the fold does today" is rewritten to what it does now and states the lifetime
document; `10-command-line-interface.md` §3.1's lifetime paragraph names it. The status file carries the revision.

## Design

```
  store/
    graphs/<checksum>.yaml            content-addressed plans, shared across lifetimes (unchanged)
    traces/<checksum>/<UTC>.yaml      one per scope run (unchanged)
    index.ndjson                      the detection hint (unchanged)
    lifetimes/
      current            -> 01a0d3...   the name of the current lifetime
      01a0c8....yaml     state: replaced  runs: [deploy Home, deploy System, upgrade Home, ...]
      01a0d3....yaml     state: current   runs: [deploy Home, deploy System]

  writ deploy ──pre-flight reads Fold(current)──▶ mint id ──▶ scope 1 trace ──▶ current := new; old := replaced
                                                            ──▶ scope 2 trace ──▶ appended
  writ upgrade / reconcile / adopt ──▶ trace ──▶ appended to current
  writ decommission (#929)          ──▶ trace ──▶ appended; state := ended
  readback.Fold                     ──▶ lifetimes/current ──▶ its runs, in order ──▶ one record
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-23; both open questions ruled.

### Phase 2: The lifetime document

- [x] `cmd/internal/cli/lifetime.go`: `Lifetime`, `LifetimeRun`, `NewLifetime`, `CurrentLifetime`, `LoadLifetime`,
      `AppendRun` (replaces the previous current on the first persisted run), `RequireCurrentLifetime` (66),
      `WriteLifetimeTrace`; atomic writes; four tests in `lifetime_test.go`.

### Phase 3: The writers

- [x] `deploy` mints after pre-flight and appends per scope; `upgrade`, `adopt`, `decommission` require the current
      lifetime and append; `secret`, `migrate` untouched. Dry runs return before any trace and touch no lifetime.

### Phase 4: The fold

- [x] The layer-journey scenario's step 3.4/3.5 ("the store remembers the old targets: after the hand cleanup they
      are `missing`", citing #845) encoded the unbounded fold; rewritten to the ruling: after the redeploy the record
      says nothing about the replaced lifetime's targets. The deploy scenario's `--store <empty>` refusal now names
      the missing deployment, not the run index.
- [x] `readback.Fold` reads the current lifetime; `collectRuns`, the index read, the trace glob, `traceTime` and
      `unsafeChecksum` go. Tests: `TestFold_NoLifetimeIsNotFound`, `TestFold_DeployReplacesTheRecord`,
      `TestFold_UpgradeStacksOnTheDeploy` (+ the kept nuked-trace finding); reconcile's missing-index test became
      `TestBuildReport_NoLifetimeIsNotFound`; the adopt harness deploys first. The two-scope case is covered at the
      lifetime level (runs append regardless of scope); the deploy fixture is single-scope.

### Phase 5: The words

- [x] Requirement 5: `5.1` "The lifetime in the store" and "What the fold does"; §3.1's pointer; both status files.

### Phase 6: Gates, metrics, and the VM sequence

- [x] `make check` exit 0 (`build/e2e/922-make-check.log`); `make test-scenario` exit 0, 8 passes. Coverage: total
      63.0%; `cmd/internal/cli` 68.8% -> 69.2%, `readback` 80.0%, `deploy` 78.7%, `adopt` 77.7%; `Fold` 96.3%,
      `AppendRun` 81.0%, `RequireCurrentLifetime` 83.3%, `WriteLifetimeTrace` 71.4%. Complexity: `Fold` 5,
      `AppendRun` 7, `replacePrevious` 6; nothing new over 8. Size: `lifetime.go` 335, `lifetime_test.go` 141,
      `readback.go` 461 (was 557).
- [x] Both VMs, snapshotted first (Linux `{01695fd8...}`, Windows `{f50b8f09...}`), build `a835739d-dirty`; logs
      `build/e2e/922-e2e-ud24.log`, `922-e2e-wd11.log`. On both: the store had no lifetime, so the first plain
      deploy refused every occupant (166 on Linux, 71 on Windows) -- the one-time migration step; `--conflict=replace`
      opened the first lifetime; a plain deploy then recognized every link and opened a second, the first `replaced`
      with its run kept; `upgrade` nothing to regenerate; `reconcile` exit 0 with **no missing and no orphan row**:
      Linux 166 linked + 1 copied (yesterday's 52 stale rows are gone -- the layer reorganization case, on the real
      history), Windows 71 linked. Timings: Linux 0.46 / 0.53 / 0.56 s; Windows 18.3 / 20.1 / 12.9 s.
- [x] Here: `make install`; the first plain `writ deploy common` refused 112 occupants in 3.0 s (no lifetime yet).
      The owner's row: one `writ deploy common --conflict=replace` opens this machine's first lifetime.

### Phase 7: Closure

- [ ] #922's five boxes ticked with evidence; lane 1 committed on `feature/922-lifetime-fold`; PR A opens when lanes
      2, 3 and 14 land on the same branch; this plan `complete` at the PR.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | a lifetime document round-trips and `current` follows the last open | unit, `cli` | the store forgets |
| 2 | deploy A then deploy B without A's file: `reconcile` does not mention it | integration, `cmd/writ` | the fold is unbounded |
| 3 | deploy then upgrade: one record, the upgrade's digests on the refreshed entry | integration | an upgrade opens a lifetime |
| 4 | a two-scope deploy then a one-scope upgrade folds as one record | integration | a lifetime is per scope |
| 5 | a trace on disk that no lifetime names is not folded | unit, `readback` | the index or the glob still feeds the fold |
| 6 | no `lifetimes/`: `Fold` returns `os.ErrNotExist` | unit | a fresh machine folds something |
| 7 | both VMs, the reorganize-then-deploy sequence | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/922-lifetime-fold.md` | Create: this plan |
| `cmd/internal/cli/lifetime.go`, `lifetime_test.go` | Create: the lifetime document |
| `cmd/internal/cli/store.go` | Modify: `WriteTrace` returns what `AppendRun` needs |
| `cmd/writ/writ/deploy/deploy.go` | Modify: mint, append, replace |
| `cmd/writ/writ/upgrade/upgrade.go`, `adopt/batch.go`, `decommission/decommission.go` | Modify: append |
| `cmd/writ/writ/readback/readback.go`, `readback_test.go` | Modify: the fold reads the current lifetime |
| `docs/architecture/5.1-reconciliation.md`, `10-command-line-interface.md`, both status files | Modify: Requirement 5 |

## Open questions

1. **A partial deploy** -- ruled 2026-09-23: the new lifetime is current with what it wrote, so the record matches
   the disk and says which scopes ran.
2. **`adopt` joins the lifetime** -- ruled 2026-09-23: yes. Adopt is a write into the current deployment; its trace
   appends, and it refuses with not-found when there is none. Making that trace a deployment record (the `files`
   annotation) and giving adopt its platform is [#931](https://github.com/NobleFactor/devlore-cli/issues/931), lane
   14, PR A beside this lane.

## Related Documents

- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule; this plan is lane 1
- [#922](https://github.com/NobleFactor/devlore-cli/issues/922) -- the task; [#762](https://github.com/NobleFactor/devlore-cli/issues/762) -- the feature
- [#923](https://github.com/NobleFactor/devlore-cli/issues/923), [#883](https://github.com/NobleFactor/devlore-cli/issues/883) -- lanes 2 and 3, PR A
- [#929](https://github.com/NobleFactor/devlore-cli/issues/929) -- decommission ends a lifetime; [#930](https://github.com/NobleFactor/devlore-cli/issues/930) -- pruning
- [docs/architecture/5.1-reconciliation.md](../../architecture/5.1-reconciliation.md)
