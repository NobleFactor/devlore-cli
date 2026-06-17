---
step: 13
title: "plan.assemble / plan.spec / plan.run / plan.save / plan.load — the assemble-spec-run split"
former_step: 16
former_title: "plan.run + plan.load + plan.save"
status: partial — Assemble proven; Run/Spec/Save/Load announced + callable but uncalled and untested
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 13 — plan.assemble / plan.spec / plan.run / plan.save / plan.load (formerly step 16)

**Status:** `partial`. The row labels this `complete`; that holds for **one of the five methods**. `Assemble` is
genuinely proven. `Run`, `Spec`, `Save`, `Load` are announced and callable from `.star` but have **zero callers
anywhere — Go or `.star` — and zero tests**.

## What this step delivers (and the proof state of each method)

The step landed as the **assemble / spec / run split** (the design evolved from one `plan.run(...)` that materialized +
ran). All five are announced (`plan/gen/provider.gen.go:20/24/27/28/29`), so all are callable from `.star`.

| Method | Def | Does | Caller evidence | Tests | Grade |
|---|---|---|---|---|---|
| `plan.assemble` | `provider.go:148` | materializes a `*op.Graph` from the reachable invocation set | **writ** `adopt/plan.go:85`, `migrate/plan_builder.go:126`, `migrate/file_ops.go:185` + **53 `.star`** files | 53 `.star` end-to-end (build→run→assert) | ✅ proven |
| `plan.run` | `provider.go:464` | `op.NewGraphExecutor(graph, spec).Run(...)` | **none** — `t.run` reimplements this directly (`test_context.go:715`), bypassing the method; 0 `.star`, 0 Go | none | ❌ uncalled, untested |
| `plan.spec` | `provider.go:411` | builds `*op.RuntimeEnvironmentSpec` | **none** — `t.run` uses `tc.buildSpec()`; 0 `.star`, 0 Go | none | ❌ uncalled, untested |
| `plan.save` | `provider.go:336` | `graph.Serialize` → JSON/YAML on disk | **none** — only commented-out placeholder in an **unregistered** fixture | none | ❌ uncalled, untested |
| `plan.load` | `provider.go:296` | `op.LoadGraph` + `op.ValidateGraph` | **none** — same placeholder | none | ❌ uncalled, untested |

## The save/load "round-trip" fixture is a stub that never runs

`cmd/devlore-test/devloretest/data/test_round_trip_writ_adopt.star` is the only `.star` mentioning `plan.save`/
`plan.load` — and only in comments. Lines 34–35 are **commented-out future assertions**:

```
#   plan.save(original, graph_path)
#   loaded = plan.load(graph_path)
```

The header reads "Phase 5+ assertions (when plan.assemble / plan.save / plan.load / t.expect_graph_equal land)." The file
is **not registered in `runner_test.go`**, so it never executes. `plan.save` / `plan.load` are never actually called.

## The harness bypasses plan.run and plan.spec

`t.run` (`test_context.go`) projects the graph, then builds and runs the executor **itself**:

```
spec, err := tc.buildSpec()                  // not plan.Provider.Spec
executor := op.NewGraphExecutor(graph, spec) // not plan.Provider.Run
result, runErr := executor.Run(context.Background(), nil)
```

So the 53 green `.star` tests prove `op.GraphExecutor` and `plan.assemble` — **not** `plan.Provider.Run`/`Spec`. The
underlying `op.LoadGraph` / `graph.Serialize` are covered by `pkg/op` tests, but the `plan.Provider` wrappers are not.

## To reach `complete`

1. `TestPlanProvider_SaveLoad_RoundTrip` — `assemble` → `Save` to a temp path → `Load` → assert structural equivalence
   (the assertion the stub fixture has only ever described in comments). Register a real `.star` equivalent, or write it
   as a Go test against `plan.Provider`.
2. Drive `plan.Provider.Run` + `plan.Provider.Spec` from at least one test (or have `t.run` call the real methods so the
   53 existing fixtures exercise them) — today nothing does.
3. If `Run`/`Spec`/`Save`/`Load` are not meant to be reachable yet, say so in the row; "complete" with zero callers and
   zero tests is not accurate.
