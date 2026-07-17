---
step: 23
former_step: 20
title: "Factor file.Resource into a taxonomic tree"
status: design in progress — questions 1–3 SETTLED 2026-07-17 (taxonomy names, the string-parameter rule, the mutator invariants incl. deletion-marks-Gone); question 4 (the common currency for "any file resource") open; the Merkle-root deliverable stands
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 23 — Factor `file.Resource` into a taxonomic tree

**Status:** design in progress (settled rulings below); no code has begun. Charter amended 2026-07-16 (the
Merkle-root directory digest); design rulings 2026-07-17.

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

## Settled design (2026-07-17)

1. **Taxonomy names.** `file.Regular`, `file.Directory`, `file.SymbolicLink` (user-named — not "Link"). No further
   variants: a hard link is an aliasing property of a regular file, not a kind (the inode already participates in
   Etag); FIFO/socket/device files have no producing or consuming action; a mount point is not a file type but an
   observation about a directory (device ID differing from its parent) — if ever needed it lands as an
   [Observation] field, never a variant.
2. **Variant assignment — declared intent, never disk observation.** Planning is offline (a product does not exist
   at plan time) and "existence is observation, not identity" is settled law. A mutator's own semantics mint its
   product variant internally (`Mkdir` → `*Directory`); a content-reading method's typed parameter declares what it
   consumes; enumerators (`Find`/`Glob`/`WalkTree` entries) construct the observed kind — they are already statting
   the disk.
3. **The string-parameter rule.** A provider method takes a resource argument only when it reads the resource's
   content (`ReadBytes`, `ReadText`, `WalkTree`'s root, `Copy`'s source) or mints its cataloged observation
   (`Observe`). Everything else — create, update, delete, and the location queries `Exists`/`IsDir`/`IsFile` —
   takes a path string; the resource is the *product* of the action, returned, never passed in. Dependency edges
   survive the string turn (edges wire by variable flow, and `file.Resource` → `string` conversion is pinned
   immediate at `pkg/op/planner.go:331`); catalog claims survive because the mutator mints identity internally,
   exactly as `Mkdir`/`WriteText`/`Copy` already do.
4. **Mutator invariants (ruled; independent of any return value).**
   - **Create**: the mutator interns the product in the catalog (producer-stamped claim via `NewResource`).
   - **Update**: the mutator interns AND moves the original to the recovery site *before* the update — the
     `prepareWrite` seam already discharges this (`provider.go:1605`).
   - **Delete**: the mutator interns via `Discover` (deletion is termination, not production — no producer stamp),
     moves the target to the recovery site, and **marks the catalog entry `Gone` on success** — the catalog
     reflects what the run *did*, not just what it observed. `Gone` stays terminal; revival remains a production
     act via the shadow path (`pkg/op/resource_catalog.go:139`).
5. **Audit findings the rule fixes.** `Remove`/`RemoveAll`/`Unlink` never intern — a slot-coerced (unlinked)
   resource is deleted and archived without the catalog ever seeing it; `WriteFile` takes a `*Resource` and interns
   nothing. Both defects vanish when the mutators take strings and mint internally.

## Open design questions

1. ~~Variant assignment~~ — settled (declared intent, above).
2. ~~String vs. resource parameters~~ — settled (the string-parameter rule, above).
3. ~~Deletion's catalog claim~~ — settled (Discover + mark `Gone`, above).
4. **The common currency for "any file resource"** — what discovery, rehydration, and mixed enumeration produce,
   and the base type's constructibility. In discussion.
5. Per-variant `Digest`/`Etag` semantics table (`Directory` = Merkle root + stat-tuple Etag are chartered;
   `SymbolicLink` standalone semantics need a ruling; `Regular` keeps current behavior).
6. Consumer signatures (`archive.Extract`, `encryption`, `starcode`, the existence-verification gate at
   `pkg/op/graph_executor.go:1016`) — each consult-gated.

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
