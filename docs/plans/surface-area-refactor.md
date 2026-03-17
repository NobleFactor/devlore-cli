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
CLI, and Starlark runtime need roughly 80 of those. Provider-only symbols belong in
`pkg/op/provider` — the provider toolkit package that external consumers (noblefactor-ops)
import when writing their own providers. Dead exports get unexported.

## Goals

1. **Shrink the public contract**: Only symbols the engine/CLI/runtime need remain in `pkg/op`.
2. **Establish the provider toolkit**: `pkg/op/provider` becomes the public package for
   provider authors. Types like `provider.Lifetime`, `provider.AccessType`, and eventually
   `provider.Base`, `provider.Resource`, etc. live here. External consumers import it directly.
3. **Clean consumer API**: A consumer imports `pkg/op` for the runtime, specific
   `pkg/op/provider/*/gen` packages for receivers (e.g., `json.Receiver`, `yaml.Receiver`),
   and optionally `pkg/op/provider` for toolkit types.
4. **Establish `pkg/iox`**: Standalone I/O utilities independent of the op framework.

## Consumer API

External consumers (e.g., noblefactor-ops) interact with the module like this:

```go
import (
    "github.com/NobleFactor/devlore-cli/pkg/op"
    "github.com/NobleFactor/devlore-cli/pkg/op/provider/json/gen"
    "github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml/gen"
    "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
)

cfg := op.NewBindingConfig("star").
    WithReceivers(json.Receiver, yaml.Receiver, ui.Receiver).
    WithColor()
star := op.NewStarlarkRuntime(cfg)
```

Each `gen/` package declares the parent's package name (e.g., `package json`), so the
consumer writes `json.Receiver`, `yaml.Receiver` — never `jsongen` aliases.

Provider authors who write their own providers additionally import the toolkit:

```go
import "github.com/NobleFactor/devlore-cli/pkg/op/provider"

// provider.Lifetime, provider.AccessType, etc.
```

### Registration

`pkg/op/provider/register.go` blank-imports all built-in provider `gen/` packages,
triggering `init()` → `op.Announce()`. All providers must announce themselves to be
available to consumers. The registration file stays in `pkg/op/provider/` — no import
cycle exists because provider subdirectories (`pkg/op/provider/file/`, etc.) import
`pkg/op`, not `pkg/op/provider`.

## Current State

| Category | Count | Location |
| --- | --- | --- |
| True public API (engine + CLI + runtime) | ~80 | `pkg/op/*.go` |
| Provider toolkit types | 2 files (currently dead in `pkg/op`) | `pkg/op/access.go`, `pkg/op/lifetime.go` |
| Provider-only API (future toolkit candidates) | 14 | `pkg/op/*.go` (misplaced) |
| Dead code (unreferenced externally) | 3 symbols | `pkg/op/recovery.go`, `pkg/op/root.go` |
| Registration hub | 1 generated file | `pkg/op/provider/register.go` (stays) |
| Standalone utilities | complete | `pkg/iox/` |

## Requirements

### Provider Toolkit Types — Move to `pkg/op/provider`

These types define the provider contract. They belong in `pkg/op/provider` where external
consumers import them:

- `AccessType`, `AccessImmediate`, `AccessPlanned`, `AccessBoth` → `provider.AccessType`,
  `provider.Immediate`, `provider.Planned`, `provider.Both`
- `ProviderLifetime`, `LifetimeStateless`, `LifetimePhase`, `LifetimeSession` →
  `provider.Lifetime`, `provider.Stateless`, `provider.Phase`, `provider.Session`

Rename rationale: the package name `provider` provides context, so prefixes are dropped
(no stutter). `provider.Lifetime` instead of `provider.ProviderLifetime`.

No import cycle: `register.go` blank-imports `provider/*/gen` packages; gen packages
import their specific parent (`provider/file`, `provider/json`, etc.), not
`pkg/op/provider` itself.

### Provider-Only Symbols (14, future phases)

These are referenced exclusively by `pkg/op/provider/*` packages and noblefactor-ops
provider code. They are candidates for eventual migration to `pkg/op/provider`:

- `AnnounceResource`
- `CallableInput`
- `CallableResource`
- `Construct`
- `Marshal`
- `MethodParams`
- `NewTombstoneBase`
- `ProviderBase`
- `RegisterConstructor`
- `RegisterReceiverParams`
- `Tombstone`
- `TombstoneBase`
- `WrapProviderInExecutingReceiver`
- `WrapProviderInPlanningReceiver`

**Removed from this list (core API, must stay in `pkg/op`):**

- `PackageManager` — part of `Platform` interface; used by `internal/model/*`, `internal/lorepackage/*`
- `Path` — part of `Root` interface; used by `internal/writ/`, `internal/e2e/`
- `Resource` — sealed interface; used by `internal/execution/preflight.go`
- `ResourceBase` — sealed interface embedding requirement
- `SearchResult` — return type of `PackageManager.Search()`; used by `internal/lorepackage/`

### Dead Exports (3 symbols)

Never referenced outside `pkg/op` itself. Unexport.

PR #235 already removed 16 symbols. The original list of ~26 was further reduced by
rigorous per-symbol grep audit (2026-03-16):

**Confirmed dead — unexport:**

- `RecoveryEntry` — `recovery.go`
- `ErrDrifted` — `recovery.go`
- `ErrReadOnly` — `root.go` (unexport to `errReadOnly`; used by public `RootReader` but
  callers never reference the sentinel by name)

**Reclassified as live (removed from list):**

