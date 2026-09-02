---
title: "star: the root becomes the shared one, the dead cli package goes, and ten stdout sites are classified"
issue: https://github.com/NobleFactor/devlore-cli/issues/743
status: in-progress
created: 2026-09-01
updated: 2026-09-01
---

# Plan: `star` adopts the output convention

## Summary

`star` builds its own root — a bare `cobra.Command` — so it inherits nothing from `cmd/internal/cli`: not
the common set, not a fix made there. It carries a second `cli` package that nothing imports. And it has
ten stdout sites that #776's invariant will refuse the moment that test exists. This moves the root onto
`cli.NewRootCmd`, deletes `cmd/star/cli`, and classifies the ten.

This is the third item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[775-lore-adoption.md](775-lore-adoption.md) and before
[776-output-enforcement.md](776-output-enforcement.md).

## Goals

1. **`star`'s root is `cli.NewRootCmd`**, so the common set is inherited and a shared fix reaches it.
2. **`cmd/star/cli` does not exist.**
3. **No `star` command writes to `os.Stdout` directly.**

## Current State

What the survey found, against what #743 was filed with:

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/star/cli` | ❌ present, **dead** | `main.go` imports `cmd/internal/cli`; nothing imports the copy |
| `renderTable` promoted to `pkg/result` | ✅ **already done** | #740 phase 4 built `TableFormatter` from it; #743's third acceptance box is already earned |
| `star`'s root | ❌ bare `cobra.Command` | `main.go:162`; binds its own `--dry-run` and `--silent`; builds the narrator in `cobra.OnInitialize` |
| the common set on `star` | ❌ absent | `star search -o json` is an unknown flag |
| stdout writes, non-test, outside the dead package | ❌ ten | eight in `main.go`, one each in `star/command.go` and `star/application.go` |

## Requirements

### Requirement 1: The root is the shared one

`cli.NewRootCmd(cli.RootConfig{...})` replaces the hand-built root. The shared root registers `--config`,
`--dry-run`, `--verbose`, `--silent` (through `AddSilentFlag`), `--interactive`, `--unattended` and the four
`--model-*` flags; its `PersistentPreRunE` builds the narrator through `SetUI` and then runs
`initRootConfig`; and it adds `version`, `man`, `config` and `self`. `RootConfig` offers no pre-run hook, so
everything `star` does at dispatch time is done by **wrapping** the shared `PersistentPreRunE`: call it,
then `star`'s own steps. Each collision, and its resolution:

| `star` today | Under the shared root |
| --- | --- |
| `--dry-run` is `BoolVar(&starruntime.DryRun)` | the shared plain `Bool`; the wrapper copies it into `starruntime.DryRun` |
| `--silent` is a local `BoolVar`; `cobra.OnInitialize` builds the narrator and installs it on `cli.SetUI` and `runtime.Environment().Status` | `AddSilentFlag` and the shared pre-run own the flag and `SetUI`; the wrapper sets `runtime.Environment().Status = cli.UI()`; the `OnInitialize` block goes |
| `PersistentPreRunE` is `runtime.Refresh(cmd)` | the wrapper's second step |
| `SilenceUsage: true` (ruling 2026-08-04) | not on the shared root; set on the returned command — whether it belongs in `RootConfig` is an open question below |
| `docs man` / `docs markdown` / `docs starlark` | kept; the shared `man` is a second route to man pages, noted, not removed here |

**Informational, no action needed.** Two things the shared root brings that `star` did not have:

- `application.NewApplication("star", rootCmd)` reads the root's persistent flags into the Starlark-visible
  `Flags` map. Under the shared root that map grows by `config`, `verbose`, `interactive`, `unattended`,
  the four `model-*` flags and the output set. Scripts see them; nothing acts on them.
- `initRootConfig` wires Viper: `~/.config/devlore/config.yaml`, a `STAR_` environment prefix,
  `BindFlags`. `star` keeps its extension config beside it. Two config paths in one program is thread 4's
  subject ([441-unified-configuration.md](441-unified-configuration.md)); this plan adds the second and
  does not reconcile them.

### Requirement 2: `cmd/star/cli` is deleted

`git rm -r cmd/star/cli`. It is dead — no importer — so there is nothing to migrate. The comments in
`main.go` that mention it are corrected or go with the code they annotate.

### Requirement 3: The ten stdout sites are classified

| Site | Classification |
| --- | --- |
| `main.go:234-235, 243, 251-252` — the `key` stubs, "not yet implemented" | narration → `cli.Note` |
| `main.go:288, 307` — "Man pages written to", "Markdown docs written to" | narration → `cli.Success` |
| `star/command.go:56`, `star/application.go:400` — Starlark `print` | narration → `cli.Note`, as `lore`'s |
| `main.go:317` — `fmt.Print(starlarkDocs)`, the `docs starlark` text | **a result, and the exception case** — see below |

**`star docs starlark` is the epic's open question made concrete.** Its result is a document for a human
to read. Through the pipeline under the json-always default, `-o json` renders it as one quoted string,
which is correct and useless. The choices: emit it and let `-o value` be the way to read it; or rule it
the exception — a command whose result is prose defaults to `value`. Recorded, not decided here; the site
is routed through `emitResult` either way so the invariant holds, and the default is the ruling's.

### Requirement 4: The test that pins the root

`TestRoot_EverySubcommandAcceptsTheCommonSet`, as `lore` has it: every subcommand inherits `--output`,
`--filter`, `--jq`, `--store`, and none registers its own.

## Implementation Phases

### Phase 1: `cmd/star/cli` goes (status: not started)

- [ ] `git rm -r cmd/star/cli`; the `main.go` comments that name it corrected
- [ ] #743's acceptance box "renderTable lives in pkg/result" ticked with a pointer to #740 phase 4

### Phase 2: The root (status: not started)

- [ ] `TestRoot_EverySubcommandAcceptsTheCommonSet` — written red first: on the current root, no
      subcommand inherits `--output`
- [ ] `cli.NewRootCmd` replaces the hand-built root; the wrapper resolves the collisions as tabled
- [ ] `emitResult` and `outputOptions` for `star`, mirroring `lore` and `writ`
- [ ] unit tests: `--dry-run` reaches `starruntime.DryRun`; `--silent` silences narration

### Phase 3: The ten sites (status: not started)

- [ ] Nine narration sites to `cli.*`
- [ ] `docs starlark` through `emitResult`; the default-rendering question logged on #740

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `cmd/star/cli` does not exist | build | the directory is restored |
| 2 | every `star` subcommand inherits the common set | unit | the root is hand-built again |
| 3 | no `star` command registers `--output`/`--format`/`--json` | unit | one is re-added |
| 4 | `--dry-run` still reaches `starruntime.DryRun` | unit | the wrapper stops copying it |
| 5 | `--silent` still silences narration | unit | the narrator is built from the wrong flag |
| 6 | no `fmt.Print`/`os.Stdout` in `cmd/star` non-test | grep | one is re-added; the tree-wide test is #776's |

**Write the failing test first.** Row 2 is red on the current root by construction.

**Not covered:** whether `docs starlark`'s default rendering should be `value`. That is the epic's open
question, and this plan routes the site without answering it.

## Migration Path

`star`'s `--silent` and `--dry-run` keep their names and meanings; they move from `star`'s registration to
the shared root's. Nothing user-visible changes except that `-o`, `--jq`, `--filter` and `--store` now work
on every `star` command, and `star` gains `version`, `man`, `config` and `self`.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/star/cli/` | Delete | dead duplicate |
| `cmd/star/main.go` | Modify | the root; the wrapper; eight sites |
| `cmd/star/star/command.go` | Modify | the Starlark print handler |
| `cmd/star/star/application.go` | Modify | the Starlark print handler |
| `cmd/star/output.go` | Create | `emitResult`, `outputOptions` |
| `cmd/star/root_test.go` | Create | the root test; the wrapper tests |

