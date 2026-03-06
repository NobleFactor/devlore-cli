# Resource Lifecycle Redesign — Decision #10 Implementation

## Context

Decision #10 in `docs/plans/resource-management.md` establishes a three-phase
resource lifecycle: **Construct → Resolve → Refresh**. The rationale is remote
execution — a graph can be planned on one machine and executed on another, so
constructors must be pure computation (no `filepath.Abs`, no `os.Stat`). The
current code violates this: `file.NewResource` does `os.Stat`, and two separate
constructor registries exist (execution-time with I/O, plan-time without).

This plan implements Decision #10 using `file.Resource` as the prototype and
updates all other providers to match.

## Steps

### 1. Add `Resolve()` to the Resource interface and ResourceBase

**Files**: `pkg/op/resource.go`, `pkg/op/resource_test.go`

Add `Resolve() error` to the `Resource` interface. Add a default no-op
implementation on `*ResourceBase`:

```go
type Resource interface {
    URI() string
    Scheme() string
    Host() string
    Path() string
    Resolve() error
    resourceBase() *ResourceBase
}

func (b *ResourceBase) Resolve() error { return nil }
```

Providers that need resolution override it. Providers that don't (service, pkg,
net) inherit the no-op.

### 2. Make file.NewResource infallible — no I/O

**File**: `pkg/op/provider/file/resource.go`

Change signature from `func NewResource(path string) (Resource, error)` to
`func NewResource(path string) Resource`. The body becomes:

```go
func NewResource(path string) Resource {
    return Resource{SourcePath: path}
}
```

No `os.Stat`, no `checksumFile`, no metadata population. Pure identity capture.

### 3. Add file.Resource.Resolve()

**File**: `pkg/op/provider/file/resource.go`

```go
func (r *Resource) Resolve() error {
    abs, err := filepath.Abs(r.SourcePath)
    if err == nil {
        r.SourcePath = filepath.Clean(abs)
    }

    info, err := os.Stat(r.SourcePath)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil // known path, no data yet
        }
        return fmt.Errorf("failed to stat: %w", err)
    }

    // populate metadata (same as current NewResource body)
    var inode, device uint64
    if stat, ok := info.Sys().(*syscall.Stat_t); ok {
        inode = stat.Ino
        device = uint64(stat.Dev)
    }
    r.Inode = inode
    r.Device = device
    r.Size = info.Size()
    r.Mode = info.Mode()
    r.ModTime = info.ModTime()
    r.Checksum = checksumFile(r.SourcePath)
    return nil
}
```

### 4. Remove filepath.Abs from Path() methods

**Files**: `pkg/op/provider/file/resource.go`, `pkg/op/provider/git/resource.go`

`file.Resource.Path()` → returns `r.SourcePath` (no filepath.Abs, no Clean).
`git.Resource.Path()` → returns `r.ClonePath` (no filepath.Abs, no Clean).

Before Resolve(), paths are raw. After Resolve(), they're absolute and clean.

### 5. Add git.Resource.Resolve()

**File**: `pkg/op/provider/git/resource.go`

Canonicalizes `ClonePath` via `filepath.Abs` + `filepath.Clean`. No os.Stat.

```go
func (r *Resource) Resolve() error {
    abs, err := filepath.Abs(r.ClonePath)
    if err == nil {
        r.ClonePath = filepath.Clean(abs)
    }
    return nil
}
```

service, pkg, net inherit the ResourceBase no-op — no changes needed.

### 6. Rename RefreshMetadata → Refresh, RefreshMetadataWith → RefreshWith

**File**: `pkg/op/provider/file/resource.go`, `pkg/op/provider/file/provider.go`

Rename methods on `file.Resource`:
- `RefreshMetadata()` → `Refresh()`
- `RefreshMetadataWith(checksum, size)` → `RefreshWith(checksum, size)`

Update the single call site in `provider.go:755`:
`result.RefreshMetadataWith(...)` → `result.RefreshWith(...)`

### 7. Eliminate dual constructor registries

**File**: `pkg/op/starvalue_marshal.go`

- Delete `planTimeConstructorRegistry` variable
- Delete `registerPlanTimeConstructor` function
- Delete `RegisterResourceConstructors` function
- Rename `constructPlanTimeResource` → `constructResource`, change it to use
  `constructorRegistry` instead of `planTimeConstructorRegistry`

**File**: `pkg/op/action_reflect.go`

- Remove `planTimeConstructorRegistry.Load` check from `validateSlotType`
  (the single `constructorRegistry` check suffices)

### 8. Update all provider init() functions

Each provider switches from `RegisterResourceConstructors(exec, plan)` to
`RegisterConstructor(ctor)` with a single pure constructor.

