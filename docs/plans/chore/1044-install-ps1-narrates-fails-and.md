---
title: "install.ps1 narrates, fails and runs programs as the PowerShell style guide's section 7 says"
issue: https://github.com/NobleFactor/devlore-cli/issues/1044
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: install.ps1 speaks through PowerShell's streams, and fails with error records

Lane 30 of #949. One pull request.

## Summary

The PowerShell style guide's section 7, Messages and Failure (noblefactor-ops `0f8b2fd`), says how a script narrates,
fails and reads native programs. `install.ps1` predates it: it narrates on the information stream with
`-InformationAction Continue`, throws strings, never notes its caller's error action, and leaves a program's standard
input open. #1044 compares the two and rules the resolution. This plan carries it out in `install.ps1` and in
`scripts/Test-InstallScript.ps1`, which tests it and has the same faults.

## Rulings

- 2026-10-19, asked whether the `catch` ends with section 7's `exit 1` or returns, as #965 requires: "return, per #965.
  file it." (#1044's own text.)
- #1044's Resolution (filed 2026-10-19): narration to `Write-Verbose`; results on the success stream; the caller's error
  action noted, `Stop` set, the work in `try`/`catch`, the `catch` restoring it and reporting with
  `$PSCmdlet.WriteError`; every refusal an error record of the script's own; standard input closed; Windows PowerShell
  5.1 still runs it (#948); `irm | iex` still leaves the session open (#965); `Test-InstallScript.ps1` checks the
  streams and records, and itself follows section 7.

## Current State

At `52011130`. `install.ps1` is 973 lines; `scripts/Test-InstallScript.ps1` 1,369.

| Area | `install.ps1` today | Lines |
| --- | --- | --- |
| Narration | `Write-Info` and `Write-Success` wrap `Write-Information -InformationAction Continue`, prefixed `info:` and `success:`; blank lines likewise | 140–160, 749–750, throughout |
| Warnings | `Write-Warn` wraps `Write-Warning` | 162–171, 934 |
| Results | `-Help`'s usage, the summary, the PATH advice, "Verify installation" and the next steps are information-stream text | 75–94, 919–965 |
| Errors | `$ErrorActionPreference = 'Stop'` at script scope, the caller's not noted; `Main` has `try`/`finally` only | 68, 745–971 |
| Refusals | `Write-Fatal` throws the message as a string, at twelve call sites | 173–186; 202, 215, 479, 582, 588, 693, 755, 785, 812, 862, 871, 914 |
| Native programs | standard input not redirected; each line of stderr to `Write-Information` | 602–662 |

`Test-InstallScript.ps1` captures the installer with `*>&1`, judges a refusal by the string it throws and a stack trace
beginning `at Write-Fatal,` (787–821), a success by the text `success: Checksum verified` (823–855), `-Help` by the
information stream (`6>&1`, 1088), and off Windows by the thrown message (1013–1035). It narrates with
`Write-Information -InformationAction Continue` and throws strings (342, 357, 366, 620, 695).

**Measured 2026-10-20, PowerShell 7.4.6 on Linux.** A script with `[CmdletBinding()]` has `$PSCmdlet` at script scope
when run as a file (`ExternalScript`) or in the script-block form (`Function`), and **not** under `irm | iex`
(`Script`). So section 7's script-scope `catch` with `$PSCmdlet.WriteError` would itself fail under `irm | iex`. An
advanced function has its own `$PSCmdlet` in every form: its `catch` reporting through `$PSCmdlet.WriteError` showed
"Install-DevLore: This script requires Windows." with id `RequiresWindows` in all three forms; run as a file, the
script then exited 1 with `exit 1`; in the script-block and `irm | iex` forms it returned and the session went on; a
caller with `$ErrorActionPreference = 'Stop'` caught the record, its id `RequiresWindows,Install-DevLore`.

## Requirements

### Requirement 1: Each stream has its one use (`install.ps1`)

- What the installer is doing goes to `Write-Verbose`: every `Write-Info` and `Write-Success` line, without its
  `info:` or `success:` prefix. `Write-Info`, `Write-Success` and `Write-Warn` go; `Write-Warning` is called directly.
- Results go to the success stream as text: `-Help`'s usage; the summary (`Installed:`, `Location:`, `Registered:`,
  `Already registered:`); the PATH advice; "Verify installation"; the documentation link; the next steps and the
  layers skipped. Same words, without the prefixes (**D3**).
