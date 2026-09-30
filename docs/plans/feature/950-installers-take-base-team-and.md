---
title: "The installers take --base, --team and --personal and never ask; install.ps1 never exits the user's session; the guide and the README show the one command"
issue: https://github.com/NobleFactor/devlore-cli/issues/950
status: active
created: 2026-09-25
updated: 2026-09-30
---

# Plan: PR B, the installers' layer flags, their exits, and their documentation

Lanes 15, 16 and 17 of #949, PR B of four (ruled 2026-09-29: "We'll four PRs: A, B, #959, and then C."). One plan
for three issues, #950, #965 and #946; each carries its `## Issue NNN` section. The first draft of #950's part
was quoted on #950 when its first branch was deleted
([comment](https://github.com/NobleFactor/devlore-cli/issues/950#issuecomment-5857023361)). This plan
supersedes it, at `f0d21c49`.

## Issue 950

Setting up a machine takes, after the installer, three `writ repo set` lines typed from memory. Ruling 2 of #798:
each installer registers the layers it is given. Its prompting half was reversed on 2026-09-26: "let's go with
docker and claude's approach. verbatim. the shell script never prompts. anything interactives belongs to the
downloaded binary"
([#798](https://github.com/NobleFactor/devlore-cli/issues/798#issuecomment-5856172826)). Failures, the same
day: "the errors are the errors. period. if they fail they fail". There's no rollback, because every operation is
idempotent. Today `install.sh` knows `--prefix` alone, and `install.ps1` knows `-Prefix` and `-Help`.

## Issue 965

`install.ps1` ends with `exit` in two places: `-Help` (`:53`) and every failure, through `Write-Fatal` (`:138`).
Under `irm … | iex` and under the preferred script-block form, the script isn't a file, so `exit` closes the
user's PowerShell window and takes the error message with it.

## Issue 946

`docs/guides/getting-started.md` § Install (`:24-39`) tells a reader to download `releases/latest/download/writ-darwin-arm64`
and similar files. No release publishes those, and `releases/latest` answers 404 while every release is a
pre-release. `README.md` § Install (`:29-43`) offers `go install …@latest`, which builds lore and writ without
star and without the version stamp, and `./install.sh` from a checkout. Neither page shows the one command or
the three flags.

## Requirements

### Requirement 1: The flags (#950)

| bash | PowerShell | environment | value |
| --- | --- | --- | --- |
| `--base=<loc>` | `-Base <loc>` | `DEVLORE_BASE` | a working-tree root or a repository URL, `writ repo set`'s grammar, passed through as given |
| `--team=<loc>` | `-Team <loc>` | `DEVLORE_TEAM` | same |
| `--personal=<loc>` | `-Personal <loc>` | `DEVLORE_PERSONAL` | same |

- **Environment variables.** Each flag has one, because `irm … | iex` takes no parameters; a flag wins over its
  variable.
- **SSH URLs.** An SSH URL clones over SSH (#950).
- **PowerShell editions.** `install.ps1` keeps one code path for Windows PowerShell 5.1 and PowerShell 7 (ruled
  2026-09-24).
- **Resolution.** The flag-over-variable choice is made at script scope, beside each script's configuration
  (`install.sh:55-61`, `install.ps1:60-69`).

### Requirement 2: No questions (#950)

- **Nothing reads from the terminal.** Neither script uses `read`, `Read-Host`, `/dev/tty` or `stty`. The only
  terminal test is `install.sh:72`'s `[[ -t 1 ]]` for color.
- **A layer not given is skipped.** The summary's last lines name each skipped layer and the command that
  registers it, in ASCII (the gate forbids anything else in `install.ps1`):

  ```text
  skipped: team; to register it later:
    writ repo set team <working-tree-root>|<repository-url>
  ```

- **Terminal or not, the output is the same.**
- **Git's own prompts are writ's.** When writ clones a URL layer, ssh's host-key question or git's credential
  prompt can reach a terminal. That's the downloaded binary's interaction, which the ruling leaves to it; the
  installer adds none.

### Requirement 3: Registration (#950)

- **When and how.** After the products install, for each layer given, base first (writ's order,
  `cmd/writ/writ/layer.go:18`), the installer runs `<prefix>/bin/writ repo set <layer> <loc>`: the writ it just
  installed, by path (`writ.exe` on Windows). The call runs in the user's working directory, so a relative
  `<loc>` resolves where the user typed it. The call is `writ repo set <layer> <loc> --unattended`: the installer runs
  writ with nobody there to answer, and `--unattended` is writ's contract for that. No code reads the flag yet;
  when a writ command gains a question, it takes its default instead of asking. writ's output is left as writ prints
  it, and the installer's summary comes after it. Ruled 2026-09-30: "put the summary last. let writ's output be."
  and, of `--unattended`, "yes".
- **Failure.** writ's own error ends the script, as a failed `self install` does (`install.sh:325`;
  `install.ps1:389-391`). There's no rollback: the recovery is the same command again (#791 makes a rerun
  report `unchanged`).
- **A layer without writ.** A layer given while `DEVLORE_TOOLS` excludes writ is an error before any network
  call. That's before `install.sh:228` and `install.ps1:286`, the first calls when `DEVLORE_VERSION` is set.

### Requirement 4: Long options (#950)

`set -euo pipefail` (`install.sh:30`) becomes `set -o errexit -o nounset -o pipefail`. Every other option
becomes its long form wherever every platform the script runs on accepts it (ruled 2026-09-26). Same behavior,
spelled out:

| Tool | Lines at `f0d21c49` | Form |
| --- | --- | --- |
| `curl` | `install.sh:120`, `:122`, `:174`, `:176` | `--silent --show-error --location --header --output` |
| `wget` | `install.sh:126`, `:128`, `:180`, `:182` | `--quiet --output-document --header` |
| `tar` | `install.sh:304`; `install.ps1:359` (macOS and Linux only) | `--extract --gzip --file --directory` (bsdtar's option table has all four) |
| `grep` | `install.sh:144`, `:161`, `:246`, `:248` | `--only-matching --before-context=5 --quiet` (FreeBSD's and Apple's grep sources have all three) |
| `head -1` | `install.sh:144`, `:161` | `awk 'NR == 1'`: Apple's `head` gained `--lines` only recently, and `awk` needs no option |
| `shasum -a 256` | `install.sh:198` | `shasum --algorithm 256` (Perl's `shasum`, on both platforms) |
| `uname`, `mkdir -p`, `rm -rf`, `mktemp -d`, `unzip -q -d` | | stay short: BSD and Info-ZIP have no long forms |

- **`--fail`.** The one-liners printed in the headers and the documents get `--fail`, which stops an HTML error page
  from being piped into bash. The four calls inside the script don't: adding it there changes how a GitHub
  API error is reported, and this requirement is "same behavior".
- **`\s`.** `install.sh:248`'s `sed 's/.*:\s*…'` uses GNU's `\s`, which BSD sed doesn't know. It becomes
  `[[:space:]]`, as `:144` and `:161` already use.

### Requirement 5: Arguments, the summary and the headers (#950)

- **Unknown arguments.** `install.sh` refuses an argument it doesn't know, with the usage and exit 1. Today a
  typo such as `--bsae=…` is dropped silently, and the layer is reported skipped. PowerShell's binder already
  refuses unknown parameters.
- **Next steps.** They stop suggesting `writ adopt` and `writ migrate`: `writ deploy` if a layer was registered,
  and the skipped lines, which come last.
- **The PATH advice names the directory installed into** (found in Phase 4). `install.sh` told every user to add
  `$HOME/.local/bin`, whatever `--prefix` said; it now prints the install directory, written from `$HOME` when it's
  under it. `install.ps1` already printed the install directory.
- **The standard script headers** (ruled 2026-09-30: "why are you not using my standard script headers?"). The
  header is the shebang, SPDX, the copyright, and `<name> - <what it does>`; usage never sits in comments.
  - `install.sh`: `# install.sh - Install lore, star and writ, and register the layers given`, then
    `# for documentation: install.sh --help`. `--help` prints the usage: the flags with their variables,
    `DEVLORE_VERSION`, `DEVLORE_TOOLS` and `GH_TOKEN`, and the one-liner.
  - `install.ps1`: the style guide's comment-based help. `.SYNOPSIS`; `.DESCRIPTION`, which says it runs on
    Windows PowerShell 5.1 and PowerShell 7; `.PARAMETER` for `Base`, `Team`, `Personal` and `Prefix`, each with its
    variable; and `.EXAMPLE` in the ruled order (#946, 2026-09-27): the script-block form, then `irm … | iex` with
    the variables. `iex "& { $(irm …) } …"` is not shown. `-Help` prints the same.
  - Both name the installers at the develop site, with the sentence "served by the DevLore site's develop
    environment, from which devlore is released today" (ruled 2026-09-30).
- **The license is `Apache-2.0` on both.** `install.ps1` declares `MIT` today, unlike the rest of devlore-cli (ruled
  2026-09-30: "yes, Apache-2.0").
- **The `.\install.ps1 -Prefix "C:\devlore"` example** (`:7`) goes; it contradicts the `~/.local` default the
  script argues for.
- **The last line of the summary before the skipped layers,** `Documentation: https://devlore.noblefactor.com`,
  becomes `Documentation: https://github.com/NobleFactor/devlore-cli#readme`: the README is public, and the site's
  pages need a sign-in (ruled 2026-09-30: "yes").
- **`GH_TOKEN`** (ruled 2026-09-30: "we read it and put it into the request headers, if it's set"):
  - The installers keep reading it and, when it's set, send it as the `Authorization` header on their GitHub API
    requests. It lifts GitHub's limit of 60 anonymous API requests an hour.
  - The warnings "No GH_TOKEN set. This will fail for private repositories." go; the repository is public.
  - The error messages that say a private repository needs a token report GitHub's own error instead.
  - `--help`, `.DESCRIPTION` and the guide mention it once.

### Requirement 6: `install.ps1` never calls `exit` (#965)

- **`-Help` returns** (`:53`). Run as a file, it still exits 0.
- **`Write-Fatal` throws.** It ends with `throw $Message`, a terminating error. That's the issue's word and the
  organization's style guide, § 6 ("Throw an actionable message"). It stays a terminating error here because
  `install.ps1:48` pins `$ErrorActionPreference = 'Stop'`. `Write-Fatal` keeps its name and signature, and its
  eight call sites are unchanged. The `finally` at `:446-449` still removes the download directory.
- **Exit codes don't change.** Run as a file (`pwsh -File`, `powershell -File`), a failure still exits 1: an
  uncaught terminating error.
- **The comments tell the truth.** The comment at `:132-136` says why there's no `exit`. The claim at `:77-81`
  that `2>` captures errors is corrected: a process's stderr captures them (`pwsh -File install.ps1 2> err.txt`);
  an in-session `2>` on the one-liner doesn't.
- **Native exit codes.** `install.ps1:359` (`tar`) and `:383` (`chmod`) gain the `$LASTEXITCODE` check the style
  guide requires after every native command. Today a failed extraction runs on to "No binaries found".
- **A caller's redirection doesn't end the run** (found by Requirement 12's review, 2026-09-30). Windows PowerShell
  5.1 turns each stderr line of a native command into an error record when a caller redirects the error stream
  (`*>&1 | Tee-Object`), and the script's `'Stop'` made the first one fatal: lore, star and writ narrate on stderr,
  so a user who logged the install lost it at the first progress line. The four native calls (`tar`, `chmod`,
  `self install`, `repo set`) run through `Invoke-NativeCommand`, which holds `'Continue'` for their duration;
  `$LASTEXITCODE` still decides. PowerShell 7.2 and later leave native stderr alone.

### Requirement 7: The gates (#950, #965)

- **PowerShell.** `install.ps1` passes the organization's PowerShell gate with no new suppression:
  - no `Write-Host`;
  - output through `Write-Info`, `Write-Success`, `Write-Warn` and `Write-Fatal`;
  - `[CmdletBinding()]` and `param()` on every function, and `[OutputType]` where one returns;
  - ASCII only;
  - the shebang on line 1.
- **Shell.** `install.sh` passes `star lint shell`.

### Requirement 8: The guide and the README (#946)

- **Both lead with the one command,** bash first, then PowerShell:
  - bash: `curl --fail --silent --show-error --location <site>/install.sh | bash -s -- --base=<path-or-url> --team=<path-or-url> --personal=<path-or-url>`
  - PowerShell, marked preferred (ruled 2026-09-27): the script-block form with `-Base`, `-Team` and `-Personal`,
    across lines with backticks.
  - PowerShell, the alternative: `irm <site>/install.ps1 | iex` with `$env:DEVLORE_BASE` and its siblings. It
    leaves the script's settings in the session, where the script-block form leaves none.
- **A table of the flags and their variables,** as `--help` prints them at this PR's head:
  - each flag is optional;
  - a layer not given is skipped and named with its command;
  - the installer never asks;
  - running it again is safe and is the recovery (ruled 2026-09-26).
- **What the command does.** It installs lore, star and writ into `~/.local` on every platform, each through its
  own `self install`. No separate `self install` step follows (`getting-started.md:41-46` and `README.md:40-42`
  go).
- **By hand, underneath, in the guide:**
  1. Download the platform's archive and the checksums file from
     `https://github.com/NobleFactor/devlore-cli/releases/download/<tag>/`.
  2. Verify the archive against the checksums file.
  3. Extract it, and move the products into `bin/` beside `share/`, where star finds its extensions.
  4. Run each product's `self install`.
  5. Run `writ repo set` for each layer.
- **The README shows the one command and links the guide in the repository for the rest.** The README is public
  on GitHub; the site's pages need a sign-in.
- **What goes:** every `releases/latest` address, every `writ-<os>-<arch>` asset, the `chmod` and `sudo mv` into
  `/usr/local/bin`, and the README's `go install` lines, which build lore and writ without star or the version
  stamp. `README.md` § Building remains the from-source route.
- **Commands use long options** (Requirement 4's table). The PowerShell lines use full names, except the ruled
  `irm`, `iex` and `&`.
- **`scripts/Test-GuideFrontmatter.sh` passes.**

### Requirement 9: Found on the way, filed

- **`irm | iex` leaves the script's settings in the user's session:** `$ErrorActionPreference = 'Stop'`, strict
  mode, and the script's functions and variables. Only the script-block form avoids it. That's a design change to
  `install.ps1`, so it's filed as #982; the documents note it beside the `iex` form (Requirement 8).
- **`writ repo set` refuses a URL layer whose clone directory exists** but isn't that layer's registration
  (`repo_cmd.go:520-522`). A rerun after a clone that landed but wasn't registered fails. That's against the
  idempotency ruling, so it's filed as #983.

### Requirement 10: PR A's owed tick

`docs/plans/fix/962-release-report-states-where-the.md`: its last box, the merge's release run showing the new
summary, is ticked (run 36641566259, `v0.1.0-dev.20260929224715`; the owner read it on 2026-09-30), and the plan
is set `complete`.

### Requirement 11: Stale install text, folded in

Ruled 2026-09-30: "fold them in".

- **`docs/guides/getting-started.md`:** it names three tools, not two (`:11`, `:19`). Its "Initialize a
  repository", "Create your first project" and "Deploy the project" stay as `develop` has them (ruled 2026-09-30:
  "yes, take them out"). Phase 4 ran them in a scratch account, and a new user can't get through them:
  - moving `~/.config/git/config` into the project takes git's name and email with it, so the commit that
    `writ deploy` requires fails;
  - copying `~/.zshrc` leaves the original, and `writ deploy` refuses to replace it;
  - `writ adopt`, the product's way to bring a file under writ, refuses on a machine where nothing has been deployed
    ("no current deployment to write into"), and a first `writ deploy` of a new layer refuses too ("no layer
    configured").

  That's the first-use scenario, which belongs to #894's replan (Out of Scope).
- **`README.md`:** the `devlore.org` link, a parked domain, goes (`:98-99`). The Noble Factor link, found in Phase
  4, points at https://github.com/NobleFactor instead of `https://noblefactor.com`, a registered domain with no
  address (ruled 2026-09-30: "point it at the github organization.").
- **`wiki/Releasing.md`:** it says the release copies both installers (since #948), and drops `DEVLORE_DOWNLOAD_BASE`,
  which the installer never reads.
- **Both installers:** the `armv7` mapping goes (`install.sh:109`, `install.ps1:175`), because no `armv7` archive is
  published. An `armv7` machine is refused as unsupported, not failed with "Asset not found".

### Requirement 12: The installers tested in CI (#950, #965)

Ruled 2026-09-30: "we should test this in ci where we have more vanilla systems", and "yes, fold it into PR B". It
replaces the owner's runs on the owner's machines, which carry GNU tools on macOS and PowerShell 7 on Windows.

- **`.github/workflows/installers.yaml`,** its own file: `ci.yaml` is where the lint work lives. It runs on pull
  requests and pushes to `develop`, `main` and `release/*` that touch `install.sh`, `install.ps1`, the test scripts,
  the workflow itself or `release.yaml`, on the runners `ci.yaml`'s scenario matrix uses. `GH_TOKEN` is the job's
  token.
- **The tests are files, runnable by hand:** `scripts/Test-InstallScript.sh` for `install.sh` and
  `scripts/Test-InstallScript.ps1` for `install.ps1`. Every run uses scratch `HOME`, XDG homes, temp and prefix.
- **`install.sh`,** on `macos-latest`, `macos-15-intel`, `ubuntu-latest` and `ubuntu-24.04-arm`, through a pipe. On
  macOS the installer runs under `/bin/bash` with `PATH=/usr/bin:/bin:/usr/sbin:/sbin`: bash 3.2, bsdtar, BSD grep
  and sed, and `shasum`.
  - `--personal=<the checkout>` and `--team=https://github.com/NobleFactor/noblefactor-ops.git`, twice: both
    registered, the second run `unchanged`, the skipped base last;
  - no flags: the three skipped layers last, exit 0;
  - `DEVLORE_TOOLS=lore --base=…`, and an unknown argument: exit 1;
  - the guide's checksum line against the release installed (`sha256sum --check`; on macOS
    `shasum --algorithm 256 --check`).
- **`install.ps1`,** on `windows-latest` and `windows-11-arm`, under Windows PowerShell 5.1 and PowerShell 7:
  - the script-block form with `-Personal` and `-Team`, twice: both registered, the state unchanged;
  - `-Help` prints the usage and returns;
  - a forced failure throws, and the next statement runs;
  - `irm | iex` with `$env:DEVLORE_BASE`, in a child session;
  - with the caller's streams redirected (`*>&1 | Tee-Object`), the run finishes;
  - run as a file, a failure exits 1 and `-Help` exits 0.
- **Both scripts pass the gates:** `star lint shell`, and the PowerShell gate.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The installers (#950, #965)

- [x] Requirements 1–5 in `install.sh` and `install.ps1`
- [x] Requirement 6 in `install.ps1`
- [x] Requirement 9's two issues filed
- [x] Requirement 11's installer part: the `armv7` mapping goes

### Phase 3: The documents (#946)

- [x] Requirement 8, with every flag and line copied from the installers at this PR's head
- [x] Requirement 11's documents: `getting-started.md`, `README.md`, `wiki/Releasing.md`
- [x] Requirement 10: PR A's plan ticked and `complete`

### Phase 4: Verify

Every run uses scratch directories for `HOME`, the XDG homes, `TMPDIR` and the prefix. The assertion is `writ repo
list` in that environment. Every run that registers is made twice, and the second must equal the first, with
writ's lines saying `unchanged`. Before and after, this machine's `~/.config/devlore` and
`~/.local/share/devlore/writ/layers` are diffed to prove them untouched.

- [x] bash, flags, through the pipe (`cat install.sh | bash -s -- --base=… --team=… --personal=…`) with paths:
      three registered
- [x] bash, a URL layer (`--team=https://github.com/NobleFactor/devlore-cli.git`): cloned into scratch, then
      `unchanged`
- [x] bash, `DEVLORE_BASE` alone: base registered, two skipped. With `DEVLORE_BASE=<a>` and `--base=<b>`, the flag
      wins
- [x] bash, no flags, once with a terminal and once `< /dev/null`: identical output once color is stripped (color
      is on for a terminal alone, by design), three skipped lines last, exit 0
- [x] bash, `DEVLORE_TOOLS=lore --base=…`: refused with exit 1 before any network call; an unknown argument: refused
      with the usage and exit 1
- [x] pwsh 7 on this host, from the tree (`Get-Content -Raw`) and from a local server (`irm`):
      - the script-block form with `-Base`, `-Team` and `-Personal`;
      - `iex` with `$env:DEVLORE_BASE`;
      - `-Help` under the script-block form returns and the session survives;
      - a forced failure (`$env:DEVLORE_VERSION = 'v0.0.0-no-such-tag'`) leaves the session open with the message;
      - run as a file, the failure exits 1 and `-Help` exits 0.
      Control: the same failure against `f0d21c49`'s copy ends the session
- [x] No `exit` in `install.ps1`, by the parser, not by text: 0 `ExitStatementAst` nodes
- [x] The gates: `star lint shell .`; the PowerShell gate reports 0 findings in `install.ps1`; ASCII only;
      `Test-GuideFrontmatter.sh`
- [x] The documents: every address answers 200. The by-hand steps, run as written into a scratch prefix on this
      host, install a working writ, and star finds its extensions. Nothing stale remains
      (`releases/latest`, `writ-darwin-`, `devlore.noblefactor.com`, `/usr/local/bin`)
- The owner's runs on a Mac and on DANOBLE-WD11-3 are replaced by Requirement 12's CI jobs (ruled 2026-09-30).

### Phase 5: The installers in CI

- [x] `scripts/Test-InstallScript.sh` and `scripts/Test-InstallScript.ps1` written, and passing on this host (bash;
      PowerShell 7)
- [x] `.github/workflows/installers.yaml` written; it parses
- [x] The gates pass on the three new files
- [ ] On the pull request: every installers job passes, on all six runners and under both PowerShell editions

### Phase 6: Merge

- [x] PR script written, shown, and handed over. The PR resolves #950, #965 and #946
- [ ] After the merge and the site's sync: the site's two installers equal `develop`'s, and the documented bash line,
      run as printed in scratch, registers its layers

## Out of Scope

- **`self upgrade`** (#947, PR C) and **the distribution fix** (#959, the PR before it).
- **Linking a PR and its release** (not requested).
- **The first-use scenario** (ruled 2026-09-30): "I should be able to set my repos up and start adopting right away.
  we might also allow 'empty' deployments." `writ adopt` and a first `writ deploy` on a fresh machine, and the
  guide's repository and deploy sections that walk a new user through them, go to the replan of #894 ("writ works
  on a machine that has never seen it"), which today leaves adopt out.

## Open Questions

All five were answered on 2026-09-30, one at a time, and are folded into the requirements above:
- [x] **1. The headers and the address.** The standard script headers, usage in `--help` and the comment-based help,
      the develop site named there with its sentence, and `Apache-2.0` (Requirement 5).
- [x] **2. The `Documentation:` line.** The README on GitHub (Requirement 5).
- [x] **3. `GH_TOKEN`.** Read, and sent in the request headers when set; the false warnings go (Requirement 5).
- [x] **4. How the installer calls writ.** `writ repo set <layer> <loc> --unattended`, writ's output as is, the
      summary last (Requirement 3).
- [x] **5. Stale install text.** Folded in (Requirement 11).
