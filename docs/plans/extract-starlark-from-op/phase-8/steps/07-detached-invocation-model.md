---
step: 7
title: "The D5 detached-invocation model — plan-time builds no graph; the graph is materialized only at Assemble"
former_step: 9
former_title: "NodeBuilder detaches from Graph"
status: incomplete — pending tests
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 7 — The D5 detached-invocation model (formerly step 9)

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 4 written** · the model is real and load-bearing, but the `plan` package has no tests.

## What this step delivers

Per phase-8 D5, plan-time evaluation **constructs no graph**. The deliverable survives the `NodeBuilder` removal intact:

- **`plan.Provider` builds no graph** — *"no `op.Graph` is constructed here; nodes produced during script evaluation
  live on detached `*op.Invocation` handles"* (`pkg/op/provider/plan/provider.go:63`). `NewProvider` calls no
  `op.NewGraph`.
- **`Promise` and `Invocation` are detached** — *"Promise is detached. It holds no graph reference. The
  producer→consumer edge is implicit in the consumer slot's PromiseValue and is materialized by plan.assemble"*
  (`pkg/op/promise.go:18`; `invocation.go:31`).
- **Materialization happens only at `Assemble`** — `op.NewGraph` is called **once**, in `Assemble`
  (`provider.go:200`), building a fresh `*op.Graph` from the reachable invocation set and **transferring catalog
  ownership** (capture + nil the runtime environment's `ResourceCatalog`, `:187-188`).

This is what makes a plan **re-runnable**: the graph is built fresh from invocations each `Assemble`, never mutated
during script evaluation.

**Description drift in the plan row:** it says "NodeBuilder dropped its graph field … plan.Provider gained `Catalog`
field." `NodeBuilder` no longer exists (reworked into `plan.adapter`), and the catalog is **not** a `plan.Provider`
field — it lives on the runtime environment and is captured at `Assemble`.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). New file:
`pkg/op/provider/plan/provider_test.go` (shared with step 5); `Promise` test in `pkg/op/promise_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestNewProvider_BuildsNoGraph` | construction creates no `op.Graph`; provider holds no graph state | ☐ | — |
| 2 | `TestAssemble_MaterializesGraphFromInvocations` | `op.NewGraph` runs only at `Assemble`, from the invocation set | ☐ | — |
| 3 | `TestAssemble_TransfersCatalogOwnership` | `Assemble` captures and nils the runtime environment's `ResourceCatalog` | ☐ | — |
| 4 | `TestPromise_Detached_NoGraphReference` | `Promise` holds no graph; `SlotValue` → `PromiseValue` carrying the producer `UnitRef` | ☐ | — |

**Behavioral coverage: 0 / 4.** The detachment is asserted by doc comments and is load-bearing for re-runnability, but
no Go test exercises it.

## Proof run

```
$ find pkg/op/provider/plan -maxdepth 1 -name '*_test.go'   # (none)
$ grep -n 'op.NewGraph' provider.go                          # only :200 (inside Assemble) — no construction-time build
```

The step reaches `complete` when rows 1–4 are ☑ and ✅.

## Remaining to reach `complete`

Write rows 1–4 (overlaps the step-5 plan test file + a `Promise` test). No production change — the model is in place;
only the row's `NodeBuilder` / `Catalog`-field wording is stale.
