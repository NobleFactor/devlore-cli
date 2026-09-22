---
title: "Lane 18: the catalog and the ledger described -- what each holds, when the record is taken, the ladder"
issue: https://github.com/NobleFactor/devlore-cli/issues/906
status: complete
created: 2026-09-22
updated: 2026-09-22
---

# Plan: Lane 18 of the command line schedule

## Summary

Lane 18 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), PR F: a documentation task under the
resource model epic. `docs/architecture/4-resource-management.md` describes identity, states, shadowing, recovery and
says the catalog travels with the graph, but no section says what the catalog holds, what the ledger is, when it is
captured, or how the etag/digest ladder applies to it -- which is how three correct rulings composed into a Merkle
walk of the home directory (#904) and how a found directory stamped as a product went unseen (#907). The section is
written against `pkg/op/resource_catalog.go` at develop after PR E, and the two features ruled 2026-09-22 --
catalog, [#908](https://github.com/NobleFactor/devlore-cli/issues/908), the live model; ledger,
[#909](https://github.com/NobleFactor/devlore-cli/issues/909), the record -- give it its shape.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 906

Task, epic #444, feature #908 (Catalog). Done when the document gains the section covering the four points, the
status file carries the row, and the section reads true against `resource_catalog.go` at the commit that lands it.

## Goals

- [ ] A reader of `4-resource-management.md` can answer, without opening the code: what the catalog holds and how
      an entry arrives; what the ledger is and when it is written; what the etag/digest ladder is and where each
      side applies it; what a receipt's ledger fields mean and what an empty digest means.
- [ ] The word "ledger" names one thing in the document: the record. The catalog's storage is "the entries".
- [ ] `4-resource-management.status.md` records the revision; `make check` green.

## Current State

Read 2026-09-22 at `75350893` (develop after PR E, #911).

| Component | Status | Notes |
| --- | --- | --- |
| §2 (`:47`) | ⚠️ | names `ResourceCatalog` as "the append-only ledger plus the URI→id namespace" and lists its surface; `Snapshot()` is shown without its `prior`; no word on what the record holds |
| §3 (`:91`) | ✅ | the three states and who transitions them; discoveries are Pending until `VerifyExistence` |
| §4.1 (`:275`) | ✅ | the two-path reconciler: the etag/digest ladder as `Resolve` applies it on a cache hit |
| §5.4-5.5 (`:336`) | ⚠️ | graph = intent, trace = observation; the ledger snapshot named ("step 48") but not described |
| §9 items 20, 21 (`:523`) | ✅ | #904's two rulings, landed by PR E |
| `4-resource-management.status.md` | ⚠️ | last revision 2026-09-08 (#647); no row for PR E or this section |
| `pkg/op/resource_catalog.go` | ⚠️ | the type comment (`:15`), the `entries` field (`:39`), `Clone` (`:83`), `IntentEntries` (`:127`), `Len` (`:334`), `Shadow` (`:482`) call the entries slice "the ledger"; `Snapshot` (`:525`) calls the record "the ledger snapshot" -- two senses in one file |
| `ResourceLedgerSnapshot`, `LedgerEntrySnapshot` (`:1014`, `:1046`) | ✅ | the record's shape: `Root`, `NextID`, `Entries` of `ID`, `URI`, `ProducerID`, `State`, `DestroyedBy`, `Etag`, `Digest` |
| `GraphExecutor.captureLedgerSnapshot` (`graph_executor.go:1249`) | ✅ | the one capture per run: `Run`'s and `ResumeUnwind`'s defers; `Trace()` after `Run` projects it |
| readers of the record | ✅ | `Rehydrate` (resume), `readback.recordedIdentity` (drift, by target and source path), receipts by id |

## Requirements

### Requirement 1: what the catalog holds, and its doors

A subsection stating the catalog as the live model a run consults: the entries (every generation of every resource,
append order), the namespace (URI to current id, per addressing regime), the per-run state of each entry, the
producer stamp (the unit that made it; empty for a discovery) and the destroyer stamp, the id counter. The doors, by
name and by what each claims: `Discover` (found, no producer), `GetOrCreate` (made, stamped with the caller),
`Shadow` (a new generation at an occupied URI), `MarkGone`, `Link`, and the questions `Resolve`, `Lookup`,
`Current`, `State`, `VerifyExistence` answer. One sentence of consequence: the catalog records which door a
provider walked through, so a provider that claims before it observes puts a false fact in the live model -- #907
as the worked example, and #908 as the feature.

### Requirement 2: what the ledger is, and when it is written

A subsection stating the ledger as the record: `ResourceLedgerSnapshot`, the catalog projected once at the end of
`Run` (and `ResumeUnwind`), carried in `Trace.Catalog`, written to the receipt; the fields of a
`LedgerEntrySnapshot` in two groups -- identity (`ID`, `URI`, `ProducerID`, `State`, `DestroyedBy`, `Root` on the
snapshot) and observation (`Etag`, `Digest`, recorded for Active entries only). When it is taken: the defers, once;
`Trace()` after `Run` projects the capture; a mid-run `Trace()` compares against it. Its readers: resume, readback,
receipts. #909 as the feature.

### Requirement 3: the ladder, and where each side applies it

A subsection that puts §4.1's table beside the ledger's use of it: an etag is one `lstat` (a directory's moves on
an immediate-child change); a digest is content, and for a directory a Merkle root of the whole tree (ruling 5d);
`verifyLocationFreshness` runs the ladder on a `Resolve` cache hit; `Snapshot(prior)` runs it against the prior
record (§9 item 20), and a `Tree` with no producer records its etag alone (§9 item 21). The paragraph the owner
asked for on 2026-09-22: a trace records what the run left and what it replaced; a boundary is neither, so its
digest answers no question the record asks.

### Requirement 4: what a reader of a receipt may rely on

A subsection, table form: field by field, identity or observation, and what its absence means -- no `Etag` and no
`Digest`: the entry was not Active at capture; `Etag` alone: a found tree, or a digest that errored (best effort, the
error is not recorded); both: what the run made, or a file it found. The UUIDv7 `transaction_id` of a method
receipt as the one time stamp the record carries today, and [#910](https://github.com/NobleFactor/devlore-cli/issues/910)
as where the rest lands.

### Requirement 5: one word, one thing

In the document, "ledger" is the record and the catalog's storage is "the entries"; §2's summary line and the
surface listing are corrected (`Snapshot(prior)`). In `resource_catalog.go`, every comment in which "ledger" means
the entries slice -- the type comment, `entries`, `Clone`, `IntentEntries`, `GetOrCreate`, `Len`, `Shadow`,
`Supersede`, `rebindEntry`, `catalogLocked`, `pendingEntries`, `restoreEntry`, `Rehydrate`: twenty-one sites, not
the six first counted -- says "the entries" or "the catalog"; the comments on `Snapshot`, `ResourceLedgerSnapshot`
and `LedgerEntrySnapshot` keep "ledger" for the record. Comments only; no identifier changes.

### Requirement 6: the status file

`4-resource-management.status.md` gains the revision note (PR E's §9 items, this section) and the completion row.

## Design

```
  4-resource-management.md
    §5  The Catalog Travels with the Graph      (5.1-5.7 as today)
    §5.8 The catalog and the ledger             <- NEW: Requirements 1-4, four subsections
    §6  Recovery                                (unchanged)
    §9  items 20, 21                            (cross-referenced from 5.8, not repeated)
```

Placement is open question 1. The section says the same thing the two feature bodies say, in the document's
register, with the code's names.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-22: placement §5.8; the VM sequence runs (the 2026-09-18 ruling
      stands for every plan).

### Phase 2: The section

- [x] §5.8 with its four subsections (Requirements 1-4); §2's summary line and surface listing corrected; §3's and
      §4's three remaining uses of "ledger" for the entries corrected (lines 249, 258, 268).

### Phase 3: One word, one thing

- [x] Requirement 5's comment edits in `resource_catalog.go` (22 sites); `gofmt` clean; `make check` exit 0
      (`build/e2e/906-make-check.log`).

### Phase 4: The status file

- [x] Requirement 6: the revision note and two rows (PR E's, this section's).

### Phase 5: Gates and the VM sequence

- [x] `make check` exit 0 (`build/e2e/906-make-check.log`); every line of the section under 120 columns; every name
      in it checked against `resource_catalog.go`, `graph_executor.go` and `readback.go` at `75350893`. The markdown
      lint is not in CI and its tool (`markdownlint-cli2`) is not installed here; not run.
- [x] Both VMs, snapshotted first, build `75350893-dirty` (11:22Z); logs `build/e2e/906-e2e-ud24.log`, `906-e2e-wd11.log`.
      `danoble-ud24-1.local`: self install 3/3; repos by URL and by path; `deploy common noblefactor-ops` under
      `stop` **refused three occupants** (`.ssh/config`, the two PowerShell profiles: the VM's personal checkout
      moved them into `Home/common` at 13:28, a minute before the run, and the standing links pointed at the old
      layer path -- devlore-cli#883, open, not this lane); under `replace` 166 files in 0.54 s; a second `stop` pass
      after that 166 files in 0.77 s; `upgrade` nothing to regenerate. `danoble-wd11-3.local`: the same repo steps;
      `deploy common noblefactor-ops --allow-dirty` 70 links, 1 skipped, 28.5 s under `stop` and 27.6 s under
      `replace`; `upgrade` no copied files.

### Phase 6: Closure

- [x] #906's three boxes ticked with evidence 2026-09-22; the pull request closes it; this plan `complete`; the
      schedule's last lane closes with it.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the section names only functions and fields that exist at the landing commit | read-through against `resource_catalog.go`, `graph_executor.go`, `readback.go` | a name is stale |
| 2 | "ledger" occurs in the document only for the record | `grep -n ledger` read line by line | the entries slice is still called the ledger anywhere |
| 3 | `make check` | gate | a comment edit breaks a lint |
| 4 | `star lint markdown` | gate | a table or a line is malformed |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/906-catalog-and-ledger.md` | Create: this plan |
| `docs/architecture/4-resource-management.md` | Modify: §5.8 new; §2 corrected |
| `docs/architecture/4-resource-management.status.md` | Modify: the revision note and the row |
| `pkg/op/resource_catalog.go` | Modify: comments only, Requirement 5 |

## Open questions

1. **Placement** -- ruled 2026-09-22: §5.8 under "The Catalog Travels with the Graph"; nothing renumbers.
2. **The VM sequence** -- ruled 2026-09-22: it runs; the 2026-09-18 ruling admits no docs-only exception.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lane 18
- [#906](https://github.com/NobleFactor/devlore-cli/issues/906) -- the task
- [#908](https://github.com/NobleFactor/devlore-cli/issues/908), [#909](https://github.com/NobleFactor/devlore-cli/issues/909) -- the two features, ruled 2026-09-22
- [#904](https://github.com/NobleFactor/devlore-cli/issues/904), [#907](https://github.com/NobleFactor/devlore-cli/issues/907) -- the bugs the section explains
- [docs/plans/fix/904-ledger-snapshot.md](../fix/904-ledger-snapshot.md) -- lane 17's plan
- [docs/architecture/4-resource-management.md](../../architecture/4-resource-management.md) -- the design of record
