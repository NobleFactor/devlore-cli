---
title: "writ verify — command plan & design"
status: draft — awaiting review (largest open-question surface of the three: no standalone command exists today)
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
2. What exists is the helper `VerifyGraphSignature(g, identities)` (`cmd/writ/writ/verify.go`) with its
   `VerifyResult` enum. It reads `g.Signature` as a **field** — dead API; the sealed graph exposes
   `Graph.Signature()` and `Graph.CanonicalContent()` methods — so the file is one of the masked build reds.
3. **Sole consumer**: `verifyGraphSignatureForReconcile` (`commands.go`) — the reconcile command (deploy-family,
   slice C) verifies a loaded receipt-graph before reconciling; invalid → "redeploy to regenerate".
4. **Signing today**: `AssembleDefinition` signs the graph when the `GraphSpec` carries a `SopsClient`; a nil client
   leaves it unsigned, and sub-graphs are left unsigned "pending the sops rewrite". Identity loading comes from
   `internal/identity.LoadIdentities`.

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

## Open questions

1. **Is verify a user-facing command at all**, or a library capability consumed by deploy/reconcile (slice C)? The
   direction to produce this design doc suggests command; confirm the surface (`writ verify <file>...`?).
2. **What does it verify?** Graph documents only, or also persisted traces/receipts (`cli.WriteTrace` output)?
   Traces are not signed today — signing them would be new design (does the receipt inherit the graph's signature
   story, or get its own?).
3. **The signing story is half-built** ("sub-graphs unsigned pending the sops rewrite"; `SopsClient` propagation).
   Is completing signing in scope for this command's step, or does verify ship against the current
   root-graph-only signing?
4. **`unsigned` severity default** — informational (current reconcile behavior: note + continue) with `--strict`
   opt-in, or fail-by-default?
5. **Identity source** — `internal/identity.LoadIdentities` (the current age-identities loader) assumed; any move
   toward the SOPS client as the one signing/verifying surface?
6. **Sequencing** — verify is not part of slices A/B/D's build-red closure (the helper fix is; the command is new
   surface). Does the command land inside step 33's slice D, or as its own step after C's spec?
