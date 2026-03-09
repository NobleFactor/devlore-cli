---
title: "Lazy Resource Registration"
issue: TBD
status: draft
created: 2026-03-08
updated: 2026-03-08
---

# Plan: Lazy Resource Registration

## Summary

Move all resource registration bookkeeping out of hand-written provider code and into the codegen
pipeline. Resources adopt the same two-phase lifecycle providers already use: `init()` announces
the type, a generated `Init()` completes registration lazily on first use. The callable extractor
is generalized into the marshaler so `mem` no longer needs a special registration path.

## Goals

1. **No bookkeeping in provider directories unless generated and untracked.** Developers write
   domain logic only. Codegen handles announcement, constructor registration, and marshaling hooks.
2. **Marshaling logic lives in the marshaler.** All value coercion — `string → Resource`,
   `*starlark.Function → func(...)` — is owned by `pkg/op`, not registered piecemeal from provider
   packages.
3. **Lazy registration.** Resources announce themselves at import time (lightweight). Full
   registration (constructor, marshaling hooks) happens on first use via a generated `Init()`.
4. **Generalize the callable extractor.** Fold `*starlark.Function → CallableResource` coercion
   into the marshaler's constructor registry so it is no longer a one-off mechanism.

## Design Constraints

### DC1: Thread-safe lazy initialization

Resource descriptors must use `sync.Once` to guarantee exactly-once initialization. Two goroutines
hitting the same resource type concurrently must not double-init or race. The current
`callableExtractorFn` (bare function pointer, no synchronization) is a known deficiency — Phase 3
must replace it with the `sync.Once`-protected descriptor pattern.

### DC2: Lazy init error semantics

When `Init()` fails, the error is cached and returned on all subsequent attempts. No retry. This
avoids repeated expensive failures and makes behavior deterministic. The resource descriptor tracks
three states: uninitialized, initialized, failed.

### DC3: First use is plan time

"Lazy on first use" means the first plan that references a resource type triggers its `Init()`. This
happens during `coerceSlotValue` in the planned bridge — before execution begins. Resources that no
plan references pay zero initialization cost. Resources that a plan does reference are fully
initialized before execution starts.

### DC4: Resource gen lives in the existing gen package

Generated resource descriptors go into the existing `*/gen` package alongside `provider.gen.go`
(e.g., `pkg/op/provider/file/gen/resource.gen.go`). Since `register.go` already blank-imports each
`*/gen` package, no changes to `register.go` are needed — the resource descriptor's `init()` fires
automatically.

### DC5: Callable extraction is plan-time only

Callable extraction reads source code from the planning machine, compiles it, and stores the
compiled form in slots. Execution happens later, potentially on different machines. Since `*os.Root`
is an execution-time value (scoped to the target machine), it is irrelevant to extraction. The
`*os.Root` parameter added to the extraction chain in the os.Root work (Phase 5) must be reverted
as part of Phase 3 — extraction signatures should not accept `*os.Root`.

## Current State

| Component               | Status          | Notes                                                                   |
| ----------------------- | --------------- | ----------------------------------------------------------------------- |
| Provider two-phase init | ✅ Working      | `init()` announces, `InitAll()` registers lazily                        |
| Resource constructors   | ❌ Eager        | Hand-written `init()` in each `resource.go` calls `RegisterConstructor` |
| Callable extractor      | ❌ Special-case | `mem` registers via `RegisterCallableExtractor` in `init()`             |
| Codegen for providers   | ✅ Working      | `star` reads `provider.go`, emits `gen/*.gen.go`                        |
| Codegen for resources   | ❌ Missing      | No codegen reads `resource.go`                                          |

### Current init() registrations (hand-written)

| Package      | init() in resource.go                                         | What it registers                                                 |
| ------------ | ------------------------------------------------------------- | ----------------------------------------------------------------- |
| `file`       | `RegisterConstructor[Resource]`                               | `string → file.Resource`                                          |
| `git`        | `RegisterConstructor[Resource]`                               | `string → git.Resource`                                           |
| `pkg`        | `RegisterConstructor[Resource]`                               | `string → pkg.Resource`                                           |
| `service`    | `RegisterConstructor[Resource]`                               | `string → service.Resource`                                       |
| `appnet`     | `RegisterConstructor[Resource]`                               | `string → appnet.Resource`                                        |
| `mem`        | `RegisterConstructor[Resource]` + `RegisterCallableExtractor` | `string → mem.Resource` + `*starlark.Function → CallableResource` |
| `archive`    | (none)                                                        | Tombstone only                                                    |
| `encryption` | (none)                                                        | Tombstone only                                                    |

### Current announcement flow

```text
register.go (blank imports gen packages)
  → gen/provider.gen.go init() → op.Announce(&xProvider{})
  → resource.go init()         → op.RegisterConstructor(...)  ← THIS IS THE PROBLEM
  → mem/resource.go init()     → op.RegisterCallableExtractor(...)  ← ALSO THIS
```

## Requirements

### R1: Generated resource announcement

Codegen reads `resource.go` and emits a resource descriptor in `gen/` that:

