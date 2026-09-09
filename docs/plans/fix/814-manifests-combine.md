---
title: "Manifests combine; only files collide"
issue: https://github.com/NobleFactor/devlore-cli/issues/814
status: draft
created: 2026-09-09
updated: 2026-09-09
---

# Plan: Manifests combine; only files collide

## Summary

The tree builder treats `packages-manifest.yaml` as an ordinary file: every layer's and every suffix directory's
manifest sits at the same relative target path, so all but the most specific one is discarded as a collision. A
manifest lands nowhere in the filesystem, and two sets of package claims cannot occupy the same place. This plan
carries every manifest to the planner in contribution order and merges the claims, so the overlay rule keeps
applying to files and stops applying to manifests.

## Issue 814

A high-priority bug under [#463](https://github.com/NobleFactor/devlore-cli/issues/463), found 2026-09-04 on the
Windows VM: the plan held one `pkg.install`, the team layer's seven-package general manifest having lost to its
one-package Windows manifest -- and on the run before, the personal layer's manifest beat both, so the team layer's
packages could not be installed at all. The ruling is recorded on the issue and is not reopened here: **manifests
combine and deduplicate across layers and specificity, never collide, contribution order preserved.** Closed by this
plan's pull request.

## Goals

- [ ] Every manifest reaches the planner, in contribution order: base → team → personal, general → specific.
- [ ] A package claimed twice is planned once, and its features are the union of what the claims asked for.
- [ ] A duplicate is a note at most, never a collision.
- [ ] Files still collide exactly as they do today.
- [ ] The writ documentation states both rules beside each other.

## Current State

Measured 2026-09-09 in the worktree, against develop at 55a6859e.

| Component | Status | Notes |
| --- | --- | --- |
| `tree/node.go` | ✅ marks them | `packages-manifest.yaml` / `.json` get the `manifest.resolve` pipeline |
| `tree/builder.go` | ❌ the defect | both builders key every entry by target ID and keep one; a manifest's ID is the same in every layer and every suffix directory |
| `segment.MatchDirectories` | ⚠️ reverse order | sorts by specificity **descending**, so the walk yields specific before general and contribution order needs an explicit sort |
| `deploy/plan.go` `splitManifests` | ✅ separates | already filters manifests out of the file entries by their operation |
| `deploy/plan.go` `planManifests` | ⚠️ per manifest | calls `PlanPackages` once per path, so no seam sees the union |
| `lore.Planner.PlanPackages` | ⚠️ single path | loads one manifest, resolves each entry through the registry, builds phases |
| `docs/guides/writ/packages-manifest.md` | ❌ silent | states neither the union rule nor the file collision rule |

## Requirements

### Requirement 1: The builder carries every manifest

An entry whose pipeline is `manifest.resolve` bypasses collision resolution in both builders and accumulates in a new
`BuildResult.Manifests`, ordered by layer, then by specificity ascending, then by source path. `Files` no longer
carries manifests, and `splitManifests` reads `Manifests` instead of filtering.

**Why order at all, since a union does not need one** (asked 2026-09-09; the plan overstated this). Install a and b,
install b and c, get a, b, c: the merge itself is order-free, and the sequence of the resulting install units is
cosmetic. The sort earns its place twice, both narrow:

1. **Determinism.** A graph's identity is a checksum over its content, and 2.4's guarantee is that identical inputs
   produce an identical graph on any machine every time. Claims merged in map order would checksum differently run
   to run. Any stable order satisfies this; contribution order is not special for it.
2. **Claims that disagree on something a union cannot combine.** A package name carries a version, so a general
   manifest may say `jq` where the platform manifest says `jq@1.7`. Features union; a version cannot. The later,
   more specific claim wins -- last-in-wins, the same intuition the file overlay has.

Point 2 is where walk order bites: the matcher returns specific directories **first**, so a naive last-wins over walk
order would hand the win to the general claim, which is backwards. The sort is what makes last-in-wins mean what a
reader expects, not what makes the union work.

Files are untouched. The collision rule is right for something that lands at one path.

### Requirement 2: The planner sees the union, and claims merge

`lore.Planner` gains `PlanManifests(provider, environment, paths []string)`, which loads every manifest in order,
merges the entries, and then runs the loop `PlanPackages` runs today. `PlanPackages` keeps its signature for lore's
own single-manifest use; nothing existing changes shape.

Merging, precisely:

- **Identity is the resolved release**, not the string. A package can be written `jq`, `brew:jq` or `pkg:brew/jq`
  and mean one package; the registry resolve that already happens per entry is what makes them comparable.
- **Features union.** Two claims on one package contribute the union of their `with:` lists rather than the later
  overwriting the earlier. A general manifest asking for a package and a platform manifest asking for it with a
  feature means both, which is what "combined" says.
- **Position is the first claim's**, so the merged list is stable and reads in contribution order.
- **A conflict the union cannot resolve -- a version pin -- goes to the last claim**, which the sort makes the most
  specific one in the highest layer.

  **Whether a version should be a conflict at all is a design question, filed 2026-09-09 as
  [#872](https://github.com/NobleFactor/devlore-cli/issues/872).** Today's model makes it one: the resource's URI is
  versionless, so two versions are one entry. That is right for apt and winget, right for brew by another route, and
  wrong for nix and npm, where versions coexist -- and it sits oddly beside #813, which made qualifiers identity.
  This plan applies the existing rule and does not decide it.

### Requirement 3: A duplicate is a note

Duplicates are ordinary in this scheme -- a general and a platform manifest naming one package is the design working
-- so they are narrated at `Note` level at most, and never as a `Collision`. The existing collision narration in
deploy keeps its wording for files.

### Requirement 4: Both rules, written down

`docs/guides/writ/packages-manifest.md` states the union rule and names the file overlay rule beside it, so a reader
meets the distinction where manifests are described. [#849](https://github.com/NobleFactor/devlore-cli/issues/849)
consolidates both into the design record when it opens; this plan does not wait for it.

## Design

**Why a new `Manifests` field rather than a flag on the entries.** `Files` is sorted by ID, and every manifest shares
one ID, so any ordering among them inside that slice is arbitrary. A separate, explicitly ordered slice is the only
shape in which contribution order can be stated at all.

**Why merging belongs in the planner, not the deploy command.** The deploy side holds paths; the planner holds the
manifest reader, the registry client and the feature merge. Moving any of that into deploy would be a second copy of
the manifest reader.

**Why identity is the resolved release.** Deduplicating raw strings would catch the literal duplicate line and miss
the same package spelled two ways, which is the case #813 just made reachable. The resolve is not extra work: it
already runs once per entry.

**Open question for the user, stated rather than assumed.** Whether a duplicate should be narrated at all by default,
or only under `--verbose`. This plan narrates at `Note`, on the reasoning that a duplicate is normal; if it should be
silent, that is a one-line change.

## Implementation Phases

### Phase 1: The failing tests

- [ ] `tree`: a layer with `noblefactor/` and `noblefactor.Windows/` manifests -- both survive, general first.
- [ ] `lore`: a general claim of `jq` and a platform claim of `jq@1.7` -- the pinned version wins.
- [ ] `tree`: two layers each with a manifest -- both survive, base before team.
- [ ] `tree`: a file at one target path in two layers still collides, one survivor.
- [ ] `lore`: two manifests naming one package -- one claim; features unioned; order preserved.

### Phase 2: The builder

- [ ] `BuildResult.Manifests`; both builders route `manifest.resolve` entries to it, sorted.
- [ ] `splitManifests` reads `Manifests`; `Files` no longer carries them.

### Phase 3: The planner

- [ ] `PlanManifests` over the union, with the merge of Requirement 2.
- [ ] `planManifests` calls it once instead of looping.

### Phase 4: The documentation and closure

- [ ] `docs/guides/writ/packages-manifest.md` states both rules.
- [ ] The plan's status; the issue's boxes.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Two specificities in one layer both survive | unit, `tree` | the manifest bypass is removed |
| 2 | Two layers both survive | unit, `tree` | the bypass is layer-blind |
| 3 | Contribution order is general → specific, base → team | unit, `tree` | the explicit sort is dropped and walk order returns |
| 3b | A version pin in the more specific claim wins | unit, `lore` | last-wins runs over walk order, handing it to the general claim |
| 4 | A file still collides | unit, `tree` | the bypass is too wide |
| 5 | One package claimed twice plans once, features unioned | unit, `lore` | the merge is a last-wins overwrite |
| 6 | A deploy with manifests at two specificities plans every package once | devlore-test | any of the above |
| 7 | `writ deploy noblefactor --dry-run` plans the team layer's eight Windows-applicable packages, not one | manual, **the user's** | the fix is incomplete on the real layers |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/tree/builder.go` | Modify | `Manifests`; the bypass in both builders; the sort |
| `cmd/writ/writ/tree/builder_test.go` | Modify | rows 1 through 4 |
| `cmd/writ/writ/deploy/plan.go` | Modify | `splitManifests`, `planManifests` |
| `cmd/lore/lore/builder.go` | Modify | `PlanManifests` and the merge |
| `cmd/lore/lore/builder_test.go` | Modify | row 5 |
| `cmd/devlore-test/devloretest/` | Create | row 6 and the Go test that names it |
| `docs/guides/writ/packages-manifest.md` | Modify | both rules |

## Related Documents

- [#814](https://github.com/NobleFactor/devlore-cli/issues/814) -- this plan
- [#813](https://github.com/NobleFactor/devlore-cli/issues/813) -- merged; the purl form the old rule made the only manifest on that machine
- [#849](https://github.com/NobleFactor/devlore-cli/issues/849) -- the writ design record that will consolidate both rules
- [#812](https://github.com/NobleFactor/devlore-cli/issues/812) -- next in the queue; found on the same test
