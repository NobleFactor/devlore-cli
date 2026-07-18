---
step: 23
former_step: 20
title: "Factor file.Resource into a taxonomic tree"
status: slices 1–2 COMPLETE 2026-07-18 (variants + Merkle root + fragment-stripped location keying; ResourceCatalog.MarkGone; suite green) — design closed 2026-07-17, all six questions settled; CAS canonical-form identity RULED 2026-07-18, implementation post-phase-8; slices 3–4 pending
proof_run: 2026-06-17
parent: ../../phase-8.md
---

# Step 23 — Factor `file.Resource` into a taxonomic tree

**Status:** slices 1–2 complete (2026-07-18); slices 3–4 pending. Charter amended 2026-07-16 (the Merkle-root
directory digest); design rulings 2026-07-17; the mid-slice keying finding and the CAS ruling 2026-07-18.

## What this step delivers

*(The 2026-06-17 charter as written — superseded in detail by the rulings below: "Link" became `SymbolicLink`;
the per-variant metadata fields it lists moved to `Observation` before this step began, so variants are
behavior-differentiated; parameters follow the string-parameter rule rather than blanket variant rewrites.)*

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
5. **The common currency for "any file resource" (ruling 4, 2026-07-17).** The base `file.Resource` keeps its
   name and its charter role (shared identity: `ResourceBase` + `SourcePath`) but becomes embedded-only — no
   independent constructibility; every construction point mints a variant. Mixed-kind contexts traffic in a thin
   file-local interface, **`file.Entry`** (after the `fs.DirEntry` precedent): `Find`/`Glob` return
   `[]file.Entry`, `WalkTree`'s Reducer receives one, `Observe` accepts one. "Kind unknown" does not survive the
   other rulings: enumerators stat anyway (observed variant for free); rehydration reads the serialized canonical
   type id (which now names the variant); every remaining `DiscoverResource` caller knows its kind by intent.
   Implementation notes that ride the ruling: per-variant `Unmarshal*`/`ConvertFrom`/`Equal`/`String` (no
   promoted-method half-fills), and the existence-verification gate at `pkg/op/graph_executor.go:1016` enrolls
   three type ids.
6. **Per-variant `Digest`/`Etag` semantics (ruling 5, 2026-07-17 — 5a–5e).** `Regular` carries over current
   behavior (Digest = streamed sha256 of the bytes; Etag = sha256 of the packed size/mtime_ns/inode stat tuple).
   `Directory` is as chartered (Digest = the Merkle root; Etag = the cheap stat tuple). The settled cells:
   - **5a** `SymbolicLink.Digest` = sha256 of the readlink result (the literal target path), never following —
     a symlink IS a tiny file whose content is a path; matches the Merkle sub-scheme, works for dangling links
     (legal), cannot cycle.
   - **5b** `SymbolicLink.Etag` = the lstat-based stat tuple of the link inode itself. Fixes a latent defect:
     the catch-all's Etag uses `root.Stat`, which FOLLOWS symlinks — a link's Etag today reflects its referent
     and errors on a dangling link.
   - **5c** Merkle serialization is platform-stable: forward-slash-normalized relative paths, unambiguous
     delimiting (kind marker + path + NUL + entry digest), lexicographic order — step 48's recorded digests
     compare across runs and platforms.
   - **5d** The Merkle root covers everything: no gitignore filtering, no `.git` skip. Enumerators skip for
     convenience; a digest is content identity — skipping content would report "unmodified" over a modified
     tree. A live `.git` inside a deployed tree churns the digest as the repo breathes; that churn is the truth.
     The empty directory digests deterministically (the hash over zero entries).
   - **5e** Kind mismatch errors: a variant's Digest/Etag on a path observed to be another kind errors with a
     kind-mismatch message (the plan asserted one kind, the disk shows another); the catch-all's
     `op.ErrUnimplemented` directory branch disappears.