- `FallibleAction` — part of action type hierarchy (parallel to `CompensableAction`)
- `PhaseStatus` + constants — used by `internal/execution`, `internal/lore`, `internal/starlark`
- `BackoffStrategy` + constants — transitively exposed via `Phase.RetryPolicy.Backoff`
- `ResourceDescriptor` — generated `resource.gen.go` files implement this interface
- `Encoder` — `Graph.Serialize(enc Encoder)` is called from 4 packages outside `pkg/op`

**Reclassified as provider toolkit (moved to Phase 3):**

- `AccessType`, `AccessImmediate`, `AccessPlanned`, `AccessBoth`
- `ProviderLifetime`, `LifetimeStateless`, `LifetimePhase`, `LifetimeSession`

### Target Structure

```
pkg/
  iox/                             ← standalone I/O utilities (complete)
    close.go
  op/
    *.go                           ← ~80 symbols: engine contract
    flow/                          ← flow control actions (complete)
    provider/
      register.go                  ← generated: blank-imports all built-in provider gen/ packages
      access.go                    ← provider.AccessType, provider.Immediate, ...
      lifetime.go                  ← provider.Lifetime, provider.Stateless, ...
      file/
        provider.go                ← file.Provider (hand-written)
        gen/
          receiver.gen.go          ← file.Receiver (generated, package file)
          params.gen.go
          ...
      json/
      yaml/
      ...
```

## Implementation Phases

### Phase 1: Create `pkg/iox` — `complete`

- [x] Create `pkg/iox/close.go` with `Close` function
- [x] Add tests in `pkg/iox/close_test.go`
- [x] Adopt `iox.Close` at call sites identified in the inspection cleanup

### Phase 2: Relocate `internal/execution/flow` → `pkg/op/flow` — `complete`

- [x] `git mv internal/execution/flow pkg/op/flow`
- [x] Update imports in `pkg/op/provider/register.go`, `internal/execution/flow_test.go`,
      `internal/execution/compensation_test.go`
- [x] Verify `make check` passes

### Phase 3: Establish `pkg/op/provider` as provider toolkit

Add provider contract types alongside the existing `register.go`.

- [ ] Create `pkg/op/provider/access.go`:
  - `provider.AccessType` (was `op.AccessType`)
  - `provider.Immediate` (was `op.AccessImmediate`)
  - `provider.Planned` (was `op.AccessPlanned`)
  - `provider.Both` (was `op.AccessBoth`)
- [ ] Create `pkg/op/provider/lifetime.go`:
  - `provider.Lifetime` (was `op.ProviderLifetime`)
  - `provider.Stateless` (was `op.LifetimeStateless`)
  - `provider.Phase` (was `op.LifetimePhase`)
  - `provider.Session` (was `op.LifetimeSession`)
- [ ] Delete `pkg/op/access.go` and `pkg/op/lifetime.go`
- [ ] Verify `make check` passes

### Phase 4: Unexport dead exports (3 symbols)

- [ ] Unexport `RecoveryEntry` → `recoveryEntry` in `recovery.go`
- [ ] Unexport `ErrDrifted` → `errDrifted` in `recovery.go`
- [ ] Unexport `ErrReadOnly` → `errReadOnly` in `root.go`; update `root_test.go` and
      `triad_test.go` (`package op_test`) via `export_test.go`
- [ ] Verify `make check` passes

### Phase 5: Migrate provider-only symbols to `pkg/op/provider` (future)

Migrate the 14 provider-only symbols from `pkg/op` to `pkg/op/provider`. This is a
larger effort with cross-repo impact (noblefactor-ops generated code templates must
update). Scoped separately.

- [ ] Design symbol grouping within `pkg/op/provider`
- [ ] Update code generator templates in noblefactor-ops
- [ ] Migrate symbols
- [ ] Regenerate all provider `gen/` files
- [ ] Verify both repos build and test clean

### Phase 6: Verify and clean up

- [ ] `make check` passes
- [ ] Verify consumer API works: `pkg/op` for runtime, `pkg/op/provider/*/gen` for
      receivers, `pkg/op/provider` for toolkit types
- [ ] Re-run GoLand inspections and confirm reduced surface area

## Related Documents

- [goland-inspection-cleanup.md](./goland-inspection-cleanup.md) — Inspection cleanup plan
- [shared-provider-receivers.md](./shared-provider-receivers.md) — Provider receiver framework
- [reflection-marshaler.md](./reflection-marshaler.md) — Marshaler infrastructure

## Open Questions

- [ ] Should `Marshal` stay in `pkg/op` (used by core `output.go`) or move to
  `pkg/op/provider` with a re-export?
- [ ] Exact file layout for the 14 provider-only symbols within `pkg/op/provider`
  (single file per concept or grouped?)
- [x] ~~Does `Path` belong in provider toolkit?~~ — No. Core API, part of `Root` interface.
- [x] ~~5 symbols classified as provider-only~~ — Reclassified as core API:
  `PackageManager`, `Path`, `Resource`, `ResourceBase`, `SearchResult`.
- [x] ~~Some "dead" symbols may be transitively used~~ — confirmed: `Encoder`, `PhaseStatus`,
  `BackoffStrategy`, `ResourceDescriptor`, `FallibleAction` are all live.
- [x] ~~`ErrReadOnly` is used in `root.go`~~ — used internally only; unexport to `errReadOnly`.
- [x] ~~`AccessType` and `ProviderLifetime` dead?~~ — No. They are provider contract types
  that belong in the toolkit, not dead code.
- [x] ~~Import cycle with register.go?~~ — No. Provider subdirectories import `pkg/op`,
  not `pkg/op/provider`. register.go stays in place.
