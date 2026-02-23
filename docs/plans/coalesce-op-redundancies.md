---
title: "Coalesce pkg/op Redundancies"
status: in-progress
created: 2026-02-23
updated: 2026-02-23
---

# Plan: Coalesce pkg/op Redundancies

## Summary

Remove all re-export shims from `internal/execution/` and `internal/starlark/`
so that `pkg/op` is the single authority for its types. This is a greenfield
product with no legacy consumers to support; the shims add indirection without
value and violate the governing principle.

## Goals

1. **Single authority**: All action-framework types (`Action`, `Context`,
   `Result`, `ActionRegistry`, etc.) are imported directly from `pkg/op`.
2. **No re-export shims**: Delete every `type X = op.X` and `var X = op.X`
   alias in `internal/execution/` and `internal/starlark/`.
3. **No dead code**: Remove the duplicate `Orders` method from
   `internal/execution/plan.go` (identical to `DependsOn`, zero callers).

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `pkg/op` canonical types | Working | Single source of truth |
| `internal/execution/action.go` | Shim | Re-exports 6 types from `pkg/op` |
| `internal/execution/registry.go` | Shim | Re-exports `ActionRegistry`, `NewActionRegistry` |
| `internal/execution/provider_registry.go` | Shim | Re-exports `ProviderRegistrar`, `RegisterProvider`, `RegisterAllProviders` |
| `internal/execution/graph.go` | Shim | Re-exports `HydrateGraph` |
| `internal/starlark/output.go` | Shim | Re-exports `FillSlot` |
| `internal/execution/plan.go` Orders method | Dead code | Identical to `DependsOn` |

## Implementation Phases

### Phase 1: Remove re-export shims from `internal/execution/`

Delete these 4 files (pure aliases to `pkg/op`):

| File | Re-exports |
|------|-----------|
| `internal/execution/action.go` | Action, CompensableAction, Context, Result, UndoState, ErrNotCompensable |
| `internal/execution/registry.go` | ActionRegistry, NewActionRegistry |
| `internal/execution/provider_registry.go` | ProviderRegistrar, RegisterProvider, RegisterAllProviders |
| `internal/execution/graph.go` | HydrateGraph |

Update all consumers to use `pkg/op` directly:

- [ ] `internal/execution/executor.go` -- `Context` -> `op.Context`, `CompensableAction` -> `op.CompensableAction`
- [ ] `internal/execution/recovery.go` -- `UndoState` -> `op.UndoState`, `*Context` -> `*op.Context`, `CompensableAction` -> `op.CompensableAction`, `ErrNotCompensable` -> `op.ErrNotCompensable`
- [ ] `internal/execution/hooks.go` -- `*Context` -> `*op.Context`, `Result` -> `op.Result`; add `op` import
- [ ] `internal/execution/plan.go` -- `*ActionRegistry` -> `*op.ActionRegistry`
- [ ] `internal/execution/flow/choose.go` -- `execution.X` -> `op.X` for aliased types; keep native types
- [ ] `internal/execution/flow/gather.go` -- same pattern
- [ ] `internal/execution/flow/elevate.go` -- `execution.Context/Result/UndoState` -> `op.X`
- [ ] `internal/execution/flow/wait_until.go` -- same pattern
- [ ] `internal/execution/flow/register.go` -- `execution.ActionRegistry` -> `op.ActionRegistry`
- [ ] `internal/lore/builder.go` -- `*execution.ActionRegistry` -> `*op.ActionRegistry`
- [ ] `internal/lore/commands.go` -- `execution.NewActionRegistry()` -> `op.NewActionRegistry()`
- [ ] `internal/writ/commands.go` -- `execution.NewActionRegistry()` -> `op.NewActionRegistry()`
- [ ] `internal/writ/graph_builder.go` -- `*execution.ActionRegistry` -> `*op.ActionRegistry`
- [ ] `internal/writ/migrate/plan.go` -- `execution.NewActionRegistry()` -> `op.NewActionRegistry()`
- [ ] `internal/writ/migrate/session.go` -- `execution.NewActionRegistry()` -> `op.NewActionRegistry()`
- [ ] All corresponding `*_test.go` files with the same mechanical replacements

### Phase 2: Remove FillSlot re-export from `internal/starlark/`

**Cross-repo**: Requires template change in `noblefactor-ops.binding-unification`.

- [ ] In `noblefactor-ops`: update `tplPlanFillSlots` in `internal/starlark/receiver_go_gen.go` -- `FillSlot(` -> `op.FillSlot(`
- [ ] Regenerate all 9 `plan_*_gen.go` files in devlore-cli
- [ ] Delete `internal/starlark/output.go`

### Phase 3: Delete dead code in `internal/execution/plan.go`

- [ ] Delete `Orders` method (identical to `DependsOn`, zero callers)

## Files to Create/Modify

| File | Action | Phase |
| --- | --- | --- |
| `internal/execution/action.go` | Delete | 1 |
| `internal/execution/registry.go` | Delete | 1 |
| `internal/execution/provider_registry.go` | Delete | 1 |
| `internal/execution/graph.go` | Delete | 1 |
| `internal/execution/executor.go` | Modify | 1 |
| `internal/execution/recovery.go` | Modify | 1 |
| `internal/execution/hooks.go` | Modify | 1 |
| `internal/execution/plan.go` | Modify | 1, 3 |
| `internal/execution/flow/choose.go` | Modify | 1 |
| `internal/execution/flow/gather.go` | Modify | 1 |
| `internal/execution/flow/elevate.go` | Modify | 1 |
| `internal/execution/flow/wait_until.go` | Modify | 1 |
| `internal/execution/flow/register.go` | Modify | 1 |
| `internal/lore/builder.go` | Modify | 1 |
| `internal/lore/commands.go` | Modify | 1 |
| `internal/writ/commands.go` | Modify | 1 |
| `internal/writ/graph_builder.go` | Modify | 1 |
| `internal/writ/migrate/plan.go` | Modify | 1 |
| `internal/writ/migrate/session.go` | Modify | 1 |
| `internal/starlark/output.go` | Delete | 2 |
| `internal/starlark/plan_*_gen.go` (9 files) | Regenerate | 2 |
| All corresponding `*_test.go` files | Modify | 1 |

## Verification

1. `make build` -- compiles
2. `make test` -- all tests pass
3. `make check` -- vet, lint, test all pass
4. Grep for aliased type usage -- zero matches
5. Grep for unqualified `FillSlot(` in `internal/starlark/` -- zero matches
6. Grep for `legacy|backward|compat|deprecated` -- zero matches

## Related Documents

- [Binding Unification](./binding-unification.md) -- parent plan (all phases complete)
