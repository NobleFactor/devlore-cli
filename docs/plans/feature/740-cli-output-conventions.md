---
title: "One output convention, every app"
issue: https://github.com/NobleFactor/devlore-cli/issues/740
status: in-progress
created: 2026-08-28
updated: 2026-09-01
---

# Plan: One output convention, every app

## Summary

`cmd/internal/cli` already defines an output convention -- `AddOutputFlags` binds `--filter`, `--format`,
and `--jq`; `BuildPipeline` composes a `result.Pipeline` of filter, formatter, and sink. One
command out of forty-six uses it. Every other command invents its own flags. The convention also has no way
to point at the **execution store**, which is where definitions and traces persist, so that has been
improvised too. This plan adds `--store`, renames `--format` to `--output` / `-o`, adds the `none`
rendering, and adopts the whole set everywhere.

## Where we are (2026-09-01)

This plan is **thread 1 of four**, worked in order: this epic, then resource management
(`Epic:ResourceModel`), then the writ lifecycle surface ([#762](https://github.com/NobleFactor/devlore-cli/issues/762)),
then unified configuration ([#441](https://github.com/NobleFactor/devlore-cli/issues/441), planned at
[441-unified-configuration.md](441-unified-configuration.md)). Threads 1 and 3 were previously
entangled -- #762 renames the command this epic's next phase rewrites. The work lands as
`writ reconcile`: the rename is part of it, not a later step, so this epic's writ phase and #762's phase 2
are one piece of work rather than two adjacent ones.

**Four items remain**, in the epic's own order:

| # | Item | Phase | Issue |
| --- | --- | --- | --- |
| 1 | `writ reconcile` -- the rename and the 30 stdout call sites, one piece of work | 3b | **landed** -- [#774](https://github.com/NobleFactor/devlore-cli/issues/774), [774-writ-reconcile.md](774-writ-reconcile.md) |
| 2 | `lore` -- search table, hand-rolled flags, thirteen `fmt.Print` | 4 | [#775](https://github.com/NobleFactor/devlore-cli/issues/775) -- [775-lore-adoption.md](775-lore-adoption.md); [#741](https://github.com/NobleFactor/devlore-cli/issues/741) closes there only if verified |
| 3 | `star` -- register the set, delete `cmd/star/cli` | 4 / 4b | [#743](https://github.com/NobleFactor/devlore-cli/issues/743) -- [743-star-adoption.md](743-star-adoption.md) |
| 4 | Enforcement -- the invariant tests | 5 | [#776](https://github.com/NobleFactor/devlore-cli/issues/776) -- [776-output-enforcement.md](776-output-enforcement.md) |

All four now have issues and plans. `lore`'s is kept **separate** from #741 so a user-visible defect is
not buried inside a refactor; enforcement is one issue for both tests, since they are the same invariant
seen twice and together they define what "done" means for this epic -- which is why it is sequenced
last, red until the other three land.

**One correction landed with this update.** `10-command-line-interface.status.md` carried "`writ` consumes
the set it registers" as unchecked after [#753](https://github.com/NobleFactor/devlore-cli/issues/753) and
[#754](https://github.com/NobleFactor/devlore-cli/issues/754) had both closed in PR #747 -- the document
reported landed work as outstanding. Corrected there, and recorded rather than quietly ticked.

**The process this thread is worked under** is
[noblefactor-ops `development-process.md`](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/development-process.md):
one open worktree at a time, every issue in it resolved before a pull request, issues logged on discovery
with their resolution site decided at that moment, and every commit updating every document it touches.

## Goals

- [x] `--store` is part of the shared convention, not per-command improvisation.
- [x] `--output` / `-o` selects the rendering, as in `aws`, `az`, and `kubectl` -- never a destination.
- [x] `--output none` turns the result off, reachable from config and env where a shell is not.
- [ ] `devlore-test`, `lore`, `star`, and `writ` each register the full common set on their root, so
      every command of all four accepts every flag.
- [ ] No command invents an output flag of its own.
- [ ] Results go to stdout or a file; narration goes to stderr. Enforced, not merely stated.
- [x] A boolean `--json` does not exist anywhere.

## Current State

Measured, not estimated -- **as of 2026-08-28, before any phase landed.** This section is the baseline the
plan set out to change and is kept in that tense deliberately: the adoption figures are what justify Phase 5,
and rewriting them as work lands would erase the evidence. For what has since changed, see Implementation
Phases.

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
| `writ reconcile` | `--json` (**bool**) | a boolean, not a format |
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

`--json` is replaced by `--output json`. A boolean cannot express a third format, which is why
`writ reconcile` and `writ verify` cannot render yaml today.

## Implementation Phases

### Phase 1: Extend the convention -- COMPLETE

- [x] Add `Store string` to `SinkOptions` and `--store` to `AddOutputFlags`.
- [x] `AddOutputFlags` registers on `PersistentFlags()`, so a root call covers every subcommand.
- [x] Rename `--format` to `--output` with the `-o` short form, via `StringVarP`.
- [x] Add the `none` rendering to `pkg/result`.
- [x] Resolve a store root, defaulting to the state root `GraphsDir`/`TracesDir` use today.
- [x] `GraphsDir()` and `TracesDir()` resolve under the chosen root, together.
- [x] Tests that a relocated store keeps its layout, checksum keying, and run index.
- [x] `--store` accepts a relative path. Unplanned, and found by running the binary rather than the tests:
      every test passed `t.TempDir()`, which is absolute, while `--store ./s` failed. [OpenTree] requires an
      absolute path deliberately -- a drive-relative path on Windows anchors to whichever drive the process
      is standing on -- so [SetStoreRoot] absolutizes at the seam.

### Phase 2: Bring devlore-test into agreement -- COMPLETE

- [x] Replace `--output stream=dest` and `--receipt-format` with `--output` and `--store`.
- [x] Fix what each artifact contains -- #738: the graph document is currently written to the stream named
      `receipt`, and the stream named `graph` emits `t.run` results.
- [x] Persist an execution trace, or state its absence as a decision (#738).
- [x] Assert artifact CONTENT in `cli_test.go`; existence-only assertions are why #738 survived.

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
| `cmd/writ/writ/status/report.go` (`reconcile/` under #762) | 22 `fmt.Print*` | the reconcile report |
| `cmd/writ/writ/verify/verify.go` | 2 `fmt.Print*` | now removed with `presentReport` |
| `deploy`, `decommission`, `upgrade`, `secret` | 4 `SerializeGraphs(os.Stdout, ...)` | the dry-run plan dump |
| `cmd/writ/writ/migrate/session.go` | 1 `os.Stdout` | TUI session output |
| `cmd/writ/writ/migrate_cmd.go` | 1 `os.Stdout` | `FormatMigrationPlan` |

`migrate_cmd.go:157`'s `os.Stdout.Stat()` is a TTY check, which §9 permits, and is not among them.

### Phase 3b: Bring writ's renderings into agreement -- UNBLOCKED

The status report is a human table, so this phase waited on the shared `TableFormatter`. That landed with
Phase 4's first task, and Phase 3d settled how a value of any shape renders. This phase is now the next work.

One question it raises and does not answer: `Report` is an object of three slices and a struct -- shape S4 --
so `-o list` renders four lines with each section as compact JSON, and `-o table` renders one row of four
cells. Legible for `list`, useless for `table`. Whether that is acceptable, or whether the presenters need a
case for a sectioned object, is a design question for after the pipeline is wired. Wiring is not blocked on
it: `-o json`, `-o yaml`, `-o none`, `--jq`, and `--filter` all become correct immediately.

**Measured 2026-08-30, before starting**, when the command was named `writ status`; #762 renames it to
`writ reconcile`, which is what it is called throughout below. `writ reconcile` honors one of eight
formats. `-o json` produces
JSON; `yaml`, `table`, `list`, `csv`, `value`, `none`, and `template=BODY` all produce the same
byte-identical human dashboard. `-o none` prints ten lines where its contract is silence, and `-o yaml`
emits text a parser fails on. The bridge is one bool at `cmd/writ/writ/config.go:160`.

The format value is also never validated (#754), because it never reaches `FormatterByName`:
`writ reconcile -o bogus` prints the dashboard and exits 0, and so does every writ command but `verify`. That is
a second defect, distinct from the wrong renderings -- one does the wrong thing, the other accepts wrong
input.

`--store` is read nowhere in writ at all (#753), which is the severe face of the same cause. `readback.go`
reads `TracesDir()` and `GraphsDir()` to fold runs, so `writ reconcile --store <elsewhere>` reports on the
default store as though it had complied -- the wrong data rather than the wrong shape.

The shared cause is a flag registered on a root that no leaf consumes. `root.go:49` registers the whole set,
so cobra advertises it everywhere; `commands.go:302` consumes it once. Registration without consumption is
worse than absence: an absent flag errors, a present one that does nothing lies.

- [ ] `runStatus` calls `BuildPipeline` and emits `*Report`, as `runVerify` already does. `BuildReport`
      already returns the value, so this deletes `status.Execute`'s branch, `presentJSON`, `presentText`,
      and `JSONOutput` together -- the 22 `fmt.Print` calls go with them.
- [ ] The four dry-run `SerializeGraphs(os.Stdout, ...)` dumps emit the plan as the command's result.
- [ ] `migrate`'s own `--format` retires; its `text` rendering is a domain question like `lore list`'s
      `manifest`, and does not join the shared set.
- [ ] `migrate/session.go`'s stdout write is classified: narration to stderr, or a result to the pipeline.
- [ ] `writ` resolves `--store` through `cli.SetStoreRoot` before any command that touches the store,
      restoring on exit as `devlore-test` does (#753). Every command routed through `readback` is affected,
      not only `status`.
- [ ] Every writ command validates `--output`, which follows from reaching `BuildPipeline` (#754).

### Phase 3c: The format value accepts an argument -- COMPLETE

- [x] `--output` parses `NAME=ARGUMENT`, splitting on the first `=`. A bare name is unchanged.
- [x] `template=<body>` renders through a Go template. It is the only argument-taking format at first.
- [x] `value` renders raw: no quoting, no document syntax, no header. It is what makes `--jq` complete --
      a jq-built string has no other format that prints it as written.
- [x] An unknown `NAME` and a `NAME=` with an empty argument both error, naming the format.
- [x] `tsv` dropped, leaving `csv` and `value` as the delimited pair. It was added on the `awk`/`cut`
      rationale, and that rationale does not survive inspection: those tools have no quote awareness, so
      quoting a field that contains a tab does not stop the row splitting -- it yields `"a` and `b"` instead
      of `a` and `b`. Quoting helps only a caller running a real parser, and that caller is better served by
      `csv` and its universal library support. gcloud and aws each ship one raw delimited format (`value`,
      `text`) and no `tsv`, and the naming is why: `tsv` promises round-tripping that an unquoted format
      cannot keep, so neither tool claimed the name. `DelimitedFormatter` keeps all three attributes; `Raw`
      is what separates the two survivors.

### Phase 3d: The formatting rules, and stage 1 -- COMPLETE

The rules were written first and judged the code, rather than the code being read and described. Three of
the four findings below are defects the rules exposed on their first contact with a real result.

- [x] `docs/architecture/10-command-line-interface.md` §8 states the two stages, the eight shapes a
      presenter must answer for (S1-S8), the per-shape matrix, and the divergences from PowerShell --
      measured against pwsh 7.5.4 rather than recalled.
- [x] A non-scalar cell renders as **compact JSON** at any depth. It went through `fmt.Sprint` before, so a
      nested map rendered as Go's own `map[runs:3]` -- a notation naming the language rather than the data,
      which nothing downstream can parse.
- [x] A type that renders itself is excluded: an `error`, a `fmt.Stringer`, an `encoding.TextMarshaler`. A
      `time.Time` is a struct, and JSON-encoding it would replace the form it defines for itself.
- [x] **Stage 1 is real.** `normalize` moved from inside the jq filter to the head of `Pipeline.Emit`, so
      every presentation is a presentation of the JSON. Before this, `-o list` named a field `UnitCount`
      and `--jq . -o list` named the same field `unit_count` -- the same result, two vocabularies,
      depending on an unrelated flag. The json names are what the Starlark surface shows a customer, and
      they are now what every rendering shows.
- [x] Normalization keeps `json.Number`. Decoding to float64 rounds any integer past 2^53, which is the
      defect #712 records; gojq's conversion stays inside the jq filter, where it is gojq's requirement.
- [x] A single record is one row (S3). `table` refused an object outright, and `csv` rendered the whole
      record into one cell -- two different wrong answers to the shape a command produces whenever it
      reports on one thing.
- [x] `list` added: one field per line, keys aligned within a record, each record keeping its own keys.
      It is the rendering for a result that is wide or heterogeneous, where `table` is sparse and `json`
      is punctuation to see past. gcloud ships `list` and `flattened` separately; one suffices here because
      the compact-JSON cell rule already handles the nesting `flattened` exists to spread out.
- [x] A conformance suite: one fixture, every formatter. It carries `json:` tags that differ from the Go
      names, a nested object, an array, an integer past 2^53, a bool, and an empty string -- so a formatter
      that disagrees with the others about a field's NAME or about what a nested value looks like fails
      there rather than in a command months later.

### Phase 4: Bring star and lore into agreement

- [x] One `TableFormatter` in `pkg/result`, rune-aligned via `text/tabwriter` (#741's alignment half).
      Written as "promote star's `renderTable`"; it is really star's tabwriter approach plus the delimited
      formatter's column inference. star's version carried its own reflection, which would have been a third
      implementation of column selection -- so `cmd/star/cli` stays deletable whole rather than half salvaged.
- [x] Convert `lore`'s `runSearch` (`commands.go:525-556`) first: it is the only real table in the tree, and
      converting it is a bug fix as much as a refactor (see the byte-truncation defect it carries). Landed in
      #779 ([775-lore-adoption.md](775-lore-adoption.md)); #741's byte-count cut went with it.
- [ ] `star` registers the common set, and `cmd/star/cli` is deleted. The copy is gone -- #743 phase 2; it
      was dead, nothing imported it -- and the set lands with the root in #743 phase 3
      ([743-star-adoption.md](743-star-adoption.md)).
- [x] `lore`'s `bundle`, `onboard`, and `list` drop their hand-rolled flags. `list` is a stub returning
      "not yet implemented", so it adapts at no cost; its `--format manifest` is a domain rendering and does
      not join the shared set. Landed in #779: `bundle` and `onboard` take a positional destination.
- [x] The thirteen `fmt.Print` calls are triaged: narration to `cli.*`, results to the sink. Landed in #779.

### Phase 4b: Every in-scope program uses the shared infrastructure

**Ruled 2026-08-31.** Membership in the suite is about infrastructure, not about shipping. A program that
assembles its own root inherits nothing later, and the reach of a fix in `cmd/internal/cli` is exactly the
set of programs that route through it -- measured, not assumed, in §15.

- [ ] `devlore-test` builds its root through `cli.NewRootCmd` rather than constructing a `cobra.Command`
      directly. It has `AddOutputFlags` and therefore #753 and #754, and lacks #755's help wrapping for no
      reason anyone chose: at `COLUMNS=70` its longest flag line is 389 columns where `writ` and `lore` are
      at 70. This holds whether or not `devlore-test` ever ships.
- [ ] `star` uses `cmd/internal/cli` and `cmd/star/cli` is deleted -- the same task as #743, restated here
      because the duplication's cost is now demonstrated rather than argued: a defect fixed in the shared
      package is fixed once per package, and star got neither of this branch's three fixes. The deletion
      landed in #743 phase 2; the root moves in phase 3.
- [x] `lore` registers the common set on its root rather than on `inspect` alone, which is what makes a
      program-wide fix program-wide. Landed in #779.
- [ ] The shared root's commands -- `config`, `man`, `self`, `version` -- are one set on the four programs,
      a program's additions attach beneath, and no usage text follows any error. Ruled 2026-09-02 in
      [743-star-adoption.md](743-star-adoption.md) and recorded in the design (§2, §9, §12, decisions 7-9);
      lands with #743 phase 3.

### Phase 5: Enforce it

- [ ] A test that fails when a command registers an output flag of its own.
- [ ] A test that fails on a direct `os.Stdout` write from a command package.
- [ ] A test that fails when a root registers the common set and a leaf command does not consume it.
      Neither invariant above catches #753 or #754: `writ` registers no flags of its own, and the
      `os.Stdout` check finds `status` but not `repo`, which is equally unvalidated while printing nothing
      of its own. Greppable as "every root calling `AddOutputFlags` has every leaf reaching
      `BuildPipeline`".
- [ ] Regenerate the CLI docs and confirm every command documents the same flags.
- [x] Correct `docs/plans/extract-output-package.md`, which is marked complete while describing an
      `internal/output` package that was never created. Done ahead of the rest of this phase: a completed
      plan describing absent code misleads anyone who reads it for where the code lives, and it holds the
      last measurement showing adoption halving unnoticed.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | A result with no flags goes to stdout as JSON | unit | The default changes |
| 2 | `-o table` renders a table and creates no file named `table` | unit | `-o` is bound to a destination |
| 3 | `--store <dir>` puts definitions and traces under that root | unit | Only one subdirectory moves |
| 3a | A relocated store keeps its run index and checksum keying | unit | The store is treated as a dump dir |
| 4 | A trace in a relocated store still resolves to its definition | unit | `GraphChecksum` ties break |
| 5 | Every format value round-trips through the pipeline | unit | A formatter is unregistered |
| 6 | `-o json` and `-o yaml` both work on `writ reconcile` | unit | `--json` boolean returns |
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
