---
title: "Lane 17: the ledger snapshot compares etags and recomputes only what changed, and never Merkle-walks a tree"
issue: https://github.com/NobleFactor/devlore-cli/issues/904
status: complete
created: 2026-09-21
updated: 2026-09-22
---

# Plan: Lane 17 of the command line schedule

## Summary

Lane 17 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), a resource-model bug taken here because it
was found running lane 16's own `writ deploy`. `ResourceCatalog.Snapshot()` records, for every Active entry, both
content-identity tiers -- `Etag` and `Digest` -- unconditionally, and it is captured twice per run. A
`file.Directory`'s digest is a Merkle root of its whole tree, and the catalog holds directories nobody produced: the
creation boundaries `findClosestExistingDir` discovers, `~` among them. So every deploy on this machine hashes the home
directory, twice, for 169 seconds, and discards the result. Ruled 2026-09-21: compare, then recompute only if needed.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 904

Bug, Severity High, P1, epic #444; no feature owns the catalog and ledger (the owner places it). Measured
2026-09-21: 169 s, no output, one thread in `sha256.block` under `openat`/`read`/`fdopendir`, no git child. The three
rulings that compose into it are each right in its place: the ledger records both tiers; a directory's digest is its
Merkle root (ruling 5d); a boundary is a discovered resource. `directory.Etag`'s own comment states the rule the
snapshot never applies: "a changed Etag is the trigger for the full Digest comparison".

## Goals

- [ ] A second `Snapshot` over an unchanged catalog computes no digest: every entry's etag matches the prior's and
      the prior's digest is carried forward. `Trace()` after `Run` is 166 `lstat`s and no hashing.
- [ ] A tree writ only found -- a `file.Directory` discovered as a creation boundary, no producer -- records its
      etag in the ledger and no digest; a tree writ produced or moved keeps both tiers, so `reconcile` can still ask
      whether a directory writ deployed has changed since. Nothing that reads a ledger loses anything: `readback`
      looks up targets and sources by path, and a boundary is neither.
- [ ] `writ deploy common` on this machine ends in under five seconds, the trace recorded, both VM sequences green.

## Current State

Read 2026-09-21 at `409d1e89`.

| Component | Status | Notes |
| --- | --- | --- |
| `ResourceCatalog.Snapshot()` (`pkg/op/resource_catalog.go:539`) | ❌ | every Active entry: `Etag()` then `Digest()`, best effort, no comparison with any prior |
| `GraphExecutor.captureLedgerSnapshot` (`graph_executor.go:1247`) | ❌ | `Run`'s and `ResumeUnwind`'s defers capture once; `e.ledgerSnapshot` holds it |
| `GraphExecutor.Trace()` (`graph_executor.go:483`) | ⚠️ | snapshots again only while an environment is alive -- a mid-run call; after `Run` the defer has set `e.environment = nil` and `Trace()` projects the capture. **Corrected 2026-09-21 during Phase 3:** the deploy path captures once, not twice; the walk that made `~` a 169-second deploy is the single capture's `Digest()` on the boundary directories |
| `file.directory.Digest()` (`provider/file/directory.go:187`) | ✅ as designed | Merkle root, no skips, ruling 5d -- right for a directory that is a product; the ledger is the wrong caller |
| every kind's `Etag()` | ✅ | one `lstat`: sha256 of (size, mtime_ns, ino); a directory's moves on immediate-child change |
| `verifyLocationFreshness` (`resource_catalog.go:775`) | ✅ | the ladder already in use: etags first, digests only when they differ |
| `readback.recordedIdentity` (`readback.go:429`) | — | the one reader of ledger tiers: keys `file:` entries by path; consumers look up **targets** and **sources** -- `RecordedDigest`, `RecordedSourceDigest` -- so discovered sources' digests are consumed and must stay |
| `LedgerEntrySnapshot` (`:1002`) | ✅ | `ID`, `URI`, `ProducerID`, `State`, `DestroyedBy`, `Etag`, `Digest` -- what the comparison needs |
| `Rehydrate` | ✅ | reads identity and state only; a missing digest changes nothing on resume |
| `resource_ledger_snapshot_test.go` | ⚠️ | `TestSnapshot_CapturesContentIdentity`, `..._RoundTrips`; the probe has `etag`, `digest`, `digestErr`; no call count |

