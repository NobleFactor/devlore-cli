---
title: "Lane 2: reconcile's words name system-versus-record drift, the record as the reference"
issue: https://github.com/NobleFactor/devlore-cli/issues/923
status: complete
created: 2026-09-23
updated: 2026-09-23
---

# Plan: Lane 2 of the writ lifecycle schedule

## Summary

Lane 2 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), PR A, on the branch lane 1 committed to.
Ruled 2026-09-23: **the record is the desired state**; the layer checkout is only deploy's input and becomes the
desired state by being deployed. `reconcile` compares the system to the record and to nothing else, and says so in
six words: `linked` and `copied` when the system matches the record; `absent` when the record says a target is
there and it is not; `changed` when the target is there but is not what the record says; `dangling` when a link is
what the record says and its referent does not resolve; `stale` when the deployed thing is faithful and its source
has moved on. `orphan`, `missing`, `conflict`, `modified` and `modified-or-stale` are retired.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 923

Task, epic WritDeployment, feature #762; waits on #922, which landed on this branch at `217f52fb`.

## Goals

- [ ] `writ reconcile`'s state vocabulary is exactly `linked`, `copied`, `absent`, `changed`, `dangling`, `stale`,
      each with the repair it names, and no system-side word reads the layer checkout.
- [ ] The two pages and the command's help say the six words and what each means.

## Current State

Read 2026-09-23 at `217f52fb`.

| Component | Status | Notes |
| --- | --- | --- |
| `reconcile/report.go` `State` (`:85`) | ❌ | eight states by `iota`: linked, copied, missing, conflict, orphan, modified-or-stale, stale, modified |
| `classifyLink` (`reconcile.go:151`) | ❌ | absent target -> `missing`; not a symlink -> `conflict`; referent unresolvable and source gone -> `orphan` (`decommission`); source unresolvable -> `orphan`; endpoint differs -> `conflict`; else `linked`. Never compares the referent's content to the recorded source digest |
| `classifyCopied` (`reconcile.go:202`) | ⚠️ | absent -> `missing`; source unreadable -> `orphan`; target digest != recorded -> `modified` (`upgrade --force`); source moved -> `stale` (`upgrade`); no recorded identity -> `modified-or-stale` |
| `modified-or-stale` | ❌ by #922 | a lifetime is new, so every run in one carries the step-48 identity; the indeterminate class has no case left |
| `commands.go` `reconcile` Long (`:230`) | ❌ | the eight-word table |
| `reconcile_integration_test.go` (`:146`, `:151`, `:191`, `:204`) | ⚠️ | asserts missing, conflict, modified, orphan |
| `scenario_layer_journey_test.go` (`:1138`) | ⚠️ | logs `orphan=29` between the commits; no assertion on the word |
| `5.1-reconciliation.md` (`:145`), `10-command-line-interface.md` | ⚠️ | say the words are retired by #923; do not yet list the six |

## Requirements

### Requirement 1: six words, explicit values

`State` has six values with explicit constants (no `iota`): `StateLinked`, `StateCopied`, `StateAbsent`,
`StateChanged`, `StateDangling`, `StateStale`; `Label()` yields the six lowercase words; the five retired
identifiers are gone.

### Requirement 2: the link classifier

