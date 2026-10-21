---
title: "The installer suites check the download directory is removed, and the local checks run what CI runs"
issue: https://github.com/NobleFactor/devlore-cli/issues/1051
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: the installer suites check that nothing is left behind, and one list of checks serves CI and the local gate

Lane 35 of #949. One pull request, resolving #1051, #670 and #1110.

## Summary

Three issues, one pull request (ruled 2026-10-21: "We will do that change in the same PR as lane 35").

- **#1051.** #958 fixed `install.sh` leaving its download directory in `TMPDIR`, and proved it by hand. Neither suite
  checks it, so the regression has no test.
- **#670.** CI's quality gate and the local gate are three lists that drift: `ci.yaml`'s steps, `make check`, and each
  PR script's pre-flight. `make check` runs `complexity` and `verify-ldflags`, which no workflow runs, and does not
  run `test-race`, the scenarios or the installer suites, which CI runs. The ruling of 2026-08-25 stands: "CI is a
  strict superset of the local checks by default, and a developer can opt into running everything locally."
- **#1110.** CI installs PSScriptAnalyzer with `Install-Module`, the version written in `ci.yaml` alone; on #1108 that
  step failed to find PSGallery. A developer's machine gets whatever version its profile installed, unpinned.

## Rulings

- 2026-08-25, #670: "CI is a strict superset of the local checks by default, and a developer can opt into running
  everything locally — including the cross-platform checks, whose setup can be postponed."
- 2026-10-20: "we should be using Install-PSResource and pinning."
- 2026-10-21: "We will do that change in the same PR as lane 35." Then, of the work as described: "Go, let's get this
  shit done."

## Current State

At `66669b9c`:

| Gate | Runs |
| --- | --- |
| CI's `quality-gate` job | build for every platform, `go mod tidy` unchanged, `make regenerate` unchanged, guide frontmatter, `make vet-all`, `star lint go`, `make lint-all`, `star lint shell`, `star lint starlark`, code metrics (a report), PowerShell lint from the base layer |
| CI's `test` and `scenario` jobs | `make test-race` and `make test-scenario` on six platforms |
| CI's Installers workflow | both suites, on Linux, macOS and Windows under 5.1 and 7 |
| `make check` | `vet`, `lint`, `shell-lint` (the old `.github/scripts/shell-lint.sh`, not `star lint shell`), `complexity`, `verify-ldflags`, `test` |
| Each PR script's pre-flight | its own copy of CI's quality-gate steps, then both suites, `test-race`, `test-scenario` |

Measured 2026-10-21 on develop: `make complexity` and `make verify-ldflags` both pass, so CI can run them at once.

The suites: `scripts/Test-InstallScript.sh` gives each account its own `TMPDIR`, `<scratch>/<account>/tmp`, and
`install.sh` makes its download directory there with `mktemp -d`. `scripts/Test-InstallScript.ps1`'s `Use-Account`
points `TMP` and `TEMP` at `<scratch>/<account>/tmp`, and `install.ps1` makes `devlore-install-<guid>` under
`[System.IO.Path]::GetTempPath()`. Neither suite looks there afterwards.

## Requirements

### Requirement 1: Every installer run leaves nothing in its temporary directory (#1051)

- Both suites check, after every run of the installer, whether it installed or was refused, that it removed its
  download directory: the `install.sh` suite that the account's `TMPDIR` is empty, the `install.ps1` suite that its
  temporary directory holds no `devlore-install-*`.
