---
title: "Plan statuses: every plan uses the process's five words, and CI keeps it so"
issue: https://github.com/NobleFactor/devlore-cli/issues/1027
status: draft
created: 2026-10-04
updated: 2026-10-04
---

# Plan: every plan uses the process's five statuses

## Summary

development-process.md and `docs/plans/TEMPLATE.md` give a plan five statuses: `draft`, `approved`, `active`,
`complete`, `abandoned`; `in-progress` and `chartered` were retired on 2026-09-21 (NobleFactor/noblefactor-ops#219).
Of the 349 files under `docs/plans`, 133 are outside that vocabulary or carry no frontmatter, and nothing checks:
devlore-cli's CI validates the guides' frontmatter and not the plans'. This plan brings every file under
`docs/plans` to frontmatter whose `status` is exactly one of the five words, and adds the gate that keeps it so,
so the pull request can state it with the gate's output as the proof.

## Issue 1027

Chore, epic `Ops:Process` (NobleFactor/noblefactor-ops#142), feature NobleFactor/noblefactor-ops#156; #916 lane
45, PR C. The owner, 2026-10-04: "update the 32 devlore-cli plans still using the retired in-progress or chartered
statuses. DO THAT NOW." Then: "rigidly follow process to create an issue and in the PR i want you to say with 100%
certainty that all plans are using our status vocabulary."

## Goals

1. Every file under `docs/plans` carries frontmatter whose `status` is exactly one of the five words.
2. CI fails a plan whose status is missing or outside the vocabulary.
3. The pull request states goal 1 with the gate's output as the proof.

## Current State

| State | Files |
| --- | --- |
| Status is exactly one of the five words | 215, 32 of them edited 2026-10-04 from `in-progress` and `chartered` |
| One of the five words, with commentary or the wrong case | 53 |
| Outside the vocabulary | 23 |
| No frontmatter, no status | 49 |
| No frontmatter, a status elsewhere (a yaml code block, the body) | 8 |
| `TEMPLATE.md`, frontmatter without a status | 1 |

`extract-starlark-from-op/phase-8/lore-migration.md` has a blank line ahead of its frontmatter, so its frontmatter
does not parse. `TEMPLATE.md`'s body lists `draft | in-progress | complete | abandoned`.

The base layer's gate, `.github/scripts/Test-Frontmatter.sh`, reads every tracked `.md` in the repository, takes no
path to narrow it, and requires `title` alone of a working document: a plan without a status passes it.

## Requirements

### Requirement 1: the retired words

`in-progress` and `in progress` become `active`, the word development-process.md says they mean; `chartered`
becomes `approved`. The 32 such edits were made in the worktree on 2026-10-04 before this plan existed, which breaks
the process's order; open question 1 rules them.

### Requirement 2: commentary after the word

53 statuses carry one of the five words followed by commentary (`COMPLETE 2026-07-18 — all four slices landed`) or
in the wrong case (`Draft`). The status becomes the lower-case word alone; the commentary is ruled by open question
2.

### Requirement 3: values outside the vocabulary

| Value | Files | Becomes |
| --- | --- | --- |
| `proposed` | 5 (`writ-secret-*`) | `draft` |
| `charter — chartered …` | 5 (steps 53, 55, 58, 59, 60) | `approved`, unless the step's own boxes show the work begun or done |
| `IMPLEMENTED`, `SHIPPED`, `settled — implemented …`, `closed`, `completed` | 8 | `complete` |
| `done pending commit` | 1 (step 44) | `complete` once the commit it awaited is found in the history; otherwise `active` |
| `ready` | 1 (`workflow-rename.md`) | `approved` |
| `design-solidified` | 1 (step 50) | `approved` |
| `pending` | 1 (`compensation/phase-2.md`) | `draft` |
| `deferred` | 1 (step 38) | `draft` |

Open question 3 asks for the table's approval.

### Requirement 4: files without frontmatter

The 57 gain frontmatter: `title` from the document's first heading, `status` from reading the document. The 8 with
a status elsewhere move it into the frontmatter. Each file's ruling is listed in the pull request.

### Requirement 5: the template and the stray blank line

`TEMPLATE.md`'s body lists the five words. `lore-migration.md` loses the blank line ahead of its frontmatter.

### Requirement 6: the gate

CI fails when a file under `docs/plans` lacks frontmatter, lacks `status`, or carries a status outside the five
words. How, given the base layer's script as it stands, is open question 4.

## Implementation Phases

### Phase 1: The plan

- [ ] This document, reviewed with the owner.

### Phase 2: The statuses and the frontmatter

- [ ] Requirements 1 to 5, each file's ruling recorded for the pull request.

### Phase 3: The gate

- [ ] Requirement 6, as open question 4 rules it.

### Phase 4: Acceptance and closure

- [ ] The gate passes over `docs/plans`, and its output is in the pull request.
- [ ] A scratch plan with `status: in-progress`, and one without a status, each fail the gate.
- [ ] This plan's status is `complete` in the last commit of PR C.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1027-plan-statuses-every-plan-uses.md` | Create | This plan |
| `docs/plans/**/*.md` | Modify | Requirements 1 to 5 |
| `.github/workflows/ci.yaml` | Modify | Requirement 6 |

## Related Documents

- NobleFactor/noblefactor-ops#219: the five words ruled
- development-process.md, § Documents on every commit; documentation-standards.md: the gate and its families
- #916, lane 45

## Open Questions

1. **The 32 edits made before this plan.** (a) They stand as Requirement 1's work, committed after this plan is
   approved. Recommended: they are exactly Requirement 1. (b) They are reverted and redone after approval.
2. **Commentary after the word.** (a) It moves into the body, as the first paragraph under the title, beginning
   "Status:". Recommended: it is the plan's history. (b) It is dropped.
3. **Requirement 3's table**, approved as written or amended.
4. **The gate.** (a) The base layer's script unchanged, over every tracked `.md` in devlore-cli: every document in
   the repository conforms or is exempted, far wider than this issue. (b) The base layer's script gains a path scope
   and a rule that a plan carries a status, as an issue of its own in NobleFactor/noblefactor-ops; devlore-cli's CI
   runs it at a pinned ref, as personal's CI runs the base layer's scripts. Recommended: one script for the
   organization, as documentation-standards.md asks. (c) A check of devlore-cli's own, which documentation-standards.md
   asks repositories not to write.