- Calls `op.AnnounceResource()` in `init()` (lightweight — type name only)
- Provides a generated `Init()` that calls `op.RegisterConstructor` with the constructor logic

The hand-written `init()` function in `resource.go` is deleted. The constructor function itself
remains hand-written in `resource.go` as an exported function (e.g., `ResourceFromValue`) that
codegen references.

### R2: Lazy resource initialization

`pkg/op` gains a resource announcement registry (parallel to the provider one):

- `AnnounceResource(desc ResourceDescriptor)` — called in generated `init()`
- `InitResource[T]()` — called lazily on first use (e.g., when `coerceSlotValue` encounters type T)
- `ResourceDescriptor` interface: `Name() string`, `Init()` — similar to provider `Provider`

The marshaler (`coerceSlotValue`) checks whether the target type's resource has been initialized
before attempting coercion. If not, it calls `Init()` first.

### R3: Generalize callable extraction into the marshaler

The callable extractor is a special case of value coercion: `*starlark.Function → func(...)`. This
should be handled by the same marshaler path that handles `string → Resource`.

Steps:

1. Move `buildCallableFunc`, `initCallableSlots`, and the callable adapter logic from `callable.go`
   into the marshaler
2. The `mem` package registers its `Extract` + `Compile` pipeline as a constructor for
   `CallableResource`, using the same `AnnounceResource` / lazy `Init()` pattern
3. Remove `RegisterCallableExtractor` and `callableExtractorFn` — the marshaler handles this
   natively

### R4: Constructor stays hand-written, registration is generated

The constructor function (the logic that converts `any → Resource`) remains in hand-written
`resource.go`. Codegen only generates the bookkeeping wrapper.

Codegen detects the constructor by matching the signature `func(any) (Resource, error)` — the
name is chosen by the developer. The recommended name is `ResourceFromValue`:

```go
// Hand-written in resource.go (developer writes this)
func ResourceFromValue(v any) (Resource, error) {
    s, ok := v.(string)
    if !ok {
        return Resource{}, fmt.Errorf("file.Resource: expected string, got %T", v)
    }
    return NewResource(s), nil
}

// Generated in gen/resource.gen.go (codegen writes this)
type fileResource struct{}

func (d *fileResource) Name() string { return "file.Resource" }

func (d *fileResource) Init() {
    op.RegisterConstructor(provider.ResourceFromValue)
}

func init() {
    op.AnnounceResource(&fileResource{})
}
```

Packages without a constructor (e.g., `archive`, `encryption` — tombstone-only today) are skipped
by codegen. When resources are added later, the developer writes a `Resource` struct + constructor
and codegen picks it up automatically.

### R5: Codegen reads resource.go

The `star` codegen tool is extended to:

1. Detect `Resource` structs in `resource.go` (by embedding `op.ResourceBase`)
2. Find the constructor function by signature: `func(any) (Resource, error)`
3. Error if multiple functions match (one constructor per resource type)
4. Emit `gen/resource.gen.go` with announcement and lazy init

## Implementation Phases

### Phase 1: Resource announcement infrastructure

Add the resource announcement mechanism to `pkg/op`, parallel to the provider one.

- [ ] Define `ResourceDescriptor` interface in `pkg/op`: `Name() string`, `Init() error`
- [ ] Implement `AnnounceResource()` and resource announcement registry
- [ ] Implement lazy `InitResource()` called from the marshaler on first use
- [ ] Each descriptor wraps `Init()` in `sync.Once` — exactly-once guarantee (DC1)
- [ ] Failed `Init()` caches the error and returns it on subsequent calls (DC2)
- [ ] Add tests for the announcement and lazy init lifecycle
- [ ] Add tests for concurrent first-use (two goroutines, same type)

**Files**:

- `pkg/op/announce.go` — Modify: add resource announcement alongside provider announcement
- `pkg/op/starvalue_marshal.go` — Modify: check and trigger lazy init in `coerceSlotValue`

### Phase 2: Constructor convention and generated resource descriptors

Establish the hand-written constructor convention and generate the resource descriptors.

- [ ] Refactor existing `init()` functions in each `resource.go` to export the constructor as
      `ResourceFromValue` (or any name — codegen matches by signature) and remove the `init()`
- [ ] Extend `star` codegen to read `resource.go` and emit `gen/resource.gen.go`
- [ ] Update `register.go` generation to import resource gen packages
- [ ] Verify all existing tests pass

**Files**:

- `pkg/op/provider/file/resource.go` — Modify: export constructor, delete `init()`
- `pkg/op/provider/git/resource.go` — Modify: same
- `pkg/op/provider/pkg/resource.go` — Modify: same
- `pkg/op/provider/service/resource.go` — Modify: same
- `pkg/op/provider/appnet/resource.go` — Modify: same
- `pkg/op/provider/mem/resource.go` — Modify: same (constructor only; callable extractor in Phase 3)
- `pkg/op/provider/*/gen/resource.gen.go` — Create: generated resource descriptors
- `noblefactor-ops/cmd/star` — Modify: extend codegen to read resource.go

### Phase 3: Generalize callable extraction

Fold the callable extractor into the marshaler so `mem` no longer needs a special registration.

