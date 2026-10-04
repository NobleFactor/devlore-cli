---
title: "The installers refuse an archive they can't verify, and install.sh says why it stops"
issue: https://github.com/NobleFactor/devlore-cli/issues/1002
status: active
created: 2026-10-01
updated: 2026-10-03
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

## Issue 1008

`install.sh` can't install when GitHub's API sends a release's JSON on one line: `get_asset_id`
(`install.sh:174-179`) reads the JSON line by line, takes the `"id"` from the lines before an asset's name, and on the
one-line form resolves every asset to the release's own id, so every download fails. GitHub sends that form some of
the time (captured 2026-10-01). `install.ps1` parses the JSON and is not affected. Found in this lane's work; ruled
2026-10-02: "fix it in this lane."

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
- **One list of cases, run against both installers** (ruled 2026-10-02: "I insist on your recommendation. I want
  it."): every case that can apply to both is tested against both, so each proves the behavior both promise and
  neither drifts from the other.

## Requirements

Where a requirement rests on a decision of this plan, it names it (**D*n***); the owner's review can overrule any.

### Requirement 1: Each installer refuses an archive it can't verify

Before extracting anything, and before downloading the archive: each installer downloads the checksums file first,
then the archive, as `self upgrade` does (ruled 2026-10-02: "install.sh and install.ps1 should each download the
checksums file before the archive").

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

### Requirement 3: Neither installer stops without saying why

- `install.sh`'s asset lookups (`:281`, `:287`), line lookup (`:302`) and latest-release lookup (`:247-250`) report
  their failure: the missing asset, line or release, by name.
- A refused download fails with GitHub's own `message` and the asset's name. `install.sh` checks the HTTP status
  (curl's `--write-out '%{http_code}'`, not `--fail`, by 950's ruling).
- `install.ps1` reports the same failures in the same terms: a refused download, a missing archive and a missing or
  rate-limited release each name what failed and carry GitHub's `message`, rather than PowerShell's raw error.
  (Widened 2026-10-02 by the one-list ruling: the same cases prove the same behavior; and ruled the same day:
  "ensure that install.ps1 does the same thing for whatever its using for downloads".)

### Requirement 3b: A spent API limit says when to run again

When GitHub's API refuses for its rate limit (a 403 or 429 with `x-ratelimit-remaining: 0`), both installers say so,
give the reset time from GitHub's `x-ratelimit-reset` header as a clock time and in minutes, tell the user to run the
installer again after it, and, when `GH_TOKEN` is unset, that setting it raises the limit. (Ruled 2026-10-02: "if
you've reached your api limit, we report that you reached your api limit ... we should instruct the user to rerun
later. can we get a reset time?") The message, approved 2026-10-02 ("I like the message"):

> GitHub's API limit for this address is used up. It resets at 18:42 (in 23 minutes); run the installer again after
> that. Setting GH_TOKEN raises the limit.

The last sentence appears only when `GH_TOKEN` is unset. When it is set, GitHub counts the limit per token, so
"for this address" reads "for your token" (ruled 2026-10-03, accepting the recommendation). Case 12's rate-limit
variant asserts all of it, with and without `GH_TOKEN`.

### Requirement 3a: install.sh downloads by name, with curl alone

- **By public link, not by asset id** (#1008; ruled 2026-10-02, "fix it in this lane", taking the recommendation):
  each file comes from `https://github.com/NobleFactor/devlore-cli/releases/download/<tag>/<name>`, as `self upgrade`
  downloads from each asset's `browser_download_url`. No asset id is looked up and no asset is found by reading
  GitHub's JSON, so its layout no longer matters. The API is asked only for what a link can't give: the newest
  release's tag, read in a way that works on the indented and the one-line form alike; and, for a release selected
  with `DEVLORE_VERSION`, whether it exists, asked only after a download answers 404, because GitHub's download
  host answers a missing release and a missing file with the same plain-text 404. A selected release that downloads asks
  the API nothing. (Decided 2026-10-02, the owner: "I already answered A.")
- **`curl` only** (ruled 2026-10-02: "drop wget"). The published one-liner already needs `curl`; without it,
  `install.sh` refuses, naming `curl`.

### Requirement 4: Both installers take only the file they asked for, by exact name

GitHub's download host matches a file's name regardless of case: asked for `DEVLORE-CLI_…_CHECKSUMS.TXT`, it serves
the real checksums file (checked live 2026-10-02). So a download names the file it wants, and the installer checks
the name GitHub says it served, in the response's `Content-Disposition` header, and refuses a file by another name:
"Could not download <name> from release <tag>: GitHub served <other name>, a file by another name". When GitHub names
no file, the download is accepted and the checksum still decides. Case 10 (the archive under another case) and case
14 (the checksums file under another case) refuse this way. (Ruled 2026-10-02: "write it into requirement 4".)

### Requirement 5: The by-hand steps match the installer

`docs/guides/getting-started.md` says its by-hand steps are what the installer does (`:76`), so its checksum lines
(`:90`, `:114-115`) find the line by exact name too, and so does the test that runs them
(`scripts/Test-InstallScript.sh:217`).

### Requirement 6: One list of cases proves every refusal, against both installers, on every platform CI runs

The download address stays fixed: "The installers always download from GitHub Releases; nothing overrides where"
(`wiki/Releasing.md:200`), so no test hook is added to the installers (**D5**). `install.sh`'s tests stand in for
GitHub with a fake `curl` first on `PATH`; `install.ps1`'s with functions named `Invoke-RestMethod` and
`Invoke-WebRequest` in the test's scope. `DEVLORE_VERSION` pins the fixture's tag.

**The cases, each run against both installers** (ruled 2026-10-02):

| # | Case | Expected |
| --- | --- | --- |
| 1 | No checksums file in the release | refuses, naming the file and the tag |
| 2 | No line for the archive | refuses, naming the archive and the checksums file |
| 3 | A line only for a name that contains the archive's (`<archive>.sig`) | refuses, as 2 |
| 4 | A line that matches only as a pattern (a `.` in the name replaced) | refuses, as 2 |
| 5 | A line naming the archive in another case | refuses, as 2 |
| 6 | A mismatch | refuses, naming both hashes |
| 7 | The checksums file's download refused (404, GitHub's JSON) | refuses, with GitHub's message and the file's name |
| 8 | The archive's download refused (404) | refuses, with GitHub's message and the archive's name |
| 9 | No archive for this platform in the release | refuses, naming the archive and the tag |
| 10 | The archive published under a name in another case | refuses, as 9 |
| 11 | A look-alike asset, its name differing only at a dot, listed before the real one | takes the real one, installs |
| 12 | No release; a rate limit, on the latest-release lookup | refuses, saying which, with GitHub's message |
| 13 | Success, plain lines; with CRLF lines; with `<hash> *<name>` lines | prints "Checksum verified", installs |
| 14 | The checksums file published under a name in another case | refuses, as 1 |
| 15 | The archive's line behind a byte-order mark | refuses, as 2 |
| 16 | A release selected with `DEVLORE_VERSION` that doesn't exist | refuses, naming the tag |

Cases 14 to 16 were added 2026-10-02 by ruling ("add the three cases to the table"). For `install.sh`, every case
also runs with GitHub's JSON on one line (#1008).

**Each installer's own:** `install.sh` with no `sha256sum` or `shasum` (a `PATH` of only the tools it needs):
refuses, naming both. `install.ps1` with invisible characters in a name (soft hyphen, zero-width space, trailing NUL):
refuses, as 2 or 9.

Every refusal asserts its message, that the archive was never downloaded where the checksums file decides it, that
"Extracting" never appears, and that nothing was installed. For `install.sh` it asserts exit status 1. `install.ps1`'s
refusals run inside the test's own session, which has no exit status: each is proven thrown by `Write-Fatal`, and one
check proves a `Write-Fatal` throw exits 1 when the installer runs as a file. The before-and-after record shows which
cases today's installers already handle: case 6 and 13 for both, and for `install.ps1` cases 9 and 12's "no
release", where it already stops loudly. (Corrected 2026-10-02: this first said every refusal case fails against
today's installer, and that every refusal asserts exit status 1.)

- **Where:** the Installers workflow (`.github/workflows/installers.yaml`) on its six runners and both PowerShell
  editions, which runs on any pull request touching the installers.
- **And macOS with MacPorts' GNU tools first** (ruled 2026-10-02: "add it. i use ports, not brew to get the tools."):
  a seventh leg installs MacPorts on a macOS runner, then `port install coreutils gsed grep gawk gnutar`, and runs the
  whole `install.sh` suite with `PATH=/opt/local/libexec/gnubin:/opt/local/bin:/opt/local/sbin:/usr/bin:/bin`, so
  every tool the installer calls resolves to its GNU version, as on the owner's Macs. On macOS the test script
  normally runs the installer with Apple's tools only, so this leg passes `--keep-path`, which keeps the caller's
  `PATH` and `bash`; the installers are unchanged (ruled 2026-10-03, accepting the recommendation).
- **Live, after the merge:** the site serves the merged installers; one install on this machine through the
  published one-liner prints "Checksum verified", and the CI legs above stand for macOS and Windows (950's ruling).

### Requirement 6a: The test script finishes

`scripts/Test-InstallScript.sh` can end before its summary: when a real install fails (a network outage, say),
`tag=$(writ --version | awk …)` in the guide's section fails under errexit and the script exits 127 without reporting
what passed and failed. It reports every check and its summary whatever fails. (Found 2026-10-01 in this work; ruled
2026-10-02: "On 5: update the test script's summary.")

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

- [x] `install.sh`'s fake `curl` and fixtures, and the one list of cases; each refusal case fails against today's
      script for the right reason
- [x] `install.ps1`'s stand-ins and the same list; each refusal case fails against today's script

### Phase 3: install.sh (Requirements 1 to 4, 3a)

- [x] Refusals, the exact-name line, the reported lookups and downloads; downloads by public link with `curl` alone;
      its tests pass, on both layouts of GitHub's JSON; `star lint shell` clean

### Phase 4: install.ps1 (Requirements 1 to 4)

- [x] Refusals, the exact-name line and assets, the failures reported by name; its tests pass under 5.1 and 7; the
      PowerShell gate clean

### Phase 5: The documents (Requirements 5, 7, 8)

- [x] The guide's by-hand lines and their test; #947's plan; the headers and help

### Phase 6: Verify

- [x] `make test`; CI's quality gate; the Installers workflow green on all seven legs and both editions (the last
      two proven by the PR script's gate, which merges only once every check has passed)

### Phase 7: Merge

- [x] PR script written, shown, and handed over; its `git add` list proven with `git add --dry-run`. The PR resolves
      #1002 and #1008
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
- **D2. The installers don't check GitHub's digest.** The issue doesn't ask for it. GitHub computes the digest itself
  from whatever bytes the release holds, and recomputes it when an asset is replaced, so it can't catch a swapped
  archive; the checksums file, which our build writes (`Makefile:610`), can. In bash it would also mean parsing JSON
  with `grep`. (Reason corrected 2026-10-02 by ruling: it first said the two come from the same release job, which
  they don't.)
- **D3. No third tool for `install.sh`.** With neither `sha256sum` nor `shasum`, it fails and says so; `openssl` is
  not tried. Every platform the installers serve has one of the two today.
- **D4. `install.ps1`'s asset lookups become case-sensitive,** found while reading the same lines; the published
  names never vary in case, so nothing that works today stops working.
- **D5. No test hook in the installers.** The tests stand in for GitHub from outside: a fake `curl` for bash, and
  functions shadowing the web cmdlets for PowerShell.
