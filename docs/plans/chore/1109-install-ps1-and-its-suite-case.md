---
title: "install.ps1 and its suite case their type names as .NET does, and end each function with its value"
issue: https://github.com/NobleFactor/devlore-cli/issues/1109
status: complete
created: 2026-10-29
updated: 2026-10-29
---

# Plan: install.ps1 and its suite case their type names as .NET does, and end each function with its value

The PowerShell style guide gained two rules on 2026-10-20, after devlore-cli's two PowerShell files were last brought
into line with it: a type name is cased as .NET cases it (section 5), and a function's last statement is its value,
`return` being for leaving early (section 9). install.ps1 and scripts/Test-InstallScript.ps1 follow neither. This is
a style change with no change in behavior.

One pull request, from `chore/1109-install-ps1-and-its-suite-case`, resolving #1109. Lane 59 of the schedule
NobleFactor/devlore-cli#949.

## Rulings

- 2026-10-20, the owner, written into the guide by noblefactor-ops#326: "I really like using correctly cased type
  names"; "the return value is the value of the last statement".

## Current State

At `0b67c2b4`, develop as the branch opened:

| File | Lowercase type names | Functions ending with `return` and a value | Functions |
| --- | --- | --- | --- |
| `install.ps1` | 53 | 10 | 20 |
| `scripts/Test-InstallScript.ps1` | 111 | 13 | 22 |

Counted by a grep for the lowercase accelerator and primitive names in brackets, and by reading each function's last
statement. In install.ps1 the ten are Get-ApiHeader, Get-HttpStatus, Get-GitHubAnswer, Get-ResponseHeader,
Get-RateLimitMessage, Get-LatestVersion, Get-ServedName, ConvertTo-CommandLineArgument, Get-ListedChecksum and
Get-FileIdentity; in the suite, ConvertTo-SuiteError, Use-Account, Get-LayerRoot, Get-Registration, Invoke-Child,
ConvertTo-EncodedCommand, Initialize-FixtureArchive, Get-GitHubRefusal, Get-ClockTime, Get-LookAlikeName,
Invoke-FixtureInstall, Invoke-RestMethod and Get-RequestName. Three of them, Get-FileIdentity, Invoke-Child and
Invoke-FixtureInstall, end in a block with a `return` inside a branch rather than a bare final `return`.

## Requirements

### Requirement 1: Type names

Every type name in both files is cased as section 5 says, in parameter declarations, `[OutputType]` attributes,
casts and `-is`/`-as` operands alike:

| Written | Becomes |
| --- | --- |
| `[string]`, `[string[]]` | `[String]`, `[String[]]` |
| `[int]` | `[Int32]` |
| `[long]` | `[Int64]` |
| `[bool]` | `[Boolean]` |
| `[double]` | `[Double]` |
| `[char]` | `[Char]` |
| `[object]`, `[object[]]` | `[Object]`, `[Object[]]` |
| `[hashtable]` | `[Hashtable]` |
| `[scriptblock]` | `[ScriptBlock]` |
| `[regex]` | `[Regex]` |
| `[type]` | `[Type]` |
| `[switch]`, `[ref]`, `[ordered]`, `[pscustomobject]` | `[Switch]`, `[Ref]`, `[Ordered]`, `[PSCustomObject]`, accelerators with no .NET type of their own, cased as the guide cases `[Switch]` |

Fully qualified names, `[System.Management.Automation.ErrorRecord]` and the like, are already .NET's and stay. The
C# inside install.ps1's `Add-Type` here-string is not PowerShell and stays. PowerShell reads a type name without
regard to case, so no behavior changes.

### Requirement 2: Function endings

Every function ends with its value, not `return` and its value. A `return` that leaves a function early stays. For
the three functions that end in a block, each branch ends with its value in place of its `return`, so the function's
last statement is the block and the block's value is the function's; where a branch returns nothing and the function
goes on, the early `return` stays. Each of the twenty-three is read, not pattern-replaced, since a `return` whose
removal would let execution fall through to a later statement changes behavior.

### Requirement 3: Verification

- The grep of Requirement 1 counts zero in both files.
- Each of the twenty-three functions' endings read and recorded as the value, with the early returns that remain
  named.
- The organization's PowerShell gate, `make powershell-lint`, and the parser find nothing; `make check-all` passes
  on this host, where the PowerShell suite checks the refusal alone.
- CI's install.ps1 legs pass under Windows PowerShell 5.1 and PowerShell 7 on both Windows runners, where the suite
  runs in full against the changed installer: the behavior check this change needs.

## Implementation Phases

### Phase 1: The plan

- [x] This plan reviewed with the owner, 2026-10-29: "Execute the plan"; committed with the work, by the owner's
      direction that the PR script makes the commit

### Phase 2: The work, in one commit

- [x] Requirements 1 and 2 in both files. Type names: 53 and 111 recased by the table, plus the generic arguments of
      two `Dictionary[String,String]` type names written as strings in the suite, which the bracket grep did not
      see. Endings: 9 functions in install.ps1 and 13 in the suite now end with their value; Get-FileIdentity already
      did, its only `return` being C# inside `Add-Type`, so the issue's count of 10 was 9 and 1. Three `return`s
      inside Invoke-FixtureInstall's stand-in script blocks leave a branch early and stay. One consequence found by
      the analyzer: with `return` gone from the stand-in Invoke-RestMethod's `, $releases`, PSScriptAnalyzer infers
      an `Array` result its `[OutputType]` did not name, an Information finding the gate counts; the attribute now
      says `[PSCustomObject], [Array]`, which is what the stand-in writes
- [x] Requirement 3's local checks, the grep at zero in both files, every ending read, both files parsing with zero
      errors and the analyzer finding nothing
- [x] `make check-all` green on this host, 2026-10-29: "check-all passed on linux/arm64", both installer suites among
      it, the PowerShell one at the refusal that is all it checks off Windows

### Merge

- [x] `make check-all`, finished green as the PR script runs it before the commit that carries this plan; a failure
      there stops the script before anything is committed, so this box and the commit stand or fall together
- [x] PR script `~/Workspace/NobleFactor/devlore-cli.1109-install-ps1-and-its-suite-case.bash` written, shown and
      handed over; the PR resolves #1109 and records lane 59 on #949; CI's Windows legs are its behavior check. Its
      one commit, the pull request's last, sets this plan `complete`

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirements 1 and 2 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirements 1 and 2 |

## Open Questions

None.
