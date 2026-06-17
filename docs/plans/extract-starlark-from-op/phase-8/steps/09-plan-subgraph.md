---
step: 9
title: "plan.subgraph structural-container primitive — compensable, result-flowing"
former_step: 12
former_title: "plan.subgraph primitive"
status: complete
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 9 — plan.subgraph structural-container primitive (formerly step 12)

**Status:** `complete` · **8 / 8 tests written and passing** — the first genuinely test-proven step since step 3.

## What this step delivers

`plan.subgraph` — `flow.Provider.Subgraph` (`pkg/op/provider/flow/provider.go:352`), surfaced flat (`flow` is
`RoleAction|RoleRoot`; announced at `flow/gen/provider.gen.go:31`) — is the **structural-container combinator**:

- It dispatches the `*op.Subgraph` from `activation.Unit` (materialized by `plan.run`/`Assemble` from the
  `op.ExecutableUnit` children), walks its children via `walkSubgraphChildren`, and **flows the leaf result up**.
- It is **compensable**: returns an `*op.RecoveryStack`; `CompensateSubgraph` (`:391`) is the paired undo.
- The **items-iteration form is a guarded stub**: `items []any` with `len > 0` returns
  `"flow.Subgraph: items iteration not yet implemented"` — a future iteration extension, not part of this primitive.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail. All present and passing.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestSubgraph_ReturnsRecoveryStack` (`provider_test.go:110`) | dispatch returns an `*op.RecoveryStack` | ☑ | ✅ |
| 2 | `TestSubgraph_RejectsItems` (`:129`) | the items-iteration form is rejected (stub guard) | ☑ | ✅ |
| 3 | `TestSubgraph_CompensateSubgraph_RoundTrip` (`:148`) | forward + compensate round-trip | ☑ | ✅ |
| 4 | `TestCompensateSubgraph_NilStack_NoOp` (`:139`) | compensate no-ops on a nil stack | ☑ | ✅ |
| 5 | `TestSubgraphBoundAction_FlowsLeafResult` (`result_flow_test.go:28`) | the leaf result flows up (Go side) | ☑ | ✅ |
| 6 | `TestSubgraphBoundAction_FlowsLeafResult_Starlark` (`result_flow_starlark_test.go:44`) | result flows through the bridge (`plan.subgraph` → `plan.run`) | ☑ | ✅ |
| 7 | `TestSubgraphAction_DryRun` (`gen/action.gen_test.go:166`) | dry-run path | ☑ | ✅ |
| 8 | `TestSubgraphAction_CompensableInterface` (`gen:222`) | the generated action satisfies the compensable interface | ☑ | ✅ |

**Coverage: 8 / 8 — the structural primitive is fully proven.**

## Proof run

```
$ go test ./pkg/op/provider/flow/... -run 'Subgraph' -v
--- PASS x8 (ReturnsRecoveryStack, RejectsItems, CompensateSubgraph_RoundTrip, CompensateSubgraph_NilStack,
            FlowsLeafResult, FlowsLeafResult_Starlark, SubgraphAction_DryRun, SubgraphAction_CompensableInterface)
ok  ...flow   ok  ...flow/gen
```

## Findings

- **Description drift in the plan row.** The signature evolved from `Subgraph(children ...op.ExecutableUnit) []any`
  (returning a slice of nils) to the compensable action `Subgraph(activation, items, kwargs) (any, *op.RecoveryStack,
  error)` that dispatches `activation.Unit` as `*op.Subgraph`. Worth refreshing.
- **Items-iteration is an explicit stub** (tested-as-rejected) — a flagged future extension.

## Disposition

`complete` — the structural-container primitive is real and fully test-proven. No tests to add for the core; the
items-iteration form is intentionally deferred.
