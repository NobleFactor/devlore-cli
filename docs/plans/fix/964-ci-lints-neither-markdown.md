---
title: "CI lints four of eleven; the gate arrives with its sweep"
issue: https://github.com/NobleFactor/devlore-cli/issues/964
status: active
created: 2026-09-28
updated: 2026-09-28
---

# Plan: CI lints four of eleven; the gate arrives with its sweep

## Issue 964

`.github/workflows/ci.yaml` runs `make vet-all`, `star lint go ./...`, `make lint-all` and
`star lint shell .`. It never runs `star lint all`, `markdown`, `go-style` or `copyright`, and it has no
PowerShell step although `install.ps1` ships to users. `make check` runs none of them either.

This is lane 7 of the lint tooling schedule,
[noblefactor-ops#232](https://github.com/NobleFactor/noblefactor-ops/issues/232), added 2026-09-28.

### The count is 51, not 33

The issue says `install.ps1` carries 33 findings. Measured 2026-09-28 with the gate CI would actually
run -- noblefactor-ops's `Test-PowerShell.ps1` at `develop`, not PSScriptAnalyzer alone:

```
powershell: 3 checked, 3 with findings     # 51 findings
```

| Count | Rule | Source |
| ---: | --- | --- |
| 30 | `PSAvoidUsingWriteHost` | analyzer |
| 9 | function has no `[CmdletBinding()]` | **the gate's own check** |
| 4 | function has no `param()` block | **the gate's own check** |
| 3 | missing `#!/usr/bin/env pwsh` | **the gate's own check** |
| 2 | script has no `param()` block | **the gate's own check** |
| 1 each | `PSReviewUnusedParameter`, `PSUseBOMForUnicodeEncodedFile`, `PSUseSingularNouns` | analyzer |

The 33 was PSScriptAnalyzer's half. `Test-PowerShell.ps1` adds four checks of its own -- shebang,
`param()`, `[CmdletBinding()]`, parse -- and those are 18 of the 51. The issue's table wants correcting:
measuring a gate with a different tool than the one that will run it is how a gate turns out to cost
more than it was scoped for.

By file: `install.ps1` 47, and 2 each in two test fixtures.

## Why this is one plan and three pull requests

The deliverable is a repository whose CI runs every linter it has, read-only, and whose sweeps are
clean. Ruled 2026-09-27: *"I want to run all of our lint without `--fix` in CI. The time to run with
`--fix` is during pre-commit and the result ought to be proofed by the engineer before we complete the
commit. It's a guard."*

That is too much for one pull request, and the pieces have different blockers. **The issue is not
split; the phases are the lanes.**

| Phase | Lane of #232 | Ships | Blocked by |
| --- | --- | --- | --- |
| 2 | 7 | The PowerShell gate and `install.ps1` | nothing |
| 3 | 9 | copyright, markdown, spelling, Starlark, `make lint`, the hook | lanes 2, 3 and **8** of #232 |
| 4 | 10 | the go-style sweep | **devlore-cli#938** (lane 5) |

Lane 8 is [devlore-cli#721](https://github.com/NobleFactor/devlore-cli/issues/721), added to #232 on
2026-09-28. Phase 3 waits on it for the reason in Requirement 3.

**Phase 2 ships alone and first**, because it is the only part another session is waiting on.
devlore-cli#949's lanes 11 and 21 both edit `install.ps1` and are held for this work; its lane 13 waits
on lane 11. Nothing else here touches installers, so holding three of their lanes behind copyright and
markdown would be waste.

**Phase 4 cannot precede devlore-cli#938.** #938 records that the goast production registry's `default`
branch silently returns the wrong production, so *"`star lint go-style --fix` does summaries and bodies;
the schema vocabulary for everything else is inert, and declaring it is silently ignored."* Running that
fixer across 843 files before #938 lands would mass-produce wrong answers with no signal. The precedent
is local and recent: `Invoke-ScriptAnalyzer -Fix` prepended UTF-8 byte-order marks to seven files in
personal#216 while "fixing" whitespace, and only `git diff -w` caught it.

## Requirements

### Requirement 1: the gate is the base layer's, run out of a pinned checkout

Exactly as personal#216 did it, and for the same reason: `Test-PowerShell.ps1` and
`PSScriptAnalyzerSettings.psd1` are the organization's, and the settings file says so in its own header.
CI checks noblefactor-ops out at a pinned ref and runs the script from there. No copy lands in this
repository, and the rule set cannot drift between the three.

### Requirement 2: CI runs every linter read-only; `--fix` never appears in it

`make lint` is the entry point and calls `star lint all`. `make lint-fix` exists for the desk and the
commit hook and is never invoked by CI. `make lint-tools` runs first and fails loudly when a tool is
missing -- this repository's own `Makefile` records the *2026-08-04 silent-pass incident*, where a
polluted stream made every lint run appear to succeed.

### Requirement 3: `star lint all` must actually mean all

`star lint` has `copyright`, `go`, `go-style`, `markdown` and `shell`. It has no `powershell`, no
`spelling` and no `starlark`, so `all` is not all. Phase 3 adds them.

**Starlark is not buildifier's to cover, and an earlier draft of this plan had that wrong.** Ruled
2026-09-28: *"buildifier is being replaced... it doesn't do nearly enough."* devlore-cli#721 is the
charter, written out of the docker package rewrite, and its load-bearing observation is that every
defect that shipped there was **syntactically valid Starlark**:
`plan.package.install(...)` where the namespace is `pkg`, `plan.verify(...)` where no provider has a
`Verify` method -- all four `verify.star` files were nothing but those calls -- `plan.file.write(path=)`
where it is `write_text(destination_path=)`, and `plan.user`, `plan.notify` and `plan.download(url,
dest)`, none of which exist. buildifier parses all of it without complaint and always would have. It
knows Bazel's dialect, not devlore's provider surface, so it structurally cannot see a wrong namespace,
an absent method, a wrong arity or a misspelled keyword.

`LintStarlark` reads the **generated** ground truth instead -- `action_names.gen.go` for every valid
`<namespace>.<method>`, each provider's `ParameterNames` for every valid keyword -- so it cannot drift
from codegen. buildifier would need a hand-maintained second copy of that surface, which is the
diverged-second-copy failure this organization already names. Hence lane 8 before Phase 3.

#232's **"Explicitly out"** section does not bar this, and an earlier draft of this plan misread it as
doing so. It excludes two specific tracked defects -- a hole inside star's existing `LintStarlark`
design and wrong-keyword call sites in `HookPreCommit`/`HookPrePush` -- not the capabilities.

## Found while planning this, and resolved in lane 2

**The plan status vocabulary has no gate, and devlore-cli uses 21 different words for it.** Measured
2026-09-28 across `docs/plans/`, counting both the frontmatter `status:` field and the `**Status:**`
line some documents restate it on:

| | Word | Occurrences |
| --- | --- | ---: |
| valid | `complete` | 161 |
| valid | `draft` | 64 |
| valid | `active` | 10 |
| valid | `abandoned` | 1 |
| valid | **`approved`** | **0** |
| invalid | `in-progress` | **34** |
| invalid | `charter` | 13 |
| invalid | `proposed` | 5 |
| invalid | `settled`, `completed`, `not-started` | 2 each |
| invalid | `ready`, `pending`, `in`, `done`, `closed`, `deferred`, `design`, `implemented`, `design-solidified`, `chartered`, `re-chartered` | 1-2 each |

The five words were fixed by
[noblefactor-ops#219](https://github.com/NobleFactor/noblefactor-ops/issues/219), which retired
`in-progress` and `chartered` by name. `in-progress` has since been written 34 more times. `approved` --
the one status that means a human reviewed the document -- appears nowhere in the repository. `status:
in` is a truncation nobody caught.

Nothing checks any of it. devlore-cli's `scripts/Test-GuideFrontmatter.sh` walks `docs/guides` only, and
requires a different schema again (`title`, `description`, `tool`, `category`, `order`).

**This is lane 2's, not this lane's.** [devlore-cli#932](https://github.com/NobleFactor/devlore-cli/issues/932)
already exists to make `star lint frontmatter` reach the documentation standard, carrying a
`FrontmatterConfig` with the families, the vocabularies and the exemptions. The status vocabulary is one
of those vocabularies, so the check belongs there and is committed to there -- ruled 2026-09-28. A
document's status is then wrong in CI rather than wrong forever.

**No sweep.** Ruled the same day: *"we don't [have] time to address this bullshit... we'll deal with
these as we encounter them."* So the existing 70-odd are corrected when a document is touched for some
other reason, not in a pass of their own. #721's was corrected here because this plan cites it.

## Open questions

- [x] **Are test fixtures linted?** **Yes.** Ruled 2026-09-28: *"Everything is linted. EVERYTHING.
      ALWAYS. NO EXCEPTIONS WITHOUT A COMPELLING ARGUMENT."* The argument that a fixture represents
      content we do not control, and stops doing so once it is in house style, was weighed and is not
      compelling enough to carve a hole in the gate. The two files --
      `cmd/writ/testdata/layer-journey/personal-a/.../w.ps1` and
      `cmd/writ/writ/migrate/testdata/fixture/.../Initialize-SshIdentity.ps1` -- are fixed in Phase 2,
      4 findings between them. Discovery in `Test-PowerShell.ps1` is unchanged, so no base-layer change
      is needed and the other two repositories are untouched.
- [x] **May Phase 3 include Starlark and the commit hook?** **The question was mine, and it was wrong.**
      #232's "Explicitly out" section excludes two specific tracked defects -- a hole inside star's
      existing `LintStarlark` design, and wrong-keyword call sites in `HookPreCommit`/`HookPrePush` that
      `shell-lint-via-star.md` tracks. It does not exclude the capabilities. An earlier draft of this
      plan read "these two bugs belong to other work" as "this capability is forbidden" and spent an
      owner's question on it.
- [x] **Markdown is measured, and the answer is that there is no policy to measure against.** An
      earlier draft said this was unmeasurable for want of Node. That was wrong twice over: this box has
      Node v26.7.0 and the Mac has v26.9.0 -- what was missing was the npm package `markdownlint-cli2`,
      installed 2026-09-28. Measured then, over 464 files: **36,857 findings**, of which
      **25,318 are `MD013/line-length`**, 4,877 `MD060/table-column-style` and 4,766
      `MD033/no-inline-html`.

      **Those are markdownlint's defaults, because no configuration exists.** Not tracked in this
      repository, not shipped in the `LintMarkdown` extension, not deployed -- `config.sync()` has no
      markdown config to sync. So 36,857 is not a defect count; it is the cost of adopting a stranger's
      house style for 464 documents.

      This is the one thing personal#216 did not have to solve. PowerShell had
      `PSScriptAnalyzerSettings.psd1` in the base layer, saying in its own header that it is the
      organization's. Markdown has no equivalent. **So Phase 3 writes one, in noblefactor-ops, run out
      of the `.base` checkout exactly as Requirement 1 has the PowerShell gate run** -- and only then is
      there a number worth sweeping to.

- [x] **`star lint markdown` cannot run on Windows**, found 2026-09-28. It passes all 464 discovered
      paths as arguments and exceeds the command-line limit:
      `markdownlint-cli2 failed: The command line is too long.` The measurement above had to be taken by
      invoking `markdownlint-cli2 "**/*.md"` directly. A glob, batching, or a response file fixes it.
      **This goes to lane 2**, ruled 2026-09-28: devlore-cli#932 is the lint provider reaching the
      standard, and a defect in `lint.markdown` is a lint provider repair. Raising it as a question here
      was a misreading of #232's rule against splitting a committed lane -- a lane fixing a defect in the
      component it already owns is that lane doing its job, not scope creep.

## Implementation phases

### Phase 1: the plan lands

- [x] This document is reviewed and approved -- 2026-09-28
- [x] Committed before any other change on this branch

### Phase 2: the PowerShell gate, and `install.ps1` clean (lane 7)

- [ ] `ci.yaml` gains a PowerShell job on `ubuntu-latest`, modeled on noblefactor-ops's, checking the
      base layer out at a pinned ref and installing PSScriptAnalyzer 1.25.0
- [ ] `install.ps1`'s 47 findings resolved; the 30 `Write-Host` calls become
      `Write-Information -InformationAction Continue`, the house form everywhere else
- [ ] `install.ps1` still runs on **Windows PowerShell 5.1**, which is what a fresh Windows machine has
      and what #948 landed it for. **`$PSStyle` is 7.2 and later, so it must not be used here**, unlike
      personal's `Install-Rustup.ps1` where it preserved the color
- [ ] The two test fixtures fixed, 4 findings -- ruled in scope like everything else
- [ ] The gate reports `3 checked, 0 with findings`
- [ ] The other session told, because devlore-cli#950 and #965 are unblocked by this phase and by
      nothing else in this plan

### Phase 3: every remaining linter (lane 8)

- [ ] `star lint powershell`, `star lint spelling`, `star lint starlark`, and `lint.all` running them
- [ ] `make lint`, `make lint-fix`, `make lint-tools`; `make check` calls `make lint`
- [ ] The commit hook runs `--fix`, then **refuses the commit** when it rewrote anything, so the diff is
      read before it lands, and it is reached whatever `core.hooksPath` a machine uses
- [ ] `ci.yaml` calls `make lint`
- [ ] copyright (984 files, passing today), markdown (471, unmeasured), spelling, and **`star lint
      starlark` from lane 8, not buildifier** (162 `.star` files) all clean

### Phase 4: the go-style sweep (lane 9)

- [ ] devlore-cli#938 has landed
- [ ] `star lint go-style --fix` run over the tree, then `git diff -w` read for anything that moved
      **outside** the intended class, then `make vet-all` and the full suite. The compiler and the tests
      are the review; nobody reads 843 files
- [ ] `go-style` added to CI

### Phase 5: green, and the documents this makes stale

- [ ] #964's own table corrected from 33 to 51, with the reason
- [ ] This document set to `complete` in the last commit of the last pull request

## Related documents

- [noblefactor-ops#232](https://github.com/NobleFactor/noblefactor-ops/issues/232) -- the lint tooling schedule; this is lanes 7, 8 and 9
- [personal#216](https://github.com/David-Noble-at-work/personal/issues/216) -- the same gate in personal, 234 findings to zero; the precedent for Requirement 1
- [devlore-cli#938](https://github.com/NobleFactor/devlore-cli/issues/938) -- why Phase 4 waits
- [devlore-cli#949](https://github.com/NobleFactor/devlore-cli/issues/949) -- the schedule whose lanes 11, 13 and 21 Phase 2 unblocks