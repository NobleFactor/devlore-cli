---
title: "Coalesce pkg/op Redundancies"
status: complete
created: 2026-02-23
updated: 2026-03-16
---

# Plan: Coalesce pkg/op Redundancies

## Completion Summary (2026-03-16)

**All three goals met.** `pkg/op` is the single authority for action-framework types.

### What was done

- **Phase 1 — shim removal:** All 4 re-export shim files deleted
  (`internal/execution/action.go`, `registry.go`, `provider_registry.go`, `graph.go`).
  All consumers updated to import `pkg/op` directly.
- **Phase 2 — FillSlot re-export:** `internal/starlark/output.go` deleted. Codegen
  templates in noblefactor-ops updated to emit `op.FillSlot` directly. All
  `plan_*_gen.go` files regenerated.
- **Phase 3 — dead code:** `Orders` method removed from `internal/execution/plan.go`.

### What was not done

Nothing — all phases complete.

---

## Original Plan

### Goals

1. **Single authority**: All action-framework types (`Action`, `Context`,
   `Result`, `ActionRegistry`, etc.) are imported directly from `pkg/op`.
2. **No re-export shims**: Delete every `type X = op.X` and `var X = op.X`
   alias in `internal/execution/` and `internal/starlark/`.
3. **No dead code**: Remove the duplicate `Orders` method from
   `internal/execution/plan.go` (identical to `DependsOn`, zero callers).

### Phase 1: Remove re-export shims from `internal/execution/` — `complete`

### Phase 2: Remove FillSlot re-export from `internal/starlark/` — `complete`

### Phase 3: Delete dead code in `internal/execution/plan.go` — `complete`

## Related Documents

- [Binding Unification](./binding-unification.md) -- parent plan (all phases complete)
