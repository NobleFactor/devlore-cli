---
title: "Extract starlark infrastructure from pkg/op into pkg/op/bind"
issue: 264
status: draft
created: 2026-03-24
updated: 2026-03-24
---

# Plan: Extract Starlark Infrastructure from pkg/op

## Summary

Split `pkg/op` into a starlark-free core (`pkg/op`) and a starlark binding
package (`pkg/op/bind`). Create a plan provider (`pkg/op/provider/plan`) that
implements graph-construction as a regular immediate provider. Flatten the plan
namespace so all graph-construction primitives are top-level (`plan.choose`,
`plan.complete`, `plan.wait_until`, etc.) and eliminate the `plan.flow`
sub-namespace. After this change, `pkg/op` has zero imports of
`go.starlark.net`.

One commit per phase. `make check` must pass at each phase boundary.

## Goals

1. **`pkg/op` is starlark-free** -- zero `go.starlark.net` imports
2. **`pkg/op/bind` owns all starlark binding infrastructure** -- runtime, marshal, receivers, promise
3. **`bind` has a minimal public API** -- consumers interact with `bind.StarlarkRuntime` and `bind.StarlarkConfig` only; other exported symbols exist for generated receiver code, not for direct use
4. **Plan is a provider** -- `pkg/op/provider/plan` implements graph-construction as a fallible immediate provider; no starlark imports in the provider itself
5. **Flat plan namespace** -- `plan.choose`, `plan.source`, `plan.gather`, `plan.complete`, `plan.degraded`, `plan.fatal`, `plan.wait_until` are all top-level; `plan.flow` sub-namespace eliminated
6. **No circular dependencies** -- `bind` imports `op`, never the reverse
7. **Generated receiver code updated** -- imports `bind` for binding infrastructure

## Non-Goals

- Creating separate Go modules (this is a package-level split within the same module)
- Changing action execution behavior
- Renaming `pkg/op` itself (future work)

## Current State

`pkg/op` has 23 root-level `.go` files. Eleven import `go.starlark.net`:

| Starlark-dependent (moving to bind) | Role                                                               |
| ------------------------------------ | ------------------------------------------------------------------ |
| `starlark_runtime.go`                | StarlarkRuntime, BuildGlobals, ConfigureThread, module loader      |
| `plan_root.go`                       | PlanRoot namespace (choose, source, gather)                        |
| `starvalue_marshal.go`               | Marshal, Unmarshal, CallableResource, buildCallableFunc            |
| `starvalue_struct.go`                | StructValue (starlark.Value wrapper for Go structs)                |
| `receiver.go`                        | receiver base type, builtinFunc                                    |
| `receiver_reflect.go`                | ExecutingReceiver, WrapProviderInExecutingReceiver                 |
| `planned_reflect.go`                 | PlanningReceiver, WrapProviderInPlanningReceiver, FillSlot         |
| `promise.go`                         | Promise (starlark.Value for deferred node output)                  |
| `resource.go`                        | ResourceBase.MarshalStarvalue (one method)                         |
| `context.go`                         | Context.Thread field (one field)                                   |
| `provider.go`                        | PlanningReceiverFactory, ExecutingReceiverFactory (two interfaces) |

Starlark-free files (12) stay in `pkg/op` unchanged.

### Plan namespace today

```
plan.choose(when, then)           # plan_root.go (handwritten starlark)
plan.source(path)                 # plan_root.go (handwritten starlark)
plan.gather(promise, ...)         # plan_root.go (handwritten starlark)
plan.flow.complete(output?)       # flow/planned.go (handwritten starlark, sub-namespace)
plan.flow.degraded(format, ...)   # flow/planned.go (handwritten starlark)
plan.flow.fatal(format, ...)      # flow/planned.go (handwritten starlark)
plan.file.write(...)              # generated via PlanningReceiverFactory
plan.git.clone(...)               # generated via PlanningReceiverFactory
```

### Plan namespace after

