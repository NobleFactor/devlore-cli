---
title: "Phase 1: Framework Kernel"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 1: Framework Kernel

## Summary

Add the `Provider` interface, `Announce()`, `InitAll()`, and `Providers()`
to `pkg/op` alongside the existing `RegisterBinding`/`AllBindings` system.
No callers change yet — the old system continues to work.

## Deliverables

### 1. Provider interface (`pkg/op/provider.go`)

```go
// Provider is the required interface for all providers — generated and handwritten alike.
type Provider interface {
    Name() string
    Register(reg *ActionRegistry, ctx Context)
}

// PlannedProvider is optional. Checked via type assertion during InitAll.
type PlannedProvider interface {
    NewPlanned(graph *Graph, project string, reg *ActionRegistry) starlark.Value
}

// ImmediateProvider is optional. Checked via type assertion during InitAll.
type ImmediateProvider interface {
    NewImmediate(cfg BindingConfig) starlark.Value
}
```

### 2. Announce / InitAll / Providers (`pkg/op/announce.go`)

```go
var (
    announceMu sync.Mutex
    announced  []Provider
)

// Announce records a provider descriptor. Called in init().
// Does zero initialization — stores the value for later InitAll callback.
func Announce(p Provider) {
    announceMu.Lock()
    defer announceMu.Unlock()
    announced = append(announced, p)
}

// InitAll calls Register on every announced provider, then type-asserts for
// PlannedProvider and ImmediateProvider and records those callbacks.
// Called once by the framework when it is ready.
func InitAll(reg *ActionRegistry, ctx Context) {
    announceMu.Lock()
    providers := make([]Provider, len(announced))
    copy(providers, announced)
    announceMu.Unlock()

    for _, p := range providers {
        p.Register(reg, ctx)
    }
}

// Providers returns all announced providers (for introspection/debugging).
func Providers() []Provider {
    announceMu.Lock()
    defer announceMu.Unlock()
    out := make([]Provider, len(announced))
    copy(out, announced)
    return out
}
```

### 3. PlannedProvider / ImmediateProvider collection

`InitAll` also builds the callback maps that `BindingSet.BuildGlobals` and
`PlanRoot` will use in Phase 4. For now these maps are stored internally and
not exposed — Phase 4 wires them into `BindingSet`.

## Tasks

- [ ] Create `pkg/op/provider.go` — `Provider`, `PlannedProvider`, `ImmediateProvider`
- [ ] Create `pkg/op/announce.go` — `Announce`, `InitAll`, `Providers`
- [ ] Add unit tests in `pkg/op/announce_test.go`
  - Announce multiple providers, verify `Providers()` returns them
  - Call `InitAll`, verify `Register` was called on each
  - Type-assert `PlannedProvider`/`ImmediateProvider`, verify callbacks collected
  - Verify `Announce` is safe to call concurrently from multiple init()
- [ ] Verify `make check` passes — no existing code is changed

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider.go` | Create | Interface definitions |
| `pkg/op/announce.go` | Create | Announce/InitAll/Providers |
| `pkg/op/announce_test.go` | Create | Unit tests |

## Exit Criteria

- New interfaces and functions exist in `pkg/op`
- All tests pass
- No existing code is modified — old and new systems coexist
