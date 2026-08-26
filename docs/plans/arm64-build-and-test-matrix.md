---
title: "arm64 build and test matrix"
issue: https://github.com/NobleFactor/devlore-cli/issues/681
status: in-progress
created: 2026-08-25
updated: 2026-08-25
---

# Plan: arm64 build and test matrix

## Summary

Two build/CI chores, related but distinct:

1. **Build the entire set.** `ALL_PLATFORMS` already names all six targets, and
   `make build PLATFORM=all` already produces them — but CI never asks for them. The
   `quality-gate` job runs `make build` (host only) and `make build-all`, which sweeps
   **GOOS and not GOARCH**. arm64 compilation is therefore unverified on every push, while
   goreleaser ships arm64 binaries at release. An arm64-only break surfaces at release time.
2. **Run the entire set.** The `test` and `scenario` matrices are
   `[ubuntu-latest, macos-latest, windows-latest]` — three legs covering three of the five
   runnable pairs. Linux and Windows arm64 are never executed.

Cross-compiling proves the code *builds* for a target. A native runner proves it *runs*
there. Neither substitutes for the other, and today CI does neither for arm64 outside
macOS.

No production code changes. Two workflow jobs grow, one Makefile invocation widens, and one
test leg loses the race detector because Go does not support it there.

## Goals

1. **Every shipped target compiles on every push** — six platforms, matching what goreleaser
   releases.
2. **Every runnable target executes natively** — five runner legs, so arm64 behavior is
   observed rather than assumed.
3. **Change nothing else.** A leg that fails is a finding, not a regression introduced here.
4. **Keep the ruleset story accurate**: more legs means more check contexts, which the
   platform-test-matrix plan's phase 4 must name correctly.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `ALL_PLATFORMS` | ✅ Complete | `darwin/{amd64,arm64} linux/{amd64,arm64} windows/{amd64,arm64}` — all six |
| `make build PLATFORM=all` | ✅ Works | Pure Go, `CGO_ENABLED=0`, no per-platform toolchains; ~41s vs ~13s host-only |
| goreleaser | ✅ Complete | `goos: [linux, darwin, windows] × goarch: [amd64, arm64]` — ships all six |
| CI `make build` | ⚠️ Host only | `quality-gate` runs on `ubuntu-latest`, so this builds linux/amd64 alone |
| CI `make build-all` | ⚠️ GOOS only | `for os in linux darwin windows: GOOS=$os go build ./...` — **no GOARCH sweep**, emits no binaries |
| **arm64 compilation in CI** | ❌ **Unverified** | Nothing on the push path compiles for arm64; only release does |
| `test` matrix | ⚠️ Three legs | `[ubuntu-latest, macos-latest, windows-latest]`, `make test-race` |
| `scenario` matrix | ⚠️ Three legs | Same three, `make test-scenario` |
| darwin/arm64 execution | ✅ Covered | `macos-latest` is Apple Silicon — never a gap |
| linux/arm64 execution | ❌ Absent | `ubuntu-24.04-arm` available, free-tier eligible |
| windows/arm64 execution | ❌ Absent | `windows-11-arm` available, free-tier eligible |
| arm64 runners, private repos | ✅ GA 2026-01-29 | `windows-11-arm`, `ubuntu-24.04-arm`, `ubuntu-22.04-arm` |
| Race detector, windows/arm64 | ❌ Unsupported by Go | Upstream constraint — Requirement 3 |

## Requirements

### Requirement 1: CI builds the entire set

The Makefile already says what is true of this codebase:

> The build comes to the machine: one host produces every platform's binaries, and
> installing anywhere is a copy. The codebase is pure Go — zero `import "C"` — so the whole
> matrix cross-compiles from here with GOOS/GOARCH alone, no per-platform toolchains and no
> build VMs.

So the fix is an argument, not an architecture: `quality-gate` asks for all six.

`build-all` is a *compile* sweep across GOOS that emits no binaries; it exists to catch
`_darwin.go`/`_windows.go`/`_linux.go` files that a single-GOOS pass never sees, and its
first local run found three such files. It is worth keeping for that purpose. But it sweeps
one axis, and arm64 lives on the other. Either widen `build-all` to GOARCH as well, or have
CI run `make build PLATFORM=all` and get real binaries for the trouble. This plan prefers
the latter: it exercises the same code path releases use, and ~28 additional seconds on one
runner is not a cost worth optimizing.

**This is the higher-value half of the plan.** It closes a gap that exists today on every
push, on one runner, for seconds — where the native-runner work below adds runners to
observe behavior that is, so far, only theoretically at risk.

### Requirement 2: There is no `-latest` label for arm64

The matrix cannot be expressed as "all platforms latest". GitHub publishes arm64 runners as
**version-pinned labels only**; no `ubuntu-latest-arm` or `windows-latest-arm` exists.

| Target | Label | Architecture |
| --- | --- | --- |
| darwin | `macos-latest` | arm64 (M1) |
| linux/amd64 | `ubuntu-latest` | x64 |
| linux/arm64 | `ubuntu-24.04-arm` | arm64 — **pinned, no alias** |
| windows/amd64 | `windows-latest` | x64 |
| windows/arm64 | `windows-11-arm` | arm64 — **pinned, no alias** |

