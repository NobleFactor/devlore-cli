---
title: "The four lifecycle operations are defined by what each does to the record"
issue: https://github.com/NobleFactor/devlore-cli/issues/913
status: complete
created: 2026-09-22
updated: 2026-09-22
---

# Plan: the lifecycle operations and the record

## Summary

A documentation task under [#762](https://github.com/NobleFactor/devlore-cli/issues/762), the writ lifecycle
surface. Ruled 2026-09-22: the four lifecycle operations are defined by what each does to **the record** -- the
deployment's record of what is deployed, the receipts writ leaves. `deploy` replaces it, `upgrade` updates it,
`decommission` removes it, `reconcile` restores the system to it. The two design pages that carry the lifecycle
table state each verb from the mechanism's side and never name the record as the thing every verb acts on, and
#762's plan has "reconcile repairs" as one row of that table and nothing for the other three. The owner: "this
should not be something you have to save to memory. The code base and its supporting documentation should exude
this."

**This plan covers this one task and nothing else.** Anything found while working it stops the work and goes to
the owner for placement.

## Issue 913

Task, epic WritDeployment, feature #762. Found on `danoble-ud24-1.local` 2026-09-22: after a layer reorganization,
`writ reconcile` reports 52 records as `orphan`, then `missing` once their targets are removed -- words that treat
the record as the desired state when the checkout is -- and nothing retires them, because the inventory is
`readback.Fold`, a fold over every receipt the machine has ever written. Under the ruling the next `deploy` is the
record and those rows are gone with it; `reconcile` never retires anything.

Refined the same day: **each deploy is one lifetime.** Its receipts stack and fold naturally -- the deploy, then
the upgrades and reconciliations that follow it -- and the record is that stack, watched ebb and flow until a
later deploy replaces it or a decommission removes it. The fold is not wrong; it is unbounded. Today it folds
every lifetime the machine has ever had into one record.

## Goals

- [ ] A reader of either page meets the four-row table -- operation, effect on the record -- before the mechanics,
      and can say what each verb does to the record without reading code.
- [ ] `5.1` says in one sentence what `readback.Fold` does today and that it is not the ruling.
- [ ] #762's plan and its GitHub work list carry the three implied rows beside "reconcile reports and repairs".

## Current State

Read 2026-09-22 at `0b9426c1`.

| Component | Status | Notes |
| --- | --- | --- |
| `10-command-line-interface.md` §3.1 "The lifecycle is named once" | ⚠️ | a per-program table: `deploy` "link, write, and create files", `reconcile` "compare the receipt against the live system, report drift", `upgrade` "constrained re-deploy", `decommission` "reverse traversal" -- mechanism, not effect; "reconcile reports and repairs" stated below it |
| `5.1-reconciliation.md` "Relationship to the Writ Lifecycle" | ⚠️ | a four-row table of what happens and which record consumers read it; `Reconcile`: "fold the run index; classify against recorded identity" |
| `5.1-reconciliation.md` header note (2026-08-31) | ✅ | reconcile repairs as well as reports; no `--fix`; the repair surface undesigned |
| `docs/plans/feature/762-lifecycle-scopes.md` Requirement 2 | ⚠️ | "reconcile produces a report ... the repair half is chartered, not built here"; nothing on deploy, upgrade or decommission and the record |
| #762 work list | ⚠️ | "reconcile reports AND repairs" is the one row; the other three verbs' effect on the record is unstated |
| `cmd/writ/writ/readback/readback.go` `Fold` | ❌ by the ruling | folds every trace in the run index, newest per target winning; a deploy adds to the record and replaces nothing |
| `cmd/writ/writ/reconcile/report.go` | ❌ by the ruling | classifies a record whose source is gone as `orphan` (target present) or `missing` (target absent) |
| `5.1-reconciliation.status.md`, `10-command-line-interface.status.md` | ⚠️ | no row for this revision |

## Requirements

### Requirement 1: the table, in the command reference

`10-command-line-interface.md` §3.1 opens with the table below, the per-program table following it as the
mechanics; one sentence says the record is the subject of every verb.

### Requirement 2: the table, in the reconciliation page

`5.1-reconciliation.md` "Relationship to the Writ Lifecycle" opens with the same table; the existing table of
mechanisms and record consumers follows. One paragraph states the lifetime model: a deploy opens a lifetime, its
upgrades and reconciliations stack on it, and the record is the fold of that stack until a deploy replaces it or a
decommission removes it. One sentence states what `readback.Fold` does today -- folds every receipt the machine has
ever written, across lifetimes -- and that bounding it to the current lifetime is #762's, chartered and not built.
The header note gains the ruling's date.

### Requirement 3: the status files

Both status files note the revision, dated, with the issue.

### Requirement 4: #762 carries the other three rows

`762-lifecycle-scopes.md` gains Requirement 10, "The record", stating the table, the lifetime model, and the
fold bounded to one lifetime as chartered work; #762's GitHub work list gains three boxes -- deploy replaces the
record (a lifetime begins; the fold is bounded to it), upgrade updates it, decommission removes it -- beside
"reconcile reports AND repairs".

## Design

```
| Operation      | Effect on the record                                                       |
| -------------- | -------------------------------------------------------------------------- |
| `deploy`       | replaces it: the new deployment is the record; what the previous record    |
|                | held and this one does not is gone from it                                 |
| `upgrade`      | updates it: the same deployment, refreshed                                 |
| `decommission` | removes it                                                                 |
| `reconcile`    | restores it: brings the system back to what the record says -- git diff,   |
|                | then one day a merge tool                                                  |
```

```
  deploy ──┬── upgrade ── reconcile ── upgrade ── ... ──┬── deploy   (replaced: a new lifetime)
           │            one lifetime: receipts stack,   │
           │            the record is their fold        └── decommission   (removed)
```

A machine sees many lifetimes, one per deploy, and the store keeps them all; one will eventually want to prune
them. Pruning is housekeeping on the store, not a fifth verb: the four operations act on the current lifetime's
record, and pruning retires lifetimes that are no longer current. Named here so the pages leave room for it; not
this task's, and not yet #762's.

Documentation only. Bounding the fold to a lifetime, reconcile's classification, and pruning are named here and
not started.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-22 ("make sure this is recorded in the plan and the design
      documents").

### Phase 2: The two pages

- [x] Requirements 1 and 2: §3.1 and the header note plus "Relationship to the Writ Lifecycle"; the table, the
      lifetime model, pruning, and the fold as built today.

### Phase 3: The status files, the plan, the issue

- [x] Requirements 3 and 4: both status files; `762-lifecycle-scopes.md` Requirement 10; #762's work list.

### Phase 4: Gates and the VM sequence

- [x] `make check` exit 0 (`build/e2e/913-make-check.log`); every changed line under 120 columns; the three table
      blocks byte-identical.
- [ ] Both VMs -- **waived by the owner 2026-09-22** ("Let's then do the PR for this and call it a day"): prose only,
      no binary changed since lane 18's sequence ran on `75350893`.

### Phase 5: Closure

- [x] #913's four boxes ticked with evidence 2026-09-22; the pull request closes it; this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the table is identical on both pages and in #762's plan | `diff` of the three blocks | they drift |
| 2 | `5.1` names `readback.Fold` and says what it does at `0b9426c1` | read-through against `readback.go` | the sentence is stale |
| 3 | `make check` | gate | a markdown table breaks a lint that reads it |
| 4 | both VM sequences | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/913-lifecycle-record.md` | Create: this plan |
| `docs/architecture/10-command-line-interface.md` | Modify: §3.1 |
| `docs/architecture/5.1-reconciliation.md` | Modify: the header note; "Relationship to the Writ Lifecycle" |
| `docs/architecture/10-command-line-interface.status.md` | Modify: the revision |
| `docs/architecture/5.1-reconciliation.status.md` | Modify: the revision |
| `docs/plans/feature/762-lifecycle-scopes.md` | Modify: Requirement 10 |

## Open questions

1. **The fold bounded to a lifetime, as its own issue** -- deferred by the owner 2026-09-22: "we'll talk about
   this when the time comes." It stays a box on #762's list.

## Related Documents

- [#913](https://github.com/NobleFactor/devlore-cli/issues/913) -- the task
- [#762](https://github.com/NobleFactor/devlore-cli/issues/762) -- the feature; [its plan](762-lifecycle-scopes.md)
- [docs/architecture/5.1-reconciliation.md](../../architecture/5.1-reconciliation.md),
  [docs/architecture/10-command-line-interface.md](../../architecture/10-command-line-interface.md) -- the pages
- [docs/plans/feature/906-catalog-and-ledger.md](906-catalog-and-ledger.md) -- lane 18, where the 52 rows were found
