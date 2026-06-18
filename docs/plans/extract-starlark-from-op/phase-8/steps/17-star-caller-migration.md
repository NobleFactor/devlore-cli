---
step: 17
title: "Migration of existing .star callers off old API forms"
former_step: 20
former_title: "Migration of existing .star callers"
status: complete (caller migration verified; 3.2 phantom-API defect corrected 2026-06-17)
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 17 — Migration of existing .star callers (formerly step 20)

**Status:** `complete` for the actual deliverable — the `.star` caller migration is verifiably done. One defect in the
secondary doc sweep: `3.2-projected-provider-api.md` documents a **non-existent** `plan.iter.item` API.

## What this step delivers

Existing `.star` callers no longer use the superseded forms. For a migration, "zero old-form hits" is the proof — and
it holds:

| Old form | Replacement | `.star` hits | Grade |
|---|---|---|---|
| `plan.choose(when=…, then=…)` direct kwargs | `plan.choose(default, plan.case(when=…, then=…))` | **0** | ✅ gone |
| `plan.flow.<method>` sub-namespace | promoted builtin (`plan.<method>`) | **0** | ✅ gone |
| `plan.elevate` / `EnvironmentValue` | removed | **0** (one stale mention in a comment) | ✅ gone |

The 7 grep hits for `plan.choose(.*when=` are all the **current** nested `plan.case(...)` form (e.g.
`plan.choose("missing", plan.case(when=exists_inv, then="found"))`), not the old direct-kwargs form. Confirmed by
`grep 'plan.choose(when=' --exclude plan.case` → empty.

## Defect: the doc sweep documents a phantom API

The row claims `docs/architecture/3.2-projected-provider-api.md` was "re-anchored to current API with `plan.iter.item`
per-frame binding." But **`plan.iter` is not a real namespace**:

- The per-iteration variable is named `item` and bound by `buildIterationFrame` (`flow/helpers.go:66` —
  `frame["item"] = op.Variable{Name: "item", Value: item}`).
- `flow/planners.go:80` documents the real surface: the frame "masks any **`plan.variable("item")`** reference."
- All gather fixtures use `plan.variable("item", default_value=None)` (3 files). `plan.iter.item` appears in **zero**
  `.star` files and **zero** Go code — only in `3.2-projected-provider-api.md:91,95` and the row's own prose.

So the real current API is `plan.variable("item")`; the doc documents a namespace that does not resolve. Either the doc
is wrong (should be `plan.variable("item")`) or `plan.iter.item` is intended sugar that was never implemented.

## Disposition

`complete` for the caller migration — the deliverable is verifiably met (0 old-form hits, the falsifiable proof a
migration requires).

**Open item RESOLVED (2026-06-17, user decision — "the code is king").** `plan.variable("item")` is canonical;
`plan.iter.item` was never implemented. `docs/architecture/3.2-projected-provider-api.md` corrected to match the code,
verified against `pkg/op/provider/flow/helpers.go` + the gather fixtures (`plan.variable("item", default_value=None)`):

- Line 75 slot-type table: the phantom **"Proxy"** row → **"Variable"** (the sealed-three slot model is
  `ImmediateValue` / `PromiseValue` / `VariableValue`; `projectToSlotValue` maps `*op.Variable` → `VariableValue`).
- Line 91 / 95: `plan.iter.item` → `plan.variable("item")`.
- Lines 97–99: the phantom **`plan.depends_on`** barrier (it exists nowhere in code or fixtures) → the real mechanism:
  consume the gather's returned handle in a downstream slot, whose Promise edge resolves only when every iteration
  completes.

Separately flagged, not in this decision's scope: the same section's `FillSlot` reference is a function-name staleness
(the slot-fill dispatch moved to the `op.Planner` machinery / `projectToSlotValue` per step 8) — left untouched.
