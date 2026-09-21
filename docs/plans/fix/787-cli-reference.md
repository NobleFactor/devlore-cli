---
title: "Lane 16: every program has a generated reference, and the build generates it"
issue: https://github.com/NobleFactor/devlore-cli/issues/787
status: complete
created: 2026-09-21
updated: 2026-09-21
---

# Plan: Lane 16 of the command line schedule

## Summary

Lane 16 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), the last. `devlore-docs` builds the
command trees of `writ` and `lore` and writes `docs/cli/**` for those two; `star` and `devlore-test` are in scope
for the convention (§2 of the specification) and have no reference. The generator learns the other two roots -- which
means star's root, today in `package main`, becomes importable -- and, by the owner's ruling of 2026-09-21, `make
build` generates the reference on every build, so the pages are never a separate step someone forgets.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 787

Bug, Severity Low, feature [#834](https://github.com/NobleFactor/devlore-cli/issues/834). Its one design question:
star's root lives in `package main` (`cmd/star/main.go`, 619 lines: the root, the `key` and `docs` commands, the
extension install hooks, and the Starlark command registration), so the generator cannot import it as it imports
`lore.NewRootCmd` and `writ.NewRootCmd`. devlore-test's root is importable today
(`devloretest.NewRootCmd`). The owner added 2026-09-21: the build builds docs.

## Goals

- [x] `make docs` generates `docs/cli/star/**` and `docs/cli/devlore-test/**` beside `writ` and `lore`; star's tree
      includes its Starlark extensions' commands, since they are embedded in the binary and load with the root.
- [x] `make build` generates the reference for the host after it builds the tools, so a checkout that built has a
      current `docs/cli/`.
- [x] A test asserts every in-scope program has a generated tree; `docs-publish.yaml` copies the two new trees
      with the rest, by construction.

## Current State

Read 2026-09-21 at `a83653fe`.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/devlore-docs/main.go` `run` | ❌ | walks `writ.NewRootCmd()` and `lore.NewRootCmd()`; nothing else |
| `cmd/star/main.go` `newRootCmd` | ❌ | `package main`; returns the root and the `*starruntime.Application` the commands run in; loads the embedded extensions last |
| `cmd/star/extensions.go` | — | `//go:embed extensions` in `package main`; an embed names paths under its own package directory, so it moves with, or beside, whatever loads it |
| `cmd/star/star` | ⚠️ | `package star` is the **extension host** -- `Extension`, `ExtensionLoader`, `ExtensionRegistry`, `Command`, and the `Application` that loads and dispatches them; five of its six files name an extension type first, none a runtime. Importers alias it `starruntime` for want of a name |
| `cmd/devlore-test/devloretest.NewRootCmd` | ✅ | importable |
| `Makefile` `build` | ❌ | builds products and tools; `docs: build` runs the generator only when asked |
| `.github/workflows/docs-publish.yaml` | ✅ | `make docs`, then copies `docs/cli/` whole; new trees ride along |
| `cmd/devlore-docs/generator_test.go` | ⚠️ | tests the walker on synthetic trees; nothing asserts which programs are walked |

## Requirements

### Requirement 1: star's root is importable, and its packages are named for what they hold

Ruled 2026-09-21: the package today at `cmd/star/star` is star's extension host, and takes the name `extension`.
Three moves, all layout:

1. `cmd/star/star` (`package star`) becomes `cmd/star/extension` (`package extension`): `extension.Loader`,
   `extension.Registry`, `extension.Application`, `extension.Command`, `extension.Extension`, read as they are.
   The `Extension` prefix comes off `ExtensionLoader` and `ExtensionRegistry`, which the package name now
   carries. A directory rename, the type renames, and four import paths.
2. A new `cmd/star/star` (`package star`) holds what `main.go` holds today except `main` and `run`:
   `NewRootCmd() (*cobra.Command, *extension.Application)`, the `key` and `docs` commands, the install and
   uninstall hooks, and the Starlark command registration. `main.go` keeps `main` and `run`, and calls
   `star.NewRootCmd()`. The same shape as `cmd/writ/writ` and `cmd/lore/lore`.
