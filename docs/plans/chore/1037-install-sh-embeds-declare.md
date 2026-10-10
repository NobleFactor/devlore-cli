---
title: "install.sh narrates and traps as Declare-BashScript does and takes both option forms; two repository tools embed the helper"
issue: https://github.com/NobleFactor/devlore-cli/issues/1037
status: draft
created: 2026-10-09
updated: 2026-10-09
---

# Plan: install.sh and two repository tools on Declare-BashScript

## Issues 1037 and 1038

Lanes 62 and 64 of #916, one pull request, after lane 63's (NobleFactor/noblefactor-ops#293, merged as c942f54). The
owner's rulings:

- #1037, 2026-10-08. Of `install.sh`: "We have a DevLore-cli install.sh script that could embed Declare-BashScript but
  should not source it." Offered the five bash files that trap without the helper: "All five now".
- #1037, 2026-10-09. Of an argument parser in the helper: "I do not want to maintain an argument parser without a
  drop-dead good reason and I'm not sure that macOS is a good enough reason." Then: "The update to install.sh will be
  two things: updated narration methods that match Declare-BashScript; updated arg processing to allow space or
  equal." Asked whether `install.sh`'s trap, and the two tools, still move onto the helper's handler: "Add the helper's
  handler to the list of changes."
- #1038, 2026-10-09. Offered `install.sh`'s loop learning the space form for its four options, a few lines that run on
  bash 3.2 and no general parser, or leaving it: "1".
- The helper, 2026-10-09: "we stop if we have the wrong version of bash. I'm either in or out based on the bash version
  number." Then: "and if any other tool we require is missing. same deal." NobleFactor/noblefactor-ops#290 made both
  the helper's.
- `install.sh`, 2026-10-09, after this plan's draft offered it a newer bash: "How many times must I say that install.sh
  MUST run with what ships with macOS?!"
- `install.sh`'s exit statuses, 2026-10-09. Offered the helper's codes by cause, or 1 everywhere as today: "Use the
  codes you suggest in 1".
- golangci-lint, 2026-10-09. MacPorts had moved this Mac to v2.14.0 against the pin, v2.13.2: "Let the commit script
  install 2.13.2 for all uses on this machine." v2.13.2 then could not read this Mac's Go 1.27.2; told that v2.14.0
  ships for every platform and that CI's install fetches it: "It seems it must be time to update to 2.14 everywhere".

## Goals

1. `install.sh` narrates with the helper's `note`, `success` and `error`, and traps through its handler, and runs with
   what ships with macOS: its bash 3.2 and its own tools.
2. `install.sh` takes `--prefix`, `--base`, `--team` and `--personal` as `--option value` and as `--option=value`, and
   refuses by name an option given last with no value.
3. `scripts/Test-InstallScript.sh` and `packaging/macports/generate-portfile.sh` embed the helper whole, and parse,
   narrate and trap through it.
4. Every embedded copy comes from one pinned commit of the helper, and `make check` fails when one drifts from it.
5. golangci-lint is v2.14.0 everywhere the tree is linted: CI, this Mac, danoble-ud24-1.local and danoble-wd11-3.

## The installers' requirements

Each installer runs with what ships with its operating system, and neither serves the other's platform (#1032). The
command-line design states the same, in `docs/architecture/10-command-line-interface.md`, § 2, The installers.

**`install.sh`, on Linux and macOS:**

1. Runs with what ships with macOS: `/bin/bash` 3.2 and macOS's own tools, bsdtar, BSD grep, sed and awk, shasum and
   curl. Nothing it does needs a newer bash, a GNU tool or GNU `getopt`. The owner, 2026-10-09: "How many times must I
   say that install.sh MUST run with what ships with macOS?!" On Linux it runs with the distribution's bash and core
   tools, and curl, which it names when missing.
2. Runs as `curl ... | bash -s -- <options>`, with nothing beside it: it sources nothing, and carries what it uses of
   Declare-BashScript. The owner, 2026-10-08: "could embed Declare-BashScript but should not source it".
3. Narrates and traps as the helper does, with the helper's `note`, `success`, `error`, handler and `Set-Traps`, and
   none of its checks for bash 5.3 or GNU `getopt` (Requirement 1).
4. Takes `--prefix`, `--base`, `--team` and `--personal` as `--option value` or `--option=value`, in a few lines that
   run on bash 3.2 (Requirement 2).
