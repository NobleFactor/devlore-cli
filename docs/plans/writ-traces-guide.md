---
title: "Writ Traces Guide"
status: complete
created: 2026-08-07
updated: 2026-08-07
---

# Plan: Writ Traces Guide

## Summary

Item 13 (chartered from the item-8 chapter rename): `docs/guides/writ/receipts.md`
documented a fiction — `writ receipt show/list/verify` commands that do not exist, a
`--sign` flag that does not exist, the retired flat receipts store, the filename-in-preimage
checksum, and age/SOPS signing. Replaced by `docs/guides/writ/graphs-and-traces.md`, built
from the verified command surface: the execution store, `writ reconcile` (all eight
classification states with their real glyphs and repair commands), `writ verify` (the
signing-policy ladder, `allowed_signers`), automatic checksums at load, and best-effort
signing at persist. The guide index gains the entry (the old guide was never listed).

## Stale CLI text fixed in passing

1. `writ reconcile --help` claimed stale-vs-modified attribution was "indistinguishable until
   recorded content identity lands (step 48)" — step 48 landed; the indicator list now
   shows the real ↑ Stale / M Modified / M Modified-or-stale split.
2. `writ verify`'s example verified `receipt.yaml` → `trace.yaml`.
3. `status` package doc: "the receipt-signature check arrives with step 46" → landed as
   `writ verify`.

## Verification

Suite green, board zero on both GOOS; every command, flag, glyph, and path in the guide
verified against the code.