3. The embed moves to where an importable package can hold it: `cmd/star/extensions/embed.go`,
   `package bundled`, `//go:embed *` over the extension directories, exporting `FS` -- `bundled.FS`, the
   extensions compiled into the binary, so the name does not collide with `extension`. The root loads from
   `bundled.FS`; nothing about how extensions are discovered or installed changes.

`root_test.go` in `cmd/star` follows whichever functions it tests.

### Requirement 2: the generator walks four roots

`devlore-docs`'s `run` walks `writ`, `lore`, `star` and `devlore-test`, in that order, each under its own
directory of `docs/cli/`. star's root is built with `star.NewRootCmd()` and its `extension.Application` closed after the walk.
The tool names are the program names; nothing else about a page changes.

### Requirement 3: the build generates the reference

`build`'s recipe, after the tools loop and only when the host is in the selection (the same guard the version
stamp uses), runs `build/devlore-docs$(HOST_GOEXE) --output-dir=docs/cli --version=$(VERSION)`. `docs: build`
stays as the standalone target and `docs-publish.yaml` keeps calling it. `docs/cli/` stays ignored.

### Requirement 4: a test names the programs

A test in `cmd/devlore-docs` runs `run` into a temporary directory and asserts a page tree exists for each of the
four programs, by name -- so a fifth program, or a dropped one, fails the test rather than the site.

### Requirement 5: the words

§12 of the specification says the reference covers the four programs and that `make build` generates it. The
Makefile's `build` and `docs` help lines say so too.

## Design

```
  make build
    |  generate, build products, build tools, stamp check
    |  host in selection?  ->  build/devlore-docs --output-dir=docs/cli --version=VERSION
    v
  devlore-docs run
    |  writ.NewRootCmd()          -> docs/cli/writ/**
    |  lore.NewRootCmd()          -> docs/cli/lore/**
    |  star.NewRootCmd()          -> docs/cli/star/**      (bundled.FS: the full tree, extensions included)
    |  devloretest.NewRootCmd()   -> docs/cli/devlore-test/**
    v
  docs-publish.yaml copies docs/cli/ whole
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-21; Requirement 1's layout ruled: `extension`, not `starruntime`,
      not a merged package.

### Phase 2: star's root

- [x] `cmd/star/star` -> `cmd/star/extension`; `ExtensionLoader` -> `Loader`, `ExtensionRegistry` -> `Registry`;
      four import paths. 2026-09-21.
- [x] `cmd/star/extensions/embed.go` (`package bundled`, `//go:embed com.*`, `FS`); the new `cmd/star/star/root.go`
      with `NewRootCmd` and what moves with it; `main.go` reduced to `main` and `run` (619 lines to 30);
      `root_test.go` follows into `package star`. `make vet` clean, 2026-09-21.

### Phase 3: The generator and the build

- [x] `devlore-docs` walks four roots, star's with its `extension.Application` closed after the walk. 2026-09-21.
- [x] `build` runs the generator after the tools loop (they always build for the host, so no guard); the help
      lines of `build` and `docs`.
- [x] The test of Requirement 4: `TestRun_EveryProgramHasATree` in `cmd/devlore-docs/main_test.go`, the four names
      from `programs`, and `star/gh/issues/report.md` as the extension page.

### Phase 4: The words

- [x] §12, 2026-09-21: four programs, in-process from `NewRootCmd`, generated by `make build`.

### Phase 5: Gates and the three numbers

