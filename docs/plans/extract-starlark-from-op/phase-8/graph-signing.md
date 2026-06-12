---
title: artifact signing (pkg/signing) — graphs + execution traces, across json/yaml/protobuf
status: draft
created: 2026-06-11
updated: 2026-06-11
---

## Summary

`pkg/signing` provides provenance signing and verification for the **two** signable op artifacts — the **graph**
(the plan) and its **execution trace** (the run record) — a concern **separate from secret encryption** (sops). Both
serialize to **three** formats (JSON, YAML, Protobuf), so a signature cannot cover the serialized *file*: the same
artifact is a different byte stream in each format. It covers the **canonical data representation** instead — sign the
canonical bytes the artifact already produces, carry the signature in the document's **`signature` field**, and verify
by re-canonicalizing. **One signature verifies in any format.**

This doc covers the **data-layer mechanism**. The **signing model** that rides on it — publisher identity, the backend
matrix, the `op.Signature` field contents, the trust model, and the us-vs-sigstore split — is settled in
[`signing-options.md`](signing-options.md). The shared `op.Signature` type (`pkg/op/signature.go`) covers both graph
and trace.

## Why not sign the file

A graph stored as JSON vs YAML vs Protobuf is three different byte streams. A signature over the bytes validates only
the one format it was produced from — a JSON-only consumer could not verify a YAML-stored graph, and vice versa.
Signing the format-independent canonical data makes the signature portable across all three.

## Strategy — data-layer signing

```
[op.Graph] ──> [canonical bytes] ──> [sign over namespace‖bytes] ──> op.Signature (inline field)
                                              │
       serialize to JSON  ──> signature field ┤
       serialize to YAML  ──> signature field ┤
       serialize to Proto ──> signature field ┘
```

We already have the canonicalization layer (json/yaml canonicalize-at-construction — phase-8 13.0(k) k.8/k.9), so the
hard part — deterministic bytes — is done. Sign those bytes; never sign the rendered file. The bytes signed are
`CanonicalContent` — the graph serialized **without `checksum` and `signature`** (`pkg/op/graph.go:490`) — prefixed
with a fixed namespace (`devlore.graph.v1` / `devlore.trace.v1`) for domain separation.

## The signature is an inline field, not a wrapping envelope

The signature rides **inside** the artifact document as the `op.Signature` field the graph already carries
(`Graph.signature`, `pkg/op/graph.go:57`) — **not** a separate `SignedXEnvelope` struct wrapping it, and **not** a
DSSE/JWS/COSE envelope. `op.Signature.Value`/`PublicKey` (`[]byte`) encode to base64 in JSON/YAML and raw `bytes` in
protobuf. The field's three-part contents and the reasoning (no hash field; publisher key in `PublicKey`; trust
verifier-side) are settled in [`signing-options.md`](signing-options.md).

## Verify (any format)

1. Decode the file (any format) into the artifact, including its `signature` field.
2. Re-canonicalize the artifact to the deterministic bytes — **excluding `checksum` and `signature`** — and prefix the
   namespace.
3. Verify `signature.value` against `signature.public_key` over those bytes (the primitive named by
   `signature.algorithm` — e.g. `ed25519.Verify` / `ecdsa.VerifyASN1`). This establishes **integrity**.
4. Resolve `signature.public_key` against the verifier's **`allowed_signers`** (out of band) to a trusted principal.
   This establishes **publisher authenticity**. The document never carries the trust list.

## Resolved

- **Signing is pluggable, not single-vendor.** The original "AWS KMS Option 1" framing is **superseded**: **SSH-key
  signing is the no-cloud default**; cloud KMS, self-hosted Vault/OpenBao, and OIDC keyless are opt-in backends. The
  full matrix and the us-vs-sigstore lane split are in [`signing-options.md`](signing-options.md).
- **Trust anchor.** Resolved per backend, **verifier-side**: OpenSSH `allowed_signers` (key → principal) for every
  backend that normalizes to SSH conventions; keyless carries its own Fulcio/OIDC/Rekor trust path. The document stores
  the signing key, never the trust list.
- **Envelope.** Resolved: **none** — a detached `op.Signature` inline field. (DSSE would matter only if graphs/traces
  had to interoperate as SLSA/in-toto attestations — a different goal.)
- **Hash.** Resolved: **no options** — the digest is intrinsic to the `algorithm` ciphersuite (Ed25519 ⟹ SHA-512, etc.);
  a negotiable hash buys no agility and invites downgrade attacks.

## Open questions

- **Canonicalization reuse + the protobuf path.** Confirm the existing json/yaml canonicalizer produces exactly the
  bytes signing needs, and that protobuf-decoded graphs canonicalize through the **same** path — otherwise the three
  formats would not share one signature.
- **Configuration + CLI surface.** How `signing.backend` + per-backend config reach `pkg/signing` through
  `application.Application.Config`, and the command-line flags — next design step.

## Status

- Design: model **settled** (see [`signing-options.md`](signing-options.md)); the data-layer mechanism here is stable.
  Canonicalization/protobuf confirmation + the config/CLI surface remain.
- Implementation: **not started.**

## Related

- [`signing-options.md`](signing-options.md) — the settled signing model: backends, signature field, trust, sigstore role.
- [`sops-config-discovery.md`](sops-config-discovery.md) — the sops/signing split (signing is not a sops concern;
  getsops has no signing capability — verified).
- phase-8 13.0(k) k.8/k.9 — the json/yaml canonicalization layer this builds on.
