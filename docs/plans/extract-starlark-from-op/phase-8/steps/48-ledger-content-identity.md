---
step: 48
title: "Ledger content identity — record Etag + Digest in the trace's catalog snapshot"
status: in-progress 2026-07-16 — design fully settled (the directory ruling closed the last open point: digest-less until step 23's Merkle deliverable); implementation next
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

## Test plan

1. Snapshot round-trip (json + yaml): Active entries carry etag + digest; Pending/Gone entries carry neither;
   a digest-erroring resource records an empty field, not a failure.
2. Per-type presence: file (content hash), git (HEAD-based; repo edit → digest change; touch → etag change only).
3. Rehydrate: identical behavior with and without the recorded fields.
4. The directory case per its ruling.
