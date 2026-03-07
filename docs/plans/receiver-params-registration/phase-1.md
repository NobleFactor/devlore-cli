---
title: "Phase 1: Store Params + Simplify WrapReceiver"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../receiver-params-registration.md
---

# Phase 1: Store Params + Simplify WrapReceiver

## Summary

Make `RegisterReflectedActions` store receiver params in the registry as a
side effect. Change `WrapReceiver` to `(name, provider)` — it looks up
params from the registry. Update the `immediate.gen.go` template and
regenerate. Delete the unused `RegisterReceiverParams[T]` generic function.

`InitAll` always runs before `NewImmediate`, so the registry is always
populated before `WrapReceiver` needs it. No shim required.

## Deliverables

### 1. Non-generic registration helper

Add an internal function that stores params using the runtime `reflect.Type`
of the provider pointer:

```go
// pkg/op/starvalue_marshal.go

func registerReceiverParamsReflect(name string, provider any, params MethodParams) {
    t := reflect.TypeOf(provider)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }
    receiverParamsRegistry.Store(t, receiverEntry{name: name, params: params})
}
```

Add the lookup helper:

```go
// pkg/op/starvalue_marshal.go

func lookupReceiverParams(t reflect.Type) (receiverEntry, bool) {
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }
    v, ok := receiverParamsRegistry.Load(t)
    if !ok {
        return receiverEntry{}, false
    }
    return v.(receiverEntry), true
}
```

### 2. RegisterReflectedActions side effect

Add one line at the top of `RegisterReflectedActions`:

```go
// pkg/op/action_reflect.go

func RegisterReflectedActions(reg *ActionRegistry, name string, provider any, params MethodParams) {
    registerReceiverParamsReflect(name, provider, params)
    // ... existing logic unchanged
}
```

### 3. WrapReceiver signature change

Drop the `params` argument. Look up from registry unconditionally:

```go
// pkg/op/receiver_reflect.go

func WrapReceiver(name string, provider any) *ReflectedReceiver {
    entry, ok := lookupReceiverParams(reflect.TypeOf(provider))
    if !ok {
        panic(fmt.Sprintf("WrapReceiver(%s): no params registered — was RegisterReflectedActions called?", name))
    }
    // ... existing bridge-building logic using entry.params
}
```

### 4. Template change

Drop `Params` from the `WrapReceiver` call:

```go
// star/extensions/com.noblefactor.devlore.Actions/templates/immediate_receiver.go.template

// Before:
func New{{.Name}}Receiver(p *provider.Provider) *op.ReflectedReceiver {
    return op.WrapReceiver("{{.ProviderName}}", p, Params)
}

// After:
func New{{.Name}}Receiver(p *provider.Provider) *op.ReflectedReceiver {
    return op.WrapReceiver("{{.ProviderName}}", p)
}
```

### 5. Regenerate all providers

Run `make build`. Every `immediate.gen.go` drops the `Params` argument.

### 6. Delete `RegisterReceiverParams[T]`

The generic function in `starvalue_marshal.go` has no callers now that
`registerReceiverParamsReflect` handles registration. Delete it.

### 7. Tests

```go
// pkg/op/starvalue_marshal_test.go

// - After RegisterReflectedActions, verify the type is in the registry
// - WrapReceiver("file", &Provider{}) succeeds (looks up from registry)
// - marshalReflect(&Provider{}) returns a ReflectedReceiver with methods
// - Planned-only provider (no RegisterReflectedActions call) is absent
//   from registry
```

## Tasks

- [ ] Add `registerReceiverParamsReflect` and `lookupReceiverParams` in `pkg/op/starvalue_marshal.go`
- [ ] Call `registerReceiverParamsReflect` at top of `RegisterReflectedActions` in `pkg/op/action_reflect.go`
- [ ] Change `WrapReceiver` signature to `(name, provider)` in `pkg/op/receiver_reflect.go`
- [ ] Update `star/.../templates/immediate_receiver.go.template` — drop `Params`
- [ ] Regenerate all providers (`make build`)
- [ ] Delete `RegisterReceiverParams[T]` from `pkg/op/starvalue_marshal.go`
- [ ] Unit tests in `pkg/op/starvalue_marshal_test.go`
- [ ] `make check` passes
- [ ] `make test-race` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/starvalue_marshal.go` | Modify | Add helpers, delete `RegisterReceiverParams[T]` |
| `pkg/op/action_reflect.go` | Modify | Store params in `RegisterReflectedActions` |
| `pkg/op/receiver_reflect.go` | Modify | `WrapReceiver(name, provider)` — no params arg |
| `star/extensions/com.noblefactor.devlore.Actions/templates/immediate_receiver.go.template` | Modify | Drop `Params` from `WrapReceiver` call |
| `pkg/op/provider/*/gen/immediate.gen.go` | Regenerate | All providers with immediate mode |
| `pkg/op/starvalue_marshal_test.go` | Create | Unit tests |

## Exit Criteria

- `receiverParamsRegistry` populated after `InitAll`
- `WrapReceiver` takes `(name, provider)` only — no `params` argument
- `RegisterReceiverParams[T]` deleted
- `marshalReflect(&file.Provider{})` returns a `ReflectedReceiver`
- All generated `immediate.gen.go` use new signature
- `make check` and `make test-race` pass
