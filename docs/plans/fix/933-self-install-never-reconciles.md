---
title: "self install replaces the record: the manifest is what the tool owns"
issue: https://github.com/NobleFactor/devlore-cli/issues/933
status: complete
created: 2026-09-23
updated: 2026-09-23
---

# Plan: self install replaces the record

## Issue 933

Lane 17 of NobleFactor/noblefactor-ops#217, ahead of #228 — "a serious bug that should be corrected
while it's fresh".

Ruled 2026-09-23: *"the manifest must be a record of what the tool owns and self install must
implicitly uninstall, if it detects a prior install"*, and *"this must be true for every app."*

## Current state

`runSelfInstall` (`cmd/internal/cli/selfinstall.go:216`) runs seven steps and never reads the prior
manifest. Step 7 calls `writeManifest` (`:632`), which builds its entries from scratch and
`WriteFile`s them. Every install therefore disowns the previous one's differing paths in the same
operation that stops writing them, and `runSelfUninstall` — which reads only the manifest — can never
reach them again.

| Step | What | In the manifest? |
| --- | --- | --- |
| 1 | binary | yes |
| 2 | man pages | yes |
| 3 | shell completions | yes |
| 4 | config and cache | **no** |
| 5 | post-install hooks (star's extensions) | yes, since #917 |
| 6 | writ layer directories | **no** |
| 7 | write manifest | — |

**Every app, by construction.** `lore` (`cmd/lore/lore/root.go:27`), `star`
(`cmd/star/star/root.go:167`), `writ` (`cmd/writ/writ/root.go:27`) and `devlore-test`
(`cmd/devlore-test/devloretest/root.go:32`) all build through `cli.NewRootCmd`, which wires
`NewSelfCmd` with this same pair. One function satisfies the ruling for all four.

## The shape of the fix, and one decision it needs

**`runSelfUninstall` cannot be called wholesale.** Beyond removing the manifest's files it also runs
`PostUninstallHooks`, **removes the tool's config and cache** (`:522-525`), and prints "Uninstalled
%s". An install that called it would delete the operator's configuration every time. The reusable
part is the file-removal core — read the record, hash-guard, remove, `cleanEmptyDirs` — and that is
what "implicitly uninstall" means here. Config was never in the record, so this is the ruling applied,
not narrowed.

**The decision: when does the retirement run?**

| | A — retire, then install | B — install, then retire the difference |
| --- | --- | --- |
| Shape | remove everything the prior record owns, then install fresh | install (overwriting same paths), then remove prior-record paths absent from the new record |
| End state | identical | identical |
| The running binary | **removed, then rewritten.** Windows refuses to delete a running executable, and `star self install` invoked from the installed copy is a real case on DANOBLE-WD11-3 | overwritten in place, as today |
| A failed install | leaves the tool **absent** — nothing here is transactional | leaves the previous generation's extra files; the tool works |
| Work | re-copies every file | copies what it would anyway |

**Ruled 2026-09-23: B.** It reaches the state the ruling demands without a window in which the tool is
missing and without removing a file the operating system may refuse to remove. It is also the literal
reading of the verb #913 gave writ for the identical defect — a deploy **replaces** the record — where
replacement is the outcome, not a delete-then-write sequence.

### The vocabulary is borrowed; the model is not

Ruled 2026-09-23: *"we should reuse it when describing this tool's function, but we aren't running
graphs here and don't have the infrastructure guarantee to support that. We get a manifest, not a
trace."*

So this plan takes writ's **verb** — an install replaces the record — and none of its machinery. A
trace carries receipts that fold, lifetimes that can be pruned, and a reconciliation that can restore
a system to a desired state because the graph that produced it is replayable. A manifest is a list of
paths and hashes written at the end of one operation. It can answer "is this file the one I wrote",
and it cannot answer "what would it take to get back here". Nothing in this plan should grow toward
the second; if a future reader finds themselves wanting `readback.Fold` here, the answer is that this
record does not have that shape and was not meant to.

## Requirements

### Requirement 1: The record is read before it is replaced

`runSelfInstall` reads the prior manifest before step 7. A missing manifest is a first install and is
normal — not the error `runSelfUninstall` raises at `:470-474`.

### Requirement 2: What the tool no longer owns is retired

Paths in the prior record and absent from the new one are removed, under the same hash guard
`runSelfUninstall` applies: a file whose hash does not match what was recorded is left and reported,
never deleted. `cleanEmptyDirs` then runs over the retired entries.

### Requirement 3: The record then matches the tree

After any install, the manifest lists exactly the files that install owns under the prefix, so
`self uninstall` leaves a clean prefix regardless of how many installs preceded it.

### Requirement 4: Config, cache and layer directories are untouched

Steps 4 and 6 place things the manifest does not record. The retirement reaches none of them, exactly
as `self uninstall`'s file loop does not.

### Requirement 5: The operator is told

The install summary reports what was retired and what was skipped for a hash mismatch, in the shape
the uninstall summary already uses. A retirement that removes files silently is the same class of
problem as the one being fixed.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The retirement

- [x] The file-removal core factored out of `runSelfUninstall` so both callers share it —
      `removeRecordedFiles`, hash guard and `cleanEmptyDirs` included (Requirements 2, 4)
- [x] `runSelfInstall` reads the prior record and retires what is no longer owned —
      `retireSupersededFiles` as step 7, before the manifest is written and after everything is
      placed (Requirements 1, 2)
- [x] The install summary reports retired and skipped files — `printRetirementSummary`
      (Requirement 5)
- [x] `runSelfUninstall` loses its `//nolint:gocognit`: extracting the loop took it back under the
      threshold

### Phase 3: Tests

- [x] A fixture install, a changed file set, a re-install: the old paths are gone
      (`TestRunSelfInstall_RetiresWhatItNoLongerOwns`) and the record names exactly what is on disk,
      compared in both directions (`TestRunSelfInstall_ManifestMatchesTheTree`) — Requirement 3
- [x] A first install with no prior record retires nothing
      (`TestRetireSupersededFiles_FirstInstallHasNothingToRetire`) — Requirement 1
- [x] A file modified after install is left untouched, with the operator's content intact
      (`TestRunSelfInstall_LeavesAFileChangedSinceItWasWritten`) — Requirement 2
- [x] A file in the prefix that no record names survives a re-install
      (`TestRetireSupersededFiles_ReachesNothingOutsideTheRecord`), which is the general form of
      Requirement 4: config, cache and writ's layer directories are recorded by nothing
- [x] **The two tests that prove the fix were made to fail.** With the retirement disabled,
      `RetiresWhatItNoLongerOwns` and `ManifestMatchesTheTree` both fail; the other three still pass,
      correctly, since they guard against over-deletion rather than under-deletion

### Phase 4: Documentation

- [x] `docs/architecture/10-command-line-interface.md` §3 "The lifecycle is named once" — where #913
      defines the four operations by their effect on the record — gains the note that `self install`
      takes the verb and not the model, and that this record is a manifest, not a trace
- [x] `self install`'s own `Long` corrected: it listed five steps and the retirement made it six.
      **Missed in the fix commit** — user-facing help is a document, and rule 12 puts its correction
      in the commit that stales it
- [x] `docs/cli/` needs no commit: it is gitignored here and generated by `make build`, published to
      the site by `.github/workflows/docs-publish.yaml`. The behaviour reaches it through the `Long`
      above

### Phase 5: Verify, then merge

- [x] `make vet`, `make lint`, `make test` — all clean. The first lint run failed on one finding:
      `misspell` enforces US spelling in Go source (`behaviour` in a test comment), even though this
      repository's prose is British. Worth knowing; it will recur
- [x] `gofmt -l` clean over every changed file
- [x] **A live re-install on this box, with the built binary.** Installed to a scratch prefix with
      `--shell bash`, then again with `--shell zsh`: it printed `Retired 1 file(s) a previous install
      placed and this one does not` and named the bash completion, which was gone. Before this change
      that file stayed forever. Then, editing the zsh completion and re-installing printed `Left 1
      modified file(s) a previous install placed`, and the operator's line survived — both halves of
      the ruling on the live path rather than in a test
- [x] PR script written, analyser-clean (parse errors 0, findings 0), shown, and handed over. It
      gains a `gofmt -l` pre-flight, guarded on there being changed Go files: `gofmt -l` with no file
      operands reads standard input, which would hang the script on a branch that touches no Go
- [x] CI runs on the pull request and the merge gate blocks until every check reports pass

## Open Questions

- [x] **A or B?** B, ruled 2026-09-23.
- [x] **Does this want writ's lifetime model?** No, ruled 2026-09-23: the vocabulary is reused to
      describe the function; the model is not. A manifest, not a trace. Recorded above.
- [ ] **The hash-mismatch skip — a proposal, not yet ruled.** Ruled 2026-09-23 that there is no
      solution to the file itself, and that is right: a file whose hash has changed cannot be deleted
      (it may be the operator's edit) and cannot be trusted (it is not what we wrote). Both of those
      stay true.

      But the *deletion* is not the part that hurts. The **forgetting** is. Today the retirement
      would leave that file and drop it from the new record, so the next install cannot even tell it
      once existed — which is this whole issue, reproduced one file at a time.

      **Proposal: the new manifest carries it as a retained entry.** A second list beside `files` —
      path, the hash we wrote, the hash found — for entries the tool owned, still present, and no
      longer matching. Nothing is deleted. The record stays a record of what the tool owns, which is
      the ruling. `self uninstall` can then end with "3 files were modified and left: …" instead of
      silence, and an operator who resolves one sees it drop off the next install.

      It stays manifest-shaped: a list with hashes, no fold, no replay. The cost is that the list
      grows while the operator ignores it, which is the honest representation of the situation.

      Not in the phases below. If it is wanted, it is a requirement and a phase of its own.

## Out of Scope

**The unbounded-fold half of writ's defect** (#922). Same class, different record, already scheduled.

**Transactional install.** Nothing in `runSelfInstall` rolls back today, and B is chosen partly so
this plan does not need to make it.

## Related Documents

- [#917](https://github.com/NobleFactor/devlore-cli/issues/917) — uninstall removes only what its manifest records; the precondition that made this visible
- [#913](https://github.com/NobleFactor/devlore-cli/issues/913), [#916](https://github.com/NobleFactor/devlore-cli/issues/916) — writ's identical defect, and the verb this borrows
- [noblefactor-ops#227](https://github.com/NobleFactor/noblefactor-ops/issues/227) — where the 24 stranded files were found
