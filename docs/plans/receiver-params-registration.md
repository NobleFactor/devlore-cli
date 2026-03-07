---
title: "Receiver Params Registration"
status: draft
created: 2026-03-06
updated: 2026-03-06
---

# Plan: Receiver Params Registration

## Summary

The generated `Register()` callback already passes `Params` to
`RegisterReflectedActions`. That function should also store them in the
receiver params registry as a side effect. Then `WrapReceiver` looks up
params from the registry instead of taking them as an argument, and the
generated `immediate.gen.go` simplifies — `Params` is no longer passed
at immediate-receiver construction time.

## Goals

1. **Unified registration**: `RegisterReflectedActions` registers both
   actions and receiver params in one call. No new code in generated
   `Register()` — it already passes everything needed.
2. **Auto-wrapping**: `marshalReflect` wraps provider struct pointers as
   receivers instead of flattening to field-only structs.
3. **Simpler immediate codegen**: `WrapReceiver` drops its `params`
   argument. `NewXReceiver` in `immediate.gen.go` simplifies.

## Current State

| Component                        | Status                       | Notes                                   |
|----------------------------------|------------------------------|-----------------------------------------|
| `RegisterReceiverParams[T]`      | Defined, never called        | `starvalue_marshal.go:95`               |
| `receiverParamsRegistry`         | Empty at runtime             | `starvalue_marshal.go:85`               |
| `RegisterReflectedActions`       | Receives `Params`, doesn't store them | `action_reflect.go:409` |
| `WrapReceiver`                   | Takes `params MethodParams` arg | `receiver_reflect.go:49` |
| Generated `immediate.gen.go`     | Passes `Params` to `WrapReceiver` | Redundant with `Register()` |
| `marshalReflect` auto-wrap check | Dead path                    | Checks registry but never finds entries |

## Implementation Phases

|     | Phase | Name                                                                              | Description                                                                                  |
|-----|-------|-----------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| [ ] | 1     | [Store params + simplify](receiver-params-registration/phase-1.md) | `RegisterReflectedActions` stores params; `WrapReceiver` drops `params` arg; regenerate |

## Design

### RegisterReflectedActions stores params

`RegisterReflectedActions` already receives `(reg, name, provider, params)`.
Add one call at the top:

```go
func RegisterReflectedActions(reg *ActionRegistry, name string, provider any, params MethodParams) {
    registerReceiverParamsReflect(name, provider, params)
    // ... existing action registration logic
}
```

`registerReceiverParamsReflect` is the non-generic internal form of
`RegisterReceiverParams` — it stores `(reflect.TypeOf(provider).Elem(), name, params)`
in the registry using the runtime type of the provider pointer.

### WrapReceiver looks up params

`WrapReceiver` drops its `params` argument and looks up from the registry:

```go
func WrapReceiver(name string, provider any) *ReflectedReceiver {
    entry, ok := lookupReceiverParams(reflect.TypeOf(provider))
    if !ok {
        panic(fmt.Sprintf("WrapReceiver: no params registered for %s", name))
    }
    // ... existing bridge-building logic using entry.params
}
```

### Generated code — no changes to Register()

The generated `Register()` is already correct:

```go
func (d *fileProvider) Register(reg *op.ActionRegistry, ctx op.Context) {
    p := &provider.Provider{}
    op.InitProvider(p, ctx)
    op.RegisterReflectedActions(reg, "file", p, Params)
}
```

No changes needed. The params are stored as a side effect of the existing call.

### Generated code — immediate.gen.go simplifies

Before:
```go
func NewFileReceiver(p *provider.Provider) *op.ReflectedReceiver {
    return op.WrapReceiver("file", p, Params)
}
```

After:
```go
func NewFileReceiver(p *provider.Provider) *op.ReflectedReceiver {
    return op.WrapReceiver("file", p)
}
```

### Flow Provider

The flow provider (`internal/execution/flow/provider.go`) registers actions
directly via `reg.Register(&Choose{})` — it does not call
`RegisterReflectedActions` and has no `Params`. No change needed.

## Related Documents

- [Provider Registration](provider-registration.md) — Parent epic
- [Provider Registration follow-up](provider-registration/follow-up.md) — Origin of this item
