---
title: "Lane 14: writ adopt carries the platform and leaves a deployment record"
issue: https://github.com/NobleFactor/devlore-cli/issues/931
status: active
created: 2026-09-23
updated: 2026-09-23
---

# Plan: Lane 14 of the writ lifecycle schedule

## Summary

Lane 14 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), the last of PR A, on the branch lanes 1 to 3
committed to. Ruled 2026-09-23: an adopted file is deployed -- after `writ adopt` the record says so, `reconcile`
says `linked`, and the next `deploy` recognizes the link as writ's own -- and adopt must carry platform
information, so the caller names the platform label and the file lands in the suffixed project directory the
deploy walk reads. Lane 1 already appends adopt's trace to the current lifetime; this lane makes that trace a
deployment record and gives adopt its platform.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 931

Task, epic WritDeployment, feature #762; inserted into PR A by the owner beside lane 1; waits on #922 (`217f52fb`).

## Goals

- [ ] `writ adopt --platform <suffix>` lands the file in `<layer>/<scope>/<project>.<suffix>/<rel>`, and a suffix the
      matcher would never read is a usage error.
- [ ] After an adopt, `reconcile` reports the link `linked` and a bare `deploy` under `stop` recognizes it.

## Current State

Read 2026-09-23 at `cf6fc635`.

| Component | Status | Notes |
| --- | --- | --- |
| `adopt_cmd.go` (`:31`) | ⚠️ | `--layer` (personal, team, base), `--project` (required), `--from-receipt`; no platform |
| `adopt/batch.go` `collectItem` (`:144`) | ⚠️ | destination is `<layer>/<scope>/<project>/<rel>`; the scope is inferred, Home or System |
| `adopt/plan.go` `BuildGraph` (`:59`) | ❌ for the record | one graph per scope: mkdir pre-stage, then a `flow.gather` over the item records whose body is `choose(exists -> failed \| move -> link)` with slots projected from the item. Its origin is `planProvider.Origin("adopt")`: tool `""`, scope `"adopt"`, no annotations |
| `readback.foldRun` (`:264`) | ❌ for adopt | skips a run whose origin tool is not `writ`; reads the `files` annotation keyed by the final invocation's unit id |
| lane 1 (`batch.go` `RunBatches`) | ✅ | requires the current lifetime (66 without one) and appends the trace through `WriteLifetimeTrace` |
| `segment` | ✅ | the suffix vocabulary the tree matches: `OS` (`Darwin`, `Linux`, `Windows`), `Unix` for either Unix, `DISTRO` (`Debian`, ...), `ARCH` (`arm64`, ...); a directory is `<project>.<suffix>[.<suffix>...]` |
| `platform.Token` | — | `Linux.Debian` is lore's vocabulary; writ's directories say `noblefactor-ops.Debian`, so the flag speaks the segment vocabulary, not the token |

## Requirements

### Requirement 1: `--platform <suffix>`

Optional. A dotted sequence of segment values -- `Darwin`, `Unix`, `Windows`, `Linux`, `Linux.Debian`,
`Darwin.arm64` -- validated against the values the matcher knows for the segments this platform declares (`OS`,
`DISTRO`, `ARCH`) plus `Unix`; any other word is a usage error (64) naming the vocabulary. The destination becomes
`<layer>/<scope>/<project>.<suffix>/<rel>`; absent, the neutral project directory as today.

### Requirement 2: the record

