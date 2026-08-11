---
title: "Writ Secret Recover"
status: proposed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Writ Secret Recover

## Summary

`writ secret recover` — break-glass decryption of SOPS documents using raw held key
material, when no configured keyservice can run (vault purged, subscription lapsed,
account dead). Chartered 2026-08-10 by the owner from the key-custody design's BYOK
class-ii recovery runbook (personal layer, `design/secrets-key-custody.md`): the envelope
model makes recovery possible with the held RSA private key, but no off-the-shelf sops
input accepts a raw PEM — only age identities are native. The primary need arises under a
BYOK custody ruling; the charter is unconditional and the age path rides along for
symmetry. Architecture home:
[3.5.13-encryption-provider](../architecture/3.5.13-encryption-provider.md) § Key custody
and break-glass recovery.

## Design

1. **Command**: `writ secret recover --key <material-file> [--stdout] <file>...` under
   the `writ secret` family beside [encrypt](writ-secret-encrypt.md) and
   [rekey](writ-secret-rekey.md). Without `--stdout`, each `<name>.sops` recovers to the
   `<name>` sibling, refusing an existing destination.
2. **Pipeline-free by design.** No graph, no trace, no execution store, no provider
   dispatch — disaster tooling must not assume the machine's writ state is healthy. The
   command calls `pkg/sops` directly; this deliberately differs from `encrypt` (which
   rides the standard pipeline) and the difference is documented in 3.5.13.
3. **Mechanics**, per document: parse the SOPS metadata; select the wrapped data-key
   entry matching the supplied material — an RSA private key (PEM) unwraps an
   `azure_kv` entry locally via RSA-OAEP-256; an age identity delegates to the native
   age path; then the recovered data key decrypts the content through the getsops store
   machinery and the plaintext is emitted. Nothing touches the network.
4. **`pkg/sops` growth**: a `Recover` entry point (material + document bytes + format →
   plaintext). The encryption provider's API is unchanged — recover sits beside it, not
   inside it.

## Verification

1. In-process fixtures constructing `azure_kv`-shaped metadata with a locally generated
   RSA key (the pattern the provider tests use for age): PEM round-trip, age round-trip,
   wrong-material refusal, existing-destination refusal.
2. `make test`; dual-GOOS lint recount at zero.
3. The custody design's drill requirement exercises this command for real under a BYOK
   ruling before the migration completes.

## Sequencing

After [writ-secret-encrypt](writ-secret-encrypt.md) lands (shared `writ secret` command
scaffolding); independent of rekey. Activated for the drill by the custody ruling.

## Open questions

1. Whether a `--data-key <hex>` last-resort input (raw recovered DK) belongs in the
   first delivery or waits for a demonstrated need.

## Amendment (2026-08-10)

Multi-cloud (ruled): recovery material differs per keysource, so the local unwrap grows
one adapter each — `azure_kv`: RSA-OAEP-256 with the held PEM (the original scope);
`kms` (AWS): the imported external key material (AES or RSA per import form); `gcp_kms`:
the import-job material. The recovery bundle's provider block records which form a key
uses. Verification items before implementing the AWS/GCP adapters: the exact wrap
algorithms sops applies against asymmetric imports on each cloud.

**Scope exemption** (ruled 2026-08-10): recover is deliberately NOT layer-scoped. Its
founding property — no assumption that the machine's writ state is healthy — includes
the layers registry itself; a fresh machine holding one blob and one bundle must be able
to recover with no registration at all. Arbitrary paths stay accepted here even though
init, encrypt, decrypt, and rekey refuse them.
