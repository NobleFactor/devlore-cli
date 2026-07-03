---
step: 22
former_step: 19
title: "Resource foundation cleanup — the 13.0(k) arc plus sub-steps (d)–(n)"
status: in-progress — 13.0(k) complete (all sub-items closed); 13.0(n) writ graph executor is the only open item
proof_run: 2026-07-03 (audit of the sub-item ledger)
parent: ../../phase-8.md
---

# Step 22 — Resource foundation cleanup (formerly 19)

**Status:** `in-progress`. Prerequisite for step 13 and everything downstream that touches Resources. The umbrella
holds two ledgers: the lettered sub-steps 22(d)–(n) — **all complete** (see the phase-8 table rows) — and the
13.0(k) sub-item arc, now also closed.

## The 13.0(k) arc (closed)

1. Delete `<M>Planned` companions — code complete; doc closure folded into k.15.
2. Twelve required Resource interfaces across all nine Resource-bearing providers — complete: `op.ResourceBase`
   shared implementations plus per-type overrides on file/git/appnet/pkg/service/mem/function/json/yaml; the k.12
   boot-discipline test asserts no Resource type leaves `Addressing` at the default sentinel.
3. Catalog operations on the addressing/digest contract — k.10 (Resolve cascade), k.13 (lifecycle integration:
   Pending/Active/Gone state machine, catalog-owned transitions per Model A), k.14 (audit-only — file-provider
   Compensate methods inspected method-by-method; no migration work remained), k.15
   (`(*ResourceCatalog).ResolvePending() []error`, wired into `GraphExecutor.Run` preflight with fail-fast
   `errors.Join`, skipped in dry-run; 8 tests) — **all done**.

Platform verification at preflight, originally scoped into k.15, moved out — tracked as #282 under step 16's
preflight scope.

## Remaining to reach `complete`

1. **13.0(n) — the writ graph executor** is the only not-started item under 13.0; it is subsumed by step 33
   (`writ migrate` full rewrite), so this umbrella closes when step 33 lands.

Detailed sub-item history: [phase-8/13.0-n.md](../13.0-n.md) (which uses its own internal sub-step numbering — not
phase step numbers).
