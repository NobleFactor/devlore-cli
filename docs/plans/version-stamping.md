---
title: "Version stamping: one stamped location, two reporting surfaces"
issue: TBD — created after review
status: draft
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

### Phase 1: centralize

- [ ] Create the owning package with the three variables and their defaults
- [ ] Commands stop declaring their own triples; `RootConfig` takes its values from the one package
- [ ] `Makefile` and `.goreleaser.yaml` name the new symbol path — one `-X` per value, not per binary

### Phase 2: the two surfaces

- [ ] `rootCmd.Version` set, with `SetVersionTemplate` producing the one-line form
- [ ] `version` writes through `cmd.OutOrStdout()`
- [ ] Tests cover all three forms: `--version`, `version`, `version --short`

### Phase 3: the guard

- [ ] The build asserts the built binary reports the version the build computed
- [ ] Verify the release path too — `.goreleaser.yaml` carries its own build stanza, and it is the
      one that ships

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

1. **Which package owns the values?** Three candidates:
   - **`pkg/application`** — a public package for what an application-shaped program needs. Fits if
     more than version metadata will live there; a package created to hold three strings is thin, and
     `pkg/` is public API surface for a build concern that is not.
   - **`cmd/internal/devlore`** — already imported by everything CLI-side, so no new package. But it
     was just chartered to hold *only* what the commands share about locations, and version metadata
     is not a location. Adding it re-mixes the concern that charter separates.
   - **`cmd/internal/version`** (unproposed) — a new CLI-internal package named for exactly what it
     holds. No public surface, no mixing, one import. Costs one more package.
   **Recommendation: `cmd/internal/version`**, unless `pkg/application` is already planned to carry
   more, in which case it wins on not multiplying packages.
2. **Does `devlore-test`, `devlore-docs`, `devlore-index` or `devlore-inventory` need the surfaces?**
   All four are stamped by the same `LDFLAGS` today. Whether they get `--version` too, or only the
   three user-facing commands do, is a decision about what those tools are.

## Related Documents

- [windows-native-permissions.md](./windows-native-permissions.md) — phase 7 moved `internal/cli` to
  `cmd/internal/cli` and rewrote these `-X` paths, which is how the defect surfaced
