---
title: "Reduce pkg/op Public Surface Area"
issue: TBD
status: in-progress
created: 2026-03-15
updated: 2026-03-16
---

# Plan: Reduce pkg/op Public Surface Area

## Summary

`pkg/op` is a flat package with 56 files and ~125 exported symbols. The execution engine,
CLI, and Starlark runtime need roughly 80 of those. 19 symbols are only used by providers
(`pkg/op/provider/*`). ~26 symbols are dead code — never referenced outside `pkg/op` itself.
This plan moves provider-only symbols behind `pkg/op/internal/`, deletes dead exports, and
establishes `pkg/iox` as a standalone utility package.

## Goals

1. **Shrink the public contract**: Only symbols the engine/CLI/runtime need remain in `pkg/op`.
2. **Isolate provider infrastructure**: Provider-only types move to `pkg/op/internal/`,
   visible to providers but invisible to consumers outside `pkg/op/`.
3. **Delete dead exports**: ~26 symbols that are never referenced externally get unexported
   or deleted. This is greenfield — no legacy users.
4. **Establish `pkg/iox`**: Standalone I/O utilities (starting with `Close`) that are
   independent of the op framework.

## Current State

| Category | Count | Location |
| --- | --- | --- |
| True public API (engine + CLI + runtime) | ~80 | `pkg/op/*.go` |
| Provider-only API | 19 | `pkg/op/*.go` (misplaced) |
| Dead code (unreferenced externally) | ~26 | `pkg/op/*.go` |
| Standalone utilities | 0 | (does not exist yet) |

## Requirements

### Provider-Only Symbols (19)

These are referenced exclusively by `pkg/op/provider/*` packages. They should move to
`pkg/op/internal/` (importable by anything under `pkg/op/` but invisible outside):

- `AnnounceResource`
- `CallableInput`
- `CallableResource`
- `Construct`
- `Marshal`
- `MethodParams`
- `NewTombstoneBase`
- `PackageManager`
- `Path`
- `ProviderBase`
- `RegisterConstructor`
- `RegisterReceiverParams`
- `Resource`
- `ResourceBase`
- `SearchResult`
- `Tombstone`
- `TombstoneBase`
- `WrapProviderInExecutingReceiver`
- `WrapProviderInPlanningReceiver`

### Dead Exports (~26)

Never referenced outside `pkg/op` itself. Unexport or delete:

- `AccessType`, `AccessImmediate`, `AccessPlanned`, `AccessBoth`
- `ProviderLifetime`, `LifetimeStateless`, `LifetimePhase`, `LifetimeSession`
- `AnyToStarlarkValue`, `GoToStarlarkValue`, `StarlarkValueToGo`
- `StarlarkDictToMap`, `StarlarkListToSlice`, `StringSliceToList`
- `UnmarshalToAny`, `ResolveInput`
- `ErrDrifted`, `ErrReadOnly`
- `FallibleAction`
- `RecoveryEntry`, `ResourceDescriptor`
- `BackoffStrategy`, `Encoder`, `PhaseStatus`

### New Package: `pkg/iox`

Standalone I/O utilities, starting with:

```go
package iox

import (
    "errors"
    "io"
)

// Close closes all provided closers, joining any errors into *err.
// Nil closers are safely skipped. Use with named returns:
//
//	defer iox.Close(&err, f, enc)
func Close(err *error, closers ...io.Closer) {
    for _, c := range closers {
        if c != nil {
            *err = errors.Join(*err, c.Close())
        }
    }
}
```

### Target Structure

```
pkg/
  iox/                          ← standalone I/O utilities
    close.go
  op/
    *.go                        ← ~80 symbols: engine contract
    internal/
      provider/                 ← 19 symbols: provider toolkit
        resource.go             ← Resource, ResourceBase, SearchResult, etc.
        base.go                 ← ProviderBase, Construct, Marshal
        tombstone.go            ← Tombstone, TombstoneBase, NewTombstoneBase
        callable.go             ← CallableInput, CallableResource
        receiver.go             ← WrapProviderIn*Receiver, RegisterReceiverParams
        registration.go         ← RegisterConstructor, MethodParams
        announce.go             ← AnnounceResource
        path.go                 ← Path (if fully provider-scoped)
        packagemanager.go       ← PackageManager
    provider/
      file/                     ← imports op + op/internal/provider
      archive/
      ...
```

## Implementation Phases

### Phase 1: Create `pkg/iox` — `complete`

- [x] Create `pkg/iox/close.go` with `Close` function
- [x] Add tests in `pkg/iox/close_test.go`
- [x] Adopt `iox.Close` at all 37 Close call sites identified in the inspection cleanup

### Phase 2: Delete Dead Exports

- [ ] Remove or unexport the ~26 dead symbols
- [ ] Verify `make check` passes
- [ ] Cross-reference with GoLand inspection cleanup (Phase 5 dead code overlaps)

### Phase 3: Create `pkg/op/internal/provider`

- [ ] Create the package structure
- [ ] Move the 19 provider-only symbols
- [ ] Update all `pkg/op/provider/*` imports
- [ ] Verify no code outside `pkg/op/` references the moved symbols

### Phase 4: Verify and Clean Up

- [ ] `make check` passes
- [ ] Verify `internal/execution`, `internal/cli`, `cmd/` cannot import `pkg/op/internal/`
- [ ] Re-export GoLand inspections and confirm reduced surface area

## Related Documents

- [goland-inspection-cleanup.md](./goland-inspection-cleanup.md) — Inspection cleanup plan
  (Phase 4 Close handling depends on `pkg/iox`, Phase 5 dead code overlaps with Phase 2 here)

## Open Questions

- [ ] Does `Path` belong in `pkg/op/internal/provider` or does the engine need it directly?
  (Engine uses `Root.NewPath` which returns `Path` — may need to stay in `op`)
- [ ] Should `pkg/op/internal/provider` be a single package or further decomposed?
- [ ] Some "dead" symbols may be transitively used (e.g., `Encoder` is a parameter type of
  `Graph.Serialize`) — verify before deleting
- [ ] `ErrReadOnly` is used in `root.go` — if Root stays in `op`, should ErrReadOnly stay too?