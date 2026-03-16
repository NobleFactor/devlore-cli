---
title: "Typed AccessType on ReceiverFactory"
issue: TBD
status: draft
created: 2026-03-16
updated: 2026-03-16
---

# Plan: Typed AccessType on ReceiverFactory

## Summary

The `AccessType` constants (`AccessImmediate`, `AccessPlanned`, `AccessBoth`) are the canonical
vocabulary for the `// +devlore:access=` directive system, but nothing programmatically references
them. The Starlark generator hardcodes `["immediate", "planned", "both"]`, and the runtime infers
access via type assertions on `ExecutingReceiverFactory` / `PlanningReceiverFactory`.

All 19 generated receiver factories repeat ~60 lines of identical boilerplate. The only
provider-specific information is: name, type, access, params, and constructor. Everything else —
`GetOrCreateProvider`, `Register`, `NewExecuting`, `NewPlanning` — is mechanical dispatch
determined entirely by the access type.

One handwritten `ReceiverFactory` exists: `flowProvider` in `internal/execution/flow`. It
registers flow-control actions (`Choose`, `Gather`, `Complete`, etc.) that are framework
primitives, not domain providers. It abuses the provider registration mechanism — returning `nil`
from `GetOrCreateProvider`, manually calling `reg.Register()`, and providing a custom `Plan`
struct for planning. Phase 0 moves flow into `pkg/op` as a built-in, eliminating the only
handwritten `ReceiverFactory` and making the "100% generated" invariant true.

With that prerequisite satisfied, this plan wires `AccessType` into a slim `ReceiverFactory`
interface, moves dispatch into a table-driven framework keyed by access type, and reduces each
generated factory from ~90 lines to ~30 lines.

## Goals

1. **Flow as framework built-in**: Move flow-control actions from `internal/execution/flow` into
   `pkg/op`. The framework registers them directly — no `ReceiverFactory` needed.
2. **Single source of truth**: `pkg/op/access.go` defines valid access values; all consumers
   reference these constants instead of hardcoded strings.
3. **Slim factory interface**: `ReceiverFactory` declares only provider-specific metadata
   (`ReceiverName`, `ProviderType`, `AccessType`, `MethodParams`, `NewProvider`). The framework
   provides all dispatch behavior.
4. **Table-driven dispatch**: An `accessSpecs` map keyed by `AccessType` defines registration,
   executing, and planning behavior. Adding a new access type = adding one map row.
5. **Eliminate optional interfaces**: `ExecutingReceiverFactory` and `PlanningReceiverFactory`
   are replaced by the dispatch table. No generated `NewExecuting`/`NewPlanning` methods.
6. **Generator validation**: The Starlark generator validates `// +devlore:access=` directives
   against the Go constants via `goast.const_groups()`.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `AccessType` constants | Defined, unused | `pkg/op/access.go` — never referenced in Go code |
| `ReceiverFactory` interface | 4 methods | `GetOrCreateProvider`, `ReceiverName`, `ProviderType`, `Register` |
| `ExecutingReceiverFactory` | Optional interface | Type-asserted in `buildOne()` |
| `PlanningReceiverFactory` | Optional interface | Type-asserted in `collectPlannedProviders()` |
| Generated factories | 19, all generated | ~90 lines each, ~60 lines boilerplate |
| `flowProvider` | Handwritten | Only non-generated `ReceiverFactory`; abuses provider interface |
| Flow actions | `internal/execution/flow` | 7 actions, custom `Plan`, depends on `execution.OrderNodes` |
| Starlark generator | Hardcoded strings | `generate.star:134` validates against `["immediate", "planned", "both"]` |
| Provider caching | In generated `Factory` struct | `GetOrCreateProvider` caches by `Root` |

### Current generated factory (json, access=both)

```go
type Factory struct {
    provider *provider.Provider
    root     op.Root
}

func (f *Factory) GetOrCreateProvider(ctx op.Context) op.ContextProvider { /* cache-by-root */ }
func (f *Factory) ProviderType() reflect.Type { /* reflect */ }
func (f *Factory) ReceiverName() string { return "json" }
func (f *Factory) NewExecuting(ctx op.Context) starlark.Value { /* one-liner */ }
func (f *Factory) NewPlanning(...) starlark.Value { /* one-liner */ }
func (f *Factory) Register(registry *op.ActionRegistry, ctx op.Context) {
    op.RegisterActions(registry, f, Params)
    op.RegisterReceiverParams(f, Params)
}
```

