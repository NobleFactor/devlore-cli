# Phase 4: Resource Type System + Starlark Value Marshaling

## Context

Phases 1–3 built the resource management infrastructure: `ResourceManager`,
`NamespaceMap`, `file.Resource` embedding `op.Resource` (a struct), and the
constructor registry for string→Resource coercion.

Phase 4 fixed the foundational type system:

- `Resource` is now an interface (sealed by unexported `resourceBase()`),
  with `ResourceBase` as the embedded struct for identity fields.
- `ResourceCatalog` replaces both `ResourceManager` and `NamespaceMap` as
  a single compositor owning the ledger and namespace.
- The `starvalue` subpackage defines `Marshaler`/`Unmarshaler` interfaces
  for custom Starlark serialization. The marshal implementation stays in
  `pkg/op/starvalue_marshal.go` due to circular dependency constraints.

**Repo**: devlore-cli
**Branch**: `feature/resource-management-phase-4`

## Design Decisions

### D1. Resource is an interface, ResourceBase is the embedded struct

Following the `Provider`/`ProviderBase` pattern already established:

```go
// Resource is the interface for all resource types.
type Resource interface {
    URI() string
    resourceBase() *ResourceBase  // seals interface to package op
}

type ResourceBase struct {
    uri      string
    id       string
    originID string
}

func NewResourceBase(uri string) ResourceBase
func (b ResourceBase) URI() string
func (b *ResourceBase) resourceBase() *ResourceBase
```

The unexported `resourceBase()` method seals the interface: only types
embedding `ResourceBase` can implement `Resource`. The catalog stamps `id`
and `originID` directly on the base (same package, private field access).

Provider resources embed `ResourceBase` instead of the old `Resource` struct:

```go
// file.Resource
type Resource struct {
    op.ResourceBase
    SourcePath string
    Inode      uint64
    // ...
}
```

### D2. ResourceCatalog replaces ResourceManager + NamespaceMap

A single compositor owns the ledger and namespace. Graph gets one field:

```go
type Graph struct {
    // ...
    Catalog *ResourceCatalog `json:"-" yaml:"-"`
}
```

```go
type ResourceCatalog struct {
    mu      sync.Mutex
    entries []Resource         // append-only ledger (interface)
    byID    map[string]int     // id → index
    ns      map[string]string  // URI → current id
    nextID  int
}
```

Methods: `Resolve(uri) string`, `Shadow(resource, originID) string`,
`Lookup(id) (Resource, bool)`, `Len() int`, `Current(uri) string`.

### D3. starvalue package — Starlark marshaling interfaces

`pkg/op/starvalue/` defines the marshaler/unmarshaler interfaces:

```go
package starvalue

type Marshaler interface {
    MarshalStarvalue() (starlark.Value, error)
}

type Unmarshaler interface {
    UnmarshalStarvalue(starlark.Value) error
}
```

The marshal/unmarshal implementation stays in `pkg/op/starvalue_marshal.go`
(not in the `starvalue` subpackage) because `marshalReflect` calls
`WrapReceiver` and `RegisterReceiverParams` takes `MethodParams` — both
`op` types, which would create a circular dependency if the implementation
moved to `starvalue`.

`ResourceBase` implements `Marshaler` to serialize its private fields.
`marshalReflect` checks for the `Marshaler` interface before walking
fields via reflection — same pattern as `encoding/json`.

### D4. Private fields, public serialization

`ResourceBase` fields are private (enforces construction through
`NewResourceBase`, stamping through catalog). Serialization is handled
by `MarshalStarvalue()` which emits a starlark struct with `uri`, `id`,
`origin_id` keys. Deserialization is handled by `UnmarshalStarvalue()`.

### D5. Queued: ResourceURI creation semantics

Who creates resource URIs varies by type — file URIs are paths, service
URIs are names, package URIs are manager/package tuples. `ResourceURI()`
currently assumes scheme + path. This needs per-scheme logic but is out
of scope for Phase 4.

## Steps

### 4a. Resource interface + ResourceBase (DONE)

**File**: `pkg/op/resource.go`

- `Resource` interface with `URI()` + sealed `resourceBase()`
- `ResourceBase` struct with private `uri`, `id`, `originID`
- `NewResourceBase(uri)` constructor
- URI scheme constants and `ResourceURI()` (unchanged)
- Delete: old `Resource` struct, `extractResource`, `resourceType` var

