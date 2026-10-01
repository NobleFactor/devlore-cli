---
title: "The installers refuse an archive they can't verify, and install.sh says why it stops"
issue: https://github.com/NobleFactor/devlore-cli/issues/1002
status: approved
created: 2026-10-01
updated: 2026-10-01
---

# Plan: the installers refuse an archive they can't verify

Lane 23 of #949 (amended 2026-10-01 by the owner: "these are good issues. log each one. add them to your schedule.").

## Issue 1002

At `01634491`, the installers don't refuse an archive they can't verify, and `install.sh` stops without saying why
when a release lacks a piece:

- **`install.ps1` warns and installs** when the release has no checksums file (`install.ps1:372`, `:398-399`) or the
  file has no line for the archive (`:390`, `:395-396`).
- **`install.sh` claims a check it didn't make:** with neither `sha256sum` nor `shasum`, `verify_checksum` warns and
  returns 0 (`install.sh:216-218`), and its caller prints "Checksum verified" (`:305`).
- **`install.sh` stops with status 1 and no message** when the release has no checksums file (`:287`), the file has no
  line for the archive (`:302`), or the release has no archive for this platform (`:281`): it runs under
  `set -o errexit -o nounset -o pipefail` (`:9`), and a lookup whose `grep` matches nothing ends the run before the
  branch that would say why. The same ends a run whose latest-release lookup finds nothing (`:247-250`).
- **A refused download is saved as the file.** `download_asset` calls curl without `--fail` (`:191`, `:193`), so
  GitHub's error body becomes the checksums file, and the run then ends at `:302`.
- **Both find the line by pattern:** `grep "${archive_name}"` (`install.sh:302`) and `-match $archiveName`
  (`install.ps1:390`); each `.` matches any character, nothing is anchored, and `-match` ignores case.
- **No installer test covers any of it.** `scripts/Test-InstallScript.sh` checks the guide's by-hand line only
  (`:204-219`), and `scripts/Test-InstallScript.ps1` checks no checksum. Neither asserts "Checksum verified" on the
  success path.

