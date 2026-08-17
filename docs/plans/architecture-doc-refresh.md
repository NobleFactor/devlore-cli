---
title: "Architecture docs: the four undescribed decisions"
issue: TBD — created after review
status: draft
created: 2026-08-17
updated: 2026-08-17
---

# Plan: architecture docs — the four undescribed decisions

## Summary

The 2026-08-16/17 sessions settled four things that no architecture document describes. Each is a rule
other work will be measured against — where files live, who owns filesystem authority, where a
machine-wide install goes, and what a scenario is for — and each currently survives only in a plan
document, a step charter, or a package doc comment. A rule that lives only in the plan that produced
it is a rule the next reader re-litigates.

This plan is **only** the four gaps. Corrections already applied on the `cli-save-artifacts` branch —
the trace-stamp owner, the persistence seam, the signing open question, and a mechanical
`internal/…` → `cmd/internal/…` path sweep across fifteen documents — are not repeated here.

## Goals

1. **Every rule has one home**, and the plan documents point at it rather than holding it.
2. **The reasoning travels with the rule.** These four were each chosen against a rejected
   alternative; a doc that states only the outcome invites the alternative back.
3. **No new document unless the subject has no owner.** Three of the four have an obvious home.

## Current State

| Decision | Lives today in | Architecture home |
| --- | --- | --- |
| The home-resolution ladder | `pkg/xdg`'s package doc, step 54 | ❌ none |
| Root ownership (caller owns, leaf receives) | windows-native-permissions phase 2b/3.1 | ❌ none — 2.4 is the near miss |
| Machine-wide install location + `--all-users` | windows-native-permissions 3.1b | ❌ none — `configuration.md` is the home |
| The scenario tier | this plan, `cmd/scenario`'s package doc | ❌ none — `7.2-e2e-testing.md` predates it |

## Requirements

### R1 — The home-resolution ladder and the XDG layout

**Where:** a new `2.9-filesystem-locations.md`. There is no document about where anything lives on
disk, which is why this rule had nowhere to go; `configuration.md` is about the config *model*, not
about locations, and the execution store's layout currently sits inside
`5-graph-trace-integrity.md` because that document needed it.

Must state:

- The four-rung ladder — absolute `XDG_<ROLE>_HOME`, then `user.Current`, then `os.UserHomeDir`, then
  an assert — and that **rung 2 outranks rung 3 deliberately**, following OpenSSH, which expands `~`
  from the account database and never consults `HOME`.
- Why rung 3 survives anyway: releases build `CGO_ENABLED=0`, so `user.Current` parses `/etc/passwd`
  and nothing else, and an LDAP/SSSD/systemd-homed user resolves through the environment or not at
  all. This is the one place our resolution is *weaker* than ssh's, and it should be written down.
- The consequence, which is the part people trip on: **home is resolved, never injected.** Setting
  `HOME` moves nothing for any user the account database can see. Redirecting a home-rooted location
  means setting the role's `XDG_*` variable or passing the path in.
- The layout — `~/.config`, `~/.local/share`, `~/.local/state`, `~/.cache`, `~/.local/bin` on **every**
  platform including Windows — and the evidence for diverging from the Windows Known Folders that
  `adrg/xdg`, `platformdirs` and `dirs` use: git, Microsoft's own OpenSSH port, Docker, kubectl, cargo.
- That `ConfigDirs`/`DataDirs` are **not** yet platform-correct ([step 59](extract-starlark-from-op/phase-8/steps/59-xdg-search-paths-on-windows.md)).

### R2 — Root ownership: the caller owns, the leaf receives

**Where:** `2.4-hermeticity-guarantees.md`, which already owns confinement but describes it as a
property of execution rather than as an ownership rule.

Must state:

- **Code receives filesystem access; it does not construct it.** The session owner opens a root; every
  leaf takes one as a parameter. `pkg/signing` receives its config root *and* its identity path;
  `appendIndexEntry` receives the state root; `document`'s successor will too.
