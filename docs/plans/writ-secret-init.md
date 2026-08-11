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
4. **Layer-scoped** (ruled 2026-08-10): targets are registered layers by name; the
   command refuses a name with no registration, pointing at `writ repo add`.
5. **Config conflict refuses without `-f/--force`** (ruled 2026-08-10): when a named
   layer's root `.sops.yaml` already carries different recipients, init fails and
   changes nothing; `--force` proceeds with the write. Deliberate contrast: the vault
   deletion-protection gate has no force — custody loss is unrecoverable, a config
   file is editable.

## Design

1. **Surface**: `writ secret init <layer>... [--azure-kv <vault> | --aws-kms <key> |
   --gcp-kms <ring>] [--key-name <name>] [-o|--output-dir <dir>] [-f|--force]` — each
   `<layer>` names a registered writ layer (personal, team, base; ruled 2026-08-10),
   and one ceremony may seed several: one key, one custody emission, one `.sops.yaml`
   write per named layer. Exactly one targeting flag, mirroring sops' own vocabulary;
   v1 implements `--azure-kv`, the other two refuse as chartered until their
   deliveries. key-name defaults to `byok-rsa4096`; output-dir defaults to the current
   directory.
2. **Assets at `--output-dir`** — exactly two files, never touched again by writ
   (replication is the owner's, per the custody ruling): `<key-name>.pem.age`, the
   passphrase-encrypted private key (age v1, scrypt); and `<key-name>.recovery.yaml`,
   the recovery bundle (publishable-without-harm). Invariant: the plaintext private key
   never exists on disk — generated in memory, encrypted, written once. One ceremony
   emits one asset pair regardless of layer count. Reference identifiers elsewhere in the family are
   self-describing (ARN / GCP resource path / AKV URL), so consuming commands need no
   provider flag; init alone targets per-provider (a vault to import *into* has no
   universal identifier shape).
3. **Ceremony steps**: verify the vault and its protections (gate) → generate locally
   (`crypto/rsa`, 4096) → import via the Azure SDK (`azkeys`, already in the dependency
   graph — no `az` shell-out) → write `<output-dir>/byok-rsa4096.pem.age` (passphrase,
   scrypt) and the recovery bundle per the custody design's template → write or amend
   `.sops.yaml` at each named layer's working-tree **root**, never deeper — the root
   file is the only shape where writ's chain discovery and the sops CLI resolve
   identically — carrying the `azure_kv` recipient.
4. **Shared spine with rekey**: replacing a key is this ceremony plus rekey's sweep —
   one implementation, two entry points.
5. **Pipeline**: rides the standard writ pipeline (graph, trace, receipts) — the
   ceremony is an ordinary planned execution; recover and decrypt sit outside the
   pipeline — disaster tooling assumes nothing, and an effectless stdout read has
   nothing to trace or compensate.

## Verification

1. Unit: gate refusal (missing soft-delete, missing purge protection, both), passphrase
   double-prompt mismatch, bundle schema completeness, `.sops.yaml` seeding, refusal
   shapes below.
2. Import path against a mocked/gated-live vault; `make test`; dual-GOOS lint at zero.
3. The custody drill (design doc, verification 1) consumes this command's artifacts.

## Open questions

1. AWS/GCP import-ceremony details behind `--aws-kms` / `--gcp-kms` (chartered
   follow-ons; the flag surface is ruled).
2. Whether the Personal migration waits for init or performs the ceremony manually
   (openssl + `az keyvault key import` per the custody design) — the migration is not
   blocked on this plan either way.