```
plan.choose(when, then)                                # provider/plan.Provider method
plan.source(path)                                      # provider/plan.Provider method
plan.gather(promises ...)                               # provider/plan.Provider method
plan.complete(output?)                                  # provider/plan.Provider method
plan.degraded(format, *args, **kwargs)                  # provider/plan.Provider method
plan.fatal(format, *args, **kwargs)                     # provider/plan.Provider method
plan.wait_until(target, predicate, timeout, interval?)  # provider/plan.Provider method
plan.file.write(...)                                    # sub-namespace (immediate receiver)
plan.git.clone(...)                                     # sub-namespace (immediate receiver)
```

## Key Design Decision: Plan as a Provider

Today, PlanRoot is handwritten starlark code -- it directly constructs
`starlark.Value`, `starlark.Builtin`, etc. This makes it starlark
infrastructure that must live in `bind`.

Instead, plan becomes a regular provider at `pkg/op/provider/plan/`. Its methods
are pure Go -- they take Go types, create graph nodes, and return Promises. The
starlark binding is handled by the generated receiver, same as file, json, etc.

```go
// pkg/op/provider/plan/provider.go -- no starlark imports
type Provider struct {
    op.ProviderBase
    graph   *op.Graph
    project string
    reg     *op.ActionRegistry
}

func (p *Provider) Source(resource string) (*op.Promise, error) { ... }
func (p *Provider) Complete(output any) (*op.Promise, error) { ... }
func (p *Provider) Degraded(format string, args ...any) (*op.Promise, error) { ... }
func (p *Provider) Fatal(format string, args ...any) (*op.Promise, error) { ... }
func (p *Provider) WaitUntil(target, predicate any, timeout string) (*op.Promise, error) { ... }
func (p *Provider) Gather(promises ...*op.Promise) ([]*op.Promise, error) { ... }
func (p *Provider) Choose(when *op.Promise, then func() error) (*op.Promise, error) { ... }
```

Plan sub-namespaces (`plan.file`, `plan.git`, etc.) are immediate receivers
returned by the existing PlanningReceiverFactory pattern -- same as PlanRoot
does today. PlanRoot becomes a thin starlark adapter in `bind` that wraps the
plan Provider's immediate receiver and injects sub-namespace receivers as
additional attributes.

**Consequences:**

- `plan_root.go` does NOT move to `bind` as a starlark file. It becomes
  `pkg/op/provider/plan/provider.go` (pure Go).
- PlanRoot in `bind` shrinks to namespace assembly: plan Provider receiver +
  sub-namespace injection. No graph-construction logic.
- `bind` contains no domain logic -- only infrastructure (runtime, marshal,
  receiver base types, promise).
- `choose`'s callable parameter (`then func() error`) is converted from
  `starlark.Callable` by the marshal infrastructure, same as `mem.Callable`.

## Key Design Decision: bind Public API

The `pkg/op/bind` package exposes a minimal public API for consumers:

- `bind.StarlarkRuntime` -- the runtime that executes starlark scripts
- `bind.StarlarkConfig` (or similar) -- configuration for constructing a runtime

Other exported symbols (`WrapProviderInExecutingReceiver`,
`PlanningReceiverFactory`, `Marshal`, etc.) exist solely for generated receiver
code in `gen/` subpackages. They are not part of the intended consumer API.

## Dependency Model

```
pkg/op/bind
  -> pkg/op                     (core types: Graph, Node, Action, Context, ...)
  -> go.starlark.net/starlark   (starlark runtime)

pkg/op
  -> (no starlark imports)

pkg/op/provider/plan
  -> pkg/op                     (Graph, Node, Promise, ActionRegistry)
  -> (no starlark imports)

pkg/op/provider/plan/gen
  -> pkg/op                     (core types)
  -> pkg/op/bind                (binding infrastructure)
  -> pkg/op/provider/plan       (provider implementation)

pkg/op/provider/file/gen
  -> pkg/op                     (core types)
  -> pkg/op/bind                (binding infrastructure)

pkg/op/flow
  -> pkg/op                     (Action interface, Graph, Node)
  -> (no starlark imports -- planned.go deleted)
```

## Entanglement Resolution

