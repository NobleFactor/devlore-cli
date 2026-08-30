---
title: "One output convention, every app"
issue: https://github.com/NobleFactor/devlore-cli/issues/740
status: draft
created: 2026-08-28
updated: 2026-08-28
---

# Plan: One output convention, every app

## Summary

`cmd/internal/cli` already defines an output convention -- `AddOutputFlags` binds `--filter`, `--format`,
and `--jq`; `BuildPipeline` composes a `result.Pipeline` of filter, formatter, and sink. One
command out of forty-six uses it. Every other command invents its own flags. The convention also has no way
to point at the **execution store**, which is where definitions and traces persist, so that has been
improvised too. This plan adds `--store`, renames `--format` to `--output` / `-o`, adds the `none`
rendering, and adopts the whole set everywhere.

## Goals

- [ ] `--store` is part of the shared convention, not per-command improvisation.
- [ ] `--output` / `-o` selects the rendering, as in `aws`, `az`, and `kubectl` -- never a destination.
- [ ] `--output none` turns the result off, reachable from config and env where a shell is not.
- [ ] `devlore-test`, `lore`, `star`, and `writ` each register the full common set on their root, so
      every command of all four accepts every flag.
- [ ] No command invents an output flag of its own.
- [ ] Results go to stdout or a file; narration goes to stderr. Enforced, not merely stated.
- [ ] A boolean `--json` does not exist anywhere.

## Current State

Measured, not estimated.

### The convention exists and is unadopted

`cmd/internal/cli/output.go` binds four flags and builds the pipeline, with a doc comment stating the intended
usage: "Call once during command setup, then call [BuildPipeline] from the cobra RunE."

**Adoption is one call site**: `cmd/lore/lore/commands.go:838`, the `inspect` command.

**And it has gone backwards.** `docs/plans/extract-output-package.md` (complete, 2026-03-19) records
`AddOutputFlags` as "Used | 2 call sites (lore inspect, **writ snapshot**)". Today there is one. `writ
snapshot` no longer exists as a command -- the `cmd/writ/writ/snapshot/` package remains, but nothing
registers a `Use: "snapshot"` anywhere in `cmd/`. A command was removed, the convention lost half its
adopters, and no document noted it.

That is this plan's justification in one line: **adoption that is not enforced decays silently.** Phase 5's
invariants exist so the next removal cannot quietly halve it again. It is also why the same plan's other
claims need checking rather than trusting -- it is marked complete while describing an `internal/output`
package that does not exist; the code went to `pkg/result` + `pkg/sink` instead.

| App | Commands | Using the convention |
| --- | --- | --- |
| lore | 19 | 1 |
| writ | 13 | 0 |
| star | 9 | 0 |
| devlore-docs | 3 | 0 |
| devlore-test | 2 | 0 |

Twelve hand-rolled `--output` / `--format` / `--json` registrations exist across `cmd/`.

### The convention has no destination

`AddOutputFlags` covers rendering only. `BuildPipeline(opts, w io.Writer)` takes the writer as a parameter, so
`--output` is not part of the contract. That is why `lore bundle`'s `-o`, `lore onboard`'s `--output`, and
devlore-test's stream routing all sit outside it: there was nothing to conform to.

### How far each app has drifted