5. As today: verifies the archive against the release's checksums file before anything is extracted, and installs
   nothing it cannot verify (#1002); installs lore, star and writ through each one's own `self install` (#798, ruling
   1); registers each layer it is given with `writ repo set`; never asks; a flag wins over its variable; and running
   the same command again is the recovery.

**`install.ps1`, on Windows,** which this plan does not change:

1. Runs with what ships with Windows: Windows PowerShell 5.1, and PowerShell 7 alike, in one code path. Ruled
   2026-09-24, reversing #798's third ruling, which had it install PowerShell 7 first; as
   `docs/plans/feature/948-install-ps1-is-published-beside.md` records it, the installer uses what is known to be on
   the box, Windows PowerShell 5.1, and installs nothing but devlore.
2. Runs as `irm ... | iex`, or as a script block made from it, with nothing beside it.
3. As `install.sh`: verifies the archive before anything is extracted, installs through `self install`, registers the
   layers it is given, and never asks.

## Current State

Measured 2026-10-09 on Danoble-MBP-A.

| What | Today |
| --- | --- |
| `install.sh` | 537 lines. Narrates with its own `info`, `success` and `warn` on stdout, and `error` on stderr, which exits 1. Traps `cleanup` on EXIT (line 77). Matches `--prefix=*`, `--base=*`, `--team=*` and `--personal=*` only (lines 50 to 67). Parses and runs under macOS's `/bin/bash` 3.2.57. |
| The helper | NobleFactor/noblefactor-ops c942f54, 367 lines. Stops with 78 on a bash older than 5.3 and on a `getopt` that isn't GNU's; then sets `errexit`, `errtrace`, `nounset` and `pipefail`, turns on `inherit_errexit`, and parses its consumer's options with GNU `getopt`. Its `error`, `note`, `success`, `Set-Traps` and `on_error_or_interrupt` use nothing newer than bash 3.0, and read `script_name`, `Heavy_ballot` and `Heavy_check_mark`. |
| Its handler under bash 3.2 | Run with the helper's own five functions and a failure inside a function: bash 3.2.57 stops with status 1 and reports `at line 43: ((BASH_SUBSHELL == 0))`, a command of the handler's own; bash 5.3.20 reports `at line 44: false`. |
| `scripts/Test-InstallScript.sh` | 903 lines, written to run on bash 3.2. Runs `install.sh` as `curl ... \| bash`, on macOS under `/bin/bash` with macOS's own tools, reading its stdout and stderr together (line 152). Expects every refusal to exit 1 (lines 229, 234, 250 and 690) and matches `error: unknown argument`. Traps `rm -rf "$scratch"` (line 67). |
| Its CI | `installers.yaml` runs it on `macos-latest`, `macos-15-intel`, `ubuntu-latest` and `ubuntu-24.04-arm`, and on `macos-latest` with MacPorts' GNU tools first on PATH. Which bash each runner finds first, and which Ubuntu `ubuntu-latest` is today, are not verified; Ubuntu 24.04 ships bash 5.2. |
| golangci-lint | Pinned at v2.13.2 (`Makefile:479`), which CI installs; CI's last green lint, 2026-10-05, ran it on Go 1.27.1. This Mac: Go 1.27.2 from MacPorts; MacPorts' golangci-lint v2.14.0 was uninstalled and v2.13.2 put in `/usr/local/bin` on 2026-10-09, and v2.13.2 cannot read Go 1.27.2's standard library ("export data version 5 is greater than maximum supported version 4"). danoble-ud24-1.local: v2.13.2 in `~/.local/bin`, Go 1.26.0. danoble-wd11-3: v2.13.2 from WinGet (`GolangCI.golangci-lint`, 2.14.0 available), Go 1.27.0. v2.14.0, released 2026-09-24, ships for macOS, Linux and Windows on amd64 and arm64, and moves `golang.org/x/tools` from 0.49.0 to 0.50.0. |
| `packaging/macports/generate-portfile.sh` | 48 lines, `set -euo pipefail`, traps `rm -f "$TMPFILE"`. GoReleaser's after hook (`.goreleaser.yaml:17`): it writes the MacPorts Portfile for each release GoReleaser makes. GoReleaser is staged for the releases to come and kept correct by `verify-ldflags`; until then `release.yaml` builds with `make dist` (`Makefile:522`). |

