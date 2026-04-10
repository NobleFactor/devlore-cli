---
title: "Lazy Provider Instance Cache on ExecutionContext"
issue: TBD
status: draft
created: 2026-04-02
updated: 2026-04-02
---

# Plan: Lazy Provider Instance Cache on ExecutionContext

## Summary

Add a lazy provider instance cache to `ExecutionContext` so that providers can obtain sibling providers through the context rather than constructing them ad hoc. All provider instantiation — in the executor, the starlark runtime, and cross-provider calls — flows through a single `ExecutionContext.Provider()` method. Each provider type is instantiated at most once per context.

## Goals

1. **Single instantiation path**: All provider construction goes through `ExecutionContext.Provider()` — no direct `NewProvider()` calls or bare `Construct()` invocations outside tests.
2. **Shared lifecycle**: Providers obtained through the same context share the same root, platform, catalog, and other execution state.
3. **Lazy creation**: Providers are only instantiated when first requested, not eagerly at startup.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `ExecutionContext` | Has `Registry` field | No provider instance tracking |
| `prepareCall()` | Calls `rt.Construct()(*ctx)` per action | Fresh instance every call |
| `compensableAction.Undo()` | Calls `rt.Construct()(*ctx)` | Fresh instance for undo |
| `StarlarkRuntime.buildOne()` | Calls `prt.Construct()(&rt.ctx)` | Fresh instance per module |
| `starcode.captureRecursive()` | Calls `file.NewProvider()` directly | Fabricates its own ExecutionContext |
| `migrate/execute.go` | Calls `file.NewProvider()` directly | Fabricates its own ExecutionContext |

## Requirements

### Requirement 1: Provider cache on ExecutionContext

Add a `providers` map and a `Provider(name string)` method to `ExecutionContext`:

```go
type ExecutionContext struct {
    // ...existing fields...
    providers map[string]any
    mu        sync.Mutex
}

func (ctx *ExecutionContext) Provider(name string) (any, error) {
    ctx.mu.Lock()
    defer ctx.mu.Unlock()

    if p, ok := ctx.providers[name]; ok {
        return p, nil
    }

    rt, ok := ctx.Registry.ActionByName(name)
    if !ok {
        return nil, fmt.Errorf("unknown provider: %s", name)
    }

    p, err := rt.Construct()(ctx)
    if err != nil {
        return nil, err
    }

    if ctx.providers == nil {
        ctx.providers = make(map[string]any)
    }
    ctx.providers[name] = p
    return p, nil
}
```

### Requirement 2: Typed convenience for common providers

Callers shouldn't need to type-assert every time. Consider a generic helper or typed wrappers:

```go
func ProviderAs[T any](ctx *ExecutionContext, name string) (T, error) {
    raw, err := ctx.Provider(name)
    if err != nil {
        var zero T
        return zero, err
    }
    p, ok := raw.(T)
    if !ok {
        var zero T
        return zero, fmt.Errorf("provider %s: expected %T, got %T", name, zero, raw)
    }
    return p, nil
}
```

### Requirement 3: Executor must use the cache exclusively

`prepareCall()` and `compensableAction.Undo()` in `pkg/op/action_types.go` must call `ctx.Provider()` instead of `rt.Construct()(*ctx)`.

### Requirement 4: Starlark runtime must use the cache exclusively

`StarlarkRuntime.buildOne()` in `pkg/op/bind/starlark_runtime.go` must call `ctx.Provider()` instead of `prt.Construct()(&rt.ctx)`.

### Requirement 5: Eliminate direct NewProvider calls

- `cmd/star/provider/starcode/provider.go:204` — use `ctx.Provider("file")` instead of `file.NewProvider()`
- `internal/writ/migrate/execute.go:67` — use `ctx.Provider("file")` instead of `file.NewProvider()`

## Implementation Phases

### Phase 1: Add Provider cache to ExecutionContext

- [ ] Add `providers map[string]any` and `mu sync.Mutex` to `ExecutionContext`
- [ ] Implement `Provider(name string) (any, error)` method
- [ ] Implement generic `ProviderAs[T]` helper (if approved)
- [ ] Add tests for lazy instantiation, caching, and concurrent access

**Files**:

- `pkg/op/context.go` — Modify

### Phase 2: Migrate executor and starlark runtime

- [ ] Update `prepareCall()` in `pkg/op/action_types.go` to use `ctx.Provider()`
- [ ] Update `compensableAction.Undo()` in `pkg/op/action_types.go` to use `ctx.Provider()`
- [ ] Update `StarlarkRuntime.buildOne()` in `pkg/op/bind/starlark_runtime.go` to use `ctx.Provider()`
- [ ] Verify tests pass

**Files**:

- `pkg/op/action_types.go` — Modify
- `pkg/op/bind/starlark_runtime.go` — Modify

### Phase 3: Eliminate direct NewProvider calls

- [ ] Update `captureRecursive()` in `cmd/star/provider/starcode/provider.go` to use `ctx.Provider("file")`
- [ ] Update `execute()` in `internal/writ/migrate/execute.go` to use `ctx.Provider("file")`
- [ ] Grep for remaining direct `NewProvider()` calls outside tests and eliminate them

**Files**:

- `cmd/star/provider/starcode/provider.go` — Modify
- `internal/writ/migrate/execute.go` — Modify

## Open Questions

- [ ] Should `prepareCall()` create a fresh provider per action call (current behavior) or reuse a cached instance? Fresh-per-call is safer for providers with mutable state, but most providers are stateless. The plan assumes reuse — if a provider needs fresh state per call, it should manage that internally.
- [ ] Should `compensableAction.Undo()` share the same cached instance as `Do()`? If yes, the provider's state from `Do()` is visible during `Undo()`.
- [ ] Does the `model.NewProvider()` in `internal/model/config.go` need to go through this cache? It's a different kind of provider (LLM backend, not an op provider) and doesn't use `ExecutionContext`.
- [ ] Should the `ProviderConstructor` signature change from `func(ctx ExecutionContext)` to `func(ctx *ExecutionContext)` to avoid copying the context?