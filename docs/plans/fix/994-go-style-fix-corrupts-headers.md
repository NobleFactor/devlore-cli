---
title: "star lint go-style --fix corrupts what it rewrites, and the copyright checker cannot see it"
issue: https://github.com/NobleFactor/devlore-cli/issues/994
status: active
created: 2026-09-30
updated: 2026-09-30
---

# Plan: a fixer that does not damage, and a checker that would have refused it

## Summary

`star lint go-style --fix` rewrites the two-line SPDX header into one line on every file it touches, and
writes `TODO(go-style): add summary` where a doc comment belongs. `star lint copyright` -- the linter whose
whole subject is that header -- would catch the first only by accident, and enforces none of the pattern its
own fixer writes. This plan fixes the writer and tightens the reader together, because the regression test
that proves the first needs the second.

It is the first of the pull requests that make `star lint all` trustworthy. Ruled 2026-09-30: *"address all
`star lint all` issues; do as many PRs as we need to close those issues; address all style issues, all of
them, every single one."* The 3,859 go-style violations are not touched here. A sweep driven through a fixer
that corrupts 848 license headers and silences 2,057 violations with placeholders would be worse than the
debt it clears.

Tracing why nothing caught the corruption led somewhere larger, ruled the same day: `star lint copyright` is
a **builtin**, one of the 17 extensions embedded in the star binary, and `build_expected_header` compiles our
copyright into it. A customer running it gets our sentence with their holder substituted in. *"Our copyrights
don't apply to customers. Our patterns are our patterns and should be defined as such."* So the header --
one copyright, two lines long, SPDX included, which we recommend and do not require -- becomes one Go
template read from `lint.copyright`, rendered by the engine star already ships. Ours is declared in
`star/config.yaml` with the entity and a year range: `Copyright 2025-{{.Year}} Noble Factor LLC`.

That in turn simplifies the checker rather than complicating it. With the whole header configured there is
nothing to pattern-match against: `check_file` renders the template and compares, so `SPDX_PATTERN` and
`COPYRIGHT_PATTERN` -- the two regexes whose looseness is #997 -- are deleted rather than tightened, and the
checker requires exactly what the fixer produces because both call one renderer. That is what closes the gap
the corruption slipped through.

## Goals

1. **`--fix` never damages a file it rewrites.** What it did not come to change survives byte for byte.
2. **`--fix` never satisfies a check with a placeholder.** A function it cannot summarize keeps its
3. **The whole header is a configured template, not code.** `star lint copyright` is a builtin that ships
   with star and runs on other people's repositories. It is one copyright, two lines long, and all of it is
   theirs to declare -- SPDX included, which we recommend and do not require. Ours is declared in
   `star/config.yaml`.
4. **`check_file` requires exactly what `fix_file` produces**, because both render the same template. One
   specification, two directions, and `SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted rather than
   tightened.
5. **One walk, in Go.** The cost is discovery: 39 recursive tree walks, 34 of which find nothing. 4.1 ms
   per file against go-style's 0.57 for strictly more work. Under one second, measured.
6. **The lint provider gets the design document it never had**, `3.5.17`, which the catalog skips and
   noblefactor-ops#232 already recorded as missing.
7. **A cross-test proves it.** `--fix` runs over a fixture and `star lint copyright` reads the result, so
   the exact corruption in #994 cannot return unnoticed.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go-style --fix` header handling | Broken | Merges the two-line SPDX block into one; reproduced 2026-09-30 on `pkg/sops/detect.go` and `pkg/sops/locate_test.go` |
| `go-style --fix` doc comments | Harmful | Writes `// <Name> TODO(go-style): add summary`, satisfying `comment.present` while documenting nothing |
| `go-style --fix` Parameters/Returns | Absent | Emits neither; that is devlore-cli#938 and is NOT in scope here |
| `copyright` blank-line rule | Absent | Nothing checks the blank line before `package`, and without it the license IS the Go package doc comment |
| `copyright` holder rule | Too loose | `holder not in found_holder` is a substring test; three variants are live and all pass |
| `copyright` spacing rule | Absent | `\s*` in both patterns accepts `//SPDX-License-Identifier:Apache-2.0` |
| `copyright` shebang rule | Inconsistent | `check_file` accepts a shebang with or without a following blank line; `fix_file` always inserts one |
| Builtin hardcodes our pattern | Wrong layer | `build_expected_header` compiles both header lines into an extension that ships with star and runs on customer code; SPDX is imposed rather than recommended |
| Tree damage | **None** | 0 tracked `.go` files lack the blank line. A hole in the gate, not damage to repair |

