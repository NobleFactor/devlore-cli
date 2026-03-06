---
title: "Phase 4: Consumer Migration"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 4: Consumer Migration

## Summary

Replace all callers of `RegisterAllProviders`, `provider.RegisterAll`,
`BindingSet.RegisterActions`, and `BindingSet.NewPopulatedRegistry` with
`op.InitAll`. Wire `BindingSet.BuildGlobals` to read from `op.Providers()`
instead of `op.AllBindings()`.

## Current Call Sites

### Direct `RegisterAllProviders` / `provider.RegisterAll`

| File | Call |
| --- | --- |
| `internal/execution/executor.go:423` | `op.RegisterAllProviders(freshReg, *execCtx)` |
| `internal/execution/executor.go:615` | `op.RegisterAllProviders(freshReg, ctx)` |
| `pkg/op/provider/register.go:28` | `op.RegisterAllProviders(reg, ctx)` |
| `internal/execution/execution_test.go:88` | `provider.RegisterAll(reg, op.Context{})` |
| `internal/execution/lifecycle_test.go` (4 sites) | `provider.RegisterAll(reg, op.Context{})` |
| `internal/lore/builder_test.go` (4 sites) | `provider.RegisterAll(reg, op.Context{})` |
| `internal/writ/migrate/session.go:449` | `provider.RegisterAll(reg, op.Context{})` |
| `internal/writ/migrate/plan.go:373` | `provider.RegisterAll(reg, op.Context{})` |

### Via `BindingSet.NewPopulatedRegistry`

| File | Call |
| --- | --- |
| `internal/writ/commands.go:82` | `bs.NewPopulatedRegistry(op.Context{})` |
| `internal/writ/commands.go:225` | `bs.NewPopulatedRegistry(op.Context{})` |
| `internal/writ/commands.go:474` | `bs.NewPopulatedRegistry(op.Context{})` |
| `internal/lore/commands.go:240` | `bs.NewPopulatedRegistry(op.Context{})` |
| `internal/lore/builder.go:158` | `bs.NewPopulatedRegistry(op.Context{})` |
| `internal/e2e/testrunner/runner.go:100` | `bs.NewPopulatedRegistry(op.Context{})` |

### `BindingSet.BuildGlobals` (reads `AllBindings`)

| File | Call |
| --- | --- |
| `internal/starlark/binding_set.go:72` | `collectPlannedFactories()` → `AllBindings()` |
| `internal/starlark/binding_set.go:84` | `AllBindings()` for `ImmediateFactory` |

## Migration

### 1. `op.InitAll` replaces action registration

All sites that call `RegisterAllProviders`, `provider.RegisterAll`, or
`NewPopulatedRegistry` switch to:

```go
reg := op.NewActionRegistry()
op.InitAll(reg, ctx)
```

### 2. `BindingSet` reads from announced providers

`BindingSet.BuildGlobals` and `collectPlannedFactories` switch from
`op.AllBindings()` to `op.Providers()`, type-asserting for
`PlannedProvider` and `ImmediateProvider`.

### 3. `provider.RegisterAll` becomes a thin wrapper (temporarily)

During this phase, `pkg/op/provider/register.go` still holds the blank
imports that trigger `init()`. Its `RegisterAll` function becomes:

```go
func RegisterAll(reg *op.ActionRegistry, ctx op.Context) {
    op.InitAll(reg, ctx)
}
```

Phase 5 deletes this wrapper entirely.

## Tasks

- [x] Update `BindingSet.RegisterActions` to call `op.InitAll` *(done in Phase 3 bridge)*
- [x] Update `BindingSet.BuildGlobals` to iterate `op.Providers()` with type assertions *(done in Phase 3 bridge)*
- [x] Update `collectPlannedFactories` to iterate `op.Providers()` with type assertions *(done in Phase 3 bridge)*
- [x] Update `BindingSet.resolveProvider` to iterate `op.Providers()` with type assertions *(done in Phase 3 bridge)*
- [x] Update `provider.RegisterAll` to delegate to `op.InitAll` *(done in Phase 3 bridge)*
- [x] Update `binding_set_test.go` to use `op.Announce` instead of `op.RegisterBinding` *(done in Phase 3 bridge)*
- [x] Replace `op.RegisterAllProviders(reg, ctx)` calls in `executor.go` with `op.InitAll(reg, ctx)`
- [x] Replace `provider.RegisterAll(reg, ctx)` calls in test files with `op.InitAll(reg, ctx)`
- [x] Replace `bs.NewPopulatedRegistry(ctx)` calls with `op.NewActionRegistry()` + `op.InitAll(reg, ctx)`
- [x] Remove unused `loreStar` imports from `lore/commands.go`, `writ/commands.go`
- [x] Convert `provider` import to blank import in `migrate/plan.go`, `migrate/session.go`
- [x] Remove unused `provider` import from `lore/builder_test.go`
- [x] Update stale `RegisterBinding` comments to reference `op.Announce()`
- [x] Verify all consumers (`writ`, `lore`, `devlore-test`, `migrate`) work end-to-end
- [x] `make test` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/execution/executor.go` | Modify | Switch to `op.InitAll` |
| `internal/writ/commands.go` | Modify | Switch to `op.InitAll` |
| `internal/lore/commands.go` | Modify | Switch to `op.InitAll` |
| `internal/lore/builder.go` | Modify | Switch to `op.InitAll` |
| `internal/e2e/testrunner/runner.go` | Modify | Switch to `op.InitAll` |
| `internal/writ/migrate/session.go` | Modify | Switch to `op.InitAll` |
| `internal/writ/migrate/plan.go` | Modify | Switch to `op.InitAll` |
| `internal/starlark/binding_set.go` | Modify | Read from `Providers()` |
| `pkg/op/provider/register.go` | Modify | Thin wrapper over `InitAll` |
| `internal/execution/*_test.go` | Modify | Switch to `op.InitAll` |
| `internal/lore/builder_test.go` | Modify | Switch to `op.InitAll` |

## Exit Criteria

- No code calls `RegisterAllProviders`, `AllBindings`, or `BindingByName`
- All action registration flows through `op.InitAll`
- All Starlark globals construction uses `op.Providers()` with type assertions
- All consumers work end-to-end
- All tests pass
