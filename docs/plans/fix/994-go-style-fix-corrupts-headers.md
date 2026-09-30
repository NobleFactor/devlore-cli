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

## Goals

1. **`--fix` never damages a file it rewrites.** What it did not come to change survives byte for byte.
2. **`--fix` never satisfies a check with a placeholder.** A function it cannot summarize keeps its
   violation, visibly, rather than gaining a `TODO` that silences the linter.
3. **`star lint copyright` enforces the pattern `build_expected_header` writes**, so check and fix describe
   one header rather than two.
4. **A cross-test proves it.** `--fix` runs over a fixture and `star lint copyright` reads the result, so
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

### Requirement 3: the checker enforces the pattern its fixer writes

`build_expected_header` is the pattern, and it is one function:

```
<comment> SPDX-License-Identifier: <license>
<comment> Copyright <holder>. All rights reserved.
```

`check_file` must accept that and reject what differs:

- **The blank line before the code is required.** It is the one spacing rule that carries meaning: a comment
  block immediately above `package` is the Go package doc comment, so without the blank line `go doc` prints
  the license as the package's documentation.
- **The copyright line is the canonical line**, not any line containing the holder as a substring. The three
  live variants converge, and the files carrying the other two are corrected here -- `star/config.yaml`
  among them, which is how the linter's own configuration came to differ from the rule it configures.
- **`check_file` and `fix_file` agree about the shebang blank line.** One of them changes; a `.ps1` fixture
  pins whichever way it goes. Our PR scripts carry SPDX on line 2 with no blank line, so this decides
  whether they conform.
- **The `min(start_line + 5, ...)` scan window** gets a stated reason, or a bound that cannot mis-handle a
  long leading comment block.

### Requirement 4: the cross-test

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

### Phase 3: the checker enforces the pattern (#997)

- [ ] `check_file` requires the blank line; a fixture with the header directly above `package` fails
- [ ] `check_file` requires the canonical copyright line rather than a substring
- [ ] The files carrying a non-canonical header are corrected, `star/config.yaml` included
- [ ] `check_file` and `fix_file` agree about the shebang blank line, pinned by a `.ps1` fixture
- [ ] The `+ 5` scan window has a reason or a bound
- [ ] `star lint copyright` over the repository passes, and the count it reports is recorded here

### Phase 4: the cross-test and the close

- [ ] `--fix` over a fixture, then `star lint copyright` over the result, asserting the header survives
- [ ] That test fails against the pre-#994 fixer, so it is proof rather than decoration
- [ ] `make test-race` and `make lint-all` pass -- what CI runs, not the packages this branch touched
- [ ] This document set to `complete` in the last commit

## Open questions

- [ ] **Does the shebang gain a blank line, or lose one?** `check_file` accepts both today and `fix_file`
      writes one. Our `.ps1` scripts have no blank line between the shebang and SPDX, so making the fixer
      match the tree is the smaller change -- but the tree was never checked, so it is not evidence of
      intent. Needs a ruling.
- [ ] **Does `star/config.yaml`'s own header become canonical?** It reads `# Copyright 2025-2026 Noble Factor
      LLC`, naming a legal entity the canonical line does not. If the entity matters, the pattern changes
      rather than the file.

## Related documents

| Document | What it is |
| --- | --- |
| [devlore-cli#994](https://github.com/NobleFactor/devlore-cli/issues/994) | The fixer's corruption and its placeholders; lane 15 |
| [devlore-cli#997](https://github.com/NobleFactor/devlore-cli/issues/997) | The checker looser than its own fixer's pattern; lane 14 |
| [devlore-cli#964](https://github.com/NobleFactor/devlore-cli/issues/964) | The sweep this unblocks; 3,859 violations remain |
| [devlore-cli#938](https://github.com/NobleFactor/devlore-cli/issues/938) | The unfinished styler port; `Parameters:`/`Returns:` emission, deliberately out of scope here |
| [noblefactor-ops#232](https://github.com/NobleFactor/noblefactor-ops/issues/232) | The lint tooling schedule and its stated end state |
| [go-style-guidelines.md](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/go-style-guidelines.md) | The rules `--fix` enforces |