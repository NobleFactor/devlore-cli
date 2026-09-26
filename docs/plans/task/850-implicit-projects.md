---
title: "Lane 10: the implicit projects are common and one per configured layer repository"
issue: https://github.com/NobleFactor/devlore-cli/issues/850
status: complete
created: 2026-09-25
updated: 2026-09-26
---

# Plan: Lane 10 of the writ lifecycle schedule

## Summary

Lane 10 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), taken into PR B with lane 9 on
2026-09-24, on lane 4's branch. The rule, ruled 2026-09-06 on #463 and restated on #850: a deploy's selection is
`common`, plus one project per configured layer repository named for the repository, plus every project named on
the command line. A repository-named project may live in any layer's `Home` and deploys with its suffix chain like
every other project. Lane 9 lands the bare form with `common` as the zero case; this lane widens the zero case to
the implicit set, and the layer journey retires its shim and runs the ruled form.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to
the owner for placement.

## Issue 850

Task, epic WritDeployment, feature #463 (thread WritDeploy); waits on lane 9 (#843), which lands first on this
branch.

## Goals

- [ ] Bare `writ deploy` deploys `common`, the repository-named project of every registered layer, and every
      project the current record already holds; `writ deploy <name>` adds `<name>` from every layer that carries
      it; `writ upgrade` selects the same way; `decommission` stays explicit.
- [ ] An unknown named project is an error naming it; an implicit project no layer carries deploys nothing and is
      not an error.
- [ ] The deploy narrates the selection: which projects were implicit, which recorded, which named.
- [ ] The layer journey's shim is gone; the pages state the rule.

## Current State

Read 2026-09-25 at `de1046e3` (lane 4 committed; lane 9 pending on the same branch).