## Requirements

### Requirement 1: the fixer preserves what it did not come to change

The header corruption is not a formatting preference. `// SPDX-License-Identifier: Apache-2.0 Copyright Noble
Factor. All rights reserved.` declares the license to be the string `Apache-2.0 Copyright Noble Factor. All
rights reserved.`, which no SPDX consumer resolves. 848 files are in scope for the go-style sweep, so one run
rewrites 848 headers into invalid ones.

The fix is not a special case for SPDX. A styler asked to add a doc comment to a declaration leaves every
other byte of the file alone, and the test says so in those terms rather than naming this one header.

### Requirement 2: no placeholder satisfies a check

`TODO(go-style): add summary` turns a visible violation into an invisible one. Over the current debt it would
convert 2,057 reported violations into 2,057 TODO comments and report the tree clean -- and the linter that
would otherwise have asked for them can no longer see them, because `comment.present` is satisfied.

A gate a placeholder can satisfy is not a gate. Where `--fix` cannot write a true summary it leaves the
violation standing. That is not a limitation to apologize for: the 2,057 doc comments are prose, and the
honest report is that a person writes them.

### Requirement 3: the whole header is ours to declare, and star's to default

Ruled 2026-09-30, across four exchanges:

> check_file should be setup to ensure check_file requires exactly what fix_file produces.
>
> the entity matters. since this is a builtin, we must do something here. our copyrights don't apply to
> customers. our patterns are our patterns and should be defined as such.
>
> we would use a year range, so 2025-{year}, not {year}
>
> the entire copyright is configurable. i don't want to insist, though i would recommend SPDX

**`star lint copyright` is a builtin.** `com.noblefactor.star.LintCopyright` is one of the 17
`com.noblefactor.star.*` extensions embedded in the star binary, so it ships with star and runs on other
people's code. `build_expected_header` compiles

```
<comment> SPDX-License-Identifier: <license>
<comment> Copyright <holder>. All rights reserved.
```

into the product. A customer running it gets our sentence with their holder substituted in. Their copyright
is not ours, "All rights reserved." is not a universal convention, and **SPDX itself is a recommendation
rather than a requirement** -- a consumer may not use it at all.

So the header becomes one configured template. It is **one copyright, two lines long** -- `SPDX_PATTERN` and
`COPYRIGHT_PATTERN` are how the Starlark happens to match it line by line, not two separable policies, and
treating them as separable was an error in an earlier draft of this plan. A consumer declares the whole
thing, in as many lines as they use.

**Go template syntax**, because star already ships the engine and Starlark already reaches it --
`pkg/op/provider/template` wraps `text/template` and exposes `RenderText(content, data)`. Inventing a
`{year}` mini-syntax would put a second templating language beside one the codebase already has.

```yaml
lint:
  copyright:
    header: |
      SPDX-License-Identifier: {{.License}}
      Copyright 2025-{{.Year}} Noble Factor LLC
```

Four constraints, because "Go templates" without them is a small programming language in a config file:

1. **A fixed data set: `.Year`, `.Holder`, `.License`, and nothing else.** A pattern that can reach
   arbitrary data is a pattern nobody can reason about.
2. **Validated when the config loads**, naming the file and the parse error -- not failing halfway through
   984 files with a stack trace.
3. **The template is the text; the comment prefix is the linter's.** `COMMENT_STYLES` keeps deriving `//`,
   `#`, `--` from the extension and applying it per line, so a consumer writes one template rather than one
   per language.
4. **The shipped default is the SPDX two-line form, documented as a recommendation.** It is what a
   repository with no opinion gets, and the documentation says plainly that it is a default and that the
   consumer is expected to set their own.

**The year is a range with a fixed start: `2025-{{.Year}}`.** Both sides resolve `.Year` to the current
year at run time, so `check_file` and `fix_file` agree on every file every day -- which is what makes a
placeholder compatible with the ruling above at all. A bare `{{.Year}}` would not be: `fix_file` would write
2026 while every correct header written in 2025 still said 2025, and the checker would have to reject them.

Two consequences, both accepted rather than overlooked:

- **The gate turns red by the calendar, not by a commit.** On 1 January a pull request that was green the
  night before fails with nothing changed. That is the mechanism working, but the message must say so --
  *"the copyright year range is stale; run `make lint-fix`"* -- or it reads as a broken gate. One
  `make lint-fix` rewrites the tree, the engineer reads a diff of one-line changes, and commits it.