### 4b. ResourceCatalog (DONE)

**File**: `pkg/op/resource_catalog.go`

- `ResourceCatalog` with internal ledger, namespace, mutex
- `NewResourceCatalog()`, `Resolve`, `Shadow`, `Lookup`, `LedgerLen`, `Current`
- `extractResource(v any) (originID string, ok bool)` — simplified

### 4c. Graph + caller updates (DONE)

- `graph.go`: single `Catalog *ResourceCatalog` field
- `output.go`: updated `extractResource` call site
- `file/resource.go`: embed `ResourceBase` instead of `Resource` struct
- Delete: `namespace.go`

### 4d. starvalue interfaces + rename marshal files (DONE)

**Directory**: `pkg/op/starvalue/`

Created `starvalue.go` with `Marshaler` and `Unmarshaler` interfaces.

The marshal/unmarshal implementation cannot move to `starvalue` due to
circular dependencies (`marshalReflect` → `WrapReceiver`, `MethodParams`).
Instead, `pkg/op/marshal.go` was renamed to `pkg/op/starvalue_marshal.go`
to clearly associate it with the `starvalue` interface package.

Files:
- `pkg/op/starvalue/starvalue.go` — interface definitions
- `pkg/op/starvalue_marshal.go` — implementation (was `marshal.go`)
- `pkg/op/starvalue_marshal_test.go` — tests (was `marshal_test.go`)

`marshalReflect` checks `starvalue.Marshaler` before reflection walk.
Struct marshaling handles embedded `Marshaler` (e.g., `ResourceBase`
inside `file.Resource`) by delegating only when the struct has no
exported fields of its own.

### 4e. ResourceBase implements starvalue.Marshaler (DONE)

**File**: `pkg/op/resource.go`

`MarshalStarvalue()` emits a starlark struct with `uri`, `id`,
`origin_id` keys. Private fields are serialized without exposing them
through the Go API.

### 4f. extractResource handles marshal round-trip (DONE)

**File**: `pkg/op/resource_catalog.go`

`extractResource` handles three forms:
- Direct `Resource` interface match
- `map[string]any` with `origin_id` key (flat form)
- `map[string]any` with nested `resource_base` key (embedded struct form)

### 4g. Import cleanup (DONE — no-op)

Since the marshal implementation stayed in `pkg/op`, no import changes
were needed. `RegisterConstructor` and `RegisterReceiverParams` remain
exported from `pkg/op`. The unexported `marshal`/`unmarshal` functions
were always package-internal.

### 4h. Tests (DONE)

- `pkg/op/resource_test.go` — ResourceBase, interface satisfaction
- `pkg/op/resource_catalog_test.go` — full catalog tests
- `pkg/op/output_test.go` — implicit edge tests via marshal round-trip
- `pkg/op/starvalue_marshal_test.go` — marshal/unmarshal, camelToSnake,
  type cache, constructors (was `marshal_test.go`)
- `pkg/op/graph_test.go` — catalog integration

## Files

| File | Action | Step |
|------|--------|------|
| `pkg/op/resource.go` | Rewrite | 4a, 4e |
| `pkg/op/resource_catalog.go` | Create | 4b, 4f |
| `pkg/op/namespace.go` | Delete | 4c |
| `pkg/op/state.go` | Delete | 4c |
| `pkg/op/graph.go` | Modify | 4c |
| `pkg/op/output.go` | Modify | 4c |
| `pkg/op/provider/file/resource.go` | Modify | 4c |
| `pkg/op/starvalue/starvalue.go` | Create | 4d |
| `pkg/op/starvalue_marshal.go` | Rename from `marshal.go` | 4d |
| `pkg/op/starvalue_marshal_test.go` | Rename from `marshal_test.go` | 4d |
| `pkg/op/resource_test.go` | Rewrite | 4h |
| `pkg/op/resource_catalog_test.go` | Create | 4h |

## Verification

1. `make build` — passes
2. `make vet` — passes
3. `make test` — passes
4. `make test-race`
5. Grep for `ResourceManager`, `NamespaceMap`, `NewResourceManager`, `NewNamespaceMap` — zero hits
6. Verify `extractResource` handles marshal round-trip (starlark struct → map[string]any → originID) — covered by `output_test.go`
