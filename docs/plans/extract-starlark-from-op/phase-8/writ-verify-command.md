---
title: "writ verify — command plan & design"
status: settled 2026-07-15 — helper fix LANDED (step 33 slice D); the command + signer are CHARTERED as step 46 (after slice C); the remaining design questions move there
created: 2026-07-15
parent: steps/33-writ-migrate-rewrite.md
---

# writ verify — plan & design

## Purpose

Verify the authenticity of a graph document: the signature is an age-encrypted SHA-256 over the graph's canonical
content (`Graph.CanonicalContent()` — the serialized graph minus checksum and signature). Verification decrypts the
signature with the operator's age identities and compares hashes. Four outcomes: `valid`, `unsigned`, `invalid`,
`missing`.

## Current state

1. **No `writ verify` command exists.** `root.go` registers deploy, decommission, reconcile, upgrade, adopt, list,
   receipt, inspect, migrate — no verify.
2. The helper `VerifyGraphSignature(g, identities)` (`cmd/writ/writ/verify.go`) with its `VerifyResult` enum —
   **fixed onto the sealed API in slice D (2026-07-15)**: reads `Graph.Signature()` (accessor, formerly a dead field
   access), matches on `Signature.Algorithm` (formerly `.Method`), and decrypts `Signature.Value` directly (raw
   `[]byte` per the sealed shape; the base64-decode step is gone). The age-decrypt logic itself is kept verbatim —
   rewiring, not redesign.
3. **Sole consumer**: `verifyGraphSignatureForReconcile` (`commands.go`) — the reconcile command (deploy-family,
   slice C) verifies a loaded receipt-graph before reconciling; invalid → "redeploy to regenerate".
4. **Nothing signs graphs today — verified 2026-07-15.** No `op.Signature` value is constructed anywhere in the
   repo. `op.NewGraph` explicitly does not sign ("signing proper lives in pkg/signing" — a package that **does not
   exist**); the `GraphSpec` "SopsClient" mention in that doc comment is a dangling reference (no such field). The
   only path that populates `Graph.signature` is the document load path preserving a signature that nothing can
   have produced. The prior claim here ("`AssembleDefinition` signs via `SopsClient`") described the ancient
   framework, not the sealed one.
5. **Scheme mismatch.** The sealed `op.Signature.Algorithm` declares `"ed25519"` / `"ecdsa-p256"`; the kept helper
   logic expects `"age"` (an age-encrypted SHA-256 of `Graph.CanonicalContent()`). One of the two stories must win
   when signing is built. Identity loading today is `cmd/writ/writ/identity.LoadIdentities` (age identities).

## Target design (proposal — the open questions govern)

1. **Fix the helper onto the sealed API** (mechanical): `g.Signature()` accessor; keep the `VerifyResult` vocabulary;
   move the helper into a home that survives slice C (it must not be stranded in the deploy-family rubble).
2. **Add the standalone command**:

   ```
   writ verify <graph-document>...
   ```

   Loads each document (`op.LoadGraph`), reports per-document: checksum status (the load already refuses a
   checksum-mismatched document), signature result (`valid` / `unsigned` / `invalid` / `missing`), and the origin
   stamp (tool / project / scope). Exit non-zero when any document is `invalid` (and, under `--strict`, when
   `unsigned`).
3. **Reconcile integration unchanged in shape** — when slice C is specced, reconcile calls the same helper.

## Deletions

None beyond the dead field access — the rewrite is additive (the command) plus mechanical (the sealed accessor).

## Test plan

1. Round-trip: a signed graph document verifies `valid`; a tampered body verifies `invalid`; a stripped signature
   reports `unsigned`; wrong identities report `invalid`.
2. Command-level: exit codes per outcome; `--strict` flips `unsigned` to failure; multi-document invocation.

## Settled (2026-07-15)

1. **The command does not land in slice D.** A verify command without a signer can only ever report `unsigned`,
   and building the signer is framework work (`pkg/signing`, the scheme decision, the graph-side seam) that cannot
   ride a writ command slice. Both were the original open questions 1 (command vs. library — command) and 6
   (sequencing).
2. **Step 46 chartered** — [steps/46-graph-signing-and-verify.md](steps/46-graph-signing-and-verify.md): build
   `pkg/signing` per the sealed declaration, settle the scheme, ship `writ verify`, wire reconcile; sequenced
   after slice C (reconcile, the sole consumer, lives in the crater).
3. **Slice D's share landed**: `VerifyGraphSignature` on the sealed API (Current state 2), logic verbatim.

## Open questions — moved to step 46

The remaining design questions (what gets signed/verified — graphs only or traces too; the scheme —
ed25519/ecdsa-p256 vs. age; identity/key source; `unsigned` severity default) are carried as step 46's design
questions, to settle one at a time before that step's code.
