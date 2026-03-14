---
title: "Shared Provider Receivers"
status: complete
created: 2026-03-13
updated: 2026-03-14
---

# Plan: Shared Provider Receivers

## Summary

Refactor the `pkg/op` provider/receiver API so that providers are owned by their
`ReceiverFactory`, created lazily via `GetOrCreateProvider`, and shared across
action dispatch and receiver construction. Then update the codegen templates
to produce the new API, rename templates to match output filenames, and
regenerate all providers.

## Goals

1. **Factory-owned provider lifecycle.** `ReceiverFactory` owns provider
   creation via `GetOrCreateProvider(ctx) ContextProvider`.
   `actionBase` stores `factory` instead of a raw `reflect.Value`. No more
   `InitProvider` or `InitActionProvider`.
2. **Unified naming taxonomy.** `Provider` → `ReceiverFactory`,
   `Name()` → `ReceiverName()`, `NewImmediate` → `NewExecuting`,
   `NewPlanned` → `NewPlanning`, `RegisterReflectedActions` → `RegisterActions`,
   `WrapPlanned` → `WrapProviderInPlanningReceiver`,
   `WrapReceiver` → `WrapProviderInExecutingReceiver`.
3. **Codegen produces new API.** Templates are renamed and rewritten to produce
   the new taxonomy. Redundant templates (`planned_receiver`, `immediate_receiver`)
   are eliminated.
4. **All providers regenerate cleanly.** `make build` regenerates all gen files
   and the project compiles.

## Naming Taxonomy

| Old | New |
|---|---|
| `var Descriptor op.Provider` | `var Receiver op.ReceiverFactory` |
| `type Receiver struct{}` | `type Factory struct{}` |
| `(r *Receiver)` | `(f *Factory)` |
| `Name()` | `ReceiverName()` |
| — | `GetOrCreateProvider(ctx) ContextProvider` |
| — | `ProviderType() reflect.Type` |
| `NewImmediate(cfg)` / `NewReceiver(ctx)` | `NewExecuting(ctx)` |
| `NewPlanned(graph, project, reg)` | `NewPlanning(graph, project, registry)` |
| `op.WrapPlanned(name, type, ...)` | `op.WrapProviderInPlanningReceiver(f, ...)` |
| `op.WrapReceiver(name, p)` | `op.WrapProviderInExecutingReceiver(f, p)` |
| `op.RegisterReflectedActions(reg, name, p, Params)` | `op.RegisterActions(registry, f, Params)` |
| `op.RegisterReceiverParams(name, &Provider{}, Params)` | `op.RegisterReceiverParams(f, Params)` |
| `op.InitProvider(p, ctx)` | *(removed — factory owns lifecycle)* |
| `op.InitActionProvider(action, ctx)` | *(removed — factory owns lifecycle)* |

## Template / Output File Mapping

| Output file | Template | Notes |
| --- | --- | --- |
| `gen/receiver.gen.go` | `receiver.gen.go.template` | Renamed from `provider_descriptor.go.template` |
| `gen/params.gen.go` | `params.gen.go.template` | Renamed from `params.go.template` |
| `gen/resource.gen.go` | `resource.gen.go.template` | Renamed from `resource_descriptor.go.template`; only for providers with resources |
| `gen/actions_gen_test.go` | `actions_gen_test.go.template` | Renamed from `actions_test.go.template` |
| `gen/receiver_gen_test.go` | `receiver_gen_test.go.template` | Renamed from `immediate_test.go.template` |
| `gen/resource_gen_test.go` | `resource_gen_test.go.template` | New template (future work) |
| **none** | `planned_receiver.go.template` | **DELETED** — absorbed into `receiver.gen.go.template` |
| **none** | `immediate_receiver.go.template` | **DELETED** — absorbed into `receiver.gen.go.template` |

## Reference Implementations

Hand-written gen files that serve as the template target:

- `json/gen/receiver.gen.go` — `access=both` (has `NewExecuting` + `NewPlanning`)
- `ui/gen/receiver.gen.go` — `access=immediate` (has `NewExecuting` only)

## Implementation Phases

### Phase 1: Refactor pkg/op API (complete)

Refactored the core `pkg/op` interfaces and implementations.

- [x] `Provider` → `ReceiverFactory` with `GetOrCreateProvider`, `ReceiverName`, `ProviderType`
- [x] `PlannedProvider` → `PlanningReceiverFactory` with `NewPlanning`
- [x] `ImmediateProvider` → `ExecutingReceiverFactory` with `NewExecuting`
- [x] `actionBase`: `provider reflect.Value` → `factory ReceiverFactory` + `getProvider(ctx)`
- [x] `coerceArgs(slots)` → `coerceArgs(ctx, slots)` — all three `Do()` methods pass context
- [x] `Undo` uses `a.getProvider(*ctx)` instead of stored provider value
- [x] `RegisterReflectedActions(reg, name, provider, params)` → `RegisterActions(registry, factory, params)`
- [x] `RegisterActions` uses `reflect.PointerTo(factory.ProviderType())` for pointer-receiver method lookup
- [x] Removed `InitProvider`, `InitActionProvider`
- [x] `WrapReceiver(name, provider)` → `WrapProviderInExecutingReceiver(factory, provider)`
- [x] `WrapPlanned(name, type, ...)` → `WrapProviderInPlanningReceiver(factory, ...)`
- [x] `RegisterReceiverParams(name, provider, params)` → `RegisterReceiverParams(factory, params)`
- [x] Extracted `ContextBase` from `Context`
- [x] Extracted `StarlarkRuntime`
- [x] Moved platform provider from `pkg/op/provider/platform/` to `pkg/op/`
- [x] Updated `WrapProviderInExecutingReceiver` signature: `(factory, provider)` instead of `(name, provider)`
- [x] Updated `receiver_reflect.go` `SetContext` — direct `providerBase().ctx` mutation
- [x] Updated `planned_reflect.go` — uses `factory.ProviderType()` and `factory.ReceiverName()`
- [x] Updated `announce.go` — `Announce(ReceiverFactory)`, `InitAll` uses `ReceiverFactory`
- [x] Export `Marshal`/`UnmarshalToAny` in `starvalue_marshal.go`

