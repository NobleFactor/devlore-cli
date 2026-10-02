---
title: "Lane 7: --scope, multi-valued, replaces the inert --target"
issue: https://github.com/NobleFactor/devlore-cli/issues/926
status: chartered
created: 2026-10-01
updated: 2026-10-02
---

# Plan: Lane 7 of the writ lifecycle schedule

## Summary

Lane 7 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), the second lane of PR C, on the branch lane 6
committed to (`04c7228b`). Ruled on #762 (2026-08-31) and on this issue (2026-09-23): **`--scope`, repeatable and
writ-only, names which scopes a run covers; absent, every scope defined on this platform.** `--target` is registered
on writ's root and read by nothing; it goes, with no alias. Scopes are the builtins -- `Home` and `System` everywhere,
and on Windows `ProgramData`, `ProgramFiles` and `ProgramFilesX86` -- plus the custom scopes `writ.scopes` names.

**This plan covers this one lane.** Anything found while working it stops the work and goes to the owner. One such
finding, open question 6, brought adopt into the scope model in a phase of this lane, and that phase closes #761,
lane 32 of #916.

## Issue 926

Task, epic WritDeployment, feature #762; waits on lane 6 (#925), committed on this branch.

## Goals

1. `writ --scope=<name>`, repeatable, selects the scopes a run covers; absent, every scope defined on this platform,
   in scope order; `--target` is gone.
2. Scopes are a model, not two hard-coded entries: each has a name, a root resolved per platform, and whether this
   platform defines it. Custom scopes come from `writ.scopes`.
3. `deploy`, `upgrade`, `reconcile` and `decommission` honor the selection.
4. Data is skipped, instructions are refused: a layer directory for a scope undefined here is skipped in silence;
   `--scope` naming one is refused.
5. Every touched Go file passes `star lint go-style`.

## Current State

