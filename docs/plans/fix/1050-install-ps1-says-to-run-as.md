---
title: "install.ps1 says to run as Administrator for a per-user PATH change, and prints an empty Next steps heading"
issue: https://github.com/NobleFactor/devlore-cli/issues/1050
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: install.ps1 knows whether a new terminal will run what it installed, and says only what is so

Lane 34 of #949. One pull request.

## Summary

Three faults in the end of `install.ps1`'s summary. Two were found 2026-10-19 auditing #949, as #1050:

- "Add it to your PATH (run as Administrator):" introduces a command that sets the `User` scope, which needs no
  elevation; run elevated as another account, it would even change that account's PATH, not the user's.
- "Next steps:" is printed always. `install.sh` prints it only when a layer was registered or skipped, and #1029's plan
  required the two installers to agree and ruled out a next step for a run that changed no layer.

The third was found 2026-10-20 reviewing the first, and folded in: ruled "fold this into lane 34". Whether to give the
PATH advice is decided by comparing the install directory's name, as a string, with this session's `PATH`. That answers
the wrong question. What the installer needs to know is whether a new terminal would run the program it installed: a
session opened before the PATH changed is stale, a folder can be named many ways, and another copy of the program can
come first.

## Rulings

- 2026-10-20, of the PATH check: "we want to know if the thing we installed exists on the PATH. we aren't doing a string
  compare." Then, of the approach: read the machine and user PATH and search them for the program. Each entry is
  taken as written: trimming one "changes the semantics of the users path settings".
- 2026-10-20, of the PATH values as parameters: "The search path is well defined and we have no need for this
  abstraction." The function reads them itself.
- 2026-10-20, of what the function returns: "I might like to have this function for use elsewhere ... did you think
  about returning a Command object as returned by Get-Command?" and, of its name, "if it works the same way and returns
  the same object, i would name it Get-CommandOnPersistentPath." Its body is split "into begin and process sections",
  and its type names are "correctly cased".
- 2026-10-20, of repeated and empty entries, after measuring that a blank entry is not the current directory and that
  a repeated folder is only searched again: "do away with the seen hash map ... concatenate and get rid of falsey
  folders."
- 2026-10-20, of PATHEXT: its extensions go in a map "that uses case-insensitive compares", built "with linq", under
  `using namespace System.Linq`.
- 2026-10-20, measured on danoble-wd11-3 and agreed: a new process's PATH is the machine PATH followed by the user PATH
  ("it's machine followed by user").
- 2026-10-20: "we don't need to worry about powershell 7" for the edges of `Get-ChildItem`'s path handling.
- 2026-10-20: "fold this into lane 34."
- 2026-10-20, of how a function is documented, now the PowerShell style guide's section 10 (noblefactor-ops#323, merged
  as noblefactor-ops#325): "If you source a file, it should document the functions using PowerShell help format.
  Otherwise, document functions like you would for Go."

## Current State

At `844a947f`, `install.ps1`:

| What | Today |
| --- | --- |
| The check (`install.ps1`, results) | `$installDir -notin ($env:PATH -split ';')`: this session's PATH, the directory's name compared as a string, ignoring case |
| The advice | a warning, "`<dir>` is not in your PATH", then "Add it to your PATH (run as Administrator):", `[Environment]::SetEnvironmentVariable('Path', "<dir>;" + ...('Path', 'User'), 'User')`, and "Or add to your PowerShell profile (`$PROFILE`):" with `$env:PATH = "<dir>;$env:PATH"` |
| "Verify installation:" | the other branch: when the directory's name is on this session's PATH |
| "Next steps:" | always, after a blank line |

Measured on danoble-wd11-3 (Windows PowerShell 5.1.26100.9711), 2026-10-20:

- `[Environment]::GetEnvironmentVariable('Path', 'Machine')` and `('Path', 'User')` read the registry, and expand a
  `REG_EXPAND_SZ` value's `%...%` themselves: the user PATH's raw value holds `%`, and the returned one does not.
- A new session's PATH is the machine entries, then the user entries. A session opened through sshd held 30 entries
  from the registry's 22 and 10: it had dropped one entry that repeats another but for a trailing `\`, and lower-cased
  another. That is sshd building its own PATH, so it says nothing of what Windows does at logon.
- A blank PATH entry is not the current directory: PowerShell, with `System32;;Windows`, `System32;` and `;System32`
  each as PATH, did not find a program that only the current location held.
