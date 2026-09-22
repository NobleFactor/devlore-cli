---
title: "star self uninstall removes only the extensions it installed"
issue: https://github.com/NobleFactor/devlore-cli/issues/917
status: complete
created: 2026-09-22
updated: 2026-09-22
---

# Plan: Uninstall removes only what it installed

## Issue 917

Lane 14 of NobleFactor/noblefactor-ops#217, and the last. `star self uninstall` promises manifest-driven removal --
"removes only files that have not been modified since installation" -- and its extensions hook does not keep that
promise.

## Current State

Read 2026-09-22 on `develop` at `99c67406`.

**Two defects, one on each side of the manifest.**

- **Install over-claims.** `installStarExtensions` (`cmd/star/star/root.go:305-329`) copies `star/extensions` into
  `<prefix>/share/star/extensions`, then returns `cli.CollectFiles(prefix, targetExtDir.Abs())` -- every file in the
  target directory, whoever put it there. Those are recorded in star's manifest as star's own.
- **Uninstall ignores the manifest.** `uninstallStarExtensions` (`:331-338`) calls
  `os.RemoveAll(<prefix>/share/star/extensions)`: the whole directory, modified or not, recorded or not.

The directory is shared by design. The loader probes `xdg.DataPath("star", "extensions")`
(`cmd/star/extension/loader.go:240`), so every extension anyone installs lives there. On DANOBLE-WD11-3 it holds
star's own 23 files and `com.noblefactor.ops.GitHub`, which **writ** deploys from the base layer. Either defect alone
deletes writ's deployment; writ still believes it deployed, and `star gh issues report` stops resolving.

**The generic uninstall already does this correctly** (`cmd/internal/cli/selfinstall.go:478-506`): for each manifest
entry it compares the file's SHA-256 with the recorded one, removes it when they match, skips and reports it when
they differ, then `cleanEmptyDirs` prunes the entries' ancestor directories. Nothing about extensions needs a
separate mechanism -- it needs the manifest to be honest.

There are no tests over either hook: `cmd/star/star/root_test.go` covers neither, and `selfinstall_test.go`'s 20
tests cover the generic path.

## Goals

1. `star self uninstall` removes the extension files star installed and leaves every other extension in place.
2. A modified extension file is skipped and reported, like every other manifest entry.
3. The empty directories star's own files leave behind are removed.

## Requirements

### Requirement 1: Install records only what it copied

`installStarExtensions` walks the **source** directory and returns one path per file it copied, relative to the
prefix: `share/star/extensions/<rel>`. It no longer reads the target directory, so a file it did not place cannot
enter its manifest.

### Requirement 2: The uninstall hook goes

`uninstallStarExtensions` and its registration in `PostUninstallHooks` are removed. With an honest manifest the
generic uninstall removes exactly star's files and `cleanEmptyDirs` prunes what empties, including
`share/star/extensions` itself when nothing else lives there.

### Requirement 3: Tests

In `cmd/star/star/root_test.go`, against a temporary prefix:

- **Install records only its own.** With a foreign file already in the target directory, the returned list holds
  every copied file and not the foreign one.
- **Uninstall leaves the foreign file.** Running the generic uninstall with that manifest removes star's files and
  leaves the foreign file and its directory.
- **A modified file survives.** A copied file whose content changed after install is skipped, as the generic path
  already promises.

## Implementation Phases

### Phase 1: Commit

- [x] This plan, first
- [x] Requirements 1 and 2 -- 2026-09-22
- [x] Requirement 3 -- 2026-09-22, with a fourth: a checkout without `star/extensions` installs nothing and claims
      nothing

### Phase 2: Verify before merge

- [x] `make vet`, `make lint` and `make test` all pass locally. `lint` first caught `gofmt` on the registration this
      change touched; formatted, then clean. `shell-lint` needs tools this host lacks and runs in CI -- 2026-09-22
- [x] Against `develop`'s `root.go`, both pin tests fail: the manifest claims
      `share/star/extensions/com.noblefactor.ops.GitHub/extension.yaml`, and the uninstall deletes it. Against this
      change they pass -- 2026-09-22
- [x] `TestSelfUninstall_LeavesAnotherInstallersExtension` is that test, run through the real `self install` and
      `self uninstall` commands against a scratch prefix, with `XDG_CONFIG_HOME` and `XDG_CACHE_HOME` redirected so
      an uninstall cannot reach the developer's own -- 2026-09-22

## Out of Scope

- **#780**, the same family on the man-page side: `installManPagesTo` records every file in the shared
  `share/man/man1`. This plan fixes the extensions only; #780's fix should follow the same shape.
- The loader's shared-directory design. One directory for every extension is deliberate.

## Open Questions

- [x] **Directory pruning:** `cleanEmptyDirs` is enough and the hook goes entirely, ruled 2026-09-22.
- [x] **The data location** -- star's extensions belong under `devlore/` like writ's -- is its own bug,
      NobleFactor/devlore-cli#918, lane 15, after this one. With an honest manifest the move cannot take another
      installer's extensions with it.