- **A file created in 2027 carries `2025-2027`.** That is what a fixed start year means, and it is normal
  for a collective work dated from the repository's beginning.

**The checker stops pattern-matching and starts comparing.** With the whole header configured, there is
nothing to pattern-match against: `check_file` renders the template, applies the comment prefix per line,
and compares it to the file's leading block. **`SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted** -- the
two regexes whose looseness is #997 stop existing rather than being tightened, and `check_file` requires
exactly what `fix_file` produces because both call the same renderer.

That changes the diagnostics, and the change is worth stating: today the checker says `Wrong license:
expected Apache-2.0, found MIT`. Render-and-compare says the header does not match the configured pattern
and shows the difference. The diff is more actionable than the label, and it cannot go stale as the pattern
changes.

### Requirement 4: the fixer reaches what the checker finds

`fix_file` runs only on files `check_file` flags:

```python
check_result = check_file(f, license, holder)
if check_result["skipped"] or check_result["ok"]:
    continue
```

The checker is therefore the gate on the fixer, and every defect it cannot see is permanently unfixable.
Requirement 3 closes that by construction -- a checker that requires exactly what the fixer produces cannot
pass a file the fixer would change -- but the coupling is worth a test of its own, because it is the reason
the four disagreements above were invisible rather than merely wrong.

### Requirement 5: the cross-test

A test runs `go-style --fix` over a fixture and then `star lint copyright` over the result, asserting the
header survives. Neither linter's own tests can express this: go-style does not know what a valid header is,
and copyright never sees go-style's output. The defect lived in the gap between them, which is where the test
goes.

### Requirement 6: one walk, in Go -- the cost is discovery, not the checking

Authorized 2026-09-30: *"you are authorized to completely rewrite the copyright extension for efficiency
based on the spec we're writing."*

Measured the same day over this repository, each figure the second of two consecutive runs so the filesystem
cache is warm, minus a 0.25 s star startup floor:

| Linter | Files | Work per file | Per file | Starlark |
| --- | ---: | --- | ---: | ---: |
| **copyright** | 998 | look at two lines | **4.1 ms** | **379 lines** |
| go-style | 855 | **parse the whole file as a Go AST**, walk every declaration | **0.57 ms** | 53 lines |

Seven times the cost for a fraction of the work. **Cold, copyright takes 12.6 s against 4.3 s warm**, so
roughly 8 s of a first run is filesystem I/O -- which is the clue, because a linter that reads 998 files
should not pay for 8 s of cold I/O.

**The cause is discovery, and it is one loop:**

```python
for ext in COMMENT_STYLES.keys():          # 39 extensions
    pattern = path + "/**/*" + ext
    files = file.find(pattern)             # a full recursive walk, honoring .gitignore
```

**39 recursive walks of the repository, of which 34 find nothing.** Only 5 of the 39 extensions exist here;
the walks for `.lisp`, `.vim`, `.erl`, `.tex`, `.zig`, `.dart`, `.java`, `.rs`, `.cpp`, `.swift`, `.proto`
and 23 others each traverse the whole tree to return an empty list. `lint-go-style.star` does **one**
`file.find("**/*.go")`, which is the entire difference in the table above.

An earlier draft of this requirement blamed the regexes and the whole-file reads. Those are real and
secondary; naming them first was assumption rather than measurement, and the correction is recorded here
because the acceptance criterion below depends on which cause is being removed.

**The secondary costs, in order:**

1. **Whole files read to examine two lines.** `file.read_text` loads every byte, then `content.split("\n")`
   allocates a Starlark string per line. The header lives in a bounded prefix.
2. **Three or more Starlark-to-Go crossings per file** -- `read_text`, two `regex.find_submatch`, plus
   `source_path.rel()` and `is_excluded` during discovery.
3. **The header rendered per file** where it varies only per comment style: 8 distinct prefixes, computed
   once, not 998 times.

**`file.WalkTree` is the primitive, and it is used from Go.** `pkg/op/provider/file/provider.go:934` is
documented as a discovery operation -- *"the walker observes existing filesystem entries; it does not produce
them"* -- and folds a `Reducer` over each entry in one depth-first traversal. It is already reachable from
Starlark and exercised there:
`plan.file.walk_tree(root=root, fn=collector, include_gitignored=True)` in
`cmd/devlore-test/devloretest/data/test_function_call_walk_tree.star`.