Three things in `pkg/op` currently reference starlark types. None are needed
by `op` itself -- they serve the starlark layer. Resolution:

### 1. Context.Thread \*starlark.Thread

`op.Context` is used by every provider. The `Thread` field is only read by
starlark marshal infrastructure (moving to `bind`).

**Resolution:** Change to `Thread any`. The `bind` package provides a typed
accessor:

```go
// pkg/op/bind
func ThreadFrom(ctx op.Context) *starlark.Thread {
    if ctx.Thread == nil { return nil }
    return ctx.Thread.(*starlark.Thread)
}
```

### 2. ResourceBase.MarshalStarvalue()

Method on `op.ResourceBase` that returns `starlark.Value`. Only called by
`starvalue_marshal.go` (moving to `bind`).

**Resolution:** Remove the method from ResourceBase. The marshal dispatch in
`bind` handles ResourceBase directly:

```go
// pkg/op/bind
func marshalResourceBase(b *op.ResourceBase) (starlark.Value, error) { ... }
```

The `starvalue.Marshaler` interface and `starvalue/` subpackage move to `bind`.

### 3. PlanningReceiverFactory / ExecutingReceiverFactory

Interfaces in `op/provider.go` that return `starlark.Value`. Only consumed by
`StarlarkRuntime` (moving to `bind`) and generated receiver code.

**Resolution:** Move both interfaces to `bind`. `op.ReceiverFactory` stays in
`op` (starlark-free). The runtime in `bind` does type assertions against the
`bind`-defined interfaces. Generated code implements interfaces from both
packages.

## Implementation Phases

### Phase 1: Create plan provider and flatten plan namespace

Create `pkg/op/provider/plan/` with a Provider whose methods implement
graph-construction as pure Go. Move `complete`, `degraded`, `fatal` from
`flow.Plan` into the plan Provider. Implement `wait_until`. Move `choose`,
`source`, `gather` from `plan_root.go` into the plan Provider.

Update PlanRoot to delegate to the plan Provider (wrapping it as an immediate
receiver) while retaining sub-namespace dispatch. Remove `flow.Plan`,
`flow.NewFlowPlan`, and `flowProvider.NewPlanning`.

- [ ] Create `pkg/op/provider/plan/provider.go` with Provider struct
- [ ] Implement Source, Complete, Degraded, Fatal, WaitUntil, Gather, Choose
- [ ] Move `fillListSlot`, `fillDictSlot` helpers to plan provider
- [ ] Update PlanRoot to delegate top-level methods to plan Provider
- [ ] Remove `flow.Plan`, `flow.NewFlowPlan` (`flow/planned.go`)
- [ ] Remove `PlanningReceiverFactory` from `flowProvider`
- [ ] Update tests -- `plan.flow.complete` -> `plan.complete`, etc.
- [ ] Verify: `make check` passes

**Files**:

| File | Action |
| --- | --- |
| `pkg/op/provider/plan/provider.go` | Create -- plan Provider with graph-construction methods |
| `pkg/op/plan_root.go` | Modify -- delegate to plan Provider, keep sub-namespace routing |
| `pkg/op/flow/planned.go` | Delete -- methods moved to plan Provider |
| `pkg/op/flow/provider.go` | Modify -- remove NewPlanning, remove starlark import |
| `pkg/op/flow/flow_test.go` | Modify -- update tests |
| `pkg/op/flow/integration_test.go` | Modify -- use plan.complete instead of plan.flow.complete |

### Phase 2: Create `pkg/op/bind` package

Create the package and move starlark binding infrastructure from `pkg/op`.
PlanRoot (now a thin namespace adapter) moves here.

**File mapping**:

| Source (`pkg/op/`) | Destination (`pkg/op/bind/`) |
| --- | --- |
| `starlark_runtime.go` | `runtime.go` |
| `plan_root.go` | `plan_root.go` (thin: namespace assembly only) |
| `starvalue_marshal.go` | `marshal.go` |
| `starvalue_struct.go` | `struct.go` |
| `starvalue/starvalue.go` | absorbed into `bind/` (Marshaler, Unmarshaler interfaces) |
| `receiver.go` | `receiver.go` |
| `receiver_reflect.go` | `receiver_reflect.go` |
| `planned_reflect.go` | `planned_reflect.go` |
| `promise.go` | `promise.go` |

