---
step: 16
title: "Topological sort + plan-time type-check pass (checkPromiseTypes)"
former_step: 19
former_title: "Topological sort + plan-time type-check pass"
status: complete — behavioral tests landed 2026-07-03 (5/5 matrix; step-15's direct symmetric-probe test absorbed)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 16 — Topological sort + plan-time type-check pass (formerly step 19)

**Status:** `complete`. The headline deliverable — `checkPromiseTypes`, the plan-time Promise→slot type verification —
is now proven in both directions and end-to-end; toposort has a direct test; and the parcel absorbed step 15's one
unwritten row (`TestTypesAreInterconvertible`, the direct symmetric-arms probe the promise-type rows lean on).

## What this step delivers

`op.ValidateGraph` (`pkg/op/validate.go:56–59`) aggregates four validators into the single envelope
`plan.Provider.AssembleDefinition` returns:

| Validator | Def | Role | Tests | Grade |
|---|---|---|---|---|
| `checkRequiredParams` | `validate.go:106` | every required param is bound (node + action-bound subgraph) | `TestValidateGraph_{RequiredBound,RequiredMissing,OptionalMissing,VariadicMissing,KwargsMissing,BoundSubgraph_MissingRequired}` (6) | ✅ |
| `checkBubbleUpConsistency` | `validate.go:178` | same-name bubble-up entries are type-consistent | `TestValidateGraph_TypeCollision_SurfacesAsViolation`, `MultipleViolations_AllJoined` | ✅ |
| `checkPromiseTypes` | `validate.go:211` | **THE type-check**: each `PromiseBinding` slot — producer output type (`Method.ResultType`) vs consumer `Parameter.Type`, via `typesAreInterconvertible` (`validate.go:265`) | matrix rows 1–4 below | ✅ |
| `checkEdges` | `validate.go:76` | endpoints, acyclicity, guarded-subgraph invariant (step 10) | step-10 suite (`guarded_edges_test.go`) | ✅ |

`topologicallySorted` (`helpers.go:250`) orders a subgraph's units for **execution**, producer-before-consumer; not
load-bearing for the static type-check (each Promise binding is independent of visit order).

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files:
`pkg/op/validate_test.go`, `pkg/op/helpers_test.go` (new), `pkg/op/convert_test.go` (absorbed step-15 row),
`test_promise_type_mismatch.star` (registered as `TestPromiseTypeMismatch`).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestValidateGraph_PromiseType_Compatible_NoError` | a compatible producer→slot binding (string→string) passes `checkPromiseTypes` | ☑ | ✅ |
| 2 | `TestValidateGraph_PromiseType_Incompatible_ReturnsViolation` | an incompatible binding (`chan int`→string) yields the `"cannot bind … output"` violation naming unit, slot, producer, and both types | ☑ | ✅ |
| 3 | `TestValidateGraph_PromiseType_ReverseOnlyConvertible_Passes` | a binding convertible only target→source **passes** — pins the symmetric-probe behavior (see below) | ☑ | ✅ |
| 4 | `test_promise_type_mismatch.star` | the type-check fires end-to-end through `plan.assemble_definition` — mkdir's `*file.Resource` promise bound to write_text's `os.FileMode` chmod slot (no conversion path; `ResourceBase.CanConvertTo` is string-only) | ☑ | ✅ |
| 5 | `TestTopologicallySorted_ProducerBeforeConsumer` | toposort orders an anti-topological input (units `[c,b,a]`, edges a→b→c) producer-first — direct, not transitive | ☑ | ✅ |

**Behavioral coverage: 5 / 5.** Realization notes: the producer side of rows 1–3 needed real reflected methods
(`Method.ResultType` reads the reflected signature), so `validate_test.go` gained the `promiseProducerFixture` +
`producerNode` fixtures alongside the existing parameter-only `makeMethod`/`makeNode` machinery. Row 4's fixture rides
the same plan-validation-only harness path as step 14's orphan fixture.

## The directional question — documented, not resolved

Row 3 pins the current contract: `typesAreInterconvertible` is **symmetric** by its documented design
("or vice versa", `convert.go:503`), so a producer output with only a *reverse* conversion path into the consumer slot
passes the plan-time check, even though dispatch-time `Convert` is one-directional. A directional D8 check would
reject that binding. Changing the relation is a design decision (production change in `validate.go:265`), not a test
gap — the test documents the behavior either way and flips from "passes" to "returns violation" if the design ever
moves.

## Proof run

Verified 2026-07-03: `pkg/op` and `cmd/devlore-test/devloretest` pass under `make test` with all five matrix tests
present, plus the absorbed `TestTypesAreInterconvertible` (see step 15).
