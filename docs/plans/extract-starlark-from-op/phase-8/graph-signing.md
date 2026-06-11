---
title: graph signing (pkg/signing) — data-layer signing across json/yaml/protobuf
status: draft
created: 2026-06-11
updated: 2026-06-11
---

## Summary

`pkg/signing` provides graph-provenance signing and verification — a concern **separate from secret encryption**
(sops). Graphs will serialize to **three** formats — JSON, YAML, and Protobuf — so a signature cannot cover the
serialized *file*: the same graph is a different byte stream in each format. It must cover the **canonical data
representation** instead. Sign the canonical JSON bytes the graph already produces, embed the signature in each
format's envelope, and verify by re-canonicalizing. **One signature verifies in any format.**

## Why not sign the file

A graph stored as JSON vs YAML vs Protobuf is three different byte streams. A signature over the bytes validates only
the one format it was produced from — a JSON-only consumer could not verify a YAML-stored graph, and vice versa.
Signing the format-independent canonical data makes the signature portable across all three.

## Strategy — data-layer signing

```
[op.Graph] ──> [canonical JSON bytes] ──> [sign] ──> Signature
                                              │
       serialize to JSON  ──> embed signature ┤
       serialize to YAML  ──> embed signature ┤
       serialize to Proto ──> embed signature ┘
```

We already have the canonicalization layer (json/yaml canonicalize-at-construction — phase-8 13.0(k) k.8/k.9), so the
hard part — deterministic bytes — is done. Sign those bytes; never sign the rendered file.

## Algorithm — stdlib, no hand-rolled crypto

- **Ed25519** (`crypto/ed25519`) — recommended default. Fast, small 64-byte signatures, side-channel resistant, zero
  external dependencies.
  - Sign: `ed25519.Sign(privateKey, canonicalBytes)`
  - Verify: `ed25519.Verify(publicKey, canonicalBytes, signature)`
- **ECDSA P-256** (`crypto/ecdsa`) — for FIPS / enterprise compliance.
  - Sign: `ecdsa.SignASN1(rand.Reader, privateKey, sha256Hash)`
  - Verify: `ecdsa.VerifyASN1(publicKey, sha256Hash, signature)`

This replaces the hand-rolled KMS/GPG sign backends with the Go standard library — consistent with the "don't
hand-roll crypto" principle. (See the open questions on whether KMS custody must still be supported.)

## The envelope (carries the signature across all three formats)

```go
type SignedGraphEnvelope struct {
    Graph     op.Graph `json:"graph"                yaml:"graph"                protobuf:"bytes,1,opt,name=graph"`
    Signature []byte   `json:"signature"            yaml:"signature"            protobuf:"bytes,2,opt,name=signature"`
    PublicKey []byte   `json:"public_key,omitempty" yaml:"public_key,omitempty" protobuf:"bytes,3,opt,name=public_key"`
}
```

- **JSON / YAML:** a `[]byte` encodes to a base64 string automatically — clean and safe.
- **Protobuf:** a `bytes` field — raw, no base64, compact.

```proto
message SignedGraphEnvelope {
    ExecutionGraph graph = 1;
    bytes signature = 2;
    bytes public_key = 3;
}
```

## Verify (any format)

1. Unmarshal the file into `SignedGraphEnvelope` with the format's decoder.
2. Re-canonicalize `envelope.Graph` to the deterministic JSON bytes.
3. `ed25519.Verify(envelope.PublicKey, canonicalBytes, envelope.Signature)` (or `ecdsa.VerifyASN1`).

## Open questions (the draft does not resolve these)

- **Key custody — the real fork.** Ed25519/ECDSA over canonical bytes uses a **local** private key. The old approach
  kept the key in cloud KMS / a GPG keyring (custody in an HSM / managed service). Local stdlib keys are simpler and
  format-agnostic but move private-key custody onto the host. Decide: stdlib local keys (the draft), keep a
  KMS/GPG-backed signer for HSM custody, or support both behind one `Signer` interface.
- **Fate of the KMS/GPG backends.** If signing is stdlib Ed25519/ECDSA, the `aws_kms` / `azure_kv` / `gcp_kms` /
  `gpg` backends are **deleted, not moved** into `pkg/signing` — they have no role. This **supersedes** the earlier
  "move the backends into pkg/signing" note in `sops-config-discovery.md`. Confirm.
- **Signing-key source.** Signing does not use `.sops.yaml`; `pkg/signing` needs its own key configuration — a key
  file path, an env var, generated-and-stored on first use? To design.
- **`PublicKey` in the envelope — trust model.** Embedding the public key makes the envelope self-verifying *for
  integrity* (the bytes weren't altered), but a self-embedded key proves nothing about *who* signed unless the
  verifier trusts that key out of band (a pinned/known key, a cert chain, a keyring). Decide the trust anchor —
  otherwise the signature only detects accidental corruption, not forgery.
- **Canonicalization reuse + the protobuf path.** Confirm the existing json/yaml canonicalizer produces exactly the
  bytes signing needs, and that protobuf-decoded graphs canonicalize through the **same** JSON path — otherwise the
  three formats would not share one signature.

## Status

- Design: **draft** — from the data-layer-signing starting point; open questions above unresolved.
- Implementation: **not started.**

## Related

- [`sops-config-discovery.md`](sops-config-discovery.md) — the sops/signing split (signing is not a sops concern;
  getsops has no signing capability — verified).
- phase-8 13.0(k) k.8/k.9 — the json/yaml canonicalization layer this builds on.
