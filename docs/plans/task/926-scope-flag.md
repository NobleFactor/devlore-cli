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

**This plan covers this one lane.** Anything found while working it stops the work and goes to the owner.

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

### Requirement 4: the pages

`10-command-line-interface.md` §4 and the `writ deploy` help say what `--scope` does; the manage-environments guide
shows it.

### Requirement 5: the style gate

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

### Phase 4: The pages and the gate

- [ ] Requirements 4 and 5; `make check` and `make test-scenario` green.

### Phase 5: The VM

Snapshot first. On `danoble-ud24-1.local`: `writ deploy --scope=Home`, then a bare deploy; `--scope=ProgramFiles`
refused; `--target=Home` refused at 64. The Windows proof is CI's; the Windows VM is gone.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 6: Closure

- [ ] The lane's commit on this branch. PR C opens after lane 8, with `Closes #926`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the builtin table and per-platform definition | unit | a scope is defined where it is not |
| 2 | custom scopes and order | unit | a custom scope is missed or misordered |
| 3 | `--scope` refusals: unknown, undefined builtin; `--target` gone; `--scope` on the four lifecycle commands only | unit | a refusal is missing, or the flag is on another command |
| 3a | `workflow verify --scope` matches the recorded scope without case | unit | `Home` selects nothing `home` selects |
| 4 | deploy, upgrade, reconcile, decommission honor the selection | integration, scenario | an operation reaches an unselected scope |
| 4a | a scoped deploy keeps every other scope in the record; a bare deploy replaces it all; a failed scope keeps its runs | unit, scenario | `--scope Home` drops another scope's entries, or a bare deploy carries a stale run |
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
| `docs/architecture/10-command-line-interface.md`, `docs/guides/writ/manage-environments.md` | Modify: the flag |

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

6. **Found 2026-10-02 answering the owner's question on adopt; open, proposed for this lane.** Two gaps between
   adopt and the scope model, both lane 7's to close as proposed:
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

## Related Documents

- [#926](https://github.com/NobleFactor/devlore-cli/issues/926) -- the issue and its ruling
- [925-scope-not-target.md](925-scope-not-target.md) -- lane 6, the rename this builds on
- [762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) -- Phase 4, and Requirement 8 on `writ.scopes`
- [2.4-hermeticity-guarantees.md](../../architecture/2.4-hermeticity-guarantees.md) -- what a scope is