## Requirements

### Requirement 1: install.sh narrates and traps as the helper does (#1037)

`install.sh` carries the helper's `error`, `note`, `success`, `on_error_or_interrupt` and `Set-Traps`, with the `EX_`
statuses and the two markers they read, exactly as the helper writes them, and sets `script_name=install.sh`: under
`curl ... | bash`, its `$0` is `bash`. It does not carry the helper's version and `getopt` checks, its
`inherit_errexit` or its parser: it runs with what ships with macOS. It sets `errexit`, `errtrace`, `nounset` and
`pipefail`, and states its cleanup with `Set-Traps cleanup`. On bash 3.2 the handler still stops it with the failing
status, but its message names one of the handler's own commands and lines (Current State).

`info` becomes `note`, `success` stays, `warn` becomes `error 0`, and `error` becomes `error` with the helper's status
for its cause, as ruled:

| Status | When `install.sh` stops |
| --- | --- |
| 64 `EX_USAGE` | an unknown option; an option given last with no value; a layer given while `DEVLORE_TOOLS` leaves out writ |
| 65 `EX_DATAERR` | the checksums file has no line for the archive; a checksum mismatch; an archive with no program `DEVLORE_TOOLS` names |
| 69 `EX_UNAVAILABLE` | curl missing; neither sha256sum nor shasum; GitHub refuses a request, or has no such release or none at all |
| 70 `EX_SOFTWARE` | a program's `self install` fails |
| 75 `EX_TEMPFAIL` | GitHub's rate limit is used up; the message says when to run again |
| 78 `EX_CONFIG` | an operating system other than Linux and macOS; an architecture other than amd64 and arm64 | The narration moves to stderr in the helper's form, `[install.sh] [+] ...`, colored whether or not stderr
is a terminal, as the helper's is; the PATH advice and the next steps stay plain lines on stdout. A message that relied
on `echo -e` to break its lines is written as separate lines.

### Requirement 2: both option forms (#1038)

The loop takes each of the four as `--option value` or `--option=value`. An option given last with no value stops with
`error` and the usage status, naming it: `--prefix needs a value`. `-h` and `--help` print the usage, which shows both
forms. A few lines that run on bash 3.2, and no general parser.

### Requirement 3: the two tools embed the helper whole (#1037)

Each carries the helper's text verbatim in a here-document that it sources with its own options:
`source /dev/stdin "$0" "<long options>" "<short options>" "$@" <<'DECLARE_BASHSCRIPT'`. Each parses with the helper's
loop, narrates with `note`, `success` and `error`, and states its cleanup with `Set-Traps`. `Test-InstallScript.sh`
still runs `install.sh` under `/bin/bash` and macOS's own tools on macOS; the suite itself then needs bash 5.3 and GNU
`getopt`, and so does `generate-portfile.sh` wherever GoReleaser runs it (Open Question 1). Sourcing a here-document
is proven on bash 5.3 before either file changes; if it fails, the text goes inline after a `set --` that gives the
helper the same arguments.

### Requirement 4: one pin, one generator, one check (#1037)

- **The pin.** `DECLARE_BASHSCRIPT_COMMIT` in the `Makefile`, today `c942f54985c06f4ce767e872bbe389e967df9bba`. It is
  the one fact; there is no vendored copy to keep beside it.
- **The generator.** `make embed-helper` fetches the helper at the pin from its public URL and writes it into the two
  tools' here-documents, and its five functions, statuses and markers into `install.sh`, between marker lines that
  name the commit.
- **The check.** `make check-embedded-helper`, which `make check` runs, generates the same into a scratch tree and
  fails on any difference, naming the file. It needs the network, as CI has.

### Requirement 5: tests and documents

- `Test-InstallScript.sh` proves both forms and the value-less refusal for each option, the new narration, and each
  status in Requirement 1's table, still running `install.sh` under macOS's bash 3.2.
- `install.sh`'s usage, and every document that shows its options or quotes its messages, say what it does now.
- The command-line design's § 2 states both installers' requirements, as the section above does.

### Requirement 6: golangci-lint v2.14.0 everywhere (part of #1020)