## Requirements

### Requirement 1: compare, then recompute only if needed

`Snapshot(prior *ResourceLedgerSnapshot)`. For each Active entry the catalog computes the `Etag` (one `lstat`); when
`prior` holds an entry with the same `ID` and the same `Etag`, the prior's `Digest` is carried forward without a
call; otherwise -- no prior, an unknown id, a changed or unreadable etag -- `Digest()` runs as today. A nil prior is
today's behavior. Pending and Gone entries record neither tier, as today.

### Requirement 2: the executor passes what it has

`captureLedgerSnapshot` passes `e.ledgerSnapshot` -- nil on a first run, the rehydrated ledger on a resume -- and
`Trace()` passes `e.ledgerSnapshot`, the defer's capture. After `Run` returns the environment is gone and `Trace()`
projects the capture without a snapshot (corrected 2026-09-21: there was never a second walk on the deploy path); a
mid-run `Trace()` compares against the capture and hashes only what moved.

### Requirement 3: a tree writ only found records its etag; a tree writ made keeps its digest

Ruled 2026-09-21: "if we move a directory or reconcile a directory we must ask, did this thing change?" -- so every
directory writ produced or moved keeps both tiers in the ledger, and only a directory writ merely found -- a creation
boundary, `ProducerID == ""` -- records its etag alone. The walks measured were all of the second kind: `~`,
`~/Library/Fonts`, `~/.config`, directories writ made nothing at and never asks about.

A resource whose `Digest()` is a Merkle root over a tree declares it through an optional contract in `pkg/op` --
`Tree`, one method, `IsTree() bool` -- and `Snapshot` records an entry's `Etag` alone when it is a tree **and** has no
producer. `file.directory` implements it. Kind alone is not the rule: `reconcile`'s cross-run question needs a
produced tree's fingerprint. Provenance alone is not the rule either: `readback` consumes discovered **sources'**
digests (`RecordedSourceDigest`, what `upgrade` compares), and a source is a regular file, cheap and needed. Live
questions -- resolving or moving a directory into a run -- go through `verifyLocationFreshness`, which reads the
disk and is untouched here.

`pkg/op` cannot name `file.Directory`; the contract is how the directory says what it is.

### Requirement 4: tests

In `resource_ledger_snapshot_test.go`, the probe gains a digest call counter. Cases: a second snapshot over an
unchanged catalog makes zero `Digest()` calls and carries every digest forward; a probe whose etag changed is
recomputed and the others are not; a probe absent from the prior is computed; a prior with an empty digest (it
errored) is recomputed rather than carried; a tree probe without a producer records its etag and no digest, and a
produced tree probe records both;
`Trace()` after `Run` in the executor tests makes no digest call.

### Requirement 5: the words

`Snapshot`'s doc comment states the ladder; `4-resource-management.md` §9 records the two rulings (the fuller
description of the catalog and ledger is lane 18, #906). `directory.Digest`'s comment names the ledger as a caller
it no longer has.

### Requirement 6: `Mkdir` claims truthfully (ruled 2026-09-22, [#907](https://github.com/NobleFactor/devlore-cli/issues/907))

