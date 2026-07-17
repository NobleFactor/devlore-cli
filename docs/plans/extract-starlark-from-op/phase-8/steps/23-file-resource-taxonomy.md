---
step: 23
former_step: 20
title: "Factor file.Resource into a taxonomic tree"
status: not-started — charter AMENDED 2026-07-16: the Merkle-root directory digest is an explicit deliverable (must land before phase-8 closes); unblocked (step 18 closed 2026-07-16)
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 23 — Factor `file.Resource` into a taxonomic tree

**Status:** `not-started`; charter amended 2026-07-16 (the Merkle-root directory digest below). No code has begun.

## What this step delivers

Split the catch-all `file.Resource` into a base type plus specialized variants: `file.Resource` keeps shared
identity + URI + SourcePath + cross-kind metadata; `file.Regular` holds regular-file fields (Checksum, Size, Mode);
`file.Directory` holds directory concerns; `file.Link` holds symlink target + follow behavior. Each variant
implements the twelve required Resource interfaces. Every provider method that takes a generic `*file.Resource` is
audited against the three variants
and rewritten to the specific one its semantics require (Copy/WriteText → `*file.Regular`; Mkdir → `*file.Directory`;
Link → `*file.Link`).

**The Merkle-root directory digest (added 2026-07-16 — must land before phase-8 closes).**
`file.Directory.Digest()` implements the Merkle-root scheme the catch-all deferred here
(`pkg/op/provider/file/resource.go:234` returns `op.ErrUnimplemented` for directories): sha256 over the sorted
(relative path, per-entry digest) pairs of the tree — regular files by content digest, symlinks hashed by their
target, subdirectories by their own Merkle root — so identical tree state yields an identical digest and any
content, rename, or structural change flips it. The step-48 ruling (2026-07-16) deliberately left directories
digest-less in the trace's ledger snapshot until this lands; the snapshot capture is best-effort over
`Resource.Digest()`, so directory digests start populating automatically the moment this deliverable exists —
no step-48 changes required. `file.Directory.Etag()` stays the cheap stat tuple.

## Evidence — not started

- `pkg/op/provider/file/resource.go:31` declares the single `type Resource struct`. There is **no** `type Regular`,
  `type Directory`, or `type Link` anywhere under `pkg/op/provider/file`.
- A tree-wide `grep` for `file.Regular` / `file.Directory` / `file.Link` (type references) returns **zero** hits.
  (The 2026-06-17 audit's three action-name matches — `adopt_cmd.go`, `adopt/plan.go`, `migrate/file_ops.go` —
  referenced files since rewritten or deleted by steps 33/47; `file.link` remains an action name throughout, never
  a type.)
- No taxonomy tests exist.

## Disposition / grade

`not-started` — accurate. No deliverable, no tests. Formerly downstream of the phase-8 exit gate (step 18, closed
2026-07-16) and the helper-test backfill (step 24); the gate no longer blocks it.
