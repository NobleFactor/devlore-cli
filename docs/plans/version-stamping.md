---
title: "Version stamping: one stamped location, two reporting surfaces"
issue: TBD — created after review
status: in-progress — all three phases landed 2026-08-16; `--version` tests and the release-path guard remain
created: 2026-08-15
updated: 2026-08-15
---

# Plan: version stamping

## Summary

Build-time version stamping does not work and never has: the `-X` flags in `Makefile` and
`.goreleaser.yaml` name symbols that do not exist, so the linker silently ignores them and every
binary — including every release binary — reports its compiled-in defaults. This plan centralizes the
stamped values in one package so a single `-X` path serves all binaries, adds a `--version` flag
alongside the existing `version` command in the shape docker uses, and adds the build-time check that
would have caught the defect.

## Goals

1. **One stamped location.** One package holds `Version`, `Commit`, and `BuildDate`; one `-X` path
   per value in the build stanza, for all seven binaries.
2. **Two surfaces, docker's split.** `--version` answers in one line; `version` prints the detail.
3. **A stamp that cannot silently die.** The build verifies that what it stamped is what the binary
   reports.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `-X` targets in `Makefile:26` | ❌ Broken | Names `…/cmd/internal/cli.Version`, `.Commit`, `.BuildDate` |
| `-X` targets in `.goreleaser.yaml:34-36`, `:52-54` | ❌ Broken | The same three names, so releases are affected too |
| Package-level `Version`/`Commit`/`BuildDate` in `cmd/internal/cli` | ❌ Absent | Every occurrence is a **struct field** |
| Per-command version variables | ✅ Present | Three separate unexported triples |
| `version` subcommand | ✅ Working | Reports whatever it is given, which today is the defaults |
| `--version` flag | ❌ Absent | Cobra's `rootCmd.Version` is never set, so no flag is generated |
| Stamp verification | ❌ Absent | Nothing compares the built binary's output to the intended version |

**The symbols do not exist.** `Version`, `Commit` and `BuildDate` appear in `cmd/internal/cli` only as
struct fields — [`cmd/internal/cli/root.go:25`](../../cmd/internal/cli/root.go) on `RootConfig` and
[`cmd/internal/cli/version.go:15`](../../cmd/internal/cli/version.go) on `VersionInfo`. `-X` sets
package-level string variables; against a field name it matches nothing, and the linker reports
nothing.

**The real variables are per-command and unexported**, one triple each:
[`cmd/writ/writ/root.go:16`](../../cmd/writ/writ/root.go),
[`cmd/lore/lore/root.go:16`](../../cmd/lore/lore/root.go), and
[`cmd/star/main.go:30`](../../cmd/star/main.go). Each is passed into `cli.RootConfig`, which builds
the `VersionInfo` the `version` command prints.

**The proof is `BuildDate`.** `build/writ version` reports `dev` / `none` / `unknown`. The first two
are also what the Makefile computes when git is unavailable, so they prove nothing on their own — but
`BUILD_DATE ?= $(shell date -u …)` always resolves, and the binary still says `unknown`. The value
was computed, passed to the linker, and dropped.

## Requirements

### R1 — one package owns the stamped values

