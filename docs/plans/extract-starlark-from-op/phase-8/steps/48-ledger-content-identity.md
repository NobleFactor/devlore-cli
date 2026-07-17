---
step: 48
title: "Ledger content identity — record Etag + Digest in the trace's catalog snapshot"
status: COMPLETE 2026-07-16 — capture + consumers landed, repository green; found and fixed en route: the executor discarded the ledger for every non-paused run (Trace.Catalog was pause-only), so completed traces carried no catalog at all
parent: ../../phase-8.md
---

# Step 48 — ledger content identity

**Chartered 2026-07-15** from the deploy-family design round
([writ-deploy-family.md](../writ-deploy-family.md), settled item 8). The drift matrix (upgrade's
source-changed / target-modified attribution; status's Stale vs. Modified classification) needs three data per
entry: source content now and target content now (both live-computable via the sealed `op.Resource` signals) and
**content as deployed** — which nothing records today: `LedgerEntrySnapshot` is `{ID, URI, ProducerID, State}`
(pkg/op/resource_catalog.go:681), and file receipts carry only the pre-archive digest of OVERWRITTEN content.

## Design (settled)

1. **`LedgerEntrySnapshot` gains both tiers** — `Etag string` + `Digest string` (both omitempty; digest in the
   canonical `"<algo>:<hex>"` form, `string` because the DTO needs absent-when-empty and `op.Digest` carries no
   marshaling; `ParseDigest` reads it back strictly).
2. **`ResourceCatalog.Snapshot()` captures them** for Active entries only (Pending has nothing on disk, Gone can't
   be read), best effort (error → field stays empty), at trace-snapshot time — immediately after the run. The
   signals are the sealed interface's own pair: Etag the cheap check (file: one stat — sha256 over the packed
   (size, mtime_ns, inode) tuple, no content read; git: HEAD short-id + dirty-tree suffix); Digest the expensive
   honest one (file: full content read + sha256; git: sha256 over HEAD SHA + dirty stash-create TREE SHA,
   timestamp-free — repo content change is already detectable today).
3. **`Rehydrate` ignores both** — reporting metadata for readback consumers, not rebuild inputs; resume unaffected.
4. **The file DIRECTORY digest gap — RULED 2026-07-16: directories stay digest-less in this step.** The
   snapshot records their Etag (the stat tuple, a shallow add/remove signal) and an empty Digest — the
   best-effort capture handles the `ErrUnimplemented` (pkg/op/provider/file/resource.go:254) as designed. No
   step-48 consumer needs directory content identity (the drift matrix classifies FILE targets; catalog
   directories are mkdir'd parents). The Merkle-root scheme is now an EXPLICIT deliverable of step 23's
   taxonomic split ([23-file-resource-taxonomy.md](23-file-resource-taxonomy.md), amended 2026-07-16 — must land
   before phase-8 closes); when `file.Directory.Digest()` exists, this step's capture populates directory
   digests automatically, no changes here.
5. **Consumer economy** (the catalog's own cascade, `verifyLocationFreshness` resource_catalog.go:507): status
   compares live Etag vs. recorded Etag first — one stat per file, one HEAD read per repo — and computes a live
   Digest only on mismatch; the recorded Digest attributes source-changed vs. target-modified. The cascade's
   comment defers drift surfacing to "a future reconciliation pass" — step 47's readback/status is that pass.
   Both fields are needed: without the recorded Etag every scan full-reads every entry; without the recorded
   Digest an Etag mismatch is unresolvable (touch vs. edit) and unattributable. **Uninformative-default rule**:
   types that don't override `Etag` inherit `ResourceBase`'s default — the URI, a constant — so recorded-vs-live
   would compare equal forever and mask drift; when the recorded Etag equals the entry's URI, the consumer
   bypasses the screen and compares digests directly.

## Scope

1. The two snapshot fields + `Snapshot()` capture + doc updates (`Trace` docs mention the recorded pair).
2. The directory-digest ruling + implementation (or explicit digest-less directory posture).
3. When landed, step 47 flips its interim posture: upgrade attributes instead of skipping-all-differing; status's
   modified-or-stale (indeterminate) rows resolve into Stale vs. Modified.

## Landed (2026-07-16)

1. **The capture**: `LedgerEntrySnapshot` gained `Etag` + `Digest` (omitempty, canonical form);
   `ResourceCatalog.Snapshot()` records both for Active entries, best effort, with the tier I/O moved OUTSIDE
   the catalog mutex (the `verifyLocationFreshness` discipline). Pinned by an in-package probe-resource pair of
   tests (Active records both; digest error → empty; Pending/Gone record neither; json+yaml round trips).
2. **Found and fixed — the trace had NO catalog for completed runs.** `Run`'s teardown snapshotted the ledger
   only when `Phase == PhasePaused`; every completed run tore the environment down and `Trace()` projected a nil
   `Catalog`. The teardown now captures for every outcome — a paused run's trace stays resumable, a completed
   run's trace records the as-deployed identity. (`Trace.Catalog` and executor docs updated.)
3. **The consumers flipped** (the step-47 interim retires):
   - readback: `Entry` gained `RecordedEtag` / `RecordedDigest` / `RecordedSourceDigest`, extracted from the
     trace catalog by file-URI path (`recordedIdentity`); `ContentDigest` exported for consumers.
   - upgrade: full attribution — a stale target (digest equals the recorded identity, source moved) regenerates
     WITHOUT `--force`; a locally-modified target force-gates; encrypted chains attribute through the recorded
     SOURCE digest (the encrypted bytes hash without decrypting); pre-capture runs stay indeterminate.
   - status: `StateStale` (repair `writ upgrade`) and `StateModified` (repair `writ upgrade --force`) split out
     of the indeterminate `StateModifiedOrStale`, which remains for pre-capture runs.
   Pinned by the flipped writ integration tests (stale-regenerates-freely, modified-is-force-gated, the
   stale/modified status classifications).
4. Directories per the ruling: Etag recorded, Digest empty until step 23's Merkle deliverable populates it
   automatically.

## Test plan

1. Snapshot round-trip (json + yaml): Active entries carry etag + digest; Pending/Gone entries carry neither;
   a digest-erroring resource records an empty field, not a failure.
2. Per-type presence: file (content hash), git (HEAD-based; repo edit → digest change; touch → etag change only).
3. Rehydrate: identical behavior with and without the recorded fields.
4. The directory case per its ruling.