- No `Write-Information`, no `-InformationAction`, no preference but `$ErrorActionPreference` set. A plain run shows the
  warning, any error, and the results; `-Verbose`, or `$VerbosePreference = 'Continue'` before `irm | iex`, shows the
  narration too.

### Requirement 2: Every error is caught and reported as an error record (`install.ps1`)

- `Main` becomes `Install-DevLore`, an advanced function holding the installer's work, because only a function has
  `$PSCmdlet` in all three forms (**D1**). It notes its caller's error action, sets `Stop`, and runs the work in
  `try`/`catch`/`finally`. The `catch` restores the caller's error action and reports the error with
  `$PSCmdlet.WriteError`; the `finally` removes the temporary directory, as now.
- After a failure: run as a file, the script exits 1; in the script-block and `irm | iex` forms it returns, and the
  session goes on (#965) (**D2**).
- `$ErrorActionPreference` is set inside `Install-DevLore`, not at script scope, so under `irm | iex` the user's own is
  never changed (**D4**).
- Every refusal is an `ErrorRecord` of the installer's own, with an id and a category, thrown inside the caught work.
  `Write-Fatal` goes. The twelve:

  | Refusal | Id | Category |
  | --- | --- | --- |
  | not Windows | `RequiresWindows` | `InvalidOperation` |
  | an architecture the releases don't publish | `UnsupportedArchitecture` | `NotImplemented` |
  | layers given, and `DEVLORE_TOOLS` leaves writ out | `LayersNeedWrit` | `InvalidArgument` |
  | GitHub's API refused (`Invoke-ApiGet`) | `GitHubRefused` | `ConnectionError` |
  | GitHub lists no release | `NoRelease` | `ObjectNotFound` |
  | a download refused | `DownloadRefused` | `ConnectionError` |
  | GitHub served a file by another name | `ServedOtherFile` | `SecurityError` |
  | the checksums file has no line for the archive | `NoChecksum` | `SecurityError` |
  | the checksum doesn't match | `ChecksumMismatch` | `SecurityError` |
  | no program in the archive for `DEVLORE_TOOLS` | `NoPrograms` | `ObjectNotFound` |
  | a program's `self install` failed | `SelfInstallFailed` | `NotInstalled` |
  | `writ repo set` failed | `LayerNotRegistered` | `NotInstalled` |

  Each message stays as it is today, word for word.

### Requirement 3: Native programs (`install.ps1`)

`Invoke-NativeCommand` redirects the program's standard input and closes it, and each line of its stderr goes to
`Write-Verbose`. Its arguments stay as they are: `ArgumentList` on PowerShell 7, one quoted command line on 5.1.

### Requirement 4: The suite checks the streams and the records (`scripts/Test-InstallScript.ps1`)

- The installer runs with `-Verbose`, its streams captured with `*>&1`, and each record kept by its kind.
- A refusal is judged by the error record: its id (`FullyQualifiedErrorId` begins with the table's id), its message,
  word for word as today, and that nothing was extracted or installed. The stack-trace check goes.
- A success is judged by the verbose record "Checksum verified" and the writ installed, as now.
- `-Help`: the usage on the success stream.
- Off Windows: the record `RequiresWindows`, with nothing installed.
- New checks:
  - a run without `-Verbose` writes no verbose record and nothing to the information stream;
  - the caller's `$ErrorActionPreference` is the same after a run as before, after a success and after a refusal;
  - a caller who sets `Stop` catches a refusal's record;
  - a native program's stdin is closed: a program that reads stdin ends rather than waits (a fixture program).
- As a file, a failure exits 1 and `-Help` exits 0, as checked today; `irm | iex` leaves the session alive, as checked
  today.

### Requirement 5: The suite follows section 7 too (`scripts/Test-InstallScript.ps1`)

Its PASS and FAIL lines and its summary are results, on the success stream; what it is doing goes to `Write-Verbose`;
its five refusals (342, 357, 366, 620, 695) are error records of its own; it is a file, so it exits 1 after reporting.

### Requirement 6: The design says so

`docs/architecture/10-command-line-interface.md` § 2, The installers: the `install.ps1` bullet says it narrates and
fails as the style guide's section 7 says, with the two departures this plan measures (**D1**, **D2**).

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `chore/1044-install-ps1-narrates-fails-and` and reviewed with the owner: approved 2026-10-20

### Phase 2: `install.ps1` (Requirements 1 to 3)

- [x] Streams, errors and native programs as Requirements 1 to 3 say. The record builder is `ConvertTo-InstallerError`:
      PSScriptAnalyzer reads `New-` as changing state and asks for ShouldProcess, and it changes none
- [x] Off Windows, here: each of the three forms refuses with `RequiresWindows`, the session going on in two and the
      file exiting 1; `Test-PowerShell.ps1` (PSScriptAnalyzer and the shebang rule) clean. Run 2026-10-20, PowerShell
      7.4.6: as a file, exit 1, and `-Help` exit 0 with the usage on the output stream; the script-block form with
      `-Verbose` shows the narration, reports `RequiresWindows`, and the session goes on; under `irm | iex` the session
      goes on and the caller's `$ErrorActionPreference` is unchanged; a caller at `Stop` catches
      `RequiresWindows,Install-DevLore`; one at `SilentlyContinue` sees nothing and `$Error` keeps the record

### Phase 3: The suite (Requirements 4 and 5)

- [x] `Test-InstallScript.ps1` as Requirements 4 and 5 say; off Windows, here, its one check passes. Done
      2026-10-20:
      - Each of the 19 refusal checks names the record's id, read from the syntax tree to hold six named parameters
        each.
      - The stdin check runs, in a child session whose standard input is a file, the installer's own
        `Invoke-NativeCommand` on a PowerShell reading its standard input to the end. Run here against this change, it
        read 0 bytes; against the installer at `52011130`, 35, the file's: it tells the two apart.
      - The fixture runs' capture, tried here on the refusal off Windows, read the record
        `RequiresWindows,Install-DevLore` with its message and the narration from the verbose stream, nothing from the
        information stream, and left the suite's error action as it was.
      - `Test-GitHubItself` sends `Authorization: Bearer`, the one form #1060 settled, which #1101's pull request
        missed there.
      - The checks that install run on Windows alone, in the Installers workflow

### Phase 4: The design (Requirement 6)

- [x] § 2's `install.ps1` bullet: how it speaks and fails, and the two measured departures (D1, D2)

### Phase 5: Merge

- [x] The gate, 2026-10-20: CI's quality gate locally (`make build PLATFORM=all`, `go mod tidy` with no diff, `make
      regenerate` with no diff, `Test-GuideFrontmatter.sh`, `make vet-all`, `star lint go`, `make lint-all`, `star lint
      shell`, `star lint starlark`, `Test-PowerShell.ps1`), the suite off Windows, `make test-race`, `make test-scenario`
- [x] PR script written, shown, and handed over; the PR resolves #1044
- [x] On Windows, before the pull request: `scripts/Test-InstallScript.ps1` at `e3cf261a` on danoble-wd11-3
      (windows/arm64), with this branch's archive from `make dist PLATFORM=windows/arm64`, 2026-10-20: every check
      passed, 84, under Windows PowerShell 5.1.26100.9711 and under PowerShell 7.6.6; `irm | iex` skipped, as off CI
- [ ] CI on the pull request, every leg: the Installers workflow's `install.ps1` legs run the suite under Windows
      PowerShell 5.1 and PowerShell 7, on amd64 and arm64

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirements 1 to 3 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirements 4 and 5 |
| `docs/architecture/10-command-line-interface.md` | Modify | Requirement 6 |

## Out of Scope

- **What `irm | iex` leaves in the user's session:** `Set-StrictMode`, the script's functions and variables. That is
  #982, lane 46 of #949. This plan's D4 keeps the error preference out of it; the rest stays #982's.
- **`install.ps1`'s Administrator advice for a per-user PATH change, and its empty Next steps heading:** #1050, lane 34.
- **The suite's check that the download directory is removed:** #1051, lane 35.

## Decisions

- **D1. The catch is an advanced function's,** `Install-DevLore`, not the script's: under `irm | iex` the script has no
  `$PSCmdlet` (measured above). The error names `Install-DevLore` in every form, where section 7's example names the
  script.
- **D2. A file exits 1; a session returns.** Section 7 ends its `catch` with `exit 1`; ruled 2026-10-19, "return, per
  #965", because in the script-block and `irm | iex` forms `exit` ends the user's session. Run as a file there is no
  session to end, and section 7's exit 1 is what tells a process, CI among them, that it failed. The script tells the
  forms apart by `$MyInvocation.MyCommand.CommandType`, `ExternalScript` for a file (measured above).
- **D3. Results are text on the success stream,** in today's words without the prefixes: a person reads them, under
  `irm | iex`, and the suite checks them as text.
- **D4. The error preference is the function's.** Set at script scope, it changes the user's session under `irm | iex`.

## Open Questions

None.
