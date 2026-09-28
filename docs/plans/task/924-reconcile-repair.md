---
title: "Lane 5: reconcile repairs by running the named operation on the drifted subset"
issue: https://github.com/NobleFactor/devlore-cli/issues/924
status: in-progress
created: 2026-09-26
updated: 2026-09-27
---

# Plan: Lane 5 of the writ lifecycle schedule

> **Stopped 2026-09-27, before any code, on a design change.** The owner ruled that reconcile runs against the git
> ref that produced each entry, and that a lifetime keeps its layer snapshots as worktrees so links can target
> them. The design, its four rulings so far and the five still open, is
> [847-kept-snapshots.md](../feature/847-kept-snapshots.md). For this lane it means: open question 3 below is
> superseded, since a restore read from the ref always reproduces the record; the "no reverse sync" reason lapses
> for links, which retain branches now carry back; the exit status and the words move with that design's third
> open ruling; and restore becomes a command flag. This plan resumes after the replan of #916.

## Summary

Lane 5 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), PR B′ on its own. Ruled 2026-09-23 on
#924: reconcile **restores** the system to the record. Repair is not a third mechanism: the record holds digests,
not bytes, so repairing is deploy's or upgrade's mechanism applied to exactly the entries the report named. Report
and repair in one run; `--dry-run`, the global every command carries, selects the report alone; no command flag.

| Finding | What the run does |
| --- | --- |
| `absent` | re-deploys the entry from the record's source: re-link or re-render |
| `changed` | re-deploys the entry from the record's source; the source is unchanged, so the result matches the record |
| `dangling` | reported and left, `deploy` named: only deploy makes a new record |
| `stale` | reported and left, `upgrade` named: re-rendering would update the record, which is upgrade by definition |

Reconcile never updates the record and never writes into a source repository.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to
the owner for placement.

## Issue 924

Task, epic WritDeployment, feature #762; waits on lane 2 (#923), landed. Branch `feature/924-reconcile-repair`,
opened from develop at `5509da4d`.

## Goals

- [ ] `writ reconcile` restores every `absent` and `changed` entry, reports what it touched, and a second run
      reports clean at 0; `--dry-run` reports the same findings and touches nothing.
- [ ] `dangling` and `stale` are reported with `deploy` and `upgrade` named and are not touched; the record's
      entries are identical before and after.
- [ ] A restoring run's receipt stacks on the current lifetime as a `reconcile` run.
- [ ] The design pages, the command's help and the user guide say the above; the "no `--fix`" note resolves to
      "`--dry-run` selects the report".

## The two rulings the schedule lists as needed first

1. **The selection surface for repair** is settled by the 2026-09-23 ruling: the drifted subset the report names,
   with no flag; `--dry-run` selects the report.
2. **Whether a file deployment depends on package installation** does not arise here: a restoring run re-plans
   file chains only, and never a package.

## Current State

Read 2026-09-26 at `5509da4d`.

