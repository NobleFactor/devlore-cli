---
title: "Single-line doc summaries in the three chartered files"
issue: TBD
status: complete
created: 2026-07-27
updated: 2026-07-27
---

# Plan: Single-line doc summaries in the three chartered files

Chartered follow-up 5 — the last of the original five — of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

The mechanical sweep (first doc-comment line followed by a text line = violation) found
nine pre-existing multi-line summaries in the three files the windows-build fix touched.
Each is rewritten as a single-line summary, a blank comment line, then the elaboration —
no content dropped, sentences relocated into the elaboration where the summary ran long.

## Changes

1. `pkg/op/default_funcs.go` — `fileModeType`, `defaultMode` (joined to one line),
   `argFileMode` (list moved below the blank).
2. `pkg/op/provider/file/helpers.go` — `errKindMismatch`, `buildCandidateAs`,
   `internEntry`, `kindMismatchError`.
3. `pkg/op/provider/file/provider.go` — `WriteFile`, `errConflictSkip`.

## Verification

- The sweep reports zero violations across all three files.
- `make vet` pass (comment-only change); `gofmt -l` clean.
