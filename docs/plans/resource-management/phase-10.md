# Package URIs — Purl Adoption (Decision #11 Implementation)

## Context

Decision #11 in `docs/plans/resource-management.md` adopts the
[package-url (purl)](https://github.com/package-url/purl-spec)
specification (ECMA-427) for canonical package identification. The
catalog URI remains a `url.URL`-compatible hierarchical key; the purl
is a separate canonical representation on the Resource.

This plan implements Decision #11 for `pkg.Resource` and updates the
constructor to align with Decision #10 (infallible constructors).

## Current State

`pkg.Resource` has a single `Name` field. The URI is `pkg:///name`
(via `url.URL{Scheme: "pkg", Path: name}`). No manager type, no
version, no purl support. The constructor accepts a string name only.

```go
// Current
type Resource struct {
    op.ResourceBase
    Name string
}
func (r *Resource) Scheme() string { return op.SchemePackage }
func (r *Resource) Host() string   { return "" }
func (r *Resource) Path() string   { return r.Name }
```

## Steps

### 1. Add Type and Version fields to pkg.Resource

**File**: `pkg/op/provider/pkg/resource.go`

```go
type Resource struct {
    op.ResourceBase
    Name    string // package name ("jq", "curl", "VisualStudioCode")
    Type    string // purl type / manager ("brew", "deb", "port", "winget")
    Version string // populated by Resolve()
}
```

### 2. Update URI component methods

**File**: `pkg/op/provider/pkg/resource.go`

```go
func (r *Resource) Scheme() string { return op.SchemePackage }
func (r *Resource) Host() string   { return r.Type }
func (r *Resource) Path() string   { return "/" + r.Name }
```

Catalog URIs:
- With type: `pkg://brew/jq`
- Without type (auto-detect): `pkg:///jq`

### 3. Add Purl() method

**File**: `pkg/op/provider/pkg/resource.go`

```go
// Purl returns the canonical package-url string (ECMA-427).
func (r *Resource) Purl() string {
    s := "pkg:" + r.Type + "/" + r.Name
    if r.Version != "" {
        s += "@" + r.Version
    }
    return s
}
```

### 4. Add Resolve() method

**File**: `pkg/op/provider/pkg/resource.go`

`Resolve()` populates `Type` from the platform's default package manager
(when the type was not specified at plan time) and `Version` from the
installed package version. This is consistent with Decision #10 — the
constructor captures portable identity, resolution populates
target-specific data.

```go
func (r *Resource) Resolve() error {
    // Type resolution requires platform context.
    // The executor injects platform before calling Resolve().
    // If Type is already set, skip manager detection.
    // Version is populated from the package manager's query.
    return nil // skeleton — implementation depends on platform injection
}
```

### 5. Update constructor to accept type

**File**: `pkg/op/provider/pkg/resource.go`

The constructor becomes infallible (Decision #10) and accepts both
name and type:

```go
func NewResource(name string) Resource {
    return Resource{Name: name}
}

func NewTypedResource(name, typ string) Resource {
    return Resource{Name: name, Type: typ}
}
```

The constructor registry accepts a string (the package name) and
produces an untyped Resource. The `Type` is set separately — either
by the planned bridge (when `manager` is specified in the Starlark
call) or by `Resolve()` at execution time.

### 6. Update init() — single infallible constructor

**File**: `pkg/op/provider/pkg/resource.go`

```go
func init() {
    op.RegisterConstructor(func(v any) (Resource, error) {
        s, ok := v.(string)
        if !ok {
            return Resource{}, fmt.Errorf("pkg.Resource: expected string name, got %T", v)
        }
        return NewResource(s), nil
    })
}
```

### 7. Update provider methods — propagate Type from Tombstone

**File**: `pkg/op/provider/pkg/provider.go`

The `Tombstone` already carries `Manager string`. On compensation, the
manager is restored from the tombstone. The `Type` on returned
Resources should match the resolved manager.

Update `Install`, `Remove`, `Upgrade` to set `Type` on returned
Resources from the resolved manager name.

### 8. Update Tombstone — align with purl type

**File**: `pkg/op/provider/pkg/resource.go`

The `Tombstone.Manager` field stores the same value as `Resource.Type`.
No structural change needed, but document that `Manager` in the
tombstone and `Type` on the resource are the same concept.

### 9. Handle winget namespace

Winget uses publisher-scoped IDs (`Microsoft.VisualStudioCode`). The
purl namespace maps to the publisher:

```
pkg:winget/Microsoft/VisualStudioCode
```

For the catalog URI: `pkg://winget/Microsoft/VisualStudioCode`
(publisher in the path, not a separate component).

The `Name` field stores the full winget ID
(`Microsoft.VisualStudioCode`). Splitting into namespace + name is a
display concern for `Purl()`:

```go
func (r *Resource) Purl() string {
    if r.Type == "winget" {
        // Split "Microsoft.VisualStudioCode" → "Microsoft" / "VisualStudioCode"
        if ns, name, ok := strings.Cut(r.Name, "."); ok {
            return "pkg:winget/" + ns + "/" + name
        }
    }
    s := "pkg:" + r.Type + "/" + r.Name
    if r.Version != "" {
        s += "@" + r.Version
    }
    return s
}
```

### 10. Update tests

**`pkg/op/provider/pkg/resource_test.go`**:
- `NewResource("jq")` produces `Resource{Name: "jq"}`
- `NewTypedResource("jq", "brew")` produces `Resource{Name: "jq", Type: "brew"}`
- URI with type: `pkg://brew/jq`
- URI without type: `pkg:///jq`
- `Purl()` with version: `pkg:brew/jq@1.7`
- Winget purl: `pkg:winget/Microsoft/VisualStudioCode`

**`pkg/op/provider/pkg/provider_test.go`**:
- Verify returned Resources have `Type` set from resolved manager

## Purl Type Mapping

| devlore source | purl type | namespace | Example URI | Example purl |
| --- | --- | --- | --- | --- |
| `brew` | `brew` | — | `pkg://brew/jq` | `pkg:brew/jq@1.7` |
| `brew` (cask) | `brew` | — | `pkg://brew/firefox` | `pkg:brew/firefox?cask=true` |
| `port` | `port` | — | `pkg://port/jq` | `pkg:port/jq@1.7` |
| `apt` | `deb` | distro | `pkg://deb/curl` | `pkg:deb/debian/curl@7.50.3-1` |
| `dnf`/`yum` | `rpm` | — | `pkg://rpm/nginx` | `pkg:rpm/nginx@1.24` |
| `winget` | `winget` | publisher | `pkg://winget/Microsoft.VisualStudioCode` | `pkg:winget/Microsoft/VisualStudioCode` |

## Files Modified

| File | Action |
| --- | --- |
| `pkg/op/provider/pkg/resource.go` | Add Type/Version fields, update URI methods, add Purl(), Resolve(), infallible constructor |
| `pkg/op/provider/pkg/resource_test.go` | Tests for new fields, URI, Purl(), Resolve() |
| `pkg/op/provider/pkg/provider.go` | Set Type on returned Resources from resolved manager |
| `pkg/op/provider/pkg/provider_test.go` | Verify Type propagation |

## Relationship to Other Plans

- **Decision #10** (`phase-9.md`): This plan depends on the
  infallible constructor and Resolve() pattern established there. Step 6
  here implements the pkg-specific constructor per that pattern.
- **Decision #11** (`resource-management.md`): This plan is the
  implementation of Decision #11.