Move tests alongside their source files.

- [ ] Create `pkg/op/bind/` package
- [ ] Move files (git mv + package rename + add `op.` prefix for core types)
- [ ] Move `PlanningReceiverFactory`, `ExecutingReceiverFactory` to `bind`
- [ ] Move `starvalue.Marshaler`, `starvalue.Unmarshaler` to `bind`
- [ ] Verify: `make vet` passes

### Phase 3: Sever starlark references from `pkg/op`

Remove the three starlark entanglements from `op` core.

- [ ] `context.go`: Change `Thread *starlark.Thread` to `Thread any`
- [ ] `resource.go`: Remove `MarshalStarvalue()` method from ResourceBase
- [ ] `resource.go`: Remove `go.starlark.net` imports
- [ ] `provider.go`: Remove `PlanningReceiverFactory`, `ExecutingReceiverFactory` (now in bind)
- [ ] `provider.go`: Remove `go.starlark.net` import
- [ ] `bind/marshal.go`: Add `marshalResourceBase` function
- [ ] `bind/runtime.go`: Add `ThreadFrom` accessor
- [ ] Delete `pkg/op/starvalue/` (absorbed into bind)
- [ ] Verify: `pkg/op` has zero `go.starlark.net` imports
- [ ] Verify: `make vet` passes

### Phase 4: Update generated code and code generator

Update the code generator to emit imports from `pkg/op/bind` instead of
`pkg/op` for binding infrastructure. Generate receiver for the plan provider.

- [ ] Update generator templates: `WrapProviderInExecutingReceiver` -> `bind.WrapProviderInExecutingReceiver`
- [ ] Update generator templates: `WrapProviderInPlanningReceiver` -> `bind.WrapProviderInPlanningReceiver`
- [ ] Update generator templates: `RegisterActions` -> `bind.RegisterActions`
- [ ] Update generator templates: `RegisterReceiverParams` -> `bind.RegisterReceiverParams`
- [ ] Update generator templates: `PlanningReceiverFactory` -> `bind.PlanningReceiverFactory`
- [ ] Update generator templates: `ExecutingReceiverFactory` -> `bind.ExecutingReceiverFactory`
- [ ] Generate receiver for `pkg/op/provider/plan/gen/`
- [ ] Regenerate all receiver code
- [ ] Verify: `make check` passes

### Phase 5: Update consumers and verify

Update all consumers of moved types.

- [ ] Update `cmd/lore/lore/` imports
- [ ] Update `cmd/devlore-test/devloretest/` imports
- [ ] Update `cmd/star/star/` imports
- [ ] Update `internal/execution/` imports
- [ ] Update `pkg/op/flow/` imports (if any remain)
- [ ] Update `pkg/op/provider/mem/` imports (callable, extract)
- [ ] Grep for stale import paths -- zero matches
- [ ] `make check` passes from repo root
- [ ] Verify `pkg/op` has zero `go.starlark.net` in imports: `grep -r 'go.starlark.net' pkg/op/*.go` returns nothing

## Risks

| Risk | Mitigation |
| --- | --- |
| `Context.Thread any` loses type safety | Single typed accessor `bind.ThreadFrom` centralizes the assertion; compile-time safety at the boundary |
| Generated code imports both `op` and `bind` | No cycle -- gen packages are leaves; generator templates updated in Phase 4 |
| `choose`'s callable parameter | Marshal infrastructure already converts starlark.Callable to Go func via `buildCallableFunc`; plan Provider accepts `func() error` |
| `mem.Callable` imports starlark for `starlark.Thread` | Already in provider subpackage; imports `bind` instead of `op` for marshal types |
| Large diff across generated files | Phase 4 regeneration is mechanical -- verify with `go generate ./... && git diff` |
| Plan Provider needs Graph and ActionRegistry not on Context | Provider constructor receives these; they're available at plan-time construction |