7. **Consumer signatures (ruling 6, 2026-07-17 — 6a–6e).**
   - **6a** `archive.Provider.Extract`: `source` becomes `*file.Regular` (content read); the destination-prefix
     discovery mints a `Directory`; products become `[]file.Entry` (an archive legitimately contains regular
     files, directories, and symlinks).
   - **6b** `encryption.Provider`: `DecryptSopsFile`/`EncryptFile` sources and products become `*file.Regular`
     throughout, including both compensators' receipt assertions and `RestoreEncoded`'s rehydration.
   - **6c** `starcode`: the inline `&file.Resource{SourcePath: …}` walk root (an unlinked, never-interned
     contract dodge) is replaced by the `Directory` discovery constructor; the `Reducer` per-entry parameter
     becomes `file.Entry`. The seal makes the bare construction IMPOSSIBLE — that impossibility is a goal of the
     step, not a side effect.
   - **6d** The existence-verification gate enrolls the three variant type ids in place of `…/file.Resource`.
     Must ride the same slice as the minting change — otherwise the pre-flight resolve pass silently stops
     verifying file resources.
   - **6e** The starlark surface renames say what the parameters are: `path` for the primary target (`remove`,
     `remove_all`, `unlink`, `exists`, `is_dir`, `is_file`), `source_path` for `backup`/`link`/`move`,
     `target_path` for `write_file`; `boundary` keeps its name (now a string). Greenfield: no aliases; in-tree
     `.star` callers update in the same slice. `../Personal` is unaffected (2026-07-17): it has layers but no
     lore package dependencies; the old-name→new-name table below documents the rename for any future external
     plans. **Prerequisite**: a working `star` must be installed
     BEFORE the surface changes (`star self install`, or pinning an LKG star build in the Makefile) so the build
     toolchain survives the transition.
8. **Audit findings the rule fixes.** `Remove`/`RemoveAll`/`Unlink` never intern — a slot-coerced (unlinked)
   resource is deleted and archived without the catalog ever seeing it; `WriteFile` takes a `*Resource` and interns
   nothing. Both defects vanish when the mutators take strings and mint internally.

## Open design questions

1. ~~Variant assignment~~ — settled (declared intent, above).
2. ~~String vs. resource parameters~~ — settled (the string-parameter rule, above).
3. ~~Deletion's catalog claim~~ — settled (Discover + mark `Gone`, above).
4. ~~The common currency~~ — settled (embedded-only base + the `file.Entry` interface, above).
5. ~~Per-variant `Digest`/`Etag` semantics~~ — settled (ruling 5, 5a–5e above).
6. ~~Consumer signatures~~ — settled (ruling 6, 6a–6e above).

All six design questions are settled; the slice plan below awaits approval.

## Mid-slice finding (2026-07-18) — variant identity splits the catalog's URI space

Slice 1's cross-kind collision pin exposed a framework deviation. [op.NewResourceBase] mints the canonical URI as
`tag:…:<reach>#<go-type-id>` — the concrete Go type id is the URI **fragment** — and the catalog keys its namespace
on the raw full string. So the same path claimed as `*Regular` and as `*Directory` yields two unrelated catalog
entries: no cross-kind collision, and (the sharper consequence) `Shadow`'s URI-keyed write-write detection would
silently fragment by kind once slice 3's mutators mint variants.

