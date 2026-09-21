---
title: "Lane 15: a command's Long wraps to the terminal, and keeps its shape"
issue: https://github.com/NobleFactor/devlore-cli/issues/759
status: complete
created: 2026-09-21
updated: 2026-09-21
---

# Plan: Lane 15 of the command line schedule

## Summary

Lane 15 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887). #755 made flag usage wrap to the
terminal; a command's `Long` still prints verbatim through cobra's help template, hand-wrapped to whatever width its
author guessed, so `COLUMNS=60 writ status --help` runs `Long` to 83 columns while every flag line wraps at 60. This
plan reflows `Long` the way #755 reflows usage, under one convention that every `Long` in the tree already follows:
an unindented line is prose and reflows; an indented line is structure and keeps its break.

**This plan covers this one lane and nothing else.** Anything found while working it stops the work and goes to the
owner for placement.

## Issue 759

Task, feature [#834](https://github.com/NobleFactor/devlore-cli/issues/834). #755 left `Long` out deliberately:
its prose has intentional shape -- `writ verify`'s policy ladder, `writ deploy`'s conflict table, `writ reconcile`'s
entry states -- and a word-wrapper that folded those into paragraphs would destroy what makes them readable. The
issue asks three things: which text is reflowable, whether authored line breaks are load-bearing, and where the
wrapper hooks.

## Goals

- [x] `COLUMNS=60 writ status --help` produces no line over 60, `Long` included; the same for every command of every
      program, at any width `displayWidth` reports.
- [x] `writ verify --help` keeps its policy ladder indented, one entry per line; `writ deploy --help` keeps its
      conflict table; `writ reconcile --help` keeps its entry-state table; every indented block in every `Long`
      keeps its line breaks.
- [x] Man pages and the generated reference are unchanged: both read `cmd.Long` directly.

## Current State

Read 2026-09-21 at `0e78c7f7`.

| Component | Status | Notes |
| --- | --- | --- |
| `wrapHelp` (`cmd/internal/cli/root.go:220`) | ✅ | #755's seam: `cobra.AddTemplateFunc("wrappedFlagUsages", …)` and `SetUsageTemplate` on the shared root |
| `wrapUsageLine`, `usageTextColumn` (`root.go`) | ✅ | breaks one over-long line, hanging continuations under its text column; a two-space run marks a name column |
| cobra's `defaultHelpTemplate` | ❌ | `{{with (or .Long .Short)}}{{. \| trimTrailingWhitespaces}}` -- `Long` verbatim; nothing in the suite sets a help template |
| the 41 `Long` texts (`cmd/`, non-test) | — | all hand-wrapped at ~80 columns in paragraphs separated by blank lines; every shaped block -- ladders, tables, dash lists, examples -- is indented by two or more spaces; no unindented line is structure |
| `lore new`, `lore test`, `lore search` | ⚠️ | end in unindented one-sentence lines (`Use --ai …`, `Use --from …`) that the convention reflows into a paragraph -- the one visible change, by design |
| `cobra/doc` man generation, `devlore-docs` | ✅ | read `cmd.Long`, not the help template; untouched |

## Requirements

### Requirement 1: the convention

A `Long` is a sequence of blocks separated by blank lines. Within a block, a line whose first character is not a
space is **prose**: consecutive prose lines are one paragraph, joined on single spaces and wrapped to the width. A
line that begins with whitespace is **structure**: it keeps its line break and its indent; when it is itself longer
than the width, it wraps as `wrapUsageLine` wraps a usage line -- hanging under its text column, or under its indent
when the text column leaves less than `helpMinimumTextWidth`. Blank lines are kept as written. Trailing whitespace
never survives.

Authored line breaks in prose are therefore not load-bearing, which is the point: an author who wants a line kept
indents it. That is what every `Long` in the tree already does with the lines it wants kept.

### Requirement 2: the seam

`wrapHelp` grows a second template function, `wrappedLong`, registered beside `wrappedFlagUsages`, and the shared
root's help template becomes cobra's default with `{{. | wrappedLong | trimTrailingWhitespaces}}` in place of
`{{. | trimTrailingWhitespaces}}`. `SetHelpTemplate` on the root reaches every command through
`HelpTemplate()`'s parent walk. The width is `displayWidth()`, as for usage: `COLUMNS` uncapped, else the terminal
capped at 120, else 100.

### Requirement 3: the words

`wrapHelp`'s doc comment says it wraps `Long` too and states the convention in one sentence, so an author writing a
new `Long` knows what an indent means. §12 of `docs/architecture/10-command-line-interface.md`, Help and generated
documentation, gains the rule for both: usage per #755, `Long` per this lane (the specification had recorded #755
only as a conformance measurement in §15).

## Design

```
  cmd.Long
    |
    |  split on blank lines into blocks; within a block, group consecutive lines by kind
    |    prose     (no leading space)  ... join with single spaces, word-wrap to width
    |    structure (leading space)     ... each line kept; wrapped by wrapUsageLine only if too long
    |  blank lines kept; every line right-trimmed
    v
  the help template prints the result where it printed .Long
```

## Implementation Phases

### Phase 1: The plan

- [x] This document, reviewed and chartered 2026-09-21, as written.

### Phase 2: The wrapper

- [x] `wrapLong(long string, width int) string` beside `wrapUsage` in `root.go`, and the prose word-wrap it needs;
      `wrappedLong` registered in `wrapHelp`; the root's help template set. 2026-09-21; `make vet` clean.
- [x] `wrapHelp`'s doc comment states the convention.

