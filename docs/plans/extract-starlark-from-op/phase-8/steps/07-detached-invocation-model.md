---
step: 7
title: "The D5 detached-invocation model — plan-time builds no graph; the graph is materialized only at Assemble"
former_step: 9
former_title: "NodeBuilder detaches from Graph"
status: complete — behavioral tests landed 2026-07-03 (4/4 matrix)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 7 — The D5 detached-invocation model (formerly step 9)

**Status:** `complete` · **Behavioral tests: 4 / 4 written** · both halves proven: structural (no graph-typed state
on `plan.Provider`, `op.Invocation`, or `op.PromiseBinding`) and behavioral (detached until `AssembleDefinition`
roots the invocation set; catalog ownership transfers by pointer).

## What this step delivers

Per phase-8 D5, plan-time evaluation **constructs no graph**. The deliverable survives the `NodeBuilder` removal intact:

- **`plan.Provider` builds no graph** — *"no `op.Graph` is constructed here; nodes produced during script evaluation
  live on detached `*op.Invocation` handles"* (`pkg/op/provider/plan/provider.go:63`). `NewProvider` calls no
  `op.NewGraph`.
- **`Invocation` is detached** — it holds `Target` + `Label` only, no graph reference. The producer→consumer
  relationship travels as a unit ID: [op.Invocation.Binding] returns an `op.PromiseBinding` carrying the producer's
  ID, and the edge is materialized by `plan.assemble_definition` (`pkg/op/invocation.go:6-16`). (The former
  `op.Promise` type and `pkg/op/promise.go` no longer exist — the value chain is
  `*op.Invocation` → `Invocation.Binding()` → `PromiseBinding`.)
- **Materialization happens only at `Assemble`** — `op.NewGraph` is called **once**, in `AssembleDefinition`
  (`provider.go:200`), building a fresh `*op.Graph` from the supplied invocation set and **transferring catalog
  ownership** (capture + nil the runtime environment's `ResourceCatalog`, `:187-188`).

This is what makes a plan **re-runnable**: the graph is built fresh from invocations each `Assemble`, never mutated
during script evaluation.

**Description drift in the plan row:** it says "NodeBuilder dropped its graph field … plan.Provider gained `Catalog`
field." `NodeBuilder` no longer exists (reworked into `plan.adapter`), and the catalog is **not** a `plan.Provider`
field — it lives on the runtime environment and is captured at `Assemble`.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files:
`pkg/op/provider/plan/provider_test.go` (shared with step 5); detachment test in `pkg/op/invocation_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestNewProvider_BuildsNoGraph` | construction creates no `op.Graph` (reflect scan: no graph-typed Provider field); a planned invocation's `Target.ParentID()` stays empty until assembly | ☑ | ✅ |
| 2 | `TestAssembleDefinition_MaterializesGraphFromInvocations` | the graph materializes at `AssembleDefinition` from the invocation set — each invocation's Target is a root child, rooted under `graph.Root()` | ☑ | ✅ |
| 3 | `TestAssembleDefinition_TransfersCatalogOwnership` | `AssembleDefinition` captures the runtime environment's `ResourceCatalog` (same pointer on the graph) and nils the environment's reference | ☑ | ✅ |
| 4 | `TestInvocation_Detached_NoGraphReference` | neither `op.Invocation` nor `op.PromiseBinding` declares graph-typed state; `PromiseBinding.Resolve(nil, nil)` is nil — resolution consults the recovery stack, never a graph | ☑ | ✅ |

**Behavioral coverage: 4 / 4.** Realization notes: rows 2–3 name `AssembleDefinition` (the matrix's `TestAssemble_`
prefix abbreviated the real method name); row 4 was chartered as `TestPromise_Detached_NoGraphReference` in
`pkg/op/promise_test.go`, but no `op.Promise` type exists — the test lands in `pkg/op/invocation_test.go` against the
real value chain (`Invocation` → `Binding()` → `PromiseBinding`), joining the pre-existing `TestInvocation_Binding`
(which already proves the producer-ID edge half).

## Proof run

Verified 2026-07-03: `pkg/op` and `pkg/op/provider/plan` pass under `make test` with the four matrix tests present.
Rows 1–3 plan through the real registry (`file.mkdir` via the test binary's `file/gen` blank import) against a
confined-fsroot runtime environment — the same construction the lifecycle API suite uses.
