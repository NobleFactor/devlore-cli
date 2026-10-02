---
title: "Lane 22: configuration is one file; self install stops writing config.d"
issue: https://github.com/NobleFactor/devlore-cli/issues/1009
status: draft
created: 2026-10-02
updated: 2026-10-02
---

# Plan: Lane 22 of the writ lifecycle schedule

## Summary

Lane 22 of [#916](https://github.com/NobleFactor/devlore-cli/issues/916), placed in PR C by the owner on 2026-10-02 and
worked on lane 6's branch beside lanes 7 and 8. Found planning lane 7: `self install` writes
`~/.config/devlore/config.d/<program>.yaml` for each of the four programs, and no program reads it, so an edit there
changes nothing. The owner's direction the same day: **configuration is centralized; if a real need to separate it
appears, the separation follows git's include model.** `docs/architecture/configuration.md` § One user configuration
file states it. This lane makes the code agree: one file, read by every program, and nothing written that nothing
reads.

**This plan covers this one lane.** Anything found while working it stops the work and goes to the owner.

## Issue 1009

Bug, epic UnifiedConfiguration (#441), feature #456. Waits on nothing. Proposed order in PR C: this lane's code next,
then lane 7's phases 2 to 6, then lane 8.

## Goals

1. `self install` writes no file that no program reads: no `config.d/`, no `config.d/<program>.yaml`.
2. One reader: every program reads `~/.config/devlore/config.yaml` and nothing else.
3. Each program's documented defaults have one home, as open question 1 rules.
4. Every document names the one file; none names `config.d`.
5. Every touched Go file passes `star lint go-style`.

## Current State

Read 2026-10-02 at `01634491`, the branch's merge base.

| Component | Status | Notes |
| --- | --- | --- |
| `self install`, config step | ❌ | `initDevloreConfig` (`cmd/internal/cli/selfinstall.go:1088`) creates `config.d/` and writes `config.d/<program>.yaml` when absent; it seeds `config.yaml` from `schema.SharedDefaultConfig` when absent |
| `self uninstall` | ⚠️ | `removeDevloreConfig` (`:760`) removes `config.d/<program>.yaml`, then the directory once it is empty |
| `cli.InitViper` | ⚠️ | reads `~/.config/devlore/config.yaml`; its `UseSharedConfig: false` branch reads `~/.config/<program>/config.yaml`, and no caller passes false |
| `config edit` | ⚠️ | with no `config.yaml`, seeds it with the program's `DefaultConfig`, whose header names `config.d/<program>.yaml` |
| the defaults | ⚠️ | five embeds in `schema/schema.go`: the shared one, and one per program keyed under the program's name |
| lore's defaults | ⚠️ | `lore.ai_provider` and `lore.sources` uncommented; no code declares `lore.ai_provider`, and lore reads `lore.model.provider` and `lore.model.model` (`cmd/lore/lore/commands.go:774-783`); open question 2 |
| the documents | ⚠️ | the four defaults headers, `schema/schema.go:22`, `:28` and `:39`, and `docs/architecture/9-star-extensions.md:219` name `config.d`; `configuration.md` corrected 2026-10-02 |
| the install record | -- | configuration is outside it (#933), so a re-install retires nothing in the configuration home |
| #780 | -- | open, on schedule #894: its requirement 1 says each program "generates its own default config", and it edits `selfinstall.go` too |
| this Mac | -- | `config.yaml` is the owner's (19 B, `writ.vars`); `config.d/` holds `lore.yaml` and `star.yaml` (the current defaults, unedited), `writ.yaml` (3278 B, unlike the current default) and `test.yaml` (0 B, 2026-03-02; no program is named `test` today) |

## Requirements

### Requirement 1: self install writes the one file

`initDevloreConfig` creates the configuration home and seeds `config.yaml` as open question 1 rules, and writes
nothing else: `config.d/` is never created. `removeDevloreConfig` and its call go. `self uninstall` leaves the
configuration home alone, since `config.yaml` is the user's.

### Requirement 2: one reader

`ViperConfig.UseSharedConfig`, `InitViper`'s `~/.config/<program>/config.yaml` branch and `BindFlags`'s unprefixed
form are deleted: `InitViper` reads `~/.config/devlore/config.yaml`, and `BindFlags` binds `<program>.<flag>`. Under
the ruling a per-program file is what git's include model would govern if a need appeared, never a second search path.

### Requirement 3: the documented defaults

As open question 1 rules. What moves keeps its comments, and documents only keys a program reads (open question 2).

### Requirement 4: the documents

- `schema/schema.go`'s doc comments and the defaults headers name the file the content lands in.
- `docs/architecture/9-star-extensions.md:219` stops naming `config.d/star.yaml` as star's configuration.
- `docs/architecture/configuration.md` § The command surface states the ruled answer in its default-config bullet.
  § One user configuration file loses its "Today" paragraph, and `configuration.status.md` its discrepancy, in the
  commit that lands the fix.
- `self install`'s help, step 4, is checked against what the step now does.

### Requirement 5: the style gate

Every touched Go file passes `star lint go-style`; `gofmt`, `make check` and `make test-scenario` are green.

## Implementation Phases

### Phase 1: The plan

- [x] `configuration.md` § One user configuration file, its correction of star's sources, and the status document's
  discrepancy, written 2026-10-02 on the owner's direction.
- [ ] This document reviewed and chartered; open questions 1 and 2 ruled.

### Phase 2: The code

- [ ] Requirements 1 to 3, with the unit tests below.

### Phase 3: The documents and the gate

- [ ] Requirements 4 and 5.

### Phase 4: The VM

From the built binaries, bundled and copied: after `self install` of all four programs the configuration home holds
`config.yaml` alone; writ reads a setting placed there; `self uninstall` leaves the home as it was.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 5: Closure

- [ ] The lane's commit on this branch. PR C carries `Closes #1009`.

## Test Plan

- **Unit, `cmd/internal/cli`:** the config step, into a temporary configuration home, writes `config.yaml` as ruled
  and creates no `config.d`; run again over an edited `config.yaml`, it leaves the edit in place; `self uninstall`
  leaves the home unchanged.
- **Unit, `InitViper`:** a setting in `config.yaml` is read; the same setting in `config.d/<program>.yaml` or in
  `~/.config/<program>/config.yaml` is not.
- **Scenario:** `make test-scenario` green.
- **VM:** Phase 4.

## Migration Path

No code retires the files already on machines: under the governing principle there is no legacy handling. The owner
deletes `~/.config/devlore/config.d/` by hand on each machine, after moving anything edited there into `config.yaml`.
On this Mac, `writ.yaml` is the one file that differs from the current default, and the one worth reading first.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirement 1 |
| `cmd/internal/cli/viper.go`, `cmd/internal/cli/root.go` | Modify | Requirement 2 |
| `cmd/internal/cli/config.go` | Modify | `config edit` seeds what open question 1 rules |
| `schema/schema.go`, `schema/defaults/*.yaml` | Modify | Requirement 3 |
| `cmd/devlore-test/devloretest/root.go`, `cmd/lore/lore/root.go`, `cmd/star/star/root.go`, `cmd/writ/writ/root.go` | Modify | the `DefaultConfig` each passes, as open question 1 rules |
| `cmd/internal/cli/*_test.go` | Create/Modify | the test plan |
| `docs/architecture/configuration.md`, `docs/architecture/configuration.status.md` | Modify | written 2026-10-02; the interim removed with the fix |
| `docs/architecture/9-star-extensions.md` | Modify | Requirement 4 |

## Open questions

1. **Where each program's documented defaults go, now that `config.d` is not written.** Today each program embeds two
   defaults files: the shared one, seeded into `config.yaml`, and its own, written to `config.d/<program>.yaml` and
   used by `config edit` to seed `config.yaml` when it is absent.
   - **(a) One seeded file.** The five defaults files become one, holding the shared sections and every program's
     section, each commented. Every program's `self install` seeds it when `config.yaml` is absent, and `config edit`
     seeds the same file; `ConfigInfo.DefaultConfig` and the four per-program embeds go. For: the first install of
     any program documents every key in the one file the user edits; the four programs behave identically, which is
     #780's uniformity requirement; no merge logic, and less code. Against: a section added in a later release never
     reaches a machine whose `config.yaml` exists, as today and as with any seeded file; #780's "generates its own
     default config" becomes "seeds the one default config".
   - **(b) Seed the shared file only.** `config.yaml` is seeded from the shared defaults, as today; each program's
     keys are documented in its guide and by `config schema`; the per-program files go. For: closest to git, which
     seeds no user file at all; the documentation is versioned with the program and never stale on a machine.
     Against: the commented examples a user reads today, writ's scopes and segments above all, leave the machine for
     the guides.
   - **(c) Each program appends its section at install when `config.yaml` lacks one.** For: keeps "its own default
     config" literally, and a program installed later documents itself in an existing file. Against: an installer
     edits a file the user owns, on every install; uninstall cannot take a section back once it is edited; appended
     text must not collide with a key the user wrote another way.

   **Recommendation: (a).** It is the one file the ruling names, documented at first install, and no installer edits
   the user's file afterwards.

2. **lore's documented keys** (asked after question 1). lore's defaults set `lore.ai_provider.model`,
   `lore.ai_provider.preferences` and `lore.sources` uncommented. No code declares `lore.ai_provider`; lore reads
   `lore.model.provider` and `lore.model.model` (`cmd/lore/lore/commands.go:774-783`), and `cmd/internal/config`
   declares `lore.preferences` and `lore.sources` beside a root `model:`. Moving that content, this lane can correct
   it to the keys lore reads, or carry it unchanged and commented, and file the mismatch as its own bug under #456.

## Related Documents

- [#1009](https://github.com/NobleFactor/devlore-cli/issues/1009) -- the issue
- [configuration.md § One user configuration file](../../architecture/configuration.md#one-user-configuration-file)
  -- the owner's direction, 2026-10-02
- [#780](https://github.com/NobleFactor/devlore-cli/issues/780) -- the four programs' `self` uniformity, on #894
- [#933](https://github.com/NobleFactor/devlore-cli/issues/933) -- `self install` replaces its record; configuration
  is outside the record
- [926-scope-flag.md](../task/926-scope-flag.md) -- lane 7, where this was found
