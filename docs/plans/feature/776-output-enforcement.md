---
title: "Enforcement: the tests that fail when a command leaves the output convention, the shared commands they catch, and the last program onto the shared root"
issue: https://github.com/NobleFactor/devlore-cli/issues/776
status: chartered
created: 2026-09-01
updated: 2026-09-03
---

# Plan: Enforce the output convention

## Summary

Adoption of the output convention went backwards, not merely slowly. `extract-output-package.md` recorded
two call sites in March — `lore inspect` and `writ snapshot`. `writ snapshot` was removed, taking half the
convention's adopters with it, and nothing recorded that it had. Invariants that are not tested decay
silently. These tests are what "done" means for the epic.

This is the last item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[774-writ-reconcile.md](774-writ-reconcile.md), [775-lore-adoption.md](775-lore-adoption.md), and
[743-star-adoption.md](743-star-adoption.md). All three landed before this plan was chartered — #774 on
2026-08-31, #775 on 2026-09-01, #743 on 2026-09-03 — so the sequencing story changed: two of the three
tests arrive green, and the first arrives **red on the shared package itself**. `cmd/internal/cli`'s own
`config` and `man` commands, which every program now carries, print their results and their narration
with `fmt.Print`. The convention's enforcer found the convention's home first.

## Goals

1. **Three invariants have a test each**, and each has been shown to fail — on the tree where it is red,
   and on a fixture where it is not.
2. **The shared commands keep the convention** they now put on every program: `config get`, `config list`,
   `config path`, `config schema` and `config validate` emit results through the pipeline, and `man`
   narrates.
3. **The generated CLI reference documents one flag set** across every command of every program.
4. **The epic's convention box closes** on these tests being green, not on a belief that the work is done.
   #740 itself stays open for its remaining members (#757, #782, #759, #742).

## Current State

Measured 2026-09-03 on develop at `4c8c62e8`, the four programs on the shared root:

| Invariant | Test | Status |
| --- | --- | --- |
| No command package writes to `os.Stdout` directly | `TestNoDirectStdout_InScope` | **red at phase 1** — 13 writes in `cmd/internal/cli`: 9 in `config.go`, 3 in `man.go`, 1 prompt in `selfinstall.go`; **green at phase 2** |
| No command shadows an inherited flag, none binds a reserved name | `CheckNoOwnOutputFlag`, from each program's root test | green after phase 2 — four shadows found and resolved on the way |
| Every root is the shared root, and nothing builds a pipeline of its own | `CheckSharedSetOnRoot`, `TestNoPrivatePipeline_InScope` | green — `NewRootCmd` registers the set once; zero `pkg/result` importers outside `cmd/internal/cli` |

Two more `os.Stdout` mentions are not writes and the test must not count them: `root.go` asks the
terminal's width through `os.Stdout.Fd()` to wrap help, and `writ migrate` asks whether stdout is a
terminal. Two are terminal handoffs, not writes of a result: `config edit` gives the editor the terminal,
and `man` gives the pager the terminal. The convention has no clause for those yet; this plan proposes one.

`cmd/devlore-docs`, `cmd/devlore-index` and `cmd/devlore-inventory` print too, and are outside the walk:
§2 of the design lists them as not yet in scope. When one joins, it joins the walk.

## Rulings

- [x] **2026-09-03, the interaction model — the plan of record for every interactive command, onboarding
      and migration first.** Two kinds of child, two rules. A child run for its output — `git`,
      `shellcheck`, anything a provider executes — is captured, always, and never sees the terminal; a
      graph never needs a TTY. A child launched for the user to drive — `$EDITOR` under `config edit`,
      `man` under `man`, and no other — gets the three descriptors through **one seam**,
      `cli.RunInteractive`, which refuses without a terminal and names the alternative (`config path`; the
      page as a result on stdout). Invariant 1's walk allows `os.Stdout` in that one function and nowhere
      else. And the interactive commands themselves work the way an agent's terminal does: interactive
      only when a TTY is present, through `internal/console`; each decision is **one question with a
      short menu and a default**, asked once, never asking what a flag already settled; every answer is
      recorded into the artifact the command produces — the manifest, the migration plan — so the
      interactive run and `--unattended` or `--from <answers>` produce the same artifact and the artifact
      is the record; progress narrates on stderr and the artifact is the result on stdout; stop anywhere
      and resume from the recorded answers; no editor or pager is ever opened by the flow — it shows the
      diff or the draft and asks. Recorded in §10 of the design by this plan's Phase 1.

