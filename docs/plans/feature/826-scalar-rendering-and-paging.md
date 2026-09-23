---
title: "Lanes 5 to 7: a document rendering, a terminal one, the grouping, and a pager"
issue: https://github.com/NobleFactor/devlore-cli/issues/826
status: complete
created: 2026-09-14
updated: 2026-09-18
---

# Plan: Lanes 5 to 7 of the command line schedule

## Summary

Lanes 5, 6 and 7 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), the schedule that closes
the command-line epic, in one pull request that closes feature
[#833](https://github.com/NobleFactor/devlore-cli/issues/833). The renderings produce data in eight shapes
and a document in none, so a report reaches the terminal as markdown source; the set is nine names in one
alphabetical run that answers what exists and never which one you want; and every rendering writes straight
through, so a long one scrolls off the screen.

**This plan covers these three lanes and nothing else.** Anything found while working them stops the work and
goes to the owner for placement.

## Issue 826

Lane 5. `star gh issues report --markdown` returns a markdown report as one string. `--output value` prints
it verbatim — proved offline on `star docs starlark --output value` — but verbatim means the markup comes
too: `##`, `|---|`, and the rest. Nothing renders a document for reading. Rewritten 2026-09-14 after the
original premise (that `value`'s scalar path is an accident) was measured false: §8's shape table contracts
it at row S1.

## Issue 895

Lane 6. Nine names in one alphabetical list. Grouping them answers the question the list cannot — which one
do I want — and the grouping is already load-bearing: lane 7 must know which renderings a person reads.
Ruled 2026-09-14, with two corrections it forces. `yaml` is serialization, not a human rendering: all three
serialized renderings round-trip, and that yaml is pleasanter to read than json is a property of yaml, not a
different job. And the four remaining human renderings are two pairs, each split by one axis, so nothing in
the set is redundant.

## Issue 830

Lane 7. Every program writes stdout straight through, so `star docs starlark`, the command tree
([#341](https://github.com/NobleFactor/devlore-cli/issues/341)) and a 65-to-775-line report scroll away.
`git` and `gh` both page, under a rule that is settled: only on a TTY, only when the output is long, the
environment's pager honored, and a switch to refuse. #830's own list of which renderings page is superseded
by lane 6's grouping — see Requirement 4.

## Goals

- [x] Lane 5: `--output markdown` produces a markdown document; `--output terminal` renders that document for
      a terminal: bold, italic, color, word wrap, box-drawn tables.
- [x] Lane 6: every rendering declares its group, the groups are the headings in the man pages and in
      `--output`'s help, and `yaml` sits with `csv` and `json`.
- [x] Lane 7: a long human rendering pages the way `git diff` does, and a piped one is byte-identical to
      today's.
- [x] Feature [#833](https://github.com/NobleFactor/devlore-cli/issues/833) closes with three ledger
      questions answered in this document: rows 12, 13 and 14.

## Current State

Measured 2026-09-14 in the worktree `/Users/david-noble/Workspace/NobleFactor/devlore-cli.826-scalar-rendering-and-paging`,
against develop at the merge of pull request #890.

| Component | Lane | Status | Notes |
| --- | --- | --- | --- |
| `result.Pipeline.Emit` | 5 | ✅ | `normalize` → filter → format; every formatter presents JSON |
| `result.MarkdownFormatter` | 5 | ✅ | landed 2026-09-14; the shape table of Requirement 1, tests green |
| `result.TerminalFormatter` | 5 | ✅ | landed 2026-09-15; glamour under our style, `NO_COLOR` honored, tests green |
| `result.DelimitedFormatter` | 5 | ✅ | `value` emits a scalar verbatim, as §8 row S1 says it must |
| `docs/plans/result-text-formatter.md` | 5 | ❌ | charters `text` as tabwriter columns — which shipped as `table` |
| `github.com/charmbracelet/glamour` | 5 | ✅ | already a direct dependency, used at `cmd/internal/console/model.go:50` |
| `result.Formatter` | 6 | ❌ | no group; the interface is `Format` alone |
| `outputUsage` in `cmd/internal/cli/output.go` | 6 | ❌ | one alphabetical run; `yaml` sold as "reading by eye" |
| the generated man pages | 6 | ❌ | carry the flat list |
| `sink.Sink.IsTTY` | 7 | ✅ | the TTY query exists at `pkg/sink/sink.go:49` |
| the pager | 7 | ❌ | nothing pages; no `--no-pager`, no environment variable, no config key |
| `pkg/result/pipeline.go:5-13` | — | ❌ | the package doc says three formatters ship; ten will |

## Requirements

### Requirement 1: `markdown` renders a document (lane 5)

`MarkdownFormatter` derives a markdown document from the normalized JSON, sharing the key derivation
`table`, `csv` and `value` use: the union of keys across records, `json:` tag names, a non-scalar cell as
compact JSON.

| Shape | `markdown` |
| --- | --- |
| S1 scalar string | verbatim — a report is already the document |
| S1 other scalar | the value alone |
| S2 array of scalars | a bullet list, one element per item |
| S3 flat object | a GFM table: header row, one row |
| S4 nested object | as S3; nested values as compact JSON |
| S5 array of flat objects | a GFM table: header row, one row per element |
| S6 array of objects, differing keys | as S5; absent keys render empty |
| S7 array of arrays | a bullet list, each inner array as compact JSON |
| S8 empty array, or null | nothing, exit 0 |

The rule behind the table: **a GFM table where the derivation yields headers, a bullet list where it does
not.** GFM has no headerless table, and inventing column names would name fields the data does not have.

### Requirement 2: `terminal` renders the document for a terminal (lane 5)

`TerminalFormatter` holds a `MarkdownFormatter`, renders into a buffer, and hands that markdown to glamour:
goldmark parses it, glamour's renderer walks the tree, and a style sheet decides what each element becomes. No
change to the `Formatter` interface for this — composition is a field.

Named by the owner 2026-09-14. In the prior art `plain` means unstyled — pandoc's `--to plain`, run on 2.17,
removes the markup and emits no escape codes, and bat's `--style=plain` means no decorations — so it would have
named the opposite of what this rendering does. `ansi` names a national standards body. `terminal` names the
destination, the way groff's `-Tutf8` names its device, and so warns that escape codes come with it.

**The style is ours.** No bundled sheet fits: `notty` and `ascii` are byte-identical and keep every marker
(`# `, `**`, `*`, backticks), and `dark` keeps `##` below h1 and paints h1 as a colored banner. The style starts
from `NoTTYStyleConfig` and consumes the markers, re-emitting each as an attribute:

| Element | Renders as |
| --- | --- |
| h1 | bold, italic, underlined; no `#` |
| h2 to h6 | bold; no `#` |
| strong | bold; no `**` |
| emph | italic; no `*` |
| strikethrough | crossed out; no `~~` |
| code span | colored; no backticks |
| code block | syntax-highlighted by chroma |
| block quote | indented under `│ `, italic |
| bullet | `- ` |
| table | box-drawn by `lipgloss/table`; links in a cell collected as a footnote list under the table |
| link | its text, then the URL |

**Nothing is probed.** glamour's color profile defaults to TrueColor rather than asking the terminal, and the
style is fixed rather than glamour's `auto`, which reads the terminal's background. So `--output terminal` emits
the same bytes piped, redirected, or on a TTY, and §7's rule holds without exception. A caller who wants no
escape codes asks for `markdown`.

**The width comes from the environment, and the padding goes.** Ruled 2026-09-18. One resolver answers for
help text and for this rendering, promoted out of `cmd/internal/cli/root.go` where `helpWidth` already lives:
`COLUMNS` when set and sane, honored exactly and uncapped, since a user who exports it has said what they
want; else the terminal width, capped at 120, because prose at 211 columns is unreadable and glow caps for
the same reason; else 100. `pkg/result` cannot import `cmd/internal/cli`, so the width is injected at
`BuildPipeline` (`cmd/internal/cli/output.go:267`), which is where the formatter is constructed.

glamour pads every line out to the wrap width through `reflow/padding` (`ansi/margin.go:32`) and offers no
way to stop it, so the formatter trims the trailing padding itself: a reader who copies a report out of the
terminal should not collect trailing spaces, and the padding is most of the escape-code bulk.

**`NO_COLOR` is honored.** When it is set and not empty, the style's colors are dropped and its attributes
kept: no-color.org's rule suppresses color, not bold, italic or underline. The research, and why the suite
honors a convention no standards body stands behind, is recorded in §10 of the specification.

### Requirement 3: Every rendering declares its group (lane 6)

| Group | Renderings | What it is |
| --- | --- | --- |
| Composed | `template=BODY`, `value` | you chose the shape |
| Document | `markdown`, `terminal` | a document; source, or rendered for a terminal |
| Nothing | `none` | the exit code alone |
| Records | `list`, `table` | records laid out for a person |
| Serialized | `csv`, `json`, `yaml` | lossless; a library reads it back |

**The group is a property of the formatter**, so `result.Formatter` gains a `Group()` method beside `Format`.
A table mapping names to groups would be a second place that knows the set, drifting the first time anyone
adds a name; a method makes a missing group a compile error against the guards each formatter already
carries. `Group` is a string type with five explicit constants, no `iota`.

The group names become headings: in the generated man pages, and in `outputUsage`, the renderings appear
under their group rather than in one alphabetical run — alphabetical within a group. `yaml`'s line stops
claiming it is for reading by eye.

### Requirement 3a: a man page reads like git's (lane 6)

Ruled 2026-09-18: the man pages look like git's man pages. `git-clone(1)` gives each option a tag line and an
indented block, and every continuation stays at that indent:

```
       -l, --local
	   When the repository to clone from is on a local machine, this flag
	   bypasses the normal "Git aware" transport mechanism and clones the
```

**Why the help text cannot simply be reused.** `doc.GenMan` does not write roff. It writes markdown and hands
it to md2man (`cobra/doc/man_docs.go:105-116`), putting the flag's usage on a tab-indented line. roff fills
what it is given, so a usage string carrying headings and columns arrives as one filled paragraph: columns
become tabs and continuations fall back to the paragraph margin. git's pages come from asciidoc, which emits
`.RS 4` per option; md2man has no markdown that produces that sequence cleanly.

**What ships**, in two parts, with cobra's generator untouched:

1. **A usage text per audience.** `pflag.Flag.Usage` is a field, so [withManUsage] installs `outputUsageMan`
   on the flags named in `manUsage`, runs the generator, and restores them. It wraps both `doc.GenMan`
   (`man.go`) and `doc.GenManTree`. The flag objects are the root's persistent flags, shared by every command,
   so one swap covers every page. A terminal sees `outputUsage`, a man page sees `outputUsageMan`: one set of
   renderings, two layouts.
2. **A tidy pass over the generated roff.** `outputUsageMan` writes each rendering as a bold name followed by
   a block quote, which is the only markdown that opens an indented block. md2man surrounds it with paragraph
   breaks -- `quoteTag` is `\n.PP\n.RS\n` -- so the name and its description render three blank lines apart.
   [tidyManRoff] collapses that sequence to `.RS 4` and removes the blank line md2man writes before every
   `.PP`. It runs on the bytes cobra produced, in both paths; in the install path the rewrite goes through the
   `fsroot.Dir` the command owns.

**What was measured** before landing on that, each built and rendered through `mandoc`:

| The markdown | What roff made of it |
| --- | --- |
| The terminal text, two columns | columns collapse to tabs; continuations at the margin |
| The terminal text, two lines per rendering | uniform, but a long description wraps to the margin |
| A definition list (`term` / `: text`) | `.TP`; hanging or two-line depending on the tag's width |
| A bullet list per group | tight, but a bullet and an inline description, not git's shape |
| **A block quote per rendering, tidied** | git's shape: tag line, `.RS 4` block, continuations at the indent |

### Requirement 4: A long human rendering pages (lane 7)

Under every condition below, or not at all:

- **stdout is a TTY.** `sink.Sink.IsTTY` already answers this. Piped or redirected output never pages, so
  `--output json > file` is byte-identical with or without a terminal.
- **The rendering is one a person reads.** **Records and Document page; Serialized, Composed and Nothing do
  not.** This supersedes #830's own list, which pages `yaml` — serialization — and pages `value` and
  `template`, which are the shapes you composed to send somewhere else. Lane 6 makes this a question the
  formatter answers rather than a list the pager keeps.
- **The output is long.** `less -FRX` semantics — quit if it fits on one screen, keep color, do not clear on
  exit. A short result never sees the pager.
- **Nothing refused it.** `--no-pager` on the shared root, and a `pager: false` configuration key.

Two flags on the shared root, both git's, with git's short forms: `--no-pager` never pages, and `--paginate`
pages whenever stdout is a TTY, including a short result and a Serialized or Composed rendering. No flag takes
a pager command; git has none either. Ruled 2026-09-14.

**Given both, `--no-pager` wins, and the pair is not refused.** Measured 2026-09-18: `git -p -P log` and `git
-P -p log` both exit 0. An earlier draft marked them mutually exclusive, and that refusal was the one usage
error in the suite cobra words in a way `isUsageError` does not match -- it exited 1 where §9 says 64. The
refusal is gone; `shouldPage` resolves the pair.

**Which pager, and whether to page, are two questions.** Ruled 2026-09-18: git's approach entire.

*Which* is `$DEVLORE_PAGER`, then the `pager` configuration key, then `$PAGER`, then `less -FRX` -- git's
`GIT_PAGER`, `core.pager`, `PAGER`, default, with `$DEVLORE_PAGER` playing `GIT_PAGER` because the root is
shared by four programs and #830's `$STAR_PAGER` would have had `writ deploy` consulting star's variable. An
empty value anywhere disables paging, as `GIT_PAGER=` does.

*Whether* is `--no-pager`, `--paginate`, and the group rule; none of them touches `pager`, so refusing a pager
today leaves the configured one in place for tomorrow.

`pager` holds a command rather than a boolean, which is why the key is positively named while the flag is
`--no-pager`: they answer different questions. A boolean disable in configuration is absent here as in git.

### Requirement 5: Three ledger questions are answered (rows 12, 13, 14)

- **Row 12 — the `lore list` default-rendering exception**, open in three plans. Answered by §7: json is the
  default everywhere, and a domain rendering is an app-specific flag outside the common set, never a value
  added to the shared list. `lore list` takes no exception. The three plans are corrected in this pull
  request's documentation commit.
- **Row 13 — [#774](https://github.com/NobleFactor/devlore-cli/issues/774)'s sectioned-object question.**
  Answered by §8 row S4: one row, nested values as compact JSON, reshaped with `--jq` when a caller wants
  them spread. No sectioned rendering is added.
- **Row 14 — [#741](https://github.com/NobleFactor/devlore-cli/issues/741) closed with the
  `installed`-decoration box unticked.** §7 rules that `installed` stays a field that `--output json` emits,
  not a `*` folded into a name column. The box is retracted on #741 with that note.

## Design

```
  Go value        JSON                     presentation                 destination
     |              |                           |                            |
     v              v                           v                            v
  +--------+   +---------+  filter     +------------------+        +------------------+
  | struct |-->| object  |--> --jq --->|     --output     |------->| TTY? long? and   |
  | slice  |   | array   |             |                  |        | Records/Document?|
  | map    |   | scalar  |             | Serialized       |        |   yes: pager     |
  | scalar |   | null    |             |   csv json yaml  |        |   no:  stdout    |
  +--------+   +---------+             | Records          |        +------------------+
             normalize, once           |   list table     |
                                       | Document         |          the pager asks the
                                       |   markdown ──┐   |          formatter its group;
                                       |   terminal <─┘   |          it keeps no list
                                       | Composed         |
                                       |   template value |
                                       | Nothing          |
                                       |   none           |
                                       +------------------+
```

`markdown` is a presentation of the JSON, like `table`. `terminal` is a presentation of the *markdown*, and
the only formatter that composes another. The pager sits after the formatter and knows nothing about which one
ran, beyond the group the formatter names.

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered. Chartered 2026-09-14; `$DEVLORE_PAGER` ruled the same day, and
      lane 6 inserted the same day on the owner's placement ruling.

### Phase 2: Lane 5, `markdown`

- [x] `pkg/result/markdown.go`: `MarkdownFormatter`, the shape table of Requirement 1, sharing the existing
      key derivation rather than copying it.
- [x] `result.FormatterByName` gains `"markdown"`; the `--output` help text gains its line.
- [x] Golden tests for every row of the shape table. Green 2026-09-14 on the full suite.

### Phase 3: Lane 5, `terminal`

- [x] `pkg/result/terminal.go`: `TerminalFormatter`, holding a `MarkdownFormatter`, rendering through glamour
      with the style of Requirement 2.
- [x] `result.FormatterByName` gains `"terminal"`; the `--output` help text gains its line.
- [x] `NO_COLOR` drops the style's colors and keeps its attributes.
- [x] The width resolver moves out of `root.go` and answers for both help and `terminal`; `BuildPipeline`
      injects it. `cmd/internal/cli/width.go`, 2026-09-18.
- [x] The trailing padding glamour adds is trimmed, and a style the trim would have dropped is closed.
- [x] Tests: no marker survives; strong, emphasis and headings carry their attributes; the bytes do not depend
      on whether stdout is a terminal; under `NO_COLOR` the attributes survive and no color does. Green
      2026-09-15 on the full suite, `make test` exit 0.

### Phase 4: Lane 6, the grouping

- [x] `result.Group`, five explicit constants, and `Group()` on the `Formatter` interface; every formatter
      implements it. `pkg/result/group.go`, 2026-09-18. `DelimitedFormatter` answers from its own `Raw` field,
      since one type backs both `csv` and `value`.
- [x] `outputUsage` renders the renderings under their group headings, alphabetical within a group; `yaml`
      moves to Serialized and its line stops claiming it is for reading by eye.
- [x] The generated man pages carry the same headings. Verified 2026-09-18 with `writ man --install` and
      mandoc: all five headings appear in `writ.1`.
- [x] The man pages read like git's (Requirement 3a): `withManUsage` gives the generator `outputUsageMan`
      and restores the flag afterwards, and `tidyManRoff` collapses md2man's paragraph breaks into the
      `.RS 4` block git uses. Both generation paths covered, the install path rewriting through the
      command's own `fsroot.Dir`. Rendered against `git-clone(1)` on 2026-09-18: same shape.
- [x] Tests: every registered name has a group; the groups partition the set with nothing left over; the help
      text carries every group as a heading and every name under its own group. Green 2026-09-18, `make test`
      exit 0.

### Phase 5: Lane 7, the pager

- [x] The pager in `cmd/internal/cli`, since every program carries the same output set and the TTY test
      belongs in one place. `pager.go`, 2026-09-18: `shouldPage`, `pagerCommand`, `pagerSession`.
- [x] `--paginate` and `--no-pager` on the shared root, with git's short forms `-p` and `-P`, which no command
      in this repository binds, and marked mutually exclusive; the `$DEVLORE_PAGER`, then `$PAGER`, lookup.
      **The two switches are not common-set members** -- the set is §4's four pipeline flags and §14 polices
      it; these take no value and change no rendering, so they carry their own usage constants instead.
- [x] The configuration key: `pager`, holding the pager command, read between `$DEVLORE_PAGER` and `$PAGER`.
      Ruled 2026-09-18 after measuring that `BindFlags` makes viper follow the flag rather than the reverse, so
      a bound key changes nothing unless code reads it -- `--verbose` and `--dry-run` are the only two that do.
      `pager: false` was dropped: it would invert the flag's polarity and leave a bound, unread `no-pager` key
      beside it as a trap.
- [x] The paging decision asks the formatter its group; the pager keeps no list of names.
- [x] Tests: piped output is byte-identical with the pager configured; Serialized and Composed never page; a
      short result never pages; `--no-pager` refuses; an empty environment value disables; `--paginate` pages
      a short result and a Serialized rendering on a TTY, and never a pipe. 2026-09-18: `shouldPage`'s
      eleven-case table and `pagerCommand`'s precedence in unit tests, plus the real binary -- `writ repo
      list --output table` piped, with and without `DEVLORE_PAGER=cat` and with `--paginate`, byte-identical
      each time. `emitWriter`'s pager branch needs a pseudo-terminal and is covered by the binary, not the
      unit tests.

### Phase 6: The specification

- [x] §7's format set, §8's diagram and shape table, and §10 gain `markdown`, `terminal`, the grouping, and the
      paging rule. 2026-09-18: §7 carries the group table and the `markdown` to `terminal` chain, §8 shows the
      chain in the diagram and gives `markdown` its own column in the shape table, §10 carries paging, color,
      and why the two switches are not common-set members.
- [x] `pkg/result/pipeline.go`'s package doc names the ten renderings by group instead of three formatters.
- [x] Row 12's three plans corrected; row 14's box retracted on #741 with §7's ruling. 2026-09-18: row 12
      closed in `775-lore-adoption.md` (twice), `740-cli-output-conventions.md`, and
      `10-command-line-interface.status.md`, whose Outstanding-work bullet is gone; row 14 struck through on
      #741 with the reason, since the box was never what that issue fixed.

### Phase 7: Installed and exercised on this machine

- [x] **The three numbers, before the pull request is written** (ruled 2026-09-18): coverage per package and
      total, code size, and the complexity figures themselves -- not that the gate passed. `make check`
      fails only above the ceiling, so a function resting at it passes in silence.

      Measured 2026-09-18 at `make check` exit 0:

      | Number | Value |
      | --- | --- |
      | Coverage, `pkg/result` | 90.6% |
      | Coverage, `cmd/internal/cli` | 67.6% |
      | Coverage, total | 62.6% |
      | Complexity, highest touched | `FormatterByName` 13, down from 16 before the spec parse came out; `MarkdownFormatter.Format` 10; `addOutputFlags` 9 |
      | Complexity, over the ceiling | none in this lane; the tree's `>15` list is pre-existing test functions |
      | Size, new production code | 860 lines: `markdown.go` 246, `terminal.go` 230, `pager.go` 203, `group.go` 110, `width.go` 71 |
      | Size, new tests | 1,014 lines across 7 files, so 1.18 lines of test per line of code |
      | Size, files changed | 7: `selection.go`, `pipeline.go`, `conformance_test.go`, `spec_test.go`, `output.go`, `man.go`, `root.go` |

      Measured at the final state -- after the Windows backslash fix, the pager's configuration chain, and the
      removal of the mutual exclusion -- rather than at any intermediate one.

      Thin spots, stated rather than hidden: `emitWriter` 45.5%, because its pager branch needs a
      pseudo-terminal and is exercised by the binary instead; `man.go`'s generation paths, which were
      untested before this lane and remain so.
- [x] `make install`, and `writ version` names the build under test: `v0.1.0-dev.20260914030948-dirty`,
      commit `9cbddea3`, on all three machines.
- [x] Every rendering exercised against a real result. `star gh issues report --by schedule` under
      `--output terminal`; `writ repo list` under `markdown` and `terminal` on darwin, linux and windows; the
      same result piped with and without `DEVLORE_PAGER` and with `--paginate`, byte-identical each time; the
      grouped help and the generated man page on darwin and linux. The pull request's test plan lists what ran;
      the transcripts are the session's.
- [x] The three numbers, and the thin spots named rather than hidden -- see the table above.
- [x] **End to end on the virtual machines**, ruled 2026-09-18 and required of every plan from now on. On
      `danoble-ud24-1.local` (linux/arm64) and `danoble-wd11-3.local` (windows/arm64, through Git Bash), in
      this order: snapshot the Parallels virtual machine; bundle the built binaries and copy them; `writ self
      install`; register base, team and personal by URL and by path; `writ deploy`; `writ upgrade`.

      Run 2026-09-18, and **what ran differs by machine**. Both were snapshotted first
      (`DANOBLE-UD24-1` `{31f9799e}`, `DANOBLE-WD11-3` `{5a290151}`), and both are reachable over **IPv6
      only** -- plain `ping` fails on the Windows machine, whose IPv4 address is APIPA.

      | Step | linux/arm64 | windows/arm64 |
      | --- | --- | --- |
      | snapshot | taken | taken |
      | `self install` | clean | clean |
      | the renderings and the grouped help | as on darwin | as on darwin |
      | the man page | git's shape | not generated |
      | `repo remove` × 3 | exit 0 each | exit 0 each |
      | `repo add` by URL × 3 | exit 1 each: the clone destination exists ([#792](https://github.com/NobleFactor/devlore-cli/issues/792)) | exit 1 each: `Could not resolve host: github.com` |
      | `repo add` by path × 3 | exit 0 each; all three registered | impossible: nothing on disk to point at |
      | `deploy`, bare | exit 64 ([#843](https://github.com/NobleFactor/devlore-cli/issues/843)) | not run |
      | `deploy common` | exit 1, refusing over 16 occupied targets under `stop` | not run |
      | `deploy common --conflict=replace` | exit 1 at `file.mkdir-4` ([#822](https://github.com/NobleFactor/devlore-cli/issues/822)) | not run |
      | `upgrade` | exit 1 with no run index, then exit 0 once one existed ([#756](https://github.com/NobleFactor/devlore-cli/issues/756)) | not run |

      **The Windows machine ends unregistered**, and that is mine: its three registrations pointed at empty
      `layers\` directories -- the [#840](https://github.com/NobleFactor/devlore-cli/issues/840) shape, with
      `base` already `broken` -- and my removes took them while every re-registration failed for want of DNS.
      The sequence now sits there as `C:\Users\david-noble\writ-e2e.ps1`, to run when its networking is
      fixed; the snapshot restores the prior state if that is wanted instead.

      **What the runs found, none of it this lane's:**

      - [#822](https://github.com/NobleFactor/devlore-cli/issues/822), `Severity:High`, `Priority:P1`:
        `file.mkdir` ignores `--conflict`, so a dangling symlink at a directory target fails the deploy under
        every policy. Reproduced exactly: `file.mkdir: /home/david-noble/.Personal-secrets/gnupg exists, but
        is not a directory`, with `--conflict=replace` given. It was on no schedule; ruled onto
        [#894](https://github.com/NobleFactor/devlore-cli/issues/894) the same day as lane 11, **before**
        [#831](https://github.com/NobleFactor/devlore-cli/issues/831), since listing a target that
        `file.mkdir` still refuses to replace would name the failure without fixing it.
      - [#831](https://github.com/NobleFactor/devlore-cli/issues/831) evidenced rather than argued: the
        pre-flight listed 16 occupied targets and missed the one that stopped the run.
      - [#756](https://github.com/NobleFactor/devlore-cli/issues/756) confirmed: `upgrade` errored on an
        unwritten store, and exited 0 with "No copied files to upgrade" once a run had written one.
      - [#792](https://github.com/NobleFactor/devlore-cli/issues/792) on every layer: an existing clone in
        writ's own home blocks re-registration by URL.
      - Adjacent to [#883](https://github.com/NobleFactor/devlore-cli/issues/883): the occupant that stopped
        the deploy was a dangling symlink writ did not make, so `occupantIsOurs` is not what governs it.

- [ ] **The owner's row, not mine, and the one step left:** `writ deploy` or `writ upgrade` converges the
      machine before the pull request opens. Handed over as a command, never run here. This plan is otherwise
      complete; the pull request script waits on it.

### Phase 8: Closure

- [x] The three issues carry no acceptance checklists -- #826, #895 and #830 were rewritten during this lane
      and state their requirements as prose -- so there are no boxes to tick. Recorded rather than claimed.
- [x] Lanes 5, 6 and 7 close when #826, #895 and #830 do, which the pull request's `Closes` lines perform;
      `star gh issues report --by schedule` reads their state rather than a mark in the schedule's table.
- [x] Feature #833 closes with them.
- [x] This plan `complete` in the last commit, under the rule of noblefactor-ops#199.

## Test Plan

| # | Lane | What it proves | Level | Fails when |
| --- | --- | --- | --- | --- |
| 1 | 5 | `markdown` answers every row of the shape table | unit, `result` | a shape is unhandled |
| 2 | 5 | A scalar string passes through `markdown` unchanged, newline-terminated once | unit, `result` | a report is mangled |
| 3 | 5 | `markdown` and `table` name columns identically | unit, `result` | the derivation was copied, not shared |
| 4 | 5 | A cell carrying a pipe or a newline never ends its row | unit, `result` | one record renders as two |
| 5 | 5 | `terminal` bytes are the same whether or not stdout is a terminal | unit, `result` | the rendering changes when observed |
| 6 | 5 | `terminal` consumes every marker; headings, strong and emphasis carry attributes | golden, `result` | the markup survives |
| 7 | 5 | Under `NO_COLOR`, `terminal` keeps bold and italic and carries no color | unit, `result` | the variable is ignored, or strips styling too |
| 8 | 5 | The width is `COLUMNS`, else the terminal capped at 120, else 100 | unit, `cli` | the order or the cap is wrong |
| 9 | 5 | No rendered line ends in whitespace | unit, `result` | glamour padding survives |
| 10 | 6 | Every registered name answers `Group()` | unit, `result` | a formatter has no group |
| 11 | 6 | The five groups partition the set, nothing left over or counted twice | unit, `result` | the grouping drifts from the registry |
| 12 | 6 | `outputUsage` carries every group name as a heading | unit, `cli` | the help reverts to a flat list |
| 13 | 7 | Piped output is byte-identical with a pager configured | unit, `cli` | a pipeline changes when observed |
| 14 | 7 | Serialized and Composed never page; Records and Document do | unit, `cli` | the pager keeps its own list |
| 15 | 7 | A result shorter than the screen never pages | unit, `cli` | every result opens `less` |
| 16 | 7 | `--no-pager` and an empty `$DEVLORE_PAGER` both refuse | unit, `cli` | the switch is unenforced |
| 17 | 7 | `--paginate` pages a short result and a Serialized rendering on a TTY, never a pipe | unit, `cli` | the flag is inert, or pages a pipe |
| 18 | 7 | `$DEVLORE_PAGER` wins over `$PAGER`; `$PAGER` wins over `less -FRX` | unit, `cli` | the order is not git's |

## Files to Create/Modify

| File | Lane | Action |
| --- | --- | --- |
| `docs/plans/feature/826-scalar-rendering-and-paging.md` | — | Create |
| `pkg/result/markdown.go`, `markdown_test.go` | 5 | Create — done |
| `pkg/result/terminal.go`, `terminal_test.go` | 5 | Create |
| `pkg/result/group.go`, `group_test.go` | 6 | Create |
| `pkg/result/selection.go`, `selection_test.go` | 5, 6 | Modify: two names; the group of each |
| `pkg/result/pipeline.go` | 5, 6 | Modify: `Group()` on the interface; the package doc names ten formatters |
| `pkg/result/delimited.go`, `json.go`, `list.go`, `none.go`, `table.go`, `template.go`, `yaml.go` | 6 | Modify: each declares its group |
| `pkg/result/conformance_test.go`, `spec_test.go` | 5, 6 | Modify: the new names join the conformance set |
| `cmd/internal/cli/output.go` | 5, 6, 7 | Modify: the help text and its headings; `outputUsageMan`; `--paginate`, `--no-pager`; inject the width |
| `cmd/internal/cli/man.go` | 6 | Modify: `withManUsage` wraps both generators |
| `cmd/internal/cli/width.go`, `width_test.go` | 5, 7 | Create: the resolver `helpWidth` becomes, with the 120 cap |
| `cmd/internal/cli/pager.go`, `pager_test.go` | 7 | Create |
| `cmd/internal/cli/root.go` | 7 | Modify: `--paginate` and `--no-pager` on the shared root |
| `cmd/internal/cli/man.go` | 6 | Modify: the man pages carry the group headings |
| `docs/architecture/10-command-line-interface.md` | 5, 6, 7 | Modify, §7, §8, §10, last commit |
| `docs/plans/result-text-formatter.md` | 5 | **Remove** — superseded; the owner's script, never mine |

## Open questions

1. **Word wrap width.** glamour wraps to the width it is given and defaults to 80. Wrapping to the terminal's
   width would be a probe, which §7 forbids, so the width is fixed; which width is the owner's ruling. The first
   rendering shown uses glamour's 80.
2. **No `--markdown`.** Ruled 2026-09-14: when this work is done no command carries a `--markdown` flag;
   `star gh issues report` returns data and `--output markdown` derives its document, sectioned by the shape of
   the result. Two things that ruling needs are not placed: the sectioned derivation, which reverses
   Requirement 5's answer to ledger row 13, and the change to the report extension, which lives in
   `NobleFactor/noblefactor-ops` and has no issue.

3. **`NO_COLOR` in the narrator and the console.** §10 now says the suite honors `NO_COLOR` everywhere it adds
   color. `--output terminal` is lane 5's. The narrator, `pkg/status/narrator.go:73`, ignores the variable, and
   the console honors it only through lipgloss; neither is in any lane. Not placed.
4. **§10 says color never reaches the result stream.** Its second bullet reads "Color and progress indicators
   only when stderr is a TTY, and never in the result stream." `--output terminal` puts color in the result
   stream by design. The bullet and the ruling cannot both stand; the amendment is the owner's.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lanes 5 to 7
- [#833](https://github.com/NobleFactor/devlore-cli/issues/833) -- the feature these three lanes close
- [#740](https://github.com/NobleFactor/devlore-cli/issues/740) -- the epic, and its closure ledger, rows 12, 13, 14
- [#341](https://github.com/NobleFactor/devlore-cli/issues/341) -- the command tree; a beneficiary of the pager, not this work
- `docs/architecture/10-command-line-interface.md` §7, §8, §10 -- the rulings applied
- `docs/plans/result-text-formatter.md` -- superseded by this plan