| Component | Status | Notes |
| --- | --- | --- |
| `config.go` `withCommonProject` (`:37`) | ⚠️ | injects `common` alone; lane 9 makes it the zero case. Called by `parseDeployConfig` and `parseUpgradeConfig`, never by decommission |
| `repo_cmd.go` `repoRegistration(ctx, layer)` (`:645`) | ✅ | one registration per role with `Root`, `Source`, `State`; the repository's name is `filepath.Base(Root)`, which is the clone's name per #793 for a URL registration and the directory's name for a path one |
| `segment.MatchDirectories` (`matcher.go:15`) | ⚠️ | matches `<project>[.<suffix>]` directories under a source root; a project no layer carries matches nothing and errors nowhere, so "unknown name is an error" has to be decided before the tree is built |
| `deploy.go` narration (`:250`) | ⚠️ | narrates files deployed per scope; nothing says which projects were selected or why |
| `readback.Fold` | ✅ | the record; a project is selected while it has entries (#850, ruled 2026-09-26); not-found when never deployed |
| `scenario_layer_journey_test.go` `deploy`/`implicitToday` (`:319`, `:336`) | ⚠️ | the shim: names `common` plus each registered repository's name where some layer's Home carries it |
| the journey's step 2.6 (`:1065`) | ⚠️ | skipped until `bareDeploy`: "deploy adds a named project to the machine's selection and a later bare deploy keeps it" |
| `docs/guides/writ/repositories.md:136`, `platform-awareness.md:151` | ⚠️ | state the implicit `common`; nothing about repository-named projects |
| `10-command-line-interface.md` §3.1 deploy row | ⚠️ | lane 9 says the bare form is the implicit set; this lane says what the set is |
| #849, the design record | open, unscheduled | asks for the rule in one place; this lane writes the rule on the pages that exist and says so on #849 |

## Requirements

### Requirement 1: the selection

One function in `cmd/writ/writ`, `resolveSelection(ctx, named []string) (Selection, error)`, replaces
`withCommonProject` for deploy and upgrade:

- `Implicit`: `common`, then for each registered role in layer order the repository's name, `filepath.Base` of the
  registration's `Root`, deduplicated.
- `Recorded`: every project with an entry in the current record (`readback.Fold`), minus the implicit ones;
  not-found means none. **Ruled 2026-09-26: the bare form is `writ deploy common`, and it remembers everything it
  deployed.** The record is the selection: a project stays selected while the record holds a file of it, so a bare
  deploy after `writ deploy thenobles` keeps thenobles wherever thenobles put a file. No selection list of its own.
- `Named`: the command line, minus anything already selected. A name no registered layer carries, in any suffix
  form, is refused: `unknown project "x": no registered layer has Home/x or Home/x.<suffix>`, exit 64.
- `Projects()`: implicit, recorded, named, in that order, which is what the tree builds.

`decommission` keeps its explicit list. `upgrade` selects the same way; its copied inventory is filtered by the
selection as today.

### Requirement 2: the narration

Before the plan is built, deploy narrates the selection on one line per kind:
`Projects: common, noblefactor-ops, devlore-cli, personal (implicit); thenobles (recorded); noblefamily (named)`,
omitting an empty kind. `--dry-run` narrates the same.

### Requirement 3: the tests

1. A unit test for `resolveSelection` over a temporary layers directory: implicit from the registrations, recorded
   from a seeded deployment's receipts, named added and deduplicated, unknown refused with the name in the message.
2. The integration test #850 asks for, in `scenario_integration_test.go` or a fixture of its own: three fixture
   layers, `personal/Home/devlore-cli` present, `common` in all three, `noblefamily` in personal alone; a bare
   deploy links the first two and not the third; `deploy noblefamily` links all three; a later bare deploy keeps
   `noblefamily`; the narration names each kind.
3. The layer journey: `deploy` runs the ruled form (named projects only); `implicitToday` and the shim go; the
   probe keeps `bareDeploy` from the refusal and drops `implicitByName` as a separate capability. Step 2.6 runs.

### Requirement 4: the pages

- `10-command-line-interface.md` §3.1: the deploy row says the implicit set is `common` plus one project per
  configured layer repository plus what the record holds.
- `docs/guides/writ/repositories.md` and `platform-awareness.md`: the rule and the four-layer example from #850.
- `docs/guides/writ/manage-environments.md` "Deploy projects": the bare form first, then naming a project adds it.
- #849 gets a comment naming where the rule now lives.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered; open question 1 ruled 2026-09-26.

### Phase 2: The code and its tests

- [x] Requirements 1, 2, 3, 2026-09-26: `selection.go` (`resolveSelection`, `Selection.Projects`, `Selection.Narration`) and its unit tests; deploy and upgrade select through it; the journey's shim and `implicitToday` are gone and step 2.6 runs. The #850 integration test is the layer journey itself: its fixture has `personal/Home/devlore-cli`, `common` in all three layers and `thenobles` in personal alone, and its steps 1.x, 2.1, 2.5 and 2.6 are the four assertions the issue lists. `make check` and `make test-scenario` green.

### Phase 3: The pages

- [x] Requirement 4, 2026-09-26: the §3.1 deploy row, the three guides; #849's comment goes with the PR.

### Phase 4: Both virtual machines

With lane 9's run, one bare `writ deploy --allow-dirty --conflict=replace` per machine: the narration names the
implicit set (`common`, `noblefactor-ops`, `devlore-cli`, `personal`), `reconcile` exits 0, and the record holds
every implicit project the layers carry. On wd11 this is the ruled restore. Output kept under
`build/e2e/850-e2e-<host>.log`.

- [x] `danoble-ud24-1.local` (linux/arm64), 2026-09-26: see [843-bare-deploy.md](../fix/843-bare-deploy.md) phase 4; the implicit set resolved from the registrations, the path-registered personal layer contributing its directory's name, `Personal`.
- [x] `danoble-wd11-3.local` (windows/arm64), 2026-09-26: the same; the ruled restore.

### Phase 5: Closure

- [ ] The commit on this branch with lane 9; PR B (`Closes #756`, `Closes #843`, `Closes #850`).

**Found on both VMs, for placement:** the personal layer is registered by path at `~/Workspace/Personal`, so its
implicit project is `Personal`, the directory's name per the rule, and a `Home/personal` project would not be implicit
on a case-sensitive filesystem. The rule stands as ruled; whether the repository's name should come from the remote
(`personal`) rather than the directory is the owner's to place.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the selection is implicit, recorded, named, and an unknown name is refused | unit, `writ` | any kind is miscomputed |
| 2 | a bare deploy links the implicit set and a named project is added and kept | scenario | the set or its persistence is wrong |
| 3 | the journey runs the ruled form end to end, step 2.6 included | scenario, layer journey | the binary and the ruling disagree |
| 4 | a bare deploy converges a real machine | both VMs | the registrations resolve to the wrong names |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/task/850-implicit-projects.md` | Create: this plan |
| `cmd/writ/writ/config.go` | Modify: `resolveSelection` replaces `withCommonProject` |
| `cmd/writ/writ/common_project_test.go` | Replace: the selection's unit test |
| `cmd/writ/writ/deploy/deploy.go` | Modify: the selection narration |
| `cmd/writ/scenario_integration_test.go` and its fixture | Modify: the #850 integration test |
| `cmd/writ/scenario_layer_journey_test.go` | Modify: the shim goes |
| `docs/architecture/10-command-line-interface.md`, `docs/guides/writ/{repositories,platform-awareness,manage-environments}.md` | Modify: the rule |

## Open questions

1. **Ruled 2026-09-26: the record is the selection.** The bare form is `writ deploy common`, and it remembers
   everything it deployed; a project that put no file on this machine has nothing to be remembered by, and is not.
   No selection record beside the receipts.

## Related Documents

- [#850](https://github.com/NobleFactor/devlore-cli/issues/850) -- the rule, the acceptance list, the 2026-09-23 ruling
- [#843](https://github.com/NobleFactor/devlore-cli/issues/843), [843-bare-deploy.md](../fix/843-bare-deploy.md) -- lane 9
- [#849](https://github.com/NobleFactor/devlore-cli/issues/849) -- the design record, unscheduled
- [#793](https://github.com/NobleFactor/devlore-cli/issues/793) -- a clone is named the way git clone names it
- [implicit-common-project.md](../implicit-common-project.md) -- the 2026-08-09 plan this widens