### Target generated factory

```go
type Factory struct{}

func (f *Factory) ReceiverName() string                          { return "json" }
func (f *Factory) ProviderType() reflect.Type                    { return reflect.TypeOf((*provider.Provider)(nil)).Elem() }
func (f *Factory) AccessType() op.AccessType                     { return op.AccessBoth }
func (f *Factory) MethodParams() op.MethodParams                 { return Params }
func (f *Factory) NewProvider(ctx op.Context) op.ContextProvider { return provider.NewProvider(ctx) }
```

## Requirements

### R0: Move flow actions into `pkg/op`

The flow package (`internal/execution/flow`) contains 7 handwritten actions and a custom `Plan`
for Starlark planning. It depends on two functions from `internal/execution`:

- `OrderNodes(nodes, edges)` — topological sort using Kahn's algorithm; operates only on
  `op.Node` / `op.Edge`
- `FillSlotsFromData(slots, data)` — 6-line slot filler; operates on `map[string]any`

Both are graph utilities that belong in `pkg/op`. Moving them eliminates the
`internal/execution` dependency, allowing flow to live in `pkg/op`.

**What moves:**

| From | To | Contents |
| --- | --- | --- |
| `internal/execution/executor.go` | `pkg/op/graph_order.go` | `OrderNodes`, `FillSlotsFromData`, `topologicalSortNodes`, `sortNodesByDepth` |
| `internal/execution/flow/choose.go` | `pkg/op/flow_choose.go` | `Choose`, `chooseComplement` |
| `internal/execution/flow/gather.go` | `pkg/op/flow_gather.go` | `Gather`, `gatherComplement`, `iterationUndo`, `iterOutcome`, concurrency helpers |
| `internal/execution/flow/complete.go` | `pkg/op/flow_complete.go` | `Complete` |
| `internal/execution/flow/degraded.go` | `pkg/op/flow_degraded.go` | `Degraded`, `reassembleArgs`, `reassembleKwargs` |
| `internal/execution/flow/fatal.go` | `pkg/op/flow_fatal.go` | `Fatal` |
| `internal/execution/flow/elevate.go` | `pkg/op/flow_elevate.go` | `Elevate` |
| `internal/execution/flow/wait_until.go` | `pkg/op/flow_wait_until.go` | `WaitUntil`, `PredicateFunc`, `parseDurationSlot` |
| `internal/execution/flow/planned.go` | `pkg/op/flow_plan.go` | `Plan`, `NewFlowPlan`, `fillListSlot`, `fillDictSlot` |
| `internal/execution/flow/provider.go` | deleted | `flowProvider` replaced by direct framework registration |
| `internal/execution/flow/flow_test.go` | `pkg/op/flow_test.go` | All flow tests (tests only import `pkg/op`, no changes needed) |

**What changes in `internal/execution`:**

- `executor.go` calls its local `OrderNodes` / `FillSlotsFromData`. After the move, these become
  `op.OrderNodes` / `op.FillSlotsFromData`. Since `executor.go` already imports `pkg/op`, this
  is a one-line change per call site.

**Flow registration without `ReceiverFactory`:**

The framework registers flow actions directly in `InitAll`:

```go
func registerFlowActions(reg *ActionRegistry) {
    reg.Register(&Choose{})
    reg.Register(&Gather{})
    reg.Register(&Elevate{})
    reg.Register(&WaitUntil{})
    reg.Register(&Complete{})
    reg.Register(&Degraded{})
    reg.Register(&Fatal{})
}
```

The flow `Plan` is built directly by the framework's planning infrastructure when constructing
the `plan.flow` namespace — no `PlanningReceiverFactory` type assertion needed.

### R1: Slim ReceiverFactory interface

With `flowProvider` eliminated, all `ReceiverFactory` implementations are generated. The
interface slims to 5 metadata-only methods:

```go
type ReceiverFactory interface {
    ReceiverName() string
    ProviderType() reflect.Type
    AccessType() AccessType
    MethodParams() MethodParams
    NewProvider(ctx Context) ContextProvider
}
```

### R2: Table-driven dispatch

A single `accessSpecs` map defines the behavioral semantics of each `AccessType`. Shared
building-block functions are composed into specs — no duplication.