| Command | Today | Wrong how |
| --- | --- | --- |
| `lore inspect` | `AddOutputFlags` | correct |
| `lore bundle` | `--output, -o <path>` | destination outside the convention |
| `lore onboard` | `--output <dir>` + `--format` | own `--format`, own destination |
| `lore list` | `--format table\|manifest\|json` | own `--format`, values differ |
| `writ status` | `--json` (**bool**) | a boolean, not a format |
| `writ verify` | `--json` (**bool**) | a boolean, not a format |
| `writ migrate` | `--format json\|yaml\|text` | own `--format`; help text corrupted (#739) |
| `devlore-test run` | `--output stream=dest` x3, `--receipt-format` | stream routing; a second format flag |

### The stdout/stderr split is a comment, not a rule

`devlore-test`'s source records the ruling -- "results are files, narration is stderr, and stdout stays clean
(ruled 2026-08-20)" -- but nothing enforces it. `writ` writes to `os.Stdout` directly in seven places; `lore`
uses `fmt.Print` thirteen times.

## Requirements

### Requirement 1: `--store` names the execution store's root

`cmd/internal/cli/store.go` defines a store, not a directory: a definition persists once under `GraphsDir()`
keyed by its checksum; a trace persists per run under `TracesDir()` in a per-definition subdirectory, tied
back through `Trace.GraphChecksum`, with a per-definition run index.

`--store <dir>` relocates that whole structure -- both subdirectories, the checksum keying, and the index.
Writing loose files into `--store` is not an implementation of this flag; it severs a trace from the
definition it ran.

The store is already user-visible (`writ secret`'s help names it). What has never existed is a way to point
at a different one.

### Requirement 2: `--output` / `-o` selects the rendering

`--format` is renamed. Five tools were surveyed and the split is 3-2 -- `aws`, `az`, and `kubectl` use
`--output`; `docker` and `gcloud` use `--format`. The full weighing, including the case against, is design
decision 1 of `docs/architecture/10-command-line-interface.md`.

The suite adds no `--output <file>`: a result reaches a file by shell redirection, because stdout already is
one.

### Requirement 2a: `none` is a rendering, not a flag

`--output none` produces no result at all -- the command does not render it. This is not `> /dev/null`, which
renders the result and then discards it, and it is reachable from a config file or an environment variable
where no shell exists to redirect. `aws` ships it as `off` for exactly this reason; `az` and `gcloud` spell it
`none`, and the majority spelling wins.

### Requirement 3: The common set is registered once per app, on the root

`devlore-test`, `lore`, `star`, and `writ` each call `AddOutputFlags` on their root command, as persistent
flags. Every command of all four then accepts every flag in the set, without opting in.

A command that emits a result calls `BuildPipeline` in its `RunE`. No command formats a result by hand, and
no command registers an output flag of its own.

### Requirement 4: Narration is stderr, results are stdout or a file

`cli.Note`, `cli.Warn`, `cli.Error`, and `cli.Success` narrate to stderr. Results go to the sink. A direct
`fmt.Print` or `os.Stdout` write in a command is a defect.

### Requirement 5: No boolean format flags

`--json` is replaced by `--output json`. A boolean cannot express a third format, which is why `writ status`
and `writ verify` cannot render yaml today.

## Implementation Phases

### Phase 1: Extend the convention

- [ ] Add `Store string` to `SinkOptions` and `--store` to `AddOutputFlags`.
- [ ] `AddOutputFlags` registers on `PersistentFlags()`, so a root call covers every subcommand.
- [ ] Rename `--format` to `--output` with the `-o` short form, via `StringVarP`.
- [ ] Add the `none` rendering to `pkg/result`.
- [ ] Resolve a store root, defaulting to the state root `GraphsDir`/`TracesDir` use today.
- [ ] `GraphsDir()` and `TracesDir()` resolve under the chosen root, together.
- [ ] Tests that a relocated store keeps its layout, checksum keying, and run index.

### Phase 2: Bring devlore-test into agreement

- [ ] Replace `--output stream=dest` and `--receipt-format` with `--output` and `--store`.
- [ ] Fix what each artifact contains -- #738: the graph document is currently written to the stream named
      `receipt`, and the stream named `graph` emits `t.run` results.
- [ ] Persist an execution trace, or state its absence as a decision (#738).
- [ ] Assert artifact CONTENT in `cli_test.go`; existence-only assertions are why #738 survived.

### Phase 3a: Bring writ's flags into agreement -- COMPLETE

- [x] `--json` becomes `--output` on `status` and `verify`, by deletion rather than alias.
- [x] `writ` registers the common set on its root; every subcommand accepts every flag.
- [x] `verify.Execute` returns `[]Report` and the command emits them. Rendering belongs to the pipeline, not
      to the package that computes the answer.
- [x] The corrupted flag descriptions are corrected (#739).

**The scope was measured wrong when this plan was written.** It said "the seven direct `os.Stdout` writes",
counted by grepping the literal `os.Stdout` -- which misses every `fmt.Print`, and those write to stdout just
the same. The real figure is **30 call sites**:

| Location | Calls | What it is |
| --- | --- | --- |
| `cmd/writ/writ/status/report.go` | 22 `fmt.Print*` | the human status report |
| `cmd/writ/writ/verify/verify.go` | 2 `fmt.Print*` | now removed with `presentReport` |
| `deploy`, `decommission`, `upgrade`, `secret` | 4 `SerializeGraphs(os.Stdout, ...)` | the dry-run plan dump |
| `cmd/writ/writ/migrate/session.go` | 1 `os.Stdout` | TUI session output |
| `cmd/writ/writ/migrate_cmd.go` | 1 `os.Stdout` | `FormatMigrationPlan` |

`migrate_cmd.go:157`'s `os.Stdout.Stat()` is a TTY check, which §9 permits, and is not among them.

### Phase 3b: Bring writ's renderings into agreement -- BLOCKED on the TableFormatter

The status report is a human table. Converting it needs the shared `TableFormatter` scheduled in Phase 4, so
this phase follows it rather than preceding it.

- [ ] `status/report.go`'s 22 `fmt.Print` calls render through the pipeline as `table`.
- [ ] The four dry-run `SerializeGraphs(os.Stdout, ...)` dumps emit the plan as the command's result.
- [ ] `migrate`'s own `--format` retires; its `text` rendering is a domain question like `lore list`'s
      `manifest`, and does not join the shared set.
- [ ] `migrate/session.go`'s stdout write is classified: narration to stderr, or a result to the pipeline.

### Phase 3c: The format value accepts an argument

- [ ] `--output` parses `NAME=ARGUMENT`, splitting on the first `=`. A bare name is unchanged.
- [ ] `template=<body>` renders through a Go template. It is the only argument-taking format at first.
- [ ] `value` renders raw: no quoting, no document syntax, no header. It is what makes `--jq` complete --
      a jq-built string has no other format that prints it as written.
- [ ] An unknown `NAME` and a `NAME=` with an empty argument both error, naming the format.

### Phase 4: Bring star and lore into agreement

- [ ] Promote `cmd/star/cli/output.go`'s `renderTable` into `pkg/result` as the suite's one
      `TableFormatter`, fixing the rune-safety defect #741 records. It is a move, not a fresh write.
- [ ] Convert `lore`'s `runSearch` (`commands.go:525-556`) first: it is the only real table in the tree, and
      converting it is a bug fix as much as a refactor (see the byte-truncation defect it carries).
- [ ] `star` registers the common set, and `cmd/star/cli` is deleted -- it duplicates eighteen
      exported names from `cmd/internal/cli` and now contradicts it (#743).
- [ ] `lore`'s `bundle`, `onboard`, and `list` drop their hand-rolled flags. `list` is a stub returning
      "not yet implemented", so it adapts at no cost; its `--format manifest` is a domain rendering and does
      not join the shared set.
- [ ] The thirteen `fmt.Print` calls are triaged: narration to `cli.*`, results to the sink.

### Phase 5: Enforce it

- [ ] A test that fails when a command registers an output flag of its own.
- [ ] A test that fails on a direct `os.Stdout` write from a command package.
- [ ] Regenerate the CLI docs and confirm every command documents the same flags.
- [ ] Correct `docs/plans/extract-output-package.md`, which is marked complete while describing an
      `internal/output` package that was never created.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | A result with no flags goes to stdout as JSON | unit | The default changes |
| 2 | `-o table` renders a table and creates no file named `table` | unit | `-o` is bound to a destination |
| 3 | `--store <dir>` puts definitions and traces under that root | unit | Only one subdirectory moves |
| 3a | A relocated store keeps its run index and checksum keying | unit | The store is treated as a dump dir |
| 4 | A trace in a relocated store still resolves to its definition | unit | `GraphChecksum` ties break |
| 5 | Every format value round-trips through the pipeline | unit | A formatter is unregistered |
| 6 | `-o json` and `-o yaml` both work on `writ status` | unit | `--json` boolean returns |
| 6a | `-o none` emits nothing on stdout, errors still on stderr | unit | `none` renders anyway |
| 7 | Narration appears on stderr while stdout holds only the result | unit | A `cli.Note` reaches stdout |
| 8 | No command package writes to `os.Stdout` directly | unit | A direct write is added |
| 9 | No command registers its own `--output`/`--store`/`--json` | unit | A hand-rolled flag is added |
| 9a | All four roots expose the full common set on every subcommand | unit | A root skips the set |
| 10 | devlore-test's graph artifact contains the graph document | unit | #738 regresses |
| 11 | An artifact that should have content is not empty | unit | Existence-only assertions return |

**Write the failing test first.** Rows 8 and 9 are the ones that keep this from drifting back; both are
greppable invariants and both are red today.

**Not covered, as a decision:** the `result.Pipeline` formatters themselves are already tested; this plan does
not re-test them. Nor does it change what any command *computes* -- only where the answer goes and how it is
rendered.

## Migration Path

Greenfield, per the repository's governing principle: no released CLI contract needs preserving. `--json`
disappears rather than being aliased; a `--json` alias would be exactly the backward-compatibility shim the
governing principle forbids.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/internal/cli/output.go` | Modify | `--store` joins; `--format` becomes `--output` / `-o` |
| `cmd/internal/cli/output_test.go` | Modify | Store-root resolution and flag binding |
| `cmd/internal/cli/store.go` | Modify | `GraphsDir`/`TracesDir` resolve under a chosen root |
| `pkg/result` | Modify | The `none` renderer; `TableFormatter` in Phase 4 |
| `cmd/devlore-test/devloretest/commands.go` | Modify | Adopt the convention; fix artifact contents |
| `cmd/devlore-test/cli_test.go` | Modify | Assert content, not existence |
| `cmd/writ/writ/commands.go` | Modify | `--json` retires; sink replaces direct stdout writes |
| `cmd/writ/writ/migrate_cmd.go` | Modify | Adopt the convention; corrected help text |
| `cmd/lore/lore/commands.go` | Modify | `bundle`, `onboard`, `list` adopt the convention |
| `docs/cli/**` | Regenerate | The generated reference follows the flags |
| `docs/architecture/10-command-line-interface.md` | Create | The spec this plan implements |
| `docs/plans/extract-output-package.md` | Modify | Correct a completed plan that describes absent code |

## Related Documents

- Epic #740 -- Command line interface: one output convention, every app
- Issue #743 -- `cmd/star/cli` duplicates `cmd/internal/cli`
- Issue #742 -- CLI code under the root `internal/`
- Issue #738 -- devlore-test is broken: empty artifact, inverted streams, no trace
- Issue #739 -- CLI flag help says "Promise ..." where it means "Output ..."
- `docs/architecture/10-command-line-interface.md` -- the spec, including the five-tool survey and the
  settled flag names; this plan is its implementation
- `docs/plans/extract-output-package.md` -- where the convention came from, and where adoption was
  last measured at two call sites
- `cmd/internal/cli/output.go` -- the convention this plan extends and adopts

## Open Questions

- [ ] `lore list` defaults to `table` today while the convention defaults to `json`. Settled in the spec as
      json-always; confirm no command needs an exception before Phase 4 converts `list`.
- [ ] Do `--filter` and `--jq` belong on every command, or only on those emitting collections?