Read 2026-10-01 at `04c7228b`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/writ/writ/root.go` `--target` | ❌ | a persistent string flag, default `Home`, read by nothing |
| `cmd/writ/writ/layer.go` `ScopeOrder()` | ❌ | two hard-coded scopes, System then Home; no Windows builtins, no custom scopes |
| `ScopeSystem()` | ⚠️ | `/` on every platform; on Windows that is drive-relative. Step 58 ([#392](https://github.com/NobleFactor/devlore-cli/issues/392)) is chartered and open |
| `writ.scopes` | ⚠️ | lane 6 reads `writ.scopes.Home` and `.System` as root overrides; nothing reads custom names |
| deploy | ⚠️ | walks `ScopeOrder()` in every layer; one graph per scope; the record carries the scope name lower-cased (`home`) |
| upgrade, decommission | ⚠️ | group the record's entries by scope, one graph each; no selection |
| reconcile | ⚠️ | classifies every entry; no selection |
| `workflow verify --scope` | ⚠️ | the shared root's own flag, one recorded scope matched exactly: `--scope home` finds home's documents, `--scope Home` finds none |
| elevation | -- | not implemented anywhere in writ; out of this lane, as the acceptance list does not ask for it |
| the Windows VM | -- | gone (2026-10-01); CI's Windows jobs are the Windows proof |

## Requirements

### Requirement 1: the scope model

`layer.go` gains the builtin table and the resolver:

| Builtin | Defined on | Root |
| --- | --- | --- |
| `Home` | every platform | the user's home directory |
| `System` | every platform | `/` on Unix; `%SystemDrive%\` on Windows (the step-58 fix, taken here since the table needs it) |
| `ProgramData` | Windows | `%ProgramData%` |
| `ProgramFiles` | Windows | `%ProgramFiles%` |
| `ProgramFilesX86` | Windows | `%ProgramFiles(x86)%` |

Custom scopes are every `writ.scopes` key that is not a builtin name, each with its configured root. Builtin names
are reserved on every platform. Order: the builtins in the table's order, `System` first as today, then the custom
scopes alphabetically. See open question 1 for a `writ.scopes` key that names a builtin.

### Requirement 2: the flag

`--scope` registered on `deploy`, `upgrade`, `reconcile` and `decommission` (ruled 2026-10-02, open question 3), a
repeatable string slice read from the command's own flag: a choice for one run, not a setting, so there is no
`writ.scope` key and no environment variable. Absent: every defined scope. A name that is not a scope, or a builtin
undefined on this platform, is refused with `ExitUsage` (64) naming it and the scopes defined here. Names match
without case. `--target` is removed; `writ deploy --target=Home` is then an unknown-flag usage error (64) from the
shared root.

`workflow verify --scope` keeps its own flag and matches the recorded scope without case, so `--scope Home` selects
what `--scope home` does. It filters records and refuses nothing: the store keeps the runs of scopes no longer
defined.

### Requirement 3: the operations honor the selection

- **deploy:** `CollectLayerSources` walks the selected scopes only; a layer directory for an unselected scope is not
  planned; one for a scope undefined here is skipped with no output.
- **upgrade, decommission:** the record's entries are filtered to the named scopes before their graphs are built.
  With no `--scope`, every entry (ruled 2026-10-02, open question 4).
- **reconcile:** the report covers the named scopes' entries only, and the exit code reads the same subset. With no
  `--scope`, the whole record.
- **order:** deploy's graphs, upgrade's regenerations and decommission's removals run in scope order, the model's,
  in place of deploy's and decommission's hard-coded `{"system": 0, "home": 1}` maps and upgrade's alphabetical sort,
  which ran home before system: the defined scopes, then any scope the record holds that is no longer defined, by
  name, then unscoped entries. One comparator in `readback`, which all four operations import, gives the order.
- **the record** (ruled 2026-10-02, open question 5): every run a lifetime records carries its scope. A deploy's new
  lifetime carries forward, by reference, the current lifetime's runs for the scopes the deploy was not asked to
  run -- none without `--scope` -- and a scope it ran and failed keeps its previous runs. So
  `writ deploy --scope Home` replaces Home's part of the record and keeps every other scope's.

### Requirement 4: adopt and the scope model

Ruled 2026-10-02, open question 6:

- **inference:** each item's scope is the one, of the scopes this platform defines, whose root is the deepest that
  holds the item. A tie goes to the scope that comes first in scope order. An item under no scope's root is refused
  as a missing item is: an error naming it, and the rest adopt.
- **the directory:** the item lands in its scope's directory in the layer: a builtin's own name (`Home/`, `System/`,
  `ProgramData/`), or a custom scope's existing directory, found without case as deploy finds it, else the name
  `writ.scopes` gives it.
- **the root:** the item's path is taken relative to its scope's root, from the model, in place of the `/` adopt
  assumes for System today (#761).
- **the record:** the run records its scope in lower case, as deploy's runs do, so the `--scope` filters and the
  lifetime's carry-forward see adopted entries. Records already written with `Home` or `System` need nothing: the
  next bare deploy replaces them.
- **order:** the scope groups run in scope order, as deploy's graphs do, in place of sorted roots: `RunBatches`
  walks `Config.Scopes`, which `ScopeOrder()` orders. Adopt produces only scopes defined here, so it needs none of
  `readback.CompareScopes`' places for the others.
- **the structure:** `adopt` cannot import `writ`, which imports it, so the cobra layer hands adopt the scopes:
  `adopt.Config` gains `Scopes []adopt.Scope`, each the scope's lower-case name, its directory in the layer, and its
  root, built from `ScopeOrder()` by `adoptScopes` in `adopt_cmd.go`. The inference (`inferScope`) reads them;
  `Config.TargetRoot` stays the Home root that relative items resolve against. Batches are keyed by scope name.
- **the run's root** (ruled 2026-10-02, open questions 7 and 8): each scope's run is confined to the deepest directory
  that holds both the scope's root and the layer, since a run writes into the layer; the record's `target_root` stays
  the scope's root. An item whose scope's root shares no directory with the layer, a layer on another Windows drive,
  is refused as an item under no root is. The directory comes from `fsroot.CommonAncestor`, new: lexical, keeping a
  volume's root whole (`C:\`), blind to case on Windows, and answering none across volumes. It replaces
  `deploy.CommonAncestor` and `migrate.commonAncestor`, so deploy's, upgrade's and layer registration's run roots
  stop answering `C:` for a drive's root and `\` across drives; each refuses when its paths share no directory.
- **the help:** `writ adopt`'s help and `inferScope`'s doc comment say what the model does; `%SystemRoot%` goes.

### Requirement 5: the pages

`10-command-line-interface.md` §4 and the `writ deploy` help say what `--scope` does, and the help of `upgrade`,
`reconcile` and `decommission` shows it once each (agreed 2026-10-02); the manage-environments guide shows it, and
says how adopt picks a scope. §4.1 takes the lane's rulings: the flag on the four lifecycle commands (open question
3), absent reading the whole record for all but deploy (4), a builtin's key relocating it (1), a scoped deploy
replacing only its scopes (5), and adopt's inference (6, 7).

### Requirement 6: the style gate

Every Go file this lane touches passes `star lint go-style` in the lane's commit.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-10-01; open question 1 ruled.

### Phase 2: The model and the flag

- [x] Requirements 1 and 2, with unit tests for the table, the resolver, the order and every refusal; `--scope` on the
  four lifecycle commands and `workflow verify --scope` matching without case, as open question 3 ruled.

### Phase 3: The operations

- [x] Requirement 3, with a scenario step: `--scope=Home` deploys Home alone; bare deploy deploys every scope.
  `TestWritDeployScenario_Scopes` deploys a custom scope, `Staging`, beside Home: `--scope Home` deploys Home
  alone, a bare deploy adds Staging, and a later `--scope Home` keeps Staging in the record (open question 5).

### Phase 4: Adopt and the scope model

- [x] Requirement 4, with unit tests for the inference (the deepest root wins, a tie goes to scope order, an item
  under no root is refused, a relocated Home, a custom scope) and a step in `TestWritDeployScenario_Scopes`: after
  its deploys, `writ adopt` of a file under the sandbox's Home and one under Staging's root lands each in its scope's
  directory in the layer, and `writ reconcile --scope Home` and `--scope Staging` each report the adoption.
  `TestAdopt_LeavesADeploymentRecord` asserts the entry and the run name `home`. `fsroot.CommonAncestor`'s tests
  cover a shared tree, a volume's root, and, on Windows, case and two drives; `make vet-all` compiles them for every
  platform. Closes #761 with PR C.

### Phase 5: The pages and the gate

- [x] Requirements 5 and 6; `make check` and `make test-scenario` green. The flag's usage line had named every
  defined scope as the default on all four commands; each now states its own, the others' being the whole record.

### Phase 6: The VM

Snapshot first. On `danoble-ud24-1.local`: `writ deploy --scope=Home`, then a bare deploy; `--scope=ProgramFiles`
refused; `--target=Home` refused at 64; `writ adopt` of a file under `/etc` lands in `System/`, and
`writ reconcile --scope System` reports it. The Windows proof is CI's; the Windows VM is gone.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 7: Closure

- [ ] The lane's commit on this branch. PR C opens after lane 8, with `Closes #926` and `Closes #761`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the builtin table and per-platform definition | unit | a scope is defined where it is not |
| 2 | custom scopes and order | unit | a custom scope is missed or misordered |
| 3 | `--scope` refusals: unknown, undefined builtin; `--target` gone; `--scope` on the four lifecycle commands only | unit | a refusal is missing, or the flag is on another command |
| 3a | `workflow verify --scope` matches the recorded scope without case | unit | `Home` selects nothing `home` selects |
| 4 | deploy, upgrade, reconcile, decommission honor the selection | integration, scenario | an operation reaches an unselected scope |
| 4a | a scoped deploy keeps every other scope in the record; a bare deploy replaces it all; a failed scope keeps its runs | unit, scenario | `--scope Home` drops another scope's entries, or a bare deploy carries a stale run |
| 4b | adopt infers each item's scope and root from the model and records the scope in lower case | unit, scenario | an item lands in the wrong scope or beneath the wrong root, or `reconcile --scope Home` misses an adopted entry |
| 5 | the behavior on a real install | VM | the machine behaves otherwise |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/task/926-scope-flag.md` | Create: this plan |
| `cmd/writ/writ/layer.go`, `layer_test.go`, `root.go`, `root_test.go`, `config.go` | Modify: the model, the flag, the selection |
| `cmd/writ/writ/commands.go` | Modify: `deploy`, `upgrade`, `reconcile` and `decommission` register `--scope` |
| `cmd/internal/cli/workflow.go`, `workflow_test.go` | Modify: `workflow verify --scope` matches without case |
| `cmd/internal/cli/lifetime.go`, `lifetime_test.go` | Modify: runs record their scope; a deploy's lifetime carries forward the scopes it does not run |
| `cmd/writ/writ/readback/readback.go`, the `deploy`, `upgrade`, `decommission`, `reconcile` and `adopt` packages | Modify: the scope filter, the scope order, and the scope each run records |
| `docs/architecture/5.1-reconciliation.md`, `docs/architecture/10-command-line-interface.md`, `docs/plans/feature/847-kept-snapshots.md` | Modify: the generation model, as ruled |
| `cmd/writ/writ/deploy`, `upgrade`, `reconcile`, `decommission` | Modify: honor the selection |
| scenario tests | Modify: the `--scope` step |
| `cmd/writ/scenario_integration_test.go`, `cmd/writ/scenario_layer_journey_test.go`, `cmd/writ/testdata/personal-repo/Staging/` | Modify, Create: the `Staging` leg; the sandbox's scope roots; §8's widths and §9's helpers-first in the two files touched |
| `cmd/writ/writ/adopt/adopt.go`, `batch.go`, `plan.go`, `adopt_integration_test.go`, `cmd/writ/writ/adopt_cmd.go`, `cmd/writ/scenario_integration_test.go` | Modify: adopt infers, roots and records its scopes from the model; the help and the package doc say so; the scenario's adopt step |
| `cmd/writ/writ/adopt/batch_test.go`, `batch_windows_test.go` | Create: the inference's unit tests; a layer on another drive |
| `pkg/fsroot/fsroot.go`, `commonancestor_test.go`, `commonancestor_unix_test.go`, `commonancestor_windows_test.go` | Modify, Create: `CommonAncestor` and its tests, in files of their own: `fsroot_test.go` carries 33 style findings this lane does not take on |
| `cmd/writ/writ/deploy/plan.go`, `cmd/writ/writ/upgrade/upgrade.go`, `cmd/writ/writ/migrate/register.go` | Modify: the run roots come from `fsroot.CommonAncestor`; the two copies go |
| `cmd/writ/writ/migrate/register_unix_test.go` | Delete: its one test moves to `fsroot` with the function it tested |
| `docs/architecture/10-command-line-interface.md`, `docs/guides/writ/manage-environments.md` | Modify: the flag, and how adopt picks a scope |
| `docs/architecture/2.4-hermeticity-guarantees.md` | Modify: a builtin's name in `writ.scopes` relocates it (open question 1), where the page still refused it |
| `cmd/writ/writ/commands.go`, `layer.go` | Modify: the help of `deploy`, `upgrade`, `reconcile` and `decommission` shows `--scope`, each with its own default |

## Open questions

1. **Ruled 2026-10-01: a builtin's key overrides its root.** "It's an override if you relocate home or system (the
   builtins)." `writ.scopes.Home` and `writ.scopes.System` relocate those scopes, as #762 ruled and lane 6 built. A
   key naming a builtin this platform does not define -- `ProgramFiles` on Unix -- introduces nothing and is refused
   (78). The issue's acceptance line ("a `writ.scopes` entry named `System` is a configuration error") is amended to
   that.

2. **Found 2026-10-01, before any lane 7 code, ruled 2026-10-02: lane 6's `writ.targets` refusal blocks the commands
   that would fix it.** The refusal runs in writ's root pre-run, so it fires for every command. With `writ.targets` set,
   `writ config list` and `writ config unset writ.targets` both exit 78 -- the user cannot use writ to remove the key
   writ refuses. Lane 7's new refusal (a `writ.scopes` key naming a builtin undefined here) would inherit the same
   flaw. Proposed: both checks run for the lifecycle commands only -- `deploy`, `upgrade`, `reconcile`,
   `decommission` and `adopt` -- and never for `config`, `self`, `version` or `help`. The fix is to lane 6's commit
   on this branch, unmerged, so it lands with PR C. **Ruled 2026-10-02: "fix it the way you proposed."** Both
   refusals run for the lifecycle commands only; `config unset writ.targets` then works like the removal of any
   other unknown key, as `git config --unset` does. Fixed 2026-10-02 in its own commit on this branch:
   `lifecycleCommands` in `root.go`, and `TestRoot_ConfigRunsWithWritTargets`; lane 6's plan carries the amendment.

3. **Found 2026-10-02 in phase 2, ruled the same day: (a).** A `--scope` on writ's root collides with the shared
   root's `workflow verify --scope`, one recorded scope's definitions and traces: the root's checker refuses a
   command that redefines a flag it inherits, since cobra would let the local one win silently
   (`TestRoot_KeepsTheOutputConvention`). The two name one thing, the scope a lifecycle operation ran in, which its
   definition and traces record. Offered: (a) `--scope` on the four lifecycle commands, and `workflow verify --scope`
   matching without case; (b) the root's flag, with the shared `workflow verify` reading it on writ alone; (c) a
   rename of one. **Ruled (a).** The word `workflow` itself changes in lane 30 (#1017), when the schedule reaches it.

4. **Found 2026-10-02 planning phase 3, ruled the same day: (a).** Read literally, "absent, every scope defined on
   this platform" would filter the record too: with no `--scope`, `reconcile`, `upgrade` and `decommission` would
   lose the entries of a custom scope since removed from `writ.scopes`, and the unscoped entries of single-source
   mode. Offered: (a) with no `--scope` they read the whole record, and `--scope` only narrows; (b) the literal
   reading. **Ruled (a)**: the record is the reference. Deploy is unaffected: with no `--scope` it deploys every
   scope defined here.

5. **Found 2026-10-02 in phase 3, ruled the same day: lifetimes are generations.** Deploy replaces the record
   (#913): each invocation mints a new lifetime (`cli.NewLifetime`, `deploy.go:139`), and the record is the current
   lifetime's runs alone. So `writ deploy --scope Home` would leave a lifetime holding Home's runs only, and every
   other scope's entries would drop out of the record while their files stay deployed: `reconcile` would stop
   seeing them, `decommission` could not reach them, and the next deploy's pre-flight, reading the Home-only record,
   would find their links foreign. The owner framed it as two alternatives -- decommission before re-deploying, or
   carry forward what is not re-deployed -- and asked how package managers see it: only GNU Stow, which keeps no
   record, re-deploys by tearing down; dpkg, rpm, Homebrew and Nix treat a re-install as a delta against their
   record. **Ruled: writ follows git and Nix** (`5.1-reconciliation.md` § Lifetimes are generations):
   - a lifetime references its runs and replicates none, a complete set like a commit's tree, not a delta on its
     parent;
   - each run records its scope, and, for each layer, the commit it deployed from (#847's pins, per run);
   - a scoped deploy replaces only its scopes;
   - decommission with nothing named removes everything; `--scope` and projects narrow it.

   Ruled the same day, walking a first install: adopt needs no deployment, and requires a registered repository;
   both are #1018, lane 31, which carries adopt's part of the model since lane 14 (#931) had closed.

   **What lane 7 builds of it** (Requirement 3, the record): every run records its scope; a deploy's new lifetime
   carries forward, by reference, the current lifetime's runs for the scopes it was not asked to deploy -- none when
   no `--scope` is given, so a bare deploy still replaces everything, including the runs of lifetimes written before
   runs recorded a scope -- and a scope the deploy ran and failed keeps its previous runs. The rest is amended into
   lanes 11, 12, 13, 16, 17 and 31 when the schedule reaches them.

6. **Found 2026-10-02 answering the owner's question on adopt, ruled the same day: (a).** Two gaps between adopt
   and the scope model:
   - adopt records its scope capitalized, `Home` or `System` (`inferScope`, `cmd/writ/writ/adopt/batch.go`), where
     deploy records `home` and `system`. With phase 3's filters, `reconcile --scope Home` misses adopted entries,
     `decommission --scope Home` leaves adopted links, and a `--scope Home` deploy carries adopt's old Home run
     forward. Proposed: every run records its scope in lower case.
   - #931's item 4 handed this lane adopt's scope inference over the platform's scope set ("a Windows item under
     `%ProgramData%` infers `ProgramData`"); this plan did not list it. Adopt still infers Home or System alone,
     and takes System's root to be `/` itself (`collectItem`) rather than from the model. On Windows, `inferScope`'s
     doc comment and `writ adopt`'s help (`adopt_cmd.go`) say System is `%SystemRoot%`; the code makes everything
     outside the home directory System. Proposed: adopt infers
     the scope whose root is the deepest that holds the item, over `ScopeOrder()`, and takes that scope's root
     from the model. #761 is this gap on Windows; lane 7's phase 2 fixed its other half (System is
     `%SystemDrive%\`).

   Offered: (a) both in this lane, as a phase before the pages, with #761 joining PR C; (b) the case fix here and
   the inference in lane 31 (#1018); (c) both in lane 31. **Ruled (a)**: Requirement 4 and phase 4. #761 joins PR C as
   lane 32 of #916, and phase 4 closes it.

7. **Found 2026-10-02 running phase 4's scenario step, ruled the same day: (a).** Adopt confines each run to its
   scope's root (`buildSpec(scope.Root)`), and a run writes into the layer, so the layer must lie beneath that root.
   A usual install meets that for Home, whose root holds the layer under `$HOME`, and for System, whose root is `/`
   (or `%SystemDrive%\`, with the layer on the system drive), so adopt has never failed on it. Phase 4 sends an item
   to the scope whose root holds it: a custom scope's (`/srv/staging`), a Windows scope's (`C:\ProgramData`), or a
   relocated Home's. The layer is beneath none of those, so the run refuses before it moves a file:
   `…/Personal/Home/adopted lies outside scoped root …/home`. The scenario's sandbox keeps its layer outside its
   Home, which is how the step found it, for Home as well. Offered:
   - (a) anchor each adopt run at the deepest directory that holds both the scope's root and the layer, and refuse
     an item when none does (a layer on another Windows drive); the record's `target_root` stays the scope's root.
     Home and System on a usual install run exactly as before;
   - (b) anchor every adopt run at the root of its scope's volume (`/`, `C:\`);
   - (c) keep the scope's root, and refuse, naming the layer, an item whose scope's root does not hold the layer;
     custom scopes, the Windows scopes and a relocated Home then cannot be adopted into on a usual layout.

   **Ruled (a)**, the tightest confinement an adoption allows: Requirement 4's run-root bullet.

8. **Found 2026-10-02 building open question 7's (a), ruled the same day: (a).** The deepest directory that holds
   two paths is computed twice already, by the same segment-matching code: `deploy.CommonAncestor`, for deploy's
   and upgrade's run roots, and `migrate.commonAncestor`, for layer registration. On Windows it answers wrongly at a
   drive's root. For `C:\` and `C:\Users\…` it answers `C:`, the current directory on drive C rather than its root;
   for paths on two drives it answers `\`, the root of whatever drive the process stands on, where the answer is
   none; and it compares names with their case, which Windows ignores. Deploy and upgrade for System, whose root is
   `%SystemDrive%\`, reach the first; adopt into ProgramData or ProgramFiles would too. Containment is fsroot's
   question (`fsroot.RelWithin`, lexical and volume-aware), and the deepest common directory is the same question.
   Offered:
   - (a) add it to fsroot, volume-aware and blind to case on Windows, answering none across volumes; adopt uses it,
     and it replaces deploy's and migrate's copies in this phase, which closes #761, System on Windows;
   - (b) the same fsroot function for adopt now; the copies' defect filed as a bug for a lane of its own;
   - (c) a helper local to adopt.

   The owner then raised two roots per run, a target root and the source roots, which is #597's named roots.
   **Ruled (a)** for this lane: `fsroot.CommonAncestor` replaces both copies, and adopt uses it. Named roots is
   designed after this lane lands, as lane 33 of #916: "we have been talking about named roots for a while now ... we
   will design that after we land this lane."

## Related Documents

- [#926](https://github.com/NobleFactor/devlore-cli/issues/926) -- the issue and its ruling
- [#761](https://github.com/NobleFactor/devlore-cli/issues/761) -- adopt's half of the System root, closed by phase 4
- [#931](https://github.com/NobleFactor/devlore-cli/issues/931) -- lane 14, whose item 4 handed adopt's inference here
- [#597](https://github.com/NobleFactor/devlore-cli/issues/597) -- named roots, lane 33, designed after this lane
- [925-scope-not-target.md](925-scope-not-target.md) -- lane 6, the rename this builds on
- [762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) -- Phase 4, and Requirement 8 on `writ.scopes`
- [2.4-hermeticity-guarantees.md](../../architecture/2.4-hermeticity-guarantees.md) -- what a scope is