```go
type accessSpec struct {
    register  func(reg *ActionRegistry, f ReceiverFactory)
    executing func(f ReceiverFactory, ctx Context) starlark.Value
    planning  func(f ReceiverFactory, g *Graph, project string, reg *ActionRegistry) starlark.Value
}

func wrapExecuting(f ReceiverFactory, ctx Context) starlark.Value {
    return WrapProviderInExecutingReceiver(f, f.NewProvider(ctx))
}

func wrapPlanning(f ReceiverFactory, g *Graph, project string, reg *ActionRegistry) starlark.Value {
    return WrapProviderInPlanningReceiver(f, g, project, reg, f.MethodParams())
}

func registerParams(reg *ActionRegistry, f ReceiverFactory) {
    RegisterReceiverParams(f, f.MethodParams())
}

func registerActions(reg *ActionRegistry, f ReceiverFactory) {
    RegisterActions(reg, f, f.MethodParams())
}

func registerBoth(reg *ActionRegistry, f ReceiverFactory) {
    registerActions(reg, f)
    registerParams(reg, f)
}

var accessSpecs = map[AccessType]accessSpec{
    AccessImmediate: {register: registerParams,  executing: wrapExecuting, planning: nil},
    AccessPlanned:   {register: registerActions, executing: nil,           planning: wrapPlanning},
    AccessBoth:      {register: registerBoth,    executing: wrapExecuting, planning: wrapPlanning},
}
```

Dispatch at each call site:

```go
func registerFactory(reg *ActionRegistry, f ReceiverFactory) {
    spec, ok := accessSpecs[f.AccessType()]
    if !ok {
        panic(fmt.Sprintf("%s: unknown AccessType %q", f.ReceiverName(), f.AccessType()))
    }
    spec.register(reg, f)
}

func buildExecuting(f ReceiverFactory, ctx Context) starlark.Value {
    if spec := accessSpecs[f.AccessType()]; spec.executing != nil {
        return spec.executing(f, ctx)
    }
    return nil
}

func buildPlanning(f ReceiverFactory, g *Graph, project string, reg *ActionRegistry) starlark.Value {
    if spec := accessSpecs[f.AccessType()]; spec.planning != nil {
        return spec.planning(f, g, project, reg)
    }
    return nil
}
```

### R3: Provider caching moves to framework

`GetOrCreateProvider` caching moves to the framework. The `announced` registry holds a
`factoryEntry` that wraps the `ReceiverFactory` with cached state:

```go
type factoryEntry struct {
    factory  ReceiverFactory
    cached   ContextProvider
    root     Root
}

func (e *factoryEntry) getOrCreate(ctx Context) ContextProvider {
    if e.cached == nil || e.root != ctx.Root {
        e.cached = e.factory.NewProvider(ctx)
        e.root = ctx.Root
    }
    return e.cached
}
```

The generated factory struct becomes stateless (`type Factory struct{}`).

### R4: Delete optional interfaces

`ExecutingReceiverFactory` and `PlanningReceiverFactory` are deleted from `pkg/op/provider.go`.
All dispatch is table-driven via `accessSpecs`.

### R5: Slim receiver template

`receiver.gen.go.template` shrinks to emit only the five metadata methods. All conditional
blocks (`{{if .has_immediate}}` / `{{if .has_planned}}` / `{{if .has_actions}}`) are removed.
The `Factory` struct becomes empty. The `starlark` import is removed.

### R6: Validate in Starlark generator

Replace the hardcoded validation in `struct_access()` (`generate.star:134`) using the existing
`goast.const_groups()` builtin.

```python
# Before:
if value not in ["immediate", "planned", "both"]:

# After — validate against canonical constants from pkg/op/access.go:
_access_groups = goast.const_groups("pkg/op/access.go", type_name="AccessType")
_valid_access = [c.value for c in _access_groups[0].constants]

def struct_access(path):
    # ...parse directive...
    if value not in _valid_access:
        fail("invalid +devlore:access value %r (valid: %s)" % (value, ", ".join(_valid_access)))
    return value
```

## Implementation Phases

### Phase 0: Move flow into `pkg/op` (prerequisite)

Eliminates the only handwritten `ReceiverFactory`, making the "100% generated" invariant true.

