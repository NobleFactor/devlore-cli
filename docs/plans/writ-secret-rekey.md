---
title: "Writ Secret Rekey"
status: proposed
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Writ Secret Rekey

## Summary

`writ secret rekey [<file>...]` — re-wrap the data keys of existing SOPS documents
against the current `.sops.yaml` recipients, the `sops updatekeys` equivalent. This is
the day-two operation for key custody changes: adding an escrow recipient, rotating the
Azure Key Vault key version, or removing a compromised recipient. Chartered 2026-08-09;
sequenced after [writ-secret-encrypt](writ-secret-encrypt.md) and the personal-repo
migration (it needs an encrypted fleet to operate on).

## Design sketch

1. **New provider action** `encryption.rekey_file`: load the SOPS document, rebuild its
   key groups from the governing `.sops.yaml` (the encrypter's existing discovery),
   re-wrap the data key, write back. Compensable — the receipt carries the original
   document bytes for restore.
2. **Command**: `rekey` beside `encrypt` under `writ secret`; with no arguments it
   sweeps every `*.sops` file governed by a discoverable `.sops.yaml` under the current
   tree, with arguments it takes explicit files.
3. **Distinction to preserve**: `updatekeys` re-wraps the data key only (content
   untouched, cheap); `rotate` generates a new data key and re-encrypts content. This
   plan delivers the former; `rotate` waits for a demonstrated need.

## Verification

1. Unit: recipient-set change reflected in document metadata; content bytes decrypt
   identically before and after; no-governing-rule error.
2. `make test`, dual-GOOS lint recount at zero.

## Open questions

1. Whether the no-argument sweep form belongs in the first delivery or arguments-only
   ships first.
