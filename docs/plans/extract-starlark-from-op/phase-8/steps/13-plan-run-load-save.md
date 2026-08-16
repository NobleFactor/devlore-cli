---
step: 13
title: "plan.assemble_definition / plan.spec / plan.run / plan.save_definition / plan.load_definition — the assemble-spec-run split"
former_step: 16
former_title: "plan.run + plan.load + plan.save"
status: complete — behavioral tests landed 2026-07-03 (all five methods proven through both APIs)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 13 — plan.assemble_definition / plan.spec / plan.run / plan.save_definition / plan.load_definition (formerly step 16)

**Status:** `complete`. All five methods are proven through **both** planning APIs — the Go `plan.Provider` surface
and the `.star` surface. The 2026-06-16 audit below is preserved as history; its "zero callers, zero tests" claims
were superseded by the lifecycle suites (which the audit predates) plus two focused tests landed 2026-07-03.

## Method names

The audit was written against the pre-rename surface. Current names: `plan.assemble` → `plan.assemble_definition`
(`provider.go:148`), `plan.save` → `plan.save_definition` (`provider.go:367`), `plan.load` → `plan.load_definition`
(`provider.go:327`). `plan.spec` (`provider.go:442`) and `plan.run` (`provider.go:495`) kept their names.

## Proof state per method (verified 2026-07-03)

| Method | Go-API proof | Starlark-API proof | Grade |
|---|---|---|---|
| `AssembleDefinition` | `lifecycle_api_test.go` (all five tests), `gather_api_test.go`, step-7 matrix (`TestAssembleDefinition_MaterializesGraphFromInvocations`, `_TransfersCatalogOwnership`) | 53 `.star` fixtures + the lifecycle scripts | ✅ |
| `SaveDefinition` | `TestGraphSaveLoadExecuteTrace_ViaPublicAPI` (save → load → checksum identity → execute the *loaded* graph) | `plan.save_definition` in `TestGraphSaveLoadExecute_ViaStarlark` (`lifecycle_starlark_test.go`) and `lifecycle_e2e_test.go` | ✅ |
| `LoadDefinition` | same round-trip + the resume variants (`TestGraphSaveLoadResume_ViaPublicAPI`, `resumeThenFailRollsBack`, `resumePromiseFidelity`) | `plan.load_definition` in the same scripts — the *loaded* graph is the one run | ✅ |
| `Spec` | `lifecycle_api_test.go:75`/`:142`, `gather_api_test.go:76` (explicit args); `TestProvider_Spec_DefaultsFromPlanningEnvironment` (2026-07-03 — the defaults contract directly) | `plan.spec()` all-defaults in both lifecycle scripts | ✅ |
| `Run` | `gather_api_test.go:81` — the failure path: the run error names the failing unit and completed iterations compensate LIFO; `TestProvider_Run_NilArguments_Error` (2026-07-03 — the guard clauses) | `plan.run(loaded, plan.spec())` — the success path, side effect asserted | ✅ |

The two 2026-07-03 tests close the residue the suites left implicit:

1. `TestProvider_Spec_DefaultsFromPlanningEnvironment` (`provider_test.go`) — `Spec("", "", nil)` falls back to the
   planning environment's program name and flags, and mints a **fresh** `fsroot.Dir` handle at the same anchor (same
   `Name()`, different handle), so successive Runs don't share a Root that closes when the first executor finishes.
2. `TestProvider_Run_NilArguments_Error` (`provider_test.go`) — `Run(nil, spec)` and `Run(graph, nil)` return errors.

## Historical audit (2026-06-16, superseded)

The audit found `Run`/`Spec`/`Save`/`Load` announced but with zero callers and zero tests: `t.run` reimplemented
execution via `op.NewGraphExecutor` + `tc.buildSpec()`, and save/load appeared only as commented-out placeholders in
the unregistered `test_round_trip_writ_adopt.star`. That was accurate then; the lifecycle suites
(`lifecycle_api_test.go`, `lifecycle_starlark_test.go`, `lifecycle_e2e_test.go`, `gather_api_test.go`) subsequently
drove every method through both APIs. `t.run` remains harness sugar that builds its own executor — by design
(`runner.go:334`: scripts wanting spec control call `plan.run(graph, plan.spec(...))` instead).

## Residual debris

`cmd/devlore-test/devloretest/data/test_round_trip_writ_adopt.star` is still an unregistered stub whose comments cite
the pre-rename `plan.save`/`plan.load` and a `t.expect_graph_equal` that does not exist. Its purpose (a
writ-adopt-flavored round-trip) is writ territory — routed to step 33 (writ migrate rewrite) to be rewritten under
the current names or deleted.
