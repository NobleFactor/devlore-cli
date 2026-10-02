---
title: "Lane 22: configuration is one file, and no program writes it"
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
file states it.

**Ruled 2026-10-02, open question 1: (d). We behave like git and leave configuration alone.** No program writes or
removes the user's configuration: `self install` writes neither `config.d/` nor `config.yaml`, and `self uninstall`
removes neither. Every program holds its own defaults in code, verified the same day (Current State), and the
documentation the seed files carried moves to the guides.

**This plan covers this one lane.** Anything found while working it stops the work and goes to the owner.

## Issue 1009

Bug, epic UnifiedConfiguration (#441), feature #456. The order in PR C, ruled 2026-10-02: lane 7's code first, then
lane 8, then this lane and lanes 23 to 28 (#1010 to #1015), which change the same configuration code.

## Goals

1. No program writes or removes the user's configuration: `self install` writes no `config.d/` and no `config.yaml`,
   and `self uninstall` removes neither.
2. One reader: every program reads `~/.config/devlore/config.yaml` and nothing else.
3. Every program holds its own defaults in code; the settings are documented in the guides, never in a seeded file.
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
| defaults held in code | ✅ | 2026-10-02: 33 reads over 17 keys, enumerated in full; every key has a default in code except `lore onboard`'s AI provider, which refuses by design. All four programs run with an empty configuration home, and none writes one |
| settings documented and unread | ❌ | `lore.ai_provider`, lore's preferences and sources, `lore.quiet`, `devlore-test.receipt_format`, `secrets`: ruled a bug 2026-10-02, removed by #1013 (lane 26) |
| the documents | ⚠️ | the four defaults headers, `schema/schema.go:22`, `:28` and `:39`, and `docs/architecture/9-star-extensions.md:219` name `config.d`; `configuration.md` corrected 2026-10-02 |
| the install record | -- | configuration is outside it (#933), so a re-install retires nothing in the configuration home |
| #780 | -- | open, on schedule #894: its requirement 1 says each program "generates its own default config", and its acceptance list asks `self install` to produce one; the ruling contradicts both |
| this Mac | -- | `config.yaml` is the owner's (19 B, `writ.vars`); `config.d/` holds `lore.yaml` and `star.yaml` (the current defaults, unedited), `writ.yaml` (3278 B, unlike the current default) and `test.yaml` (0 B, 2026-03-02; no program is named `test` today) |

## Requirements

### Requirement 1: no program writes the configuration

`initDevloreConfig` goes. `self install` neither creates the configuration home nor writes `config.yaml` or
`config.d/`; the cache step stays, since the cache is the program's. `removeDevloreConfig` and its call go:
`self uninstall` leaves the configuration home as it found it.

### Requirement 2: one reader

`ViperConfig.UseSharedConfig`, `InitViper`'s `~/.config/<program>/config.yaml` branch and `BindFlags`'s unprefixed
form are deleted: `InitViper` reads `~/.config/devlore/config.yaml`, and `BindFlags` binds `<program>.<flag>`. Under
the ruling a per-program file is what git's include model would govern if a need appeared, never a second search path.

### Requirement 3: the defaults files go

The five embedded defaults (`schema.SharedDefaultConfig` and the four per-program `*DefaultConfig`),
`schema/defaults/*.yaml`, and `ConfigInfo.DefaultConfig` with the root's field that feeds it are deleted. What they
document that a program reads moves to the guides, each setting into the guide of the feature it configures
(proposed): `writ.scopes` and `writ.vars` to `docs/guides/writ/manage-environments.md`, `writ.segments` to
`docs/guides/selectors.md` § Extra segments, and `self.channel` and `self.prerelease` to
`docs/guides/getting-started.md` § Upgrade. What no program reads is not carried (#1013).

### Requirement 4: `config edit` creates the file on demand

git writes a user's configuration only on the user's command; asked to edit one that does not exist
(`git config --global --edit`), it creates it with a commented template. `config edit` does the same: on a missing
`config.yaml` it creates the file, holding only a comment that names the guides, and opens it. `config set` and
`config unset` write the file on the user's command, as they do today. Derived from the ruling; confirmed at review.

### Requirement 5: the documents

- `docs/architecture/9-star-extensions.md:219` stops naming `config.d/star.yaml` as star's configuration.
- `docs/architecture/configuration.md` § The command surface says no program writes the configuration and each holds
  its own defaults. § One user configuration file loses its "Today" paragraph, and `configuration.status.md` its
  discrepancy, in the commit that lands the fix.
- `self install`'s help, step 4, says it creates the cache only.
- #780's requirement 1 and its default-config acceptance item are amended to the ruling. #780 is a lane of schedule
  #894, so the amendment is the owner's.

### Requirement 6: the style gate

Every touched Go file passes `star lint go-style`; `gofmt`, `make check` and `make test-scenario` are green.

## Implementation Phases

### Phase 1: The plan

- [x] `configuration.md` § One user configuration file, its correction of star's sources, and the status document's
  discrepancy, written 2026-10-02 on the owner's direction.
- [x] Open question 1 ruled (d), and open question 2 resolved by #1013, 2026-10-02.
- [ ] This document reviewed and chartered.

### Phase 2: The code

- [ ] Requirements 1 to 4, with the unit tests below.

### Phase 3: The documents and the gate

- [ ] Requirements 5 and 6, with Requirement 3's guide sections.

### Phase 4: The VM

From the built binaries, bundled and copied: after `self install` of all four programs the configuration home is as it
was before, present or absent; writ reads a setting placed there by hand; `self uninstall` leaves it as it was.

- [ ] `danoble-ud24-1.local` (linux/arm64)

### Phase 5: Closure

- [ ] The lane's commit on this branch. PR C carries `Closes #1009`.

## Test Plan

- **Unit, `cmd/internal/cli`:** `self install`, into a temporary prefix and configuration home, writes nothing into the
  configuration home, whether it exists or not; `self uninstall` leaves a `config.yaml` exactly as it was.
- **Unit, `config edit`:** on a missing file it creates one holding only the comment; on a present file it changes
  nothing before the editor opens.
- **Unit, `InitViper`:** a setting in `config.yaml` is read; the same setting in `config.d/<program>.yaml` or in
  `~/.config/<program>/config.yaml` is not.
- **Scenario:** `make test-scenario` green.
- **VM:** Phase 4.

## Migration Path

No code retires the files already on machines: under the governing principle there is no legacy handling. The owner
deletes `~/.config/devlore/config.d/` by hand on each machine, after moving anything edited there into `config.yaml`.
On this Mac, `writ.yaml` is the one file that differs from the current default, and the one worth reading first. An
existing `config.yaml` stays the user's; nothing removes it.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/selfinstall.go` | Modify | Requirement 1 |
| `cmd/internal/cli/viper.go` | Modify | Requirement 2 |
| `cmd/internal/cli/root.go` | Modify | Requirement 2; its `DefaultConfig` field goes (Requirement 3) |
| `cmd/internal/cli/config.go` | Modify | `ConfigInfo.DefaultConfig` goes; Requirement 4 |
| `schema/schema.go` | Modify | the five embeds go (Requirement 3) |
| `schema/defaults/*.yaml` | Delete | Requirement 3 |
| `cmd/devlore-test/devloretest/root.go`, `cmd/lore/lore/root.go`, `cmd/star/star/root.go`, `cmd/writ/writ/root.go` | Modify | stop passing `DefaultConfig` (Requirement 3) |
| `cmd/internal/cli/*_test.go` | Create/Modify | the test plan |
| `docs/guides/writ/manage-environments.md`, `docs/guides/selectors.md`, `docs/guides/getting-started.md` | Modify | Requirement 3's documentation |
| `docs/architecture/configuration.md`, `docs/architecture/configuration.status.md` | Modify | written 2026-10-02; the interim removed with the fix |
| `docs/architecture/9-star-extensions.md` | Modify | Requirement 5 |

## Open questions

1. **Where each program's documented defaults go, now that `config.d` is not written.** **Ruled 2026-10-02: (d).**
   Today each program embeds two defaults files: the shared one, seeded into `config.yaml`, and its own, written to
   `config.d/<program>.yaml` and used by `config edit` to seed `config.yaml` when it is absent.
   - **(a) One seeded file.** The five defaults files become one, holding the shared sections and every program's
     section, each commented. Every program's `self install` seeds it when `config.yaml` is absent, and `config edit`
     seeds the same file; `ConfigInfo.DefaultConfig` and the four per-program embeds go. For: the first install of
     any program documents every key in the one file the user edits; the four programs behave identically, which is
     #780's uniformity requirement; no merge logic, and less code. Against: a section added in a later release never
     reaches a machine whose `config.yaml` exists, as today and as with any seeded file; #780's "generates its own
     default config" becomes "seeds the one default config".
   - **(b) Seed the shared file only.** `config.yaml` is seeded from the shared defaults, as today; each program's
     keys are documented in its guide and by `config schema`; the per-program files go. For: closer to git; the
     documentation is versioned with the program and never stale on a machine. Against: the commented examples a
     user reads today, writ's scopes and segments above all, leave the machine for the guides.
   - **(c) Each program appends its section at install when `config.yaml` lacks one.** For: keeps "its own default
     config" literally, and a program installed later documents itself in an existing file. Against: an installer
     edits a file the user owns, on every install; uninstall cannot take a section back once it is edited; appended
     text must not collide with a key the user wrote another way.
   - **(d) Write no user configuration at all, as git does.** No seeded file, no record of one, nothing to remove.
     For: nothing is written that the user did not ask for, and nothing has to decide who owns a shared file or
     whether it changed. Against: the documentation of the settings lives only in the guides.

   I recommended (a). The owner ruled (d): "we behave like git. we leave config alone."

2. **lore's documented keys.** **Resolved 2026-10-02:** the owner ruled every documented setting that no code reads a
   bug, and #1013 (lane 26) removes them. Under (d) lore's defaults file is deleted, so this lane carries none of them
   into the guides.

## Related Documents

- [#1009](https://github.com/NobleFactor/devlore-cli/issues/1009) -- the issue
- [configuration.md § One user configuration file](../../architecture/configuration.md#one-user-configuration-file)
  -- the owner's direction, 2026-10-02
- [#1010](https://github.com/NobleFactor/devlore-cli/issues/1010) to
  [#1015](https://github.com/NobleFactor/devlore-cli/issues/1015) -- lanes 23 to 28, found checking the defaults for
  this lane's ruling
- [#780](https://github.com/NobleFactor/devlore-cli/issues/780) -- the four programs' `self` uniformity, on #894
- [#933](https://github.com/NobleFactor/devlore-cli/issues/933) -- `self install` replaces its record; configuration
  is outside the record
- [926-scope-flag.md](../task/926-scope-flag.md) -- lane 7, where this was found
