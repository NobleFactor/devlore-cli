---
step: 8
title: "Target-type-driven slot-fill (ExecutableUnit-assignable → structural unit reference) + catalog.Link convenience"
former_step: 11
former_title: "NodeBuilder.fillSlot dispatches by target type; catalog.Link extraction"
status: incomplete — pending tests
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 8 — Target-type-driven slot-fill + catalog.Link (formerly step 11)

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 3 written** · the deliverable survives (the dispatch moved out of the removed `NodeBuilder`), unverified.

## What this step delivers

Per phase-8 D2, slot-fill **dispatches on the target parameter's type** — and the deliverable survived the `NodeBuilder`
removal by moving into the `op.Planner` machinery:

- **ExecutableUnit-assignable slots get the structural unit reference.** `executableUnitType =
  reflect.TypeFor[ExecutableUnit]()` (`pkg/op/planner.go:17`); when a param accepts `op.ExecutableUnit`
  (`executableUnitType.AssignableTo(param.Type)`, `planner.go:277`), the slot carries the unit itself (a structural
  reference — e.g. `plan.subgraph`'s children) rather than a value-side `PromiseValue`.
- **Value-typed slots get a `PromiseValue`.** The plan-side projection (`pkg/op/provider/plan/helpers.go:159`,
  `projectToSlotValue`) maps `*op.Invocation`→`PromiseValue`, `*op.Promise`→`PromiseValue`, `*op.Variable`→
  `VariableValue`, else `ImmediateValue`.
- **`catalog.Link`** (`pkg/op/resource_catalog.go:314`) — a thin convenience over `Resolve` returning the canonical
  linked Resource; used at `:220`.

**Description drift in the plan row:** `NodeBuilder.fillSlot` is the dead vehicle — the dispatch now lives in
`planner.go`, and `NodeBuilder.linkResource` collapsed into the inline `catalog.Link` call site.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files:
`pkg/op/planner_test.go`, `pkg/op/resource_catalog_test.go`, `pkg/op/provider/plan/helpers_test.go` (new).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestExecutableUnitType_AssignableDispatch` | `executableUnitType.AssignableTo` selects the structural-reference branch for `ExecutableUnit`-accepting params and not for value-typed params | ☐ | — |
| 2 | `TestResourceCatalog_Link_ReturnsCanonicalEntry` | `Link` interns and returns the canonical linked Resource (the convenience over `Resolve`) | ☐ | — |
| 3 | `TestProjectToSlotValue_Dispatch` | invocation→`PromiseValue`, promise→`PromiseValue`, variable→`VariableValue`, default→`ImmediateValue` | ☐ | — |

**Behavioral coverage: 0 / 3.** No test exercises the `AssignableTo` dispatch, `catalog.Link`, or `projectToSlotValue`.

## Proof run

```
$ grep -n 'executableUnitType' pkg/op/planner.go        # :17 (cached type), :277 (AssignableTo dispatch)
$ grep -n 'func (c \*ResourceCatalog) Link' resource_catalog.go   # :314
$ find pkg/op/provider/plan -maxdepth 1 -name '*_test.go'         # (none)
```

The step reaches `complete` when rows 1–3 are ☑ and ✅.

## Remaining to reach `complete`

Write rows 1–3. No production change — the dispatch and `catalog.Link` are in place; only the row's `NodeBuilder.fillSlot`
wording is stale.