- [ ] Remove `*os.Root` from the extraction chain: `Extract`, `ExtractWithName`, `synthesize`,
      `extractLambdaBody`, `extractDefSource`, `readSource` (DC5 — revert Phase 5 threading)
- [ ] Move `buildCallableFunc` and callable adapter logic into the marshaler as native coercion
- [ ] Register `mem.Extract` + `mem.Compile` as a resource constructor via `AnnounceResource`
- [ ] Remove `RegisterCallableExtractor`, `callableExtractorFn`, and `ExtractCallable` —
      replaced by `sync.Once`-protected descriptor (DC1)
- [ ] Update `planned_reflect.go` and `action_reflect.go` to use the marshaler for callable
      coercion instead of calling `ExtractCallable` directly
- [ ] Verify callable tests pass

**Files**:

- `pkg/op/callable.go` — Delete: merge `CallableResource` interface and adapter logic into
  `starvalue_marshal.go`
- `pkg/op/starvalue_marshal.go` — Modify: absorb callable adapter logic; handle
  `*starlark.Function → func(...)` natively
- `pkg/op/planned_reflect.go` — Modify: remove direct `ExtractCallable` call
- `pkg/op/action_reflect.go` — Modify: remove callable special-case
- `pkg/op/provider/mem/resource.go` — Modify: remove `RegisterCallableExtractor` from init
- `pkg/op/provider/mem/extract.go` — Modify: remove `*os.Root` from extraction signatures
- `pkg/op/provider/mem/extract_test.go` — Modify: update tests for removed `*os.Root` parameter

### Phase 4: Cleanup

- [ ] Verify no hand-written `init()` remains in any `resource.go`
- [ ] Run `make check` — full quality gate
- [ ] Grep for `RegisterConstructor`, `RegisterCallableExtractor`, `ExtractCallable`,
      `callableExtractorFn` — confirm no direct calls remain outside generated code

## Migration Path

No external migration needed — this is an internal refactoring. The generated API surface
(`gen/*.gen.go`) changes but those files are untracked and regenerated by `make generate`.

Provider developers see one change: the constructor function in `resource.go` must be exported
(e.g., `ResourceFromValue`) instead of being an anonymous function inside `init()`. Codegen
detects it by signature, not by name.

## Files to Create/Modify

| File                                    | Action | Purpose                                                         |
| --------------------------------------- | ------ | --------------------------------------------------------------- |
| `pkg/op/announce.go`                    | Modify | Add `AnnounceResource`, `ResourceDescriptor`, resource registry |
| `pkg/op/starvalue_marshal.go`           | Modify | Lazy init check in `coerceSlotValue`; absorb callable logic     |
| `pkg/op/callable.go`                    | Delete | Merge into `starvalue_marshal.go`                               |
| `pkg/op/planned_reflect.go`             | Modify | Remove `ExtractCallable` call                                   |
| `pkg/op/action_reflect.go`              | Modify | Remove callable special-case                                    |
| `pkg/op/provider/*/resource.go`         | Modify | Export constructor as `ResourceFromValue`, delete `init()`      |
| `pkg/op/provider/*/gen/resource.gen.go` | Create | Generated resource descriptors                                  |
| `pkg/op/provider/mem/extract.go`        | Modify | Remove `*os.Root` from extraction signatures                    |
| `pkg/op/provider/mem/extract_test.go`   | Modify | Update tests for removed `*os.Root` parameter                   |
| `noblefactor-ops/cmd/star`              | Modify | Extend codegen to read `resource.go`                            |

## Related Documents

- [Go Style Guidelines](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/guides/go-style-guidelines.md)
- `pkg/op/announce.go` — existing provider announcement mechanism
- `pkg/op/callable.go` — current callable extractor (to be generalized)

## Resolved Decisions

- **Constructor naming**: Codegen matches by signature `func(any) (Resource, error)`, not by name.
  Recommended name: `ResourceFromValue`. Error if multiple functions match in the same file.
- **Tombstone-only packages** (`archive`, `encryption`): No constructor signature match, so
  codegen emits nothing. When resources are added later, codegen finds the constructor and
  generates the descriptor automatically.
- **Callable adapter location**: Merge `callable.go` into `starvalue_marshal.go`. At ~860 lines
  combined, this is manageable. The `CallableResource` interface, `buildCallableFunc`,
  `initCallableSlots`, and related helpers all move into the marshaler.
- **Thread safety**: Per-descriptor `sync.Once` for exactly-once initialization. Replaces the
  unprotected `callableExtractorFn` function pointer.
- **Error caching**: Failed `Init()` caches the error. No retry. Deterministic behavior.
- **Gen package placement**: Resource descriptors go in the existing `*/gen` package alongside
  `provider.gen.go`. No changes to `register.go` needed.
- **`*os.Root` in extraction**: Reverted. Extraction is plan-time only; root is execution-time
  only (target machine). The `*os.Root` parameter threaded through the extraction chain in the
  os.Root work (Phase 5) was premature — extraction always reads source from the planning machine
  via `os.ReadFile`. Phase 3 removes `*os.Root` from the extraction signatures.

## Open Decisions

(None — all design questions resolved.)