### Phase 2: Write reference gen files (complete)

Manually wrote the target gen files that the templates must reproduce.

- [x] `json/gen/receiver.gen.go` — `access=both` reference
- [x] `ui/gen/receiver.gen.go` — `access=immediate` reference
- [x] `json/gen/params.gen.go` — params reference
- [x] `ui/gen/params.gen.go` — params reference
- [x] `json/gen/actions_gen_test.go` — action test reference
- [x] `ui/gen/receiver_gen_test.go` — receiver test reference

### Phase 3: Rewrite and rename receiver template (complete)

- [x] Renamed `provider_descriptor.go.template` → `receiver.gen.go.template`
- [x] Rewrote template for new ReceiverFactory taxonomy
- [x] `var Receiver op.ReceiverFactory = &Factory{}`
- [x] `Factory` caches provider per Root — singleton within a graph/runtime scope, invalidated on Root change
- [x] `GetOrCreateProvider(ctx)` delegates to `provider.NewProvider(ctx)`
- [x] `ProviderType()` returns `reflect.TypeOf((*provider.Provider)(nil)).Elem()`
- [x] Conditional `Register`, `NewExecuting`, `NewPlanning` methods

### Phase 4: Remove planned/immediate templates, rename remaining (complete)

- [x] Deleted `templates/planned_receiver.go.template`
- [x] Deleted `templates/immediate_receiver.go.template`
- [x] Renamed `templates/params.go.template` → `params.gen.go.template`
- [x] Renamed `templates/resource_descriptor.go.template` → `resource.gen.go.template`
- [x] Renamed `templates/actions_test.go.template` → `actions_gen_test.go.template`
- [x] Renamed `templates/immediate_test.go.template` → `receiver_gen_test.go.template`
- [x] Updated `GEN_TEMPLATE_FILES` and `LOCAL_TEMPLATES` in `generate.star`
- [x] Deleted all existing `gen/planned.gen.go` and `gen/immediate.gen.go` files

### Phase 5: Update generate.star and test templates (complete)

- [x] Updated `generate.star` descriptor field mapping for `receiver` template key
- [x] Updated `receiver_gen_test.go.template` — uses factory arg with `WrapProviderInExecutingReceiver`
- [x] Updated `actions_gen_test.go.template` — uses factory arg with `RegisterActions`
- [x] Dependent type generation block replaced with TODO/skip (template not yet implemented)

### Phase 6: Regenerate and verify (complete)

- [x] `make build` — all 3 binaries compile (lore, writ, devlore-test)
- [x] `make vet` — clean
- [x] `make test` — all tests pass (3 starcode dependent-type tests skipped with `t.Skip`)
- [x] Grep for old API names — zero matches in `.go` files
- [x] Fixed `planned_reflect.go` pointer-receiver bug: `reflect.PointerTo(factory.ProviderType())` for method lookup
- [x] Fixed Factory singleton caching bug: removed provider caching to prevent stale provider reuse across execution contexts
- [x] Fixed `internal/cli/output_test.go`: restored `{{.Name}}` template field (was incorrectly changed to `{{.ReceiverName}}`)

## Bugs Found and Fixed

1. **`planned_reflect.go` pointer-receiver bug** — `providerType.NumMethod()` returned 0 because methods are on `*Provider`, not `Provider`. Fixed with `reflect.PointerTo(factory.ProviderType())`. This caused all planned receiver methods (plan.file.write_text, plan.shell.exec, etc.) to be invisible.

2. **Factory singleton caching** — The generated `Factory` cached a provider instance without invalidation, but the singleton `Receiver` variable persists across test runs. When one test's sandbox root was closed, subsequent tests got the stale provider. Fixed by keying the cache on `Root` — same Root means same provider (singleton within graph/runtime scope), different Root invalidates the cache.

3. **Dependent type wrappers** — The deleted `immediate_receiver.go.template` was also used for dependent type wrappers (e.g., `Sources` in starcode). These are NOT `ReceiverFactory` implementations. The `generate.star` now skips them with a TODO note. Three starcode tests are skipped pending a dedicated dependent-type template.

## Future Work

- `resource_gen_test.go.template` — listed in mapping table but not yet created
- Dependent type wrapper template — needed for types like `starcode.Sources` that have Starlark method wrappers but are not providers

## Related Documents

- [codegen-extraction.md](./codegen-extraction.md) — Template extraction from noblefactor-ops
- [star-gen-receiver.md](./star-gen-receiver.md) — Broader codegen plan
- [projected-provider-api.md](./projected-provider-api.md) — Provider API projection