| Provider | File | Change |
|----------|------|--------|
| file | `pkg/op/provider/file/resource.go` | `RegisterConstructor` → calls `NewResource(s)` (now infallible), wraps in `(Resource, error)` for type check |
| git | `pkg/op/provider/git/resource.go` | `RegisterConstructor` → single `ctor` |
| service | `pkg/op/provider/service/resource.go` | `RegisterConstructor` → single `ctor` |
| pkg | `pkg/op/provider/pkg/resource.go` | `RegisterConstructor` → single `ctor` |
| net | `pkg/op/provider/net/resource.go` | `RegisterConstructor` → single `ctor` |

### 9. Update planned_reflect.go

**File**: `pkg/op/planned_reflect.go`

- `resolveResourceParam`: calls `constructResource` (renamed from
  `constructPlanTimeResource`)
- `shadowOutputParam`: same rename

No logic changes — the function works identically, just uses the unified
registry.

### 10. Update file.NewResource callers — add Resolve() calls

All callers that previously relied on `NewResource` doing os.Stat now call
`Resolve()` explicitly. The error handling shifts from `NewResource` to
`Resolve()`.

**`pkg/op/provider/file/provider.go`** (4 sites):
- `Link:208` → `result = NewResource(path.SourcePath)` then `result.Resolve()`
- `WalkTree:450` → `resource := NewResource(path)` then `resource.Resolve()`
- `Read:657` → `r := NewResource(path.SourcePath)` then `r.Resolve()`
- `prepareWrite:696` → `result = NewResource(resource.SourcePath)` then
  `result.Resolve()`

**`pkg/op/provider/encryption/provider.go`** (1 site):
- Line 48 → `result := file.NewResource(destination.SourcePath)` then
  `result.Resolve()`

### 11. Update tests

**`pkg/op/provider/file/resource.go` tests** — new tests for `Resolve()`:
- Resolve on existing file populates metadata
- Resolve on non-existent file returns nil, Exists() == false
- Resolve canonicalizes relative paths to absolute
- Unresolved resource has empty metadata, Exists() == false
- Refresh and RefreshWith (renamed)

**`pkg/op/provider/file/provider_test.go`** (5 sites):
- Remove error handling from `NewResource` calls

**`pkg/op/provider/file/gen/` tests** (2 sites):
- `integration_test.go:33`, `actions_test.go:475` — drop `_` error

**`pkg/op/planned_reflect_test.go`**:
- Change `registerPlanTimeConstructor` calls to `RegisterConstructor`
- Remove deferred `planTimeConstructorRegistry.Delete` cleanup
- Use `constructorRegistry.Delete` cleanup instead

**`pkg/op/action_reflect_test.go`**:
- Verify `validateSlotType` still works with single registry

**`pkg/op/resource_test.go`**:
- Add test for `ResourceBase.Resolve()` default no-op

## Files Modified

| File | Action |
|------|--------|
| `pkg/op/resource.go` | Add Resolve() to interface + ResourceBase default |
| `pkg/op/resource_test.go` | Add Resolve() tests |
| `pkg/op/starvalue_marshal.go` | Remove plan-time registry, rename constructPlanTimeResource |
| `pkg/op/action_reflect.go` | Remove planTimeConstructorRegistry check from validateSlotType |
| `pkg/op/planned_reflect.go` | Rename constructPlanTimeResource → constructResource |
| `pkg/op/provider/file/resource.go` | Infallible NewResource, add Resolve(), rename Refresh methods, single constructor |
| `pkg/op/provider/file/provider.go` | Add Resolve() calls, update RefreshMetadataWith → RefreshWith |
| `pkg/op/provider/file/provider_test.go` | Remove NewResource error handling |
| `pkg/op/provider/file/gen/integration_test.go` | Drop error return |
| `pkg/op/provider/file/gen/actions_test.go` | Drop error return |
| `pkg/op/provider/git/resource.go` | Add Resolve(), simplify Path(), single constructor |
| `pkg/op/provider/service/resource.go` | Single constructor |
| `pkg/op/provider/pkg/resource.go` | Single constructor |
| `pkg/op/provider/net/resource.go` | Single constructor |
| `pkg/op/provider/encryption/provider.go` | Add Resolve() call |
| `pkg/op/planned_reflect_test.go` | Use RegisterConstructor instead of registerPlanTimeConstructor |
| `pkg/op/action_reflect_test.go` | Update validateSlotType tests |

## Relationship to Other Plans

- **Decision #11** (`phase-10.md`): The pkg provider's single
  constructor (Step 8) is further extended by Decision #11, which adds
  `Type`, `Version`, and `Purl()` to `pkg.Resource`. That plan depends on
  the infallible constructor and `Resolve()` pattern established here.

## Verification

1. `make build` — passes
2. `make vet` — passes
3. `make test` — passes
4. Grep for `planTimeConstructorRegistry` — zero hits
5. Grep for `RegisterResourceConstructors` — zero hits
6. Grep for `registerPlanTimeConstructor` — zero hits
7. Grep for `RefreshMetadata` — zero hits (renamed to Refresh/RefreshWith)
8. Grep for `filepath.Abs` in `*/resource.go` — only in `Resolve()` methods
9. Verify `file.NewResource` returns single value (no error)
10. Verify unresolved resource reports `Exists() == false`