### Phase 3: Tests

- [x] `wrapLong` unit tests: a paragraph reflows at 60 and at 100; two paragraphs stay two; an indented ladder keeps
      every break; an over-long indented line hangs under its text column; blank lines survive; width zero returns
      the input; a `Long` with a multi-byte character wraps where it looks like it should. Six tests in
      `help_wrap_test.go`, 2026-09-21; the first's "fewer breaks" claim was wrong (two paragraphs at 100 reflow to the
      authored six) and now asserts the first line packed past the author's 82 columns.
- [x] The measured case as a test: `writ status`'s `Long` at width 60 has no line over 60 -- the first test's text is
      #759's own measurement.

### Phase 4: The words

- [x] §12 of the specification, 2026-09-21: the help-wrapping rule, usage and `Long` together.

### Phase 5: Gates and the three numbers

- [x] `make check` and `make test-scenario`, both exit 0, 2026-09-21.
- [x] Coverage per package and total, complexity, code size. 2026-09-21: total 62.8%, `cmd/internal/cli` 68.9%;
      `wrapLong` and `wrapProse` 100%, `wrapHelp` 87.5%, `wrapUsageLine` 95.8%; no function in `root.go` over 8;
      `root.go` 424 lines, `help_wrap_test.go` 209.

### Phase 6: Installed and exercised

- [x] `make install`, 2026-09-21, `0e78c7f7-dirty`. `status` was retired since #759 was written (#782); its successor
      `reconcile` carries the same `Long`, and at `COLUMNS=60` no line of it is over 60. `writ workflow verify` keeps
      the ladder one entry per line, `writ deploy` its conflict table, `writ reconcile` its entry-state table;
      `lore search --help` shows the two `Use --x` sentences reflowed into one paragraph. Two lines of
      `writ reconcile --help` are still over 60 and neither is `Long`: an `Example:` line (cobra prints
      `{{.Example}}` verbatim, a third template slot) and a flag-usage line whose unbreakable token is `--config`'s
      default path. Both are findings for the owner, outside this lane.
- [x] `danoble-ud24-1.local`, snapshotted first, 2026-09-21, `0e78c7f7-dirty`
      (`build/e2e/2026-09-21-linux-danoble-ud24-1-lane15.log`): self install 0; base and team `unset` twice, `set` by
      URL, by URL again and by path; personal by path; bare `deploy` 64; under `stop` refused 9 occupied targets;
      under `--conflict=replace` **166 files (165 links, 1 template)**, exit 0; `upgrade` 0. `--help` at 60: no line
      of `Long` over 60 in `reconcile` or `deploy`; the two lines over are the `Example:` line and `--config`'s
      default token, neither `Long`; the policy ladder one entry per line.
- [x] `danoble-wd11-3.local`, snapshotted first, 2026-09-21 (`…-windows-danoble-wd11-3-lane15.log`): self install 0;
      every `repo` step as on Linux, git-named clones; `--help` at 60 correct (Git Bash's `awk` counts bytes, so two
      58-rune lines holding `—` and `→` are reported over 60; they are not). Deploy refused: `layers have uncommitted
      changes: [personal]` -- the machine's `Workspace\Personal` checkout is dirty, that checkout's state, not this
      lane's; rerun under `--allow-dirty` (`…-lane15-deploy.log`): under `stop` refused one occupant, `.minttyrc`;
      under `--conflict=replace` **74 links, 1 skipped**, exit 0; `upgrade` 0.
- [ ] The owner's row: `writ deploy` or `writ upgrade` before the pull request opens.

### Phase 7: Closure

- [x] #759's three verification lines ticked with evidence 2026-09-21; the pull request closes it; this plan `complete`.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | a paragraph reflows to 60 and to 100 | unit, `cli` | prose is still printed verbatim |
| 2 | two paragraphs separated by a blank line stay two | unit, `cli` | blank lines are eaten |
| 3 | an indented ladder keeps every break and its indent | unit, `cli` | structure is folded into prose |
| 4 | an over-long indented line hangs under its text column | unit, `cli` | a table row overflows or loses its column |
| 5 | width zero returns the input untouched | unit, `cli` | a missing width breaks help |
| 6 | a multi-byte character wraps by runes | unit, `cli` | byte counting splits a word |
| 7 | `writ status`'s `Long` at 60: no line over 60 | unit, `cli` | the measured case regresses |
| 8 | `COLUMNS=60 <program> <command> --help` on the installed binaries | installed | the template hook is not reached |
| 9 | the VM sequence, with `--help` at 60 on each machine | end to end | a platform differs |

## Files to Create/Modify

| File | Action |
| --- | --- |
| `docs/plans/feature/759-long-wraps.md` | Create |
| `cmd/internal/cli/root.go` | Modify: `wrapLong`, `wrappedLong`, the help template, `wrapHelp`'s comment |
| `cmd/internal/cli/help_wrap_test.go` | Modify: the cases |
| `docs/architecture/10-command-line-interface.md` | Modify: §12, one paragraph |

## Open questions

1. **The three `lore` commands whose trailing `Use --x …` lines reflow.** The convention says indent what you want
   kept; leaving them as they are means they become a paragraph. Stated here for the owner; the plan leaves the
   texts alone.

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lane 15
- [#759](https://github.com/NobleFactor/devlore-cli/issues/759) -- the task
- [#755](https://github.com/NobleFactor/devlore-cli/issues/755) -- flag usage wraps; the seam and the wrapper this lane extends
- [#834](https://github.com/NobleFactor/devlore-cli/issues/834) -- the feature
