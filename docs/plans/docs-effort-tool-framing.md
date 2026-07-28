---
title: "EFFORT.md: the human is the performer, the machine is the tool"
issue: TBD
status: complete
created: 2026-07-28
updated: 2026-07-28
---

# Plan: EFFORT.md — the human is the performer, the machine is the tool

Ruling 2026-07-28: nothing in the repository may frame Claude as a contributor —
machines do not contribute; they are tools used by contributors. The repository sweep
(working tree, contributor graph, release notes, full commit history) found one living
line with actor-framing to correct.

## Changes

1. `docs/EFFORT.md` § Tooling — "All development performed with Claude Code
   (AI-assisted). Single human author reviewing, steering, and approving all changes."
   becomes "A single human author performed all development — reviewing, steering, and
   approving every change — using Claude Code as a tool." The human is the subject and
   performer; the tool is instrumental.

## Sweep evidence, recorded

- Working tree: no `Generated with Claude`, no `Co-Authored-By: Claude`, no robot-emoji
  attribution anywhere; remaining "Claude" mentions are product features (the Anthropic
  model provider), tool configuration (`CLAUDE.md`), and descriptions of tool behavior —
  none are credits.
- GitHub contributors graph: exactly one contributor, the human author.
- Release notes: no attribution.
- Commit history: two February commits (`2c49ed5`, `283c27e`) carry machine co-author
  trailers; ruling 2026-07-28 is to leave history as history (no rewrite).

## Verification

Docs-only change. `grep -ri "co-authored-by: claude\|generated with claude"` over the
tree stays empty; `docs/EFFORT.md` names the human as performer.
