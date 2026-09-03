---
title: "star: the root becomes the shared one, the dead cli package goes, and ten stdout sites are classified"
issue: https://github.com/NobleFactor/devlore-cli/issues/743
status: complete
created: 2026-09-01
updated: 2026-09-02
---

# Plan: `star` adopts the output convention

## Summary

`star` builds its own root — a bare `cobra.Command` — so it inherits nothing from `cmd/internal/cli`: not
the common set, not a fix made there. It carries a second `cli` package that nothing imports. And it has
ten stdout sites that #776's invariant will refuse the moment that test exists. This moves the root onto
`cli.NewRootCmd`, deletes `cmd/star/cli`, and classifies the ten.

`star` is also the code generator every `make` target runs through, so a change to `star` that breaks
`star` leaves a tree that cannot rebuild the tool that would repair it. The last-known-good process the
Makefile documents governs this work: the snapshot is taken before the first edit and retired after the
tree builds on its own.

Moving the root exposed a fourth thing: three Starlark extension commands bind flags the common set owns.
Extension commands are commands, so they meet the convention too — the loader refuses a reserved name,
and a script's return value is its result.

This is the third item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[775-lore-adoption.md](775-lore-adoption.md) and before
[776-output-enforcement.md](776-output-enforcement.md).

## Goals

1. **`star`'s root is `cli.NewRootCmd`**, so the common set is inherited and a shared fix reaches it.
2. **`cmd/star/cli` does not exist.**
3. **No `star` command writes to `os.Stdout` directly.**
4. **`star` keeps running throughout.** The LKG is taken before the first edit and retired after the tree
   builds from source again, and the installed `star` runs at the end.
5. **Extension commands are commands.** None binds a reserved name, and a script's return value reaches
   stdout through the shared pipeline.

## Current State

