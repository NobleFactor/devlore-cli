---
title: "Provider Registration: Announce-and-Callback Model"
status: complete
created: 2026-03-06
updated: 2026-03-06
---

# Plan: Provider Registration

## Summary

Replace the init()-heavy provider registration system with an
announce-and-callback model. Every `init()` function — generated or
handwritten — does exactly one thing: call `op.Announce()` with a zero-value
descriptor. The framework calls back through the `Provider` interface to
register actions, create planned receivers, and create immediate receivers.
Resource providers and flow actions share the same single registration path.
The registration pattern is identical regardless of authorship; a human-written
flow provider descriptor is structurally indistinguishable from a generated
resource provider descriptor.

## Implementation Phases

| | Phase | Name | Description |
| --- | --- | --- | --- |
| [x] | 1 | [Framework kernel](provider-registration/phase-1.md) | `Provider` interface, `Announce`, `InitAll` in `pkg/op` |
| [x] | 2 | [Flow actions](provider-registration/phase-2.md) | Flow provider descriptor, fix #188 |
| [x] | 3 | [Codegen templates](provider-registration/phase-3.md) | Update `star` templates to emit descriptors instead of `RegisterBinding` |
| [x] | 4 | [Consumer migration](provider-registration/phase-4.md) | Replace `RegisterAllProviders`, `BindingSet.RegisterActions`, all call sites |
| [x] | 5 | [Cleanup](provider-registration/phase-5.md) | Remove `ProviderBinding`, `RegisterBinding`, `AllBindings`, dead code |
| [x] | 6 | [Statement-level e2e tests](provider-registration/phase-6.md) | One test per statement type per provider: planned, immediate, flow |
| [x] | 7 | [BindingConfig receivers](provider-registration/phase-7.md) | Move receiver selection into `BindingConfig`; eliminate `With()` |

## Phase Completion Notes

### Phase 1 — Framework Kernel

- Added `Provider`, `PlannedProvider`, `ImmediateProvider` interfaces to `pkg/op/provider.go`
- Renamed existing `Provider` → `ContextProvider` (context injection interface, unexported method)
- Created `pkg/op/announce.go`: `Announce()`, `InitAll()`, `Providers()`, `resetAnnounced()`
- Created `pkg/op/announce_test.go`: 7 tests covering announce, InitAll, type assertions, concurrency

### Phase 2 — Flow Actions

- Created `internal/execution/flow/provider.go`: handwritten descriptor identical to generated pattern
- Deleted `internal/execution/flow/register.go` (old `Register(reg)` free function)
- Added blank import in `pkg/op/provider/register.go` for flow package
- Updated flow tests and `internal/execution/flow_test.go` to use `op.InitAll`

### Phase 3 — Codegen Templates + Consumer Bridge

- Created `provider_descriptor.go.template`; removed `graph_actions.go.template` from pipeline
- Stripped init/RegisterBinding blocks from `planned_receiver.go.template` and `immediate_receiver.go.template`
- Updated `generate.star`: added `provider_descriptor`, removed `graph_actions`, added `compute_descriptor_init()`
- Regenerated all 18 providers with new `provider.gen.go`; deleted all `actions.gen.go` files
- **Bridge fixes (pulled forward from Phase 4):**
  - `provider/register.go`: `RegisterAll` → `op.InitAll`
  - `binding_set.go`: `RegisterActions` → `op.InitAll`; `BuildGlobals`, `resolveProvider`,
    `collectPlannedFactories` → `op.Providers()` with type assertions
  - `binding_set_test.go`: rewritten to use `op.Announce` with test provider types
- Note: `starcode/gen/marshal.go` has a second `init()` registering receiver params — unrelated to
  provider registration. Exit criteria (one `init()` per gen package calling `op.Announce()`) is met.

### Phase 4 — Consumer Migration

- `executor.go`: replaced `op.RegisterAllProviders` (2 sites) with `op.InitAll`
- `lore/commands.go`: replaced `NewPopulatedRegistry` with `op.NewActionRegistry()` + `op.InitAll`; removed `loreStar` import
- `lore/builder.go`: replaced `NewPopulatedRegistry` in `resolve()` with `op.NewActionRegistry()` + `op.InitAll`
- `writ/commands.go`: replaced 3 `NewPopulatedRegistry` calls; removed `loreStar` import
- `writ/migrate/plan.go`, `session.go`: replaced `provider.RegisterAll` with `op.InitAll`; converted `provider` to blank import
- `e2e/testrunner/runner.go`: replaced `bs.NewPopulatedRegistry` with `op.NewActionRegistry()` + `op.InitAll`
- `lore/builder_test.go`: replaced 4 `provider.RegisterAll` calls with `op.InitAll`; removed `provider` import
- Updated stale `RegisterBinding` comments to reference `op.Announce()`
- Zero callers remain for `RegisterAllProviders`, `AllBindings`, or `BindingByName`

### Phase 5 — Cleanup

- Deleted `pkg/op/binding_registry.go` (`ProviderBinding`, `RegisterBinding`, `AllBindings`, `BindingByName`,
  `PlannedFactory`, `ImmediateFactory`, `bindingRegistry` map)
- Deleted `pkg/op/provider_registry.go` (`ProviderRegistrar`, `RegisterAllProviders`)
- Removed `RegisterAll` wrapper from `pkg/op/provider/register.go` (file now contains only blank imports)
- Replaced `PlannedFactory` type usage with `PlannedProvider` interface:
  - Renamed `NewPlanRootFromFactories` → `NewPlanRootFromProviders` (takes `map[string]op.PlannedProvider`)
  - Renamed `collectPlannedFactories` → `collectPlannedProviders` (returns `map[string]op.PlannedProvider`)