- [ ] Move `OrderNodes`, `FillSlotsFromData`, `topologicalSortNodes`, `sortNodesByDepth` from
      `internal/execution/executor.go` to `pkg/op/graph_order.go`
- [ ] Update `internal/execution/executor.go` call sites to use `op.OrderNodes` / `op.FillSlotsFromData`
- [ ] Move all 7 flow action files from `internal/execution/flow/` to `pkg/op/flow_*.go`
- [ ] Move `Plan` and helpers from `internal/execution/flow/planned.go` to `pkg/op/flow_plan.go`
- [ ] Move flow tests from `internal/execution/flow/flow_test.go` to `pkg/op/flow_test.go`
- [ ] Delete `internal/execution/flow/provider.go` (`flowProvider`)
- [ ] Add `registerFlowActions(reg)` call in `InitAll`
- [ ] Wire `NewFlowPlan` into the planning infrastructure (where `collectPlannedProviders`
      currently type-asserts `flowProvider` as `PlanningReceiverFactory`)
- [ ] Delete `internal/execution/flow/` directory
- [ ] `make check` passes

**Files**:

- `pkg/op/graph_order.go` — Create (moved from `internal/execution/executor.go`)
- `pkg/op/flow_choose.go` — Create (moved from `internal/execution/flow/choose.go`)
- `pkg/op/flow_gather.go` — Create (moved from `internal/execution/flow/gather.go`)
- `pkg/op/flow_complete.go` — Create (moved from `internal/execution/flow/complete.go`)
- `pkg/op/flow_degraded.go` — Create (moved from `internal/execution/flow/degraded.go`)
- `pkg/op/flow_fatal.go` — Create (moved from `internal/execution/flow/fatal.go`)
- `pkg/op/flow_elevate.go` — Create (moved from `internal/execution/flow/elevate.go`)
- `pkg/op/flow_wait_until.go` — Create (moved from `internal/execution/flow/wait_until.go`)
- `pkg/op/flow_plan.go` — Create (moved from `internal/execution/flow/planned.go`)
- `pkg/op/flow_test.go` — Create (moved from `internal/execution/flow/flow_test.go`)
- `internal/execution/executor.go` — Modify (use `op.OrderNodes`, `op.FillSlotsFromData`)
- `internal/execution/flow/` — Delete (entire directory)
- `pkg/op/announce.go` — Modify (add `registerFlowActions` to `InitAll`)
- `internal/starlark/runtime.go` — Modify (wire `NewFlowPlan` directly, not via type assertion)

### Phase 1: Atomic interface + dispatch + template change

All `ReceiverFactory` implementations are now generated. Change everything atomically.

- [ ] Slim `ReceiverFactory` to 5 methods in `pkg/op/provider.go`
- [ ] Delete `ExecutingReceiverFactory` and `PlanningReceiverFactory` from `pkg/op/provider.go`
- [ ] Create `pkg/op/access_dispatch.go` with `accessSpec`, building blocks, `accessSpecs` map,
      and dispatch functions (`registerFactory`, `buildExecuting`, `buildPlanning`)
- [ ] Add `factoryEntry` with `getOrCreate()` to `pkg/op/announce.go`
- [ ] Update `InitAll` to call `registerFactory` instead of `p.Register(reg, ctx)`
- [ ] Update `StarlarkRuntime.buildOne()` to call `buildExecuting()`
- [ ] Update `collectPlannedProviders()` in `internal/starlark/runtime.go` to use `AccessType()`
- [ ] Update `internal/starlark/plan_root.go` to use `buildPlanning()`
- [ ] Update action execution to use `factoryEntry.getOrCreate()` for provider caching
- [ ] Slim `receiver.gen.go.template` to 5 metadata methods, empty struct, no starlark import
- [ ] Regenerate all 19 provider `receiver.gen.go` files
- [ ] `make check` passes

**Files**:

- `pkg/op/provider.go` — Modify (slim interface, delete optional interfaces)
- `pkg/op/access_dispatch.go` — Create (dispatch table + building blocks)
- `pkg/op/announce.go` — Modify (`factoryEntry`, `registerFactory`, update `InitAll`)
- `pkg/op/starlark_runtime.go` — Modify (replace `buildOne`)
- `internal/starlark/runtime.go` — Modify (replace `collectPlannedProviders`)
- `internal/starlark/plan_root.go` — Modify (use `buildPlanning`)
- `star/.../templates/receiver.gen.go.template` — Modify (strip to 5 methods)
- `pkg/op/provider/*/gen/receiver.gen.go` — Regenerate (19 files)