`self upgrade` (#1001) refuses each of these: a release lacking either file
(`cmd/internal/cli/selfupgrade_release.go:370-373`), a file with no line for the archive
(`cmd/internal/cli/selfupgrade_archive.go:432-436`), finding the line by exact name (`listedChecksum`,
`selfupgrade_archive.go:500-514`).

## Rulings

- **This issue, corrected and widened** (ruled 2026-10-01, agreeing): each installer refuses an archive it can't
  verify, naming what is missing; `install.sh` never stops without saying why; it never reports a check it didn't
  make; each finds the line by exact name.
- **From the installers' rulings** (`docs/plans/feature/950-installers-take-base-team-and.md`):
  - The installers never prompt (950:20-22). Errors are errors; no rollback; the same command again is the recovery
    (950:23-25).
  - Errors report GitHub's own message (950:146).
  - `--fail` goes on the published one-liners only; "The four calls inside the script don't: adding it there changes
    how a GitHub API error is reported" (950:108-110).
  - Long options wherever every platform the script runs on accepts them; BSD tools keep short forms where they have
    no long ones (950:92-112).
  - One `install.ps1` code path for Windows PowerShell 5.1 and PowerShell 7 (950:54-55); no `exit` in `install.ps1`,
    which fails by throwing through `Write-Fatal` (950:149-168).
  - CI proves the platforms ("we should test this in ci where we have more vanilla systems", 950:252-253).
- **"Nothing is installed that isn't verified"** (#947, Requirement 5), which the installers now meet too.

## Requirements

Where a requirement rests on a decision of this plan, it names it (**D*n***); the owner's review can overrule any.

### Requirement 1: Each installer refuses an archive it can't verify

Before extracting anything:

- **No checksums file in the release:** both fail, naming the file and the tag.
- **No line for the archive:** both fail, naming the archive and the checksums file.
- **No SHA-256 tool** (`install.sh` only; `install.ps1` uses `Get-FileHash`, which both editions have): fail, naming
  `sha256sum` and `shasum` (**D3**).
- **A mismatch** stays a failure naming both hashes, as today (`install.sh:221-223`, `install.ps1:304`).
- "Checksum verified" is printed only after a check that ran.
- Exit status stays 1 (`install.sh`'s `error`, `:107-110`; `install.ps1` throws through `Write-Fatal`).

### Requirement 2: The line is found by exact name

A line is `<64 hex characters>`, then a space, then a space or `*`, then the archive's name, exactly and with case;
a trailing carriage return is ignored; the first match wins. This is `listedChecksum`'s rule
(`selfupgrade_archive.go:500-514`), so the installers, the guide and `self upgrade` agree on one format:

- `install.sh`: fields compared by `awk`, working under macOS's `/bin/bash` 3.2 and BSD `awk`.
- `install.ps1`: fields compared with `-ceq` after splitting.

### Requirement 3: install.sh never stops without saying why

- The asset lookups (`:281`, `:287`), the line lookup (`:302`) and the latest-release lookup (`:247-250`) report
  their failure: the missing asset, line or release, by name.
- A download checks its HTTP status (curl's `--write-out '%{http_code}'`, not `--fail`, by 950's ruling); on a
  refusal it fails with GitHub's own `message` and the asset's name.

### Requirement 4: install.ps1 finds assets by exact name

The asset lookups (`install.ps1:368`, `:372`) use `-ceq`, so a name differing only in case is not taken (**D4**).

### Requirement 5: The by-hand steps match the installer

`docs/guides/getting-started.md` says its by-hand steps are what the installer does (`:76`), so its checksum lines
(`:90`, `:114-115`) find the line by exact name too, and so does the test that runs them
(`scripts/Test-InstallScript.sh:217`).

### Requirement 6: Tests prove every refusal, on every platform CI runs

The download address stays fixed: "The installers always download from GitHub Releases; nothing overrides where"
(`wiki/Releasing.md:200`), so no test hook is added to the installers (**D5**).

- **`install.sh`** (`scripts/Test-InstallScript.sh`): a fake `curl` first on `PATH` answers the release and asset
  requests from fixtures, with `DEVLORE_VERSION` pinning the fixture's tag. Cases: no checksums file, no line, a line
  for a name that merely contains the archive's, a mismatch, a refused download (the fixture answers 404 with
  GitHub's JSON), no archive for the platform, no SHA-256 tool (a `PATH` of only the tools the script needs), and the
  success path, which must print "Checksum verified". Each failure asserts status 1, its message, and that nothing
  was extracted.
- **`install.ps1`** (`scripts/Test-InstallScript.ps1`): functions named `Invoke-RestMethod` and `Invoke-WebRequest`
  in the test's scope stand in for GitHub. Cases: no checksums file, no line, a name differing only in case, a
  mismatch, and the success path; on Windows PowerShell 5.1 and PowerShell 7.
- **Where:** the Installers workflow (`.github/workflows/installers.yaml`) on its six runners and both PowerShell
  editions, which runs on any pull request touching the installers.
- **Live, after the merge:** the site serves the merged installers; one install on this machine through the
  published one-liner prints "Checksum verified", and the CI legs above stand for macOS and Windows (950's ruling).

### Requirement 7: #947's plan references these issues

The process requires a deferred item to name its issue: `docs/plans/fix/947-self-upgrade-fetches-nothing-it.md`'s Out
of Scope (`:333-338`) says "a follow-up issue" four times; each becomes its issue: #1002, #1003, #1004, #1005.

### Requirement 8: The documents

- The installers' headers and help say they refuse what they can't verify.
- `docs/guides/getting-started.md`, per Requirement 5; any sentence in it, `README.md` or `wiki/Releasing.md` that
  says the installers skip verification is corrected.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, approved, before the work. #1002's table links it.

### Phase 2: The tests, failing (Requirement 6)

- [ ] `install.sh`'s fake `curl` and fixtures, and every case; each fails against today's script for the right reason
- [ ] `install.ps1`'s stand-ins and every case; each fails against today's script

### Phase 3: install.sh (Requirements 1 to 3)

- [ ] Refusals, the exact-name line, the reported lookups and downloads; its tests pass; `star lint shell` clean

### Phase 4: install.ps1 (Requirements 1, 2, 4)

- [ ] Refusals, the exact-name line and assets; its tests pass under 5.1 and 7; the PowerShell gate clean

### Phase 5: The documents (Requirements 5, 7, 8)

- [ ] The guide's by-hand lines and their test; #947's plan; the headers and help

### Phase 6: Verify

- [ ] `make test`; CI's quality gate; the Installers workflow green on all six runners and both editions

### Phase 7: Merge

- [ ] PR script written, shown, and handed over; its `git add` list proven with `git add --dry-run`. The PR resolves
      #1002
- [ ] After the merge, the site serves the merged installers, and an install on this machine through the published
      one-liner prints "Checksum verified"

## Out of Scope

- **GitHub's `sha256` digest in the installers** (**D2**).
- **Making the Installers workflow's checks required:** they run only when the installers change, so a required
  check would sit pending on every other pull request.
- **Running `install.ps1` under PowerShell 7 on macOS and Linux, and `install.sh` under Git Bash, in CI.**

## Decisions

Engineering choices that follow from the rulings; each can be overruled.

- **D1. Exit status stays 1** for every refusal, as the tests already assert; no per-cause exit codes in the
  installers.
- **D2. The installers don't check GitHub's digest.** The issue doesn't ask for it, the checksums file and the digest
  come from the same release job, and in bash it means parsing JSON with `grep`.
- **D3. No third tool for `install.sh`.** With neither `sha256sum` nor `shasum`, it fails and says so; `openssl` is
  not tried. Every platform the installers serve has one of the two today.
- **D4. `install.ps1`'s asset lookups become case-sensitive,** found while reading the same lines; the published
  names never vary in case, so nothing that works today stops working.
- **D5. No test hook in the installers.** The tests stand in for GitHub from outside: a fake `curl` for bash, and
  functions shadowing the web cmdlets for PowerShell.
