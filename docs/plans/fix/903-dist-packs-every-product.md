---
title: "make dist packs every product, and star ships with its extensions"
issue: https://github.com/NobleFactor/devlore-cli/issues/903
status: complete
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

`install.sh` and `install.ps1` take the product list from the archive: every file at its root is a product.
`DEVLORE_TOOLS` keeps `all` and accepts any one product name.

**Found while implementing:** both installers ran `<tool> self-install --prefix=...`, and no product has that command.
It is `self install [prefix]` (`cmd/internal/cli/selfinstall.go:72,88`). `writ self-install` answers `unknown command`
and exits 64, and the installers swallowed that as a warning, so man pages and completions were never installed from a
release. `self install` also copies the binary to `<prefix>/bin` itself, and `star`'s version copies its extensions
from `<exeDir>/../share/star/extensions`.

So each product installs itself. The installer extracts to `pkg/`, moves each product into `pkg/bin`, and runs
`pkg/bin/<tool> self install <prefix> --unattended` from `pkg/`. `star` then finds the archive's `pkg/share` at
`<exeDir>/../share`, and the installer copies nothing by hand. A failed `self install` is now fatal, since the product
is not installed without it. `install.ps1` checks `$LASTEXITCODE`, because `try`/`catch` never sees a native exit code.

### Requirement 4: XDG on Windows

Ruled 2026-09-22: "we aren't using the Windows location for app installs. we're going to stick with XDG conventions
on windows." `install.ps1`'s default prefix becomes `~/.local` on every platform, so binaries land in `~/.local/bin`
and extensions in `~/.local/share/star/extensions`, the `xdg.DataPath` the loader already probes. The loader is
unchanged. `install.ps1`'s help and usage text change with the default.

## Implementation Phases

### Phase 1: Commit

- [x] This plan, first
- [x] Requirement 1 and Requirement 2 in the Makefile -- 2026-09-22
- [x] Requirement 3 in both installers -- 2026-09-22
- [x] Requirement 4, per the ruling, in the same installer commit, since it is two lines of `install.ps1` -- 2026-09-22
- [x] `make check`: `vet`, `lint` (golangci-lint 2.13.2, installed natively through winget for this), `complexity`,
      `verify-ldflags` and `test` all exit 0 on this machine. `shell-lint` can't run here, because `shfmt` and
      `shellcheck` aren't installed and `shellcheck` has no native ARM64 build, so CI's run on the PR is its gate --
      2026-09-22

### Phase 2: Verify before merge

- [x] `make dist-all PLATFORM=linux/arm64` on this machine: the `tar.gz` holds exactly 26 files, `lore`, `star`,
      `writ` and the 23 extension files under `share/star/extensions`, and the check passes. The `zip` path, used for
      Windows, is not run here because this machine has no `zip`; the release runner, `ubuntu-latest`, has it, and the
      Phase 3 release proves it -- 2026-09-22
- [x] The check fails when the recipe drops a product: a copy of the Makefile whose `tar` line packs only `lore writ`
      stopped with `does not hold exactly the products and star's extensions` / `missing: star`, exit 2. The real recipe
      passed above -- 2026-09-22
- [x] The installers' mechanism, from a staged `pkg/bin` and `pkg/share` of the `windows-arm64` builds: `self install`
      into a scratch prefix exits 0 for all three, `<prefix>/bin` holds all three binaries, and star copied all 23
      extension files to `<prefix>/share/star/extensions`. `self install` creates `~/.config/devlore` and
      `~/.cache/devlore` files only when missing, and left the existing ones unchanged. `install.ps1` parses, with the
      same 33 PSScriptAnalyzer findings as `develop`, all pre-existing (30 `PSAvoidUsingWriteHost`); `install.sh` passes
      `bash -n`, and CI's shell-lint is the gate for `shfmt` and `shellcheck` -- 2026-09-22

### Phase 3: Verify after merge (the live path)

- [x] The release from `develop` at `463f9780`, `v0.1.0-dev.20260922122246`: all six archives hold 26 files each --
      `lore`, `star`, `writ` and the 23 extension files under `share/star/extensions`. That includes both Windows
      `zip`s, the path this host could not build locally -- 2026-09-22
- [x] On DANOBLE-WD11-3, `install.ps1` from that release installed all three products at `v0.1.0-dev.20260922122246`,
      and `star` copied its 23 extensions to `~/.local/share/star/extensions`. `star devlore --help` resolves, listing
      `actions`, `knowledge`, `model`, `package` and `test`.

      **`star gh issues report` does not run, for a reason outside this plan.** The base layer's
      `com.noblefactor.ops.GitHub` extension calls `shell.exec`, which the shell provider runs as `sh -c`
      (`pkg/op/provider/shell/provider.go:53`), and this host keeps no `sh` on PATH. It fails with `exec: "sh":
      executable file not found in %PATH%`. That is the rewrite #799 and #801 describe: argv execution needs no shell
      present or chosen. Installing `star` did not cause it; it made it visible, this being the first time `star` ran
      here -- 2026-09-22

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
