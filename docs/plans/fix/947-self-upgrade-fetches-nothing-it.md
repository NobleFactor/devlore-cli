---
title: "self upgrade fetches the newest build on its channel, verifies it, and upgrades every devlore program in the prefix"
issue: https://github.com/NobleFactor/devlore-cli/issues/947
status: approved
created: 2026-09-30
updated: 2026-09-30
---

# Plan: self upgrade fetches, verifies and upgrades the suite

Lane 20 of #949, alone: "PR C, alone, next: large (a new path in every program's `self` command, and Windows' running
`.exe`), with its rulings asked first" (ruled 2026-09-29: "We'll four PRs: A, B, #959, and then C.").

## Issue 947

`writ self upgrade`, and `lore` and `star` alike, fetches nothing. At `42e6e4ed`:

- **It re-installs the running binary over itself.** `newUpgradeCmd` (`cmd/internal/cli/selfinstall.go:124-154`) finds
  the prefix from the running executable (`:142`, `resolveInstalledPrefix` at `:805-829`) and calls the same
  `runSelfInstall` that `self install` does (`:146`). No Go code in the repository speaks to GitHub's releases.
- **It doesn't even rewrite the binary.** `installBinary` (`:850-884`) returns early when the running executable is
  the target (`:871-873`); the comment at `:870` says the opposite. The comparison is by string, so a prefix reached
  through a symbolic link defeats it: `self install` from the installed copy then fails with `openat bin/writ: text
  file busy` (reproduced).
- **Nothing can write over a running binary.** On Linux, `copyFile`'s truncating `Create` (`:1163`) fails with
  `ETXTBSY` while the target runs; renaming a new file over it succeeds (reproduced in Go on kernel 7.0). On Windows,
  rewriting, deleting and renaming over a running `.exe` all fail; only renaming the running `.exe` itself succeeds.
  On macOS, rewriting a signed binary in place gets its next run killed. So every build published to date, run as a
  child `self install`, fails to replace a program that is running.
- **The help promises what the code doesn't do:** "Upgrade <tool> in place" (`:130-139`).
- **Only one test,** that the subcommand exists (`cmd/internal/cli/selfinstall_test.go:103-119`). The scenario covers
  install and uninstall (`cmd/scenario/selfinstall_scenario_test.go:28-79`), and `make test-scenario` runs only
  `TestSelfInstallScenario` (`Makefile:396`).
- **A binary doesn't know its channel.** It is stamped with `Version`, `Commit` and `BuildDate` (`Makefile:94`,
  `pkg/application/version.go:25-31`). The version's suffix hints at where it was built (`-dev.` from develop, `-rc.`
  from main and `release/*`, none from a `v*` tag), but only by convention, and a local build carries a
  `git describe` string or `dev`. The stamp proof (`Makefile:305-326`) compares the version alone.
- **Every release records its branch.** `release.yaml:79-86` writes `Ref: ${{ github.ref }}` into the release body:
  `Ref: refs/heads/develop` on all 408 releases to date, each a develop pre-release with seven assets (six archives
  and `devlore-cli_<tag>_checksums.txt`), each asset carrying GitHub's own `sha256` digest. No regular release exists:
  `/releases/latest` answers 404.
- **Releases are tagged on the default branch.** `gh release create` names no `--target`, so GitHub creates the tag at
  the head of `develop`, the default branch. For a develop build that is the right commit, or close to it; a build
  from `main` or `release/*` would be tagged on develop's head.
- **No program handles a signal.** `main` calls `cmd.Execute()` with no context (`cmd/writ/main.go:28`), and on an
  interrupt Go exits without running deferred calls.

## Rulings

- **All programs together** (ruled 2026-09-30, "1. all programs together"): `self upgrade`, run from any of the
  three, upgrades every devlore program installed in the prefix, to one version.
