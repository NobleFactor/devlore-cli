---
step: 44
title: "Align service/encryption/git receipts onto RestoreEncoded"
status: done pending commit (2026-07-13) — RestoreEncoded added to all three via catalog resolution; custom Unmarshal/hydrate retired; format-parameterized (json+yaml) tests green
proof_run: TestReceipt_RestoreEncoded_JSONandYAML (service/encryption/git), each json + yaml
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

## What this step did — done 2026-07-13

Each of the three now overrides `Receipt.RestoreEncoded(runtimeEnvironment, base op.ReceiptData, fields map[string]any)`
— the decode the recovery stack drives (`op.reconstructReceipt` at re-arm), env threaded in **explicitly** as a
parameter — and the stack-unused custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate` are removed (repo-wide grep
confirmed no non-stack caller; the unrelated `Resource` unmarshalers stay). Orphaned imports cleaned (`encoding/json`
from `encryption`/`git`; `strings` from `service`).

**Finding — the charter's `DiscoverResource` premise was wrong (a real bug the new tests caught).** A `Resource.URI()`
is a canonical **tag URI** (`tag:…:file:///path#…`), which is **not** a `DiscoverResource` input — `git`/`encryption`'s
`DiscoverResource(base.ResourceURI)` fails ("expected file scheme, got tag"). The old `hydrate` carried this bug; it was
never exercised (stack-unused, untested). The **correct** resolution is the rehydrated catalog: the resource was produced
during the forward run and registered under its URI, so `RestoreEncoded` resolves it through the catalog's URI→id
namespace — `catalog.Lookup(catalog.Current(base.ResourceURI))` — then `Restore(base)` (mirrors `file.Receipt`, which
resolves via the catalog by `resource_id`):

1. **`service.Receipt.RestoreEncoded`** — catalog-resolve (`*service.Resource`) → `Restore(base)` → `WasRunning` /
   `WasEnabled` read from `fields`.
2. **`encryption.Receipt.RestoreEncoded`** — catalog-resolve (`*file.Resource`) → `Restore(base)`. No provider fields.
3. **`git.Receipt.RestoreEncoded`** — catalog-resolve (`*git.Resource`) → `Restore(base)`. No provider fields.

## Verification — ✅ passed 2026-07-13

1. **✅** Each of `service.Receipt` / `encryption.Receipt` / `git.Receipt` implements `op.Receipt.RestoreEncoded`; the
   custom `UnmarshalJSON` / `UnmarshalYAML` / `hydrate` are gone (no non-stack caller, repo-wide).
2. **✅ New format-parameterized coverage** — `TestReceipt_RestoreEncoded_JSONandYAML` in each package: a real receipt is
   marshaled, decoded into the `(base, fields)` pair the stack hands `RestoreEncoded` in **both json and yaml**, and its
   resource (plus service's `was_running` / `was_enabled`) is shown to reconstruct from the rehydrated catalog. This is
   the unit-level format-neutrality gate; the executor-level resume proof (à la `file` in
   `plan.TestGraphResumeThenFail_RollsBack_ViaPublicAPI`) is **not** added here — these providers' forward ops aren't
   graph-wired in tests — and remains a follow-up.
3. **✅** `make test` green (FAIL set is the standing step-18/33 + pwsh gate); `make vet` clean over the touched packages.
