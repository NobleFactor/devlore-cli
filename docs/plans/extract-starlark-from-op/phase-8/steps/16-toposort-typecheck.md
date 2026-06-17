---
step: 16
title: "Topological sort + plan-time type-check pass (checkPromiseTypes)"
former_step: 19
former_title: "Topological sort + plan-time type-check pass"
status: partial — required-param + bubble-up checks tested; checkPromiseTypes (the headline Promise→slot type-check) untested
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 16 — Topological sort + plan-time type-check pass (formerly step 19)

**Status:** `partial`. The row labels this `complete`. The required-param and bubble-up validators are well tested, but
the **headline deliverable — `checkPromiseTypes`, the plan-time Promise→slot type verification — has zero tests**.
Toposort is exercised only transitively.

## What this step delivers

`op.ValidateGraph` (`pkg/op/validate.go:56–58`) aggregates three validators into the single envelope
`plan.Provider.Assemble` returns:

| Validator | Def | Role | Tests | Grade |
|---|---|---|---|---|
| `checkRequiredParams` | `validate.go:77` | every required param is bound (node + action-bound subgraph) | `TestValidateGraph_{RequiredBound,RequiredMissing,OptionalMissing,VariadicMissing,KwargsMissing,BoundSubgraph_MissingRequired}` (6) | ✅ |
| `checkBubbleUpConsistency` | `validate.go:149` | same-name bubble-up entries are type-consistent — `typesAreInterconvertible` at `subgraph.go:685` | `TestValidateGraph_TypeCollision_SurfacesAsViolation` (`"incompatible types"`), `MultipleViolations_AllJoined` | ✅ |
| `checkPromiseTypes` | `validate.go:182` | **THE type-check**: each `PromiseValue` slot — producer output type (`Method.ResultType`) vs consumer `Parameter.Type`, via `typesAreInterconvertible` at `validate.go:234` | **none** | ❌ |

`topologicallySorted` (`helpers.go:250`) orders a subgraph's units for **execution** (`subgraph.go:602/789`),
producer-before-consumer. The row correctly notes it is **not** load-bearing for the static type-check (each Promise
binding is independent of visit order). It has no direct test but is transitively exercised by all 53 `.star`
executions (a broken order would fail them).

## The gap: checkPromiseTypes is unproven

- No `validate_test.go` case builds an incompatible `PromiseValue`→slot binding.
- The `"cannot bind %q output (%s) to declared type %s"` violation (`validate.go:235`) is asserted by **no test and no
  `.star` fixture** (`grep -c "cannot bind"` → 0 across tests and fixtures).
- `test_writ_adopt_type_mismatch.star` (`"not assignable to declared type"`) hits **`helpers.go:122`** — a value-side
  assignability check at slot-fill — **not** `checkPromiseTypes`. So the `validate.go:234` `typesAreInterconvertible`
  call (Promise→slot, **directional** in intent) is never exercised.

This is also where step 15's **directional-vs-symmetric** question bites: `validate.go:234` calls the **symmetric**
`typesAreInterconvertible(source, target)` for a check whose intent (per D8) is **directional** (can *producer output*
fill *consumer slot*). With zero tests on `checkPromiseTypes`, a reverse-only-convertible binding would pass undetected.

## Test matrix (the missing rows)

| # | Test | Proves | Grade |
|---|---|---|---|
| 1 | `TestValidateGraph_PromiseType_Compatible_NoError` | a compatible producer→slot binding passes `checkPromiseTypes` | ☐ |
| 2 | `TestValidateGraph_PromiseType_Incompatible_ReturnsViolation` | an incompatible binding yields the `"cannot bind … output"` violation | ☐ |
| 3 | `TestValidateGraph_PromiseType_ReverseOnlyConvertible` | a binding convertible only target→source — the directional-vs-symmetric case | ☐ |
| 4 | `test_promise_type_mismatch.star` | the type-check fires end-to-end through `plan.assemble` | ☐ |
| 5 | `TestTopologicallySorted_ProducerBeforeConsumer` | toposort orders edges correctly (direct, not transitive) | ☐ |

## Disposition

`partial`. `checkRequiredParams` and `checkBubbleUpConsistency` are proven. The central D8 deliverable
(`checkPromiseTypes`) is wired but untested, and resolving step 15's symmetric-probe question requires exactly the
tests above (rows 2–3). Toposort works (transitively proven) but lacks a direct test.
