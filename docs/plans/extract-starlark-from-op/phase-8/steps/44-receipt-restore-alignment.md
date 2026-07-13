---
step: 44
title: "Align service/encryption/git receipts onto RestoreEncoded"
status: charter — not started (surfaced by step 42 slice 3b, 2026-07-12)
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 44 — Align `service` / `encryption` / `git` receipts onto `RestoreEncoded`

**Status:** `charter` — chartered 2026-07-12 out of [step 42](42-compensator-interface.md) slice 3b, which unified the
recovery-stack serialization on the receipt-owned `RestoreEncoded` path. Three concrete receipts are **behind**: they
reconstruct through a recovery stack via the bare `op.ReceiptBase.RestoreEncoded`, so their resource and
provider-specific state are lost on resume. This step brings them onto the same path as `file.Receipt` / `pkg.Receipt`.

**Depends on** step 42 (slice 3b): the structural-discriminator reader and `op.reconstructReceipt` →
`Receipt.RestoreEncoded` path this step conforms to.

## The gap (research, step 42 slice 3b)

The recovery stack reconstructs each receipt through `op.reconstructReceipt`, which resolves the concrete type from
`compensating_action` (→ `compensatorType`) and calls `Receipt.RestoreEncoded(env, base, fields)` — **never** a receipt's
`UnmarshalJSON`/`UnmarshalYAML`. Two tiers of concrete receipt exist:

- **Aligned** (`RestoreEncoded` override → full stack round-trip): `file.Receipt`, `pkg.Receipt`.
- **Behind** (override the older custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate`, **no** `RestoreEncoded` override):
  `service.Receipt`, `encryption.Receipt`, `git.Receipt`. Their custom decode path is **stack-unused**, so through a
  recovery stack they fall to `op.ReceiptBase.RestoreEncoded` (base only — no resource, no provider fields). There is
  **no** trace/resume test coverage for any of the three.

Slice 3b aligned their **encode** (`service.Receipt.MarshalYAML` embeds `op.ReceiptData`; `encryption`/`git` inherit
`op.ReceiptBase.MarshalYAML`) so 3b did not regress them — but their **decode** through the stack is still the bare base.

## What this step does

Give each of the three a `Receipt.RestoreEncoded(runtimeEnvironment, base op.ReceiptData, fields map[string]any)` override
that resolves its resource against the rehydrated catalog and restores its provider-specific state, then retire the
now-redundant custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate` (stack-unused). Each `RestoreEncoded` is the existing
`hydrate` body with `base` as its input:

1. **`service.Receipt.RestoreEncoded`** — `DiscoverResource(runtimeEnvironment, strings.TrimPrefix(base.ResourceURI,
   "svc:"))`, `Restore(base)`, then `WasRunning` / `WasEnabled` from `fields` (`boolField`). Retire its custom
   `UnmarshalJSON` / `UnmarshalYAML` / `hydrate`.
2. **`encryption.Receipt.RestoreEncoded`** — `file.DiscoverResource(runtimeEnvironment, base.ResourceURI)`,
   `Restore(base)`. No provider-specific fields. Retire its custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate`.
3. **`git.Receipt.RestoreEncoded`** — `DiscoverResource(runtimeEnvironment, base.ResourceURI)`, `Restore(base)`. No
   provider-specific fields. Retire its custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate`.

Open question to settle during implementation: whether any direct (non-stack) call site still needs a receipt's
`UnmarshalJSON` (the step-42 research found none in `pkg/op`; confirm before deleting, and keep the `Resource`
unmarshalers, which are unrelated).

## Verification

1. Each of `service.Receipt` / `encryption.Receipt` / `git.Receipt` implements `op.Receipt.RestoreEncoded`; the custom
   `UnmarshalJSON` / `UnmarshalYAML` / `hydrate` are gone (or justified if a direct call site survives).
2. **New trace round-trip + resume coverage** for each: a receipt pushed onto an `op.RecoveryStack`, serialized, reloaded,
   re-armed against a rehydrated catalog, and shown to reconstruct its resource + provider state (the coverage the three
   currently lack). This is the real gate — the encode already round-trips; the decode is what this step fixes.
3. `make test` green (modulo the standing step-18 gate); `make vet` clean.
