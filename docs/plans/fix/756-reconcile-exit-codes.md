---
title: "Lane 4: reconcile's three exit codes -- 66 never deployed, 0 clean, 1 drifted"
issue: https://github.com/NobleFactor/devlore-cli/issues/756
status: in-progress
created: 2026-09-24
updated: 2026-09-24
---

# Plan: Lane 4 of the writ lifecycle schedule

## Summary

Lane 4 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), PR B, first commit on this branch. Ruled
2026-09-23 on the issue: reconcile is valid only after a deployment, and its exit status is one of three answers a
script can gate on the way it gates on `git diff --exit-code`:

| Answer | Exit |
| --- | --- |
| never deployed: no record to compare against | `ExitNoInput` (66) |
| deployed and clean: the system matches the record | `ExitOK` (0) |
| deployed and drifted: any `absent`, `changed`, `dangling` or `stale` entry | `ExitError` (1) |

The issue was filed for the opposite reading, an absent index read as empty; it closes the ruled way. The index
is a hint the fold no longer consults ([#922](https://github.com/NobleFactor/devlore-cli/issues/922)), so the
original fix has nothing left to attach to.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to
the owner for placement. Lane 5 ([#924](https://github.com/NobleFactor/devlore-cli/issues/924), repair) follows on
this branch under its own plan.

## Issue 756

Bug, epic WritDeployment, feature #762; waits on nothing. Branch `fix/756-reconcile-exit-codes`, opened from
develop at `d54626d2`.

## Goals

- [ ] `writ reconcile` exits 66 on a store with no current deployment, 0 when every entry is `linked` or `copied`,
      and 1 when any entry is `absent`, `changed`, `dangling` or `stale`; the report is rendered in every case it
      exists.
- [ ] The two design pages, the command's help and the user guide say the three codes.
- [ ] Every test that runs `writ reconcile` asserts the exit code the entries call for.

## Current State

Read 2026-09-24 at `d54626d2`.

| Component | Status | Notes |
| --- | --- | --- |
| `readback.Fold` (`readback.go:182`) | ✅ | answers a wrapped `os.ErrNotExist` naming "no current deployment" when the store has no current lifetime |
| `deploy` pre-flight (`deploy.go:286`) | ✅ | reads that not-found as an empty inventory: a never-deployed machine deploys. Must keep working, so the code is not assigned in `Fold` |
| `reconcile.BuildReport` (`reconcile.go:60`) | ❌ | passes the not-found error up uncoded; `ExitCode` maps it to 1, the same status as drift |
| `runReconcile` (`commands.go:249`) | ❌ | emits the report and returns nil: drift exits 0 |
| `reconcile.Report` (`report.go`) | ❌ | no predicate says whether the report holds drift |
| `cli.RequireCurrentLifetime` (`lifetime.go:198`) | ✅ | already codes 66 for the writing operations and names this lane as the reader's twin |
| `10-command-line-interface.md` §3.1 (`:168`) | ⚠️ | names the three answers, not the numbers; §9 (`:695`) already lists drift under 1 |
| `5.1-reconciliation.md` (`:68`, `:151`) | ⚠️ | "not-found rather than an empty report"; the words table carries no exit status |
| `commands.go` reconcile `Long` (`:230`) | ⚠️ | the six words and their repairs; no exit status |
| `docs/guides/writ/manage-environments.md` | ❌ | the whole page is behind the tool: `--packages`, `--fix`, a nine-row symbol table in the retired vocabulary, `writ adopt <project> <file>` (the project is `--project`), a `writ inspect` command that does not exist, a decommission "state file" table that never existed. **Ruled 2026-09-24: rewritten in full to the six words and the current option set, folded into this lane** |
| `commands.go` decommission `Example` (`:131`) | ❌ found | `writ decommission --force noblefactor` names a flag decommission does not define; see open question 2 |
| `TestBuildReport_NoLifetimeIsNotFound` (`reconcile_integration_test.go:281`) | ⚠️ | asserts `os.ErrNotExist`, not the code |
| `scenario_integration_test.go:520` | ⚠️ | `--store <empty>` asserts the message, not 66 |
| `journey.reconcile` (`scenario_layer_journey_test.go:397`) | ⚠️ | fatal on any nonzero exit; after this lane a drifted step exits 1 by contract, and the helper must assert the code the entries call for |

## Requirements

### Requirement 1: never deployed is 66

`BuildReport` codes the fold's not-found: `errors.Is(err, os.ErrNotExist)` becomes
`cli.ExitWith(cli.ExitNoInput, err)`, the message unchanged. The code is assigned in the reader that answers the
question, not in `Fold`, because deploy's pre-flight reads the same not-found as "nothing to conflict with".

### Requirement 2: drifted is 1, with the report rendered

`Report` gains the predicate `HasDrift() bool`: true when any entry's state is other than `StateLinked` or
`StateCopied`. `runReconcile` emits the report first, then returns
`cli.ExitWith(cli.ExitError, fmt.Errorf("reconcile: %d of %d entries drifted", …))` when the predicate holds. The
report reaches stdout under every `--output`, including `none`, where the exit code is the result (§8); the one-line
error is stderr's, as §9 requires of every failure. `--silent` never changes the code (§10).

Health findings (a trace the lifetime names that is gone) are the store's self-report, not drift: the ruling's list
is the four words, and the findings stay in the report at exit 0.

### Requirement 3: the tests carry the contract

1. `TestBuildReport_NoLifetimeIsNotFound` also asserts `cli.ExitCode(err) == cli.ExitNoInput`.
2. A unit test for `HasDrift`: false on linked and copied alone, true on each of the four.
3. `scenario_integration_test.go`'s `--store <empty>` step asserts exit 66 through `exec.ExitError`, and a new step
   removes one deployed link, runs `reconcile -o json`, and asserts exit 1 with the report on stdout naming that
   target `absent`; then redeploys and asserts 0.
4. `journey.reconcile` asserts, on every call, that the exit is 1 exactly when the entries hold drift and 0
   otherwise; every existing step keeps its assertions on the words.

### Requirement 4: the pages say the numbers

- `10-command-line-interface.md` §3.1: the "valid only after deployment" paragraph carries the three-row table
  and the `reconcile` row of the operations table points at it.
- `5.1-reconciliation.md`: the words table gains an "Exit" column (`linked`, `copied` → 0; the four → 1), and the
  "not-found" sentence names 66.
- `commands.go` reconcile `Long`: an "Exit status" block after the words.
- `docs/guides/writ/manage-environments.md`, rewritten in full (ruled 2026-09-24): the record as the thing every
  operation acts on; deploy with `--conflict`, `--allow-dirty`, `--segment`, `--dry-run`; reconcile with the six
  words, their repairs and the three exit codes; upgrade with `--force`; adopt by `--project`, `--layer`,
  `--platform`, `--from-receipt`; decommission with `--prune`; migrate as it is. `writ inspect` and the decommission
  state-file table are gone, since neither exists.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered (go, 2026-09-24); open question 1 ruled (the guide rewritten in full and folded in) and open question 2 ruled (the example line dropped).

### Phase 2: The code and its tests

- [x] Requirements 1, 2, 3; `make check` and `make test-scenario` green on darwin/arm64, 2026-09-24. The report also gains `DriftCount`, which the one-line error and the predicate share.

### Phase 3: The pages

- [x] Requirement 4, 2026-09-24. Found and fixed on the same page: 5.1's "At reconcile" item still read `fresh`, `modified`, `conflicting`, #923's leftover; it now reads the six words.

### Phase 4: Both virtual machines

Snapshot first. Install the built writ by the remote smoke-test procedure. On each: `writ reconcile` on the
current lifetime (0, clean); `writ reconcile --store <fresh>` (66, the message); remove one deployed link and
`writ reconcile -o json` (1, the report names it `absent`, the code in `$?`); `writ deploy` and `writ reconcile`
(0). Raw output kept under `build/e2e/756-e2e-<host>.log`.

- [x] `danoble-ud24-1.local` (linux/arm64), 2026-09-24: 0 clean (167 entries), 66 on a fresh store with the message, 1 with the report naming the removed link `absent` (also under `-o none` and `--silent`), 0 after the redeploy. The plain redeploy first refused because the VM's personal clone has uncommitted changes; `--allow-dirty` restored it. Logs `build/e2e/756-e2e-ud24.log` and `-step3.log`.
- [ ] `danoble-wd11-3.local` (windows/arm64), 2026-09-24: 0 clean, 66 on a fresh store with the message, 1 with the report naming the removed link `absent` (also under `-o none` and `--silent`). The first drift step removed nothing because the selector tested `/.config/` against backslash paths; rerun with a path-neutral selector. The restore step (`writ deploy common noblefactor-ops --allow-dirty`) was refused: nine occupied targets not in the record. Diagnosed from the store, read-only: my deploy at 13:10:48Z wrote graph `3f62…` (72 links, the nine annotated and receipted, lifetime `01a0d38a`); a deploy I did not run, at 13:20:49Z, wrote graph `a1a8…` (63 links, project `common` alone) and replaced that lifetime with `01a0d393`; the nine links on disk now belong to a replaced lifetime and are foreign by the ruled semantics. The record and the fold are right; the machine needs a full deploy. Ruled 2026-09-24: no `writ deploy` runs anywhere until the bare form (lane 9, pulled ahead into PR B) works; the VM's restore is a bare deploy after it lands. Logs `build/e2e/756-e2e-wd11.log`, `-step2.log`, `756-wd11-store-read*.sh`.
- [ ] Unexplained on `danoble-ud24-1.local`: one `writ reconcile` at about 18:12Z reported `1 of 167 entries drifted`; the next, at 18:16Z, and every one since, report clean. Not reproduced; recorded, not diagnosed.

### Phase 5: Closure

- [ ] Coverage, size and complexity in the report; the commit on this branch; #756 closes with PR B.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | not-found is coded 66 | unit, `reconcile` | the code is assigned nowhere |
| 2 | `HasDrift` reads exactly the four words | unit, `reconcile` | a word is miscounted |
| 3 | `--store <empty>` exits 66 | scenario | the code is lost between `BuildReport` and `main` |
| 4 | a removed link exits 1 with the report rendered | scenario | the emit is skipped or the code is 0 |
| 5 | every journey reconcile's code matches its entries | scenario, layer journey | any step's code disagrees with its words |
| 6 | the three codes on a real install | both VMs | the store on a machine behaves otherwise |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/756-reconcile-exit-codes.md` | Create: this plan |
| `cmd/writ/writ/reconcile/reconcile.go` | Modify: code the not-found |
| `cmd/writ/writ/reconcile/report.go` | Modify: `HasDrift` |
| `cmd/writ/writ/reconcile/reconcile_integration_test.go` | Modify: tests 1, 2 |
| `cmd/writ/writ/commands.go` | Modify: `runReconcile`, the `Long` |
| `cmd/writ/scenario_integration_test.go` | Modify: tests 3, 4 |
| `cmd/writ/scenario_layer_journey_test.go` | Modify: test 5 |
| `docs/architecture/10-command-line-interface.md`, `docs/architecture/5.1-reconciliation.md` | Modify: the numbers |
| `docs/guides/writ/manage-environments.md` | Rewrite: the whole page (ruled 2026-09-24) |

## Open questions

1. **Ruled 2026-09-24.** The user guide was in the retired vocabulary throughout; the owner ruled it rewritten in
   full to the six words and the current option set, in this lane's commit.
2. **Ruled 2026-09-24: dropped.** `writ decommission --help` showed an example, `--force`, for a flag it does not
   define (`commands.go:131`); found while reading the option set for the guide, removed in this commit.

## Related Documents

- [#756](https://github.com/NobleFactor/devlore-cli/issues/756) -- the bug and the 2026-09-23 ruling
- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule; [#924](https://github.com/NobleFactor/devlore-cli/issues/924) -- lane 5, the rest of PR B
- [923-reconcile-words.md](../feature/923-reconcile-words.md) -- the six words this lane's exit reads
- [10-command-line-interface.md §9](../../architecture/10-command-line-interface.md) -- the exit-code set