What the survey found, against what #743 was filed with:

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/star/cli` | ✅ deleted, phase 2 | it was dead: `main.go` imports `cmd/internal/cli`, nothing imported the copy |
| `renderTable` promoted to `pkg/result` | ✅ **already done** | #740 phase 4 built `TableFormatter` from it; #743's third acceptance box is already earned |
| `star`'s root | ❌ bare `cobra.Command` | `main.go:162`; binds its own `--dry-run` and `--silent`; builds the narrator in `cobra.OnInitialize` |
| `star`'s `version`, `self`, `config`, `docs` | ⚠️ present, **collide** | `version` and `self` registered by `main.go` itself (`:221`, `:328`); `config` is two Starlark extensions, `ConfigShow` and `ConfigSync`; `docs man` and `docs markdown` duplicate the shared `man` and `make docs` |
| usage text after an error | ⚠️ inconsistent | `star` and `devlore-test` set `SilenceUsage: true` on their own roots; the shared root does not, so `lore` and `writ` print the usage block after every error |
| the common set on `star` | ❌ absent | `star search -o json` is an unknown flag |
| extension flags on reserved names | ⚠️ three | `devlore actions generate --output <dir>`, `devlore actions validate --format`, `devlore knowledge extract --format`; cobra lets a leaf shadow the inherited flag, so `generate -o json` meant a directory |
| a script's return value | ❌ discarded | `Command.Run` drops it; a script that has a result prints it itself |
| extension command order | ⚠️ by chance | commands register in map order, so `setup` appeared twice on one build and once on the next |
| stdout writes, non-test, outside the dead package | ❌ ten | eight in `main.go`, one each in `star/command.go` and `star/application.go` |
| `build/star.lkg` | ✅ taken 2026-09-02 | built from `1aaba0fd` (develop plus this plan); loads 28 of 28 modules; installed with `star self install` |

## Rulings

All are the same rule — **the shared route on all four programs, and a program's own additions attached
beside it** — applied to three commands and then to the extension commands.

- **2026-09-02, `config`:** one set of `config` subcommands on all four programs — the shared root's
  (`edit`, `get`, `list`, `path`, `schema`, `set`, `unset`, `validate`). `star`'s extension commands
  `show` and `sync` attach beneath it. Nothing in the shared set carries either name.
- **2026-09-02, `man`:** the shared `man [command]` is the one route to man pages on all four programs.
  `star docs man` duplicates it and goes; `star docs markdown` duplicates `make docs` (`devlore-docs`, for
  the whole suite) and goes. `star docs starlark` is star's addition and stays, the one leaf under
  `star docs`.
- **2026-09-02, usage text:** **never printed on an error.** Not after a command that ran and failed, and not
  for a bad flag, a wrong argument count or an unknown subcommand either — cobra's usage dump is not
  context-aware and gives no specific guidance, so it is noise in every case. `cli.NewRootCmd` sets
  `SilenceUsage: true` on the shared root, which cobra honors for every command beneath it. `star`'s own
  setting goes with its hand-built root; `devlore-test` keeps its own until it moves onto the shared root.
- **2026-09-02, extension flags:** the three reserved-name flags are resolved by the design's own rules,
  and the loader refuses a reserved name so a fourth cannot appear. **`generate` was pointing at a source
  tree and calling it a destination:** `--source` and `--output` named the same package in every caller,
  because the generated files land inside the provider's package — five to seven under `gen/`, and
  `action_names.gen.go` at its root. So the operand is the **source package**, and there is no destination
  to name. `--source`, `--output` and `--write` go with it, and so does `--gen`, a boolean the script
  required to be true. The preview — narrating what would be written instead of writing it — is what the
  global `--dry-run` already means on every command, so `generate` honors it and no `--preview` is added.
  **Both `--format` flags go:** validate's was a rendering, and the global `--output` replaces it; extract's
  only narrowed which of its two artifact files to write, and it now writes both, which its default already
  did. Markdown was validate's default rendering and had no caller; it goes with the flag.

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
| `SilenceUsage: true` (ruling 2026-08-04) | **ruled above:** the shared root sets it, for all four programs; `star`'s own setting goes with its root |
| `version` — `main.go:221` registers `cli.NewVersionCmd` itself | the shared root registers the same command; `star`'s registration goes |
| `self` — `main.go:328` registers `cli.NewSelfCmd` with `PostInstallHooks` and `PostUninstallHooks` that install and remove the Starlark extensions | the shared root builds its `self` from `RootConfig` alone, with no seat for hooks; **`RootConfig` gains `PostInstallHooks` and `PostUninstallHooks`**, passed through to the shared `SelfInstallInfo`; `star`'s registration goes. A `cmd/internal/cli` change |
| `config` — the `ConfigShow` and `ConfigSync` extensions add `show` and `sync` under a `config` group that `loadStarlarkCommands` creates | **ruled above:** the shared `config` is the group; extension loading attaches to an existing command of the same name instead of adding a second |
| `docs man <dir>` / `docs markdown <dir>` / `docs starlark` | **ruled above:** `docs man` and `docs markdown` go; `docs starlark` stays |
| no default config of its own | `schema/defaults/star.yaml` and `schema.StarDefaultConfig`, so `self install` seeds `config.d/star.yaml` the way it seeds the other three (#780's rule) |

**Informational, no action needed.** Three things the shared root brings that `star` did not have:

- `application.NewApplication("star", rootCmd)` reads the root's persistent flags into the Starlark-visible
  `Flags` map. Under the shared root that map grows by `config`, `verbose`, `interactive`, `unattended`,
  the four `model-*` flags and the output set. Scripts see them; nothing acts on them.
- `initRootConfig` wires Viper: `~/.config/devlore/config.yaml`, a `STAR_` environment prefix,
  `BindFlags`. `star` keeps its extension config beside it, so `star config get` reads one source and
  `star config show` another. Two config paths in one program is thread 4's subject
  ([441-unified-configuration.md](441-unified-configuration.md)); this plan adds the second and does not
  reconcile them.
- The shared root hides `man` and `completion` from help, the same as on lore and writ. Both still run.

### Requirement 2: `cmd/star/cli` is deleted

`git rm -r cmd/star/cli`. It is dead — no importer — so there is nothing to migrate. The comments in
`main.go` that mention it are corrected or go with the code they annotate.

### Requirement 3: The ten stdout sites are classified

| Site | Classification |
| --- | --- |
| `main.go:234-235, 243, 251-252` — the `key` stubs, "not yet implemented" | **corrected in phase 4:** not narration — a stub that prints a note and exits 0 reports a success it did not deliver, which the repository's checklist forbids. Each leaf now fails with an error naming ADR-040 |
| `main.go:288, 307` — "Man pages written to", "Markdown docs written to" | narration; the commands go with the `man` ruling, and the sites with them |
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

### Requirement 5: `star` continues to run — the LKG process

The Makefile resolves `$(STAR)` to `build/star.lkg` when that file exists and to `build/star` otherwise,
and while the snapshot exists the `build/star` rule is never consulted. So every codegen step in this
branch runs on the snapshot, and an edit that breaks `star`'s compilation or its startup cannot strand the
tree. The process, as the Makefile and `role-vocabulary.md` state it:

1. `make star-lkg` **before any edit** — done 2026-09-02, from `1aaba0fd`.
2. Do the work; codegen runs on the snapshot throughout.
3. `make star` builds from source; the tree compiles on its own.
4. Remove `build/star.lkg`.
5. `make regenerate` produces no diff — otherwise the snapshot was load-bearing, not an escape hatch.
6. `make install` runs `star self install`; the installed `star` runs a command end to end from a cwd
   where its extensions load.

**Steps 3 to 6 belong to phase 3's commit, not to a phase of their own.** Requirement 6 changes how the
Makefile invokes `generate`, and the snapshot only understands the old form; the first `make regenerate`
after that change must already run on the star built from source. So phase 3's script removes the
snapshot before it builds.

The snapshot is a build artifact under the ignored `build/`; it is never committed. The fresh build was
installed the same day with `star self install`: binary, man pages, completions for bash, PowerShell and
zsh, and the five `com.noblefactor.devlore.*` extensions. Taking that install exposed
[#780](https://github.com/NobleFactor/devlore-cli/issues/780) — every app's manifest claims every man
page in the shared directory — which is its own worktree.

### Requirement 6: Extension commands are commands

A Starlark extension command is a command of `star`, and the convention applies to it whole: it inherits
the common set, it binds none of the reserved names, and its result reaches stdout through the shared
pipeline. Five pieces:

1. **The loader refuses a reserved name.** `defineFlags` rejects `filter`, `format`, `jq`, `json`,
   `output` and `store`, naming the extension, the command and the flag, so the failure is at load and not
   at the first `-o json` that silently means something else.
2. **A script's return value is the command's result.** `Command.Run` returns it, converted to the
   JSON-shaped Go value the pipeline renders — dicts with string keys, lists, scalars, None as nothing —
   and refuses anything else by name. The cobra leaf emits it through `emitResult`; a script that returns
   None emits nothing. A script never prints its result.
3. **`generate` takes the package as its one operand:** `star devlore actions generate [--dry-run]
   <package>`. The files land inside that package, where they always did. The 30 Makefile recipes change
   from `--source=$(P)/x --gen=true --write=true --output=$(P)/x` to `$(P)/x`; the extension yaml declares
   the operand and drops the four flags; `generate.star` reads `package`, writes unless `ctx.dry_run`, and
   under `--dry-run` narrates the paths it would write rather than every file's content.
4. **`validate` returns its report** and renders nothing; `--format` and the three markdown printers go.
5. **`extract` writes both artifacts always**; `--format` goes. CI never passed it.

And one repair found on the way: extension commands register in name order, so a group's leaf (`setup`)
exists before its children (`setup.check`) attach beneath it. A map walk decided that by chance.

**Not done here, recorded:** a command a script invokes through the `commands` provider is a nested
invocation, and its result does not return to the calling script — the provider's contract carries an
error alone. Whether it should is an open question below. And `validate` fails today before it renders
anything, looking for `internal/starlark`, a path that no longer exists; that predates this branch and is
logged as its own issue.

## Implementation Phases

### Phase 1: Take the LKG (status: complete)

- [x] `make star-lkg` in the worktree before any edit — `build/star.lkg`, 2026-09-02, from `1aaba0fd`
- [x] `star self install` from the fresh build; it loads the tree's extensions where the July build could not

### Phase 2: `cmd/star/cli` goes (status: complete)

- [x] `git rm -r cmd/star/cli`; the two `main.go` comments that named it corrected; the three architecture
      documents that described the copy as live now say it was dead and is gone
- [x] #743's acceptance box "renderTable lives in pkg/result" ticked with a pointer to #740 phase 4

### Phase 3: The root, and the extension commands with it (status: complete)

- [x] `TestRoot_EverySubcommandAcceptsTheCommonSet` — written red first: on the current root, no
      subcommand inherits `--output`; and red a second time on the three extension flags
- [x] `RootConfig` gains `PostInstallHooks` and `PostUninstallHooks`, passed through to `SelfInstallInfo`
- [x] `cli.NewRootCmd` sets `SilenceUsage: true`; `star`'s own setting goes with its root
- [x] `cli.NewRootCmd` replaces the hand-built root; the wrapper resolves the flag collisions as tabled;
      `star`'s own `version` and `self` registrations go; `schema.StarDefaultConfig` seeds its config
- [x] extension loading attaches `show` and `sync` beneath the shared `config` (the ruling), in name order
- [x] `docs man` and `docs markdown` go; `docs starlark` stays (the ruling)
- [x] `emitResult` and `outputOptions` for `star`, mirroring `lore` and `writ`
- [x] the loader refuses reserved flag names; `Command.Run` returns the script's result and the leaf emits it
- [x] `generate` takes the package as its operand, `--source`, `--gen`, `--write` and `--output` gone,
      `--dry-run` previews, and the 30 Makefile recipes follow; `validate` returns its report; `extract`
      writes both artifacts; both `--format` flags gone
- [x] unit tests: `--dry-run` reaches `starruntime.DryRun`; `--silent` wires one narrator; `star config`
      carries the shared set plus `show` and `sync`; `docs` carries `starlark` alone; `self install` still
      installs the extensions; no usage text on any error; reserved names refused; the script's return
      value is the result
- [x] the commit's script retires the LKG: `build/star.lkg` removed, `make build` from source,
      `make regenerate` no diff, `make test`, `make install` -- every gate a step of the script that commits
- [x] the design documents and the 740 plan record star as on the shared root

### Phase 4: The remaining sites (status: complete)

- [x] The two Starlark `print` handlers narrate through `cli.Note`, as lore's does; `--silent` gates them
- [x] The five `key` stub lines: the three leaves fail with an error naming ADR-040 rather than printing a
      note and exiting 0 — a correction to this plan's classification, per the checklist's rule that no
      stub reports success
- [x] `docs starlark` through `emitResult`; the default-rendering question logged on #740 by the commit's
      script
- [x] `grep` clean: no `fmt.Print` or `os.Stdout` in `cmd/star` outside tests

### Phase 5: Ambient, not non-hermetic (status: complete)

Ruled 2026-09-02: the opposite of hermetic is **ambient** — a runtime that takes its context from what
surrounds it — not "non-hermetic", which names an absence. Nine sites: the runtime banner and its comment
in `pkg/op/starlarkbridge/runtime.go`, two in `pkg/op/graph_executor.go`, two in the bridge's tests,
`3.6-method-classification.md`, and two plan documents.

- [x] every `non-hermetic` in code, tests and documents reads `ambient` — eleven lines in six files; the
      pair defined once beside "hermetic" in `2.4-hermeticity-guarantees.md`'s Objective

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `cmd/star/cli` does not exist | build | the directory is restored |
| 2 | every `star` subcommand inherits the common set | unit | the root is hand-built again, or an extension binds a reserved name |
| 3 | no `star` command registers `--output`/`--format`/`--json` | unit | one is re-added |
| 4 | `--dry-run` still reaches `starruntime.DryRun` | unit | the wrapper stops copying it |
| 5 | `--silent` still silences narration | unit | the narrator is built from the wrong flag |
| 6 | `star config` is the shared set plus `show` and `sync`, one command named `config` | unit | extension loading adds a second `config` |
| 7 | `star self install` still installs the extensions | unit | the hooks are dropped in the move |
| 8 | no usage text on any error — a failed command, a bad flag, a bad argument count, an unknown subcommand — on all four roots | unit | the shared root stops setting `SilenceUsage` |
| 9 | `star docs` has `starlark` and nothing else; `star man` is the shared command | unit | a duplicate route comes back |
| 10 | the loader refuses each reserved flag name and accepts any other | unit | `defineFlags` stops checking |
| 11 | a script's return value comes back as JSON-shaped Go; None as nothing; a callable refused | unit | `Run` drops the value again |
| 12 | `generate <package>` reproduces the committed files byte for byte, and `--dry-run` writes nothing | `make regenerate`, unit | the operand is read wrong, or dry-run writes |
| 13 | no `fmt.Print`/`os.Stdout` in `cmd/star` non-test | grep | one is re-added; the tree-wide test is #776's |
| 14 | with the LKG removed, `star` builds from source and `make regenerate` is a no-op | build | the snapshot was load-bearing |
| 15 | the installed `star` runs a command end to end | smoke | the root migration broke startup |

**Write the failing test first.** Row 2 is red on the current root by construction, and it went red a
second time on the extension flags, which is how they were found.

**Not covered:** whether `docs starlark`'s default rendering should be `value`. That is the epic's open
question, and this plan routes the site without answering it.

## Migration Path

`star`'s `--silent` and `--dry-run` keep their names and meanings; they move from `star`'s registration to
the shared root's. `version` and `self` are the same commands from the shared root. `config` keeps `show`
and `sync` and gains the shared set beside them. `star docs man <dir>` becomes `star man`, the shared
route; `star docs markdown <dir>` becomes `make docs`; `star docs starlark` is unchanged. `lore` and `writ`
stop printing the usage block after an error. And `-o`, `--jq`, `--filter` and `--store` now work on
every `star` command.

`star devlore actions generate --source <pkg> --gen --write --output <pkg>` becomes
`star devlore actions generate <pkg>`, and `--dry-run` is its preview; the Makefile is the only caller and
changes with it. `validate --format` goes: `-o json` and `-o yaml` render
the report, and markdown is no longer offered. `extract --format` goes: both artifacts are always written.
CI's knowledge-extract workflow passes neither flag and is unchanged.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/star/cli/` | Delete | dead duplicate |
| `cmd/internal/cli/root.go` | Modify | `RootConfig` post-install and post-uninstall hooks; `SilenceUsage: true` |
| `cmd/internal/cli/root_test.go` | Create | no usage text on any error, on the shared root |
| `cmd/star/main.go` | Modify | the root; the wrapper; `version`, `self`, `docs man`, `docs markdown` registrations go; reserved names refused; results emitted; name-ordered registration; the sites |
| `cmd/star/output.go` | Create | `emitResult`, `outputOptions` |
| `cmd/star/root_test.go` | Create | the root, wrapper, `config`, `docs`, `self`, usage, reserved-name, result and stub tests |
| `cmd/star/star/command.go` | Modify | `Run` returns the script's result; the Starlark print handler |
| `cmd/star/star/result.go` | Create | a Starlark value to the JSON-shaped Go value the pipeline renders |
| `cmd/star/star/application.go` | Modify | the nested-invocation seam; the Starlark print handler |
| `cmd/star/star/*_test.go` | Modify | `Run`'s new signature; the result test |
| `schema/schema.go`, `schema/defaults/star.yaml` | Modify, Create | `StarDefaultConfig` |
| `star/extensions/com.noblefactor.devlore.Actions/extension.yaml` | Modify | `generate`'s package operand; `--source`, `--gen`, `--output`, `--write` and validate's `--format` gone |
| `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` | Modify | reads `package`; honors `--dry-run` |
| `star/extensions/com.noblefactor.devlore.Actions/commands/validate.star` | Modify | returns the report; the markdown printers go |
| `star/extensions/com.noblefactor.devlore.Knowledge/extension.yaml` | Modify | `--format` gone |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/extract.star` | Modify | writes both artifacts always |
| `Makefile` | Modify | the 30 `generate` recipes pass the package alone |

## Acceptance criteria

The canonical checklist; the pull request carries a copy and links here. **Every box is checked before
merge**, and a box that cannot be checked from this branch says what checks it.

**Goals** — the four from #743, and the five the survey, the charter and the rulings added

- [x] `cmd/star/cli` no longer exists — phase 2
- [x] `star` registers the common set through `cmd/internal/cli.AddOutputFlags` — by way of
      `cli.NewRootCmd`; `TestRoot_EverySubcommandAcceptsTheCommonSet` walks the tree — phase 3
- [x] `renderTable` lives in `pkg/result` as the suite's one table formatter — done in #740 phase 4 as
      `TableFormatter`; ticked here with that pointer, no work in this branch
- [x] No exported name is defined in two CLI packages — follows from the deletion, phase 2
- [x] No `star` command writes to stdout directly — the ten sites, `grep` clean — phase 4
- [x] `star`'s `version`, `self`, `config` and `man` are the shared commands; `config` carries `show` and
      `sync` beneath, `docs` carries `starlark` alone — the shared route on all four programs (ruled
      2026-09-02) — phase 3
- [x] No usage text on any error, on all four programs — set once on the shared root (ruled 2026-09-02) —
      phase 3; `devlore-test` keeps its own setting until it moves onto the shared root
- [x] Extension commands bind no reserved name, the loader refuses one, and a script's return value is its
      result; `generate`'s operand is the package and `--dry-run` its preview; both `--format` flags are
      gone (ruled 2026-09-02) — phase 3
- [x] `star` continues to run — LKG taken before the first edit (done 2026-09-02, from `1aaba0fd`),
      retired by phase 3's script after the tree built on its own, `make regenerate` no diff,
      `make install` succeeded — every one a gate the commit could not pass without

**Test rows**

- [x] 1 — `cmd/star/cli` does not exist: `make build` green with the directory gone, phase 2
- [x] 2 — every subcommand accepts the common set: `TestRoot_EverySubcommandAcceptsTheCommonSet` (unit)
- [x] 3 — no command registers its own output flag: the same test, on local flags (unit)
- [x] 4 — `--dry-run` reaches `starruntime.DryRun` (unit)
- [x] 5 — `--silent` wires one narrator (unit)
- [x] 6 — `star config` is the shared set plus `show` and `sync` (unit)
- [x] 7 — `star self install` still installs the extensions (unit)
- [x] 8 — no usage text on any error, all four roots (unit)
- [x] 9 — `star docs` has `starlark` alone; `star man` is the shared command (unit)
- [x] 10 — reserved flag names refused, others accepted (unit)
- [x] 11 — a script's return value is the result (unit)
- [x] 12 — `generate <package>` reproduces the committed files; `--dry-run` writes nothing (`make regenerate`, unit)
- [x] 13 — no `fmt.Print`/`os.Stdout` in `cmd/star` non-test (grep) — phase 4
- [x] 14 — LKG removed, `star` builds from source, `make regenerate` no diff (build)
- [x] 15 — the installed `star` runs a command end to end (smoke)
- [ ] 16 — the suite passes on **all five platforms**: darwin-arm64 locally; **checked here when every
      `test (…)` leg is green** — no scenario touches `star`, so the unit legs are the evidence

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1; phase 4b names this work
- [775-lore-adoption.md](775-lore-adoption.md) — the item before; the same root test and `emitResult`
- [776-output-enforcement.md](776-output-enforcement.md) — the tests that certify this
- [441-unified-configuration.md](441-unified-configuration.md) — thread 4; where `star`'s two config paths meet
- [role-vocabulary.md](../role-vocabulary.md) — the LKG order as first written down
- Issue [#743](https://github.com/NobleFactor/devlore-cli/issues/743)
- Issue [#780](https://github.com/NobleFactor/devlore-cli/issues/780) — found taking the LKG; every app's
  manifest claims every man page; its own worktree
- Issue [#781](https://github.com/NobleFactor/devlore-cli/issues/781) — `devlore actions validate` looks for `internal/starlark`, which no longer exists, and
  fails before it renders anything

## Open Questions

- [ ] `star docs starlark` — the json-always default renders prose as a quoted string. Exception, or
      `-o value` is the answer and the docs say so? Logged on #740.
- [ ] A command a script invokes through the `commands` provider gets no result back — the contract
      carries an error alone. Should a nested invocation's result return to the calling script, as a
      Starlark value?