## Acceptance criteria

The canonical checklist; the pull request carries a copy and links here. **Every box is checked before
merge**, and a box that cannot be checked from this branch says what checks it.

**Goals** — the four from #743, and the one the survey added

- [ ] `cmd/star/cli` no longer exists
- [ ] `star` registers the common set through `cmd/internal/cli.AddOutputFlags` — by way of
      `cli.NewRootCmd`; `TestRoot_EverySubcommandAcceptsTheCommonSet` walks the tree
- [x] `renderTable` lives in `pkg/result` as the suite's one table formatter — done in #740 phase 4 as
      `TableFormatter`; ticked here with that pointer, no work in this branch
- [ ] No exported name is defined in two CLI packages — follows from the deletion
- [ ] No `star` command writes to stdout directly — the ten sites, `grep` clean

**Test rows**

- [ ] 1 — `cmd/star/cli` does not exist: `make build`
- [ ] 2 — every subcommand accepts the common set: `TestRoot_EverySubcommandAcceptsTheCommonSet` (unit)
- [ ] 3 — no command registers its own output flag: the same test, on local flags (unit)
- [ ] 4 — `--dry-run` reaches `starruntime.DryRun` (unit)
- [ ] 5 — `--silent` silences narration (unit)
- [ ] 6 — no `fmt.Print`/`os.Stdout` in `cmd/star` non-test (grep)
- [ ] 7 — the suite passes on **all five platforms**: darwin-arm64 locally; **checked here when every
      `test (…)` leg is green** — no scenario touches `star`, so the unit legs are the evidence

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1; phase 4b names this work
- [775-lore-adoption.md](775-lore-adoption.md) — the item before; the same root test and `emitResult`
- [776-output-enforcement.md](776-output-enforcement.md) — the tests that certify this
- [441-unified-configuration.md](441-unified-configuration.md) — thread 4; where `star`'s two config paths meet
- Issue [#743](https://github.com/NobleFactor/devlore-cli/issues/743)

## Open Questions

- [ ] `SilenceUsage: true` — `star` ruled it on 2026-08-04 because a failing verdict is not a usage error.
      Does the suite adopt it in `RootConfig`, or does `star` set it alone?
- [ ] `star docs starlark` — the json-always default renders prose as a quoted string. Exception, or
      `-o value` is the answer and the docs say so? Logged on #740.