- **A build keeps its own channel, stamped into the binary explicitly** (ruled 2026-09-30, "2. a build keeps its own
  channel. the channel is stamped into the binary explicitly. we must ensure that i can switch channels easily. a
  possibility is providing configuration as an override."; "3. ... we encode channel in the binary and allow that you
  can change it with configuration").
- **Two channels, `develop` and `release`** (ruled 2026-09-30: "in my mind we have two and main is not one. The two
  are develop and release. we might talk about pre-release channels at some point.").
- **GitHub's `prerelease` flag distinguishes a pre-release** (ruled 2026-09-30: "we will use githubs prerelease flag to
  distinguish prereleases from \"regular releases\"").
- **`release` follows GitHub's `/releases/latest`; `--prerelease`, and the `self.prerelease` setting, add the release
  channel's pre-releases; every develop build is a pre-release; a build from `main` or `release/*` stamps channel
  `release` with pre-releases included** (ruled 2026-09-30, approving that proposal: "I like that. it maps directly to
  what github offers. we should use their word: --prerelease.").
- **Auto-upgrade stays with #83** (ruled 2026-09-30, "we will not worry about auto-upgrade at this point. we leave
  that for #83.").

Earlier rulings this work honours:

- **`self install` replaces its record:** "the manifest must be a record of what the tool owns and self install must
  implicitly uninstall, if it detects a prior install ... this must be true for every app" (#933, 2026-09-23), and
  "It is a manifest, not a trace" (`docs/architecture/10-command-line-interface.md:213-218`). An upgrade is a
  `self install` of a fetched build: #928's lifetimes don't apply.
- **The scripts fetch and delegate:** "running each program's own `self install`, which is the one installer the
  suite has ... The scripts fetch and delegate; they do not re-implement" (#798, Ruling 1). The upgrade fetches, then
  hands each program to its own `self install`.
- **A flag always wins:** "Highest to lowest: flags, then environment variables ... A command must not read
  configuration in a way that overrides an explicitly passed flag"
  (`docs/architecture/10-command-line-interface.md:888-893`).
- **`GH_TOKEN`:** "we read it and put it into the request headers, if it's set" (ruled 2026-09-30,
  `docs/plans/feature/950-installers-take-base-team-and.md:142`).
- **Every command accepts every flag** (`cmd/internal/cli/root.go:95-96`, ruled 2026-09-03), and a child run for its
  output "is captured, always, and never sees the terminal" (`docs/architecture/10-command-line-interface.md:765-771`,
  ruled 2026-09-03).
- **Every operation is idempotent; no rollback; a command's errors are its own.**
- **CI proves the platforms** ("we should test this in ci where we have more vanilla systems", ruled 2026-09-30 for
  PR B).
- **star ships no extensions** (#990, ruled 2026-09-30: "`star self install` harvests exactly nothing"). #990 is the
  other session's; this work neither depends on it nor asserts anything about star's extensions.

## Requirements

Where a requirement rests on a decision of this plan, it names it (**D*n***). Each decision is at the end, and the
owner's review can overrule any of them.

### Requirement 1: The channel is stamped into every binary

- `pkg/application` gains `Channel` and `Prerelease`, both strings, because `-X` sets only strings (a `bool` fails
  the link). `Prerelease` is stamped `true` or `false` and read through a function; empty means false. `LDFLAGS`
  stamps them beside `Version` (`Makefile:94`) from `CHANNEL` and `PRERELEASE`, empty unless given; `verify-ldflags`
  (`Makefile:476-488`) and `.goreleaser.yaml` carry them.
- `release.yaml` gives them from the ref it builds, in the same step that sets GitHub's `prerelease` flag:

  | Built from | Version | `prerelease` | `CHANNEL` | `PRERELEASE` |
  | --- | --- | --- | --- | --- |
  | `develop` | `v0.1.0-dev.<stamp>` | `true` | `develop` | `true` |
  | `main`, `release/*` | `v…-rc.<stamp>` | `true` | `release` | `true` |
  | a `v*` tag | `vX.Y.Z` | `false` | `release` | `false` |

  A local build stamps neither (**D6**).
- **The stamp is proven.** The `version` command's result (`VersionReport`, `cmd/internal/cli/version.go:14-23`) gains
  `channel` and `prerelease`, and whenever `CHANNEL` is given the build's stamp proof (`Makefile:305-326`) compares
  them, as it compares the version: an `-X` naming a missing symbol is silent (`pkg/application/version.go:16-21`), and
  a release without its channel would refuse every plain upgrade.
- `--version` names them: `writ version v0.1.0-dev.<stamp> (develop), build <commit>`, `(release)`, or
  `(release, prerelease)`.
- Each release is tagged at the commit it was built from: `gh release create --target <sha>`.

### Requirement 2: The channel is chosen, and switching is one command

| What you want | Command | Standing setting |
| --- | --- | --- |
| Releases only | `self upgrade --channel release` | `self.channel: release` |
| Releases and their pre-releases | `self upgrade --channel release --prerelease` | `self.channel: release`, `self.prerelease: true` |
| Every develop build | `self upgrade --channel develop` | `self.channel: develop` |

What a run upgrades to is decided, first to last:

1. **`--from <archive>`:** that archive; no channel is resolved and GitHub is not asked.
2. **`--channel <develop|release>`** (**D2**).
3. **`DEVLORE_VERSION`**, the pin the installers and the release report already use (`install.sh:78`,
   `install.ps1:95`, `release.yaml:107`): that exact release, whatever its channel (**D4**). `latest`, the
   installers' default (`install.sh:245`, `install.ps1:343`), is no pin. A pin set together with `--channel` is
   ignored, with a note saying so: a flag always wins.
4. **`self.channel`**, in the configuration shared by every program (**D3**).
5. **The stamped channel.**

The pre-release switch comes with the channel, never assembled from two places: `--prerelease` given means true;
otherwise it is false with `--channel`, `self.prerelease` (false if unset) with `self.channel`, and the stamped
`Prerelease` with the stamped channel. It means something only on `release`: every `develop` build is a pre-release.

The channel persists through the installed builds' stamp: one `writ self upgrade --channel release` moves the suite to
`release`, and every later plain `self upgrade` stays there. `--prerelease` persists that way only while what it
installs is itself a pre-release; the standing choice is `self.prerelease: true`. With no archive, no channel and no
pin, the command refuses and names `--channel` and `self.channel`.

### Requirement 3: The newest release on the channel is found

- **`release`:** `GET /repos/NobleFactor/devlore-cli/releases/latest`, which GitHub serves only for a published release
  whose `prerelease` flag is `false`. A 404 there means the channel has no release yet, which is today's answer.
- **`develop`, and `release` with `--prerelease`:** `GET /repos/NobleFactor/devlore-cli/releases?per_page=100`, page by
  page, newest first, drafts skipped (an owner's token sees them while a release uploads). On `develop`, the first
  release whose body says `Ref: refs/heads/develop`; on `release` with `--prerelease`, the first whose body doesn't
  (**D1**). The walk stops after 10 pages and says what it didn't find (**D9**).
- **A pin:** `GET /repos/NobleFactor/devlore-cli/releases/tags/<tag>`.
- The release must carry this platform's archive and the checksums file; if not, the command fails naming both and
  the tag.

### Requirement 4: It downloads without a token, and uses one when given

- Assets download from their public `browser_download_url`, so the API is asked only to find the release: one request
  on `release`, for a pin, and on `develop` while a develop build is the newest; on `release` with `--prerelease`,
  every page back to the newest non-develop release. None exists, so today that reads all 408 releases, 5 pages. The
  anonymous limit is 60 requests an hour.
- `GH_TOKEN`, if set, goes as the `Authorization` header on `api.github.com` requests only.
- Nothing prompts.

### Requirement 5: Nothing is installed that isn't verified

- The checksums file's line for the archive is found by exact name. The archive's SHA-256 must equal that line and,
  when it comes from GitHub, the asset's `digest`.
- A missing checksums file, a missing line or a mismatch fails the command before anything is extracted.

### Requirement 6: The archive is unpacked into scratch, contained, and the scratch always goes

- Unpacked into `fsroot.OpenScratch` (`pkg/fsroot/fsroot.go:133`), whose `Close` removes it.
- `self upgrade` runs under `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`: an interrupt cancels the
  download and the child (`pkg/process/runner.go:160-176`), and the command returns through `Close`.
- Containment as the archive provider's ruling has it (`docs/architecture/3.5.1-archive-provider.md:240-282`): an
  absolute name, a `..` that climbs out, or a path through a symbolic link is refused.
- Laid out as the installers lay it out: the programs in `pkg/bin/`, `share/` beside it (`install.sh:318-345`), so
  each program's `self install` finds what it expects.

### Requirement 7: Every devlore program in the prefix is upgraded, together

- **The suite** is every program the archive carries (`lore`, `star`, `writ`, `Makefile:232`) that the prefix holds:
  its binary in `bin/`, or its manifest at `share/<program>/manifest.json`. A program the prefix doesn't hold is not
  added; one an interrupted run left without its binary is put back.
- **For each program, the upgrade places the binary itself, then delegates.** It puts `pkg/bin/<program>` into
  `<prefix>/bin/` by Requirement 9's replacement, then runs `pkg/bin/<program> self install <prefix>`, with
  `--shell` passed through, from `pkg/`. The child then finds a target that isn't running, whatever build it is, so a
  release published before this change installs too.
- **Children run through `pkg/process.Runner`,** captured and relayed per the interaction ruling. A child narrates on
  stderr (`cmd/internal/cli/root.go:88-90`), which `Runner.Run` relays as warnings (`pkg/process/runner.go:135`), so
  `Runner` gains a way to relay a child's stderr as notes, and the upgrade uses it.
- **The first failure stops the run** and names the program. Nothing is rolled back; running it again finishes the
  job.
- **Refusals:** run from a program the archive doesn't carry (`devlore-test`), or from a binary renamed away from its
  program's name, the command refuses and says why.

### Requirement 8: Already current is a no-op

An installed program's version is the `version` in its manifest (`selfinstall.go:46-51`, `:710-712`), which its
`self install` writes from its stamp; a program whose manifest is missing or unreadable isn't current. When every
program in the suite is at the resolved release, the command says so, downloads nothing, changes nothing, and exits 0.

### Requirement 9: A running binary is replaced without a window in which it's missing

One function, used by `installBinary`, so every install gets it, and by the upgrade (Requirement 7):

- The new binary is written to the fixed name `bin/<name>.new` at mode `0750` and renamed over the target. A fixed
  name keeps a failed run idempotent (**D5**).
- On Windows, when the target is running, the running `.exe` is first renamed to `<name>.exe.old`, then `.new` is
  renamed into place. `self install` records a `<name>.exe.old` it finds in its manifest, so the next install retires
  it and `self uninstall` reaches it.
- `installBinary`'s early return compares with `os.SameFile`, not strings, and its comment says what it does.
- A manifest that can't be written fails the install (`selfinstall.go:277-280` only warns today): the upgrade relies
  on each child's exit status.

### Requirement 10: `--from <archive>` is the offline path

It installs a local archive by Requirements 5 to 9, less the `digest`, which only GitHub's API carries. The checksums
file must sit beside it; the tag comes from the archive's name; an archive for another operating system or
architecture is refused.

### Requirement 11: The global options hold

- `--dry-run` resolves the channel and the release, downloads and verifies it into scratch, reports the channel,
  whether it includes pre-releases, the release, the prefix and the suite, and changes nothing. `self install` itself
  ignores `--dry-run`, so the upgrade stops before placing a binary or starting a child.
- `--silent`, `--output` and the rest behave as everywhere: the result (`from`, `to`, `channel`, `prerelease`,
  `prefix`, `programs`) is emitted.

### Requirement 12: Errors say what happened

GitHub's own `message` is reported. A 404 from `/releases/tags/<tag>` names the tag; a 404 from `/releases/latest`
says the `release` channel has no release yet. A 403 or 429 with `x-ratelimit-remaining: 0` or `retry-after` is a rate
limit: it names `GH_TOKEN` when that is unset, and the reset time when it is set. A 401 says `GH_TOKEN` was refused. A
channel with no release names the channel; a failed child names its program.

### Requirement 13: Tests

- **Unit, against `httptest`:** the precedence, `--prerelease` with each source of the channel, and a pin beside
  `--channel`; `/releases/latest` for `release`; the newest release on `develop`, and on `release` with
  `--prerelease`, across pages, drafts skipped, and the request count of each path; asset names on the six platforms;
  the token on API requests only; checksum and digest mismatches, a missing line, a missing file; contained
  extraction; the suite, including a program left without its binary; the refusals, the no-op, `--dry-run`; each
  error; an interrupt in the middle of a download, after which no scratch is left; a narrating child relayed as notes.
- **Replacing a running binary:** a unit test starts a copy of an executable from `<prefix>/bin`, keeps it running,
  and installs over it. It fails against today's code on Linux, and runs on the required `test (windows-*)` legs,
  so Windows is proven by a required check.
- **The scenario,** `TestSelfUpgradeScenario`, on the six scenario legs, run by `make test-scenario` (`Makefile:396`):
  - The test builds its own fixture: `go build` of `./cmd/lore`, `./cmd/star` and `./cmd/writ` for the runner's
    platform, stamped `v0.0.0-scenario.1`, packed in `make dist`'s layout (`Makefile:534-550`) as
    `devlore-cli_v0.0.0-scenario.1_<goos>_<goarch>.<ext>`, with its checksums file. Not `make dist`, which would
    restamp `build/` (`Makefile:513`) or inherit its stamp (`Makefile:56`, `:88`).
  - It `self install`s the three from `build/` into a sandbox prefix whose environment sets `TMP` and `TEMP` beside
    `TMPDIR`, and `DEVLORE_VERSION` unset (`Makefile:54` reads the same name as a build's version).
  - `<prefix>/bin/writ self upgrade --from <archive> --shell bash`, from the installed copy: every binary is the
    fixture's and every manifest names `v0.0.0-scenario.1`; on Windows, `bin/writ.exe.old` exists and is in writ's
    manifest. A second run is a no-op.
- **Live, before the pull request,** on this machine (linux-arm64), from a scratch prefix, never the owner's `PATH`:
  this branch's build, `self install`ed there, runs `self upgrade --channel develop` and moves `lore`, `star` and
  `writ` to the newest develop pre-release. That build predates this change, so the no-op is proven in Phase 7.

### Requirement 14: The documents

- The help: `self upgrade`'s Short and Long (`selfinstall.go:130-139`), with `--channel` and `--prerelease`.
- `docs/architecture/10-command-line-interface.md` (`:199-218`): an upgrade is `self install` of a fetched build,
  replacing the record; the channels, their stamp, `--prerelease`, `self.channel` and `self.prerelease`.
- `docs/guides/getting-started.md`: how to upgrade, and how to switch channels.
- `docs/plans/self-command.md` (`:3-8`, `:25`, `:32`, `:72-77`, `:182`): set `complete`, its upgrade text marked
  superseded by this plan.
- The schema: `self.channel` and `self.prerelease` in `schema/devlore-config.json`. In
  `schema/defaults/devlore-shared.yaml` they are documented as comments, as `secrets` is: that file becomes a new
  install's `config.yaml` (`selfinstall.go:1077-1081`), and a value set there would outrank every build's stamp.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, approved, before the work. #947's table links it.

### Phase 2: The channel (Requirements 1, 2)

- [ ] `Channel` and `Prerelease` stamped by the Makefile and `release.yaml`, carried by `verify-ldflags` and
      `.goreleaser.yaml`, proven by the stamp check, reported by `version` and `--version`; `--target` on
      `gh release create`
- [ ] `self.channel` and `self.prerelease` in the schema; `--channel` and `--prerelease`; the precedence, with its
      tests first

### Phase 3: Replacing a binary (Requirement 9)

- [ ] The running-binary test first, failing; then `.new` and rename, Windows' `.old` and its record, `os.SameFile`,
      the manifest error

### Phase 4: The upgrade (Requirements 3 to 8, 10 to 12)

- [ ] Its tests first, against `httptest`
- [ ] Resolve, download, verify, unpack, the suite, placing and delegating, the no-op, `--from`, `--dry-run`, the
      interrupt, the errors and the result; `Runner`'s stderr as notes

### Phase 5: The scenario and the documents (Requirements 13, 14)

- [ ] `TestSelfUpgradeScenario`, and `make test-scenario` running it
- [ ] The help, the architecture document, the guide, the old plan, the schema

### Phase 6: Verify

- [ ] `gofmt`; `make test`; CI's quality gate; `make test-scenario`; the live check from a scratch prefix

### Phase 7: Merge

- [ ] PR script written, shown, and handed over. The PR resolves #947
- [ ] After the merge, DANOBLE-UD24-1 takes the develop pre-release built from the merge through the site's
      `install.sh` (a build before this one fetches nothing): `writ --version` names `(develop)`, and the release's tag
      is at the merge commit. On the pre-release after that, `writ self upgrade` on this machine moves `lore`, `star`
      and `writ` to it, and a second run is a no-op
- [ ] The follow-up issues filed (see Out of Scope), and #83 and #339 amended: their `auto-upgrade.channel`
      (`stable` / `preview`) and "latest LKG" give way to the two channels

## Out of Scope

- **Auto-upgrade** (`auto-upgrade.enable` and `.prompt`): #83, by ruling. #83's third setting, `auto-upgrade.channel`
  (`stable` / `preview`), and the "latest LKG" that #83 and #339 say `self upgrade` installs, are replaced by the two
  channels; Phase 7 amends both issues.
- **Pre-release channels:** "we might talk about pre-release channels at some point" (ruled 2026-09-30).
- **star's extensions:** #990. Its self-copy truncation, reproduced here (`cmd/star/star/root.go:313-337`), goes with
  the code #990 deletes; a comment on #990 carries the evidence.
- **The installers' checksums,** still warn-and-skip and matched by regular expression (`install.sh:297-311`,
  `install.ps1:384-400`): a follow-up issue.
- **`self uninstall` on Windows from the installed copy** can't remove the running `.exe`, counts it as "modified",
  and then deletes the manifest (`selfinstall.go:594-597`, `:628`, `:653-658`): a follow-up issue.
- **`self install` ignores `--dry-run`** (it writes the tree): a follow-up issue.
- **`self uninstall --unattended` still prompts** (`selfinstall.go:187-201`): a follow-up issue.
- **Every manifest records every man page in `share/man/man1`:** #780.

## Decisions

Engineering choices that follow from the rulings; each can be overruled.

- **D1. A release's channel is read from its body's `Ref:` line,** which every release already carries:
  `refs/heads/develop` is `develop`; any other is `release`. The upgrader doesn't parse tags; the `prerelease` flag is
  GitHub's, by ruling.
- **D2. `--channel` switches in one command,** and the channel persists through the new binaries' stamp. No separate
  `self channel` command.
- **D3. `self.channel` and `self.prerelease` are shared settings,** beside `model` and `registry`, because the channel
  belongs to the suite. Every program reads the shared configuration already (`cmd/internal/cli/root.go:167-176`).
- **D4. `DEVLORE_VERSION` is the only pin.** `--version` is the root's version flag (`version.go:46-53`), and a
  second pin would be a second name for one thing.
- **D5. Windows renames the running `.exe` aside.** Of the ways to replace it, only this one keeps the command's exit
  status and leaves the program missing for no longer than two renames. One `<name>.exe.old` per program stays until
  the next install retires it.
- **D6. A local build has no channel,** so it upgrades only when told what to: `--channel`, `self.channel`, a pin, or
  `--from`.
- **D7. No "going backwards" check.** The resolved release is installed whenever it differs from what's installed,
  as the installers do. A switch from `develop` to `release` may therefore install an older build; that is what the
  switch asks for.
- **D8. Shells are detected as `self install` detects them,** with `--shell` passed through, as every install path
  does today.
- **D9. The release list is read 10 pages deep at most** (1,000 releases, about 17 MB), so `release` with
  `--prerelease` can't walk without bound while develop builds are all there is.
