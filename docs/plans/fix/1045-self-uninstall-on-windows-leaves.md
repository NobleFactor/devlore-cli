---
title: "self uninstall on Windows leaves its manifest behind, and the scenario that would have caught it never runs"
issue: https://github.com/NobleFactor/devlore-cli/issues/1045
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: every scenario runs, and an uninstall on Windows leaves no manifest

Lane 31 of #949. One pull request.

## Summary

Two defects, found 2026-10-19 auditing #949, and one fix: the test that proves the second fails until the first is
fixed. `make test-scenario` names four scenario tests and not `TestSelfUninstallFromInstalledCopyScenario` (#1003), so
that scenario has never run. And on Windows, an uninstall run from the installed copy rewrites its manifest to record
the running `.exe`, hands the `.exe`'s deletion to a detached `cmd.exe`, and exits: `cmd.exe` deletes the `.exe`, and
nothing deletes the manifest, which then records a file that is gone.

## Current State

At `83e6b965`.

| What | Where | Today |
| --- | --- | --- |
| The scenario target | `Makefile`, `test-scenario` | four `go test -run` lines: `TestWritDeployScenario`, `TestWritLayerJourneyScenario` (./cmd/writ), `TestSelfInstallScenario`, `TestSelfUpgradeScenario` (./cmd/scenario) |
| The scenarios | `*_test.go` | every test that skips unless `WRIT_SCENARIO_RUN` or `DEVLORE_SCENARIO_RUN` is set; `TestSelfUninstallFromInstalledCopyScenario` (`cmd/scenario/selfuninstall_scenario_test.go:20`) among them, matched by no `-run` |
| The uninstall | `cmd/internal/cli/selfinstall.go`, `runSelfUninstall` | the running image, on Windows, left out of the in-place removal and kept in `remaining`; the manifest rewritten to record `remaining` |
| The deferred deletion | `cmd/internal/cli/selfdelete_windows.go`, `deferRunningImageDeletion` | a detached `cmd.exe` loop deletes the `.exe`, retrying about once a second up to 120 times, and stops |

A recorded file that is already gone counts as removed (`removeRecordedFiles`), so a later run that finds the `.exe`
gone finishes the uninstall.

## Requirements

### Requirement 1: Every scenario runs, and a guard says so

- `make test-scenario` runs `TestSelfUninstallFromInstalledCopyScenario`, on a line of its own like the others.
- A test, `TestScenarioTargetRunsEveryScenario` in `cmd/scenario`, runs under `make test` (it is not a scenario
  itself): it finds every test function in `./cmd/scenario` and `./cmd/writ` that reads `DEVLORE_SCENARIO_RUN` or
  `WRIT_SCENARIO_RUN`, in its own body or through a function of its package it calls (cmd/writ's read it in
  `newScenarioSandbox` and `newJourney`), by parsing their source, and fails for each that no `-run` pattern of the Makefile's
  `test-scenario` recipe, for that package, matches. Failing it before the Makefile line is added proves it.

### Requirement 2: On Windows, the manifest goes when the running image goes

- When the running image is all that remains, the deferred deletion deletes the manifest after the `.exe` is gone,
  then the manifest's directory if empty. The `cmd.exe` loop does it in the same command: delete the `.exe`; once it
  no longer exists, delete the manifest and remove the directory, then stop.
- When the deletion never succeeds (the cap reached), the manifest stays, recording the `.exe`, so a later run can
  finish (#1045's Resolution).
- When other files are kept as well, the manifest stays, recording them and the `.exe`; a later run, finding the `.exe`
  gone, counts it removed. Unchanged.
- Elsewhere nothing changes: the running image is removed in place, and the manifest with it.

### Requirement 3: Proven where it fails

- `TestSelfUninstallFromInstalledCopyScenario` passes on Linux here, under `make test-scenario`.
- On danoble-wd11-3 (windows/arm64), the scenario fails at `83e6b965` ("left its manifest behind") and passes with the
  fix.
- CI's scenario legs, windows-amd64 and windows-arm64 among them, pass.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1045-self-uninstall-on-windows-leaves` and reviewed with the owner: approved
      2026-10-20, each decision ruled on its own (D1: "Yes, the Go test under make test"; D2: "Yes, cmd.exe deletes the
      manifest")

### Phase 2: Every scenario runs (Requirement 1)

- [x] The guard test, failing for `TestSelfUninstallFromInstalledCopyScenario`; then the Makefile line, and it passes.
      Done 2026-10-20: `make test` failed with "make test-scenario does not run
      TestSelfUninstallFromInstalledCopyScenario in ./cmd/scenario" and nothing else, then passed with the line. The
      writ scenarios read their gate in helpers, so the guard follows a test's calls within its package: with the
      Makefile's `TestWritDeployScenario` pattern broken once, it named `TestWritDeployScenario_Deploy` and `_Harness`
- [x] `make test-scenario` here: the uninstall scenario runs and passes on Linux, for the first time (lore, star, writ;
      2026-10-20)

### Phase 3: The manifest on Windows (Requirement 2)

- [x] `deferRunningImageDeletion` takes the manifest to delete when the image is all that remains. `runSelfUninstall`
      removes every other recorded file first, so what the image leaves behind is known when its deletion is scheduled;
      where nothing defers it, it is removed in place after them. `make vet-all`, `make test` and `make test-scenario`
      green here
- [x] On danoble-wd11-3: the scenario fails without the fix and passes with it. `make test-scenario`, 2026-10-20: at
      `1e910ac1` (the scenario run, the fix not yet in), "lore left its manifest behind after uninstall", and star's
      and writ's; at `6fd484c6`, every scenario passes, `TestSelfUninstallFromInstalledCopyScenario` (9.09s) among them

### Phase 4: Merge

- [x] The gate: CI's quality gate locally, `make test-race`, `make test-scenario`, 2026-10-20. `make lint-all` first
      flagged, for Windows, `"%s"` in the manifest's command (gocritic `sprintfQuotedString`); `%q` would double a
      Windows path's backslashes, so the manifest's paths are quoted by hand, as the image's is. The command line is the
      same text, so the Windows run at `6fd484c6` holds; `make lint-all`, `make vet-all` and `make test` green after
- [x] PR script written, shown, and handed over; the PR resolves #1045
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `Makefile` | Modify | Requirement 1 |
| `cmd/scenario/scenario_target_test.go` | Create | Requirement 1 |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirement 2 |
| `cmd/internal/cli/selfdelete_windows.go`, `selfdelete_other.go` | Modify | Requirement 2 |

## Decisions

- **D1. The guard is a Go test under `make test`,** not a Makefile step: it reads the source by Go's parser, so a
  scenario is found by what it does (reads the variable that gates it), not by its name, and it fails in every
  ordinary test run, CI's included, before anyone runs the scenarios.
- **D2. `cmd.exe` deletes the manifest,** after the `.exe`: only it knows when the image is gone, since the process that
  scheduled it has exited. The manifest goes only when the image is all that remained, so the manifest never outlives
  a record it still needs to keep.

## Open Questions

None.