- `Get-Command` given a full path returns an `ApplicationInfo`, even from a folder named `b[x]`; given a path with no
  extension, it finds nothing, so applying PATHEXT is the caller's work.
- `Get-ChildItem -Path` reads `[...]` in a folder's name as a wildcard and silently searches nothing there;
  `-LiteralPath` searches it. `Get-ChildItem -LiteralPath` returns matches in the order of the folders given.
- `Get-Command` and `where.exe` search this process's PATH alone.

Neither suite has a run in which all three layers are already registered.

Neither PowerShell file follows section 10: `install.ps1`'s 19 functions and the suite's 21 carry free-form comments,
some in the form `Name: description`, some none (`Write-Pass`, `Write-Fail`). `install.ps1` writes
`-ErrorAction SilentlyContinue` once (`Install-DevLore`'s cleanup), where section 6 says `Ignore`. Lines run past 120
columns in both: 8 in `install.ps1` and 14 in the suite, some of them addresses that cannot break.

## Requirements

### Requirement 1: Whether a new terminal would run what was installed

`Get-CommandOnPersistentPath -Name <name> [-All]` gets a program as `Get-Command` does, from the PATH a new terminal
searches. As written and tested 2026-10-20:

- `begin` reads the folders, the machine PATH's entries then the user PATH's, as
  `[Environment]::GetEnvironmentVariable` returns them (the registry's, expanded), each as written; an empty entry
  goes, a repeat stays. It reads the machine's PATHEXT into a `Dictionary[String, Int32]` that compares without case,
  each extension to its place, by LINQ's `ToDictionary` under `using namespace System.Linq`.
- `process` takes each name, from `-Name` or the pipeline. A name with an extension is searched for as written; a name
  without one, with each of PATHEXT's extensions. Each folder in turn is searched with `Get-ChildItem -LiteralPath`,
  its files taken in PATHEXT's order; a file whose extension is not a key, `writ.exe.config`, is not a program.
- It writes `Get-Command`'s `ApplicationInfo` for the first copy, or with `-All` for every copy in order, and, as
  `Get-Command` does, a non-terminating error, `CommandNotFoundOnPersistentPath`, for a name it finds nothing for.

The installer asks it of `writ`, or, when writ was not installed, of the first program it did install, with
`-ErrorAction Ignore`. Nothing is not on the PATH; a `Source` that is the installed file, compared without case, is on
it; any other `Source` is another copy that a new terminal runs instead.

### Requirement 2: The advice follows the answer

- On the PATH: "Verify installation:" and its commands, as today; no advice.
- Not on the PATH: the warning "`<dir>` is not on your PATH", then "Add it to your PATH:" (no elevation), the same
  `User` command, and "then open a new terminal: this one keeps the PATH it started with." The profile line goes with
  its reach stated: "Or, for PowerShell alone, add to your profile (`$PROFILE`):".
- Another copy first: a warning naming it, "`<found>` comes before `<installed>` on your PATH, so `<program>` runs
  it", and the same advice to put the install directory first.

### Requirement 3: "Next steps:" only with a step

`install.ps1` writes the blank line and "Next steps:" only when a layer was registered or skipped, as `install.sh`
does. A run that changed no layer ends with the documentation line.

### Requirement 4: The suites check it

- On the PATH and another copy first are checked by hand on danoble-wd11-3, under 5.1 and 7, against its real PATH,
  and recorded in the pull request's test plan: the suite could reach them only by writing a real user's PATH in the
  registry. So is the function's own behavior: by name and by name with an extension, in other case, `-All`, the
  pipeline, a missing name, files in PATHEXT's order whatever their case, and the function under `Invoke-Expression`
  and `[scriptblock]::Create`.
- In the fixture installs, the install directory is on no PATH, so the summary gives the advice for a program not on
  the PATH: no line says "Administrator", and one says to open a new terminal.
- Both suites register base too, a git working tree each makes in its scratch directory with `git init` (a path layer
  need only be one: `validateWorkingTree`, `cmd/writ/writ/repo_cmd.go`), then run with no layer given: all three named
  as already registered, none skipped, no "Next steps:", the documentation line last.

### Requirement 5: Both PowerShell files are documented as section 10 says, and laid out as sections 4 and 9 say

- `install.ps1`'s comment-based help stays the script's entry point. Each of its functions, `Install-DevLore`
  included, takes a Go-style header (D3).
- Each of the suite's functions, the two it defines inside `Invoke-FixtureInstall` included, takes a Go-style header.
- Every other comment in both is `#` full sentences filled to column 120, and a comment that only narrates the
  statement after it goes; `SilentlyContinue` becomes `Ignore`; a line past 120 columns is broken unless it is an
  address that cannot be: the help's two examples, which a user copies whole.
- Found doing it, and fixed as found: both files break sections 4 and 9 as well. A control structure lacks its blank
  line before or after, `catch`, `finally` and `else` sit on their closing brace's line, `if` is used as a one-line
  expression, and two functions (`Get-ApiHeader`, `Test-Checksum`) take their parameters on one line with no blank
  line after. Each is laid out as the guide says; a checker built on PowerShell's parser finds none left.

## Implementation Phases

### Phase 1: The plan

- [x] This plan committed on `fix/1050-install-ps1-says-to-run-as` and reviewed with the owner: "update the plan and
      let's execute", 2026-10-20

### Phase 2: The PATH check (Requirements 1 and 2)

- [x] `Get-CommandOnPersistentPath` in `install.ps1`, and the advice that follows it; ruled 2026-10-20 to call it with
      `-All` and compare each copy with the installed file by identity, `Get-FileIdentity`: the volume serial number
      and 128-bit file ID of `GetFileInformationByHandleEx`'s `FileIdInfo`, so a hard link, a symbolic link and a
      junction each name the installed file, and a copy does not. A copy in a machine PATH folder gets the advice
      that it needs an administrator (ruled "A")
- [x] By hand on danoble-wd11-3 under 5.1 and 7, the installer's own block against the real PATH: installed where the
      PATH finds it, and a hard link to that copy, verify; a copy off the PATH behind writ on the user PATH, the user
      advice; a `cmd` copy behind `System32`'s, and `klist` in `System32` behind the Zulu JDK's, the machine advice.
      The suite on danoble-wd11-3, 85 passed under 5.1 and 85 under 7; off Windows here, the refusal; the
      organization's PowerShell gate, 4 checked, none with findings

### Phase 3: Next steps (Requirement 3) and the every-layer case (Requirement 4)

- [x] "Next steps:" only with a step
- [x] Both suites' every-layer-registered case, and the `install.ps1` suite's checks of the PATH advice: the
      `install.sh` suite here, every check passed; the `install.ps1` suite on danoble-wd11-3, 92 passed under 5.1 and
      92 under 7, and against `844a947f`'s `install.ps1` its four new checks of the summary failed, the rest passed

### Phase 4: Documentation (Requirement 5)

- [x] `install.ps1`'s and the suite's headers, comments and layout; the organization's PowerShell gate, 4 checked, none
      with findings; the `install.ps1` suite on danoble-wd11-3, 92 passed under 5.1 and 92 under 7; the PATH advice by
      hand again, as Phase 2 ran it, unchanged

### Phase 5: Merge

- [ ] The gate: CI's quality gate locally, both suites, `make test-race`, `make test-scenario`, all finished green
      before the PR script is called ready
- [ ] PR script written, shown, and handed over; the PR resolves #1050
- [ ] CI on the pull request, every leg; nothing acts on the checks while the script's gate watches them

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `install.ps1` | Modify | Requirements 1 to 3 |
| `scripts/Test-InstallScript.ps1` | Modify | Requirement 4 |
| `scripts/Test-InstallScript.sh` | Modify | Requirement 4 |
| `install.ps1`, `scripts/Test-InstallScript.ps1` | Modify | Requirement 5 |

## Out of Scope

- **`install.sh`'s PATH check.** On Linux and macOS there is no registry: a new terminal's PATH is what the user's shell
  profile builds, which the installer cannot read reliably, so it keeps checking the shell's `$PATH`.

## Decisions

- **D1. The base layer is a scratch working tree, not a URL.** The suites' team layer is a URL already, and base by URL
  would clone a second repository from GitHub on every run; a local `git init` is all a path layer needs.
- **D2. Search the PATH, not ask a child process.** A child `where.exe` given the machine and user PATH would use
  Windows' own search; searching the folders with `Get-ChildItem -LiteralPath`, in Windows' order, gives the same
  answer for one program's file name and stays in this process. The search is the owner's approach of 2026-10-20,
  `Get-ChildItem` over the PATH's folders, with `-LiteralPath` and both scopes added.
- **D3. `install.ps1` is a script that is run, so its functions take Go-style headers.** Under `irm | iex` its text runs
  in the user's scope and its functions stay defined there, as a sourced file's would. But none of them is the user's
  to call: the user runs the installer, whose entry point is the script's own help, and `Install-DevLore` is called by
  the script's last line.

## Open Questions

None.
