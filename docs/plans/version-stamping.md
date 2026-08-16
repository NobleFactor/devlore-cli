---
title: "Version stamping: one stamped location, two reporting surfaces"
issue: TBD — created after review
status: complete — all three phases landed 2026-08-16, both surfaces tested, both build paths guarded
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
- [x] Tests covering all three forms. `--version` gets two: that it answers in **one line** carrying
      no build date (the property that distinguishes it from `version`), and that `-v` still means
      verbose — cobra would take `-v` for `--version` given the chance

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
- [x] The release path is covered — **and it is not goreleaser**

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

**This plan named the wrong release path.** It asserted that `.goreleaser.yaml` "carries its own
build stanza, and it is the one that ships." It does not ship anything.
[`release.yaml:65`](../../.github/workflows/release.yaml) runs **`make dist`**, and `.goreleaser.yaml`
is referenced by nothing but its own header comment and two forward-looking lines in
`wiki/Releasing.md`. The published asset names match `dist-all`'s own pattern, not goreleaser's.

So the release path is guarded where it actually is:

- **`dist-all` now depends on `build`** — not for its binaries, which it cross-compiles itself, but
  for the stamp check. It uses the same `LDFLAGS` and the same `VERSION`, and it cannot run what it
  produces for other platforms; `build` proves on the host that those flags bind to real symbols.
- **`verify-ldflags`, in `make check`**, asserts `.goreleaser.yaml` names the same package. That file
  is dormant, not dead: whoever adopts goreleaser inherits whatever is in it, and a wrong path there
  would fail exactly the way this whole defect failed. Agreement is a sufficient check precisely
  because the Makefile's own paths are proved to bind by `build`.

## Two platform failures, two different causes

The guard's first CI run failed on macOS and Windows. They looked like one problem and were not —
which is why each got read rather than assumed.

### macOS — GNU make 3.82+ is now a declared prerequisite

`bash: -c: line 1: syntax error: unexpected end of file`. `.ONESHELL:` is a GNU make **3.82**
feature; macOS ships **3.81** — the last GPLv2 release, so it will never advance — and older make
ignores the directive *silently*, running each recipe line in its own shell and splitting a
multi-line `if` mid-statement.

The repository already had multi-line recipes in `complexity`, `dist-all`, `lint-all` and
`build-all`. All of them had only ever run on ubuntu, so the dependency had been invisible;
`build` runs everywhere, so it was the first to meet 3.81. A developer on a stock Mac running
`make dist` would have hit the same wall.

**Ruled 2026-08-16: 3.82+ is a prerequisite, not a constraint to write around.** So, rather than
keeping the accommodating one-liners:

- The Makefile **declares it** — an `ifneq` on `MAKE_VERSION` that refuses to run with the fix in the
  message, instead of a shell error naming nothing.
- CI **conforms** — the macOS legs of all three jobs install GNU make and put it first on `PATH`.
  ubuntu and windows already satisfy it.
- `README.md` states it under Building, with the `brew install make` line.
- The two new recipes are back in their readable multi-line form.

Team tooling — GNU make included — is headed for a lore package list for devlore engineers, which is
where the prerequisite will eventually be installed rather than merely documented.

### Windows — the guard caught a real defect, in the Makefile's own variables

Windows make handled the multi-line recipe fine. The guard fired, and it was right to:

```
ERROR: version stamp did not bind.
  build computed: 8a62b17-dirty
  binary reports: 8a62b17
```

`?=` defines **recursively expanded** variables, so `$(shell git describe …)` and `$(shell date …)`
re-run at *every reference*. `LDFLAGS :=` captured one set of answers at parse time; the check
named `$(VERSION)` again and got another. On Windows the two differed because codegen rewrites
tracked `*.gen.go` files mid-build (line endings), so `--dirty` changed its answer between the link
and the check.

`VERSION`, `COMMIT` and `BUILD_DATE` are now flattened to simply-expanded after their `?=`
definitions: one evaluation per make run, so the stamped value and anything compared against it
cannot disagree. Overrides from the environment or command line still win — verified with
`make build VERSION=v9.9.9-test`.

**This is the guard earning its place on its first run.** The variables have behaved this way since
long before this plan; nothing compared two evaluations, so nothing noticed.

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
