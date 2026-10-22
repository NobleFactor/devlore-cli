---
title: "Invoke-Expression leaves install.ps1's settings in the user's session"
issue: https://github.com/NobleFactor/devlore-cli/issues/982
status: active
created: 2026-10-21
updated: 2026-10-21
---

# Plan: install.ps1 runs in a scope of its own, and leaves nothing in the user's session

Lane 46 of #949. One pull request.

## Summary

Run as `Invoke-RestMethod <url>/install.ps1 | Invoke-Expression`, `install.ps1` runs in the user's own scope, and what
it sets stays in their session after it ends: `Set-StrictMode -Version Latest`, its functions, and its variables,
`$AuthToken`, which holds `GH_TOKEN`'s value, among them. (`$ErrorActionPreference` no longer leaks: #1044 set it
inside `Install-DevLore`.) The script-block form, `Invoke-Command -ScriptBlock ([scriptblock]::Create(...))`, runs in a
scope of its own and leaves nothing.

## Rulings

- 2026-10-21, of the options #982 gave: "run in its own scope."
- 2026-10-21: PowerShell commands are named by their full cmdlet names, "USE THE LONG NAMES!!!!"

## Current State

At `272e6a98`, after `param()`, at the script's top level: `Set-StrictMode`, `$InstallersUrl`, the `-Help` block, the
configuration (`$GitHubRepo`, `$Version`, `$Tools`, the layers from the environment, `$AuthToken`), every function,
and the `Install-DevLore` call; then `exit 1` when run as a file and the call failed
(`$MyInvocation.MyCommand.CommandType -eq 'ExternalScript'`). The layers' fallback relies on each parameter being a
`[string]`, so an unset environment variable leaves `''`. `docs/guides/getting-started.md` notes that the
`Invoke-Expression` form leaves the installer's settings in the session. The suite's `Invoke-Expression` case runs in
CI alone, since it installs into `~/.local`, and checks the session goes on and its error action is unchanged.

## Requirements

### Requirement 1: The installer's body runs in a scope of its own

- Everything after `param()` up to and including the `Install-DevLore` call runs inside
  `Invoke-Command -ScriptBlock { ... }`: strict mode, the configuration, the functions and the call. Each ends with
  that scope, in every form the installer is run in.
- The exit check moves inside, after the call: `$?` there says whether `Install-DevLore` reported an error; after the
  block it is true whatever the block reported (measured 2026-10-21: the suite's 'as a file: a failure exits 1'
  failed, exit 0, with the check out there). Whether the script runs as a file is decided outside, from the script's
  `$MyInvocation`, and passed in through `-ArgumentList`, so it sets no variable in the caller's scope. In the block
  `$script:MyInvocation` is whichever script called the script-block form (measured: the suite's in-session cases
  ended the suite).
- The layers' fallback reads the environment as `[string]`, so an unset variable is `''`, as before.
- The five parameters' own variables are all that `Invoke-Expression` leaves, holding what was passed, which under
  `Invoke-Expression` is nothing.

### Requirement 2: The documents and the suite

- Getting started's note that the `Invoke-Expression` form leaves the installer's settings goes: neither form leaves
  any.
- The suite's `Invoke-Expression` case runs everywhere the suite runs on Windows, in a child session whose home and
  XDG directories are scratch, so it installs nothing into the user's. After the run it checks: the session goes on,
  strict mode is off (an unset variable reads as `$null`), no installer function is defined, and no installer
  variable is, beyond the five parameters. It fails at `272e6a98`.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/982-irm-iex-leaves-install-ps1-s`; the design ruled 2026-10-21, "run in its own
      scope"

### Phase 2: The installer and the suite (Requirements 1 and 2)

- [x] `install.ps1`'s body in its own scope; getting started's note gone
- [x] The suite's `Invoke-Expression` case in a scratch home, failing at `272e6a98` and passing after, on danoble-wd11-3
      under 5.1 and 7; the whole suite there under both. At `272e6a98` under 5.1: 132 pass, 3 fail, the three new
      checks (strict mode on; the three functions; the five variables). After: 135 pass, 0 fail, under 5.1 and 7

### Phase 3: Merge

- [ ] `make check-all`, finished green as the PR script runs it, read before this box is ticked
- [ ] PR script written, shown, and handed over; the PR resolves #982
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirement 1 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirement 2 |
| `docs/guides/getting-started.md` | Modify | Requirement 2 |

## Open Questions

None.