Adopt's graph carries the origin deploy's carries: tool `writ`, the scope name, and annotations `target_root` and
`files`, one entry per adopted file with `target` (the original location, now the link), `source` and `read_from`
(the project location), `action` `file.link`, `layer`, `project`. `foldRun` then reads an adopt run as it reads a
deploy run, and `reconcile` reports the link `linked`, pre-flight recognizes it (`AsRecorded`, #883).

The `files` annotation is keyed by the link unit's id and matched to receipts by unit id; a `flow.gather` runs one
body for every item, so one unit id would stand for every file. **Open question 1** decides the graph's shape.

### Requirement 3: scope inference

Unchanged this lane: Home under the home directory, System elsewhere. When #926 lands, inference reads the scope
set of the platform; this lane leaves the seam.

### Requirement 4: the words

The `writ adopt` reference documents `--platform` and the vocabulary; `10-command-line-interface.md`'s adopt entry
and `5.1`'s "The lifetime in the store" say an adoption is a write into the current deployment.

**Found in Phase 3:** a graph holding a `flow.choose` does not load back -- `op.LoadGraph: subgraph ... has no
action_name in the document` -- so adopt's in-graph guard made every adoption's trace a finding instead of a record.
Filed as devlore-cli#939 (bug, Serialization). In this lane the existing-destination guard runs in `RunBatches`
before any graph, the chain is `file.move` then `file.link` with no subgraph, and the graph loads. The in-graph guard
returns with #939 if it is wanted.

**Found in Phase 5 on both VMs:** `writ adopt` had never worked on a real install. A registered layer is a symlink
(`layers/personal -> ~/Workspace/Personal`), adopt planned its destinations through that link, and the confined run
root refused the write: `file.mkdir: mkdirat .local/share/devlore/writ/layers/personal/...: path escapes from
parent`. The fixture used a real directory, so no test met it. `parseAdoptConfig` now resolves the layer path through
its symlink, which is also what the record must name, since links target the origin.

## Design

```
  writ adopt --layer team --project noblefactor-ops --platform Linux.Debian ~/.config/foo
    -> team/Home/noblefactor-ops.Linux.Debian/.config/foo   (moved)
    <- ~/.config/foo                                          (link, as deploy would make it)
    -> trace: origin writ/Home; files{ link unit: target ~/.config/foo, source <project path>,
                                      file.link, team, noblefactor-ops }
    -> lifetimes/current: + adopt run
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-23: one chain per item; the segment vocabulary.

### Phase 2: The platform

- [x] `--platform` on the command; `adopt.ValidatePlatform` against the detected segments' values plus the OS
      family; `Config.ProjectDirectory`; `TestAdopt_Platform`.

### Phase 3: The record

- [x] One chain per item (`file.move` then `file.link`), the origin `writ`/scope with `target_root` and `files`,
      and `RunBatches` writes the graph before the run (it never had; the fold found every adopt trace's graph
      unavailable). `TestAdopt_LeavesADeploymentRecord`: the fold holds the link, `AsRecorded` is true, the
      lifetime's last run is adopt, `reconcile` says `linked`.

### Phase 4: The words

- [x] Requirement 4: the `writ adopt` Long and example; `5.1` "The lifetime in the store"; the status file.

### Phase 5: Gates, metrics, and the VM sequence

- [x] `make check` exit 0 (`build/e2e/931-make-check.log`, after `RunBatches` split under the cognitive gate);
      `make test-scenario` exit 0, 8 passes. Coverage: total 63.1%; `adopt` 77.7% -> 78.9%; `BuildGraph` 87.5%,
      `RunBatches` 88.2%, `runBatch` 79.2%, `ValidatePlatform` 85.7%, `ProjectDirectory` 100%. Complexity:
      `runBatch` 8, `BuildGraph` 7, `RunBatches` 6. Size: `plan.go` 154 (was 165), `batch.go` 420 (was 322).
- [x] Both VMs, snapshotted first (Linux `{ce1703cb...}`, Windows `{543c1bf1...}`); logs `build/e2e/931-e2e-ud24.log`,
      `931-e2e-wd11.log`. On both: `--platform Ubuntu` refused with exit 64 naming the vocabulary (Linux: `Linux, Unix,
      arm64`; Windows: `Windows, arm64`); a fresh file adopted with the platform's own suffix landed under
      `Home/noblefactor-ops.Linux` and `Home/noblefactor-ops.Windows` in the personal checkout, the original now a
      link; the current lifetime's last run is `adopt`; `reconcile` reports the link `linked`, `file.link`, personal,
      noblefactor-ops; a deploy under `stop` (`--allow-dirty`, the adoption having dirtied the checkout) accepted
      the link and ran; the adoption undone and a deploy restored every entry `linked` (Linux 166 + 1 copied,
      Windows 71). Windows' step 0 and 5 ran under `replace`: that VM's personal working tree carries nine files its
      HEAD removed, which #852's dirty-tree judgment owns, not this lane.

### Phase 6: Closure

- [ ] #931's five boxes ticked with evidence 2026-09-23; committed (the commit script is written); **PR A opens**
      closing #922, #923, #883, #931; this plan `complete` at the PR.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `--platform Darwin` lands under `<project>.Darwin`; `--platform Ubuntu` is a usage error | unit, `adopt` | the vocabulary is not enforced |
| 2 | an adopted link is in the record, `linked`, and passes pre-flight | integration | the trace is not a deployment record |
| 3 | adopt with no current lifetime exits 66 | integration | already lane 1's; kept |
| 4 | both VMs, the adopt sequence | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/931-adopt-record.md` | Create: this plan |
| `cmd/writ/writ/adopt_cmd.go` | Modify: `--platform` |
| `cmd/writ/writ/adopt/batch.go`, `plan.go`, tests | Modify: the suffix, the origin and annotation, the graph's shape |
| `cmd/writ/writ/readback/readback.go` | Modify only if open question 1 keeps the gather |
| `docs/architecture/10-command-line-interface.md`, `5.1-reconciliation.md`, status files | Modify: Requirement 4 |

## Open questions

1. **The graph's shape.** The writ-adopt design (2026-07-15) settled on one `flow.gather` over the items with
   projected slots. A deployment record is keyed by unit id, and a gather gives every item the same link unit. Two
   ways out: (a) plan one chain per item, as deploy does -- one `file.move` and one `file.link` unit per file, the
   mkdir pre-stage kept -- so each link has its own unit id and the record needs nothing new; or (b) keep the gather
   and teach readback to match a receipt by the link it produced (the receipt's result resource's path) instead of
   by unit id. I recommend (a): adopt batches are small, the concurrency the gather bought is not worth a second
   join rule in the record, and one shape for "what writ left" is easier to read back. It reverses a settled
   design point, so it is the owner's. **Ruled 2026-09-23: one chain per item.**
2. **The vocabulary.** The flag speaks the segment vocabulary the tree matches (`Darwin`, `Unix`, `Debian`,
   `arm64`, dotted), not `platform.Token`'s `Linux.Debian`. The issue body says the token; the directories say
   otherwise. I recommend the segment vocabulary, since that is what deploys. **Ruled 2026-09-23: the segment
   vocabulary.**

## Related Documents

- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule; this plan is lane 14
- [#931](https://github.com/NobleFactor/devlore-cli/issues/931) -- the task; [#762](https://github.com/NobleFactor/devlore-cli/issues/762) -- the feature
- [writ-adopt-command.md](../extract-starlark-from-op/phase-8/writ-adopt-command.md) -- the settled gather design
- [922-lifetime-fold.md](922-lifetime-fold.md) -- lane 1, where adopt learned to append
