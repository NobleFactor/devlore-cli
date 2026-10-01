---
title: "star lint go-style --fix corrupts what it rewrites, and the copyright checker cannot see it"
issue: https://github.com/NobleFactor/devlore-cli/issues/994
status: active
created: 2026-09-30
updated: 2026-10-01
---

# Plan: a fixer that does not damage, and a header that is configuration

## Summary

`star lint go-style --fix` rewrote the two-line SPDX header into one line on every file it touched and wrote
`TODO(go-style): add summary` where a doc comment belonged. Both are fixed, in Phase 2, which is complete.
`star lint copyright` -- the linter whose whole subject is that header -- would have caught the first only by
accident, and enforces none of the pattern its own fixer writes.

This is the first of the pull requests that make `star lint all` trustworthy. Ruled 2026-09-30: *"address all
`star lint all` issues; do as many PRs as we need to close those issues; address all style issues, all of
them, every single one."*

**The design settled over 2026-09-30 and 2026-10-01, and it is far smaller than the first draft of this
document proposed.** The header is a literal string in configuration with no template fields at all; the
language table moves into configuration; and the whole thing stays in Starlark. The earlier draft argued for a
Go provider method on performance grounds that measurement destroyed, and for a clock capability to resolve a
year placeholder the final design does not carry. Both are struck, and the record of why is kept here because
the reasoning was wrong in an instructive way.

## Goals

1. **`--fix` never damages a file it rewrites.** What it did not come to change survives byte for byte.
   **Complete.**
2. **`--fix` never satisfies a check with a placeholder.** A function it cannot summarize keeps its
   violation, visibly. **Complete.**
3. **The header is configuration, not code.** `star lint copyright` is a builtin that ships with star and runs
   on other people's repositories, so our copyright does not belong compiled into it. It is a literal string
   a reader can read, with no template fields.
4. **`check` requires exactly what `fix` produces**, because both use one string. `SPDX_PATTERN` and
   `COPYRIGHT_PATTERN` are deleted rather than tightened.
5. **The language table is configuration too**, with defaults shipped in the manifest, so a consumer can add a
   file type without patching a builtin -- and so `.yaml` being absent stops being unfixable from outside.
6. **One walk, and it stays in Starlark.** The cost was 39 recursive traversals of which 34 found nothing, not
   the checking. 4.3s warm became 1.1s by deleting dict entries; there is no performance case for Go here.
7. **The linter claims copyright only on files we own.** Stamping a header is a legal assertion and `--fix`
   makes it automatic; a language in the table that should not be there writes our copyright onto code we do
   not own. The exclusions, not the table, are what prevent that.
8. **A cross-test proves it.** `--fix` runs over a fixture and `star lint copyright` reads the result, so the
   exact corruption in #994 cannot return unnoticed.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go-style --fix` header handling | **Fixed** | Phase 2. `renderCommentDecl` dispatches on style; verbatim comments are emitted as parsed |