- Updated stale comments in `lifetime.go`, `binding_config.go`, `plan_root.go`, `binding_set.go`
- Zero references to any deleted type/function in `.go` files
- `make test` passes, `make test-race` passes

### Phase 6 — Statement-Level E2E Tests

- Added `t.expect_equal(actual, expected)` to TestContext for asserting immediate return values
- Added `runScriptDryRun` helper for providers requiring external resources (archive, encryption, git, net, pkg, service)
- Added `runScriptImm` helper for immediate-only providers (json, yaml, regexp, shell, template, ui, star* providers)
- 24 new test functions covering planned, immediate, and dry-run paths
- Discovered bugs: `file.join` variadic reflection panic, `flow.choose` false-branch executor bug (pre-existing)
- Makefile: fixed json/regexp/yaml `access=both` rules, added `provider.gen.go` to grouped targets, removed phantom `actions.gen.go`
- `register.go`: added 7 missing blank imports (json, regexp, yaml, staranalysis, starcomplexity, starindex, starstats)

### Phase 7 — BindingConfig Receivers

- Added `Receivers []string` to `BindingConfig` — sole source of receiver selection
- `NewBindingSet(cfg)` reads `cfg.Receivers` to populate `included` map
- Deleted `BindingSet.With()` — no callers remain
- Added `WithReceivers(...string)` option to test `Runner`
- Updated all 5 call sites: `lore/builder.go`, `runner.go`, `binding_set_test.go`, `integration_test.go`, `devloretest/commands.go`
- Un-skipped all 12 immediate e2e tests
- Fixed test scripts: struct type assertions, keyword params, bytes literals, shell return semantics
- `make test` and `make test-race` pass

## Goals

1. **Single registration path**: All action sources — resource providers and
   flow actions — register through `op.Announce()` / `op.InitAll()`. No
   parallel systems to keep in sync.
2. **No initialization in init()**: `init()` appends a zero-value descriptor
   to a slice. No factories, no instantiation, no registry population.
3. **Framework controls timing**: `InitAll(reg, ctx)` is called when the
   framework is ready. Providers receive context at callback time, not import
   time.
4. **Observable registration**: Each provider is a struct with methods. You
   read the struct, you see what it registers. One point of entry for
   debugging initialization per use case.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Provider action registration | 42 init() functions calling `op.RegisterBinding()` | Heavy initialization at import time |
| Flow action registration | `flow.Register(reg)` called only in `flow_test.go` | Never called in production — `plan.choose` panics (#188) |
| `ProviderBinding` struct | Function-pointer fields (`ActionRegistrar`, `PlannedFactory`, `ImmediateFactory`) | Nil-checking required |
| `RegisterAllProviders` | Iterates `AllBindings()`, calls each `ActionRegistrar` | Flow actions excluded |
| `BindingSet.RegisterActions` | Duplicates `RegisterAllProviders` behind an indirection | Two paths for the same operation |

## Design

The design is documented in the architecture docs:

- [Projected Provider API — Provider Registration](
  ../architecture/3.2-projected-provider-api.md#provider-registration)
  — Announce/callback model, `Provider` interface, flow actions on the same
  path
- [Provider Loading and Lifetime](
  ../architecture/3.1-provider-loading.md) — Provider lifecycle,
  `Access` and `Lifetime` directives, registration examples
- [Action Namespaces](
  ../architecture/3-operation-namespaces.md) — How to add new
  providers using the announce model

### Core Interfaces

```go
// Required — every provider implements this.
type Provider interface {
    Name() string
    Register(reg *ActionRegistry, ctx Context)
}

// Optional — checked via type assertion.
type PlannedProvider interface {
    NewPlanned(graph *Graph, project string, reg *ActionRegistry) starlark.Value
}

type ImmediateProvider interface {
    NewImmediate(cfg BindingConfig) starlark.Value
}
```

### Framework API

```go
func Announce(p Provider)                    // called in init()
func InitAll(reg *ActionRegistry, ctx Context) // called by framework
func Providers() []Provider                  // introspection
```

## Migration Path

The migration is internal — no user-facing API changes. The sequence:

1. Add new interfaces and `Announce`/`InitAll` alongside existing system
2. Create flow provider descriptor (immediately fixes #188)
3. Regenerate all provider code with new templates
4. Switch all call sites from `RegisterAllProviders` to `InitAll`
5. Delete old registration infrastructure

Each phase leaves the system in a working state. Phase 2 can ship
independently to fix #188.

## Related Documents

- [Projected Provider API](../architecture/3.2-projected-provider-api.md)
  — Authoritative design for the announce-and-callback model
- [Provider Loading and Lifetime](../architecture/3.1-provider-loading.md)
  — Lifetime directives and registration examples
- [Action Namespaces](../architecture/3-operation-namespaces.md)
  — Adding new providers
- [BindingSet Redesign](binding-set-redesign.md)
  — Related plan for BindingSet opt-in model
- [Binding Unification](binding-unification.md)
  — Related plan for receiver/plan generation from providers
- Issue #188 — `flow.choose` not registered in ActionRegistry

## Open Questions

- [ ] Should `Access` and `Lifetime` be fields on the provider descriptor
  struct, or methods on the interface? Fields are simpler; methods allow
  per-method granularity in the future.
- [x] ~~Should `BindingSet` be collapsed into `InitAll` + a simpler globals
  builder, or kept as a separate concern?~~ Resolved in Phase 7: `BindingSet`
  kept as a separate concern. `With()` eliminated; receiver selection moved to
  `BindingConfig.Receivers`. `BuildGlobals` and `ConfigureThread` remain on
  `BindingSet` as the Starlark globals builder.
