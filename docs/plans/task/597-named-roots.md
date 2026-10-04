---
title: "Lane 33: named roots -- one fsroot.Binding for each tree an activity touches"
issue: https://github.com/NobleFactor/devlore-cli/issues/597
status: approved
created: 2026-10-02
updated: 2026-10-03
---

# Plan: Lane 33 of the writ lifecycle schedule

## Summary

Lane 33 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), a design lane in PR C, after lane 7
([#926](https://github.com/NobleFactor/devlore-cli/issues/926), complete). #597 asks for a runtime environment that
holds a named set of roots, with `(root-name, rel)` identity. Lane 7 showed why it cannot wait. Every run is confined
to one root, so the root must widen to the common ancestor of every tree the run touches: `$HOME`, or `/` when a layer
lives elsewhere, with nothing at all across two Windows drives. The owner ruled the model on 2026-10-02 (§ Rulings).
This lane produces the design document and the issues that build it. **It changes no code.** Lane 34
([#521](https://github.com/NobleFactor/devlore-cli/issues/521), elevation) follows it, since the two meet at System's
root.

## Issue 597

Task, epic ResourceModel, feature [#546](https://github.com/NobleFactor/devlore-cli/issues/546) ("Paths and URIs:
neutral identity, native access"). On #916 as lane 33, PR C, after lane 7. The design also answers
[#571](https://github.com/NobleFactor/devlore-cli/issues/571), star's session-root anchor, whose candidate 4 is this
design. #571 is lane 44 on #916, done in this lane, and PR C closes it with #597 (amended 2026-10-03).

## Goals

1. **One model for every program.** writ, lore, star and devlore-test each bind one `fsroot.Binding` for each tree an
   activity touches, and each tree is confined to itself.
2. **No run widens its confinement.** The widening to a common ancestor is retired: `fsroot.CommonAncestor` and the
   runs that use it.
3. **Identity that relocates.** A location is `(root name, rel)`, so a record made under one account or machine binds
   on another.
4. **star works in its immediate runtime.** A script's paths bind when they are created, with no pre-flight pass.
5. **A build that is planned, not discovered.** The design states its costs and the order of the work, and the issues
   are filed from it.

## Current State

Read 2026-10-02 at `e2a75b2c`.

| Component | Status | Notes |
| --- | --- | --- |
| `op.RuntimeEnvironment` | one root | `rootPath` and a lazily opened `fsroot.Dir`; `Root()` asserts one exists. 79 call sites read it, by a grep of `pkg` and `cmd` outside tests and generated code; the code's own comment says about 75 |
| `op.RootBinder` | one root | the pre-flight seam: each root-relative resource re-binds to `environment.Root()` (`pkg/op/graph_executor.go:1284`) |
| `fsroot.Path` | ready | the triad of [4.4](../../architecture/4.4-root-path-triad.md): rel, root, abs; a path already carries its root |
| plan space | reserved | `@name/...` is refused today, reserved for this design (`pkg/op/provider/file/planspace.go`) |
| writ deploy, upgrade, adopt, layer registration | widened | one root per run, `fsroot.CommonAncestor` of the scope's target root and every source: usually `$HOME`, `/` when a layer lives elsewhere; two Windows drives are refused |
| writ readback, `workflow verify` | volume root | `WithRoot(string(filepath.Separator))`, which on Windows is drive-relative |
| lore | working directory | `cmd/lore/lore/commands.go` anchors at `wd`; a package plans against its cache ([2.4](../../architecture/2.4-hermeticity-guarantees.md) §Lore Package Scope) |
| star | working directory | `WithRoot(wd)` in `cmd/star/extension/application.go`, #571's interim |
| devlore's own configuration, state and store | dedicated roots | `cli.OpenTree` at the XDG anchors (`cmd/internal/cli/tree.go`) |
| elevation | stub | `pkg/op/provider/elevator` answers not implemented; lane 34 |

## Rulings

All by the owner, 2026-10-02, in the design discussion that opened this lane.

1. **Binding is the model.** "a run opens one fsroot for each tree it touches: the scope, the layer, and the lore
   package declare which named roots they use and in what role." A scope is three trees, its target and one per
   layer, "and could potentially grow from there"; lore's package is a different thing from writ's scope; and "the
   collection of such things, including targets, is represented by an fsroot bound to some activity."
2. **The type is `fsroot.Binding`.** It lives beside `op.Binding`, a slot's value: "it's the most clearcut use of the
   term we have."
3. **The activity is the lifecycle operation**, one per writ scope as today, and it applies to each `fsroot.Binding`
   in the role it declares. Lore's activity is one package install, across its package, system and config trees.
4. **The role is declared where a root is used, not fixed on the root.** Deploy reads `personal:home`; adopt writes
   it.
5. **The root travels with the data.** Every location a provider touches arrives bound to its `fsroot.Binding`; the
   pre-flight seam generalizes from the one root to the binding a location names; `RuntimeEnvironment.Root()` goes.
   "I don't see that B or C can work."
6. **star works in its immediate runtime.** A path binds when it is created: `@name/rel` to that binding, a relative
   path to `work`, an absolute path to the deepest binding that holds it. **An absolute path no binding holds is
   refused.**
7. **star's default bindings** are `work` (the working directory), `repo` (the enclosing repository, unbound outside
   one), `config`, `data`, `state`, `cache` and `scratch` (the session's private temp folder). There is no `home`
   default. **The default names are reserved.** A script adds roots through a `roots:` section in `extension.yaml`
   (name, path, role), the main way; through `star.roots` in configuration; or for one run with `--root name=path`.
8. **The vocabulary.**
   - A writ target takes its scope's lower-case name: `home`, `system`, `programdata`, a custom `staging`.
   - A layer tree is `<layer>:<scope>`: `personal:home`.
   - Lore names its own tree `package` and its targets with the shared names.
   - Plan space spells a location `@name/rel`.

   A name says which tree, not which directory: `personal:home` binds to a pinned snapshot when deploy runs and to the
   live repository when adopt runs.
9. **The XDG trees are reserved `fsroot.Binding`s in every program:** `config`, `data`, `state`, `cache` and `bin`.
   `bin` comes from `XDG_BIN_HOME`, the de-facto extension `pkg/xdg` already honors, else `~/.local/bin`. Each name
   is its variable without `XDG_` and `_HOME`.

## Requirements

The design document must specify each of these. The rulings are fixed; the open questions are the document's to
answer.

### Requirement 1: the model

`fsroot.Binding`: what it holds (the name, the directory, the opened root) and who makes one. The activity, and the
set of bindings it holds. How a writ scope, a layer, a lore package and a star script declare the roots they use and
their roles. One graph per writ scope's activity; one per lore install.

### Requirement 2: identity and the record

Locations as `(root name, rel)`: resources, catalog keys, URIs and serialization. Receipts record root names, so
compensation binds the same trees. Records written before die, by the greenfield rule; the document says so and
states what the change costs (#597's residual costs).

### Requirement 3: providers

Binding at pre-flight for graphs, by name, through the generalized `op.RootBinder`; binding at creation for star's
immediate runtime, by ruling 6's rules, one function for both. The removal of `RuntimeEnvironment.Root()` and the
migration of its call sites. Operations that cross roots: a move between two roots (adopt), and a link whose target
lies in another root (deploy).

### Requirement 4: the programs

- **writ:** targets, layer trees, a snapshot for deploy against the live repository for adopt, and the retirement of
  the widening and of `fsroot.CommonAncestor`.
- **lore:** `package` and its targets.
- **star:** the defaults, the `extension.yaml` schema for `roots:`, `star.roots`, `--root`, and the refusal.
- **devlore-test:** its anchor, as a binding.
- **Everywhere:** the XDG bindings, with `cli.OpenTree`'s roots becoming them, and the binding of a directory that
  does not exist yet (a fresh install's XDG trees).

### Requirement 5: where elevation attaches

The interface lane 34 must meet: elevation attaches to a binding, `system` first, not to a whole run.

### Requirement 6: the build

The features and tasks that build the design, under #546 or a feature of their own, in build order, each naming what
it retires. #571 closes with the design.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed with the owner and approved: "I approve lane 33" (2026-10-03).
- [x] The design document begun with it, 2026-10-02: `4.6-named-roots.md` and its status page hold the rulings,
  the index lists them, and 2.4, 4.4 and 4.5 point to them. The owner: "Please also ensure the design docs track
  this work as we progress, starting now." From here on each ruling enters 4.6 the day it is made, not in phase 3.

### Phase 2: Research

Prior art first, so that the design answers it rather than rediscovering it. Each finding that confirms or would
change a ruling is stated, and a change goes to the owner before the design takes it.

- [ ] Capability-based directories: WASI's preopened directories, Capsicum, and Landlock.
- [ ] Namespaces and mounts: Plan 9's `bind`, Fuchsia's namespaces, and OCI bind mounts.
- [ ] Build sandboxes: Bazel's execroot and sandbox, and Nix store paths.
- [ ] The primitives under fsroot: Go's `os.Root` and openat2's `RESOLVE_BENEATH` and `RESOLVE_IN_ROOT`.

### Phase 3: The design document, complete

- [ ] `docs/architecture/4.6-named-roots.md` and its status page, begun in phase 1: Requirements 1 to 5, the
  research's findings, and the open questions answered or put to the owner.
- [ ] The documents that describe one root today, beyond phase 1's pointers: 2.4 (§Confinement and Source
  Reachability), 4.4 (identity gains a root name), 4.5 (two roots), and 10-command-line-interface.md (star's
  `--root`).

### Phase 4: The build issues

- [ ] Requirement 6: each issue filed with its epic, feature and kind, and placed on a schedule as the owner rules.

### Phase 5: Closure

- [ ] The lane's commits on this branch. PR C opens after lane 8, with `Closes #597` and `Closes #571`.

## Migration Path

None in this lane, which changes no code. The design states the build's: records written before named roots die,
by the greenfield rule, and a deploy without `--scope` writes the first record under the new identity.

## Test Plan

A design lane is verified by review against its rulings and its requirements.

| # | What it proves | How | Fails when |
| --- | --- | --- | --- |
| 1 | every ruling is in the design, unchanged | review, ruling by ruling | a ruling is missing or reworded into something else |
| 2 | the model works end to end | worked walk-throughs: writ deploy and adopt, a lore install, a star script | a walk-through touches a tree it has no binding for |
| 3 | the identity change is costed | review against #597's residual costs | a cost is unstated |
| 4 | the build is complete | every requirement maps to a filed issue | a requirement has no issue |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/597-named-roots.md` | Create | this plan |
| `docs/architecture/4.6-named-roots.md`, `4.6-named-roots.status.md` | Create | the design, begun with this plan and kept current |
| `docs/architecture/index.md` | Modify | the design's entry |
| `docs/architecture/2.4-hermeticity-guarantees.md`, `4.4-root-path-triad.md`, `4.5-fsroot-variants.md` | Modify | pointers now; their one-root text in phase 3 |
| `docs/architecture/10-command-line-interface.md` | Modify | star's `--root`, in phase 3 |

## Open Questions

1. **Identity's costs.** #597 lists them: the URI grammar, catalog keys, and serialization. Its precedent keeps the
   schema at 1 and lets old documents die. Confirm.
2. **Undo across activities.** A receipt records its root names, but the activity that undoes may differ from the one
   that did: decommission undoes deploy's links. How does it bind the same trees?
3. **Operations that cross roots.** A move between two roots, as adopt does, and a link whose target lies in another
   root, as deploy makes.
4. **Where elevation attaches.** Presumably to one binding, `system`; lane 34 designs the mechanism.
5. **A binding whose directory does not exist yet.** A fresh install's XDG trees; 4.5's answer is to create through a
   parent root, then open.
6. **`repo` without git.** The rule that finds the enclosing repository: walking up for `.git`, a directory or, in a
   worktree, a file.
7. **Found writing this plan.** writ's readback and `workflow verify` anchor at `string(filepath.Separator)`, which on
   Windows is drive-relative. The design retires it; whether to fix it sooner is the owner's call.

## Related Documents

- [#597](https://github.com/NobleFactor/devlore-cli/issues/597) -- the issue, its scenario and candidates
- [#546](https://github.com/NobleFactor/devlore-cli/issues/546) -- the feature: neutral identity, native access
- [#571](https://github.com/NobleFactor/devlore-cli/issues/571) -- star's session-root anchor, answered here
- [#563](https://github.com/NobleFactor/devlore-cli/issues/563) -- declared module subsets, the same capability shape
- [#521](https://github.com/NobleFactor/devlore-cli/issues/521) -- lane 34, elevation, which follows this lane
- [#685](https://github.com/NobleFactor/devlore-cli/issues/685) -- fsroot's vocabulary
- [926-scope-flag.md](926-scope-flag.md) -- lane 7, which found the widening's limits
- [2.4-hermeticity-guarantees.md](../../architecture/2.4-hermeticity-guarantees.md),
  [4.4-root-path-triad.md](../../architecture/4.4-root-path-triad.md),
  [4.5-fsroot-variants.md](../../architecture/4.5-fsroot-variants.md),
  [2.1-typed-slots.md](../../architecture/2.1-typed-slots.md) -- one root today, and `op.Binding`