| Approach | Tree walks | Starlark-to-Go crossings |
| --- | ---: | ---: |
| Today | **39**, 34 of them fruitless | ~3 per matched file, about 3,000 |
| `walk_tree` from Starlark | **1** | **1 per entry walked** -- every directory and ignored file, not only the 998 matched |
| `WalkTree` inside a Go provider method | **1** | **1 total** |

From Starlark, `walk_tree` trades 39 walks for one walk plus a callback on every entry in the tree: very
likely still a large win, but it makes the cost scale with tree size rather than with matched files. In Go it
is one crossing for the whole sweep, which is `goast`'s shape and the architecture this requirement asks for.
**`walk_tree` from Starlark is recorded as the cheap intermediate** -- one line changed, no new provider --
if the 4 seconds are wanted before the rewrite lands.

**Two things are unmeasured and must be measured before the number below is committed to:** whether the
reducer's per-entry crossing is cheap in absolute terms, and whether `walk_tree`'s `activationRecord`
requirement imposes plan-machinery overhead that `file.find` avoids.

**The target is stated so it can fail.** Under one second over 998 files -- go-style's order of magnitude for
strictly less work. A rewrite landing at 6 seconds has not met this requirement, and the measurement is
recorded in this document rather than asserted.

### Requirement 7: the lint provider gets the design document it never had

`docs/architecture/3.5-provider-catalog.md` runs from `3.5.1-archive-provider.md` to
`3.5.16-ui-provider.md`. **There is no `3.5.17-lint-provider.md`, and no `.status.md` beside it**, though
every other provider has both. noblefactor-ops#232 recorded the gap and named the file; nothing has written
it.

This rewrite is the occasion, and it is not optional: a provider is being created here, and creating one
without the document every sibling has is how the catalog came to skip a number in the first place.

| Document | What changes |
| --- | --- |
| `docs/architecture/3.5.17-lint-provider.md` | **New.** The provider's methods, the commands over them, the configured header template, and render-and-compare as the checking model |
| `docs/architecture/3.5.17-lint-provider.status.md` | **New.** As every sibling has |
| `docs/architecture/3.5-provider-catalog.md` | Gains the `lint` row it lacks; it has a `goast` row already |
| `docs/architecture/9-star-extensions.md` | `CopyrightConfig` gains `header`; LintCopyright is this document's worked example, so its example changes with it |
| `docs/architecture/configuration.md` | `lint.copyright`'s shape |
| `docs/cli/star/lint/copyright.md` | **Generated.** Regenerated by the build, never edited by hand |

## Implementation phases

### Phase 1: the plan

- [x] This document is reviewed and approved -- 2026-09-30
- [x] Committed before any other change on this branch

### Phase 2: the fixer stops damaging files (#994) -- COMPLETE

- [x] A test pins that `--fix` preserves every byte it did not come to change, on a fixture whose header is
      the canonical two lines -- `TestSaveAsPreservesTheHeaderByteForByte`, plus
      `TestSaveAsKeepsVerbatimCommentsVerbatim` over all four verbatim styles
- [x] `--fix` preserves the two-line header; the tests fail before the change and pass after. **Proved on
      the two files that were measured corrupted**, `pkg/sops/detect.go` and `pkg/sops/locate_test.go`
- [x] A test pins that `--fix` emits no `TODO(go-style)` anywhere -- `TestFixWritesNoPlaceholder`
- [x] `--fix` leaves a violation standing rather than writing a placeholder --
      `TestFixLeavesTheViolationReportable` asserts the outcome rather than the absence, so deleting the
      placeholder and emitting an empty doc comment instead would still fail
- [x] The `TODO(go-style)` text is removed from the source: `makeStubParagraph`, `makeHeaderParagraph` and
      `makeStubList` are deleted, not made unreachable
- [x] The violation count is **unchanged in what is detected**, which is what this box was protecting.
      Recorded below, because the criterion as originally written -- "still reports 3,859" -- was not
      satisfiable and would have been wrong to force.

**What was actually wrong, and it was one contract broken in one place.** Four `CommentStyle` constants
already say "Verbatim", and `Cleanup` already honors that by declining to style them. `SaveAs` did not: it
sent every `CommentDecl` through `renderDoc`, which is `go/doc/comment`'s prose printer, and a prose printer
reflows. Two adjacent comment lines are one paragraph; a paragraph that fits the width budget comes back as
one line. The rule was respected on the way in and broken on the way out. `renderCommentDecl` now dispatches
on style, and `renderVerbatim` emits `cd.cg`'s lines as parsed -- `ast.Comment.Text` the field, not
`ast.CommentGroup.Text()` the method, since stripping markers is what the caller is being protected from.

