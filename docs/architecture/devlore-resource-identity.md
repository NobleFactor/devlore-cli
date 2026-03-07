# Resource Identity

This document defines how resources are identified across the system.
Every resource has a URI — a single string that uniquely identifies it
within the resource catalog. The URI is the only identity contract
between a resource and the rest of the system.

See also:
- [devlore-resource-management.md](devlore-resource-management.md) — catalog, shadows, tombstones, resolution lifecycle
- [devlore-mem-resource.md](devlore-mem-resource.md) — `mem:` scheme and callables
- [mem-resource plan](../plans/mem-resource.md) — implementation plan (Phase 0: interface change)

## 1. URI as the Single Identity

A resource's identity is its URI. The catalog keys on it. The slot
system passes it. Pre-flight validation checks it. Receipt serialization
records it. No other method on the Resource interface participates in
identity — the URI string is the sole contract.

```go
type Resource interface {
    URI() string
    Resolve() error
    resourceBase() *ResourceBase
}
```

`URI()` returns a cached string. It does not recompute on every call.
Each concrete type constructs its URI at creation time (or after
`Resolve()` mutates identity-bearing fields) and caches the result.

`Resolve()` populates provider-specific metadata via I/O (e.g.,
`os.Stat` for files, compilation for callables). If resolution changes
identity-bearing fields (e.g., path canonicalization), the cached URI
is updated.

`resourceBase()` is the interface seal — it returns a pointer to the
embedded `ResourceBase`, allowing the catalog to stamp `id` and
`originID`. Not part of the identity contract.

## 2. URI Syntax — RFC 3986

