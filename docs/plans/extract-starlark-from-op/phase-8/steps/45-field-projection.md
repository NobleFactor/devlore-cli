---
step: 45
title: "Field projection — record-valued variables project a field at resolve time (plan.item)"
status: complete (2026-07-15) — landed end to end: Variable.Field + VariableBinding projection + document form + the four stamp sites + GatherPlanner record validation + the ValidateGraph outside-gather scope check + plan.variable(field=)/plan.item surfaces; 5 op tests + 3 devloretest fixtures green; step 33 slice A unblocked
proof_run: 2026-07-15 (make test — pkg/op, flow, plan, devloretest green incl. test_gather_projection / test_choose_in_gather / test_gather_projection_missing_field)
parent: ../../phase-8.md
---

# Step 45 — field projection (`plan.item`)

**Chartered 2026-07-15** out of the writ-adopt gather design
([writ-adopt-command.md](../writ-adopt-command.md), pre-slice A0): a gather body exposes the whole `item` with no
in-plan derivation (the A4 fixture pins it), so record-shaped items need a projection surface for their fields.
Ruled: this is its own step — it changes framework surface every tool can use — and lands, reviewed, before step 33
slice A consumes it.

## Design (settled in the adopt design discussion)

1. **`op.Variable` gains `Field`** (`variable.go:83`) — the plan-time reference shape; empty means the whole value.
2. **`plan.Provider.Item(field)`** → `&op.Variable{Name: "item", Field: field}`; announced → the adapter exposes
   `plan.item("source")`.
3. **`VariableBinding` grows a `field` member** (`binding.go:145`) — the sealed three-variant binding set is
   unchanged; `Resolve` looks up the frame value, then projects the field (the record arrives as the converted
   natural form, `map[string]any`).
4. **The three stamp sites carry `Field` through** — `planner.go:294`, `plan/helpers.go:217`, and the document-load
   path `node.go:294`; the slot's document form gains `field` beside the variable name (round-tripped).
5. **Plan-time validation in `GatherPlanner.Plan`** — with immediate items: every `item`-projection field must exist
   in every record (a plan error, not a nil at dispatch); the step-16 type check reads the projected field's value
   type; the gather uniqueness contract restated over the projected values feeding file ops; `plan.item` outside a
   gather body is a friendly plan error (the A2 frame-stripping rule makes it unresolvable anyway).
6. **Scope for free** — projections resolve wherever the frame binds the variable: nested subgraphs, choose
   when-predicates and branches, wait_until bodies (frame inheritance, pinned by A1). No per-combinator work.

## Test plan

1. `VariableBinding` projection resolve (+ whole-value behavior unchanged when `field` is empty) and document
   round-trip through both codecs.
2. `GatherPlanner` validation: missing field in any record = plan error; `plan.item` outside a gather = plan error.
3. `.star` fixture: gather over record items with a multi-invocation body of projections — deliberately dissolving
   the A4 single-value limitation and restating the uniqueness contract in field terms.
4. `.star` fixture: choose-inside-gather — projection through inherited frames (when-predicate and branch slots).

## Settled

1. **Projection surface (ruled 2026-07-15): both.** `plan.variable(name, field=...)` is the primitive — any
   record-valued variable in scope projects a field; `plan.item(field)` is sugar over it (`plan.variable("item",
   field=field)`) for the gather-body case. `plan.Provider.Variable` gains the `field` parameter;
   `plan.Provider.Item` delegates to it.

## Exit — met 2026-07-15

The test plan is green (op: projection resolve / round-trip / outside-gather scope violation; flow: GatherPlanner
record validation via the missing-field fixture; devloretest: gather-with-records multi-invocation body,
choose-inside-gather, plan-time missing-field error). Step 33 slice A is unblocked as a pure consumer.

## Implementation notes (2026-07-15)

1. A fourth stamp site surfaced beyond the design's three: `flow/planners.go` `projectKwargValue` (the combinator
   planners' kwarg projection). All four carry `Field`.
2. Gather-ness in the [ValidateGraph] scope walk is read through the bound [Action] (`boundActionName`) — an
   action-bound subgraph's `ActionName()` is empty (that accessor serves by-name binding), which the first cut
   missed and every in-gather projection was wrongly flagged.
3. Newly-compiling packages surfaced stale tests fixed in passing: `migrate/receipt_integration_test.go`
   (`Assemble` → `AssembleDefinition`; `trace.State`/`RunStateCompleted` → the `RunStatus` triplet) and
   `internal/e2e/migrate_test.go` (dead `op.ImmediateOf` + the nonexistent `path` slot → `destination_path`);
   `plan/gather_api_test.go` gained the `Variable` arity.