Copyright was the case that was measured, but it was never the only victim: delineators, region markers and
section headers were all being reflowed too. They survived in practice only because each sits alone between
blank lines, so reflow had nothing to merge.

**A third defect surfaced while testing, unmeasured before:** `--fix` turned **1 violation into 2**.
`CheckCompliance` checks for `Parameters:` and `Returns:` only when a doc comment exists, so writing the stub
satisfied the first check and unmasked two more. The placeholder did not merely silence the real violation,
it inflated the count.

**The count: 3,859 to 3,850, and every one of the nine is named.**

| Where | HEAD | Now | Why |
| --- | ---: | ---: | --- |
| `production.go` | 30 | 25 | the three deleted stub producers, five violations between them |
| `production_test.go` | 18 | 15 | two renamed tests gained doc comments; one test was folded into another and deleted |
| `source_file_test.go` | 12 | 11 | one renamed test gained a doc comment |

Measured by running go-style over a detached worktree at HEAD and diffing the reports per violation, not per
count. **Nothing was introduced** -- the "reported now but not at HEAD" set is empty -- and the three new
files contribute zero violations. Decisively: the **old binary and the new binary both report 3,850 on the
new tree**, so detection is unchanged and the delta is source edits alone. `star lint go-style` never runs
the styler, so it could not have been otherwise, and measuring it beat assuming it.

**Found while reconciling those nine, and not fixed here:** the section check is
`strings.Contains(text, "Parameters:")`, so a doc comment that merely MENTIONS the string satisfies it.
`makeHeaderParagraph`'s own comment reads `(e.g., "Parameters:")` and was therefore never reported as missing
that section. Twelve comment lines in the repository mention `Parameters:` or `Returns:` outside a section
header -- an inline `// Returns: the expanded path`, a generic's `// Type Parameters:` -- so the count is a
small undercount. Same class of defect as #997: a substring where a structure is meant. Filed separately
rather than folded in, because fixing it raises the count and this phase must not move it.

### Phase 3: the whole header becomes a configured template (#997)

- [ ] `lint.copyright.header` carries the template, rendered by `pkg/op/provider/template`'s `RenderText`
- [ ] The data set is exactly `.Year`, `.Holder`, `.License`; a template referencing anything else fails
- [ ] The template is validated when the config loads, naming the file and the parse error
- [ ] `COMMENT_STYLES` still supplies the prefix per line; the template carries text, not comment markers
- [ ] The shipped default is the SPDX two-line form, **documented as a recommendation and a default**, with
      the documentation saying the consumer is expected to set their own
- [ ] `star/config.yaml` declares ours: `SPDX-License-Identifier: {{.License}}` and
      `Copyright 2025-{{.Year}} Noble Factor LLC`
- [ ] A test pins that `.Year` is the current year on both sides, so a January rollover makes the tree stale
      rather than making the checker and the fixer disagree
- [ ] The staleness message names `make lint-fix` and says the year range is why, so a gate that reddens
      overnight with no commit does not read as broken

### Phase 4: check and fix become one renderer (#997)

- [ ] `check_file` renders the template, applies the comment prefix, and compares; it pattern-matches
      nothing
