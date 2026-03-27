---
title: "Extract starlark infrastructure from pkg/op into pkg/op/bind"
issue: 264
status: in-progress
created: 2026-03-24
updated: 2026-03-25
---

# Plan: Extract Starlark Infrastructure from pkg/op

## Summary

Split `pkg/op` into a starlark-free core (`pkg/op`) and a starlark binding
package (`pkg/op/bind`). Create a plan provider (`pkg/op/provider/plan`) that
implements graph-construction as a regular immediate provider. Flatten the plan
namespace. Consolidate registries. Unify method dispatch.

## Phase Status

| Phase | Status | PR |
|-------|--------|-----|
| 1. Create plan provider, flatten plan namespace | complete | #266 |
| 1.50. Add **kwargs to receiver bridges | complete | #267 |
| 2+3+4. Create bind, move files, sever starlark, update codegen | complete | #268 |
| 5+6. Sever starlark, consolidate registries, unify method dispatch | in-progress | — |

## What Phase 5+6 Covers

### Sever starlark from pkg/op (Phase 5)

- `Context.Thread` → `any`; `bind.ThreadFrom()` accessor
- `ResourceBase.MarshalStarvalue()` removed; `ID()`, `OriginID()` exported;
  StructValue marshals via method dispatch
- `PlanningReceiverFactory`, `ExecutingReceiverFactory` moved to `bind/receiver.go`
- `dependent_type.gen.go.template` updated for bind imports
- `pkg/op` has zero `go.starlark.net` imports in source files

### Consolidate registries (Phase 6)

Two registries:

1. **`ReceiverRegistry`** (`pkg/op/registry.go`) — single registry for
   providers AND resources. Stores `ReceiverFactory` entries and `Method`
   entries. Replaces `ActionRegistry`, `announcedReceivers`,
   `receiverParamsRegistry`, `resourceRegistry`, and `constructorRegistry`.

2. **Type cache** (`pkg/op/bind/registries.go`) — marshal struct introspection
   cache. Bind-only, used for starlark marshaling.

### Unifying Providers and Resources

Providers and Resources are both Go types with methods and properties. The
difference is lifecycle and role:

- **Provider** — lives outside the graph, implements actions, cached by factory
- **Resource** — flows through the graph as data, has properties and methods

Same underpinnings:
- Both have methods described by `Method`
- Both get starlark receivers via the same bridge (`WrapProviderInExecutingReceiver`)
- Both marshal the same way (`StructValue` for properties, method bridges for callables)
- Both register in the same `ReceiverRegistry` with the same `Method` entries

The codegen difference is just what it emits — provider vs resource directives
control factory shape and action registration. The underlying template and
`Method` type are shared.

### Method type

`Method` is a callable — not just metadata. It implements the `Action`
interface for graph dispatch and provides the same information for starlark
bridge construction. One type for both dispatch paths.

```go
// pkg/op/method.go
type Method struct {
    Factory    ReceiverFactory
    Reflect    reflect.Method
    Name       string       // "file.write_text"
    ParamNames []string     // cleaned: no ?, *, **
    Compensate reflect.Method
    Kind       MethodKind   // pure, fallible, compensable
}

// Graph dispatch — implements Action interface
func (m *Method) Do(ctx *Context, slots map[string]any) (Result, Complement, error)
```

Graph execution: nodes reference Methods by action name. The executor looks up
the Method in the registry and calls `Do`.

Starlark bridges: `WrapProviderInExecutingReceiver` and
`WrapProviderInPlanningReceiver` read Method entries to build closures. One
reflection pass at registration. Zero at bridge construction.

Tests: `method.Do(ctx, slots)` — direct dispatch, no receiver wrapper needed.

### ReceiverRegistry

```go
type ReceiverRegistry struct {
    factories map[string]ReceiverFactory  // by receiver name
    methods   map[string]*Method          // by action name
}
```

Provider methods get action names (`"file.write_text"`) for graph dispatch.
Resource methods get type-scoped entries for marshal/receiver construction.
Same `Method` type, same lookup, same bridges.

### ReceiverFactory interface

```go
type ReceiverFactory interface {
    GetOrCreateProvider(ctx Context) ContextProvider
    MethodParams() map[string][]string
    MethodParamsFor(name string) []string
    ProviderType() reflect.Type
    ReceiverName() string
    Register(ctx Context, registry *ReceiverRegistry)
}
```

### Generated receiver (single file)

No separate `params.gen.go`. MethodParams inline in the factory var. One file
per provider gen package:

```go
var Receiver op.ReceiverFactory = &receiverFactory{
    methodParams: bind.MethodParams{
        "WriteText": {"destination", "content", "mode?"},
        ...
    },
}
```

### AttributeResolver

Dynamic sub-namespace routing for the plan Provider:

```go
type AttributeResolver interface {
    ResolveAttr(name string) any
}
```

The bridge checks this via type assertion when an unknown attribute is
requested. The returned value goes through `Marshal`.

## Goals

1. `pkg/op` starlark-free except for Context.StarlarkThread() accessor
2. `pkg/op/bind` owns all starlark binding infrastructure
3. Single `ReceiverRegistry` for providers, resources, and their methods
4. `Method` type shared between graph execution and starlark bridges
5. Providers and Resources unified — same Method, same bridges, same registry
6. Generated receiver code in one file with inline params
7. No `Override` on receivers
8. No `PlanRoot` — plan Provider is a regular ExecutingReceiver with AttributeResolver

## Dependency Model

```
pkg/op/bind
  -> pkg/op                     (core types: Graph, Node, Context, Method, ...)
  -> go.starlark.net/starlark   (starlark runtime)

pkg/op
  -> go.starlark.net/starlark   (Context.StarlarkThread() accessor only)

pkg/op/provider/plan
  -> pkg/op                     (ProviderBase, Graph, Node)
  -> pkg/op/bind                (Promise, PlanningReceiverFactory)

pkg/op/provider/*/gen
  -> pkg/op                     (core types, ReceiverFactory)
  -> pkg/op/bind                (WrapProviderIn*Receiver, MethodParams)
  -> pkg/op/provider/*          (provider implementation)
```
