---
title: "install.ps1's programs read a byte-order mark from their closed standard input under Windows PowerShell 5.1 with a UTF-8 console"
issue: https://github.com/NobleFactor/devlore-cli/issues/1104
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: no byte-order mark reaches a program install.ps1 runs

Lane 57 of #949: a defect lane 30 (#1103) merged. One pull request.

## Summary

#1044 gave each program `install.ps1` runs a standard input that is redirected and closed, so a program that reads it
ends rather than waits. Under Windows PowerShell 5.1 with a UTF-8 console, that input is not empty: .NET Framework 4.8
writes `Console.InputEncoding`'s preamble, `EF BB BF`, the moment the process starts. This plan keeps it out.

## Rulings

- 2026-10-20: "We do not store boms. Ever!"
- 2026-10-20, offered the encoding set for the moment the process starts, an inherited standard input, or
  `cmd /c ... < NUL`: "Implement your solution. In the past I've redirect input to the null device, but this is ok."

## Current State

At `46122426`, `install.ps1`'s `Invoke-NativeCommand` sets `RedirectStandardInput` and closes `StandardInput` after
`Process.Start`. Measured on danoble-wd11-3 (windows/arm64), 2026-10-20:

| Edition | Console input encoding | The program reads |
| --- | --- | --- |
| 5.1.26100.9711 (.NET Framework 4.8) | OEM 437 | nothing |
| 5.1.26100.9711 (.NET Framework 4.8) | UTF-8 | `239,187,191` |
| 7.6.6 (.NET 10) | either | nothing |

Closing the writer's base stream instead does not help under 5.1 (tried): the preamble is written at `Process.Start`.
.NET Framework's `ProcessStartInfo` has no `StandardInputEncoding`; .NET 10's has.

The suite's stdin check (`scripts/Test-InstallScript.ps1`) passed here and failed on CI's runners only because their
console is UTF-8 and this machine's is OEM.

## Requirements

### Requirement 1: The program's standard input is empty

`Invoke-NativeCommand` reads `[Console]::InputEncoding`. When it is UTF-8 with a preamble, it sets UTF-8 without one for
the moment `Process.Start` runs, and restores the caller's in a `finally`. The code page is 65001 throughout, so section
7's rule that a script never changes the console's code page holds. Without a console, where reading the encoding
fails, nothing is set.

### Requirement 2: The suite catches it on any console

The stdin check's child session sets its input encoding to UTF-8 with a preamble before it runs `Invoke-NativeCommand`,
so the check finds the mark whatever console a runner has, this machine's OEM one included. It also checks that the
child's input encoding is UTF-8 with its preamble again afterwards.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1104-install-ps1-s-programs-read-a`; the approach ruled 2026-10-20

### Phase 2: The fix and its check

- [x] Requirements 1 and 2
- [x] The suite's stdin check fails against `46122426`'s `install.ps1` and passes with the fix, under 5.1 on
      danoble-wd11-3; the whole suite passes there under 5.1 and 7; `Test-PowerShell.ps1` clean. Run 2026-10-20 at
      `5939bc39`: with the fix, 85 of 85 under Windows PowerShell 5.1.26100.9711 and under PowerShell 7.6.6; with
      `46122426`'s `install.ps1` under 5.1, 84 and one failure, "stdin read: 3", as on CI's runners, though this
      machine's console is OEM

### Phase 3: Merge

- [ ] The gate: CI's quality gate locally, the suite off Windows, `make test-race`, `make test-scenario`
- [ ] PR script written, shown, and handed over; the PR resolves #1104
- [ ] CI on the pull request, every leg: the `install.ps1` legs under 5.1 and 7 on amd64 and arm64. Nothing acts on the
      checks while the script's gate watches them (NobleFactor/noblefactor-ops#317, 2026-10-20)

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirement 1 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirement 2 |

## Decisions

- **D1. UTF-8 alone.** Of the code pages a console takes, UTF-8's encoding is the one .NET gives a preamble; the
  encoding is set only when it has one, and only to the same code page.

## Open Questions

None.
