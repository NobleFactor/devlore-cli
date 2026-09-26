---
title: "Lane 9: bare writ deploy deploys the implicit set; the argument requirement goes"
issue: https://github.com/NobleFactor/devlore-cli/issues/843
status: complete
created: 2026-09-24
updated: 2026-09-24
---

# Plan: Lane 9 of the writ lifecycle schedule

## Summary

Lane 9 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), pulled ahead of lane 5 into PR B on
2026-09-24 and taken on lane 4's branch. Ruled: no `writ deploy` runs anywhere until the bare form works, because
`writ deploy` with no project refuses ("requires at least 1 arg(s)") though `common` is implicit and "common should
always deploy" (the owner, 2026-09-21). The argument requirement goes; bare `writ deploy` deploys the implicit set.

The issue's premise that "the config path handles zero projects" is not true as written: `withCommonProject` returns
an empty selection unchanged, so dropping the arity check alone would plan nothing. The zero case has to become
`common`.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to
the owner for placement.

## Issue 843

Bug, epic WritDeployment, feature #463 (thread WritDeploy); waits on nothing. On `fix/756-reconcile-exit-codes`
after lane 4's commit.

## Goals

- [ ] `writ deploy` with no project deploys `common`, on every scope the deploy runs today, and its narration says
      the set was implicit.
- [ ] The command's `Use`, help and examples, the deploy design text and the user guide say the bare form is the
      ordinary one.
- [ ] The layer journey runs the bare form where it can, and every test that runs `writ deploy` still passes.

## Current State

Read 2026-09-24 at `d54626d2` plus lane 4.

| Component | Status | Notes |
| --- | --- | --- |
| `commands.go` deploy `Args: cobra.MinimumNArgs(1)` (`:48`) | ❌ | the refusal |
| `commands.go` deploy `Use: "deploy [flags] <project>..."` (`:28`) | ❌ | says a project is required |
| `config.go` `withCommonProject` (`:37`) | ❌ | zero projects returns zero; its doc says emptiness "already means every project", which no caller relies on: deploy is the only caller and refuses zero |
| `common_project_test.go` "empty means every project, unchanged" (`:20`) | ❌ | pins the wrong zero case |
| `upgrade [<project>...]` (`:165`) | ✅ | no arity check; zero projects already means all copied files |
| `decommission` `MinimumNArgs(1)` (`:131`) | ✅ | stays: destruction is explicit (the same doc comment) |
| `scenario_layer_journey_test.go` probes (`:291`) | ⚠️ | `bareDeploy` is probed by the refusal string, and `implicitByName = bareDeploy` (`:295`): once the bare form ships, the journey assumes #850's repository-named projects too; see open question 1 |
| `docs/guides/writ/manage-environments.md` "Deploy projects" | ⚠️ | rewritten today; says "to deploy every project, name them" |
| `docs/architecture/10-command-line-interface.md` §3.1 deploy row | ⚠️ | does not say what bare deploy means |

## Requirements

### Requirement 1: the bare form

The arity check on `deploy` goes. `withCommonProject(nil)` returns `["common"]`; the doc comment says the zero case
is the implicit set, `common` today and the repository-named projects with #850. The deploy narration that names
the projects says `(implicit)` when the command line named none. `decommission` keeps its check.

### Requirement 2: the surfaces

`Use: "deploy [flags] [<project>...]"`, the `Long` opening sentence and an example `writ deploy    # common, the
implicit set`; the §3.1 deploy row in `10-command-line-interface.md` says bare deploy is the implicit set and points
at #850 for the rest; the guide's "Deploy projects" section opens with the bare form.

### Requirement 3: the tests

1. `TestWithCommonProject`'s zero case becomes `["common"]`.
2. `scenario_integration_test.go`: a bare `writ deploy` in the sandbox deploys the `common` files and nothing named,
   and `reconcile` reports them `linked`.
