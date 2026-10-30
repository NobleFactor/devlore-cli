---
title: "install.ps1 has no exit code and no -Help switch"
issue: https://github.com/NobleFactor/devlore-cli/issues/1161
status: complete
created: 2026-10-29
updated: 2026-10-29
---

# Plan: install.ps1 has no exit code and no -Help switch

install.ps1 exits 1 after a failure when run as a file, deciding how it was run from `$MyInvocation` before its
`Invoke-Command` block and passing the answer in; in the script-block and `irm | iex` forms it returns. It also takes a
`-Help` switch that prints a usage block, where PowerShell's help is the comment-based help `Get-Help` reads. Both
go: the exit code, its flag, the comments that explain them, the help sentence that promises it, the suite's checks
and the spec's sentence; and the switch, its parameter, its help entry, the constant only it read and the suite's
checks of it.

One pull request, from `feature/1161-install-ps1-has-no-exit-code-the`, resolving #1161. Lane 60 of the schedule
NobleFactor/devlore-cli#949.

## Rulings

- 2026-10-23, the owner: "We're bound to Window Powershell, not PowerShell." "The hack required to get an exit code is
  unacceptable." "Install.ps1 has no exit code. Remove all reference to it from the spec."
- 2026-10-28, the owner, of the `-Help` switch: "The help block is SUPPOSED to be removed", "help is ALWAYS provided
  by help comments". The switch goes with the exit code.

## Current State

At `67b21238`, develop as the branch left it, install.ps1 has both. In the worktree, uncommitted, the owner's edit of
2026-10-27 and 2026-10-28 has removed: the exit check after `Install-DevLore`, `param([bool] $RunAsFile)`, the comment
above the check, the `-Help` block, `[switch]$Help` and its `.PARAMETER`, and the help's sentence "A failure leaves the
session open; run as a file, the script exits 1." It parses (checked 2026-10-28, zero parser errors).

What remains of what goes:

| Where | What |
| --- | --- |
| `install.ps1:76` | `-ArgumentList ($MyInvocation.MyCommand.CommandType -eq 'ExternalScript')`, an argument nothing receives, and `param()` under it |
| `install.ps1:70-75` | the comment's last two sentences, which explain the argument |
| `install.ps1:86` | `$InstallersUrl`, read by the `-Help` block alone; the analyzer warns on it now |
| `install.ps1:22-24` | the help says nothing of what a failure does; the issue keeps "A failure leaves the session open, in every form" |
| `scripts/Test-InstallScript.ps1:1424-1438` | the `-Help` check |
| `scripts/Test-InstallScript.ps1:1720-1739` | the two "as a file" checks, a failure exits 1 and `-Help` exits 0 |
| `scripts/Test-InstallScript.ps1:31-32` | the description's sentence naming them |
| `docs/architecture/10-command-line-interface.md:115-116` | "and only a run as a file exits 1 after a failure, since in the script-block and `irm | iex` forms `exit` would end the user's session" |

The suite's three checks run on Windows alone: off Windows the suite stops after the refusal ("every check passed
(off Windows, the refusal is all there is to check)", run here 2026-10-29 against the working tree, exit 0). So the
failure they would show against the working tree is CI's Windows legs' to show, not this host's; they are removed on
the strength of the switch and the exit being gone from the script, which the parser and the analyzer confirm.

## Requirements

### Requirement 1: install.ps1

- The `Invoke-Command` call takes no `-ArgumentList` and its block no `param()`. The comment above it ends at "Only
  the parameters' own variables are the caller's, holding what was passed."
- `$InstallersUrl` and its comment go.
- The help's streams paragraph ends: "A failure leaves the session open, in every form."
- Nothing else changes. `#Requires -Version 5.1`, the analyzer suppression for `Prefix` and the rest stay as they are.

### Requirement 2: The suite

- The `-Help` check and the two "as a file" checks go, with their `Use-Account` calls and the `$env:DEVLORE_VERSION`
  save and restore they alone used, and the description's sentence at `:31-32`.
- The suite runs green under PowerShell 7 on this host through `make check-all`; the Windows PowerShell 5.1 leg is
  CI's, on the pull request.

### Requirement 3: The spec

`docs/architecture/10-command-line-interface.md:115-116` says what is so: the catch is `Install-DevLore`'s, and a
failure leaves the session open and returns, in every form; install.ps1 has no exit code, ruled 2026-10-23. The
`#965` link stays, as the reason `exit` was never an option under `irm | iex`.

### Requirement 4: No other mention

`grep -rn -- '-Help' docs README.md wiki` names nothing of install.ps1's switch once the work is done; a mention
found goes in the same commit.

## Implementation Phases

### Phase 1: The plan

- [x] This plan reviewed with the owner, 2026-10-29: "Execute the plan"; committed with the work, by the owner's
      direction that the PR script makes the commit

### Phase 2: The work, in one commit

- [x] Requirements 1 to 4: install.ps1 parses with zero errors and the analyzer reports nothing, the
      `InstallersUrl` warning among what is gone; the suite parses and names neither the switch nor the file exit;
      the spec says what is so; `grep -rn -- '-Help' docs README.md wiki` names nothing of install.ps1
- [x] `make check-all` passes on this host, 2026-10-29: "check-all passed on linux/arm64", both installer suites
      green, the PowerShell one at the refusal that is all it checks off Windows

### Merge

- [x] `make check-all`, finished green as the PR script runs it before the commit that carries this plan; a failure
      there stops the script before anything is committed, so this box and the commit stand or fall together
- [x] PR script `~/Workspace/NobleFactor/devlore-cli.1161-install-ps1-has-no-exit-code-the.bash` written, shown and
      handed over; the PR resolves #1161 and records lane 60 on #949. Its one commit, the pull request's last, sets
      this plan `complete`

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirement 1 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirement 2 |
| `docs/architecture/10-command-line-interface.md` | Modify | Requirement 3 |
| `docs/architecture/2.6-execution-policies.md`, `.status.md` | Already committed | An engine design note, `0b1f2c4d`, placed on this branch at the owner's direction on 2026-10-28; unrelated to the installer, rides with this PR |

## Open Questions

None.
