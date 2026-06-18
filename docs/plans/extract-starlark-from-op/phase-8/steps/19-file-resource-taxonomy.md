---
step: 19
title: "Factor file.Resource into a taxonomic tree"
status: not-started — confirmed (no variant types exist)
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 19 — Factor `file.Resource` into a taxonomic tree

**Status:** `not-started`. The row's label is accurate — no work has begun.

## What this step delivers

Split the catch-all `file.Resource` into a base type plus specialized variants: `file.Resource` keeps shared identity +
URI + SourcePath + cross-kind metadata; `file.Regular` holds regular-file fields (Checksum, Size, Mode); `file.Directory`
holds directory concerns; `file.Link` holds symlink target + follow behavior. Each variant implements the twelve required
Resource interfaces. Every provider method that takes a generic `*file.Resource` is audited against the three variants
and rewritten to the specific one its semantics require (Copy/WriteText → `*file.Regular`; Mkdir → `*file.Directory`;
Link → `*file.Link`).

## Evidence — not started

- `pkg/op/provider/file/resource.go:31` declares the single `type Resource struct`. There is **no** `type Regular`,
  `type Directory`, or `type Link` anywhere under `pkg/op/provider/file`.
- A tree-wide `grep` for `file.Regular` / `file.Directory` / `file.Link` (type references) returns **zero** hits. The
  three matches that surface (`cmd/writ/writ/adopt_cmd.go:30`, `adopt/plan.go:31`, `migrate/file_ops.go:67`) all
  reference the **`file.Link` action/method**, not a `file.Link` type.
- No taxonomy tests exist.

## Disposition / grade

`not-started` — accurate. No deliverable, no tests. This step is downstream of the phase-8 exit gate (step 18) and the
helper-test backfill (step 20); it has not been picked up.