| `go-style --fix` placeholders | **Fixed** | Phase 2. The three stub producers are deleted |
| `go-style --fix` Parameters/Returns | Absent | devlore-cli#938, and NOT in scope here |
| `copyright` header source | Hardcoded | `build_expected_header` compiles our copyright into a builtin |
| `copyright` checker | Too loose | Two regexes, a substring holder test, no spacing rule |
| `copyright` language table | Hardcoded, and wrong | 39 extensions, **5 exist here**, `.yaml` absent -- which is why 22 of our own MIT files are invisible (#999) |
| `copyright` `patterns` config field | Dead | Declared, defaulted, never read (#1000) |
| `copyright` discovery | 39 walks | 34 find nothing. **4.3 s warm; 1.1 s with four** |
| Tree damage | **None** | 0 tracked `.go` files lack the blank line before `package` |

## Requirements

### Requirement 1: the fixer preserves what it did not come to change

**Complete, Phase 2.** A styler asked to add a doc comment to a declaration leaves every other byte alone.
The test says so in those terms rather than naming the SPDX header, so the three other verbatim comment
styles are covered by the same assertion.

### Requirement 2: no placeholder satisfies a check

**Complete, Phase 2.** `TODO(go-style): add summary` turned a visible violation into an invisible one. Where
`--fix` cannot write a true summary it leaves the violation standing. A summary is prose; it cannot be derived
from a name.

### Requirement 3: the header is a literal string in configuration

Ruled across five exchanges on 2026-09-30 and 2026-10-01. `star lint copyright` is one of the 17 extensions
embedded in the star binary, so `build_expected_header` compiled Noble Factor's copyright into a product that
runs on other people's code.

> Our copyrights don't apply to customers. Our patterns are our patterns and should be defined as such.
>
> The headers are configuration, not code. I should be reading config.

So `lint.copyright.header` carries the whole header -- one copyright, two lines long -- as a literal:

```yaml
lint:
  copyright:
    enabled: true
    header: |
      SPDX-License-Identifier: Apache-2.0
      Copyright (c) 2025 Noble Factor. All rights reserved.
```

**No template fields.** Ruled 2026-10-01 after weighing what each would buy:

| Field | Why not |
| --- | --- |
| `{{.Year}}` | It needs a clock. No provider exposes one, `renderFuncs` holds only `Env`, and a clock forfeits `ClaimDeterministic` because `time` is on the capability list. The year is fixed at 2025, the year of initial publication, and never changes |
| `{{.Holder}}` | It substitutes one config string into another config string, saving nobody any typing while adding a field, a doc entry and a `<no value>` hazard |
| `{{.License}}` | The only one deriving from outside the config, via `license: auto` reading `LICENSE`. Dropped with the rest: the identifier is typed into the header where a reader can see it |

Consequently `template.render_text`, `LICENSE_PATTERNS`, `detect_license`, `resolve_license` and the `license`
and `holder` config keys are all deleted. The template layer is gone, not deferred.

**The year is 2025 and stays 2025.** Measured 2026-10-01 against practice: Google freezes the year of creation
(`golang/go` still says 2009), JetBrains carries a range and bumps it (`kotlin` says `2010-2026` while
`intellij-community` says `2000-2025`, the same week, in one organization), and Microsoft omits the year
entirely. REUSE calls years optional and offers four forms; the Linux Foundation discourages ranges because
"copyright notices are rarely kept up to date as a file evolves, resulting in inaccurate statements." A single
frozen year is Google's model and REUSE's first option, and it costs nothing to maintain.

**The copyright line stays conventional rather than `SPDX-FileCopyrightText`.** The kernel's own rules mention
that tag once -- "if desired" -- and no kernel file uses it; `lib/string.c`, `kernel/sched/core.c` and
`scripts/checkpatch.pl` all carry a conventional notice. Google, JetBrains and Microsoft do the same. The SPDX
*identifier* is the part with ISO standing (ISO/IEC 5962:2021) and the part the kernel mandates, and it stays.

### Requirement 4: check and fix are one string

`check` compares the file's leading lines to the configured header with the comment prefix applied. It
pattern-matches nothing, so **`SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted** -- the two regexes whose
looseness is #997.

> check_file should be setup to ensure check_file requires exactly what fix_file produces.

With a literal header this is structural rather than aspirational: both sides use the same string. The
acceptance is still stated as a property -- any file `check` accepts is byte-identical to what `fix` would
write -- because `fix` runs only on what `check` flags, so a checker looser than its fixer makes its own
misses permanently unfixable.

The diagnostic changes with it: `Wrong license: expected Apache-2.0, found MIT` becomes a difference against
the configured header. Less specific, more actionable, and it cannot go stale as the header changes.

### Requirement 5: the language table is configuration, with defaults in the manifest

The table decides which files are checked and how they are commented, and it was hardcoded in a builtin. 39
extensions, of which **five exist in this repository**; absent were `.yaml`, `.yml`, `.ps1`, `Makefile` and
`Dockerfile`. That absence is why 22 of our own `extension.yaml` files declare MIT inside an Apache-2.0
repository and nothing could ever have told us (#999).

Modeled on how others split the problem, measured 2026-10-01:

| Tool | Identification | Comment syntax |
| --- | --- | --- |
| VS Code | `extensions`, `filenames`, `filenamePatterns`, `firstLine` | a separate `language-configuration.json` with `lineComment` and `blockComment` |
| license-maven-plugin | extension-to-style map | a `HeaderType` enum carrying `firstLine`, `beforeEachLine`, `endLine`, `skipLinePattern`, two detection patterns |
| addlicense | a switch on extension | the same switch |

VS Code's shape is the one adopted -- language-keyed, so everything about a file type sits together -- plus two
fields from license-maven-plugin that VS Code has no need for, because VS Code only ever *inserts* a comment
and never finds and replaces an existing header:

- **`skip`** -- the line the header must follow rather than precede. `^#!` for scripts; `^#\s*syntax=` for a
  Dockerfile, whose parser directive stops being a directive if anything precedes it. The kernel's rules state
  the same requirement in prose: the identifier goes on "the first line that can contain a comment," line two
  in a script with a shebang, and `scripts/checkpatch.pl` is a live example.
- **`detect`** -- matches a line of a header already present, so `fix` knows what to replace rather than
  assuming every leading comment line is part of it.

```yaml
# extension.yaml defaults -- shipped, so a consumer gets these free
languages:
  go:
    extensions: [".go"]
    comments: {line_comment: "//"}
  shell:
    extensions: [".sh", ".bash", ".zsh"]
    first_line: "^#!.*\\b(sh|bash|zsh|dash|ksh)\\b"
    skip: "^#!"
    comments: {line_comment: "#"}
  dockerfile:
    filenames: ["Dockerfile", "Containerfile"]
    filename_patterns: ["Dockerfile.*", "*.dockerfile"]
    skip: "^#\\s*syntax="
    comments: {line_comment: "#"}
```

**`first_line` is consulted only for a path with no extension**, and only after `filenames`,
`filename_patterns` and `extensions` have all failed. It reads the file, so that precedence rule is what keeps
it to a handful of reads rather than the whole tree. It is also not optional: **14 of Personal's shell scripts
have no extension** -- `Build-DarwinInitializationPackage`, `Sync-CommonBuildTools`,
`Install-UnixUserConfiguration` -- and `Install-*` is used for both zsh and PowerShell there, so no name
pattern separates them. Line 1 does.

`languages` replaces the dead `patterns` field, which **closes #1000 by replacement rather than deletion**: the
want was real, and its shape -- one template per language, `{license}` placeholders, a `match` regex beside the
`replace` -- was wrong on all three counts.

### Requirement 6: one walk

Discovery, not checking, was the cost. Measured 2026-09-30, warm, minus a 0.25 s startup floor: copyright took
**4.1 ms per file** against go-style's **0.57 ms**, while go-style parses an entire Go AST and copyright looks
at two lines. The cause was one loop:

```python
for ext in COMMENT_STYLES.keys():          # 39 extensions
    files = file.find(path + "/**/*" + ext)
```

**39 recursive walks, 34 finding nothing.** Deleting 35 dict entries took the warm run from **4.3 s to 1.1 s**
over the same 1001 files -- which is where an earlier draft of this requirement went wrong. It attributed the
cost to regexes and whole-file reads, then argued for moving the sweep into a Go provider method. The
experiment that settled it took ninety seconds and showed the fix is a dict. **There is no performance case
for Go here**, and the rule the codebase actually follows is narrower than "work goes in Go": `lint.starlark`
is in Go because it resolves calls against `op.ReceiverRegistry()`, which Starlark cannot see. Comparing two
lines of text is not that.

So the sweep stays in Starlark, walking once and deciding per entry, and the ~0.85 s that remains covers four
walks and all 1001 files.

### Requirement 7: the cross-test

A test runs `go-style --fix` over a fixture and then `star lint copyright` over the result, asserting the
header survives. Neither linter's own tests can express this: go-style does not know what a valid header is,
and copyright never sees go-style's output. The defect lived in the gap between them, which is where the test
goes.

### Requirement 8: the linter claims copyright only on files we own

Stamping a header is a legal assertion, and `--fix` makes it automatic. That reverses the risk calculation this
plan carried until 2026-10-01, when it argued for a generous default language table on the grounds that a
wrong comment marker "fails loudly and locally":

| | Failure |
| --- | --- |
| **A language absent from the table** | the file goes unchecked -- #999. A real gap, visible the moment anyone looks, and recoverable |
| **A language present that should not be** | `Copyright (c) 2025 Noble Factor. All rights reserved.` written onto **code we do not own** |

A wrong marker is cosmetic and local. A wrong inclusion is a false ownership claim, and nothing in the
pipeline would question it.

**There is a live example in the repository.** The only `.vim` file in any of the three repositories is
`gruvbox.vim`, a third-party colorscheme, in a writ migrate fixture. The only `.lua` files are `init.lua`
fixtures beside it. **With the exclusions applied there is no `.lua` or `.vim` file in scope at all** -- so the
earlier justification for putting them in the default table ("we have two Lua files") was counting excluded
fixtures, measured with `git ls-files` and no exclusion filter. Same error as the 39-extension table, one layer
up.

**So the protection is not the language table. It is the exclusions**, and they carry more weight than their
three lines suggest:

```yaml
exclude:
  - "**/testdata"
  - "**/vendor"
  - "wiki/**"
```

Verified 2026-10-01: of 1,083 tracked files whose type the table covers, **1,001 sit outside `testdata` and the
linter checks exactly 1,001.** The 82 it skips include the 40 MIT `.star` files of the `docker-package`
regression corpus and `gruvbox.vim`. The exclusion is doing the right job, and the reason is not "it is
testdata" but **"it is not ours"** -- which is also why #721's corpus must stay excluded whatever is decided
about MIT elsewhere.

**One thing is verified-working and not understood.** `matches_pattern`'s `**/` branch tests
`path.endswith("/" + suffix)`, which matches the directory `.../testdata` rather than a file inside it, and the
whole-segment branch searches for the literal `/**/testdata/`, which cannot occur. By inspection neither branch
should match `cmd/.../testdata/x.star` -- yet the count proves the exclusion works. **The reading is wrong
somewhere and Phase 5 must not touch that function until it is understood**, because the exclusions are what
stand between `--fix` and a false ownership claim.

**Two in-scope types raise the same question and are not decided here:**

- **`.otf`, 42 files.** Fonts, almost certainly licensed from a third party. Binary, so they cannot carry a
  header -- but they must not claim one either, and REUSE's `.license` sidecar is the only model that reaches
  them.
- **`.1` and `.man`, 69 files.** If they are generated, the generator owns their headers and editing them is
  editing generated output.

And the ruling owed on #999 is narrower than it first looked: **the 22 MIT files are all outside `testdata`**,
so they are files we own and the header is wrong, rather than fixtures where MIT is correct.

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

### Phase 3: the header becomes configuration (#997, #1000)

- [ ] `lint.copyright.header` carries the whole header as a literal; the compiled-in `build_expected_header`
      is deleted
- [ ] `star/config.yaml` declares ours:
      `SPDX-License-Identifier: Apache-2.0` / `Copyright (c) 2025 Noble Factor. All rights reserved.`
- [ ] **No template fields.** `template.render_text`, `LICENSE_PATTERNS`, `detect_license`, `resolve_license`
      and the `license` and `holder` config keys are deleted, not deferred
- [ ] `patterns` is removed from `CopyrightConfig`; `languages` replaces it, which closes #1000
- [ ] `docs/` records that the header is the consumer's to set and that the shipped default is a default

### Phase 4: check and fix become one string (#997)

- [ ] **`SPDX_PATTERN` and `COPYRIGHT_PATTERN` are deleted**, not tightened
- [ ] `check` compares the file's leading lines to the configured header with the comment prefix applied
- [ ] A property test: any file `check` accepts is byte-identical to what `fix` would write
- [ ] `check` requires the blank line before the code; a fixture with the header directly above `package`
      fails. In Go that comment block IS the package doc comment, so this rule changes meaning, not appearance
- [ ] `check` requires the `skip` line to precede the header where a language declares one, pinned by a `.ps1`
      fixture and a `Dockerfile` fixture
- [ ] A test pins the coupling: `fix` runs only on what `check` flags, so a looser checker makes its own
      misses unfixable
- [ ] `star lint copyright` over the repository reports a count, and that count is recorded here

### Phase 5: the language table becomes configuration (#999)

- [ ] `lint.copyright.languages` carries the table; defaults ship in `extension.yaml`
- [ ] Each language declares identification by `filenames`, `filename_patterns`, `extensions` and
      `first_line`, resolved in that order
- [ ] `first_line` is consulted **only** for a path with no extension and only after the other three fail; a
      test pins that an extensionless `#!/usr/bin/env bash` script is claimed and that a `.go` file never
      causes a read for identification
- [ ] `skip` and `detect` are per language, with `^#\s*syntax=` for Dockerfile pinned by a fixture -- a header
      above a parser directive silently changes how the file builds
- [ ] **Merge or replace is decided and tested.** A project setting `languages` must extend the shipped
      defaults rather than supersede them, or a consumer adding one language silently loses every built-in
      style and finds out when the linter passes a file it never opened
- [ ] The 39-entry table is gone; the one that replaces it covers what these repositories actually contain
- [ ] `star lint copyright` over the repository is measured warm and cold, and both are recorded. 4.3 s warm
      and 12.6 s cold today

### Phase 6: the cross-test and the close

- [ ] `--fix` over a fixture, then `star lint copyright` over the result, asserting the header survives
- [ ] That test fails against the pre-#994 fixer, so it is proof rather than decoration
- [ ] `make test-race` and `make lint-all` pass -- what CI runs, not the packages this branch touched
- [ ] This document set to `complete` in the last commit

## Open questions

- [x] **Does the shebang gain a blank line, or lose one?** Superseded. `skip` is per language and the header
      follows the skipped line, which is the kernel's own rule.
- [x] **Does the header carry a year?** **One year, 2025, frozen** -- ruled 2026-10-01. Google's model and
      REUSE's first option. No clock, no provider, no annual sweep.
- [x] **`SPDX-FileCopyrightText` or a conventional notice?** **Conventional** -- ruled 2026-10-01. The kernel
      mentions the tag once as "if desired" and does not use it; neither do Google, JetBrains or Microsoft.
- [x] **`Noble Factor` or `Noble Factor LLC`?** **`Noble Factor`** -- ruled 2026-10-01. 17 U.S.C. §401 permits
      "an abbreviation by which the name can be recognized."
- [x] **What template variables are offered?** **None** -- ruled 2026-10-01.

- [ ] **Which license do the 22 MIT files carry?** **This blocks Phase 5's completion**, and it is a licensing
      ruling rather than a lint judgment ([#999](https://github.com/NobleFactor/devlore-cli/issues/999)). With
      no `license: auto`, the header says `Apache-2.0` literally, so every one of those files fails the moment
      `.yaml` enters the table. Either they are wrong and are corrected, or the licensing is deliberately mixed
      and the configuration says so with a reason. **No header was rewritten pending the answer.** Two
      oddities found alongside them: eight files already use `SPDX-FileCopyrightText`, and one attributes
      copyright to **David Noble** rather than the company.
- [ ] **Allow list or deny list?** VS Code's `copyrightFilter` starts at `'**'` and subtracts, so a file type
      nobody considered is **checked by default and fails** until someone excludes it with a reason. The
      `languages` table is an allow list, whose failure mode is silence -- which is exactly how 22 MIT files
      went unseen. Default-include is the right polarity for "no one gets a pass," and adopting it means the
      table stops being scope and becomes only *how* to comment a file already in scope. Not in this plan's
      scope; raised because the choice is load-bearing.
- [ ] **Do `.md`, `.1`, `.man` and `.conf` carry headers?** 595 Markdown files, 84 roff, 26 conf across the
      three repositories. VS Code excludes `**/*.md` explicitly. REUSE would cover all of them, and binary
      files too, through a `.license` sidecar -- the only model that reaches the 43 `.otf` fonts.
- [ ] **Do the other two repositories adopt this header?** noblefactor-ops and personal carry four more
      variants between them, and Personal is already on the chosen form for 100 files. Converging them is a
      sweep of its own and is not in this plan's scope.

## Related documents

| Document | What it is |
| --- | --- |
| [devlore-cli#994](https://github.com/NobleFactor/devlore-cli/issues/994) | The fixer's corruption and its placeholders; Phase 2 closed it |
| [devlore-cli#997](https://github.com/NobleFactor/devlore-cli/issues/997) | The checker looser than its own fixer's pattern |
| [devlore-cli#998](https://github.com/NobleFactor/devlore-cli/issues/998) | The section checks that are substring tests; found reconciling Phase 2's nine |
| [devlore-cli#999](https://github.com/NobleFactor/devlore-cli/issues/999) | The 22 MIT files the linter cannot see; blocks Phase 5 |
| [devlore-cli#1000](https://github.com/NobleFactor/devlore-cli/issues/1000) | The dead `patterns` field that `languages` replaces |
| [devlore-cli#964](https://github.com/NobleFactor/devlore-cli/issues/964) | The go-style sweep this unblocks; 3,850 violations remain |
| [devlore-cli#938](https://github.com/NobleFactor/devlore-cli/issues/938) | The unfinished styler port; `Parameters:`/`Returns:` emission, out of scope here |
| [noblefactor-ops#232](https://github.com/NobleFactor/noblefactor-ops/issues/232) | The lint tooling schedule and its stated end state |
| [9-star-extensions.md](../../architecture/9-star-extensions.md) | The extension model, with `CopyrightConfig` as its worked example |
| [go-style-guidelines.md](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/go-style-guidelines.md) | Mandates a header no `.go` file in this repository carries |