3. The layer journey's probe: see open question 1.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered (2026-09-26). Open question 1 ruled 2026-09-24: lane 10 joins PR B (its plan: [850-implicit-projects.md](../task/850-implicit-projects.md)).

### Phase 2: The code and its tests

- [x] Requirements 1 and 3, 2026-09-26. Lane 9 alone left the layer journey red: its probe reads the refusal's absence as the ruled form, so lanes 9 and 10 land in one commit, and `make check` and `make test-scenario` are green with both.

### Phase 3: The surfaces

- [x] Requirement 2, 2026-09-26, with lane 10's wording: the bare form is the implicit set and what the record holds.

### Phase 4: Both virtual machines

Snapshot first; install the built writ by the remote smoke-test procedure. On each: bare `writ deploy --allow-dirty
--conflict=replace` (the ruled restore on wd11, and the first bare deploy anywhere), then `writ reconcile -o none`
exits 0 and the record holds `common`. Output kept under `build/e2e/843-e2e-<host>.log`.

- [x] `danoble-ud24-1.local` (linux/arm64), 2026-09-26: `Projects: common, noblefactor-ops, devlore-cli, Personal (implicit)`, 172 files, reconcile 0, the record common 70 + noblefactor-ops 102; `nosuchproject` refused at 64; a plain bare redeploy accepted everything as writ's own. Log `build/e2e/850-e2e-ud24.log`.
- [x] `danoble-wd11-3.local` (windows/arm64), 2026-09-26: the same, 76 files; this was the ruled restore, and the nine noblefactor-ops links lane 4 found foreign are in the record (noblefactor-ops 9). Log `build/e2e/850-e2e-wd11.log`.

### Phase 5: Closure

- [ ] The commit on this branch with lane 10; PR B (`Closes #756`, `Closes #843`, `Closes #850`) with coverage, size and
      complexity; the schedule and thread reports.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | zero projects is `common` | unit, `writ` | the zero case is unchanged |
| 2 | bare deploy deploys common | scenario | the arity check stays or the plan is empty |
| 3 | the journey's bare steps run | scenario, layer journey | the probe or the shim disagree with the binary |
| 4 | bare deploy on a real install | both VMs | the machine behaves otherwise |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/843-bare-deploy.md` | Create: this plan |
| `cmd/writ/writ/commands.go` | Modify: the deploy command's arity, `Use`, help, examples |
| `cmd/writ/writ/config.go` | Modify: `withCommonProject`'s zero case |
| `cmd/writ/writ/common_project_test.go` | Modify: the zero case |
| `cmd/writ/writ/deploy/deploy.go` | Modify: the narration says `(implicit)` |
| `cmd/writ/scenario_integration_test.go` | Modify: the bare deploy step |
| `cmd/writ/scenario_layer_journey_test.go` | Unchanged here: lane 10 retires the shim |
| `docs/architecture/10-command-line-interface.md` | Modify: §3.1 deploy row |
| `docs/guides/writ/manage-environments.md` | Modify: "Deploy projects" |

## Open questions

1. **Ruled 2026-09-24: lane 10 joins PR B.** The journey's probe treats the bare form and the repository-named
   implicit projects as one change (`implicitByName = bareDeploy`), and the owner's need is a bare deploy that
   converges the machine, which is the implicit set and not `common` alone. Lane 9 lands the bare form with the zero
   case `common`; lane 10, on the same branch, widens the zero case to the implicit set and the journey runs the
   ruled form.

## Related Documents

- [#843](https://github.com/NobleFactor/devlore-cli/issues/843) -- the bug and the 2026-09-23 ruling
- [#850](https://github.com/NobleFactor/devlore-cli/issues/850) -- lane 10, the implicit set
- [#916](https://github.com/NobleFactor/devlore-cli/issues/916) -- the schedule, amended 2026-09-24
- [756-reconcile-exit-codes.md](756-reconcile-exit-codes.md) -- lane 4, the first commit on this branch