| Component | Status | Notes |
| --- | --- | --- |
| `reconcile.BuildReport` (`reconcile.go:60`) | ⚠️ | classifies and returns; nothing runs. `Config` has no dry-run |
| `runReconcile` (`commands.go`) | ⚠️ | emits, then exits 1 on drift (#756); reads no `--dry-run` |
| `Entry.Repair` (`reconcile.go:142–206`) | ⚠️ | names `writ deploy` for absent and changed links, `writ upgrade --force` for a changed copy: what the user would run. With this lane the run does it |
| `upgrade.buildScopeGraph` and `upgrade.runGraph` (`upgrade.go:432`, `:525`) | ✅ | the mechanism: re-plans recorded entries through `deploy.PlanFileChain`, one graph per scope, run under `conflict: replace`, the trace appended to the current lifetime. Upgrade-only today; see open question 2 |
| `cli.RunOperationReconcile` (`lifetime.go:46`) | ✅ | the operation name the lifetime records; unused so far |
| `journey.reconcile` (`scenario_layer_journey_test.go`) | ⚠️ | runs `reconcile -o json` as a read; after this lane a plain run restores, so the helper becomes `reconcile --dry-run` and a step exercises the repair |
| lane 4's drift scenario (`scenario_integration_test.go`) | ⚠️ | removes a link and expects 1 with `absent`; becomes `--dry-run` for the report, then a plain run that restores |
| `5.1-reconciliation.md` (`:14`, `:74`), `10-command-line-interface.md` (`:161`) | ⚠️ | "repair is in scope and unbuilt"; "the surface for selecting repair is undesigned" |
| `docs/guides/writ/manage-environments.md` "Reconcile" | ⚠️ | "reconcile reports and leaves the machine as it found it; you run the repair it names" |

## Requirements

### Requirement 1: the run

`reconcile.Execute(ctx, cfg) (*Report, error)`: `BuildReport`, then unless `cfg.DryRun`, the restoring run over the
`absent` and `changed` entries, grouped by scope, one graph per scope re-planned through the family's shared seam
(ruled 2026-09-26, open question 2, reading a: upgrade's `buildScopeGraph` and `runGraph` move to the deploy package
beside `PlanFileChain` as `deploy.PlanRecordedEntries` and `deploy.RunRecordedGraph`, parameterized by the run
operation, and upgrade and reconcile both call them) from each entry's recorded source, target, project and layer, and
run under `conflict: replace` so a foreign occupant at a `changed` target is archived to the recovery site before the
recorded content returns. No layer is pinned and no dirty-layer gate applies: the record is the reference, and a
source whose content moved is `stale` by digest before any run. The trace is appended to the current lifetime as a
`reconcile` run through `cli.WriteLifetimeTrace`; a run that has nothing to restore writes nothing. After the run each
restored entry is classified again; one that is not `linked` or `copied` now is a failure naming the target.

### Requirement 2: the report

`Entry` gains `Restored bool`, true for an entry the run re-deployed; `State` stays what the run found. `Repair`
names what restores the entry: `writ reconcile` for `absent` and `changed`, `writ deploy` for `dangling`,
`writ upgrade` for `stale`. `Report.HasDrift` and `DriftCount` stay as found; `Report.Remaining()` counts the drift
the run left. The stderr line becomes `reconcile: N of M entries drifted; K restored; J left`, kinds omitted when
zero.

### Requirement 3: the exit status

Ruled 2026-09-26 (open question 1, reading a): the status is the state at exit. 0 when every entry is `linked` or
`copied` once the run returns; 1 when `dangling` or `stale` remain, or a restore failed; 66 when never deployed. Drift
that was found and restored is in the report and the stderr line, not the code. Under `--dry-run` the status is lane
4's: 1 on any drift, since nothing was restored.

### Requirement 4: the tests

1. `reconcile_integration_test.go`: from the Classifications fixture, `Execute` restores an absent copy, a changed
   link and a changed copy, leaves a stale copy and a dangling link with their names, and a second `Execute`
   reports clean; `--dry-run` restores nothing and the targets are untouched; the lifetime's last run is
   `reconcile`; the record's entries are equal before and after a run that restores.
2. `scenario_integration_test.go`: lane 4's drift step reads with `--dry-run` (66, 1), then a plain run restores
   the removed link and exits per open question 1, and a plain run after it exits 0.
3. The layer journey's helper reads with `--dry-run`; one step removes a target and edits a copy, runs the plain
   form, and asserts both restored and the record unchanged.

### Requirement 5: the pages

- `5.1-reconciliation.md`: the header note and "At reconcile" say repair is built and how; the words table's
  repair column says `reconcile` restores absent and changed; the "no `--fix`" sentence resolves to `--dry-run`.
- `10-command-line-interface.md` §3.1: "the repair half is chartered, not built" becomes what the run does.
- `commands.go` reconcile `Long`: the table gains what the run does per word and a line on `--dry-run`.
- `docs/guides/writ/manage-environments.md` "Reconcile": reconcile restores; `--dry-run` reports.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered (continue, 2026-09-27); open questions 1 and 2 ruled 2026-09-26 (both (a)).

### Phase 2: The code and its tests

- [ ] Requirements 1–4; `make check` and `make test-scenario` green.

### Phase 3: The pages

- [ ] Requirement 5.

### Phase 4: Both virtual machines

Snapshot first; the built writ by the remote smoke-test procedure. On each, #924's acceptance run: `writ deploy`
(bare); remove one linked target, edit one copy, edit one source; `writ reconcile --dry-run` reports three and
exits 1 touching nothing; `writ reconcile` restores two, reports one `stale`, exits 1; `writ upgrade`;
`writ reconcile` exits 0. The lifetime's runs read deploy, reconcile, upgrade. Output under
`build/e2e/924-e2e-<host>.log`.

- [ ] `danoble-ud24-1.local` (linux/arm64)
- [ ] `danoble-wd11-3.local` (windows/arm64)

### Phase 5: Closure

- [ ] Coverage, size and complexity in the report; the commit; PR B′ (`Closes #924`); the schedule and thread
      reports.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | absent and changed are restored, dangling and stale left, the record unchanged | unit, `reconcile` | the run touches the wrong set |
| 2 | `--dry-run` touches nothing | unit, `reconcile` | the run ignores the flag |
| 3 | the receipt stacks as a `reconcile` run | unit, `reconcile` | the lifetime is not written |
| 4 | the command restores end to end and the codes follow | scenario | the wiring or the code is wrong |
| 5 | the acceptance sequence on a real install | both VMs | the machine behaves otherwise |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/task/924-reconcile-repair.md` | Create: this plan |
| `cmd/writ/writ/reconcile/reconcile.go`, `report.go` | Modify: `Execute`, `Restored`, `Remaining`, the repair names |
| `cmd/writ/writ/deploy/recorded.go` (new) | Create: `PlanRecordedEntries` and `RunRecordedGraph`, moved from upgrade |
| `cmd/writ/writ/upgrade/upgrade.go` | Modify: calls the shared seam; its own builder and runner go |
| `cmd/writ/writ/reconcile/restore.go` (new) | Create: the restoring run over the drifted subset |
| `cmd/writ/writ/commands.go`, `config.go` | Modify: `--dry-run` into the config, the stderr line, the `Long` |
| `cmd/writ/writ/reconcile/reconcile_integration_test.go` | Modify: tests 1–3 |
| `cmd/writ/scenario_integration_test.go`, `scenario_layer_journey_test.go` | Modify: test 4 |
| `docs/architecture/5.1-reconciliation.md` and status, `10-command-line-interface.md` and status | Modify: requirement 5 |
| `docs/guides/writ/manage-environments.md` | Modify: the reconcile section |

## Open questions

3. **Superseded 2026-09-27 by the kept-snapshots design:** restoring from the pinned ref always gives back what the
   record says, so no precondition is needed. Kept for the record. **Found 2026-09-27 starting phase 2, before any
   code: an `absent` or `changed` entry can also have a source that is gone or has moved.** The classifiers decide
   `absent` and `changed` from the target before they look at the source (`reconcile.go`, `classifyLink` and
   `classifyCopied`), so the report's `absent` and `changed` sets can hold entries whose restore would not give back
   what the record says:
   - a link whose recorded source no longer resolves: restoring it re-creates a dangling link;
   - a copy whose source's content moved: re-rendering it writes new content, which updates the record -- upgrade's
     job by the 2026-09-23 ruling, and a breach of "reconcile never updates the record".

   Proposed: the run restores an `absent` or `changed` entry only when the result is what the record says -- a link
   whose recorded source resolves; a copy whose source is readable and whose content still has the recorded source
   digest (a copy with no recorded source digest cannot be proved, and is left). Any other is reported with its word
   and left, naming the repair for its cause: a source that is gone names `writ deploy`, a source that moved names
   `writ upgrade` (`writ upgrade --force` for a `changed` copy, whose local edit upgrade otherwise keeps). This is the
   2026-09-23 invariant applied, not a new rule, but it decides which entries the run touches, so it is put to the
   owner before code.

Ruled 2026-09-26 by the owner:

1. **The exit status of a run that restored:** (a), the state at exit (Requirement 3).
2. **Where the restoring mechanism lives:** (a), upgrade's builder and runner move to the deploy package as the
   family's shared seam, called by upgrade and reconcile (Requirement 1).

## Related Documents

- [#924](https://github.com/NobleFactor/devlore-cli/issues/924) -- the ruling and the acceptance list
- [#913](https://github.com/NobleFactor/devlore-cli/issues/913) -- reconcile restores the system to the record
- [923-reconcile-words.md](../feature/923-reconcile-words.md) -- the words the run reads
- [756-reconcile-exit-codes.md](../fix/756-reconcile-exit-codes.md) -- the codes the run keeps under `--dry-run`