- [x] `make check` and `make test-scenario`, both exit 0, 2026-09-21, after three rounds of the rename's collateral:
      two revive findings (the package comment, the blank imports' justification), `root_test.go`'s `Chdir` one level
      short, and my qualifier replace catching `star.` in an external test package and in an extension id string.
- [x] Coverage per package and total, complexity, code size. 2026-09-21: total 63.0%; `cmd/devlore-docs` 85.8%,
      `cmd/star/star` 82.8%, `cmd/star/extension` 71.7%; `run` 94.1%, `NewRootCmd` 92.3%, `NewLoader` 100%; nothing in
      the change over 8. Size: `cmd/star/main.go` 41 (from 619), `root.go` 590, `root_test.go` 373, `embed.go` 15,
      `devlore-docs/main.go` 72, `main_test.go` 35.

### Phase 6: Installed and exercised

- [x] `make build` leaves `docs/cli/{writ,lore,star,devlore-test}` and the four root pages; `make install` 2026-09-21;
      `docs/cli/star.md` has the same four sections as `writ.md` with a Child Commands table, and `docs/cli/star/` holds
      a page per command `star --help` lists, `help` and `version` skipped by design for every program;
      `star/gh/issues/{audit,report}.md` are the extensions' pages.
- [x] End to end, 2026-09-21, build `a83653fe-dirty`, both snapshotted first (logs under `build/e2e/`, `…-lane16.log`).
      `danoble-ud24-1.local`: install 0; base and team `unset` twice, `set` by URL (git-named clones), by URL again and
      by path `unchanged`; personal by path; bare `deploy` 64; `deploy common noblefactor-ops` **166 files** under
      `stop` and again under `replace`; `upgrade` 0. `danoble-wd11-3.local`: the same `repo` steps; deploy under
      `--allow-dirty` (that machine's personal checkout is dirty): `stop` refuses one occupant, `replace` **66 links**;
      `upgrade` 0.
- [ ] The owner's row: `writ deploy` or `writ upgrade` before the pull request opens.

### Phase 7: Closure

- [x] #787's three acceptance boxes ticked with evidence 2026-09-21; the pull request closes it; the epic's last lane;
      this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `run` into a temporary directory writes a tree for each of `writ`, `lore`, `star`, `devlore-test` | unit, `devlore-docs` | a program is not walked |
| 2 | star's tree contains an extension command's page (`gh/issues/report.md`) | unit, `devlore-docs` | the root was built without its extensions |
| 3 | the existing walker tests | unit, `devlore-docs` | the move broke the walker |
| 4 | `make build` leaves `docs/cli/` with four trees | installed | the build does not generate |
| 5 | `make test-scenario` green | scenario | star's root move broke the binary |
| 6 | the VM sequence | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/fix/787-cli-reference.md` | Create |
| `cmd/star/star/*` -> `cmd/star/extension/*` | Rename (the extension host); `Loader`, `Registry` |
| the four importers of the extension host | Modify: import path |
| `cmd/star/extensions/embed.go` | Create: `package bundled` |
| `cmd/star/extensions.go` | Remove: its embed moves |
| `cmd/star/star/root.go` (new package) | Create: `NewRootCmd` and what moves from `main.go` |
| `cmd/star/main.go` | Modify: `main` and `run` remain |
| `cmd/star/root_test.go` | Move or modify, with what it tests |
| `cmd/devlore-docs/main.go` | Modify: four roots |
| `cmd/devlore-docs/main_test.go` | Create: Requirement 4 |
| `Makefile` | Modify: `build` generates for the host; help lines |
| `docs/architecture/10-command-line-interface.md` | Modify: §12 |

## Open questions

1. **The layout of Requirement 1** -- ruled 2026-09-21: `extension`. `starruntime` was what the alias said; a merged
   `star/star` was the smallest diff; the package's own types say extension host, so that is its name.
2. **`make build` generating on every build** costs one generator run (about a second) and writes into an
   ignored directory; CI's `make build PLATFORM=all` does it too. Stated so it is not a surprise.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lane 16
- [#787](https://github.com/NobleFactor/devlore-cli/issues/787) -- the bug
- [#834](https://github.com/NobleFactor/devlore-cli/issues/834) -- the feature
- [#743](https://github.com/NobleFactor/devlore-cli/issues/743) -- `man` is the one route to man pages; `star docs markdown` removed as `make docs`'s duplicate