This contradicts the standing identity design,
[architecture/4.1-resource-identity.md](../../../architecture/4.1-resource-identity.md): §2 rules that the fragment
is metadata ("NOT part of the catalog key — the catalog strips the fragment when keying; two URIs that differ only
in fragment resolve to the same resource"), the blessed fragment uses are instance context (mem: node IDs, git:
commit pins), and a file URI is bare RFC 8089. The deviation entered with step 22(k)'s `tag:…#<go-type-id>`
redefinition, which was never reconciled with 4.1 (prior bite: step 44's "`Resource.URI()` is a tag URI
`DiscoverResource` rejects").

**Remedy (rides slice 1): fragment-stripped catalog keying for location-addressed entries** — the 4.1-faithful fix.
The fragment-free payload already exists on [op.ResourceBase] as `specific` (`ReachabilityURI()`). The catalog keys
[op.AddressingLocation] entries on it; the typed assertion in the variant constructors then genuinely detects
cross-kind claims, and write-write detection is whole again. `ResourceType()` dispatch is untouched. Transition
note: until the slice-4 seal, catch-all and variant claims on one path now collide by design — slices 3 and 4 must
convert each flow coherently, never half.

**CAS identity RULED 2026-07-18 (implementation deferred beyond phase 8).** Canonical-form identity: the codec
specifier is metadata, never identity — equal canonical content is one identity, and shadowing yaml with json or
protobuf is sanctioned ("we respect the format": the codec rides as metadata on the entry, and reads/writes honor
it). Implementation — content-hit shadow mechanics in [op.ResourceCatalog.GetOrCreate] plus the protobuf↔canonical
mapping (protojson bridge; never hash protobuf wire bytes) — lands after phase 8. Original framing kept below.

**Deferred beyond phase 8 — CAS fragment keying (json / yaml / protobuf).** Content-addressed URIs
(`tag:…:<algo>:<hex>#<type>`) keep full-string keying for now: stripping would immediately unify entries whose
hashes agree across formats — by 22(k)'s design, semantically-equal YAML and JSON content share a hash (yaml is "an
alternative input rendering of json.Resource"), and protobuf is on the books as a third format. Whether
equal-content-different-format is one catalog identity or several is a real design question; it is parked until
phase 8 completes, together with reconciling architecture/4.1 against 22(k)'s tag-URI redefinition. String-keyed
lookups (`Current`) try the exact key, then the stripped key, so both keying regimes coexist behind one API.
Direction noted 2026-07-18 (decision still post-phase-8): format-neutral identity via canonicalize-then-hash — the
precedent family is RFC 7638/8785, XML C14N, RDF canonicalization, Noms/Unison/Nix, and 22(k)'s own yaml-through-
JSON path; byte-identity systems (git, OCI, IPFS) are counter-precedents whose goals differ — they are storage and
transport for opaque payloads (identity = retrieval + tamper evidence), while this catalog is a semantic ledger for
conflict detection, dependency wiring, drift, and attribution, where "read json, write yaml, identity maintained"
is the point. Protobuf cannot even play byte-identity (no canonical bytes; its own docs warn against hashing wire
form) and joins purely as a codec over the canonical model (protojson bridge).

## Slice plan (approved 2026-07-18)

1. **Slice 1 — COMPLETE 2026-07-18** (additive; suite green; the must-land Merkle deliverable banked, plus the
   mid-slice keying remedy). `file.Entry`; `Regular`/`Directory`/`SymbolicLink` embedding the base (still
   constructible until the seal); per-variant constructors; per-variant `Digest`/`Etag` per 5a–5e including the
   kind-mismatch errors; the Merkle scheme (5c serialization, 5d scope) with tests: identical trees agree,
   content/rename/structure changes flip the root, the empty directory is deterministic, symlinks hash by target, the
   serialization is platform-stable. Nothing consumes the variants yet.
2. **Slice 2 — COMPLETE 2026-07-18**: `op.ResourceCatalog.MarkGone(r)` — the mutator-side counterpart of
   `VerifyExistence`'s discovery-side transition. Any state transitions in (deleting a Pending entry is legal),
   re-marking is idempotent, uncataloged input panics via assert (programming error), Gone stays terminal
   (discovery refused with known-gone), and revival remains a production act (GetOrCreate appends a fresh
   generation; the terminated generation stays Gone in the ledger). Five tests pin the contract.
3. **Slice 3 — the provider audit + the starlark surface** (the breaking slice). PREREQUISITE first: install a
   working `star` (`star self install`, or the Makefile LKG pin). Then: mutators take strings and mint variants
   internally, discharging the ruling-3 invariants (the delete trio interns via Discover, archives to the
   recovery site, marks `Gone`); the query trio takes strings; `WalkTree` takes `*Directory`; `Find`/`Glob`
   return `[]file.Entry`; `Observe` accepts `file.Entry`; the `Reducer` signature changes; the gen surface
   regenerates (the 6e renames); in-tree `.star` callers update; the existence gate enrolls the three variant
   ids (6d rides here); starcode's call-site fix (6c) rides here for greenness; the old-name→new-name table
   lands in this doc.
4. **Slice 4 — consumers + the seal**: archive (6a) and encryption (6b) migrate to the variant constructors;
   `file.NewResource`/`file.DiscoverResource` retire in favor of the per-variant constructors; the base seals
   embedded-only (bare `&file.Resource{…}` construction becomes impossible); a tree-wide sweep confirms no bare
   constructions remain; step and master docs close out.

Each slice is one commit via the user-run script; `make test` green and `gofmt` clean per slice.

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