- The worked example, because it is what made the rule concrete: `WriteTrace` writes three things into
  one tree — the document, the `latest` symlink, the index line — and under the old shape its callee
  opened a fourth root for itself.
- The counter-rule: a **`// Confinement:`** comment is the sanctioned exception, and it states why the
  root cannot serve — a source path the operator named, a generator that writes its own files, an SSH
  directory that is not ours to confine.
- That a save does **not** create store layout: `op.SaveGraph` writes, `cli.WriteGraph` decides where.

### R3 — Where an installation goes

**Where:** `configuration.md`, under a new section beside "Where sections live".

Must state:

- Per-user default `~/.local`; machine-wide is the **usual directories within `/usr/local`** on Darwin
  and Linux, `%ProgramFiles%\<Vendor>\<Product>` on Windows.
- `/opt/local` was **rejected**: not in FHS — which reserves only `/opt/bin`, `/opt/doc`,
  `/opt/include`, `/opt/info`, `/opt/lib`, `/opt/man` for local use — and its one real-world claimant
  is MacPorts.
- `--all-users` selects it and the positional prefix remains the escape hatch, because a prefix
  argument **cannot express machine-wide on Windows**: `%ProgramFiles%\<Vendor>\<Product>` is not a GNU
  prefix and carries obligations no path can state (the Uninstall registry key, machine `PATH` in
  `HKLM`).
- Not `--system`: writ's layer vocabulary already uses `System` for the `/` target root.
- `writ.targets.{home,system}` override the deployment roots — distinct from the *install* prefix, and
  the only way to redirect a deployment now that home is not injectable.
- The precedent worth recording: Git, Python and Vim install as single prefixes under
  `%ProgramFiles%` and keep system config **inside the prefix**; only software with machine-wide
  mutable state (Docker, Chocolatey) uses `%ProgramData%`. writ has none, so it needs no `%ProgramData%`.

### R4 — The scenario tier

**Where:** `7.2-e2e-testing.md`, which describes the AI-model E2E harness and predates scenarios.

Must state:

- The three tiers and what each can prove: unit tests (in process), **scenarios** (the real binaries in
  a sandbox, `make test-scenario`), and the AI E2E harness (needs API keys).
- The rule scenarios exist for, stated as the incident that proved it: **an in-process test can encode
  the same wrong assumption the code makes.** The self-install unit test asserted `bin/<tool>` with no
  extension, agreeing with a bug that made Windows installs unrunnable; the scenario, driving the
  shipped binary, disagreed on its first run.
- What belongs where: `cmd/writ` for writ's own deploy scenario, `cmd/scenario` for scenarios owned by
  no single command.
- The sandbox contract — every `XDG_*` redirected, and `writ.targets.home` for a deployment, because
  `HOME` no longer redirects anything (R1).

## Implementation Phases

### Phase 1: the three that have homes

- [ ] R2 into `2.4-hermeticity-guarantees.md`
- [ ] R3 into `configuration.md`
- [ ] R4 into `7.2-e2e-testing.md`
- [ ] Each plan/step doc that currently holds the rule gains a pointer to its new home

### Phase 2: the one that needs a home

- [ ] `2.9-filesystem-locations.md` (R1), plus its `.status.md` companion per the house pattern
- [ ] Move the execution-store layout diagram out of `5-graph-trace-integrity.md`, or link it — that
      document should own integrity, not locations

## Open Questions

- [ ] Does `2.9` also absorb the install locations (R3), leaving `configuration.md` to reference it?
      One document about where things live is tidier than two, but install locations are configurable
      and the rest are not.
- [ ] Do the `.status.md` companions get the same treatment, or are they regenerated?

## Related Documents

- [windows-native-permissions.md](windows-native-permissions.md) — holds R2 and R3 today
- [step 54](extract-starlark-from-op/phase-8/steps/54-xdg-anchors-on-windows.md) — holds R1's ruling
- [step 59](extract-starlark-from-op/phase-8/steps/59-xdg-search-paths-on-windows.md) — the search-path
  defect R1 must mention
- [version-stamping.md](version-stamping.md) — a fifth candidate if `pkg/application` grows further
