---
title: "The writ lifecycle surface: reconcile, scopes, and the vocabulary settlement"
issue: https://github.com/NobleFactor/devlore-cli/issues/762
status: in-progress
created: 2026-08-31
updated: 2026-09-01
---

# Plan: The writ lifecycle surface

## Summary

`writ` and `lore` run the same four-operation lifecycle over different subjects, and they disagree about what
to call it. This plan settles the vocabulary, restores `writ reconcile`, and makes `--scope` mean something.
Three of the four operations already share a name across both programs; `writ status` is the sole divergence,
and it exists because a 2026-07 rename traded a name for an implementation gap that is now closing.

## Goals

1. **Name the lifecycle once.** Four operations, the same words in every program, no `status` anywhere.
2. **Settle scope against target.** A scope is an execution context; a target is a destination path. Both
   words keep a job.
3. **Make `--scope` work.** Multi-valued, writ-only, replacing a flag that is registered and read by nothing.
4. **Open the builtin set.** Five builtins, reserved everywhere and defined per platform, with custom scopes
   declared in configuration.
5. **Correct the documents that describe removed code.** Five sites across three architecture documents
   describe types with zero occurrences in the tree.

## Current State

### The lifecycle is named twice

`lore reconcile` returns `fmt.Errorf("reconcile: not yet implemented")`.

| Operation | `lore` | `writ` |
| --- | --- | --- |
| deploy | `deploy` | `deploy` |
| **reconcile** | **`reconcile`** — a stub | **`status`** — works; the only implementation in the suite |
| upgrade | `upgrade` | `upgrade` |
| decommission | `decommission` | `decommission` |

`cmd/writ/writ/commands.go:187` holds the only `Use: "status"` in `cmd/`.

**The rename was deliberate and its reason has lapsed.** `writ reconcile` was replaced by `writ status` at
phase-8 step 47. From `writ-deploy-family.md:136`:

> **`writ status` replaces `writ reconcile`.** "Reconcile" promises mutation the command does not perform;
> … "reconcile" retires entirely — no `--fix`: the repair for each finding is [named] instead.

Reconcile now reports **and** repairs (ruled 2026-08-31), so the promise will be kept and the name returns.