`Mkdir` observes first and claims what happened: when the leaf already exists the product is a discovery
(`DiscoverDirectory`, no producer) and no receipt, as the receipt already is; when `mkdirAll` creates it the product
is a production (`NewDirectory` with the caller's stamp) and the `MutationCreateDir` receipt with its boundary, as
today. The returned `Directory` is the same value either way. With this, every deploy parent -- `.`, `.config`,
`.claude` -- enters the ledger as found, and Requirement 3's rule records its etag alone. A found directory is
Active, not Pending (found 2026-09-22 in the first receipt: 26 parents Pending, so no etag): the call observed it,
so it verifies it through `VerifyExistence`; nothing consumes a mkdir's product, so nothing else ever would. The
catalog is not
changed: it records faithfully which door the provider walks through, and `Mkdir` was walking through the producer
door before it knew.

## Design

```
  Run ... defer captureLedgerSnapshot():  Snapshot(e.ledgerSnapshot)  <- the capture: etags; digests for what the
                                                                          run made; a found tree etag-only
  Trace() after Run:                      e.ledgerSnapshot             <- the capture, projected; no snapshot
  Trace() mid-run:                        Snapshot(e.ledgerSnapshot)  <- compared; only what moved is hashed

  Snapshot(prior):
    for each Active entry:
      etag := resource.Etag()                          one lstat
      if resource.(Tree) && no producer:  record etag  a boundary: no Merkle walk
      elif prior[id].Etag == etag: carry prior digest  no I/O
      else:                        digest := resource.Digest()
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-21; Requirement 3 refined to provenance-and-kind by the owner's
      ruling; the contract is `Tree`.

### Phase 2: The catalog

- [x] `Snapshot(prior)` with the etag comparison; the `Tree` contract in `pkg/op`; `file.directory` implements it.
- [x] `captureLedgerSnapshot` and `Trace()` pass `e.ledgerSnapshot`; every other caller passes nil (four test sites).

### Phase 3: Tests

- [x] Requirement 4's cases: five in `resource_ledger_snapshot_test.go`, one in `graph_executor_test.go`;
  `make test` green.

### Phase 4: The words

- [x] Requirement 5: `Snapshot`'s doc comment; §9 items 20 and 21; `directory.Digest`'s comment names the ledger's rule.

### Phase 5: Gates and the three numbers

- [ ] `make check` and `make test-scenario`, both exit 0.
- [ ] Coverage per package and total, complexity, code size.

### Phase 6: `Mkdir` claims truthfully

- [x] Requirement 6 in `provider.go` (`Mkdir` returns the boundary it found, verified Active through
      `foundDirectory`); `TestMkdir_Idempotent` asserts the empty stamp, nil receipt and Active state over an existing
      directory; `TestMkdir_CreatesDirectory` asserts the caller's stamp and the receipt over a created one;
      `TestDirectory_IsTree`; `make check` green 2026-09-22 (`build/e2e/907-make-check-4.log`); `Mkdir` complexity 17 -> 14.

### Phase 7: Installed and exercised

- [x] `make install` 2026-09-22 10:46 and 11:10; `writ deploy common` here: **3.4 s** and **4.0 s**, exit 0, 111 links
      (was 182 s on 2026-09-21). The receipt: 26 directories, all Active, none with a producer, all with an etag, none
      with a digest; the 137 method receipts' UUIDv7 commits span 0.028 s and the receipt is written 0.0 s after the
      last, against 180.8 s the day before (devlore-cli#910 records the reading).
- [x] End to end 2026-09-22, build `409d1e89-dirty` (09:15Z), both VMs snapshotted first; logs
      `build/e2e/904-e2e-ud24.log`, `904-e2e-wd11.log`, receipts `904-ud24-receipt.yaml`, `904-wd11-receipt.yaml`.
      `danoble-ud24-1.local` (linux/arm64): self install 3/3; base and team unset, set by URL (git-named clones),
      set by path unchanged; personal by path; `deploy common noblefactor-ops` 166 files (165 links, 1 template)
      **0.84 s** under `stop`, **0.56 s** under `replace`; `upgrade` nothing to regenerate; receipt: 25 directories
      Active, 0 producers, 25 etags, 0 digests; 193 method receipts span 5 ms, receipt written 2 ms after the last.
      `danoble-wd11-3.local` (windows/arm64, Git Bash): the same repo steps; `deploy common noblefactor-ops
      --allow-dirty` 70 links, 1 skipped, **12.2 s** under `stop`, **15.4 s** under `replace`; `upgrade` no copied
      files; receipt: 20 directories Active, 0 producers, 20 etags, 0 digests; 91 method receipts span 9 ms, receipt
      written 18 ms after the last -- the Windows wall time is before the units (planning, pinning), not the capture.
      Bare `writ deploy` refuses on both ("requires at least 1 arg(s)"): devlore-cli#843, open, not this lane.
- [x] The owner's row: `writ deploy common --conflict=replace` 2026-09-22 11:04, 111 links, the receipt that
      showed the found parents Pending and led to Requirement 6's Active clause.

### Phase 8: Closure

- [x] #904's three acceptance boxes and #907's four ticked with evidence 2026-09-22; the pull request closes both;
      this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | a second snapshot over an unchanged catalog makes zero `Digest()` calls and carries every digest | unit, `op` | the comparison is not applied |
| 2 | a changed etag recomputes that entry alone | unit, `op` | a stale digest is carried, or everything recomputes |
| 3 | an entry absent from the prior is computed | unit, `op` | a new resource has no digest |
| 4 | an errored prior digest is recomputed, not carried | unit, `op` | an error becomes permanent |
| 5 | a discovered tree records etag only; a produced tree records both | unit, `op` | the home directory is walked again, or a deployed directory loses its fingerprint |
| 6 | `Trace()` after `Run` makes no digest call | unit, `op` | the second capture still hashes |
| 7 | `writ deploy common` here under five seconds | installed | the walk survives somewhere |
| 8 | both VM sequences, deploy timed | end to end | a platform differs |
| 9 | `Mkdir` over an existing directory: empty stamp, nil receipt | unit, `file` | a found directory is claimed as made |
| 10 | `Mkdir` that creates: the caller's stamp, the `MutationCreateDir` receipt | unit, `file` | a made directory loses its stamp or its undo |
| 11 | the receipt of `writ deploy common` shows `file:.` with no producer and no digest | installed | the stamp survives somewhere |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/904-ledger-snapshot.md` | Create |
| `pkg/op/resource_catalog.go` | Modify: `Snapshot(prior)` |
| `pkg/op/resource.go` | Modify: the `Tree` contract |
| `pkg/op/graph_executor.go` | Modify: the two capture sites |
| `pkg/op/provider/file/directory.go` | Modify: implements `Tree`; the digest comment |
| `pkg/op/resource_ledger_snapshot_test.go` | Modify: the cases |
| every other `Snapshot()` caller (tests) | Modify: `Snapshot(nil)` |
| `docs/architecture/4-resource-management.md` | Modify: §9, the two rulings |
| `pkg/op/provider/file/provider.go` | Modify: `Mkdir` claims a found directory as a discovery (#907) |
| `pkg/op/provider/file/provider_test.go` | Modify: `TestMkdir_Idempotent`, `TestMkdir_CreatesDirectory` assert the stamp and the receipt |

## Open questions

1. **A found directory is stamped as a product** (found 2026-09-22 in Phase 7's local deploy; **ruled 2026-09-22: option 1, logged as [#907](https://github.com/NobleFactor/devlore-cli/issues/907), lane 19, Requirement 6, Phase 6**). With Phases 2-5
   green, `writ deploy common` here still took 182 s, and a goroutine dump 40 s in shows `Snapshot` -> `ledgerDigest`
   -> `directory.Digest` -> `merkleRoot` over `file:.` -- the home directory. The tree rule did not fire because the
   entry carries `producer_id: file.mkdir-1`: `deploy` plans one `file.mkdir` per distinct target parent
   (`plan.go:400`), and `Mkdir` (`provider.go:357`) mints its product with the caller's stamp *before* it learns the
   directory already exists, returning it with no receipt ("nothing to compensate"). Every parent -- `.`, `.config`,
   `.claude` -- is therefore a product of a mkdir that made nothing. The receipt confirms it: 52 directory entries,
   22 with a digest; `res-1` (`file:.`) walked the home directory and recorded no digest at all (the walk errored on
   an entry kind it cannot hash). Options: (1) `Mkdir` over an existing directory interns a discovery --
   `DiscoverDirectory`, no producer -- so the catalog says what happened and the ledger records the etag alone; the
   product it returns is the same `Directory`; `TestMkdir_Idempotent` gains the assertion; (2) keep the stamp and add
   a made-versus-found mark to the entry for the ledger to read -- new machinery, and the catalog does not see
   receipts; (3) skip every tree's digest regardless of producer -- rejected 2026-09-21 ("if we move a directory or
   reconcile a directory we must ask, did this thing change?").

1. **The contract's name** -- ruled 2026-09-21: `Tree` / `IsTree()`.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lane 17
- [#904](https://github.com/NobleFactor/devlore-cli/issues/904) -- the bug
- [#906](https://github.com/NobleFactor/devlore-cli/issues/906) -- lane 18, the catalog and the ledger described
- [#444](https://github.com/NobleFactor/devlore-cli/issues/444) -- the resource model epic
- [docs/architecture/4-resource-management.md](../../architecture/4-resource-management.md) -- the design of record