| Observation | Word | Repair |
| --- | --- | --- |
| no entry at the target | `absent` | `writ deploy` (until #924 makes it reconcile's) |
| an entry that is not a symlink, or a symlink to a different endpoint than the record's source | `changed` | `writ deploy` |
| a symlink to the record's source whose referent does not resolve | `dangling` | `writ deploy` |
| resolves, and the referent's content digest differs from the recorded source digest | `stale` | `writ upgrade` |
| resolves, content as recorded | `linked` | — |

The source is never stat'ed to decide a word other than through the link itself. **Found in Phase 2:** the record
holds no source digest for a link -- a link's source is a path slot, not a cataloged resource, so the ledger never
records it -- and so a link cannot read `stale` today; an edit under a link is in the checkout and is git's
business (consistent with #928: links are never upgrade's). The `stale` branch stays in the link classifier and
fires only when a record carries a source digest for a link; the test asserts `linked` for an edited referent.
Recording a link's source digest would be a ledger change, outside this lane; the owner rules whether it is wanted.

### Requirement 3: the copy classifier

| Observation | Word | Repair |
| --- | --- | --- |
| no entry at the target | `absent` | `writ deploy` |
| target unreadable, or its digest differs from the recorded target digest | `changed` | `writ upgrade --force` |
| target as recorded; the source the record names does not resolve | `dangling` | `writ deploy` |
| target as recorded; the source resolves and its digest differs from the recorded source digest | `stale` | `writ upgrade` |
| target as recorded, source as recorded | `copied` | — |

The encrypted chain compares the encrypted source's digest, as today; the fresh-render comparison goes, because
the recorded digests decide every case.

### Requirement 4: the words everywhere

`commands.go`'s Long lists the six; `5.1-reconciliation.md` gains the table under "The lifetime in the store";
`10-command-line-interface.md` §3.1's `reconcile` row names them; both status files note the revision. The
integration tests assert the six words; the journey scenario's log line says `dangling` where it said `orphan`.

## Design

```
  record entry ──▶ target on disk?
                     no  ──▶ absent
                     yes ──▶ link: symlink to the recorded source? ── no ──▶ changed
                                    referent resolves?             ── no ──▶ dangling
                                    referent digest == recorded source digest? ── no ──▶ stale
                                                                                 yes ──▶ linked
                             copy: target digest == recorded target digest?    ── no ──▶ changed
                                   source resolves?                            ── no ──▶ dangling
                                   source digest == recorded source digest?    ── no ──▶ stale
                                                                                 yes ──▶ copied
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-23; open question 1 ruled.

### Phase 2: The classifiers

- [x] Requirements 1-3: six explicit states, `classifyLink` and `classifyCopied` deciding from the link and the two
      recorded digests, the render-data path and five imports gone; `TestBuildReport_Classifications` covers absent,
      changed (link and copy), stale, dangling (link and copy); `TestBuildReport_Words` pins the six labels.
- [x] **Found in Phase 4 on the Linux VM:** `stale` never fired on a real deploy. The ledger records a source under
      the path the run read it from -- the pinned layer snapshot -- while the graph's `files` annotation names the
      origin, so readback's join found no source digest. The annotation now carries `read_from` (the path read;
      the origin itself when unpinned) and readback joins the source digest through it, falling back to `source`.
      Two files, `deploy/plan.go` and `readback/readback.go`; the record's meaning is unchanged.

### Phase 3: The words

- [x] Requirement 4: `commands.go`'s Long; `5.1` "Reconcile's words" with the table; §3.1's `reconcile` row; both
      status files; the journey scenario's log line is a log, unchanged.

### Phase 4: Gates, metrics, and the VM sequence

- [x] `make check` exit 0 (`build/e2e/923-make-check.log`); `make test-scenario` exit 0, 8 passes. Coverage: total
      63.1%; `reconcile` 79.4%, `readback` 80.0% -> 81.7%; `classifyEntry` 100%, `classifyCopied` 85.7%,
      `classifyLink` 55.6% (the unreadable-link branches), `fileMetadata` 83.3%. Complexity: `classifyLink` 10,
      `classifyCopied` 8. Size: `reconcile.go` 278 (was 330), `report.go` 144 (was 160).
- [x] Both VMs, snapshotted first (Linux `{69dd90bb...}`, Windows `{6ec006fe...}`); logs `build/e2e/923-e2e-ud24.log`,
      `923-e2e-wd11.log`. `danoble-ud24-1.local`: clean deploy 166 linked + 1 copied; a removed link -> `absent`
      (`writ deploy`); the copy's source edited in the checkout -> `stale` (`writ upgrade`), 165 linked, nothing
      else; the copy edited -> `changed` (`writ upgrade --force`); the copy's source moved aside -> `dangling`
      (`writ deploy`); source back and a plain deploy -> 166 linked + 1 copied, exit 0. `danoble-wd11-3.local`
      (links only; this layer deploys no copy there, so `stale` cannot arise): 71 linked; the link removed ->
      `absent`; a file where the link was -> `changed`; the link restored under `replace` and its source moved
      aside -> `dangling`; source back -> 71 linked, exit 0.

### Phase 5: Closure

- [x] #923's boxes ticked with evidence 2026-09-23; committed on `feature/922-lifetime-fold`; this plan `complete`
      at PR A.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the six labels and nothing else | unit, `reconcile` | a retired word survives |
| 2 | a removed link is `absent`; a replaced one `changed`; a broken one `dangling`; a link whose referent's content moved is `stale` | integration | a link case reads the checkout or the wrong word |
| 3 | a removed copy is `absent`; an edited copy `changed`; a copy whose source moved `stale` | integration | attribution reads the wrong digest |
| 4 | a touched-but-identical file is not `changed` | integration | the etag alone decides |
| 5 | both VMs, the three-drift sequence | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/923-reconcile-words.md` | Create: this plan |
| `cmd/writ/writ/reconcile/report.go`, `reconcile.go`, `reconcile_integration_test.go` | Modify: the six words |
| `cmd/writ/writ/commands.go` | Modify: the Long |
| `cmd/writ/scenario_layer_journey_test.go` | Modify: the log line |
| `docs/architecture/5.1-reconciliation.md`, `10-command-line-interface.md`, both status files | Modify: Requirement 4 |

## Open questions

1. **A copy whose source is gone** -- ruled 2026-09-23: `dangling`. The word's established sense is a reference
   that outlives its referent (a dangling symlink, a dangling pointer); `stale` is a derivative behind an origin that
   still exists. So `dangling` means the source the record names does not resolve, for a link and for a copy alike,
   repair `deploy`; `stale` means the source resolves and its content moved, repair `upgrade`. Reconcile reports a
   dangling entry and leaves it: the fault is the record's relationship to the checkout, and only deploy reads the
   checkout. After that deploy the derived copy remains on disk and no lifetime knows it -- deploy replaces the
   record, not the disk; whether it should is the reconciliation epic's question (#845), not this lane's.

## Related Documents

- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule; this plan is lane 2
- [#923](https://github.com/NobleFactor/devlore-cli/issues/923) -- the task; [#762](https://github.com/NobleFactor/devlore-cli/issues/762) -- the feature
- [922-lifetime-fold.md](922-lifetime-fold.md) -- lane 1, the same branch
- [#924](https://github.com/NobleFactor/devlore-cli/issues/924) -- lane 5 changes what `Repair` names