### Phase 2: Generator validation

- [ ] Replace hardcoded `["immediate", "planned", "both"]` in `struct_access()` with
      `goast.const_groups("pkg/op/access.go", type_name="AccessType")`
- [ ] Verify generator still produces identical output for all 19 providers
- [ ] `make check` passes

**Files**:

- `star/.../commands/generate.star` — Modify

### Phase 3: Verify

- [ ] `make check` passes
- [ ] Verify generated factory files are ~30 lines each
- [ ] Verify provider caching behavior is identical (test with Root changes)
- [ ] Verify no code references deleted interfaces
- [ ] Verify `internal/execution/flow/` directory is gone
- [ ] Verify flow actions are registered and execute correctly

## Cross-Repo Impact

**noblefactor-ops**: The `goast` provider at `internal/provider/goast/gen/receiver.gen.go` is
also generated from the same template pattern. After devlore-cli merges the interface change,
noblefactor-ops must regenerate its goast factory to satisfy the new `ReceiverFactory` interface.
Same template change, same regeneration — processed as a downstream update after this plan merges.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/graph_order.go` | Create | `OrderNodes`, `FillSlotsFromData`, topo sort |
| `pkg/op/flow_choose.go` | Create | `Choose` action |
| `pkg/op/flow_gather.go` | Create | `Gather` action |
| `pkg/op/flow_complete.go` | Create | `Complete` action |
| `pkg/op/flow_degraded.go` | Create | `Degraded` action |
| `pkg/op/flow_fatal.go` | Create | `Fatal` action |
| `pkg/op/flow_elevate.go` | Create | `Elevate` action |
| `pkg/op/flow_wait_until.go` | Create | `WaitUntil` action |
| `pkg/op/flow_plan.go` | Create | `Plan`, `NewFlowPlan`, slot helpers |
| `pkg/op/flow_test.go` | Create | Flow action tests |
| `pkg/op/access.go` | Keep | Constants become referenced by dispatch table |
| `pkg/op/access_dispatch.go` | Create | `accessSpec`, building blocks, dispatch table |
| `pkg/op/provider.go` | Modify | Slim to 5-method interface, delete optional interfaces |
| `pkg/op/announce.go` | Modify | `factoryEntry`, `registerFactory`, `registerFlowActions` |
| `pkg/op/starlark_runtime.go` | Modify | Replace `buildOne` with `buildExecuting` |
| `internal/execution/executor.go` | Modify | Use `op.OrderNodes`, `op.FillSlotsFromData` |
| `internal/execution/flow/` | Delete | Entire directory |
| `internal/starlark/runtime.go` | Modify | Replace `collectPlannedProviders`, wire flow `Plan` |
| `internal/starlark/plan_root.go` | Modify | Use `buildPlanning` |
| `receiver.gen.go.template` | Modify | Strip to 5 metadata methods |
| `pkg/op/provider/*/gen/receiver.gen.go` | Regenerate | 19 generated files |
| `generate.star` | Modify | Replace hardcoded validation strings |

## Related Documents

- [surface-area-refactor.md](./surface-area-refactor.md) — Parent plan; removes `AccessType`
  from dead-exports list since this plan makes them live
- [goland-inspection-cleanup.md](./goland-inspection-cleanup.md) — Phase 5 dead-code overlap

## Open Questions

- [x] ~~Does the `goast` Starlark module support `const_values()` or equivalent?~~ **Resolved**:
  `goast.const_groups(path, type_name=)` exists and returns `ConstGroupResult` with `constants`
  list, each having `name` and `value`. No new goast method needed.
- [x] ~~Are there handwritten `ReceiverFactory` implementations?~~ **Resolved**: One —
  `flowProvider` in `internal/execution/flow`. Phase 0 moves flow into `pkg/op` as framework
  built-ins, eliminating the handwritten implementation.
- [ ] The `WrapProviderInExecutingReceiver` and `WrapProviderInPlanningReceiver` functions
  currently take a `ReceiverFactory` parameter. After the interface change, do their signatures
  need adjustment?
- [ ] Should the `LifetimeType` constants follow the same pattern (add `LifetimeType()` to
  `ReceiverFactory`) once provider lifetimes are implemented?
