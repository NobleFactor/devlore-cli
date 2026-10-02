---
title: "Lane 6: the word is scope, not target"
issue: https://github.com/NobleFactor/devlore-cli/issues/925
status: complete
created: 2026-10-01
updated: 2026-10-02
---

# Plan: Lane 6 of the writ lifecycle schedule

## Summary

Lane 6 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), the first lane of PR C (lanes 6, 7 and 8 on
one branch). Ruled on #762 (2026-08-31) and on this issue (2026-09-23): **the word is scope, not target.** A scope
is a named execution context, with a name, a target root, a confinement boundary, an elevation posture and its own
graph ([2.4](../../architecture/2.4-hermeticity-guarantees.md)). A target is a path, where one file lands. The code
uses `Target` for both, four lines apart in `cmd/writ/writ/tree/builder.go`.

The rename is mechanical and changes no behavior, with one exception the issue rules: the configuration key moves
from `writ.targets` to `writ.scopes`, and `writ.targets` is refused.

**This plan covers this one lane.** Anything found while working it stops the work and goes to the owner.

## Issue 925

Task, epic WritDeployment, feature #762. Branch `feature/925-scope-not-target`, opened 2026-10-01 from develop at
`01634491`.

## Goals

1. No identifier in `cmd` or `pkg` says `Target` where it means a scope. `TargetRoot`, `Target` and `Entry.Target`
   remain, and each one's doc comment says it is a path.
2. Scope roots are read from `writ.scopes`; a configuration that sets `writ.targets` is refused.
3. Every doc comment, test name, error string and narration line that names a scope says "scope".
4. Every Go file this lane touches is fully compliant with the Go style guide: `star lint go-style` passes on each.

## Current State