- [x] **2026-09-03, the shared root owns the common set.** Four roots each remembered to call
      `cli.AddOutputFlags`, each with a variable of its own and an `emitResult` of its own around it; the
      shared `config` command, built inside `NewRootCmd`, saw none of them. Ruled: `NewRootCmd` registers
      the common set itself and owns the one `SinkOptions`, the way it owns `--dry-run`; `cli.Emit(cmd,
      value)` is the one render path, for the shared commands and the programs alike; the programs drop
      their call, their variable and their `emitResult`. `AddOutputFlags` becomes `addOutputFlags`, called
      once from `NewRootCmd`, so no program can forget it or call it twice. **#757 folds into this
      worktree** so that is true of all four: `devlore-test` moves onto the shared root here.

## Requirements

### Requirement 1: No direct write to `os.Stdout` from an in-scope command package

A test over the source of `cmd/internal/cli` and the four programs' packages — `cmd/devlore-test`,
`cmd/lore`, `cmd/star`, `cmd/writ`, test files excluded — that fails on `fmt.Print*`, `fmt.Fprint*` with
`os.Stdout` as its writer, `os.Stdout.Write*`, and `println`. It parses Go, so a comment or a doc string
naming `os.Stdout` does not count, and `os.Stdout.Fd()` or `os.Stdout.Stat()` does not either. The one
shape it lets through is the interactive handoff above, pending the ruling. The allowlist is otherwise
empty: a legitimate exception is a bug in the convention, not in the test.

### Requirement 2: No command shadows an inherited flag, and none binds a reserved name

**Ruled 2026-09-03, on the way to writing the checker.** Cobra treats a subcommand redefining an
ancestor's persistent flag by long name as an override and says nothing; redefining its shorthand
panics the first time that command runs. Neither is protection: one is a bug nobody hits knowingly, the
other is a bug somebody has not hit yet. The protection is a test that walks every command of every
program before anything ships and refuses both shapes for every flag. So the checker fails when a
subcommand defines any long name or shorthand that an ancestor's persistent flags already carry, not
only the reserved six; it fails when a command binds one of `output`, `o`, `format`, `json`, `jq`,
`filter` or `store` on itself regardless; and it fails when a subcommand does not inherit the four the
root registers. Exported from `cmd/internal/cli` so each program calls it from its own root test —
star's root is `package main` and cannot be imported, so the checker goes to the roots rather than the
roots to a central test. Lore's and star's hand-written walks become calls to it; writ and devlore-test
gain the root test they lack.

### Requirement 3: Every root registers the set through `AddOutputFlags`, and nothing renders beside it

The invariant as first written — every leaf reaches `BuildPipeline` — flags every command that has no
result: `self install`, `config set`, `key generate`. What #753 and #754 actually were is a root that
registered the flags and nothing that honored them: `--output` never validated, `--store` never resolved.
Both are now done at the root by `AddOutputFlags` itself. So the invariant worth holding is two checks:
the root test asserts that the four flags on the root are the ones `AddOutputFlags` binds, by their usage
text; and a source test asserts that nothing outside `cmd/internal/cli` calls `result.NewPipeline`,
`result.FormatterByName` or constructs a formatter — the only way to a rendering is `BuildPipeline`.

### Requirement 4: Each test is shown to fail

