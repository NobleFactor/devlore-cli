---
title: "star keeps its data under devlore/, like writ"
issue: https://github.com/NobleFactor/devlore-cli/issues/918
status: active
created: 2026-09-22
updated: 2026-09-22
---

# Plan: star's data moves under devlore/

## Issue 918

Lane 15 of NobleFactor/noblefactor-ops#217. star is part of devlore, and its config and cache already say so:
`~/.config/devlore/config.d/star.yaml`, `~/.cache/devlore/star`. Its data does not.

## Current State

Read 2026-09-22 on `develop` at `c3033073`.

| Path builder | Result |
| --- | --- |
| `devlore.DataPath(...)` (`cmd/internal/devlore/devlore.go:78`) | `~/.local/share/devlore/...` |
| `devlore.WritLayersDir()`, `devlore.WritReposDir()` | `~/.local/share/devlore/writ/{layers,repos}` |
| **`cmd/star/extension/loader.go:240`** | **`xdg.DataPath("star", "extensions")` → `~/.local/share/star/extensions`** |

Every site that names the path:

| File | Line | What |
| --- | --- | --- |
| `cmd/star/extension/loader.go` | 240 | the user and system probes |
| `cmd/star/extension/extension.go` | 23-24 | the `Source` constants' documentation |
| `cmd/star/star/root.go` | 329 | `self install`'s target |
| `cmd/star/star/root.go` | 381 | `findExtensionsDir`'s `<exeDir>/../share/...` |
| `Makefile` | 523, 543 | the release archive's staging |
| `.goreleaser.yaml` | 94, 98 | the same, for the day goreleaser is adopted |

Outside this repository: **noblefactor-ops' base layer** deploys `com.noblefactor.ops.GitHub` to the old user path
through writ. It has to move in step, in its own PR, or `star gh issues report` stops resolving -- the report agent
rule 3 requires.

The project-local probe is the repository root -- found by walking up for `.git` (`config.GitWorkspaceRoot`), not an
environment variable, though `extension.go:22` documents it as `${GIT_WORKSPACE_ROOT}` -- plus `star/extensions`. It is
a path inside a checkout, not user data, and it does not move.

## Goals

1. star's data lives under `devlore/`, built through `devlore.DataPath`, like writ's.
2. Nothing breaks on a machine that still has extensions at the old path, until it is redeployed or reinstalled.
3. The base layer moves with it, so `star gh issues report` keeps working.

## Requirements

### Requirement 1: The new paths

| What | Was | Becomes |
| --- | --- | --- |
| User data | `~/.local/share/star/extensions` | `~/.local/share/devlore/star/extensions` |
| System | `/usr/local/share/star/extensions` | `/usr/local/share/devlore/star/extensions` |
| `self install` target | `<prefix>/share/star/extensions` | `<prefix>/share/devlore/star/extensions` |
| `findExtensionsDir` | `<exeDir>/../share/star/extensions` | `<exeDir>/../share/devlore/star/extensions` |
| Release archive | `share/star/extensions` | `share/devlore/star/extensions` |

The loader takes the user path from `devlore.DataPath("star", "extensions")` rather than `xdg.DataPath`, so the
convention is expressed once.

### Requirement 2: The old paths stay probes, deprecated

After each new path, the loader still probes the old one, user and system alike, and `findExtensionsDir` still looks
for an archive's `share/star/extensions` after `share/devlore/star/extensions`. A machine
installed before this release keeps its extensions at the old path, and the base layer's move lands in a second PR
on a second repository: a flag day would break `star gh issues report` in the gap between them. The probe is marked
deprecated in the code, with the issue that removes it.

### Requirement 3: The `Source` constants

`SourceUser` and `SourceSystem` document the new paths. If Requirement 2 keeps the old probe, it reports as
`SourceUser` too: it is the same kind of location, at a path being retired.

### Requirement 4: Tests

`cmd/star/extension`'s tests cover the search-path list. They gain: the new user path comes before the old one, and
both are present while the deprecation lasts.

## Implementation Phases

### Phase 1: Commit

- [x] This plan, first
- [x] Requirements 1 and 3, the Go changes -- 2026-09-22
- [x] Requirement 2, per the ruling: both old paths kept, each after its replacement, removed by #920 -- 2026-09-22
- [x] The Makefile and `.goreleaser.yaml` staging, and the archive check's expected list -- 2026-09-22
- [x] Requirement 4, and the design document: `star-extensions.md` gains the scope-and-ownership model and the
      corrected Search Path table, and drops the `\` fiction -- 2026-09-22

### Phase 2: Verify before merge

- [x] `make vet`, `make lint` and `make test` all pass -- 2026-09-22
- [x] `make dist-all PLATFORM=linux/arm64`: 26 files, the three binaries and 23 extension files at
      `share/devlore/star/extensions`, nothing at the old layout, and the recipe's own check passed -- 2026-09-22
- [x] From a staged archive layout, `self install` placed 23 files at `<prefix>/share/devlore/star/extensions` and
      none at the old path; with `XDG_DATA_HOME` pointed at `<prefix>/share`, `star devlore --help` resolves through
      the new user probe -- 2026-09-22

### Phase 3: The base layer, and the live path

- [ ] noblefactor-ops PR: `com.noblefactor.ops.GitHub` moves to `Home/common/.local/share/devlore/star/extensions`
- [ ] `writ deploy common`, then `star gh issues report` resolves from the new path. (It still fails for the
      `shell.exec` reason of #799 and #801; "resolves" here means the command is found.)
- [ ] `Remove-BrokenLinks` clears the old links, and the empty `~/.local/share/star` is gone

## Out of Scope

- `star`'s config and cache, already under `devlore/`.
- The `shell.exec` failure that keeps `star gh issues report` from running: #799, #801.
- writ's own paths, which are already right.

## Open Questions

- [x] **Requirement 2: two passes,** ruled 2026-09-22. Pass 1, this lane: add the new paths, keep the old. Pass 2, its
      own issue: remove the old.
- [x] **Project scope stays,** ruled 2026-09-22. The model is settled: **scopes come from star** -- project, user,
      system, and embedded as a packaging convenience -- and **ownership comes from writ**. A repository's maintainer
      chooses which scope their extensions inhabit. This repository's own `star/extensions` is project scope because it
      is build tooling: `make generate` calls `star devlore actions generate` and `knowledge-extract.yaml` calls
      `devlore knowledge extract`, both from a bare checkout with nothing deployed. The base layer's
      `com.noblefactor.ops.GitHub` is user scope, which is why writ deploys it there.
