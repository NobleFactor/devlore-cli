---
step: 8
title: "Target-type-driven slot-fill (ExecutableUnit-assignable → structural unit reference) + catalog.Link convenience"
former_step: 11
former_title: "NodeBuilder.fillSlot dispatches by target type; catalog.Link extraction"
status: complete — behavioral tests landed 2026-07-03 (3/3 matrix)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 8 — Target-type-driven slot-fill + catalog.Link (formerly step 11)

**Status:** `complete` · **Behavioral tests: 3 / 3 written** · the dispatch, `catalog.Link`, and the plan-side
projection are each proven behaviorally; a stale `*op.Promise` bullet in `projectToBinding`'s doc comment was removed
with this parcel.

## What this step delivers

Per phase-8 D2, slot-fill **dispatches on the target parameter's type** — and the deliverable survived the `NodeBuilder`
removal by moving into the `op.Planner` machinery:

- **ExecutableUnit-assignable slots get the structural unit reference.** `executableUnitType =
  reflect.TypeFor[ExecutableUnit]()` (`pkg/op/planner.go:17`); when a param accepts `op.ExecutableUnit`
  (`executableUnitType.AssignableTo(param.Type)`, `planner.go:276`), the slot carries the unit itself as an
  `ImmediateBinding` (a structural reference — e.g. `plan.subgraph`'s children) rather than a value-side
  `PromiseBinding`.
- **Value-typed slots get a `PromiseBinding`.** The plan-side projection (`pkg/op/provider/plan/helpers.go:160`,
  `projectToBinding`) maps `*op.Invocation` → `PromiseBinding`, `*op.Variable` → `VariableBinding`, else
  `ImmediateBinding`. (The former `projectToSlotValue` name and its `*op.Promise` case are gone with the `op.Promise`
  type.)
- **`catalog.Link`** (`pkg/op/resource_catalog.go:314`) — a thin convenience over `Resolve` returning the canonical
  linked Resource; used at `:220`.

**Description drift in the plan row:** `NodeBuilder.fillSlot` is the dead vehicle — the dispatch now lives in
`planner.go`, and `NodeBuilder.linkResource` collapsed into the inline `catalog.Link` call site.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files:
`pkg/op/planner_test.go` (new), `pkg/op/resource_catalog_test.go`, `pkg/op/provider/plan/helpers_test.go` (new).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestActionPlanner_ExecutableUnitAssignableDispatch` | one planned call against a synthetic two-parameter receiver: the `ExecutableUnit`-typed slot receives `ImmediateBinding(Target)` (structural reference); the string-typed slot receives the invocation's `PromiseBinding` (value-side output, producer-ID edge) | ☑ | ✅ |
| 2 | `TestCatalog_Link_ReturnsCanonicalEntry` | `Link` interns a first sighting as a discovery entry and returns the canonical entry (input discarded) on a second sighting — the convenience over `Resolve` with the catalog ID dropped | ☑ | ✅ |
| 3 | `TestProjectToBinding_Dispatch` | invocation→`PromiseBinding` (producer ID), variable→`VariableBinding` (name), default→`ImmediateBinding` (raw value) | ☑ | ✅ |

**Behavioral coverage: 3 / 3.** Realization notes: row 1 was chartered as `TestExecutableUnitType_AssignableDispatch`
(a bare reflect assertion); it landed behaviorally through `ActionPlanner.Plan` with a never-announced synthetic
receiver type, which proves the dispatch *and* the resulting bindings. Row 2 follows the file's `TestCatalog_*` naming
convention. Row 3 names the real helper `projectToBinding` (the matrix's `projectToSlotValue` was its pre-rename
citation); its former `*op.Promise` case does not exist — the stale doc-comment bullet at `helpers.go:148` was removed.

## Proof run

Verified 2026-07-03: `pkg/op` and `pkg/op/provider/plan` pass under `make test` with the three matrix tests present.
Row 1's fixtures (`slotDispatchProvider`, `planInvocatorStub`) are never announced, so the process registry stays
clean; row 3 constructs its producer node via `WithActionNamed("file.mkdir")` against the test binary's announced
registry.
