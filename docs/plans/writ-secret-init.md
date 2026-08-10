---
title: "Writ Secret Init"
status: proposed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Writ Secret Init

## Summary

`writ secret init` — the BYOK ceremony, the family's fourth member (named 2026-08-10;
`init` over `create`/`key`/`provision` — the terraform-shaped precedent: make this
repository ready to work against remote provider state). One run per key: generate the
RSA-4096 keypair locally; import it to the target cloud KMS; emit the custody artifacts
— the passphrase-encrypted PEM blob (age, scrypt) and the recovery bundle — and seed the
governing `.sops.yaml` recipient. The custody boundary is the owner's ruling made
mechanical: writ emits blob and bundle to `--out-dir`; replication to the owner's sites
is the owner's. Architecture home:
[3.5.13](../architecture/3.5.13-encryption-provider.md) § Key custody and break-glass
recovery.

## Ruled requirements

1. **Deletion protection is a hard gate, no bypass flag.** Azure: refuse a vault
   without soft-delete + purge protection, naming what's missing and the `az` command
   that fixes it. When AWS/GCP land: AWS's 7–30 day deletion window is verified
   (platform-enforced); GCP requires a destroy-scheduled-duration floor.
2. **Three-cloud support** (AWS KMS, Azure Key Vault, GCP KMS) is the product
   requirement; v1 implements Azure behind a provider-neutral design. Import ceremonies
   differ per cloud (Azure: PEM import; AWS: external-key-material wrap ceremony; GCP:
   import jobs) and each is its own follow-on delivery.
3. **The passphrase is never stored** — double interactive prompt; no flag, no
   environment variable, no history.

## Design

1. **Surface (v1, Azure)**: `writ secret init --vault <name> [--key-name <name>]
   [--out-dir <dir>]` — key-name defaults to `byok-rsa4096`; out-dir defaults to the
   current directory. Reference identifiers elsewhere in the family are
   self-describing (ARN / GCP resource path / AKV URL), so consuming commands need no
   provider flag; init alone targets per-provider (a vault to import *into* has no
   universal identifier shape).
2. **Ceremony steps**: verify the vault and its protections (gate) → generate locally
   (`crypto/rsa`, 4096) → import via the Azure SDK (`azkeys`, already in the dependency
   graph — no `az` shell-out) → write `<out-dir>/byok-rsa4096.pem.age` (passphrase,
   scrypt) and the recovery bundle per the custody design's template → add the
   `azure_kv` recipient to the governing `.sops.yaml`.
3. **Shared spine with rekey**: replacing a key is this ceremony plus rekey's sweep —
   one implementation, two entry points.
4. **Pipeline**: rides the standard writ pipeline (graph, trace, receipts) — the
   ceremony is an ordinary planned execution; only `recover` is pipeline-free.

## Verification

1. Unit: gate refusal (missing soft-delete, missing purge protection, both), passphrase
   double-prompt mismatch, bundle schema completeness, `.sops.yaml` seeding, refusal
   shapes below.
2. Import path against a mocked/gated-live vault; `make test`; dual-GOOS lint at zero.
3. The custody drill (design doc, verification 1) consumes this command's artifacts.

## Open questions

1. `.sops.yaml` conflict: when a different recipient already governs the paths — amend
   beside it or refuse and require explicit editing.
2. AWS/GCP targeting flags for their import ceremonies (chartered follow-ons).
3. Whether the Personal migration waits for init or performs the ceremony manually
   (openssl + `az keyvault key import` per the custody design) — the migration is not
   blocked on this plan either way.
