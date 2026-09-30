---
title: "star lint go-style --fix corrupts what it rewrites, and the copyright checker cannot see it"
issue: https://github.com/NobleFactor/devlore-cli/issues/994
status: draft
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
   violation, visibly, rather than gaining a `TODO` that silences the linter.
3. **The whole header is a configured template, not code.** `star lint copyright` is a builtin that ships
   with star and runs on other people's repositories. Both lines are theirs to declare -- SPDX included,
   which we recommend and do not require -- and ours are declared in `star/config.yaml`.
4. **`check_file` requires exactly what `fix_file` produces**, because both render the same template. One
   specification, two directions, and `SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted rather than
   tightened.
5. **A cross-test proves it.** `--fix` runs over a fixture and `star lint copyright` reads the result, so
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

## Implementation phases

### Phase 1: the plan

- [ ] This document is reviewed and approved
- [ ] Committed before any other change on this branch

### Phase 2: the fixer stops damaging files (#994)

- [ ] A test pins that `--fix` preserves every byte it did not come to change, on a fixture whose header is
      the canonical two lines
- [ ] `--fix` preserves the two-line header; the test above fails before the change and passes after
- [ ] A test pins that `--fix` emits no `TODO(go-style)` anywhere, on a fixture whose functions it cannot
      summarize
- [ ] `--fix` leaves a violation standing rather than writing a placeholder
- [ ] The `TODO(go-style)` text is removed from the source, not merely made unreachable
- [ ] `star lint go-style` over the repository still reports **3,859** -- this phase clears no violations and
      must not appear to

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

### Phase 5: the cross-test and the close

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