Three exported package-level variables with sensible defaults, so an unstamped build (`go run`, an
IDE build, a developer's `go build`) still reports something honest rather than empty strings.

Each command keeps naming *itself* — the binary name is not a stamped value — and stops declaring its
own `version`/`commit`/`buildDate` triple.

### R2 — `--version` is one line, as docker's is

`docker --version` prints `Docker version 27.3.1, build ce12230` and exits: one line, no sections,
no daemon contact. The analog:

```console
$ writ --version
writ version 0.4.0, build ed6f468
```

Cobra generates the flag as soon as `rootCmd.Version` is set, and `SetVersionTemplate` fixes the
exact text. `-v` is **not** a shorthand for it here: this repository's commands already use `-v`
elsewhere, and the collision is worse than the missing convenience.

### R3 — `version` keeps the detail block, as docker's does

`docker version` prints sectioned detail — Version, API version, Go version, Git commit, Built,
OS/Arch. Ours has no client/server split, so it stays one block, which is what
[`cmd/internal/cli/version.go:26`](../../cmd/internal/cli/version.go) already prints. Two corrections
while the surface is open:

- It writes with `fmt.Printf` to `os.Stdout` rather than `cmd.OutOrStdout()`, so its output cannot be
  captured through the command the way every other command's can.
- `--short` prints the bare version, which is `--version` under another name. Keep it — it is the
  scriptable form — but document the three as one family rather than two accidents.

### R4 — the build proves the stamp landed

The defect survived because nothing ever compared the stamped value to the reported one. A check in
the build path — build, run `--version`, assert it contains `$(VERSION)` — turns a silent
mis-target into a failed build. This is the requirement that keeps R1 from rotting the next time a
package moves.

### R5 — an unstamped build is not an empty build

`VERSION` resolves through `git describe … || echo "dev"`, so it is never empty today. That is luck,
not design: `-X pkg.Version=` sets the variable to the empty string, which would make a mis-set
`DEVLORE_VERSION` produce a binary reporting nothing at all. Either the stanza omits an empty value or
the reporting substitutes the default.

## Implementation Phases

### Phase 1: centralize — **complete 2026-08-16**

- [x] `pkg/application/version.go` holds `Version`, `Commit` and `BuildDate` with their defaults
- [x] All four commands (`writ`, `lore`, `star`, `devlore-test`) stop declaring their own triples
- [x] `Makefile` and `.goreleaser.yaml` name `pkg/application` — one `-X` per value, not per binary

**Proved by the binaries, not by the build succeeding.** After `make build`, all four report the same
stamp:

```
Version:    v0.1.0-dev.20260815050413-1-gf980693b-dirty
Commit:     f980693b
Built:      2026-08-16T18:24:33Z
```

That is the first time these flags have bound to anything. Before this, every binary — including
every release artifact — reported `dev` / `none` / `unknown`, because the stanzas named
`internal/cli.Version`, where `Version` was only ever a struct field.

### Phase 2: the two surfaces — **complete 2026-08-16**

- [x] `cli.AddVersionFlag` sets `rootCmd.Version` and the one-line template; installed on all four
      root commands
- [x] `version` writes through `cmd.OutOrStdout()`
- [ ] Tests covering all three forms — **not written**; see Remaining below

```console
$ build/writ --version
writ version v0.1.0-dev.20260815050413-1-gf980693b-dirty, build f980693b
```

`lore`, `star` and `devlore-test` print the same shape.

**`star` lost its own spelling.** It carried a hand-rolled `version` command printing
`star <version> (<commit>) built <date>` — one tool reporting a different shape is the drift
centralizing exists to end, so it now uses `cli.NewVersionCmd` like the others. Nothing asserted on
the old format, checked before changing it.

### Phase 3: the guard — **complete 2026-08-16**

- [x] `make build` compares what the build stamped against what the binary prints, and fails with the
      cause named
- [ ] The release path is **not** covered — see Remaining

**Proved by watching it refuse**, not by watching it pass. Building with a deliberately wrong symbol
path:

```console
$ make build LDFLAGS='-ldflags "-X …/pkg/application.NoSuchSymbol=x"'
ERROR: version stamp did not bind.
  build computed: v0.1.0-dev.20260815050413-1-gf980693b-dirty
  binary reports: dev
  The -X paths in LDFLAGS name symbols that do not exist — check pkg/application.
make: *** [Makefile:102: build] Error 1
```

## Remaining

Two items, both deliberate rather than forgotten:

1. **Tests for the three surfaces.** `version` and `version --short` have tests that predate this
   work and still pass; `--version` has none. The template is built with `fmt.Sprintf` and rendered
   by cobra, so the case worth pinning is that the flag exists and prints one line.
2. **The release path is unguarded.** `.goreleaser.yaml` carries its own build stanza with its own
   `-X` flags, and `make build`'s check cannot see it. A goreleaser build with the paths wrong would
   ship silently — exactly the failure this plan exists to end, in the one place that reaches users.
   `goreleaser build --snapshot` plus the same comparison would close it.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| the owning package | Create | `Version`, `Commit`, `BuildDate` + defaults |
| `Makefile` | Modify | `-X` paths, plus the R4 assertion |
| `.goreleaser.yaml` | Modify | The same three `-X` paths, both build stanzas |
| `cmd/internal/cli/root.go` | Modify | Source the values centrally; set `rootCmd.Version` |
| `cmd/internal/cli/version.go` | Modify | Write through `cmd.OutOrStdout()` |
| `cmd/writ/writ/root.go` | Modify | Drop the local triple |
| `cmd/lore/lore/root.go` | Modify | Drop the local triple |
| `cmd/star/main.go` | Modify | Drop the local triple |

## Open Questions

1. ~~**Which package owns the values?**~~ **Answered 2026-08-16: `pkg/application`.**

   My recommendation of a new `cmd/internal/version` rested on a false premise — that
   `pkg/application` would be a package created to hold three strings. It already exists and already
   holds the tool's identity: the program name plus the variable resolver's flag, config, and
   override maps, constructed by each command from its own CLI plumbing and read by the framework
   through the runtime environment. A program's version belongs beside its program name, and no new
   package is needed.

   The `cmd/internal/devlore` option stays rejected for the reason given: that package was chartered
   to hold only what the commands share about *locations*, and a version is not a location.
2. **Does `devlore-test`, `devlore-docs`, `devlore-index` or `devlore-inventory` need the surfaces?**
   All four are stamped by the same `LDFLAGS` today. Whether they get `--version` too, or only the
   three user-facing commands do, is a decision about what those tools are.

## Related Documents

- [windows-native-permissions.md](./windows-native-permissions.md) — phase 7 moved `internal/cli` to
  `cmd/internal/cli` and rewrote these `-X` paths, which is how the defect surfaced