A test written after its defect is gone is shown red on a fixture before it is trusted: the source walks
run over a `testdata/` file that violates them, and the tree checkers run over a synthetic tree that
does. Invariant 1 needs no fixture to be shown red — it is red on the shared package today, and the
commit that adds it records the thirteen sites it reports.

### Requirement 5: The shared commands keep the convention

`config get` prints values, `config list` prints `key=value` lines, `config path` prints a path,
`config schema` prints JSON, `config validate` prints verdicts, and `man` prints its install notes; the
`self install` confirmation prompt goes to stdout. Under the convention: results — the value, the list,
the path, the schema, the validation report — go through the pipeline via `cli.Emit`, so `-o json`
works on every one; verdicts and notes narrate through `cli.Note` and `cli.Success`; the prompt is
narration and goes to stderr. This is the work invariant 1 turns up, and it is done here rather than
filed, because a test left red on the package that enforces it is not a test anyone trusts.

### Requirement 6: `devlore-test` is on the shared root (#757)

`devlore-test` builds its own `cobra.Command`: its own `--config`, `--verbose` and `--silent`, its own
pre-run building the narrator, its own `initConfig` that is `initRootConfig` under another name, its own
`version`, `man`, `config` and `self` wiring, and `AddOutputFlags` with a variable its `run` command
reads. Every line of that is what `NewRootCmd` does, which is why #755's help wrapping never reached it.
It moves the way star did: `cli.NewRootCmd(RootConfig{Name: "devlore-test", DefaultConfig:
schema.TestDefaultConfig, ...})`; `run` renders through `cli.Emit` and stops resolving `--store` itself,
since the root's pre-run does; `initConfig`, the static `SilenceUsage` and the hand wiring go. One thing
the shared root must learn first: it derives the environment prefix as the upper-cased name, which for
`devlore-test` is `DEVLORE-TEST` — not a shell variable. `devlore-test` passes `DEVLORE_TEST` by hand
today. `NewRootCmd` maps `-` to `_` in the prefix, for every program.

### Requirement 7: The generated docs agree

`docs/cli` is generated by `devlore-docs` from the command trees, and is gitignored here but published
on every push to `develop`. The pages follow the trees, so the flag-set agreement is proven on the trees
by the root tests, once per program; the commit's script runs `make docs` to prove generation still
succeeds. No test reads the generated pages.

## Implementation Phases

### Phase 1: The checkers, red where they can be (status: complete)

- [x] `cmd/internal/cli/invariants.go` — `CheckNoOwnOutputFlag(root)`, which also refuses any shadow of an
      inherited flag by name or shorthand (ruled while writing it), `CheckSharedSetOnRoot(root)`; and
      the source walks `NoDirectStdout(dirs...)`, `NoPrivatePipeline(dirs...)`, exported so a program's own
      test can call them; `ReservedOutputFlagNames` moved here from star, which now imports it
- [x] `cmd/internal/cli/invariants_test.go` — each checker shown red on a fixture under `testdata/` (six
      writes and two reads; one private import; a synthetic tree; a hand-rolled root), and invariant 1's
      walk over the real tree committed **red**, its thirteen sites in the commit message
- [x] `cli.RunInteractive` — the one seam, TTY-gated, naming the alternative; `config edit` and `man` call
      it; the walk allows `os.Stdout` there alone; refusal and handoff both tested with an injected
      terminal check
- [x] §10 of the design carries the interaction model as ruled: the two kinds of child, the one seam, and
      the six behaviors an interactive command has; the `!` prefix noted as open

### Phase 2: The shared root owns the common set, every root is the shared root, and the shared commands keep the convention (status: complete)

Two phases as chartered, landed as one commit: `cli.Emit` needs the root to own the options, and a second
`AddOutputFlags` on a root that already carries the set panics on the redefined flags, so the root's move
and the programs' move could not be separated.

- [x] `NewRootCmd` registers the common set and owns the `SinkOptions`, per root, so a test may build
      several; `cli.Emit`; `addOutputFlags` unexported; the environment prefix maps `-` to `_`
