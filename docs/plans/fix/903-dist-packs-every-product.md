---
title: "make dist packs every product, and star ships with its extensions"
issue: https://github.com/NobleFactor/devlore-cli/issues/903
status: approved
created: 2026-09-22
updated: 2026-09-22
---

# Plan: make dist packs every product

## Issue 903

Lane 7 of NobleFactor/noblefactor-ops#217. `PRODUCTS := lore star writ`, but `dist-all` builds and packs a hand-written
pair (Makefile:527-534):

```
go build ... -o dist/writ$ext ./cmd/writ
go build ... -o dist/lore$ext ./cmd/lore
tar -czf ... writ$ext lore$ext   |   zip ... writ$ext lore$ext
```

Every archive on the release from `a83653fe` holds `writ` and `lore` and nothing else. A machine without a checkout
can't install `star`, so every rule that routes through it can't be followed there.

## Current State

Read 2026-09-22 on `develop` at `75350893`.

- **`make dist` ships.** `.github/workflows/release.yaml:65` runs it, and `gh release create ... dist/*` publishes the
  result. Makefile:477 says `.goreleaser.yaml` "is NOT the release path -- nothing runs it today", and
  [release-path.md](../release-path.md) (#453, draft) keeps goreleaser as a later adoption. That answers the issue's
  third bullet: goreleaser is not the shipping path, and adopting it is #453's work.
- **`star` needs more than its binary.** `.goreleaser.yaml:89-98`: `//go:embed` covers `cmd/star/extensions`, but not
  `star/extensions`, where the `com.noblefactor.devlore.*` commands live, "so a binary alone answers `unknown command
  \"devlore\"`". goreleaser packs those 23 files at `share/star/extensions`. `make dist` packs none of them.
- **The installers hand-list the products too.** `install.sh:299-318` and `install.ps1:260-278` move `writ` and `lore`
  out of the extracted archive and nothing else. `DEVLORE_TOOLS` accepts `all`, `writ` or `lore`. A `star` in the
  archive would be extracted and then thrown away.
- **Where extensions are looked for.** The loader (`cmd/star/extension/loader.go:237-240`) probes
  `${GIT_WORKSPACE_ROOT}/star/extensions`, then `xdg.DataPath("star", "extensions")` (`XDG_DATA_HOME`, else
  `~/.local/share`), then `/usr/local/share/star/extensions`. `star self install` (`cmd/star/star/root.go:303-350`)
  copies from `<exeDir>/../share/star/extensions` to `<prefix>/share/star/extensions`. These agree only when
  `<prefix>` is `~/.local`. `install.ps1`'s default prefix on Windows is `~/AppData/Local/DevLore`, so on Windows the
  default install puts extensions where the loader never looks. This is release-path.md's "prefix assumption"
  defect, and it bites Windows by default.

## Goals

1. Every archive holds every product in `$(PRODUCTS)`, plus `star`'s extensions at `share/star/extensions`, the layout
   `.goreleaser.yaml` already describes.
2. The build fails when an archive's contents differ from what it should hold.
3. The installers install whatever products the archive carries, so a product added to `PRODUCTS` needs no installer
   edit.
4. On this machine, a `star` installed from a release answers `star gh issues report`, and a `star devlore` command.
   `gh issues report` is the base layer's `com.noblefactor.ops.GitHub` extension, which writ already deploys to
   `~/.local/share/star/extensions`, where the loader looks, so it needs only the binary. The `devlore` commands need
   the extensions this plan packs.

## Requirements

### Requirement 1: `dist-all` loops over `$(PRODUCTS)`

The per-platform loop builds each product in `$(PRODUCTS)` with the same `GOOS`, `GOARCH`, `CGO_ENABLED=0` and
`$(LDFLAGS)` as today. It copies `star/extensions` to `share/star/extensions` in the staging directory and packs the
products and `share/` into the same archive name, `tar.gz` or `zip` as today. The forced host `make build` stays first
and unconditional ([version-stamping.md](../version-stamping.md)). The checksum step is unchanged.

### Requirement 2: The archive check

After each archive is written, its file list is compared with the expected set: each product with the platform's
extension, plus every tracked file under `star/extensions` at `share/star/extensions/...`. On any difference the
recipe exits non-zero and names the missing and the extra entries.

### Requirement 3: The installers

`install.sh` and `install.ps1` install every executable at the archive root that matches a known product name, taken
from the archive rather than from a hand-written pair. `DEVLORE_TOOLS` keeps `all`, and accepts any product name. The
archive's `share/` tree is copied to `<prefix>/share`. Then each installed product's `self install` runs, as today.

### Requirement 4: XDG on Windows

Ruled 2026-09-22: "we aren't using the Windows location for app installs. we're going to stick with XDG conventions
on windows." `install.ps1`'s default prefix becomes `~/.local` on every platform, so binaries land in `~/.local/bin`
and extensions in `~/.local/share/star/extensions`, the `xdg.DataPath` the loader already probes. The loader is
unchanged. `install.ps1`'s help and usage text change with the default.

## Implementation Phases

### Phase 1: Commit

- [ ] This plan, first
- [ ] Requirement 1 and Requirement 2 in the Makefile
- [ ] Requirement 3 in both installers
- [ ] Requirement 4, per the ruling
- [ ] `make check`: build, vet, lint, test and shell-lint, as CI runs them

### Phase 2: Verify before merge

- [ ] `make dist PLATFORM=windows/arm64` on this machine: the zip holds `lore.exe`, `star.exe`, `writ.exe` and the 23
      extension files under `share/star/extensions`, and the check passes
- [ ] The check fails when a product is removed from the loop by hand, then passes again when it's restored
- [ ] Installing from that zip into a scratch prefix yields three binaries and the extensions, and `star` answers a
      `com.noblefactor.devlore.*` command

### Phase 3: Verify after merge (the live path)

- [ ] The next release from `develop` lists `star` in every platform archive
- [ ] On DANOBLE-WD11-3, `install.ps1` from that release installs `star`, `star gh issues report` runs, and a
      `star devlore` command resolves

## Out of Scope

- Adopting goreleaser: #453.
- The Homebrew, winget and MacPorts packaging that goreleaser describes.
- `README.md` and `LICENSE` in the archive, which goreleaser packs and `make dist` doesn't. They could be added in one
  line, but they aren't what this issue is about.

## Open Questions

- [x] **Requirement 4:** XDG conventions on Windows; `install.ps1` defaults to `~/.local`. Ruled 2026-09-22.
- [x] **Scope:** the installers are in, and "install.ps1 needs an update" was ruled 2026-09-22.
- [x] **goreleaser and the build drop:** discussed 2026-09-22 and belongs to #453, not this lane. The owner's model is
      build once into a well-known drop, and everything downstream consumes those bytes. Importing prebuilt binaries
      (`builder: prebuilt`) is GoReleaser Pro only, per its docs, so the open-source edition can't consume a drop.
      This lane's archive contents and installers hold whichever packager #453 chooses.