Also available: `ubuntu-26.04-arm`, `ubuntu-22.04-arm`, `windows-11-vs2026-arm`.

**Ruled: `ubuntu-24.04-arm`.** Not a compromise against "all platforms latest" — it *is* the
arm64 counterpart of latest. `ubuntu-latest` still resolves to Ubuntu 24.04, with no announced
migration to 26.04, and `ubuntu-26.04`/`ubuntu-26.04-arm` are marked **preview** ("some software
can be unstable on the new platform"). Matching versions across the pair also means the arm64 leg
varies architecture alone; vary the OS release as well and any failure has two candidate causes.

The Windows pair cannot be matched that way. `windows-latest` is Windows Server 2025 while
`windows-11-arm` is a client OS, and there is no Windows Server arm64 runner. That asymmetry
belongs to the platform, not to this plan, and is worth remembering when a Windows leg diverges.

A consequence worth stating: pinned labels drift. Unlike `ubuntu-latest`, these two will not
follow GitHub's image promotions, so they need an owner and a periodic bump. That is the
price of arm64 coverage today, not a defect in this plan.

**The build set is six; the runner set is five.** `darwin/amd64` compiles but is never
executed. That asymmetry is deliberate, and Apple's timeline settles it rather than leaving
it to taste:

- macOS **Tahoe 26** (September 2025) is the last Intel-supporting release.
- macOS **27** ships September 2026 — Apple Silicon only.
- Intel Macs receive **security updates through roughly fall 2028**.
- The last Intel hardware (2019 Mac Pro, 2020 MacBook Pro and iMac) left sale through mid-2023,
  so a real population persists to 2028–2030 on ordinary service life.

The decisive asymmetry is Rosetta's direction: it translates x86_64 → arm64, **never the
reverse**. An Intel Mac cannot run an arm64 binary under any emulation, so dropping
`darwin/amd64` does not degrade those users — it strands them with no fallback. No other
target in the matrix has that property.

Hence: **keep building it, do not add an Intel runner.** The build costs one more
GOOS/GOARCH pair in a pure-Go cross-compile, on a runner already paid for. A runner leg costs
macOS billing at 10× ([platform-test-matrix.md](./platform-test-matrix.md) ruling 3) to
observe a shrinking population on a known schedule. Revisit dropping the build target around
**fall 2028**, when security updates end — not before.

Note the Intel runner label set is now closed: `macos-15-intel` and `macos-26-intel` exist,
and there will never be a `macos-27-intel`. That is the complete menu, permanently.

### Requirement 3: windows/arm64 cannot run the race detector

The Go race detector supports:

> `linux/amd64`, `linux/ppc64le`, `linux/arm64`, `linux/s390x`, `linux/loong64`,
> `freebsd/amd64`, `netbsd/amd64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.

`windows/arm64` is absent. The `test` job runs `make test-race`, so that leg runs
`make test` instead. This is an upstream limitation: attempting `-race` there fails at the
toolchain, and no CI configuration changes it. `linux/arm64` **is** supported, so
`ubuntu-24.04-arm` runs `make test-race` unchanged.

The matrix therefore stops being uniform, which means `matrix.include` with an explicit
per-leg flag rather than a bare `matrix.os` list:

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - { os: macos-latest,     race: true  }   # darwin/arm64
      - { os: ubuntu-latest,    race: true  }   # linux/amd64
      - { os: ubuntu-24.04-arm, race: true  }   # linux/arm64
      - { os: windows-latest,   race: true  }   # windows/amd64
      - { os: windows-11-arm,   race: false }   # windows/arm64 — race unsupported by Go
```

The step selects on `matrix.race`, with the reason named in the workflow so the next reader
does not "fix" the inconsistency.

### Requirement 4: The existing per-leg accommodations must survive

Already in the `test` job, and they must extend rather than be re-derived:

- **macOS ships GNU make 3.81** — the last GPLv2 release, so it will never advance. The job
  installs `make` via brew and prepends `gnubin`. Unaffected here; the new legs are Linux and
  Windows.
- **`-race` requires cgo and a C toolchain**, set explicitly as `CGO_ENABLED: "1"` rather
  than relying on a platform-varying default. `ubuntu-24.04-arm` needs the same. The
  `windows-11-arm` leg skips `-race`, so it does not need mingw-w64 — fortunate, since that
  image's toolchain story is the least proven of the five.

### Requirement 5: More legs means more check contexts

Each leg reports its own check, so `test (ubuntu-latest)` becomes five contexts, and the same
for `scenario`. The `develop` ruleset currently requires exactly one context, `quality-gate`,
which is why the `test` job is non-blocking today — deliberately, and without
`continue-on-error`, so red stays visible as triage data.

Phase 4 of [platform-test-matrix.md](./platform-test-matrix.md) is a ruleset change naming
the `test (…)` contexts. It must name **five**, not three. This plan does not change the
ruleset; it changes the names that phase will have to use, and says so here so the two do not
drift.

## Implementation Phases

### Phase 1: Build the entire set (status: complete)

- [x] `quality-gate` builds all six: the `Build` step is now `make build PLATFORM=all`
- [x] `build-all` dropped from CI. `build PLATFORM=all` covers the 120 of 122 packages reachable
      from `./cmd` across all six pairs rather than three GOOS at the host's arch, and `vet-all`
      still type-checks all 122 including test files — which `go build ./...` never did. The two
      packages outside cmd's reach (`pkg/op/provider/elevator`, `pkg/op/server`) keep their
      per-GOOS coverage. The Makefile target remains for local use.
- [x] Added `host` as a PLATFORM keyword beside `all`; both resolve through `expand`
- [x] `PLATFORM` stays empty by default — a non-empty default is never overridden by a target, so
      `dist`'s default would become unreachable and releases would go host-only silently. Defaults
      moved to the call sites as keywords: `$(call select,host)` for build, `$(call select,all)`
      for dist.
- [x] Verified by expansion: `build` → `darwin/arm64`; `build PLATFORM=all` → all six;
      `dist` → all six; `dist PLATFORM=host` → `darwin/arm64`

**Files**: `.github/workflows/ci.yaml` - Modify

### Phase 2: Run the entire set (status: complete)

- [x] `test` job: `matrix.include`, five legs, `race` flag per leg
- [x] `test` job: two steps gated on `matrix.race` — `make test-race` where supported,
      `make test` on `windows-11-arm`, with Go's supported-platform list quoted at the conditional
- [x] `scenario` job: same five legs, bare `os` list since scenarios never use `-race`
- [x] `fail-fast: false` retained on both
- [x] `quality-gate` untouched, on `ubuntu-latest`

**Files**: `.github/workflows/ci.yaml` - Modify

### Phase 3: Triage what the new legs reveal (status: pending)

The point is to learn something, so the first run is data, not pass/fail.

- [ ] Record each new leg's result; failures are findings to file, not blockers on this PR
- [ ] Confirm `actions/setup-go@v5` resolves a toolchain on both new images
- [ ] Confirm the scenario harness's `writBinary` resolves `build/<goos>-<goarch>/` on the
      arm64 legs — it composes from `runtime.GOOS`/`runtime.GOARCH`, so it should hold, but
      has never run on either image
- [ ] Confirm symlink creation works on `windows-11-arm`. platform-test-matrix flags symlinks
      as the Windows risk; an arm64 image is a fresh instance of that question, not a settled
      one

### Phase 4: Record the pin owner (status: pending)

- [ ] Note beside each pinned label that it does not track GitHub's image promotions and must
      be bumped deliberately
- [ ] Decide whether `ubuntu-26.04-arm` supersedes `ubuntu-24.04-arm` once it has mileage

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `.github/workflows/ci.yaml` | Modify | Phases 1–2 — full-set build, five-leg test and scenario |
| `docs/plans/arm64-test-matrix.md` | Create | This plan |
| `docs/plans/platform-test-matrix.md` | Modify | Phase 4's context list becomes five names |
| `Makefile` | Modify | Only if `build-all` is widened to GOARCH (Open Questions) |

## Related Documents

- [platform-test-matrix.md](./platform-test-matrix.md) — the three-platform matrix this
  extends; its phase 4 ruleset change must name the new contexts
- [writ-deploy-scenario.md](./writ-deploy-scenario.md) — the scenario harness the new legs
  drive
- [arm64 standard runners in private repositories](https://github.blog/changelog/2026-01-29-arm64-standard-runners-are-now-available-in-private-repositories/) — GA 2026-01-29, free-tier eligible
- [GitHub-hosted runners reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) — the label table; source for Requirement 2
- [Go race detector](https://go.dev/doc/articles/race_detector) — supported-platform list; source for Requirement 3

## Open Questions

- [x] ~~`build-all`'s fate?~~ **Resolved:** retired from CI in favour of `make build PLATFORM=all`;
      `vet-all` retains the per-GOOS sweep over all 122 packages, test files included.
- [x] ~~`ubuntu-24.04-arm` or `ubuntu-26.04-arm`?~~ **Resolved (Requirement 2):** 24.04-arm.
      `ubuntu-latest` is still 24.04 and 26.04 is a preview image, so 24.04-arm *is* latest's
      arm64 counterpart and keeps the pair version-matched.
- [x] ~~`darwin/amd64` builds but never runs — deliberate, or a gap to close with
      `macos-15-intel`?~~ **Resolved (Requirement 2):** deliberate. Keep the build target
      because Rosetta cannot run arm64 on Intel, so dropping it strands those users outright;
      skip the runner because macOS bills at 10×. Revisit the build target around fall 2028,
      when Intel security updates end.
- [ ] Does the `windows-11-arm` image carry a C toolchain at all? Not needed here, since that
      leg skips `-race`, but it bounds what can ever run there.
- [ ] Should `scenario`'s five legs also start non-blocking, matching the `test` job's
      posture, or does its green record on three platforms justify gating immediately?