- [x] lore, star and writ drop their `AddOutputFlags` call, their `outputOptions` and their `emitResult`;
      their commands call `cli.Emit`; lore's and star's `output.go` deleted
- [x] `devlore-test` onto `cli.NewRootCmd` (#757): `run` through `cli.Emit`, its `--store` resolution
      and its local `--dry-run` gone because the root's pre-run and the root's flag do that; `initConfig`,
      the static `SilenceUsage` and the hand wiring go; its help wraps at `COLUMNS=70` like the other three
- [x] `config get`, `list`, `path`, `schema`, `validate` emit results; `man` and the prompt narrate;
      `config validate`'s result is a report `{path, present, valid, warnings}`
- [x] invariant 1's walk went green in this commit; the transition recorded here
- [x] lore's and star's hand-written walks call the checkers; writ and devlore-test gain root tests
      calling them; `TestNewRootCmd_NoUsageTextOnError` already pins the shared root

**The broadened checker found four shadows on its first run over the real tree**, every one the shape the
ruling names: `lore onboard --verbose` shadowed the root's `--verbose` with the same meaning and now reads
the root's; `star devlore test run --dry-run` the same, and its script reads `ctx.dry_run`; `star devlore
model build --model` shadowed the root's `--model` with a different meaning and is `--base-model`;
`star lint go --config` shadowed the root's `--config` with a different meaning and is
`--golangci-config`. Star's loader now refuses any flag that shadows a root flag, so an extension cannot
reintroduce one.

### Phase 3: Close the convention box (status: not started)

- [ ] `make docs` runs in the commit's script and succeeds
- [ ] `10-command-line-interface.status.md` — the enforcement box ticked, the epic's last convention box
- [ ] `740-cli-output-conventions.md` — its Phase 3 writ boxes, landed by #774 and still unticked,
      ticked with the pointer; Phase 5 ticked; `status: complete`
- [ ] `10-command-line-interface.md` §14's invariant table names the tests that enforce each row

## Test Plan

This plan **is** a test plan. The rows are the requirements restated with their failure conditions.

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | No in-scope command package writes to `os.Stdout` directly | source walk | a `fmt.Print` is added to any in-scope package |
| 2 | No command shadows an inherited flag by name or shorthand, none binds a reserved name, and every subcommand inherits the four | tree walk, per root | a command calls `Flags().String("output", …)`, or redefines `--verbose` |
| 3 | The root's four flags are the shared root's; nothing builds a pipeline beside it | tree walk + source walk | a root hand-rolls the set, or a package calls `result.NewPipeline` |
| 4 | `devlore-test`'s help wraps at `COLUMNS=70`, and its `run` renders through `-o` | unit | the root is hand-built again |
| 5 | Each checker is red on its fixture | unit | a checker stops checking |
| 6 | `config get/list/path/schema/validate` render through `-o`; `man` narrates | unit | a shared command prints again |
| 7 | `make docs` generates every page | build | a root breaks generation |

**Not covered:** whether the convention itself is right. These tests enforce the convention as written
in `10-command-line-interface.md`; a change to the convention changes the tests, not the other way
around. And a leaf that has a result and never emits it: undecidable statically, and what the per-command
tests of each program are for.

## Migration Path

`config get`, `config list`, `config path` and `config schema` gain `-o`; their default rendering becomes
json, per the convention, where today they print plain text. `config validate`'s verdict moves to stderr
and its exit code carries the answer. The `self install` prompt moves to stderr. `devlore-test`'s help
wraps, and `DEVLORE_TEST_*` environment variables keep working. Nothing else changes for a user.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/invariants.go` | Create | the checkers and the source walks |
| `cmd/internal/cli/invariants_test.go` | Create | fixtures red; the real tree |
| `cmd/internal/cli/testdata/` | Create | one violating file per source walk |
| `cmd/internal/cli/output.go`, `root.go` | Modify | the root owns the set and the options; `Emit`; `addOutputFlags`; the env prefix |
| `cmd/lore/lore/root.go`, `output.go`; `cmd/star/main.go`, `output.go`; `cmd/writ/writ/root.go`, its `emitResult` | Modify, Delete | the call, the variable and `emitResult` go; `cli.Emit` |
| `cmd/devlore-test/devloretest/root.go`, `commands.go` | Modify | onto `NewRootCmd`; `run` through `cli.Emit` (#757) |
| `cmd/internal/cli/config.go`, `man.go`, `selfinstall.go` | Modify | the thirteen sites |
| `cmd/lore/lore/root_test.go`, `cmd/star/root_test.go` | Modify | call the checkers |
| `cmd/writ/writ/root_test.go`, `cmd/devlore-test/devloretest/root_test.go` | Create | the root tests |
| `cmd/internal/cli/interactive.go` | Create | `RunInteractive`, the one seam |
| `docs/architecture/10-command-line-interface.md` | Modify | §10 the interaction model; §14 names the tests |
| `docs/architecture/10-command-line-interface.status.md` | Modify | the last box |
| `docs/plans/feature/740-cli-output-conventions.md` | Modify | stale boxes; `status: complete` |

## Acceptance criteria

The canonical checklist; the pull request carries a copy and links here. **Every box is checked before
merge**, and a box that cannot be checked from this branch says what checks it.

**Goals**

- [x] Three invariants have a test each, and each was shown red — invariant 1 on the tree, the others on
      fixtures — with the red output in the commit that added it — phase 1
- [x] The shared commands keep the convention: `config get/list/path/schema/validate` through `-o`, `man`
      and the prompt on stderr — phase 2
- [x] Every in-scope root is `cli.NewRootCmd`, which registers the common set once; no program calls
      `AddOutputFlags`, which no longer exists by that name; every root has a root test calling the
      checkers (#757 closes here) — phase 2
- [ ] `make docs` succeeds; the status doc's enforcement box and the 740 plan are closed

**Test rows**

- [x] 1 — no direct stdout write in an in-scope package (source walk) — red at phase 1, green at phase 2
- [x] 2 — no shadowed inherited flag, no reserved name, every subcommand inherits the four, per root (tree walk)
- [x] 3 — the root's set is the shared root's; no private pipeline (tree walk, source walk)
- [x] 4 — `devlore-test` wraps help at `COLUMNS=70` and renders through `-o` (unit)
- [x] 5 — each checker red on its fixture (unit)
- [x] 6 — the shared `config` subcommands render through `-o`; `man` narrates (unit)
- [ ] 7 — `make docs` generates every page (build, in the commit's script)
- [ ] 8 — the suite passes on **all five platforms**: darwin-arm64 locally; **checked here when every
      `test (…)` leg is green**

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1, the epic's plan; this closes its convention box
- [774-writ-reconcile.md](774-writ-reconcile.md), [775-lore-adoption.md](775-lore-adoption.md),
  [743-star-adoption.md](743-star-adoption.md) — the adoption work these tests certify
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) — the convention enforced
- Issue [#776](https://github.com/NobleFactor/devlore-cli/issues/776)
- Issue [#757](https://github.com/NobleFactor/devlore-cli/issues/757) — `devlore-test` onto the shared root; folded here, closes with this PR
- An issue to file: `scripts/Get-EpicReport` gains `--by`, `--view` and `--output`, the thread status document (chore, resolved in this worktree)

## Open Questions

- [ ] A `!` prefix, so a user mid-onboarding or mid-migration can run a shell command and have its output
      land in the flow — the way an agent's terminal does it. Raised 2026-09-03; to be talked through, not
      part of this plan.
- [ ] `config validate` today prints "Config x is valid" and exits 0, or prints warnings; under the
      convention the verdict is the exit code and the warnings are narration — is there a result at all?
      Proposed: a report `{path, valid, warnings[]}` as the result, so `-o json` serves a script.