The `Makefile`'s pin moves to v2.14.0, and with it CI, which installs the pinned version. This Mac takes v2.14.0 in
`/usr/local/bin`, danoble-ud24-1.local in `~/.local/bin`, each with golangci-lint's own installer at the pinned tag,
and danoble-wd11-3 through WinGet; the owner, of this Mac's: "/usr/local/bin is good". The whole tree lints clean on
it under Go 1.27.2. Its gocritic flags two formats in `cmd/writ/writ/migrate/session.go` (lines 166 and 562) that
wrap a path in back quotes by hand, `` "`%s`" ``; each takes `%#q`, which prints the same back-quoted path, and a
double-quoted one when the path itself holds a back quote. No test pins either string. Lane 35's monthly chore,
#1020, carries the next bump; this one is part of it, and #1020 stays open.

## Implementation Phases

### Phase 1: The plan

- [ ] This plan, committed on `chore/1037-install-sh-embeds-declare` and reviewed with the owner, its questions
  settled. star's last known good build was taken when the worktree opened, 2026-10-09 (`build/star.lkg`).

### Phase 2: golangci-lint v2.14.0 everywhere (Requirement 6)

- [ ] The pin moved and committed with this plan, the tree clean under it in `make check` on this Mac, and v2.14.0 on
  this Mac, danoble-ud24-1.local and danoble-wd11-3.

### Phase 3: The generator and the check (Requirement 4)

- [ ] `make embed-helper` and `make check-embedded-helper`, with `make check` calling the second; the here-document
  form proven first.

### Phase 4: The two tools (Requirement 3)

- [ ] Both embed the helper and move onto its parsing, narration and traps, with the CI Open Question 1 settles.

### Phase 5: install.sh (Requirements 1 and 2)

- [ ] The narration, the trap and both option forms, run under `/bin/bash` 3.2.57 and bash 5.3.

### Phase 6: The suite and the documents (Requirement 5)

- [ ] The suite's new cases, and the documents.

### Phase 7: Verification

- [ ] `make check` clean on this Mac, and CI green on the pull request, every installers leg.
- [ ] The suite on this Mac, with macOS's own tools and with MacPorts' (`--keep-path`), and on danoble-ud24-1.local.
- [ ] This branch's `install.sh`, piped into bash on danoble-ud24-1.local under a scratch HOME, installs the newest
  release from GitHub and registers nothing.

### Phase 8: Acceptance and closure

- [ ] The pull request, the merge and `git close-branch`; lanes 62 and 64 marked on #916, and #1037 and #1038 closed;
  #1020 records this bump, and lane 35's row says the next one is the monthly chore's.

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `install.sh` | Modify | Requirements 1 and 2 |
| `scripts/Test-InstallScript.sh` | Modify | Requirements 3 and 5 |
| `packaging/macports/generate-portfile.sh` | Modify | Requirement 3 |
| `Makefile` | Modify | Requirements 4 and 6 |
| `cmd/writ/writ/migrate/session.go` | Modify | Requirement 6: the two formats v2.14.0 flags |
| `.github/workflows/installers.yaml` | Modify | Open Question 1 |
| `docs/architecture/10-command-line-interface.md` | Modify | The installers' requirements, in the design |
| `docs/plans/chore/1037-install-sh-embeds-declare.md` | Create | This plan |

## Out of Scope

- `install.ps1`, which is PowerShell, and NobleFactor/noblefactor-ops#262's counterpart to the helper.
- The helper's own handler under bash 3.2: the owner ruled the helper in or out by bash version, and it is out there.
- #1037's first Done-when bullet, each of the three embedding the helper as NobleFactor/noblefactor-ops#268 left it,
  holds for the two tools. For `install.sh`, the owner's rulings since name its narration, its handler and its options,
  and require it to run with what ships with macOS; the whole helper would stop it on macOS's bash 3.2.

## Open Questions

1. **The CI the two tools then need.** The suite, and `generate-portfile.sh` wherever GoReleaser runs it, would require
   bash 5.3 and GNU `getopt`. **Recommended:** `installers.yaml`'s Linux legs move to `ubuntu-26.04` and its arm
   runner, whose label is not verified, and its macOS legs install Homebrew's `bash` and `gnu-getopt` for the suite
   alone, while `install.sh` still runs under `/bin/bash` and macOS's own tools there; the job that runs GoReleaser,
   when releases use it, gives `generate-portfile.sh` the same. The cost: `install.sh` is no longer tested on Ubuntu
   24.04.