`writ-deploy-family.md:31` records an original shape carrying `--drift`, `--fix` and `--json`. **None of
those three survives.** `--json` is replaced by the shared `--output` / `-o` (#740); the other two are the
deferred selector under earlier names.

### The vocabulary collides in one file, four lines apart

```go
// builder.go:49
// Target is the absolute path to the target file.
Target string

// builder.go:57
// TargetName is the target scope ("System" or "Home").
TargetName string
```

Counted across `cmd/writ`: `TargetRoot` 52, `TargetName` 16, `buildScope` 16, `byScope` 15, `TargetHome` 7,
`filesByScope` 6, `Target` 6, `TargetSpec` 4, `Scope` 4, `sortGraphsByScope` 3, `inferScope` 3,
`TargetSystem` 3, `TargetOrder` 3.

### `--target` is registered and read by nothing

`cmd/writ/writ/root.go:46` declares it. No `GetString("target")` exists anywhere. All four config paths
hardcode `cfg.TargetRoot = TargetHome()`.

### The scope set is closed, and its config key is half-present

`TargetOrder()` returns a hardcoded pair. `writ.targets` appears in `schema/defaults/writ.yaml` for a user to
uncomment and is **absent from `schema/devlore-config.json`** — the same shape as
[#746](https://github.com/NobleFactor/devlore-cli/issues/746).

### Five documented types do not exist

Zero occurrences in the tree of `RootReader`, `RootReaderWriter`, `confinedRoot`, `rootBase`,
`OpenWritableUnsandboxed`. `fsroot` exports `OpenExisting` and `OpenScratch`, and both sandbox.

## Requirements

### Requirement 1: One lifecycle vocabulary

`deploy`, `reconcile`, `upgrade`, `decommission`, in both programs. No `status` command anywhere. The
retirement is by deletion — no alias, no hidden command (§13).

### Requirement 2: Reconcile produces a report, and takes no command flags

`writ reconcile` **produces a report** answering "what changed since deploy and some number of upgrades",
like `git diff`. The repair half is chartered, not built here.

**It has no command flags.** It takes the globals — `--output` / `-o`, `--filter`, `--jq`, `--store`,
`--dry-run` — and must utilize all of them faithfully. A command that registers a flag its root already
provides is the defect #740 exists to stop.

The selector between fix, diff and summary behaviors is **deferred to the reconciliation epic**. `5.1`'s
"there is no `--fix`" is no longer the whole truth — reconcile will repair — but neither is a `--fix`
flag settled: today there is none, and the surface for choosing is undesigned.

**A caution for whoever designs it.** Two of the three candidate views are already projections of the one
document the report produces: the diff is `--jq '.entries'`, and the history is `--jq '.packages'`. Those
are stage 2 of the pipeline #740 landed, and `--jq` and `--filter` are already in the reserved set.
Adding `--diff` and `--history` flags would create a second way to select a subset of the same document,
and the two will drift. Only the repair half is genuinely new, because it is a mutation rather than a
projection — and the mutation axis already carries `--dry-run`, so anything added there has to say how
the two compose.

### Requirement 3: Reconcile is valid only after deployment

Before deployment, a not-found error rather than an empty report — so an exit code distinguishes
never-deployed from deployed-and-clean from deployed-and-drifted. This corrects
[#756](https://github.com/NobleFactor/devlore-cli/issues/756), which was filed against the opposite reading.

### Requirement 4: Scope names an execution context; target names a path

A scope carries a name, a target root, a confinement boundary, an elevation posture, and its own graph. A
scope HAS a target root, and that root is generally not known until runtime.

### Requirement 5: `--scope` is multi-valued and writ-only

`[--scope=<name>]...`, repeatable. Absent, every defined scope runs in order. `lore`, `star`, and
`devlore-test` have no scope option.

### Requirement 6: Builtins are reserved everywhere, defined per platform

| Scope | Unix | Windows | Elevated |
| --- | --- | --- | --- |
| `Home` | `os.UserHomeDir()` | `os.UserHomeDir()` | no |
| `System` | `/` | `%SystemDrive%\` | yes |
| `ProgramData` | undefined | `%ProgramData%` | yes |
| `ProgramFiles` | undefined | `%ProgramFiles%` | yes |
| `ProgramFilesX86` | undefined | `%ProgramFiles(x86)%` | yes |

Capitalization is the platform's. **`LOCALAPPDATA` and `APPDATA` are deliberately excluded**, and they differ:
Local has no supported relocation — *"there is no way to treat Local AppData in the same way"* — so it is
pinned under `%USERPROFILE%` and reaches through `Home` as `Home/AppData/Local/...`. Roaming IS officially
redirectable under domain policy, so a builtin would hardcode a location an AD policy may override; it
belongs to a custom scope when an operator has moved it.

### Requirement 7: Data is skipped, instructions are refused

| Situation | Behaviour |
| --- | --- |
| A layer contains `ProgramFiles/` on Unix | skipped, silently — the repo is shared across machines |
| `--scope=ProgramFiles` on Unix | error: not defined on this platform |
| Config declares a custom scope named `ProgramFiles`, on ANY platform | error: builtin name |

Reserving on every platform — not only where defined — is what keeps one repository meaning one thing on
every machine.

### Requirement 8: Segments and scopes are name-to-value maps — tracked by #765

**Neither key is read today.** `writ.segments` has been documented since the schema was written and has never
worked (#746); `writ.scopes` is new here. `--scope` has nothing to select until scopes can be introduced, so
this is the prerequisite for Phase 4 rather than a parallel task.

**RULED: one key per concept, holding both the name and the value.** A key introduces the name; its value
sets it. `vars` reverts to what templates interpolate and holds nothing else.

```yaml
writ:
  segments:
    ROLE: desktop            # introduced and set
    SITE:                    # introduced; set by WRIT_SEGMENT_SITE or --segment
  scopes:
    Staging: ~/staging/root  # a custom scope
    Home: ~/staging/home     # a BUILTIN's root, overridden
  vars:
    USER_NAME: "Your Name"   # template variables only
```

Three concerns were sharing `vars` -- template variables, segment values, and scope roots -- and a reader had
to know which was which by the key's name. Now each concept states its own names and its own values in one
place, and `vars` means one thing.

**A key with no value declares without setting.** That is the shape `DetectSegmentsWithNames` already
describes ("values are empty until set via CLI `--segment` flags"), and it stays reachable: a segment
declared here takes its value from `WRIT_SEGMENT_<NAME>` or `--segment`. An unset segment matches no
directory suffix, behaving as `DISTRO` does on macOS.

**A builtin scope's key overrides its default root**, which is how a deployment is aimed at a staging tree.
For `Home` it is the only way, since the home directory is resolved from the account database and `HOME`
cannot move it.

The one asymmetry is in the concepts, not the shape: segment builtins (`OS`, `DISTRO`, `ARCH`) resolve on
every platform, where scope builtins resolve per platform. So scopes need a rule segments do not -- override
the root of a builtin that resolves here, never introduce one that does not.

**#746 applies to both.** `writ.segments` is read by nothing today, so this shape is a documented design
rather than working code, and it changes from an array to a map in this plan. Scopes must not repeat that
defect: the key is read in the same change that documents it.

### Requirement 9: Elevation is per scope

A scope declares that it requires elevation; `elevation.Provider` satisfies it, as needed — sudo on Unix,
Administrator on Windows. The provider is a draft (`6.1-privilege-elevation.md`); `ScopeSpec` gains the
field now so the requirement is expressible.

## Implementation Phases

### Phase 1: Documents — tracked by #766

Documents first: the code changes cite them, and three of them currently describe removed types.

- [x] `10-command-line-interface.md` §3.1 — the lifecycle named once
- [x] `10-command-line-interface.md` §4.1 — `--scope`, the builtins, reserved names, elevation
- [x] `10-command-line-interface.md` §11.1 — segments and scopes as name-to-value maps
- [x] `2.4-hermeticity-guarantees.md` — the scope model; three "unconfined" claims; the "Six Cells" and
      "2×3 matrix" arithmetic, which assumed exactly two scopes
- [x] `5.1-reconciliation.md` — the reversion, reconcile repairs, `writ status` renamed throughout
- [x] `5.3-recovery-site.md` — the `RootReaderWriter` test mode that no longer exists
- [x] `schema/devlore-config.json` and `schema/defaults/writ.yaml` — `writ.scopes` added, `writ.segments`
      reshaped to a map, `vars` reduced to template variables
- [x] `4.4-root-path-triad.md` — §4 "Mode Switch" deleted and the rest renumbered (#767)
- [x] `10-command-line-interface.md` §15 — the conformance table says `writ reconcile` (#767)
- [x] `10-command-line-interface.md` §1 retitled "What this governs", so "scope" has one meaning (#767)

*The three boxes above were completed by #767 and stayed unticked until 2026-09-01 — recorded rather than
quietly corrected, since stale status is the drift the revised process exists to prevent.*

### Phase 2: The rename — landed under #774 (status: complete)

This phase and thread 1's `writ` item are one piece of work, done in
[774-writ-reconcile.md](774-writ-reconcile.md) phase 1.

- [x] `writ status` → `writ reconcile`; `git mv cmd/writ/writ/status cmd/writ/writ/reconcile`, `status.go` →
      `reconcile.go`, so history follows. No alias.
- [x] `status.Report` → `reconcile.Report`, and the rest of the package surface — `ReconcileConfig`,
      `parseReconcileConfig`, `newReconcileCmd`, `runReconcile`.
- [x] Scenario tests, and every document that named `writ status` as live: seven architecture and guide
      documents and nine plans changed outright; the two that *record* the step-47 rename carry a note.

### Phase 3: The vocabulary sweep

- [ ] `TargetName` → `ScopeName` (16), `TargetSpec` → `ScopeSpec` (4), `TargetOrder` → `ScopeOrder` (3),
      `TargetHome` / `TargetSystem` → `ScopeHome` / `ScopeSystem` (10).
- [ ] `TargetRoot` (52) and `Target` / `Entry.Target` (6) are UNCHANGED — they name destination paths.

### Phase 4: `--scope`

- [ ] `--scope` replaces `--target`, multi-valued and repeatable, and is read.
- [ ] `ScopeOrder()` opens: the five builtins in table order, custom scopes in discovery order after them.
- [ ] `ScopeSpec` gains an elevation field.
- [ ] Reserved-name enforcement, per Requirement 7.
- [ ] A later layer replaces an earlier one where they overlap: base, team, personal.

## Migration Path

Greenfield: `writ status` stops existing, `--target` stops existing, and `writ.targets` stops being read. No
aliases (§13). The config key is the only user-visible loss — worth stating in the release note, because
nothing validates unknown keys today, so a stale `writ.targets` would be ignored in silence.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/architecture/2.4-hermeticity-guarantees.md` | Modify | scope vocabulary; three stale claims |
| `docs/architecture/4.4-root-path-triad.md` | Modify | §4 describes removed types |
| `docs/architecture/5.3-recovery-site.md` | Modify | removed test mode |
| `docs/architecture/5.1-reconciliation.md` | Modify | reconcile repairs; the rename |
| `docs/architecture/10-command-line-interface.md` | Modify | operations, `--scope`, reserved names |
| `schema/defaults/writ.yaml` | Modify | `writ.scopes` |
| `schema/devlore-config.json` | Modify | `writ.scopes` — currently absent entirely |
| `cmd/writ/writ/status/` | Move | → `reconcile/` |
| `cmd/writ/writ/layer.go` | Modify | `ScopeSpec`, `ScopeOrder`, `ScopeHome`, `ScopeSystem` |
| `cmd/writ/writ/tree/builder.go` | Modify | `ScopeName` |
| `cmd/writ/writ/root.go` | Modify | `--scope` replaces `--target` |

## Related Documents

- [`2.4-hermeticity-guarantees.md`](../../architecture/2.4-hermeticity-guarantees.md) — the scope model
- [`5.1-reconciliation.md`](../../architecture/5.1-reconciliation.md) — the concern and its landed mechanism
- [`4.5-fsroot-variants.md`](../../architecture/4.5-fsroot-variants.md) — why the unsandboxed variants went
- [`6.1-privilege-elevation.md`](../../architecture/6.1-privilege-elevation.md) — elevation, draft
- `writ-deploy-family.md` — the phase-8 design that renamed reconcile to status
- Issues: #762 (this plan), #765 (neither config key is read — the prerequisite for `--scope`),
  #766 (the CLI specification), #761 (System roots disagree), #763 (doubled env prefix),
  #756 (corrected by the not-found ruling), #746 (segments unwired, superseded in scope by #765)

## Open Questions

1. **`4.4` §4 "Mode Switch"** — replace with a statement that there is no mode switch, one implementation
   with a varying anchor; or delete the section and renumber §5 and §6?
2. **Windows `System` root** — #761 rules `%SystemDrive%\`, and `adopt.inferScope` classifies against
   `%SystemRoot%`. Fixed there or here? It is independently fixable and blocks nothing in this plan.
