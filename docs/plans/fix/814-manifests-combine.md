---
title: "Manifests combine; only files collide"
issue: https://github.com/NobleFactor/devlore-cli/issues/814
status: chartered
created: 2026-09-09
updated: 2026-09-09
---

# Plan: Manifests combine; only files collide

## Summary

The tree builder treats `packages-manifest.yaml` as an ordinary file: every layer's and every suffix directory's
manifest sits at the same relative target path, so all but the most specific one is discarded as a collision. A
manifest lands nowhere in the filesystem, and two sets of package claims cannot occupy the same place. This plan
carries every manifest to writ's own merge in contribution order, plans one `pkg.install` per native product into
one packages subgraph per scope, and leaves the registry-package branch to the devlore provider, so the overlay rule
keeps applying to files and stops applying to manifests.

## Issue 814

A high-priority bug under [#463](https://github.com/NobleFactor/devlore-cli/issues/463), found 2026-09-04 on the
Windows VM: the plan held one `pkg.install`, the team layer's seven-package general manifest having lost to its
one-package Windows manifest -- and on the run before, the personal layer's manifest beat both, so the team layer's
packages could not be installed at all. The ruling is recorded on the issue and is not reopened here: **manifests
combine and deduplicate across layers and specificity, never collide, contribution order preserved.** Closed by this
plan's pull request.

## Goals

- [x] Every manifest reaches the merge, in contribution order: base → team → personal, general → specific.
- [x] A package claimed twice is planned once, and its features are the union of what the claims asked for.
- [x] A duplicate claim is narrated as a note naming every manifest that made it.
- [x] A scope's native packages are one subgraph of the scope graph, one `pkg.install` per product.
- [x] A claim that resolves to a registry package is noted and not planned, until the devlore provider plans it.
- [x] Files still collide exactly as they do today.
- [x] The writ documentation states both rules beside each other.

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
| `docs/guides/writ/packages-manifest.md` | ⚠️ **states the union already** | "Packages are deduplicated by name" -- and then contradicts the ruling with "A package in a higher layer overrides the same package from a lower layer, including its features" |
| `docs/guides/writ/platform-awareness.md` | ⚠️ states it too | "For `packages-manifest.yaml` files, variants are **merged** rather than replaced" |

## What the work found

**The guides already stated the union rule; the code never implemented it.** `packages-manifest.md` says packages are
deduplicated by name and `platform-awareness.md` says variants are merged rather than replaced. This was not an
undocumented rule but a documented one the builder contradicted. The same section then contradicted the ruling in the
other direction -- "a higher layer overrides the same package ... including its features" -- so both guides are
corrected here rather than merely extended.

**A version is not something a union combines, and order does not decide it.** Ruled 2026-09-09 on
[#872](https://github.com/NobleFactor/devlore-cli/issues/872): the package manager settles versions, and a merge that
cannot ask it yet resolves by satisfaction. A bare name asks for any version, so a pin satisfies both claims
whichever order they arrive in; two different pins are refused at plan time, naming both manifests, until the broker
can put the question to the manager. Order decides only where a package appears, which is what the checksum needs.

**The registry resolve is not the identity, and lore's manager choice is never read.** `Registry.Resolve` looks for a
registry package directory named by the raw string and otherwise synthesizes a release with the platform's source and
the raw string as its native name, prefix, purl and `@version` included; it parses nothing
([#784](https://github.com/NobleFactor/devlore-cli/issues/784)).
The one parser and router is the pkg provider's `buildCandidate`, which runs when a `packages` string becomes a
resource at plan time, and lore's `Source` is never passed to it. So "identity is the resolved release" named a
capability the code does not have; the identity the merge needs is the one the catalog interns.

**Row 6 of the test plan is met by writ planning the units itself.** With the merge and the loop in writ, a
functional deploy over a fixture tree holds the `pkg.install` units in its scope graph, and needs no stub planner and
no interface.

## Requirements

### Requirement 1: The builder carries every manifest

An entry whose pipeline is `manifest.resolve` bypasses collision resolution in both builders and accumulates in a new
`BuildResult.Manifests`, ordered by layer, then by specificity ascending, then by source path. `Files` no longer
carries manifests; deploy passes `Files` and `Manifests` to `buildScopeGraph` directly, and the filter that once
separated them is gone.

**Why order at all, since a union does not need one.** Install a and b, install b and c, get a, b, c: the merge itself
is order-free, and the sequence of the resulting install units is cosmetic. The sort earns its place once, and
narrowly: a graph's identity is a checksum over its content, and 2.4's guarantee is that identical inputs produce an
identical graph on any machine every time. Claims merged in map order would checksum differently run to run. Any
stable order satisfies this; contribution order is chosen because a reader can predict it.

Files are untouched. The collision rule is right for something that lands at one path.

### Requirement 2: writ merges the union and plans it, one subgraph per scope

Ruled 2026-09-09: writ owns the loop over a scope's merged manifests, and `lore.Planner` leaves writ's path. The
call to `PlanManifests` is replaced by an if block over the merged claims:

    for each claim in the scope's merged manifests:
        release := registry.Resolve(name)
        if release is a registry package:   note it; the devlore provider (#877) will plan devlore.deploy
        else:                               plan pkg.install(claim), one invocation per product

A scope's package invocations form two `flow.subgraph` units of the scope graph, by kind, beside the file chains:
the scope's native packages, `pkg.install` one per product, and the scope's devlore packages, `devlore.deploy` one
per package. An edge orders them native first, so a pipeline can rely on the manager-installed tools of its own
scope; a manager install needs nothing from a pipeline. Within each, contribution order. A subgraph's children are
peers in a topological sort and run independently unless an edge orders them, and nothing in the platform layer
serializes two invocations of one manager. **Ruling needed:** chain the native subgraph's children in contribution
order, one manager invocation at a time, or leave them independent as lore's per-package roots are today.

Until [#877](https://github.com/NobleFactor/devlore-cli/issues/877) lands the devlore subgraph does not exist: a
claim that resolves to a registry package is narrated as a note and the rest of the scope deploys. `lore deploy`
keeps `PlanPackages` for its own single-manifest use until #877 replaces both paths.

Merging, precisely:

- **Identity is what the catalog interns**, by the pkg provider's own grammar: manager type plus name. The parse in
  `buildCandidate` -- purl, then `manager:name`, then a bare name on the platform default, version split off -- is
  extracted into one exported function that `buildCandidate` and the merge both call, so plan-time identity is
  run-time identity. A claim that resolves to a registry package keys as that package's name.
- **Namespace and version are claim content, resolved by satisfaction.** A bare claim is met by a namespaced one on
  the same manager, so `jq` and `pkg:winget/jqlang/jq` are one product; two namespaces on one manager are two
  products, both planned, with a note. **Ruling needed:** the namespace half is proposed and not yet ruled.
- **Features union.** Two claims on one package contribute the union of their `with:` lists rather than the later
  overwriting the earlier. A general manifest asking for a package and a platform manifest asking for it with a
  feature means both, which is what "combined" says.
- **Position is the first claim's**, so the merged list is stable and reads in contribution order.
- **A version is settled by the package manager, not by the merge.** Interim until the broker of
  [#868](https://github.com/NobleFactor/devlore-cli/issues/868) exists, ruled 2026-09-09 on
  [#872](https://github.com/NobleFactor/devlore-cli/issues/872), deltas 1, 2 and 11: a pin satisfies an unpinned
  claim and is kept whichever order the claims arrive in; two different pins are refused at plan time, naming both
  manifests. The broker replaces the refusal by putting the question to the manager, which alone knows whether two
  versions of a package can coexist.

  **Whether a version should be a conflict at all is a design question, filed 2026-09-09 as
  [#872](https://github.com/NobleFactor/devlore-cli/issues/872).** Today's model makes it one: the resource's URI is
  versionless, so two versions are one entry. That is right for apt and winget, right for brew by another route, and
  wrong for nix and npm, where versions coexist -- and it sits oddly beside #813, which made qualifiers identity.
  This plan applies the existing rule and does not decide it.

### Requirement 3: A duplicate claim is narrated as a note naming every manifest that made it

Duplicates are ordinary in this scheme -- a general and a platform manifest naming one package is the design working
-- and they are noteworthy: whether a claim is redundant depends on which layers this machine composes, so the note
reports the fact and does not recommend. The personal layer's `jq` adds nothing on a machine with the team layer and
is load-bearing on one without it. Ruled 2026-09-09 on [#872](https://github.com/NobleFactor/devlore-cli/issues/872),
deltas 4 and 13: every merge decision is narrated, as a note, never a warning or an error. Whether a note reaches
standard output is [#873](https://github.com/NobleFactor/devlore-cli/issues/873)'s subject, the writ lifecycle output
design. The existing collision narration in deploy keeps its wording for files.

### Requirement 4: Both rules, written down

`docs/guides/writ/packages-manifest.md` states the union rule and names the file overlay rule beside it, so a reader
meets the distinction where manifests are described. [#849](https://github.com/NobleFactor/devlore-cli/issues/849)
consolidates both into the design record when it opens; this plan does not wait for it.

## Design

**Why a new `Manifests` field rather than a flag on the entries.** `Files` is sorted by ID, and every manifest shares
one ID, so any ordering among them inside that slice is arbitrary. A separate, explicitly ordered slice is the only
shape in which contribution order can be stated at all.

**Why the merge lives in writ, not in lore.** Ruled 2026-09-09. The manifest reader is `internal/manifest`, shared
already; what lore uniquely held was the registry client and the feature merge, and features are inert for a native
package. writ plans invocations itself for every file chain, and a `pkg.install` is one more.

**Why identity is the catalog's, by the pkg grammar.** Deduplicating raw strings would catch the literal duplicate
line and miss the same package spelled two ways, which is the case #813 made reachable; resolving through the
registry does not make them comparable either ([#784](https://github.com/NobleFactor/devlore-cli/issues/784)).
The catalog interns the versionless purl that `buildCandidate` builds, so the merge keys on the same parse, exported
once.

**Narrated by default, as a note; ruled, not open.** A duplicate is normal and noteworthy, so it is narrated every
time, never gated on `--verbose` and never raised to a warning. The ruling is Requirement 3's; whether the note
reaches standard output is [#873](https://github.com/NobleFactor/devlore-cli/issues/873)'s subject.

**Why lore leaves writ's path, and what waits on the LorePackaging epic.** Asked 2026-09-09: could the pkg work cut
lore out of the path from a manifest to a `pkg.install` unit? It could. What lore does on that path for a native
package, and whether the pkg work needs it:

| # | What lore does on the native path | Needed by the pkg work? |
| --- | --- | --- |
| 1 | Loads the manifest and walks its entries | No; `internal/manifest` is not lore's, and writ can call it |
| 2 | Resolve: lore package or synthetic native | Only for an entry that names a registry package |
| 3 | Synthetic lifecycle: the install phase alone | No; it wraps one `pkg.install` in a phase subgraph |
| 4 | Phase subgraph with package and phase annotations, retry policy | Not by pkg; the trace and receipts read the annotations |
| 5 | Features from `with:` into `BuildConfig` | No; a native package has no scripts to read them |
| 6 | Emits `pkg.install` with the raw strings | The whole of it; the pkg provider parses them |

Ruled 2026-09-09, in two steps. First, to wait, for three costs that belong to the LorePackaging epic
([#446](https://github.com/NobleFactor/devlore-cli/issues/446))
and its features #551, #553 and #784: a registry package's prepare, provision and verify pipelines run only through
lore; the trace and receipts read the package and phase annotations lore's subgraph carries; and `lore deploy` shares
the planner. Then, on reading the stack, to cut: a native package needs none of the wrapper, since a phase around one
manager call adds a name and a checksum and nothing that executes; the reader is shared already; and a registry
package is a different kind of thing, a pipeline with phases the manager knows nothing about, which gets its own
provider ([#877](https://github.com/NobleFactor/devlore-cli/issues/877))
rather than a costume as a manager. So lore leaves writ's path now, the registry-package branch is dormant until
#877, and the position on #446 stands: lore is a tool the pkg provider uses, not a target it routes to.

## Implementation Phases

### Phase 1: The failing tests

- [x] `tree`: a layer with `noblefactor/` and `noblefactor.Windows/` manifests -- both survive, general first.
- [x] `writ deploy`: `jq` and `jq@1.7` resolve to the pin, in either arrival order; `jq@1.6` and `jq@1.7` are refused.
- [x] `tree`: two layers each with a manifest -- both survive, base before team.
- [x] `tree`: a file at one target path in two layers still collides, one survivor.
- [x] `writ deploy`: two manifests naming one package -- one claim; features unioned; order preserved.
- [x] `writ deploy`: `jq`, `brew:jq` and `pkg:brew/jq` are one product on a brew platform; `port:jq` is a second.
- [x] `writ deploy`: a duplicate claim is narrated as a note naming every manifest that made it.
- [x] `writ deploy`: a claim that resolves to a registry package is noted and not planned.

### Phase 2: The builder -- COMPLETE (2026-09-09)

- [x] `BuildResult.Manifests`; both builders route `manifest.resolve` entries to it, sorted.
- [x] `Files` no longer carries manifests; deploy passes `Files` and `Manifests` to `buildScopeGraph` and groups
  manifests by scope in contribution order.

### Phase 3: The merge and the loop in writ

- [x] The parse in `buildCandidate` exported as `platform.ParseIdentifier`, behavior unchanged, the pkg tests green.
- [x] `deploy/manifest.go`: the merge over a scope's manifests, keyed by that parse, resolving through the registry
  client; duplicates returned as records with the manifests that made them.
- [x] The if block: a registry package is noted; a native claim plans `pkg.install`, one invocation per product.
- [x] One `flow.subgraph` per scope for its native packages, a unit of the scope graph beside the file chains; the
  devlore sibling and the edge between them are #877's.
- [x] Duplicates narrated at note level in deploy, beside the collision report.
- [x] `Config.ManifestPlanner` replaced by the registry client; nil still skips with a note.
- [x] `PlanManifests` and the merge removed from `lore.Planner`; `PlanPackages` unchanged.

### Phase 4: Matrix A

- [x] The eight-cell fixture tree, planned through `BuildGraphs`: the scope graph holds the expected `pkg.install`
  units for the host platform, in contribution order, with the notes; CI's three runners cover the three rows.
- [x] The refusal tree, and the manifest-only directory.

### Phase 5: The documentation and closure

- [x] `docs/guides/writ/packages-manifest.md` states both rules.
- [ ] The plan's status; the issue's boxes.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Two specificities in one layer both survive | unit, `tree` | the manifest bypass is removed |
| 2 | Two layers both survive | unit, `tree` | the bypass is layer-blind |
| 3 | Contribution order is general → specific, base → team | unit, `tree` | the explicit sort is dropped and walk order returns |
| 3b | A pin satisfies an unpinned claim, either order | unit, `writ deploy` | satisfaction becomes precedence |
| 3c | Two different pins are refused, naming both manifests | unit, `writ deploy` | a claim is silently dropped |
| 3d | A duplicate claim is narrated as a note naming every manifest that made it | unit, `writ deploy` | the merge is silent |
| 3e | Three spellings of one package are one product; a second manager is a second product | unit, `writ deploy` | the key is the raw string |
| 4 | A file still collides | unit, `tree` | the bypass is too wide |
| 5 | One package claimed twice plans once, features unioned | unit, `writ deploy` | the merge is a last-wins overwrite |
| 6 | A deploy over the Matrix A tree plans the expected units per platform, in order, with the notes | functional, `writ deploy` | the loop or the merge is wrong |
| 8 | A claim that resolves to a registry package is noted and not planned | unit, `writ deploy` | the dormant branch plans something |
| 7 | `writ deploy noblefactor --dry-run` plans the team layer's eight Windows-applicable packages, not one | manual, **the user's** | the fix is incomplete on the real layers |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/tree/builder.go` | Modify | `Manifests`; the bypass in both builders; the sort |
| `cmd/writ/writ/tree/manifest_union_test.go` | Create | rows 1 through 4 |
| `cmd/writ/writ/deploy/plan.go` | Modify | `buildScopeGraph` takes manifests; the loop and the packages subgraph |
| `cmd/writ/writ/deploy/manifest.go` | Create | the merge, the if block, the duplicate records |
| `cmd/writ/writ/deploy/manifest_test.go` | Create | rows 3b through 3e, 5 and 8 |
| `cmd/writ/writ/deploy/report.go` | Modify | the duplicate notes beside the collision report |
| `cmd/writ/writ/deploy/deploy.go` | Modify | `Config`: the registry client replaces `ManifestPlanner` |
| `cmd/writ/writ/commands.go` | Modify | the assignment |
| `pkg/platform/identifier.go`, `identifier_test.go` | Create | `ParseIdentifier`, the one grammar |
| `pkg/op/provider/pkg/resource.go` | Modify | `buildCandidate` calls `ParseIdentifier` |
| `cmd/lore/lore/builder.go` | Modify | `PlanManifests` and the merge removed |
| `cmd/lore/lore/manifest_merge_test.go` | Delete | its rows move to `deploy/manifest_test.go` |
| `cmd/writ/writ/deploy/manifest_matrix_test.go` | Create | row 6: Matrix A, the refusal tree, the manifest-only directory |
| `docs/guides/writ/packages-manifest.md` | Modify | both rules; the two interim sentences (#877, #868) |

## Related Documents

- [#814](https://github.com/NobleFactor/devlore-cli/issues/814) -- this plan
- [#813](https://github.com/NobleFactor/devlore-cli/issues/813) -- merged; the purl form the old rule made the only manifest on that machine
- [#849](https://github.com/NobleFactor/devlore-cli/issues/849) -- the writ design record that will consolidate both rules
- [#812](https://github.com/NobleFactor/devlore-cli/issues/812) -- next in the queue; found on the same test
- [#872](https://github.com/NobleFactor/devlore-cli/issues/872) -- the identity rulings and the fifteen-delta review this plan was corrected by
- [#873](https://github.com/NobleFactor/devlore-cli/issues/873) -- the writ lifecycle output design; where a note is displayed
- [#874](https://github.com/NobleFactor/devlore-cli/issues/874) -- 2.5 names resolve as the first stage and the boundary with the pkg provider
- [#877](https://github.com/NobleFactor/devlore-cli/issues/877) -- the devlore provider; the registry-package branch waits for it
- [#446](https://github.com/NobleFactor/devlore-cli/issues/446) -- the LorePackaging epic and its position: lore is a tool, not a target
- [#784](https://github.com/NobleFactor/devlore-cli/issues/784), [#551](https://github.com/NobleFactor/devlore-cli/issues/551), [#553](https://github.com/NobleFactor/devlore-cli/issues/553) -- resolve, vocabulary, synthesis: the lore-side features
- [devlore-registry#91](https://github.com/NobleFactor/devlore-registry/issues/91) -- the package-mapping index
- [noblefactor-ops#196](https://github.com/NobleFactor/noblefactor-ops/issues/196) -- rule 10, from this review
