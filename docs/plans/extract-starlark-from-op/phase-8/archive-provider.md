---
title: "Archive provider — implementation plan"
status: draft
created: 2026-06-28
---

# Archive provider — implementation plan

**Design of record: [`docs/architecture/3.5.1-archive-provider.md`](../../../architecture/3.5.1-archive-provider.md).**
The design carries the full model — the two-layer format split, content detection, the decompressor → container
pipeline, the receipt/compensation integration, security, and the supported-format roadmap. This document carries
**sequencing and work items only**.

## Scope — absorption of file-mutation-receipts slice 3

This plan **absorbs file-mutation-receipts slice 3** (2026-06-28). It owns the **entire** archive rewrite:

1. the exported `file.Provider` mutation surface that `archive.extract` consumes (originally the first half of slice 3),
2. the re-base of `archive.extract` onto that surface (the second half of slice 3),
3. **content-based format detection** replacing the extension switch (net-new),
4. the **decompressor → container pipeline** and the **full tar family + plain tar** (net-new).

[`file-mutation-receipts.md`](file-mutation-receipts.md) slice 3 is now a pointer here.

## Dependencies

- **file-mutation-receipts slices 1–2(b)** — the unified mutation core (slice 1, landed), the compensation seam +
  `CompensateFileMutation` (slice 2), and the cross-provider constructor migration + `Commit`-fallback drop (slice 2b).
  The rewrite calls `file.WriteFile`/`MakeDir` and relies on `stack.Unwind` routing each `*file.Receipt` to
  `file.CompensateFileMutation`. See [`file-mutation-receipts.md`](file-mutation-receipts.md).
- **Step 24 — activation-record-first invariant**
  ([`steps/24-activation-record-first-invariant.md`](steps/24-activation-record-first-invariant.md)). The exported
  mutation methods carry no `*op.ActivationRecord`, so step 24's discriminator excludes them from the Starlark surface
  with **no `+devlore:internal` flag** (file-mutation-receipts decision 5). **Sequence step 24 before S1.**
- **New module dependencies (S5):** `github.com/ulikunitz/xz`, `github.com/klauspost/compress` (zstd) — both pure Go,
  additive, on the extraction path only.

## Slices

Each slice is a commit unit that builds and tests green (see the verification model below).

### S1 — exported file-mutation surface (`file.Provider`)

The four dispatch-independent mutation methods, landing with their first cross-package consumer (`archive`):
`WriteFile(target *Resource, src io.Reader, mode os.FileMode)` (wrap `write`, chown `""`), `DeleteFile(target)` (wrap
`Remove`, `MutationDeleteFile`), `MakeDir(target, mode)` (factor a `mkdir` core out of `Mkdir`, `MutationCreateDir`),
`RemoveDir(target)` (`MutationDeleteDir`). Excluded from the Starlark surface by step 24's discriminator (gate: step 24
precedes this slice).

### S2 — `archive.extract` onto the mutation core (behavior-preserving)

Re-base `Extract` onto `openArchive` + the mutation surface, **keeping** the existing two formats and extension-internal
selection so the slice is a pure mechanism swap:

- introduce `openArchive(source) (archiveReader, error)` — the `(Name, Mode, IsDir, Reader)` entry iterator — with its
  format selection still **by extension** internally (tar.gz/tgz → tar-over-gzip; zip → zip);
- the loop calls `MakeDir` (dir entries) / `WriteFile` (file entries), pushes each returned `*file.Receipt`, and returns
  the `*op.RecoveryStack`;
- `CompensateExtract` collapses to `stack.Unwind()`;
- fix `archive/provider_test.go` (currently `[build failed]` — it still uses `len(receipts)` / `range receipts` against
  the interim `[]op.Receipt` signature).

### S3 — content detection inside `openArchive`

Swap `openArchive`'s extension selection for magic-byte detection (design §3): read up to 262 bytes with `io.ReadFull` /
`io.ReadAtLeast` (tolerating short files), `Seek(0, 0)` for a non-destructive sniff, match the magic table, and fall
back to the `ustar`-at-257 / identity (plain `tar`) path. Fix the stale contract: `Extract`'s "format is detected from
the source file's extension" doc comment (`provider.go:55`). Still routes to the existing gzip/zip readers plus the new
identity (plain `tar`) reader.

### S4 — decompressor pipeline + bzip2 (no new deps)

Introduce the Layer-A decompressor table (gzip / identity / **bzip2** via stdlib `compress/bzip2`) feeding the single
Layer-B `tar.NewReader`. Adds `tar.bz2`/`tbz2` and confirms plain `tar`. No `go.mod` change.

### S5 — xz + zstd (new deps)

Add `github.com/ulikunitz/xz` and `github.com/klauspost/compress/zstd` to the decompressor table; adds `tar.xz`/`txz`
and `tar.zst`/`tzst`. Updates `go.mod`/`go.sum`.

### S6 — security hardening + format coverage tests

Adopt `github.com/cyphar/filepath-securejoin` for symlink-aware containment (replacing the hand-rolled prefix check);
settle the special-entry-type policy (symlink/hardlink/device/FIFO — design §10 Q1) and the total-extraction cap
(design §10 Q2); table-driven detection + round-trip extraction/compensation tests across all six formats.

## Verification model

Work in the worktree `devlore-cli.extract-starlark-from-op` (not the sibling `devlore-cli` checkout). `make build`
compiles `lore` + `star`, then fails at `cmd/writ` — the **standing break** (`op.ImmediateOf` /
`plan.Provider.Assemble` undefined); treat a build that fails *only* at `cmd/writ` as clean. `make test` standing reds
to ignore: `TestBackup_DefaultSuffix` (file), `TestWalkTree_Planned` (devlore-test), `TestShellCompletionPath_PerShell`
(star/cli), plus the `cmd/writ` / `internal/e2e` / docgen build breaks. Anything else red is new and belongs to the
slice in flight. Run `gofmt -w` on every touched `.go` file.

## Status table

| Slice | Summary | Status |
|-------|---------|--------|
| S1 | Exported `file` mutation surface | not started (gated on step 24) |
| S2 | `archive.extract` onto the mutation core + `openArchive` | not started (gated on file-mutation-receipts slices 1–2b) |
| S3 | Magic-byte content detection + contract fix | not started |
| S4 | Decompressor pipeline + bzip2 (stdlib) | not started |
| S5 | xz + zstd (new deps) | not started |
| S6 | Security hardening + format-coverage tests | not started |

## Related

- [`docs/architecture/3.5.1-archive-provider.md`](../../../architecture/3.5.1-archive-provider.md) — design of record.
- [`docs/architecture/3.5-provider-catalog.md`](../../../architecture/3.5-provider-catalog.md) — the provider catalog.
- [`file-mutation-receipts.md`](file-mutation-receipts.md) — the unified mutation mechanism (slices 1–2b prerequisite;
  slice 3 absorbed here).
- [`steps/24-activation-record-first-invariant.md`](steps/24-activation-record-first-invariant.md) — the discriminator
  gating S1.