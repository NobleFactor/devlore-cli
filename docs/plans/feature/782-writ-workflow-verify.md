---
title: "Command naming and package layout: the workflow group on the shared root, groups that take no action, and cmd/internal"
issue: https://github.com/NobleFactor/devlore-cli/issues/782
status: chartered
created: 2026-09-10
updated: 2026-09-11
---

# Plan: Command naming and package layout

## Summary

Feature [#835](https://github.com/NobleFactor/devlore-cli/issues/835) of the command-line epic, and the first of
the four pull requests that close it. Three rows of the epic's closure ledger
([#740, frozen 2026-09-10](https://github.com/NobleFactor/devlore-cli/issues/740#issuecomment-5630028207)): row 8,
`writ verify` retires into a `workflow` group on the shared root that addresses the store's documents by name;
row 7, a subcommand group takes no action; row 9, CLI-only code under `cmd/internal`, and the packages that stay at
root `internal/` say why. Every change applies a ruling on the record: §2, §3, §6 and §14 of the specification, and
#742's own text. The `workflow` ruling was made 2026-09-11 reviewing this plan and is written into §6 and design
decision 10 in this worktree.

## Issue 782

`writ verify` verifies publisher signatures on the store's documents and sits at the root beside the four lifecycle
verbs, which all act on the deployment; a reader reaching for "check my environment" picks it and is wrong. Its one
operand is a path, and the only name a stored document has on disk is a checksum, so a person cannot name a stored
document to it at all.

**Ruled 2026-09-11.** The store holds documents *of* a workflow: its definition and the execution traces of its
runs. The store is one store for all four programs, and the run index is the one place a person's name for a
document exists: tool, scope, checksum, time. So the shared root gains a `workflow` group, identical on every
program, with `list` over the index and `verify` selecting from it; a path operand stays the form for a document
from outside the store. `writ verify` is deleted (§13). The noun is `workflow`, not `document`: selection is by
workflow, and `--kind` picks which of its documents.

## Issue 742

Every package under the repository-root `internal/` is imported only by `cmd/`. Root `internal/` is importable by
the whole module, so nothing stops a `pkg/` package from importing a terminal UI; `cmd/internal/` makes that a
compile error. #742 asks that `internal/console` move and that each of the other three be moved or left with the
reason written down.

| Package | Is | Imported by | Decision |
| --- | --- | --- | --- |
| `console` | a Bubble Tea terminal UI | `cmd/writ/writ`, `cmd/writ/writ/migrate` | **moves** to `cmd/internal/console`: CLI presentation |
| `credentials` | credential storage for the CLI's AI provider keys | `cmd/internal/config`, `cmd/internal/model` | **moves** to `cmd/internal/credentials`: both importers are already there |
| `manifest` | the packages-manifest format, loaded by lore and writ | `cmd/lore/lore`, `cmd/writ/writ/deploy`, `cmd/writ/writ/tree` | **stays**: a document format is domain, not presentation; a `pkg/` reader is legitimate |
| `registry` | the registry transport (git) | `cmd/internal/lorepackage`, `cmd/lore/lore`, `cmd/writ/writ` | **stays**: the devlore provider (#877) takes `lorepackage` and its transport toward `pkg/`; moving it into `cmd/internal` now moves it twice |

The two that stay carry the reason in their package doc comment, which is where #742's "justified in writing" is
read.

## Before and after

**Row 8, the workflow group.** Usage, then invocations. The group exists on every program; `writ` stands for any.

```
before   writ verify <document>... [flags]                        # writ only; a path is the only operand

after    writ workflow                                             # the group: prints help, acts on nothing
         writ workflow list [flags]
         writ workflow verify [--tool <name>] [--scope <name>] [--kind definition|trace] [--latest] [<document>...]

before   writ verify ~/.local/state/devlore/graphs/*.yaml
after    writ workflow verify                                      # every document of writ's workflows
         writ workflow verify --scope home                         # the Home workflow: definition and every trace
         writ workflow verify --scope home --kind trace --latest   # its newest execution trace alone

before   writ verify --signing-policy=reject_external ~/Downloads/shared-plan.yaml
after    writ workflow verify --signing-policy=reject_external ~/Downloads/shared-plan.yaml

before   (nothing lists the store)
after    writ workflow list                                        # tool, scope, definition checksum, runs, latest run
         writ workflow list -o table
         lore workflow list                                        # the same command, lore's workflows
         lore workflow list --tool writ                            # another program's

after    writ verify ...                                           # Error: unknown command "verify" for "writ"
after    writ workflow verify --scope home trace.yaml              # refused: operands and selectors do not mix
```

`verify`'s flags are unchanged: `--signing-policy {ignore|report|reject_external|reject}` and
`--allowed-signers <path>`, plus the common set inherited from the root.

**Row 7, the group.**

```
before   writ repo                              # lists the registrations
after    writ repo                              # prints the group's help, exit 0
         writ repo list                         # lists the registrations, as it does today
```

**Row 9, the layout.** One import line, as every one of the fourteen changes:

```
before   "github.com/NobleFactor/devlore-cli/internal/console"
after    "github.com/NobleFactor/devlore-cli/cmd/internal/console"
```

`manifest` and `registry` keep their paths.

## Goals

- [ ] The `workflow` group, `list` and `verify`, on the shared root of all four programs; `writ verify` gone (row 8).
- [ ] `writ repo` invoked bare prints help; `repo list` lists (row 7).
- [ ] `internal/console` and `internal/credentials` live under `cmd/internal`; `manifest` and `registry` state
      why they stay (row 9).
- [ ] A mechanical check that a group takes no action, called from the roots whose trees satisfy it.
- [ ] Design and user documentation updated: the specification now, the user documentation in the last commit,
      alongside completion (ruled 2026-09-11).

## Current State

Measured 2026-09-10 in the worktree, against develop at f03794d0.

| Component | Status | Notes |
| --- | --- | --- |
| `cmd/writ/writ/commands.go:272` | ❌ | `verify <document>...` at writ's root; a path is the only operand |
| `cmd/writ/writ/verify/verify.go` | ⚠️ | `Execute(ctx, cfg) ([]Report, error)` over paths; the verifier is sound and moves whole |
| `cmd/internal/cli/index.go`, `store.go` | ⚠️ | `ReadIndex` returns every event; nothing groups them into workflows |
| `pkg/op/graph.go:673` `Graph.Filename()` | ⚠️ | a scope-and-time filename nothing calls; the store names by checksum |
| `cmd/writ/writ/repo_cmd.go:42` | ❌ | the `repo` group carries `RunE` and lists registrations when bare |
| `star setup` | placement | a leaf with an action that the loader then uses as a parent: #841's, not this plan's |
| `internal/{console,credentials,manifest,registry}` | ❌ | root `internal/`, fourteen import sites across `cmd/` |
| `cmd/internal/cli/invariants.go` | ⚠️ | four checkers; none for §3's "a group takes no action" |
| `docs/architecture/10-command-line-interface.md` | ✅ | §2, §6 and decision 10 carry the ruling, in this worktree |

## Requirements

### Requirement 1: The workflow group (row 8)

`NewRootCmd` adds `workflow`, a noun, taking no action, with two leaves.

**`list`** reads the run index and folds its events into one record per workflow, keyed by the definition's
checksum: `tool`, `scope`, `definition` (the checksum), `runs` (the trace-event count), `latest_run` (the newest
trace event's time). Records are a result through `cli.Emit`, sorted by tool, scope, then latest run descending.

**`verify`** takes selectors or operands, never both. Selectors resolve documents from the index: for each selected
workflow, the definition at `GraphsDir()/<checksum>.yaml` and each trace at `TracesDir()/<checksum>/<trace_file>`;
`--kind` keeps one kind; `--latest` keeps the newest trace per definition. Operands are paths, for documents outside
the store. Each resolved path is verified exactly as today; one `Report` per document; the signing-policy ladder and
`--allowed-signers` are unchanged. `--tool` defaults to the invoking program's name, from `RootConfig.Name`;
`--scope` and `--kind` default to all. A selection that matches nothing is an empty result, exit 0 (§8, S8). Operands
and selectors together are refused naming both.

The verifier, `verify.Config`, `Report`, `Execute` and its helpers, moves from `cmd/writ/writ/verify` to
`cmd/internal/cli` unchanged in behavior; its integration test moves with it and drives the shared root.
`writ verify` is deleted from writ's tree. `Graph.Filename()` is deleted: the store names by checksum, and a function
that names otherwise is a second naming nobody uses.

### Requirement 2: A group takes no action (row 7)

`writ repo` loses its `RunE`, its `Args`, the "With no subcommand, repo lists" sentence and the bare example;
invoked bare it prints help like every other group. `repo list` is unchanged. `CheckGroupsTakeNoAction` joins
`cmd/internal/cli/invariants.go` beside the other checkers: every command with subcommands has no `Run` or `RunE`. It
is called from writ's, lore's and devlore-test's root tests now. star's root test calls it when #841 lands, and #841
records that; star's `setup` is the loader's making, and this plan does not touch the loader.

### Requirement 3: The layout (row 9)

`git mv internal/console cmd/internal/console` and the same for `credentials`; fourteen import paths rewritten;
`manifest` and `registry` gain the sentence in the table above in their package doc comments. The acceptance "no
`pkg/` package can import CLI presentation code" is Go's own rule for `cmd/internal` and needs no test.

### Requirement 4: Design and user documentation

**Ruled 2026-09-11: the design record changes with the ruling; the user documentation changes in the last commit,
the one that marks the task complete.** The specification's §2, §6 and decision 10 are already edited in this
worktree and commit with the plan. Everything below lands in the final commit. Enumerated with
`grep -rn 'writ verify' docs`:

| Document | Sites | Change |
| --- | --- | --- |
| `docs/guides/writ/graphs-and-traces.md` | §Verification heading, :79, :85, :86, :112 | the section becomes "Verification: `writ workflow verify`"; the vocabulary is definition and execution trace; the examples are the after-forms above, including `workflow list` |
| `docs/architecture/1-system-model.md` | :255, :367 | the name |
| `docs/architecture/2-execution-graph.md` | :126 | the name |
| `docs/architecture/5-graph-trace-integrity.md` | :142, :179 | the name |
| `docs/architecture/2-execution-graph.status.md` | :18 | the name |
| `docs/architecture/5-graph-trace-integrity.status.md` | :12 | the name |
| `docs/architecture/10-command-line-interface.md` §14 | the table | invariant 7: a group takes no action, enforced by `CheckGroupsTakeNoAction` from every root test |
| `docs/architecture/10-command-line-interface.md` §15 | the writ row | the `internal/console` sentence under "CLI code also lives outside cmd/" becomes past tense, naming this PR |
| `docs/cli/**` | generated | `make docs` regenerates; `writ/workflow*.md` and `lore/workflow*.md` appear, `writ/verify.md` goes; the publish workflow carries them |
| `internal/manifest/manifest.go`, `internal/registry/transport.go` | package doc | the sentence saying why each stays at root `internal/` |

The phase-8 plans under `docs/plans/extract-starlark-from-op/` that name `writ verify` are closed history and are
not edited.

## Design

**Why the shared root, not writ.** §2: the store is one store, the index carries `tool`, and a command that reads
the index is every program's. A writ-only `workflow` would be a second copy of the shared concept, which §15 records
as the way conventions decay.

**Why `--tool` defaults to the invoking program.** `writ workflow list` listing lore's runs would surprise; `gh`
defaults to the current repository for the same reason. The flag reaches the rest.

**Why operands and selectors do not mix.** Two ways to name the same document in one invocation is the two-input
shape §8 rejects for `--output` and `--template`: a rule to document, enforce and get wrong. One or the other.

**Why the group check is a checker, not a one-off test.** §14's invariants are mechanical or they decay, and this
one is a two-line walk. A root that gains a bare-acting group fails its own root test.

**Why star's half of row 7 is not here.** The extension grammar has no way to say "this name is a group", so the
loader registers `setup` as a leaf and hangs `setup.check` under it. Fixing the extension alone is not possible until
the grammar can express a group, which is #841, under the star epic. This plan's checker is what #841's fix will
satisfy.

## Implementation Phases

### Phase 1: The plan and the ruling

- [ ] This document and the specification's §2, §6 and decision 10, the branch's first commit; `status: chartered`
      on the review; the ledger's row 8 on #740 notes the ruling.

### Phase 2: The workflow group (row 8)

- [ ] The verifier moves to `cmd/internal/cli` with its integration test; `writ verify` and `Graph.Filename()` are
      deleted.
- [ ] `workflow list` over the index; the record type; the sort.
- [ ] `workflow verify`: selectors, resolution from the index, the mixing refusal, the path operand path.
- [ ] `NewRootCmd` registers the group; every root test sees `workflow list` and `workflow verify`.

### Phase 3: A group takes no action (row 7)

- [ ] `writ repo`: `RunE`, `Args`, the sentence and the bare example removed.
- [ ] `CheckGroupsTakeNoAction` in `invariants.go`, red on a fixture, called from three root tests.
- [ ] §14 gains invariant 7; #841 gains the note that star's root test calls the checker when it lands.

### Phase 4: The layout (row 9)

- [ ] `console` and `credentials` under `cmd/internal`; imports rewritten; `make build` green.
- [ ] `manifest` and `registry` doc comments carry the reason they stay.

### Phase 5: User documentation and closure, one commit

- [ ] The guide's §Verification: heading, vocabulary, the example commands including `workflow list`.
- [ ] The five architecture documents and the two status files; §15's `internal/console` sentence.
- [ ] `make docs` regenerates `docs/cli/*/workflow*.md`; `writ/verify.md` is gone.
- [ ] #782, #742 acceptance boxes ticked; rows 7, 8, 9 marked done on #740; this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `workflow verify <path>` verifies a stored document by path, on every program's root | integration, cli | the verifier did not move whole |
| 2 | `workflow verify --scope home` resolves the definition and every trace from a fixture index; `--kind`, `--latest` narrow as ruled | unit, cli | resolution reads the wrong field |
| 3 | `workflow verify --scope home doc.yaml` is refused naming both | unit, cli | the mixing rule is unenforced |
| 4 | `workflow list` folds a fixture index into one record per workflow with the right counts and latest time | unit, cli | the fold is wrong |
| 5 | `--tool` defaults to the invoking program | unit, cli | the default is unset or wrong |
| 6 | `writ verify` is unknown; `writ workflow` bare prints help | unit, writ root | the rename is partial |
| 7 | `writ repo` bare exits 0 with help and lists nothing | unit, writ root | the RunE survives |
| 8 | `CheckGroupsTakeNoAction` is red on a fixture with a bare-acting group | unit, cli | the walk misses RunE |
| 9 | writ, lore, devlore-test roots pass the check | unit, root tests | a group acts |
| 10 | `make build`, `make test` green after the moves | build | an import path was missed |
| 11 | `grep -rn 'writ verify' docs --include='*.md' \| grep -v docs/plans` returns nothing | doc gate | a document was missed |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/feature/782-writ-workflow-verify.md` | Create | this plan |
| `docs/architecture/10-command-line-interface.md` | Modify | §2, §6, decision 10 now; §14, §15 in the last commit |
| `cmd/internal/cli/workflow.go` | Create | the group, `list`, `verify`'s selectors and resolution |
| `cmd/internal/cli/verify.go` (from `cmd/writ/writ/verify/verify.go`) | Move | the verifier, unchanged |
| `cmd/internal/cli/verify_integration_test.go` (from `cmd/writ/writ/verify/`) | Move | drives the shared root |
| `cmd/internal/cli/workflow_test.go` | Create | rows 2 through 5 |
| `cmd/internal/cli/root.go` | Modify | registers `workflow` |
| `cmd/writ/writ/commands.go` | Modify | `verify` removed |
| `pkg/op/graph.go` | Modify | `Filename()` removed |
| `cmd/writ/writ/repo_cmd.go` | Modify | the group takes no action |
| `cmd/internal/cli/invariants.go`, `invariants_test.go` | Modify | `CheckGroupsTakeNoAction` and its fixture |
| `cmd/writ/writ/root_test.go`, `cmd/lore/lore/root_test.go`, `cmd/devlore-test/devloretest/root_test.go` | Modify | call the checker |
| `internal/console/**` → `cmd/internal/console/**` | Move | row 9 |
| `internal/credentials/**` → `cmd/internal/credentials/**` | Move | row 9 |
| fourteen importers under `cmd/` | Modify | import paths |
| `internal/manifest/manifest.go`, `internal/registry/transport.go` | Modify | the reason they stay |
| `docs/guides/writ/graphs-and-traces.md` | Modify | last commit: `writ workflow verify`, `workflow list` |
| `docs/architecture/1-system-model.md`, `2-execution-graph.md`, `5-graph-trace-integrity.md` | Modify | last commit: the name |
| `docs/architecture/2-execution-graph.status.md`, `5-graph-trace-integrity.status.md` | Modify | last commit: the name |

## Related Documents

- [#835](https://github.com/NobleFactor/devlore-cli/issues/835) -- the feature; #782 and #742 its tasks
- [#740](https://github.com/NobleFactor/devlore-cli/issues/740) -- the epic; the closure ledger, rows 7, 8, 9
- [#451](https://github.com/NobleFactor/devlore-cli/issues/451) -- the rename that makes `workflow` the code's word too
- [#841](https://github.com/NobleFactor/devlore-cli/issues/841) -- star's groups; the star half of row 7
- [#791](https://github.com/NobleFactor/devlore-cli/issues/791) -- `repo set`/`unset`; owns the add/remove text this
  plan leaves in place
- [#877](https://github.com/NobleFactor/devlore-cli/issues/877) -- the devlore provider; why `registry` stays
- [#884](https://github.com/NobleFactor/devlore-cli/issues/884) -- exit codes; the mixing refusal exits 64 once it lands
- `docs/architecture/10-command-line-interface.md` §2, §3, §6, §14 -- the rulings applied