Read 2026-10-01 at `01634491`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/writ/writ/layer.go` | ❌ | `TargetSpec`, `TargetHome()`, `TargetSystem()`, `TargetOrder()`; reads `writ.targets.home` and `writ.targets.system` through viper |
| `cmd/writ/writ/tree/builder.go` (`:27`, `:58`) | ❌ | `LayerSource.TargetName` and `FileEntry.TargetName`, "the target scope" |
| other callers | ❌ | `config.go` (4 calls of `TargetHome()`), `deploy/plan.go` (4), `deploy/report.go` (1), `tree/node.go` (1) |
| tests | ❌ | `layer_test.go` (7), `snapshot/snapshot_test.go` (10), `tree/tree_test.go` (3), `deploy/manifest_matrix_test.go` (1); the scenario harness's `writeTargetConfig` writes `writ.targets.home` |
| `schema/devlore-config.json` | ⚠️ | already declares `writ.scopes`, name to root directory, with the builtin names as keys (added by #764, revised by #992); no `writ.targets`; the `writ` object does not close itself to unknown keys |
| `schema/defaults/writ.yaml` | ✅ | already documents `writ.scopes` and the builtin scopes; nothing reads it yet |
| configuration validation | ⚠️ | only `writ config validate` checks the schema; loading never does, so an unknown `writ.targets` key would be silently ignored after the rename |
| `cmd/internal/cli/config.go:623` | ❌ | a comment names `writ.targets` |
| generated files | ✅ | none mention the identifiers |
| design pages | ✅ | none mention the identifiers |
| this Mac's configuration, and the personal and base layers | ✅ | none set `writ.targets` |
| the style debt in the touched files | ❌ | 140 violations today, measured with `star lint go-style`; see open question 1 |

## Requirements

### Requirement 1: the rename

| From | To |
| --- | --- |
| `TargetSpec` | `ScopeSpec` |
| `TargetHome()`, `TargetSystem()` | `ScopeHome()`, `ScopeSystem()` |
| `TargetOrder()` | `ScopeOrder()` |
| `LayerSource.TargetName`, `FileEntry.TargetName` | `ScopeName` |
| `writeTargetConfig` (scenario harness) | `writeScopeConfig` |
| `TargetRoot`, `Target`, `Entry.Target` | unchanged; their doc comments say each is a path |

### Requirement 2: the configuration key

`ScopeHome()` and `ScopeSystem()` read `writ.scopes.home` and `writ.scopes.system`; viper matches keys without case,
so the documented `Home:` and `System:` work as written. A configuration that sets `writ.targets` is refused when writ
loads its configuration, before any command runs, with `ExitConfig` (78): "writ.targets is retired; name scope roots
under writ.scopes". A test proves the refusal. The scenario harness writes `writ.scopes.Home`.

> **Amended 2026-10-02 (#926, open question 2, ruled):** the refusal runs for the lifecycle commands only -- `deploy`,
> `upgrade`, `reconcile`, `decommission` and `adopt`. Found while planning lane 7: run before every command, it also
> stopped `writ config unset writ.targets`, so the key could not be removed with writ.

### Requirement 3: the style gate

Every Go file this lane touches passes `star lint go-style` in the commit that touches it, every violation fixed,
not only the renamed lines. The commit script runs the gate on exactly those files and stops on any finding.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-10-01; open question 1 ruled: "fix them here, as the gate requires."

### Phase 2: The rename and the key

- [x] Requirements 1 and 2, 2026-10-01: 58 renames in 12 files; `writ.scopes.home|system` read; `writ.targets` refused
      at 78 by writ's root pre-run (`refuseRetiredConfiguration`, `TestRoot_RefusesWritTargets`);
      `TestScopeRoots_ReadWritScopes`; the harness writes `writ.scopes.Home`. `make check` and `make test-scenario`
      green.

### Phase 3: The style gate

- [x] Requirement 3, 2026-10-01: 176 findings across 15 files (the 12 surveyed plus `root.go`, `root_test.go` and
      `cmd/internal/cli/config.go`) cleared by a consented fan-out, seven fixers and seven skeptics, comments only,
      proven against saved copies; then by hand: one inaccurate Returns bullet in `tree/builder.go`, old-form bullets
      in `layer.go`, nine test summaries in the colon form section 4 forbids though the linter misses it, and two
      `//nolint` reasons past 120 columns. `star lint go-style` reports all 15 compliant; `make check` and `make
      test-scenario` green.

### Phase 4: Both virtual machines

Snapshot first; install the built writ by the remote smoke-test procedure. On each: check its configuration for
`writ.targets`; then a bare `writ deploy` and `writ upgrade`, with the same results as before the rename. Output under
`build/e2e/925-e2e-<host>.log`.

- [x] `danoble-ud24-1.local` (linux/arm64), 2026-10-01: no `writ.targets` in its configuration; the dry-run plan has
      820 units under the installed writ and under the new one; a configuration setting `writ.targets` refused at 78
      with the message; bare `writ deploy` 164 files, `writ upgrade` and `writ reconcile` all exit 0. Log
      `build/e2e/925-e2e-ud24.log`.
- [x] `danoble-wd11-3.local` (windows/arm64): **skipped, ruled by the owner 2026-10-01** ("we'll skip windows").
      Reachable, no `writ.targets` in its configuration; its snapshot could not be taken with 27 GB free on the host.
      CI's Windows jobs remain the Windows proof.

### Phase 5: Closure

- [x] The lane's commit on this branch, 2026-10-01. PR C opens after lanes 7 and 8, with `Closes #925`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | the identifiers and test names say scope | `make check` | a rename is missed |
| 2 | `writ.scopes.home` is read; `writ.targets` is refused at 78 | unit, `writ` | the key or the refusal is wrong |
| 3 | deploy and upgrade behave as before | scenario, both VMs | the rename changed behavior |
| 4 | every touched file is compliant | `star lint go-style` | a violation remains |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/task/925-scope-not-target.md` | Create: this plan |
| `cmd/writ/writ/layer.go`, `layer_test.go`, `config.go` | Modify: the rename, the key, the refusal |
| `cmd/writ/writ/tree/builder.go`, `node.go`, `tree_test.go` | Modify: `ScopeName` |
| `cmd/writ/writ/deploy/plan.go`, `report.go`, `manifest_matrix_test.go` | Modify: `ScopeName` |
| `cmd/writ/writ/snapshot/snapshot_test.go` | Modify: `ScopeName` |
| `cmd/writ/scenario_integration_test.go`, `scenario_layer_journey_test.go` | Modify: `writeScopeConfig`, `writ.scopes` |
| `cmd/internal/cli/config.go` | Modify: the comment |
| `cmd/writ/writ/root.go`, `root_test.go` | Modify: the refusal and its test |

## Open questions

1. **Ruled 2026-10-01: fixed here, as the gate requires.** The touched files carry 140 style violations today, and the
   Go-style sweep (#991, for #964) is fixing the same
   files.** Measured 2026-10-01: `scenario_layer_journey_test.go` 50, `scenario_integration_test.go` 22, `config.go`
   15, `tree/tree_test.go` 15, `snapshot/snapshot_test.go` 14, `tree/builder.go` 12, and 12 more across five files;
   `deploy/plan.go` and `deploy/report.go` are clean. Most are doc comments missing their Parameters or Returns
   sections. The gate requires all 140 fixed here. #991 edits the same files, so whichever merges second resolves
   conflicts in them. Recommendation: fix them here, as the gate requires; the sweep then has twelve fewer files to
   do, and its conflicts in them are resolved by taking this lane's version.

## Related Documents

- [#925](https://github.com/NobleFactor/devlore-cli/issues/925) -- the issue and its ruling
- [762-lifecycle-scopes.md](../feature/762-lifecycle-scopes.md) -- the feature, Phase 3, the vocabulary sweep
- [2.4-hermeticity-guarantees.md](../../architecture/2.4-hermeticity-guarantees.md) -- what a scope is
