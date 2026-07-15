---
step: 46
title: "Graph signing + writ verify — build pkg/signing, settle the scheme, ship the command"
status: chartered 2026-07-15 (out of step 33 slice D) — sequenced AFTER slice C (reconcile, the sole verify consumer, lives in the crater); design questions below settle before code
parent: ../../phase-8.md
---

# Step 46 — graph signing + `writ verify`

**Chartered 2026-07-15** out of step 33 slice D (ruling: a verify command without a signer can only ever report
`unsigned`, and building the signer is framework work that cannot ride a writ command slice — charter the pair as
one step). Design doc: [writ-verify-command.md](../writ-verify-command.md) — its Current state section carries the
verified 2026-07-15 fact set.

## Facts (verified 2026-07-15, recorded in the design doc)

1. **Nothing signs graphs.** No `op.Signature` value is constructed anywhere in the repo. `op.NewGraph` defers:
   "signing proper lives in pkg/signing" (`pkg/op/graph.go:109`) — a package that does not exist. The `GraphSpec`
   "SopsClient" mention in that doc comment is a dangling reference (no such field). The only path populating
   `Graph.signature` is the document load path preserving a signature nothing can have produced.
2. **Scheme mismatch.** The sealed `op.Signature.Algorithm` declares `"ed25519"` / `"ecdsa-p256"`
   (`pkg/op/signature.go:9`); the kept `VerifyGraphSignature` logic (`cmd/writ/writ/verify.go`, sealed-API fix
   landed in slice D) expects `"age"` — an age-encrypted SHA-256 over `Graph.CanonicalContent()`. One story wins.
3. **Sole consumer today**: `verifyGraphSignatureForReconcile` (`cmd/writ/writ/commands.go`) — the reconcile
   command, deploy-family, slice C.

## Scope

1. **`pkg/signing`** — the framework signer at the home the sealed graph docs already declare: sign
   `Graph.CanonicalContent()`, populate `Graph.signature` via whatever construction seam the sealed graph offers
   the load/signing path, and verify. `Trace`/receipt signing rides the same scheme decision (design question 2).
2. **Settle the scheme** — ed25519/ecdsa-p256 (the sealed declaration) vs. age (the helper's inherited
   expectation); rework `VerifyGraphSignature` (or subsume it into `pkg/signing`) accordingly, and fix the
   dangling `SopsClient` doc reference on `op.NewGraph` to describe the real seam.
3. **`writ verify <graph-document>...`** — per-document report: checksum status (the load path already refuses a
   mismatch), signature result (`valid` / `unsigned` / `invalid` / `missing`), origin stamp; exit non-zero on any
   `invalid` (severity of `unsigned` = design question 4).
4. **Reconcile integration** — `verifyGraphSignatureForReconcile` calls the same surface (lands with/after slice C).

## Design questions (settle one at a time before code)

Inherited from the design doc's open questions 2–5; question 1 (command vs. library) and question 6 (sequencing)
were settled by the charter ruling: standalone command, this step, after slice C.

1. **Scheme** — ed25519/ecdsa-p256 per the sealed declaration, or age per the inherited helper? (Key management
   story differs: age identities already exist via `cmd/writ/writ/identity.LoadIdentities`; ed25519 needs a key
   home.)
2. **What gets signed/verified** — graph documents only, or persisted traces/receipts too (`cli.WriteTrace`
   output)? Traces are unsigned today; signing them is new design.
3. **Identity/key source** — `identity.LoadIdentities` (age identities), or the SOPS client as the one
   signing/verifying surface?
4. **`unsigned` severity** — informational with `--strict` opt-in (current reconcile behavior), or
   fail-by-default?
