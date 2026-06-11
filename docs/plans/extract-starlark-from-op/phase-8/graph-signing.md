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

## Signing — AWS KMS asymmetric (Option 1, chosen)

The signing key lives in **AWS KMS** and **never leaves the HSM** — that is the key-custody decision. The app
canonicalizes the artifact, computes a SHA-256 digest, and calls KMS `Sign` with the *digest only*; KMS signs inside
the HSM and returns the raw signature bytes. The private key cannot be exported, so a compromised host can verify but
cannot forge.

- **Key spec:** `ECC_NIST_P256`, usage Sign/Verify; signing algorithm `ECDSA_SHA_256` (RSA_2048 only if compliance
  demands it). `Signature.Algorithm = "ecdsa-p256"`, `Signature.Value` = the ASN.1-DER ECDSA signature.
- **Offline verify:** fetch the verifying key once via KMS `GetPublicKey`, embed it as `Signature.PublicKey`, and
  verification runs **offline** (`ecdsa.VerifyASN1`) against the embedded key — no KMS call on the read path.
- **Cost/latency:** one KMS API call per signature (network latency, per-call fee, rate limits). Fine while signing
  is per-plan/per-trace, not per-file in a tight loop.

```go
import (
	"context"
	"crypto/sha256"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

func signWithKMS(ctx context.Context, canonical []byte, keyArn string) ([]byte, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(canonical)
	out, err := kms.NewFromConfig(cfg).Sign(ctx, &kms.SignInput{
		KeyId:            &keyArn,
		Message:          digest[:],
		MessageType:      types.MessageTypeDigest, // we send a pre-computed hash
		SigningAlgorithm: types.SigningAlgorithmSpecEcdsaSha256,
	})
	if err != nil {
		return nil, err
	}
	return out.Signature, nil
}
```

### Option 2 — local key (throughput fallback, not chosen)

If KMS per-signature latency/cost ever becomes a real bottleneck (high-volume signing), generate an Ed25519 key,
store it in AWS Secrets Manager, fetch it once at startup, and sign in-process with `crypto/ed25519`. Faster and free
per signature, but the private key lives in the app's RAM — a compromised host can exfiltrate it. Adopt only if a
profiler proves KMS is the bottleneck. (`Signature` accommodates both: `Algorithm = "ed25519"`, 64-byte `Value`.)

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

## Resolved

- **Key custody → AWS KMS asymmetric (Option 1).** The private key stays in the HSM; signing is a KMS `Sign` call over
  the SHA-256 digest. See the section above. Option 2 (local Ed25519 via Secrets Manager) is the documented
  throughput fallback, not the default.
- **Signing-key source.** A KMS key ARN (own configuration, not `.sops.yaml`). The old hand-rolled
  `aws_kms`/`azure_kv`/`gcp_kms`/`gpg` sops backends are **deleted** — `pkg/signing` calls KMS via the AWS SDK v2
  directly, not via those.

## Open questions

- **Trust anchor (still open).** Embedding the KMS public key makes the envelope self-verifying *for integrity*, and
  because the KMS key can't be stolen a valid signature proves it came from *that* key. But the verifier must still
  **pin the expected key** (KMS key ARN, or its public key) out of band — otherwise an attacker re-signs with *their*
  KMS key and embeds *their* public key. KMS solves key *custody*, not key *trust*. Decide the pinning mechanism.
- **Canonicalization reuse + the protobuf path.** Confirm the existing json/yaml canonicalizer produces exactly the
  bytes signing needs, and that protobuf-decoded graphs canonicalize through the **same** JSON path — otherwise the
  three formats would not share one signature.
- **Standard envelope vs. hand-roll.** The `Signature` struct is hand-rolled. When this is built, evaluate **DSSE**
  (in-toto/sigstore — purpose-built for provenance, algorithm-agnostic) or **JWS**/**COSE** before extending the
  struct by hand.

## Status

- Design: **draft** — from the data-layer-signing starting point; open questions above unresolved.
- Implementation: **not started.**

## Related

- [`sops-config-discovery.md`](sops-config-discovery.md) — the sops/signing split (signing is not a sops concern;
  getsops has no signing capability — verified).
- phase-8 13.0(k) k.8/k.9 — the json/yaml canonicalization layer this builds on.
