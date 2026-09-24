---
title: "install.ps1 is published beside install.sh, and runs on Windows PowerShell 5.1 — what a fresh Windows machine has"
issue: https://github.com/NobleFactor/devlore-cli/issues/948
status: active
created: 2026-09-24
updated: 2026-09-24
---

# Plan: install.ps1 is published, and runs on 5.1

Lane 1 of [#949](https://github.com/NobleFactor/devlore-cli/issues/949). Design:
[#798](https://github.com/NobleFactor/devlore-cli/issues/798), ruling 1 as it stands and ruling 3 as
reversed below.

## Issue 948

`install.ps1` has done what `install.sh` does since #914: fetch the archive for the host, verify it, unpack it
so the products sit in `bin/` beside `share/`, run each product's `self install <prefix> --unattended`. Two
things stand between it and #798's acceptance:

- **Nobody can reach it.** `release.yaml`'s "Sync install script to website" step copies `install.sh` to the
  site's `public/` and `git add`s that one path. The header's own usage line, `irm
  https://devlore.noblefactor.com/install.ps1 | iex`, is a 404.
- **It was never written for the shell it will run in.** A fresh Windows machine has Windows PowerShell 5.1
  and nothing else. The body assumes 7: `$IsWindows` under `Set-StrictMode -Version Latest` throws on 5.1,
  and `Invoke-WebRequest` without `-UseBasicParsing` engages 5.1's Internet Explorer parser.

**Ruling 3 reversed, 2026-09-24.** #798 had the body as `#Requires -Version 7` with a 5.1 preamble that
installs pwsh and re-launches. Ruled instead: the installer uses what is known to be on the box, Windows
PowerShell 5.1, and installs nothing but devlore. The shell-provider rewrite makes PowerShell 7 one shell
among the ones a user may pick, not a prerequisite; the installer does not pre-empt that by installing it.

## Goals

1. `irm https://devlore.noblefactor.com/install.ps1 | iex` works, on the next release after the merge.
2. The one script runs under Windows PowerShell 5.1 and PowerShell 7 alike, on Windows, macOS and Linux,
   one code path.
3. Nothing is installed but lore, star and writ.

## Requirements

### Requirement 1: The sync publishes both scripts

In `.github/workflows/release.yaml`, the sync step copies `install.sh` and `install.ps1`, adds both paths,
and its commit and pull-request text say "install scripts". The release summary (lines 102–107) gains the
Windows one-liner beside the Unix one. Nothing else in the workflow changes.

### Requirement 2: The body runs on 5.1

The floor is declared and the 5.1 incompatibilities fixed; everything else in the body stays #914's.

- `#Requires -Version 5.1` as the first line after the header.
- `Get-OSName`: `$IsWindows`, `$IsMacOS`, `$IsLinux` do not exist on 5.1, and under
  `Set-StrictMode -Version Latest` a missing variable is an error. On 5.1 the edition is `Desktop` and the
  platform is Windows; on 7 the automatic variables exist. The function reads `$PSVersionTable.PSEdition`
  first and the automatic variables only on Core.
- `Invoke-WebRequest` and `Invoke-RestMethod` carry `-UseBasicParsing`: on 5.1 the default engages the
  Internet Explorer DOM parser, which hangs on a machine that has never opened IE. On 7 the switch is
  accepted and ignored.
- `[Net.ServicePointManager]::SecurityProtocol` includes `Tls12` before the first request: 5.1 on a
  machine whose .NET default predates it cannot reach GitHub otherwise; on 7 the assignment is harmless.
- Everything else the body uses is in 5.1: `Expand-Archive`, `Get-FileHash`, `Get-ChildItem -File`,
  `-in`/`-notin`, `[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture` (.NET Framework
  4.7.1+, which Windows 10 and 11 carry). The audit is Phase 3's first box.

### Requirement 3: The header says so

The header's Usage states the floor: "Runs on Windows PowerShell 5.1 and PowerShell 7." The
`.\install.ps1 -Prefix` form notes that a Windows client's default execution policy refuses a downloaded
script, and that the `irm | iex` form is the one that needs no policy change. The `# Parameters` block is
unchanged (lane 2 adds the layer flags).

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change
- [x] The reversal recorded on #798 as a comment, dated, in the owner's words

### Phase 2: The change

- [ ] `release.yaml`: both scripts synced, both paths added, texts and summary updated (Requirement 1)
- [ ] `install.ps1`: the floor and the four fixes (Requirement 2)
- [ ] `install.ps1`: the header (Requirement 3)

### Phase 3: Verify

- [ ] Every cmdlet, operator and .NET member the body uses is checked against 5.1's surface, listed in
      the pull request, and nothing outside it remains
- [ ] Under pwsh 7 on this Linux host: the body runs against a scratch prefix, `DEVLORE_TOOLS=writ`, a
      real pre-release, and `<prefix>/bin/writ --version` reports that release's build
- [ ] Under Windows PowerShell 5.1 on a Windows machine (DANOBLE-WD11-3), both invocation forms, against a
      scratch prefix: the products install and answer `--version`. This is rule 11's live-path test and it
      cannot run here; it is the owner's, or a Windows job's, and this box stays open until it has
- [ ] `release.yaml` parses as YAML, and the sync step's shell block passes `bash -n` and shellcheck when
      extracted
- [ ] No gate in this repository reads `.ps1`: `star lint` has go, go-style, markdown and shell, and
      `ci.yaml` runs no PSScriptAnalyzer. The script is checked here by hand against the organization's
      settings, `noblefactor-ops/Home/common/.config/PSScriptAnalyzer`, once the module is on this host;
      the gap is noted for the lint-tooling schedule (noblefactor-ops#232)

### Phase 4: Merge, publish, converge

- [ ] PR script written, shown, and handed over
- [ ] After the merge: the next pre-release's sync opens the site PR carrying both scripts; when it merges,
      `curl -sI https://devlore.noblefactor.com/install.ps1` answers 200 and the body matches the
      repository's. Until a release runs, this box stays open and the plan `active`

## Out of Scope

- **The layer flags** — lane 2, #950.
- **Installing PowerShell 7**, or any shell. Reversed above.
- **What the products do under 5.1.** `self install` is each product's; if writ's PowerShell actions need
  pwsh on Windows, that is the shell-provider rewrite's question, not the installer's.
- **A Windows CI job that runs the installer end to end.** #855's scenario jobs cover writ's journey, not
  the installer; a job that provisions a 5.1-only Windows is its own work.