- Not the whole of `install.ps1`'s temporary directory: under Windows PowerShell 5.1, `Add-Type`'s compiler leaves an
  empty directory with a random name in `TEMP`, one a session (`Get-FileIdentity`, since #1108). Measured 2026-10-21
  on danoble-wd11-3: PowerShell 7 leaves none, and nothing records whose such a directory is. Ruled the same day: "it
  sounds like powershell is unhygenic, not a litterbug."
- Each check names its run, so a failure says which case left something.
- Checked to fail against an installer that leaves its directory behind.

### Requirement 2: One list of checks (#670, items 1 and 2)

- `make quality-gate` runs every check CI's quality-gate job runs, in its order, and `complexity` and
  `verify-ldflags` besides: build for every platform, `go.mod` tidy, generated files current, guide frontmatter,
  `vet-all`, `star lint go`, `lint-all`, `star lint shell`, `star lint starlark`, the PowerShell lint, `complexity`,
  `verify-ldflags`.
- CI's quality-gate job installs its tools, then runs `make quality-gate`. Code metrics stays a step of CI's own: it
  is a report, not a check.
- The PowerShell lint runs the base layer's `Test-PowerShell.ps1`: CI's `.base` checkout when it is there, and
  otherwise the base layer writ has registered, as `star gh` finds its repositories (noblefactor-ops#253).
- `make check` runs `make quality-gate` and `make test`. `make shell-lint` runs `star lint shell`, as CI does.

### Requirement 3: Everything CI runs that this machine can (#670, item 4)

- `make check-all` runs `make quality-gate`, `make test-race`, `make test-scenario` and both installer suites.
- It ends by naming what CI runs that it did not: the `test` and `scenario` legs of every other platform, and the
  Installers workflow's Windows and macOS legs. A green `check-all` claims what it ran, and no more.
- Every PR script's pre-flight is `make check-all`.

### Requirement 4: The PowerShell analyzer, pinned once (#1110)

- The Makefile pins PSScriptAnalyzer's version, 1.25.0, beside golangci-lint's, and `make
  print-psscriptanalyzer-version` prints it.
- CI installs it with `Install-PSResource -Name PSScriptAnalyzer -Version <pin> -Repository PSGallery -TrustRepository
  -Scope CurrentUser`, the version read from the Makefile.
- The PowerShell lint refuses, before it runs, when the installed analyzer is not the pinned version, and says how to
  install it.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `chore/1051-installer-suites-never-check` and reviewed with the owner: "Go, let's get
      this shit done", 2026-10-21

### Phase 2: Nothing left behind (Requirement 1)

- [x] Both suites' checks, failing against an installer that leaves its directory and passing against develop's: the
      `install.sh` suite here, 66 checks passing against develop's installer and 55 failing against one that keeps its
      directory, the 11 others runs refused before it makes one; the `install.ps1` suite, 39 checks, 36 failing
      against one that keeps its directory and 3 refused before it makes one
- [x] The `install.ps1` suite on danoble-wd11-3, 131 passed under 5.1 and 131 under 7; the `install.sh` suite here,
      every check passed. The design document's § 2 says both installers remove their download directory, and what
      5.1's `Add-Type` leaves

### Phase 3: One list of checks (Requirements 2 to 4)

- [x] `make quality-gate`, `make check`, `make shell-lint`, `make check-all`, the analyzer's pin and its check; each
      check's reason, written in `ci.yaml` until now, beside it in the Makefile
- [x] `ci.yaml`'s quality-gate job installs its tools and runs `make quality-gate`; the analyzer installed with
      `Install-PSResource`, which ran in a clean home here and installed 1.25.0
- [x] `make check-all` here, green at `3deedf77`: the quality gate, 113 packages under `-race`, the scenarios, both
      suites; the analyzer check accepting 1.25.0 and refusing 1.24.0 with the command that installs it

### Phase 4: Merge

- [x] The PR script's pre-flight is `make check-all`, finished green before the script is called ready
- [x] PR script written, shown, and handed over; the PR resolves #1051, #670 and #1110
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `scripts/Test-InstallScript.sh` | Modify | Requirement 1 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirement 1 |
| `Makefile` | Modify | Requirements 2 to 4 |
| `.github/workflows/ci.yaml` | Modify | Requirements 2 and 4 |
| `docs/architecture/10-command-line-interface.md` | Modify | § 2, The installers: the download directory, and 5.1's |

## Out of Scope

- **The pre-commit hook's shell lint.** `.githooks/pre-commit` runs `.github/scripts/shell-lint.sh`; moving every
  repository's shell gate to `star lint shell` is #1040.
- **Running other platforms' legs locally.** #670's "setup can be postponed": `check-all` names them as run in CI
  alone.
- **Installing the tools from scratch locally, as CI does.** The open question #670's analysis of 2026-10-20 put to
  the owner; this plan checks the analyzer's version against its pin, the way the Makefile already checks
  golangci-lint's.

## Open Questions

None.