- [ ] **`SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted**, not tightened
- [ ] A property test: for any configured pattern, `check_file(fix_file(x))` passes, and any file
      `check_file` accepts is byte-identical to what `fix_file` would write
- [ ] `check_file` requires the blank line before the code; a fixture with the header directly above
      `package` fails
- [ ] `check_file` requires the blank line after a shebang, as `fix_file` writes. **Every shebanged file in
      the repository gains one**, and the count of files changed is recorded here
- [ ] The `+ 5` scan window has a reason or a bound that cannot mis-handle a long leading comment block
- [ ] A test pins the coupling directly: `fix_file` runs only on what `check_file` flags, so a checker
      looser than the fixer makes its own misses unfixable
- [ ] The diagnostic shows the difference between the rendered pattern and the file, replacing
      `Wrong license: expected X, found Y`
- [ ] `star lint copyright` over the repository passes, and the count it reports is recorded here

### Phase 5: one walk, in Go (#997)

- [ ] The design document is written FIRST: `docs/architecture/3.5.17-lint-provider.md` and its
      `.status.md`. The provider is designed on paper before it is built, as every sibling was
- [ ] `docs/architecture/3.5-provider-catalog.md` gains the `lint` row
- [ ] **Measured before designing to a number:** the per-entry cost of a Starlark `walk_tree` reducer, and
      whether `walk_tree`'s `activationRecord` imposes plan-machinery overhead `file.find` avoids. Both are
      unknown today and both change the design
- [ ] The 39 walks become **one**, over `file.WalkTree`, with the extension decided in the reducer
- [ ] The sweep runs in a Go provider; `lint-copyright.star` becomes the command -- read config, call the
      provider, present the result -- in the shape `lint-go-style.star` already has
- [ ] Only the header's bounded prefix is read, not every byte of every file
- [ ] The rendered header is computed once per comment style, not once per file
- [ ] **Under one second over 998 files, warm, minus the startup floor.** Measured and recorded in this
      document. 4.3 s warm and 12.6 s cold today; a rewrite landing at 6 s has not met Requirement 6
- [ ] The cold figure is recorded too, because 8 s of the original 12.6 was cold I/O paid for by the 34
      fruitless walks, and removing them is most of what this phase is for
- [ ] `docs/architecture/9-star-extensions.md` updated: `CopyrightConfig` gains `header`, and LintCopyright
      is that document's worked example, so the example changes with it
- [ ] `docs/architecture/configuration.md` updated for `lint.copyright`'s shape
- [ ] `docs/cli/star/lint/copyright.md` REGENERATED by the build, not hand-edited
- [ ] Behavior is unchanged by this phase: the counts from Phases 3 and 4 hold exactly, so the rewrite is
      proved to be a rewrite and not a change of subject

### Phase 6: the cross-test and the close

- [ ] `--fix` over a fixture, then `star lint copyright` over the result, asserting the header survives
- [ ] That test fails against the pre-#994 fixer, so it is proof rather than decoration
- [ ] `make test-race` and `make lint-all` pass -- what CI runs, not the packages this branch touched
- [ ] This document set to `complete` in the last commit

## Open questions

Both questions this plan opened were ruled on 2026-09-30, before any code was written.

- [x] **Does the shebang gain a blank line, or lose one?** **It gains one.** Ruled: *"check_file should be
      setup to ensure check_file requires exactly what fix_file produces."* `fix_file` writes a blank line
      after a shebang, so the checker requires one, and every shebanged file in the repository gains one.
      The smaller change would have been to make the fixer match the tree; the ruling is the better one,
      because it makes the pair a single specification rather than two conventions that happen to overlap.
- [x] **Does `star/config.yaml`'s own header become canonical?** **The pattern changes, not the file.**
      Ruled: *"the entity matters ... since this is a builtin, we must do something here. our copyrights
      don't apply to customers. our patterns are our patterns and should be defined as such."* The header
      template becomes configuration, the builtin keeps a generic default, and `Copyright 2025-2026 Noble
      Factor LLC` is declared in `star/config.yaml` as the pattern rather than corrected away from it.

- [ ] **Do the other two repositories adopt the same pattern in this change, or later?** noblefactor-ops and
      personal carry `Copyright (c) <year> Noble Factor. All rights reserved.` in their scripts, a fourth
      variant. Nothing here forces them to converge, and each declares its own `lint.copyright`. Converging
      them is a sweep of its own and is **not** in this plan's scope -- but leaving it unstated would let
      three patterns look like one oversight rather than one decision.

## Related documents

| Document | What it is |
| --- | --- |
| [devlore-cli#994](https://github.com/NobleFactor/devlore-cli/issues/994) | The fixer's corruption and its placeholders; lane 15 |
| [devlore-cli#997](https://github.com/NobleFactor/devlore-cli/issues/997) | The checker looser than its own fixer's pattern; lane 14 |
| [devlore-cli#964](https://github.com/NobleFactor/devlore-cli/issues/964) | The sweep this unblocks; 3,859 violations remain |
| [devlore-cli#938](https://github.com/NobleFactor/devlore-cli/issues/938) | The unfinished styler port; `Parameters:`/`Returns:` emission, deliberately out of scope here |
| [noblefactor-ops#232](https://github.com/NobleFactor/noblefactor-ops/issues/232) | The lint tooling schedule and its stated end state |
| [go-style-guidelines.md](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/go-style-guidelines.md) | The rules `--fix` enforces |