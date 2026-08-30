---
title: "The Release Path"
issue: https://github.com/NobleFactor/devlore-cli/issues/453
status: draft
created: 2026-08-29
updated: 2026-08-29
---

# Plan: The Release Path

## Summary

`make dist` is the release path. `.goreleaser.yaml` is staged for the day we adopt it, and is not
run by anything today. Neither of those facts is written down anywhere, and the omission has
already cost: a reader found the config, matched its `builds: writ, lore` against a two-entry
release tarball, and spent an afternoon editing a file that ships nothing.

This plan records the state of the world and the conditions for adoption. **It proposes no change
to how releases are built.** We are not ready to release; goreleaser is adopted when we are.

## State of the world

### What ships today

```
push to develop / main / v* tag
  → .github/workflows/release.yaml
      → make dist            DEVLORE_VERSION=<computed>
          → dist-all         forced host `make build` (stamp proof), then
                             go build × 6 pairs, tar.gz / zip
          → checksums        shasum -a 256
      → gh release create ... dist/*
```

`install.sh` — served publicly from devlore.noblefactor.com — defaults to `latest`, *including
prereleases*, so every push to `develop` produces artifacts that a `curl | bash` will install. The
release path is live, not theoretical.

### What `.goreleaser.yaml` is

Staged, and deliberately kept correct. `version-stamping.md` fixed the `-X` flags in the Makefile
**and** in `.goreleaser.yaml`, and lists as a completed item:

> `Makefile` and `.goreleaser.yaml` name `pkg/application` — one `-X` per value, not per binary

The Makefile says the same in a comment at the `dist` target: *"nothing runs it today … It is kept
correct anyway: whoever adopts goreleaser inherits."* It is a configuration waiting for a decision,
not an abandoned artifact.

`star` is staged in it by this plan's companion change: a `star` build across the same six pairs,
and `star/extensions/**` shipped to `share/star/extensions` so a released binary can answer
`devlore` at all.

## Why not now

We are not releasing yet. Adoption swaps a live, working path for one that has never run, and it
would have to happen alongside the changes below rather than before them.

## What adoption requires

### Version derivation moves

`release.yaml` computes a version for **non-tag** builds — `v0.1.0-dev.<timestamp>` on every push to
`develop` — and passes it as `DEVLORE_VERSION`. Goreleaser derives `{{ .Version }}` from the git tag
and refuses to release without one. Either we tag before releasing, or we keep synthesizing and pass
it in. That is a release-model decision, not a port.

### The stamp proof must survive

`dist-all` forces `make build PLATFORM=$(HOST_GOOS)/$(HOST_GOARCH)` before it cross-compiles, so the
`-X` flags are proven to bind to real symbols on a binary that can actually run. That guards the
failure which shipped unnoticed until 2026-08-16, described in `version-stamping.md`. Goreleaser has
no equivalent; it becomes a `before:` hook calling the Makefile, or it is lost.

### The after: hook, and a fifth packaging target

`.goreleaser.yaml` declares `after: hooks: [./packaging/macports/generate-portfile.sh {{ .Version }}]`,
and that script **does** exist -- executable, Apache-2.0 headed, its own comment saying "Called by
GoReleaser as a post hook". So MacPorts is a fifth distribution target beyond the four in the config
body, and it is the only one whose tooling is actually written.

It has never run, since nothing runs goreleaser. Whether the Portfile it generates is correct is
therefore unverified.

### The prefix assumption

Two different path lists resolve star's extensions, and they only agree by coincidence:

- `findExtensionsDir()` — used by `self install` to find the source — probes
  `<exeDir>/../share/star/extensions`
- the **loader** — used at runtime — probes `${GIT_WORKSPACE_ROOT}/star/extensions`, then
  `XDG_DATA_HOME/star/extensions`, then `/usr/local/share/star/extensions`

Shipping to `share/star/extensions` works because `install.sh` defaults to `--prefix=$HOME/.local`,
which makes `<prefix>/share` equal to `XDG_DATA_HOME`. With `--prefix=/opt/devlore`, extensions
install to `/opt/devlore/share/star/extensions`, which **nothing loads**.

That is a pre-existing defect, independent of goreleaser, and it should be fixed before a released
`star` is relied upon.

## What is kept, and what goes

| Today | On adoption |
| --- | --- |
| `dist-all` build loop | replaced by `builds:` |
| `dist-all` tar/zip | replaced by `archives:` |
| `checksums` | replaced by `checksum:` |
| `release.yaml`'s `gh release create` | replaced by `release:` |
| `dist-clean` | kept — useful locally |
| **the stamp proof** | **kept, as a `before:` hook** |

## What adoption unlocks

What `make dist` cannot do and goreleaser already describes: deb and rpm via `nfpms`, a Homebrew
formula, a winget manifest, and a changelog from the tag range.

All three packaging targets are currently `skip_upload: true`, and **`NobleFactor/homebrew-tap` and
`NobleFactor/winget-pkgs` do not exist**. No release has ever carried a `.deb`, `.rpm`, or formula.
The configuration is intent, not capability — which is the honest reason adoption has not been
urgent.

## Open Questions

- [ ] Tag-driven releases, or keep synthesizing dev versions and pass them to goreleaser?
- [ ] Does `packaging/macports/generate-portfile.sh` produce a correct Portfile? It exists and
      has never executed.
- [ ] Does the prefix defect get fixed before or with adoption?
