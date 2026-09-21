---
title: "Lanes 12 to 14: file.mkdir obeys --conflict, the conflict policy has three names and one floor, and the winget driver reads winget's answer as winget does"
issue: https://github.com/NobleFactor/devlore-cli/issues/822
status: complete
created: 2026-09-20
updated: 2026-09-21
---

# Plan: Lanes 12 to 14 of the command line schedule

## Summary

Lane 12 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), pulled forward from
[#894](https://github.com/NobleFactor/devlore-cli/issues/894) by ruling of 2026-09-20. `--conflict=<policy>` answers one
question -- what writ does when something already exists at a target it is about to write -- and two of the file
provider's three writers answer it: `file.link` and `file.copy` refuse under `stop`, leave the target under `skip`,
and archive-then-write under `replace`. `file.mkdir` never reads the policy: anything at its path that is not a
directory is an error under every policy, and a real `writ deploy` stops there on both virtual machines, on a stale
symlink from the previous dotfile manager. This plan makes `Mkdir` apply the policy exactly as `Link` does, and makes
its receipt restore what `replace` displaced.

Lane 13, [#899](https://github.com/NobleFactor/devlore-cli/issues/899), found while planning lane 12 and ruled
into the same pull request on 2026-09-20: the policy's vocabulary drifts in six places -- `writ deploy`'s Examples
and the guide name `backup` and `overwrite`, which the flag rejects, and three comments call two different defaults
the floor. The same three words, put right everywhere at once.

Lane 14, [#900](https://github.com/NobleFactor/devlore-cli/issues/900), found running lane 12's Windows sequence and
ruled into the same pull request on 2026-09-21: the winget driver's post-check matched the package id case-sensitively
against winget's case-insensitive answer, so an installed package read as absent, the packages subgraph failed, and the
deploy never reached the `mkdir` occupant lane 12 exists to replace.

**This plan covers these three lanes and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 822

Bug, P1, feature [#836](https://github.com/NobleFactor/devlore-cli/issues/836) (the file provider). The defect as
measured on DANOBLE-WD11-3 and, on 2026-09-18, on `danoble-ud24-1.local`:

```
file.mkdir: C:\Users\david-noble\.config\git exists, but is not a directory
```

invoked with `--conflict=replace`, on a target the user had told writ to replace. The issue's ruling: `stop` refuses
(and [#831](https://github.com/NobleFactor/devlore-cli/issues/831)'s pre-flight lists the target with the others --
that lane's, not this one's); `skip` leaves the directory uncreated, and the units beneath it are skipped as a skipped
link's would be; `replace` archives the occupant to the recovery site, creates the directory, and the receipt
records the archive so compensation can restore it.

## Issue 899

Bug, Severity Low, feature #836. `--conflict` has three names -- `stop`, `skip`, `replace` -- and `ParseConflictPolicy`
rejects any other. The seam's floor is `replace`; `writ deploy`'s flag default is `stop`. Six sites say otherwise,
enumerated 2026-09-20 at `ad2219ef` and listed in Requirement 6. Ruled 2026-09-20: its own issue, resolved in
lane 12's pull request, so [#881](https://github.com/NobleFactor/devlore-cli/issues/881) stays on the Planner
sentence.

## Issue 900

Lane 14, added 2026-09-21. `bracket` decides an install from the post-state query alone; the winget leaf's `version`
runs `winget list --id Vim.Vim` and scans the output for a line containing `Vim.Vim`, case-sensitively; winget prints
`vim.vim`. Vim 9.2.1119 was installed and the driver reported `post=""`. We are case-sensitive; winget is not.

## Goals

- [x] `file.mkdir` over a regular file, a live symlink and a dangling symlink, under `stop`, `skip` and `replace`:
      each of the nine cells produces the outcome the contract table names, with a unit test per cell.
- [x] The receipt of a replacing `mkdir` carries the archive id and the pre-archive digest, and
      `CompensateFileMutation` restores the occupant after removing the directory it created.
- [x] `writ deploy … --conflict=replace` gets past `file.mkdir-6` on DANOBLE-WD11-3, and past `file.mkdir` on
      `danoble-ud24-1.local`, in the end-to-end sequence every pull request runs. Windows 2026-09-21 06:02:
      `C:\Users\david-noble\.config\git`, the 2022 symlink to `Home/all/.config/git`, is archived in recovery
      (`01a0c1e9-f911-…`) and the path is a directory holding five links.
- [x] Lane 14: the winget leaf reads winget's listing with the id compared under `strings.EqualFold`, from one parser
      shared by `version` and `installed`, tested on every leg; the Windows deploy gets past `pkg.install-1`.
- [x] Lane 13: the policy has three names and no others, everywhere; the seam's floor is `replace` and
      `writ deploy`'s default is `stop`, said the same way in every comment and document; the generated reference
      matches.

## Current State

Read 2026-09-20 at `ad2219ef`, in `pkg/op/provider/file/provider.go` unless named otherwise.

| Component | Status | Notes |
| --- | --- | --- |
| `Provider.Mkdir` (`:350`) | ❌ | refuses a non-directory occupant at `:363` (observed before the claim) and again at `:380`, without reading the policy |
| `Provider.Link` (`:233`) | ✅ | the template: `lstat`, the policy switch, `archiveOccupant` minting an update receipt with recovery id and digest |
| `Provider.stageWrite` (`:2087`) | ✅ | the copy/move/write trunk: the same switch, `archiveAndPrune`, `WithRecovery` on the spec |
| `conflictPolicy` (`:2063`) | ⚠️ | reads `Application.Flags["conflict"]`; its comment names `ConflictStop` as the floor -- the floor is `ConflictReplace` (`pkg/op/runtime_environment.go:841`) |
| `compensateMakeDir` (`:411`) | ❌ | unwinds the created chain to the boundary; never restores a recovery archive |
| `RecoverySite.ArchiveFile` / `RestoreFile` (`pkg/op/recovery_site.go:81`, `:166`) | ✅ | rename-based: a symlink, live or dangling, is a directory entry and renames like a file |
| `discoverEntryOfMode`, `markEntryGone` (`:1906`) | ✅ | how the delete trio observes an entry as its kind and tells the catalog it is gone |
| `NewDirectory` (`directory.go:106`) | ✅ | claims the URI; an entry already claimed under another kind is an error -- why `Mkdir` observes before claiming |
| tests, `pkg/op/provider/file` | ❌ | no test selects a policy (`testEnvironment` builds an `Application` with a nil `Flags` map); `Mkdir` has three forward tests and six compensation tests, none with an occupant |
| the layer-journey scenario | — | deploys into an empty sandbox; it has no occupied-target cell and this plan adds none |
| `docs/architecture/3.5.4-file-provider.md` §3 | ⚠️ | "at every write-path occupied-target check … the run's conflict policy governs" -- true of two writers of three |
| `cmd/writ/writ/commands.go:45-46` | ❌ | `writ deploy`'s Examples: `--conflict=backup`, `--conflict=overwrite`; `docs/cli/writ/deploy.md` is generated from them by `make docs` |
| `docs/guides/writ/manage-environments.md:36-41` | ❌ | the same two, captioned "Back up conflicting files with timestamps" and "Overwrite without backup" |
| `pkg/op/runtime_environment.go:58`, `:819` | ⚠️ | `BackupSuffix` "appended to back up filenames during conflict resolution" -- it serves `file.backup`; conflict resolution archives |
| `pkg/op/runtime_environment.go:789` | ⚠️ | `ParseConflictPolicy`: `""` "parses as the `ConflictStop` floor" -- `""` is the flag's zero value, `stop`, the command line's default; the seam's floor is `replace` |
| `pkg/platform/windows_managers_windows.go:240`, `:174` | ❌ | `version` and `installed`: `strings.Contains(line, id)` on `winget list` output; winget prints catalog casing (`vim.vim` for `Vim.Vim`) |
| `pkg/platform/helpers.go:85` `bracket` | ✅ | the verdict comes from the post-query alone, so a parser miss reads as a failed install |
| `docs/architecture/configuration.md:372` | ❌ | "`ConflictPolicy: ConflictStop` directly -- the floor is a compiled-in default"; the compiled-in default is `ConflictReplace` (`runtime_environment.go:841`) |

## Requirements

### Requirement 1: the policy governs `Mkdir`'s occupied target

`Mkdir` observes the leaf with `lstat` before anything else, as it does today. Nothing there: create, as today. A
directory: unchanged, nil receipt, as today -- the policy is never consulted for the idempotent case. Anything else --
a regular file, a live symlink, a dangling symlink -- is an occupied target, and the write-seam policy governs, in
the words `Link` and `stageWrite` already use:

| Policy | `Mkdir` does |
| --- | --- |
| `stop` | refuses: `target <abs> is occupied and the conflict policy is stop (replace archives and overwrites; skip leaves it)` |
| `skip` | returns no product and no receipt, as `Link` does; nothing on disk changes |
| `replace` | Requirement 2 |

Every refusal still precedes the claim: the `stop` and `skip` returns happen before `NewDirectory`, so the catalog's
cross-kind collision is never reached (the comment at `:360` stands, reworded for the policy).

### Requirement 2: `replace` archives the occupant, then creates

In this order, each step an existing mechanism:

1. `discoverEntryOfMode(leaf, info.Mode())` -- the occupant as its observed kind.
2. `archiveAndPrune(entry, false, "")` -- the occupant renamed into `.devlore/recovery/<id>`, with its pre-archive
   digest; the digest is the zero value for a symlink, as `archiveAndPrune` documents, and that is not an error.
3. `markEntryGone(activationRecord, entry)` -- the catalog learns the occupant is gone before the directory is claimed.
4. `NewDirectory` claims the path; `findClosestExistingDir(leaf)` now finds the parent, which becomes the boundary.
5. The receipt spec: `NewReceiptSpec(product, MutationCreateDir).WithBoundary(boundary).WithRecovery(recoveryID, digest)`.
6. `mkdirAll`, ownership, resolve, as today.

The kind stays `MutationCreateDir`: the mutation is still "a directory was created here"; the recovery fields say
what it displaced. A new `MutationReplaceDir` kind was considered and rejected -- it would add a dispatch arm for
what is one field on the receipt.

### Requirement 3: compensation restores the occupant

`compensateMakeDir` unwinds the created chain to the boundary as today; then, when `receipt.RecoveryID()` is
non-empty, it calls `RecoverySite.RestoreFile(resource.Path(), id)`, tolerating `ErrRecoverySourceNotFound` exactly
as `compensateWrite` does. After compensation, `Lstat` at the path sees the occupant again -- the symlink with its
original target, or the file with its original bytes.

### Requirement 4: nine cells, and the round trip

A test per cell: three occupants (regular file, live symlink, dangling symlink) by three policies, table-driven with a
subtest per cell, asserting for each what Requirement 1's table says -- the message, the nil returns, or the archive
and the directory. A test selects the policy the way the command line does: the test environment's
`Application.Flags = map[string]any{"conflict": <policy>}`. A round trip per occupant kind under `replace`:
forward, then `CompensateFileMutation`, then the occupant is back and the directory is gone. And the idempotent case
pinned: a directory occupant under `stop` is not a refusal.

**Delta, 2026-09-20, found by `make test` after Phase 4 landed.** Three lifecycle scenarios in
`pkg/op/provider/plan` -- `TestLifecycle_ViaStarlark/FailAndRollback`, `TestLifecycle_ViaGoAPI/FailAndRollback`, and
`TestGraphResumeThenFail_RollsBack_ViaPublicAPI` in both trace formats -- inject their failure by occupying an
`mkdir`'s path with a regular file, under an `Application` with no policy flag. That was the old contract: `mkdir`
refused every occupant. Under the `replace` floor the occupant is now archived and the run succeeds, and the three
assert "want a failure, got nil". They follow the contract: their environments (`lifecycle_e2e_test.go:233`,
`lifecycle_api_test.go:48`, `:136`, `:219`) select `stop` through `Application.Flags["conflict"]`, so the failure
they inject is the seam's refusal -- the policy a `writ deploy` runs under by default -- and their intent, "the run
fails at the un-run frontier and rolls back", is unchanged. The unoccupied scenarios in the same suites are
unaffected by the policy.

### Requirement 5: the words

`Mkdir`'s doc comment says what the policy does to it, and `conflictPolicy`'s comment names the floor that is
actually configured. `docs/architecture/3.5.4-file-provider.md` §3 says the policy governs "every write-path
occupied-target check"; this lane makes that sentence true, and §3 says so in one line: a non-directory occupant of
`mkdir`'s path is the seam's occupied target, and a directory is the idempotent case, never a conflict. The status
document's write-seam row records the date and the issue.

### Requirement 6: three names, one floor (lane 13)

The rule: the policy has three names, `stop`, `skip` and `replace`, and no others, everywhere; the seam's floor is
`replace`; `writ deploy`'s default is `stop`; "back up", "overwrite" and "timestamps" are not its vocabulary. The
six sites, as #899 enumerates them:

1. `cmd/writ/writ/commands.go:45-46` -- the Examples become `--conflict=replace` and `--conflict=skip`. The
   reference page `docs/cli/writ/deploy.md` is not a repository file: `/docs/cli/` is ignored (`.gitignore:5`) and
   `.github/workflows/docs-publish.yaml` generates it from source at publish (`make docs`, which is
   `build/devlore-docs --output-dir=docs/cli --version=$(VERSION)`), so it follows the Examples by construction;
   `make docs` proves it locally.
2. `docs/guides/writ/manage-environments.md:36-41` -- the three policies, captioned: `stop` refuses and lists the
   occupants (the default), `skip` leaves them, `replace` archives each to the recovery site and writes, restorable.
3. `pkg/op/runtime_environment.go:58` and `:819` -- `BackupSuffix` is `file.backup`'s suffix; the comments stop naming
   conflict resolution.
4. `pkg/op/provider/file/provider.go:2059` -- the floor is `ConflictReplace` (Phase 2's second box; the same edit).
5. `pkg/op/runtime_environment.go:789` -- `""` parses as `stop`, the command line's default; the seam's floor is
   `replace` and is named as such.
6. `docs/architecture/configuration.md:372` -- the compiled-in default is `ConflictReplace`.

Left as they stand, being right: `deploy.go:273`, the guide's "Stop on first conflict (default)", §3 of
`3.5.4-file-provider.md` and its status row. The proof is a search: `conflict=(backup|overwrite)` over `cmd/`,
`pkg/`, `docs/guides/` and `docs/architecture/` finds nothing, and the pull request script asserts it.

### Requirement 7: the winget leaf reads winget's answer as winget does (lane 14)

One pure function in the untagged `windows_managers.go`, `wingetListedVersion(stdout, id string) string`: for each line
of a `winget list` output, the field equal to `id` under `strings.EqualFold` is the Id column, and the field after it
is the Version; no such line returns "". `version` and `installed` in the Windows-tagged file call it. Pinned by a
table test in an untagged `windows_managers_test.go` -- the captured listing above answering `Vim.Vim` with
`9.2.1119`, a two-package listing, a header-only listing -- so the parser is tested on every CI leg, not only the two
Windows ones.

## Design

```
  file.mkdir <path>
        |
        |  1. lstat the leaf
        |       nothing there ......... create, as today
        |       a directory ........... unchanged, as today
        |       anything else ......... the policy governs:
        |            stop ............. refuse, the seam's message           <- before the claim
        |            skip ............. no product, no receipt               <- before the claim
        |            replace .......... 2
        |  2. discover the occupant as its kind; archive it; mark it gone
        |  3. claim the Directory; boundary = the closest existing ancestor, now the parent
        |  4. receipt: create_dir + boundary + recovery id and digest
        |  5. mkdir, ownership, resolve
        v
  compensation: unwind the created chain to the boundary, then restore the archived occupant
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-20; lane 13 and Requirement 6 added before the charter.

### Phase 2: The seam in `Mkdir`

- [x] The policy switch at the observed occupant; `stop` and `skip` return before the claim; `replace` runs
      Requirement 2's six steps. The refusal at `:380` becomes unreachable for a non-directory and is removed. Landed 2026-09-20: the
      six steps live in `archiveMkdirOccupant`, named as the plan's open question 1 foresaw; `make vet` clean; the
      nine cells prove it in Phase 4.
- [x] `conflictPolicy`'s floor comment says `ConflictReplace`; §3 of the file-provider document and its status row say
      `mkdir` joined the seam. The comment is done; the two documents are Phase 5's, with the rest of the words.

### Phase 3: Compensation

- [x] `compensateMakeDir` restores the recovery archive after its unwind, tolerating a missing source. Landed 2026-09-20; a
      sibling-adopted directory still stops the unwind, and then no occupant can return to a path that is still a
      directory -- the doc comment says so. The round trips prove it in Phase 4.

### Phase 4: Tests

- [x] The nine cells, table-driven, in `provider_test.go` beside the `TestMkdir_*` tests. Landed 2026-09-20 as three
      table-driven tests, one per policy, a subtest per occupant: `TestMkdir_Stop_RefusesAnOccupant`,
      `TestMkdir_Skip_LeavesAnOccupant`, `TestMkdir_Replace_ArchivesAnOccupantAndCreates`; `occupyForMkdir`,
      `assertOccupantIntact` and `assertNothingArchived` at the end of the file. `TestCompensateMkdir_NotADirectory_ReturnsError`
      asserted the old contract under the `replace` floor and went; the `stop` cells carry the refusal.
- [x] The three round trips and the directory-occupant pin. `TestMkdir_Replace_CompensationRestoresTheOccupant` and
      `TestMkdir_ADirectoryIsNeverAConflict`, 2026-09-20.
- [x] The lifecycle scenarios in `pkg/op/provider/plan` select `stop`, per Requirement 4's delta; `make test` green. Ruled and landed
      2026-09-20: `newLifecycleEnv` (`lifecycle_e2e_test.go:237`) and `resumeThenFailRollsBack`
      (`lifecycle_api_test.go:416`), two sites; `make test` exit 0.

### Phase 5: The words (lane 13)

- [x] The six sites of Requirement 6, edited 2026-09-20; `make docs` generated a `docs/cli/writ/deploy.md` whose Examples
      read `--conflict=replace` and `--conflict=skip` (the page is ignored by git and generated at publish, so nothing
      of it is committed); the search over `cmd/`, `pkg/`, `docs/guides/` and `docs/architecture/` finds nothing.

### Phase 6: The winget id (lane 14)

- [x] `wingetListedVersion` in `windows_managers.go`; `version` and `installed` call it; the table test
      (`windows_managers_test.go`, seven cases on the captured listing and two others). 2026-09-21: `make vet-all`,
      `make check` and `make test-scenario` exit 0.
- [x] The Windows binary rebuilt, copied, installed; `writ deploy common noblefactor-ops --conflict=replace` on
      DANOBLE-WD11-3 gets past `pkg.install-1`, and the log is kept under `build/e2e/`. 2026-09-21 06:02: **deployed 56 links, 1 skipped, exit 0**
      (`build/e2e/2026-09-21-windows-danoble-wd11-3-lane14-deploy-2.log`), the packages subgraph passed with Vim
      read as installed.

### Phase 7: Gates and the three numbers

- [x] `make check` and `make test-scenario`, both exit 0. 2026-09-21, after the lint gate sent `compensateMakeDir`
      (cognitive 21) into `restoreDisplacedOccupant`, the forward side's mirror.
- [x] Coverage per package and total, complexity, code size. Coverage (statement): `pkg/op/provider/file` 66.7%,
      `pkg/op/provider/plan` 56.6%, `pkg/op` 75.3%, total 62.8%; `Mkdir` 82.4%, `archiveMkdirOccupant` 80.0%,
      `compensateMakeDir` 78.4%, `restoreDisplacedOccupant` 85.7%, `conflictPolicy` 100%. Complexity: `Mkdir` 14,
      `compensateMakeDir` 12, nothing else in the change over 8. Size: `provider.go` 2,531, `provider_test.go` 2,815,
      `lifecycle_e2e_test.go` 282, `lifecycle_api_test.go` 898. Re-measured after lane 14, 2026-09-21: total 62.8%;
      `pkg/platform` 65.1%, `wingetListedVersion` 100%; `windows_managers.go` 138, `windows_managers_windows.go` 302,
      `windows_managers_test.go` 38; no function in lane 14 over 8.

### Phase 8: Installed and exercised

- [x] `make install`; on this machine, `writ version` names the build. 2026-09-21T01:03:07Z, `ad2219ef-dirty`.
- [x] End to end on `danoble-ud24-1.local`, snapshotted first, 2026-09-21, build `ad2219ef` (log:
      `build/e2e/2026-09-21-linux-danoble-ud24-1-sequence.log`): self install 0; base and team `unset` twice, `set` by
      URL clones to `repos/noblefactor-ops` and `repos/devlore-cli`, `set` by URL again and by path `unchanged`
      (team's first clone died in git on a transient `curl 92` reset, the retry succeeded); personal by path; bare
      `deploy` 64 (#843); `deploy common noblefactor-ops` under `stop` refused 25 occupied targets; under
      `--conflict=replace` **deployed 127 links, exit 0**, 25 occupants archived to recovery; `upgrade` 0. Every
      occupant on that machine was a file or a file symlink -- `.config/git` was already a directory -- so the
      `mkdir`-over-occupant cell was exercised by the unit tests there, not by the machine.
- [x] End to end on `danoble-wd11-3.local`, snapshotted running (the host had 160 GB free), 2026-09-21, build
      `ad2219ef` (logs under `build/e2e/`: `…-windows-danoble-wd11-3-sequence.log`, `…-personal-and-deploy.log`,
      `…-lane14-deploy.log`, `…-lane14-deploy-2.log`). First pass, before lane 14: self install 0; base and team by URL
      and by path, git-named clones; personal by URL fails unattended (the credential manager cannot prompt without a
      tty), registered by path to `C:\Users\david-noble\Workspace\Personal` (`27e53171`); `deploy` under `stop`
      refused 27 occupied targets; under `--conflict=replace` the run failed at `pkg.install-1: winget/Vim`, which
      became lane 14. Second pass, on the lane-14 build: the machine had been reverted to the 04:08 snapshot in the
      interval (the two clones and `.devlore` gone, `repos/` at its Sep 18 mtime), so `self install` recreated the
      three layer directories (#840's phantom) and base and team could not be re-registered by path; personal by path,
      then **`deploy common noblefactor-ops --conflict=replace`: 56 links, 1 skipped, exit 0** -- the `mkdir` over the
      `.config\git` symlink archived it and made the directory; `upgrade` 0.
- [x] The owner's row: `writ deploy` or `writ upgrade` before the pull request opens. Done by the owner 2026-09-21.

### Phase 9: Closure

- [x] #822's three acceptance boxes, #899's three and #900's three, ticked with evidence 2026-09-21; the pull
      request closes all three; this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1-3 | `stop` over a file, a live symlink, a dangling symlink: the seam's refusal; nothing archived, nothing created | unit, `file` | the refusal is still the old text, or the policy is not read |
| 4-6 | `skip` over each: nil product, nil receipt, the occupant intact | unit, `file` | anything on disk changes |
| 7-9 | `replace` over each: the directory exists; the occupant is in recovery; the receipt is `create_dir` with a recovery id, and a non-zero digest for the file | unit, `file` | the archive is missing, or the receipt cannot restore |
| 10-12 | compensation after `replace` over each: the directory is gone and the occupant is back | unit, `file` | the occupant is lost |
| 13 | a directory occupant is unchanged under `stop` | unit, `file` | the policy touches the idempotent case |
| 14 | `make test-scenario` green | scenario, six platforms | the seam broke a deploy into an empty sandbox |
| 15 | deploy past `file.mkdir` on both virtual machines | end to end | the VM case differs from the unit cells |
| 17 | `wingetListedVersion`: `Vim.Vim` over a listing printing `vim.vim` → `9.2.1119`; two packages; header only → "" | unit, `platform`, every leg | the parser is still case-sensitive |
| 18 | DANOBLE-WD11-3: `deploy … --conflict=replace` past `pkg.install-1` | end to end | the packages subgraph still stops the scope |
| 16 | no `conflict=backup` or `conflict=overwrite` anywhere in `cmd/`, `pkg/`, `docs/guides/`, `docs/architecture/`; `make docs` generates a `docs/cli/writ/deploy.md` naming only the three | search, in the pull request script; the page locally | a stale name survives |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/822-mkdir-obeys-conflict.md` | Create |
| `pkg/op/provider/file/provider.go` | Modify: `Mkdir`, `compensateMakeDir`, the `conflictPolicy` comment |
| `pkg/op/provider/file/provider_test.go` | Modify: the cells, the round trips, the pin |
| `pkg/op/provider/plan/lifecycle_e2e_test.go`, `lifecycle_api_test.go` | Modify: the lifecycle environments select `stop` (Requirement 4's delta) |
| `docs/architecture/3.5.4-file-provider.md` | Modify: §3, one line |
| `docs/architecture/3.5.4-file-provider.status.md` | Modify: the write-seam row |
| `cmd/writ/writ/commands.go` | Modify: the `writ deploy` Examples |
| `docs/guides/writ/manage-environments.md` | Modify: the conflict examples |
| `pkg/op/runtime_environment.go` | Modify: three comments |
| `pkg/platform/windows_managers.go` | Modify: `wingetListedVersion` |
| `pkg/platform/windows_managers_windows.go` | Modify: `version` and `installed` call it |
| `pkg/platform/windows_managers_test.go` | Create: the table test |
| `docs/architecture/configuration.md` | Modify: one line |

## Open questions

1. **Complexity.** `Mkdir` gains a switch and a six-step branch; if it crosses the ceiling, the replace branch
   becomes `archiveMkdirOccupant`, named for what it does, as `archiveOccupant` is for `Link`.
2. **The Windows machine's disk.** The snapshot needs headroom the host does not have; the cold snapshot sidesteps
   the 32 GB memory image, and the rest is the owner's.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lane 12
- [#822](https://github.com/NobleFactor/devlore-cli/issues/822) -- the bug, lane 12
- [#899](https://github.com/NobleFactor/devlore-cli/issues/899) -- the vocabulary, lane 13
- [#900](https://github.com/NobleFactor/devlore-cli/issues/900) -- the winget id, lane 14; related [#817](https://github.com/NobleFactor/devlore-cli/issues/817)
- [#881](https://github.com/NobleFactor/devlore-cli/issues/881) -- `writ deploy --help`'s Planner sentence, lane 14 of #894, untouched here
- [#831](https://github.com/NobleFactor/devlore-cli/issues/831) -- the pre-flight listing, the next lane in #894, not this one
- [#836](https://github.com/NobleFactor/devlore-cli/issues/836) -- the file provider feature
- [docs/plans/feature/791-repo-set-and-unset.md](../feature/791-repo-set-and-unset.md) -- the previous lanes, where the VM sequence and its findings are recorded
