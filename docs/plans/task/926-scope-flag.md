---
title: "Lane 7: --scope, multi-valued, replaces the inert --target"
issue: https://github.com/NobleFactor/devlore-cli/issues/926
status: in-progress
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

`--scope` registered on writ's root, a repeatable string slice. Absent: every defined scope. A name that is not a
scope, or a builtin undefined on this platform, is refused with `ExitUsage` (64) naming it and the scopes defined
here. Names match without case. `--target` is removed; `writ deploy --target=Home` is then an unknown-flag usage
error (64) from the shared root.

### Requirement 3: the operations honor the selection

- **deploy:** `CollectLayerSources` walks the selected scopes only; a layer directory for an unselected scope is not
  planned; one for a scope undefined here is skipped with no output.
- **upgrade, decommission:** the record's entries are filtered to the selected scopes before their graphs are built.
- **reconcile:** the report covers the selected scopes' entries only; the exit code reads the same subset.

### Requirement 4: the pages

`10-command-line-interface.md` §4 and the `writ deploy` help say what `--scope` does; the manage-environments guide
shows it.

### Requirement 5: the style gate

Every Go file this lane touches passes `star lint go-style` in the lane's commit.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-10-01; open question 1 ruled.

### Phase 2: The model and the flag

- [ ] Requirements 1 and 2, with unit tests for the table, the resolver, the order and every refusal.

### Phase 3: The operations

- [ ] Requirement 3, with a scenario step: `--scope=Home` deploys Home alone; bare deploy deploys every scope.

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
| 3 | `--scope` refusals: unknown, undefined builtin; `--target` gone | unit | a refusal is missing |
| 4 | deploy, upgrade, reconcile, decommission honor the selection | integration, scenario | an operation reaches an unselected scope |
| 5 | the behavior on a real install | VM | the machine behaves otherwise |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/task/926-scope-flag.md` | Create: this plan |
| `cmd/writ/writ/layer.go`, `layer_test.go`, `root.go`, `root_test.go`, `config.go` | Modify: the model, the flag, the selection |
| `cmd/writ/writ/deploy`, `upgrade`, `reconcile`, `decommission` | Modify: honor the selection |
| scenario tests | Modify: the `--scope` step |
| `docs/architecture/10-command-line-interface.md`, `docs/guides/writ/manage-environments.md` | Modify: the flag |

## Open questions

1. **Ruled 2026-10-01: a builtin's key overrides its root.** "It's an override if you relocate home or system (the
   builtins)." `writ.scopes.Home` and `writ.scopes.System` relocate those scopes, as #762 ruled and lane 6 built. A
   key naming a builtin this platform does not define -- `ProgramFiles` on Unix -- introduces nothing and is refused
   (78). The issue's acceptance line ("a `writ.scopes` entry named `System` is a configuration error") is amended to
   that.

2. **Found 2026-10-01, before any lane 7 code, ruled 2026-10-02: lane 6's `writ.targets` refusal blocks the commands
   that would fix
   it.** The refusal runs in writ's root pre-run, so it fires for every command. With `writ.targets` set,
   `writ config list` and `writ config unset writ.targets` both exit 78 -- the user cannot use writ to remove the key
   writ refuses. Lane 7's new refusal (a `writ.scopes` key naming a builtin undefined here) would inherit the same
   flaw. Proposed: both checks run for the lifecycle commands only -- `deploy`, `upgrade`, `reconcile`,
   `decommission` and `adopt` -- and never for `config`, `self`, `version` or `help`. The fix is to lane 6's commit
   on this branch, unmerged, so it lands with PR C. **Ruled 2026-10-02: "fix it the way you proposed."** Both
   refusals run for the lifecycle commands only; `config unset writ.targets` then works like the removal of any
   other unknown key, as `git config --unset` does. Fixed 2026-10-02 in its own commit on this branch:
   `lifecycleCommands` in `root.go`, and `TestRoot_ConfigRunsWithWritTargets`; lane 6's plan carries the amendment.

## Related Documents

- [#926](https://github.com/NobleFactor/devlore-cli/issues/926) -- the issue and its ruling
- [925-scope-not-target.md](925-scope-not-target.md) -- lane 6, the rename this builds on
- [762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) -- Phase 4, and Requirement 8 on `writ.scopes`
- [2.4-hermeticity-guarantees.md](../../architecture/2.4-hermeticity-guarantees.md) -- what a scope is
