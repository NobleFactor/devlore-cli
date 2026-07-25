---
step: 46
title: "Graph signing + writ verify — build pkg/signing, settle the scheme, ship the command"
status: COMPLETE 2026-07-16 — the default tier landed end to end (pkg/signing, sign-at-persist, writ verify, the policy ladder); deferred tiers recorded below (ssh-agent/KMS/keyless custody, cert-authority trust, the status/decommission gate wiring, the config surface awaiting the loader)
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

## Landed (2026-07-16) — the default tier

1. **`pkg/signing`**: the small [Signer] seam; `DefaultSigner` = `~/.ssh/id_ed25519` (unencrypted Ed25519) with
   the generated local keyfile fallback (`<config>/devlore/signing/ed25519`, OpenSSH PEM + `.pub` in
   authorized_keys form for trust seeding; XDG-consistent on every platform — `os.UserConfigDir` diverges on
   darwin); raw `ssh-ed25519` signatures over `namespace ‖ CanonicalContent`; `Verify` → the four-outcome
   Verdict (valid / unsigned / invalid / untrusted); the devlore-parsed `allowed_signers` (principals,
   `namespaces=` globs, validity windows; `cert-authority` recognized-but-deferred); the `Policy` ladder with
   `Judge` as the one enforcement point; `External` = the store boundary. Seven unit tests including
   cross-namespace domain-separation and the snake_case vocabulary pin.
2. **The op seams**: `Signature` docs carry the settled SSH ciphersuite names; `Graph.SignWith` (single-shot,
   crypto-free, checksum-unaffected); `Trace.Signature` + `Trace.SignWith` + `Trace.CanonicalContent`.
3. **Sign-at-persist**: `cli.WriteGraph` / `cli.WriteTrace` sign unsigned artifacts best effort — no signer
   resolvable → the artifact writes unsigned and verification reports the fact (the report-tier posture).
   Every writ integration fixture became key-hermetic (HOME/XDG_CONFIG_HOME sandboxed), so the suite generates
   sandbox keys and exercises real signing on every store write.
4. **`writ verify <document>...`** with `--signing-policy` (floor `report`), `--allowed-signers`, `--json`;
   graph documents load through the sealed integrity-checked path (a checksum mismatch reports as invalid);
   exit non-zero only on policy rejection. Three integration tests: the signed store verifies valid under
   `reject`; a tampered external copy rejects under `reject_external` and reports under `report`; unsigned
   reports under the floor and rejects under `reject`. The old age-remnant helper (`cmd/writ/writ/verify.go`)
   is deleted.
5. **Finding — trace canonicalization is document-form, not struct-form.** The live trace's typed values are
   not a marshal fixed point with their decoded forms (the stack's custom unmarshaling is lossy), so
   `Trace.CanonicalContent` canonicalizes through the generic document form (marshal → strip `signature` →
   re-marshal) and verification canonicalizes the RAW file bytes (`signing.CanonicalDocument`) — one signature
   verifies on both sides. Graphs keep their hand-built round-trip-stable canonical.

## Deferred (recorded)

1. Custody tiers: ssh-agent (FIDO/PIV), cloud KMS, Vault/OpenBao, sigstore keyless — the opt-in matrix.
2. `cert-authority` trust-list lines (SSH-certificate model).
3. The status store-health signature report and decommission's signed-state gate — the writ consumers wire in
   a follow-up round now that the verdicts exist.
4. The `signing` config section (`backend`/`key`/`allowed_signers`/`policy`) — rides the config loader
   (config-plan item 7); until then the defaults need no configuration and `writ verify` takes flags.
5. The protobuf canonicalization path (graph-signing.md's open item) — no protobuf serialization exists yet.

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

## Design questions — ALL CLOSED 2026-07-16

The charter's questions were framed from the code's remnants; the pre-existing design documents —
[graph-signing.md](../graph-signing.md) (the data-layer mechanism) and
[signing-options.md](../signing-options.md) (the signing model) — are the design of record and settle 1–3:

1. **Scheme — settled by the design docs**: `ssh-ed25519` (Ed25519 under OpenSSH key-type naming; ecdsa/rsa SSH
   suites acceptable), a RAW signature over `namespace ‖ CanonicalContent` (`devlore.graph.v1` /
   `devlore.trace.v1` domain separation), `public_key` as the OpenSSH wire blob. No envelope, no hash options.
   The inherited age construction was never part of the design (an encryption tool misused — anyone holding the
   public recipient can forge it) and dies with the implementation; `signature.go`'s drifted doc comment
   ("ed25519"/"ecdsa-p256") gets the settled SSH names.
2. **What gets signed — settled: graphs AND traces**, each under its own namespace. Mechanical gap: `op.Trace`
   carries no `Signature` field yet; it gains one.
3. **Identity/key source — settled**: the developer's SSH key (`~/.ssh/id_ed25519`, ssh-agent incl. FIDO/PIV)
   as the default; a generated local Ed25519 keyfile as the fallback; verifier-side trust via OpenSSH's
   `allowed_signers` file format, parsed by devlore. NOT age identities (explicitly flagged unrelated), NOT the
   sops client (signing left pkg/sops per sops-config-discovery.md). KMS/keyless are opt-in later tiers.
4. **Verification policy — ruled 2026-07-16** (supersedes the "unsigned severity" framing): one setting,
   `signing.policy` ∈ { `ignore`, `report`, `reject_external`, `reject` }, floor `report`; the store boundary is
   the externality marker; one enforcement point in `pkg/signing`; prior art and the full table in
   [signing-options.md §"The verification policy"](../signing-options.md).