All resource URIs conform to [RFC 3986](https://datatracker.ietf.org/doc/html/rfc3986).

```
URI = scheme ":" hier-part [ "?" query ] [ "#" fragment ]

hier-part = "//" authority path-abempty     ← authority form
          / path-absolute                   ← no authority, starts with /
          / path-rootless                   ← no authority, no leading /
          / path-empty                      ← nothing after scheme:

authority = [ userinfo "@" ] host [ ":" port ]
```

**Key rules:**

1. **Scheme** is mandatory — `ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )`
2. **Authority** is optional — present only after `://`
3. **Path** is always present (can be empty)
4. When authority is present, path must start with `/` or be empty
5. When authority is absent, path cannot start with `//`

Each scheme defines its own syntax within these rules. The system does
not impose a single decomposition (scheme + host + path) on all
resources. The URI is an opaque string to the catalog; only the resource
type that produced it understands its internal structure.

### Hierarchical vs Opaque URIs

RFC 3986 distinguishes two URI forms:

- **Hierarchical**: starts with `//` after the scheme. Go's `url.Parse`
  populates `Host` and `Path`. Used when the URI references a host or
  filesystem path: `file:///etc/foo`.

- **Opaque**: no `//` after the scheme. Go's `url.Parse` populates the
  `Opaque` field; `Host` and `Path` are empty. Used when the URI names
  a registry entry or wraps an inner URI: `pkg:brew/jq`,
  `mem:callable/file.Reducer/myfn`, `appnet:https%3A//example.com/path`,
  `svc:nginx`.

The choice is per-scheme. Resources that reference external systems
with hosts or paths use hierarchical URIs. Resources that name entries
in a local registry use opaque URIs.

Go's `url.URL` struct reflects this:

| Field | Hierarchical (`file:///etc/foo`) | Opaque (`pkg:brew/jq@1.7`) |
|---|---|---|
| `Scheme` | `file` | `pkg` |
| `Opaque` | (empty) | `brew/jq@1.7` |
| `Host` | (empty) | (empty) |
| `Path` | `/etc/foo` | (empty) |
| `Fragment` | (empty) | (empty) |

For opaque URIs, the `Opaque` field carries the full identity after the
scheme colon. The `Fragment` (after `#`) is metadata — it identifies a
specific instance or usage context but is NOT part of the catalog key.
The catalog strips the fragment when keying; two URIs that differ only
in fragment resolve to the same resource.

### Fragment as Instance Context

The fragment component (`#...`) identifies a specific usage of a
resource without changing its identity. For `mem:` resources, the
fragment carries the graph node ID where the resource was created:

```
mem:callable/file.Reducer/myfn#node1
mem:callable/file.Reducer/myfn#node2
```

Both resolve to the same catalog entry (`mem:callable/file.Reducer/myfn`).
The fragment tells the resolution layer which instance is being
referenced. This supports:

- **Deduplication**: same callable used at multiple nodes shares one
  catalog entry.
- **Instance tracking**: the fragment distinguishes specific usages for
  diagnostics, logging, and slot resolution.

### Content Hash as Metadata

For `mem:` resources, a content hash (SHA-256 of the `Data` field) is
stored as a field on the resource struct, NOT in the URI. The hash
serves two purposes:

1. **Change detection**: when a resource with the same URI appears with
   a different hash, the catalog knows the content changed and creates
   a shadow.
2. **Integrity verification**: the hash can be checked after transfer
   or deserialization to confirm the content is intact.

Putting the hash in the URI would mean the URI changes whenever the
content changes — defeating the catalog's ability to track that the
"same" resource mutated. A stable URI with a mutable hash is the
correct model for shadowing.

### mem:callable URI Structure

The `mem:callable` URI has three segments:

```
mem:callable/<function-type>/<instance-name>
```

| Segment | Meaning | Example |
|---|---|---|
| `callable` | Content type — maps to `callable.Resource` | Always `callable` |
| `<function-type>` | The named Go function type the callable adapts to | `file.Reducer`, `file.Actor`, `Predicate` |
| `<instance-name>` | The callable's identity within its function type | `count_python_files`, `file.walk_tree.fn` |

**Function type** — the named type annotated with `+devlore:callable`.
For anonymous function types (bare `func(...)` parameters without a
named type), the fallback is `<action>.<param>` — the action method
name and the parameter name that receives the callable. Named types
are the primary path.

**Instance name** — for named `def` functions, the function name is the
default. For lambdas, the instance name is derived from the call site:
`<action>.<param>`.

```
mem:callable/file.Reducer/count_python_files     ← named def
mem:callable/file.Reducer/file.walk_tree.fn      ← lambda at walk_tree's fn param
mem:callable/Predicate/is_ready                  ← named predicate
```

**Name collisions**: two functions with the same name and function type
produce the same base URI. If their content hashes differ, the second
shadows the first in the catalog. This is intentional — the catalog
treats it as a newer version of the same callable. Slot data is carried
inline, so no data is lost at the node level.

## 3. Scheme Registry

| Scheme | Form | URI Examples | Notes |
|---|---|---|---|
| `file` | Hierarchical | `file:///etc/foo`, `file:///usr/local/bin/jq` | [RFC 8089](https://datatracker.ietf.org/doc/html/rfc8089). Empty authority, absolute path. |
| `pkg` | Opaque | `pkg:brew/jq`, `pkg:deb/curl@1.7`, `pkg:winget/Microsoft/VSCode` | [Package URL (purl)](https://github.com/package-url/purl-spec) compliant. `URI()` and `Purl()` converge. |
| `svc` | Opaque | `svc:nginx`, `svc:postgresql` | Service name. Fragment reserved for future instance disambiguation (e.g., `svc:postgresql#port-5433`). |
| `appnet` | Opaque | `appnet:https%3A//example.com/path` | Wraps an inner URI with targeted escaping (`#` and `?` escaped). Renamed from `net`. |
| `git` | Opaque | `git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Fmain.go#abc123` | Opaque repo URL, optional path query, commit hash as fragment. |
| `mem` | Opaque | `mem:callable/file.Reducer/myfn`, `mem:json/config` | `content-type/[qualifier/]name`. Hash is metadata, not identity. |

Schemes are registered as constants in `pkg/op/resource.go`:

```go
const (
    SchemeFile    = "file"
    SchemeGit     = "git"
    SchemePackage = "pkg"
    SchemeService = "svc"
    SchemeMem     = "mem"
    SchemeAppNet  = "appnet"    // renamed from "net"
)
```

## 4. URI Construction

Each concrete resource type owns its URI construction. There is no
shared `NewURI(r Resource)` method that dispatches through virtual
calls. Each type builds its URI from its own fields using whatever
logic fits its scheme.

```go
// file: RFC 8089 — hierarchical, empty authority, absolute path
func (r *Resource) buildURI() string {
    return "file://" + r.SourcePath // SourcePath is always absolute
}

// pkg: purl-compliant opaque URI — URI() and Purl() converge
func (r *Resource) buildURI() string {
    s := "pkg:" + r.Type + "/" + r.Name
    if r.Version != "" {
        s += "@" + r.Version
    }
    return s
}

// svc: opaque, service name only
func (r *Resource) buildURI() string {
    return "svc:" + r.Name
}

// appnet: opaque, wraps inner URI with targeted escaping
func (r *Resource) buildURI() string {
    return "appnet:" + escapeInnerURI(r.URL)
}

// mem: opaque — content-type/function-type/instance-name
func (r *Resource) buildURI() string {
    return "mem:" + r.ContentType + "/" + r.FuncType + "/" + r.Name
}

// git: opaque repo URL, optional path query, commit fragment
func (r *Resource) buildURI() string {
    s := "git:" + url.PathEscape(r.RepoURL)
    if r.Path != "" {
        s += "?path=" + url.QueryEscape(r.Path)
    }
    if r.Commit != "" {
        s += "#" + r.Commit
    }
    return s
}
```

The URI is computed once and cached. `URI()` returns the cached string:

```go
func (r *Resource) URI() string { return r.uri }
```

If `Resolve()` changes identity-bearing fields (e.g., `filepath.Abs`
canonicalizes a relative path), the cached URI is updated.

## 5. What Changed — Interface Simplification

The original Resource interface exposed four URI-related methods:

```go
// Before — shoehorned URI decomposition
type Resource interface {
    URI() string
    Scheme() string   // "file", "git", "pkg", ...
    Host() string     // authority component
    Path() string     // path component
    Resolve() error
    resourceBase() *ResourceBase
}
```

`Scheme()`, `Host()`, and `Path()` existed to serve `NewURI(r Resource)`
in `ResourceBase`, which assembled them into a `url.URL` on every
`URI()` call. This imposed a single `scheme://host/path` decomposition
on all resources — a decomposition that didn't fit:

| Resource | Host() meant | Path() meant | Actual form |
|---|---|---|---|
| file | empty string | filesystem path | Hierarchical — fits |
| git | empty string | clone path | Opaque — never fit |
| net (now appnet) | hostname | URL path | Moving to opaque wrapper |
| pkg | package manager type | package name | Opaque purl — never fit |
| service | empty string | service name | Opaque — never fit |
| mem | (N/A) | (N/A) | Opaque — never fit |

`Host()` and `Path()` had no consistent semantics — they were just
"the two fields that go into `url.URL{Host, Path}`." Code that called
`r.Host()` couldn't know what it meant without knowing the concrete
type. And `NewURI()` recomputed a `url.URL` struct on every call,
which is unnecessary work for an immutable identity string.

The simplified interface:

```go
// After — URI is the only identity contract
type Resource interface {
    URI() string
    Resolve() error
    resourceBase() *ResourceBase
}
```

- `URI()` returns a cached string (no recomputation)
- Each type constructs its URI from its own fields
- `ResourceBase` retains parsing helpers (`Scheme()`, `Host()`, `Path()`)
  for code that needs to decompose a URI after the fact — but these are
  not interface methods

## 6. ResourceBase Helpers

`ResourceBase` provides fallback methods that parse the stored `uri`
field via `net/url.Parse`. These are available to any code that holds
a `ResourceBase` (i.e., any resource type), but they are NOT part of
the `Resource` interface:

```go
// Convenience parsers — NOT interface methods.
func (b *ResourceBase) Scheme() string   { /* parse b.uri */ }
func (b *ResourceBase) Opaque() string   { /* parse b.uri — non-empty for opaque URIs */ }
func (b *ResourceBase) Host() string     { /* parse b.uri — non-empty for hierarchical URIs */ }
func (b *ResourceBase) Path() string     { /* parse b.uri — non-empty for hierarchical URIs */ }
func (b *ResourceBase) Fragment() string { /* parse b.uri */ }
```

For opaque URIs (`pkg:`, `svc:`, `appnet:`, `mem:`), `Opaque()` returns
the data after the scheme colon; `Host()` and `Path()` return empty.
For hierarchical URIs (`file:`), `Opaque()` returns empty; `Host()` and
`Path()` return the authority and path components.

These are useful for generic code that needs to decompose an unknown
resource's URI (e.g., display, logging, routing by scheme). They parse
on every call — callers that need repeated access should cache the
result.

### git: URI Structure

```
git:<encoded-repo-url>[?path=<path>[&mode=blob|tree]]#<commit-hash>
```

The `git` scheme uses the opaque form. The repository URL is the base
identity. The commit hash in the fragment pins an immutable version.

| Component | `url.URL` Field | Role | Example |
|---|---|---|---|
| `git:` | `Scheme` | Routes to git resolver | Always `git` |
| `<encoded-repo-url>` | `Opaque` | Base resource — the repository | `https%3A%2F%2Fgithub.com%2Forg%2Frepo` |
| `?path=<path>` | `RawQuery` | Optional: file or directory within the repo | `path=src%2Fmain.go` |
| `&mode=blob\|tree` | `RawQuery` | Optional: file or directory | `mode=blob` |
| `#<commit-hash>` | `Fragment` | Immutable version pin — shadow key | `e5a4f3b22c1d...` |

**Catalog keying**: the catalog strips the fragment. Two URIs that
differ only in commit hash resolve to the same catalog entry — the
newer commit shadows the older one. The query (`?path=...`) IS part
of the catalog key, so different files in the same repo are distinct
resources. A URI with no `?path=` references the repo root.

```
git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Fmain.go#abc123
git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Fmain.go#def456
  → same catalog entry, def456 shadows abc123

git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Fmain.go#abc123
git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Futil.go#abc123
  → different catalog entries (different files)

git:https%3A%2F%2Fgithub.com%2Forg%2Frepo#abc123
  → repo root (no path query)
```

**Why fragment for commit hash**: RFC 3986 defines the fragment as
client-side state identification — not sent to the server. A commit
hash identifies a specific snapshot (state) of the repository. The
catalog uses it as the shadow/version key, consistent with how `mem:`
uses the fragment for instance context.

**Mutable references**: branch names and tags are mutable — they can
move to a new commit. For resources that should track "latest on main"
rather than pinning a commit, the `ref` query parameter provides a
resolution-time lookup:

```
git:https%3A%2F%2Fgithub.com%2Forg%2Frepo?path=src%2Fmain.go&ref=main
```

`Resolve()` resolves the mutable ref to a commit hash and updates the
cached URI with the `#<commit>` fragment. Before resolution, the URI
has no fragment (unpinned). After resolution, the fragment is set
(pinned). This mirrors how `file:` resources resolve relative paths
to absolute paths.

**Local repos**: use the `file:` scheme as the inner URL, following
the same encoding as `appnet:`:

```
git:file%3A%2F%2F%2Fhome%2Fuser%2Frepo?path=src%2Fmain.go#abc123
```

## 8. Supersession Table

| Existing Document | Section | Status | Notes |
|---|---|---|---|
| [devlore-resource-management.md](devlore-resource-management.md) | §3.3 Resource Types by URI Scheme | **Extended** | Scheme table updated; URI construction model changed |
| [devlore-resource-management.md](devlore-resource-management.md) | §2 Architectural Summary | **Extended** | Resource interface simplified |
