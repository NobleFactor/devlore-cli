---
title: "Three star design documents describe code that ships here and is documented nowhere"
issue: https://github.com/NobleFactor/devlore-cli/issues/937
status: active
created: 2026-09-23
updated: 2026-09-23
---

# Plan: the star design record arrives where the code is

## Issue 937

Step 1 of noblefactor-ops#228, lane 19 of noblefactor-ops#217 — the last lane of that schedule.
Upstream first: this repository receives, then noblefactor-ops deletes.

Three documents in noblefactor-ops are the only written design of code that ships here.

## Verified against this tree

- **"The Shape of the Tree" has no counterpart here.** Grepped `docs/` for the heading and for
  "service or product, never a tool": no hits.
- **`3.5-provider-catalog.md` lists none of the five star providers** — `starcode`, `starindex`,
  `starstats`, `starcomplexity`, `staranalysis` — while line 17 calls itself "the **index of record**
  for which providers exist".
- **Six back-references exist**, including two absolute
  `github.com/NobleFactor/noblefactor-ops/blob/develop/…` URLs at
  `docs/plans/move-star-to-devlore-cli.md:248-249` that 404 once #228 step 2 runs.

## Numbering

Ruled 2026-09-23: the numbered series, *"as is our pattern for design documents (they sort in read
order that way)."*

**§9 Star is the only section whose documents are not numbered.** Sections 1–8 and 10 all are —
`7-registry-knowledge.md` with `7.1-llm-integration.md` and `7.2-e2e-testing.md` beside it,
`10-command-line-interface.md`. §9 holds one named file. So the section is numbered here, not only
its new arrivals:

| File | From |
| --- | --- |
| `9-star-extensions.md` | `star-extensions.md`, renamed — the section document |
| `9.1-doc-comment-styling.md` | noblefactor-ops `doc-comment-styling.md` |
| `9.2-source-analysis.md` | noblefactor-ops `star-source-analysis-api.md` |
| `9.3-file-tree-walking.md` | noblefactor-ops `star-file-tree-walking.md` |

Nine references to `star-extensions.md` exist in this repository, most of them in this plan. The
rename is cheap; leaving the parent unnumbered while its children are numbered is not.

**One placement note, not acted on:** `9.3` describes `pkg/gitignore` and the file provider's
`WalkTree` — framework rather than star, so `3.5.4-file-provider.md` is arguably its home. It arrives
under §9 because that is where the star record came from. Moving it later is a rename.

## A correction to #228's scope

#228 says this step registers the documents in `index.md`, "**closing its two '(planned)' gaps**."
Following that literally would leave the index advertising something that does not exist.

§9 lists two planned entries, and **neither is one of the three documents moving**:

- **Star WASM Receivers can never be filled.** noblefactor-ops#230 deleted that document because no
  repository holds the subsystem: no `wazero` in `go.mod`, no `.wasm`, no `internal/wasm/`, and
  `move-star-to-devlore-cli.md:35` still records it as "TBD — not yet moved". **Removed.**
- **Star Configuration is not moving as a document.** Two of its tables fold into `configuration.md`.
  The entry **points there**.

The index gains three entries and loses two promises.

## Requirements

### Requirement 1: The three arrive, each saying what it is

Each gains a note at the top: what is as-is against this code, and what is superseded. #130 deleted
noblefactor-ops' Starlark warning that *"a diverged second copy is how the next person builds the
wrong thing"*; a document moved without that note is the same hazard in prose.

| Document | As-is | Superseded |
| --- | --- | --- |
| `9.1` | The production model, fuzzy slot filling, the schema fields `LintGoStyle/extension.yaml` carries | The "Porting to devlore-cli" section is **unexecuted** — tracked as #938 |
| `9.2` | The data shapes, field for field | The namespace is `starcode.capture`, not `starlark.capture`; `include_bzl` does not exist; the gitignore flag is `include_gitignored`, inverted |
| `9.3` | The ignore stack, and the go-git / no-shell-out / no-parallel-walk decisions | The whole API section. `WalkTree` is a fold returning a recovery receipt; `file.tracker()` does not exist; the package is `pkg/gitignore` |

### Requirement 2: Two sections are salvaged

- **"The Shape of the Tree"** into `9-star-extensions.md`.
- **Two tables** from `star-configuration.md` into `configuration.md`: the field-type reference and
  the `config.sync` mapping. Type names updated — `ConfigSpec` is `config.Spec`, `ConfigElement` is
  `config.Element`, `ConfigAccessor` is `config.Accessor`.

### Requirement 3: The index tells the truth

Four entries under §9, the WASM entry removed, the Configuration entry repointed.

### Requirement 4: The provider catalog stops omitting five providers

`3.5-provider-catalog.md` gains the five.

### Requirement 5: The back-references point somewhere that will exist

Six, including the two absolute URLs.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: Number, receive, salvage

- [x] `git mv star-extensions.md 9-star-extensions.md` (references in Phase 3)
- [x] The three in as `9.1`, `9.2`, `9.3`, each with its note (Requirement 1). `9.2` also lost the stray outer ```` ```markdown ```` fence that wrapped the whole document, which is why noblefactor-ops kept it in `.github/frontmatter-exempt`
- [x] "The Shape of the Tree" into `9-star-extensions.md` (Requirement 2)
- [x] Two tables into `configuration.md`, type names updated (Requirement 2)

### Phase 3: The indices

- [ ] `index.md`: four entries, WASM removed, Configuration repointed (Requirement 3)
- [ ] `3.5-provider-catalog.md`: five providers (Requirement 4)
- [ ] Six back-references repointed (Requirement 5)

### Phase 4: Verify, then merge

- [ ] `make vet`, `make lint`, `make test`
- [ ] The `en-GB_to_en-US` dictionary run over the three arrivals: zero hits
- [ ] Every internal link in the moved documents resolves
- [ ] PR script written, analyzer-clean, shown, and handed over
- [ ] CI runs on the pull request and the merge gate blocks until every check reports pass

### Phase 5: Close devlore-cli#935

- [ ] #935's last box ticked and its plan set `complete` — it reads `active` behind a closed issue
      because its Phase 4 closed noblefactor-ops#234, which landed in another repository

## Related Documents

- [noblefactor-ops#228](https://github.com/NobleFactor/noblefactor-ops/issues/228) — the audit, and step 2
- [noblefactor-ops#230](https://github.com/NobleFactor/noblefactor-ops/pull/230) — where the WASM document was deleted, and why
- [#938](https://github.com/NobleFactor/devlore-cli/issues/938) — the unexecuted port `9.1` records
