# Phase 7: Provider Method Migration to Resource-Typed Parameters

## Context

Phases 1–6 built the resource management infrastructure: `Resource`/`Tombstone`
interfaces, `ResourceBase`, `ResourceCatalog`, the `starvalue` marshaling layer,
constructor registry, and resource types for file, git, service, and pkg
providers. Phase 6 embedded `ProviderBase` in git/service/pkg/shell and removed
`io.Writer`/`Platform` from their signatures.

Phase 7 completes the migration by changing all provider method signatures to
use Resource-typed parameters. Every parameter that identifies an external
entity (file path, URL, package name, service name) becomes a typed Resource.
Configuration parameters (modes, flags, refs, managers) remain strings/bools.

This phase also creates `net.Resource` (a new resource type for network URLs),
migrates archive/encryption/net/template providers (ProviderBase embedding +
method signatures), and adds typed Tombstones for archive and encryption.

**Repo**: devlore-cli
**Branch**: `feature/resource-management-phase-7`

## What will be true when this phase is complete

1. **`net.Resource` exists** — wraps `*url.URL` in `SourceURL` field, scheme
   `"net"`, canonical URI (`net://<canonical-url>`), constructors registered.

2. **All resource-identifying parameters are typed** — no raw strings for file
   paths, URLs, package names, or service names in any provider method.

3. **All resource-producing methods return typed Resources** — `git.Clone`
   returns `git.Resource`, `pkg.Install` returns `[]pkg.Resource`,
   `archive.Extract` returns `[]file.Resource`.

4. **archive and encryption have typed Tombstones** — no more `map[string]any`
   for compensation state.

5. **archive/encryption/net/template embed `ProviderBase`** — consistent with
   git/service/pkg/shell (done in Phase 6).

6. **`make check` passes.**

## Design Decisions

### D1: net.Resource uses `net://` scheme with canonical URI

`Scheme()` returns `"net"` — the resource type scheme, not the URL's transport
scheme. Add `SchemeNet = "net"` to `resource.go`.

`URI()` produces a canonical form: `net://<canonical-url>`. The original URL's
transport scheme is stripped from the URI because resource identity is
transport-independent (`http://example.com/f` and `https://example.com/f` are
the same resource). The provider uses `SourceURL.Scheme` for actual requests.

Canonicalization rules (RFC 3986 normalization):

- **Lowercase hostname**: `Example.COM` → `example.com`
- **Strip default ports**: `:80` for http, `:443` for https, `:21` for ftp
- **Normalize percent-encoding**: uppercase hex digits (`%af` → `%AF`);
  decode unreserved characters (`%41` → `A`) per RFC 3986 §2.3
- **Strip trailing `/`**: `/path/` → `/path`
- **Collapse double `//`**: `/a//b` → `/a/b` (except leading `//`)
- **Sort query parameters**: by key, alphabetically

Example: `https://Example.COM:443/path//to/%61%62?z=1&a=2` →
`net://example.com/path/to/ab?a=2&z=1`

The original URL is always available via `Resource.SourceURL` for the provider
to use in HTTP requests.

### D2: git.Clone takes url as net.Resource, destination as file.Resource

```go
Clone(url net.Resource, destination file.Resource) (git.Resource, Tombstone, error)
```

The URL identifies a network resource (the remote repo). The destination
identifies a file resource (the local directory). The output is a git resource
(the completed clone with URL, ClonePath, and Ref populated).

### D3: archive.Extract returns []file.Resource

```go
Extract(source file.Resource, prefix file.Resource) ([]file.Resource, Tombstone, error)
```

The archive and extraction directory are file resources. The output is a flat
list of all extracted file resources. The tree structure is implicit in the
paths. The action layer shadows each resource individually (per Phase 5 D7:
`[]Resource` → iterate, shadow each).

### D4: Non-resource returns stay unchanged

- `template.Render` returns `[]byte` (rendered content, not a file)
- `net.Download` returns `[]byte` (downloaded content, not a file)
- Service compensable methods change from `string` to `service.Resource`
  (enables catalog shadowing for service state)
- Service predicates keep `bool` returns

### D5: shell provider unchanged

Commands are strings, not external state. No migration needed.

## Implementation Steps

### 7a. net.Resource type

**Files**: `pkg/op/provider/net/resource.go` (new),
`pkg/op/provider/net/resource_test.go` (new), `pkg/op/resource.go`

```go
type Resource struct {
    op.ResourceBase
    SourceURL *url.URL
}

func (r *Resource) Scheme() string { return "net" }
func (r *Resource) Host() string   { return strings.ToLower(r.SourceURL.Hostname()) }
func (r *Resource) Path() string   { return r.SourceURL.Path }
func (r *Resource) URI() string    { return "net://" + r.canonicalAuthority() }
```

`URI()` does not use `NewURI` — it constructs the canonical form directly
via `canonicalAuthority()` which applies all canonicalization rules (lowercase
host, strip default port, normalize percent-encoding, strip trailing `/`,
collapse `//`, sort query params).

Constructor: parse string via `url.Parse`. No scheme filtering — the provider
decides what it can process. Execution-time and plan-time constructors are
identical (no I/O for URLs).

- [ ] Create `net.Resource` struct with `SourceURL *url.URL` field
- [ ] Implement `Resource` interface methods (`Scheme` returns `"net"`)
- [ ] Implement `canonicalAuthority()` with full RFC 3986 normalization
- [ ] Register execution-time and plan-time constructors
- [ ] Add `SchemeNet = "net"` to `resource.go`
- [ ] Tests: constructor round-trip, URI canonicalization (host/port/encoding/
      slashes/query sorting), scheme/host/path

### 7b. git provider method migration

**Files**: `pkg/op/provider/git/provider.go`,
`pkg/op/provider/git/provider_test.go`

| Method | Current | Migrated |
|--------|---------|----------|
| Clone | `(url, path string) (string, Tombstone, error)` | `(url net.Resource, destination file.Resource) (git.Resource, Tombstone, error)` |
| Checkout | `(repo, ref string) (string, error)` | `(repo git.Resource, ref string) (git.Resource, error)` |
| Pull | `(repo string) (string, error)` | `(repo git.Resource) (git.Resource, error)` |

- `url` → `net.Resource` (network location)
- `destination` / `repo` → `file.Resource` / `git.Resource` (local path)
- `ref` stays `string` (configuration)
- All results → `git.Resource` (populated clone)

- [ ] Update Clone, Checkout, Pull signatures
- [ ] Update internals: `url.SourceURL.String()`, `destination.SourcePath`, `repo.ClonePath`
- [ ] Update `cloneFn` test hook signature
- [ ] Update tests

### 7c. service provider method migration

**Files**: `pkg/op/provider/service/provider.go`,
`pkg/op/provider/service/provider_test.go`

All 5 compensable methods: `name string` → `name service.Resource`,
return `string` → `service.Resource`.

All 3 predicates: `name string` → `name service.Resource`,
return `bool` unchanged.

| Method | Current | Migrated |
|--------|---------|----------|
| Start/Stop/Enable/Disable/Restart | `(name string) (string, Tombstone, error)` | `(name service.Resource) (service.Resource, Tombstone, error)` |
| Exists/Enabled/Running | `(name string) (bool, error)` | `(name service.Resource) (bool, error)` |

- [ ] Update all 8 method signatures
- [ ] Internals use `name.Name`
- [ ] Status messages go to `p.Context().Writer` (no longer returned)
- [ ] Update tests

### 7d. pkg provider method migration

**Files**: `pkg/op/provider/pkg/provider.go`,
`pkg/op/provider/pkg/provider_test.go`

| Method | Current | Migrated |
|--------|---------|----------|
| Install/Remove/Upgrade | `(packages []string, manager string, cask bool) ([]string, Tombstone, error)` | `(packages []pkg.Resource, manager string, cask bool) ([]pkg.Resource, Tombstone, error)` |
| Update | `(manager string) (string, error)` | unchanged (manager is config) |
| Installed/NotInstalled | `(name string) (bool, error)` | `(name pkg.Resource) (bool, error)` |
| VersionGTE | `(name, version string) (bool, error)` | `(name pkg.Resource, version string) (bool, error)` |

- `manager`, `cask`, `version` stay as config params
- [ ] Update Install/Remove/Upgrade input and output types
- [ ] Update predicate signatures
- [ ] Internals: iterate `packages` using `.Name` field
- [ ] Update tests

### 7e. archive provider migration

**Files**: `pkg/op/provider/archive/provider.go`,
`pkg/op/provider/archive/resource.go` (new — Tombstone only),
`pkg/op/provider/archive/resource_test.go` (new),
`pkg/op/provider/archive/provider_test.go`

```
Extract(source, prefix string) (string, map[string]any, error)
  ↓
Extract(source, prefix file.Resource) ([]file.Resource, Tombstone, error)
```

Tombstone:
```go
type Tombstone struct {
    op.TombstoneBase
    ExtractedDir string
    Files        []string
}
```

- [ ] Create `archive.Tombstone` type
- [ ] Update Extract: inputs → `file.Resource`, output → `[]file.Resource`
- [ ] Update CompensateExtract to accept typed `Tombstone`
- [ ] Embed `ProviderBase`
- [ ] Update tests

### 7f. encryption provider migration

**Files**: `pkg/op/provider/encryption/provider.go`,
`pkg/op/provider/encryption/resource.go` (new — Tombstone only),
`pkg/op/provider/encryption/resource_test.go` (new),
`pkg/op/provider/encryption/provider_test.go`

```
DecryptSopsFile(sourceFile file.Resource, destinationFilename string)
  → (file.Resource, map[string]any, error)
  ↓
DecryptSopsFile(source file.Resource, destination file.Resource)
  → (file.Resource, Tombstone, error)
```

Tombstone:
```go
type Tombstone struct {
    op.TombstoneBase
    DecryptedPath string
}
```

- [ ] Create `encryption.Tombstone` type
- [ ] Update DecryptSopsFile signature
- [ ] Implement `CompensateDecryptSopsFile` (currently panics "not implemented")
- [ ] Embed `ProviderBase`
- [ ] Update tests

### 7g. net provider migration

**Files**: `pkg/op/provider/net/provider.go`,
`pkg/op/provider/net/provider_test.go`

```
Download(url string) ([]byte, error)
  ↓
Download(url net.Resource) ([]byte, error)
```

Return stays `[]byte` (raw downloaded content).

- [ ] Update Download signature
- [ ] Internals use `url.SourceURL.String()` for HTTP request
- [ ] Embed `ProviderBase`
- [ ] Update tests

### 7h. template provider migration

**Files**: `pkg/op/provider/template/provider.go`,
`pkg/op/provider/template/provider_test.go`

```
Render(templateData map[string]any, source, path, project string, content []byte)
  → ([]byte, error)
  ↓
Render(templateData map[string]any, source file.Resource, path file.Resource,
       project string, content []byte) → ([]byte, error)
```

Return stays `[]byte`. `templateData`, `project`, `content` stay unchanged
(configuration/data, not resources).

- [ ] Update Render signature
- [ ] Internals use `source.SourcePath`, `path.SourcePath`
- [ ] Embed `ProviderBase`
- [ ] Update tests

### 7i. Regenerate and verify

- [ ] `make build` — regenerate all gen files
- [ ] `make check` — full quality gate
- [ ] Update master plan (`docs/plans/resource-management.md`) phase status
- [ ] Update architecture doc Section 3.3 and Section 11

## Key Files

| File | Action | Step |
|------|--------|------|
| `pkg/op/resource.go` | Add `SchemeNet` constant | 7a |
| `pkg/op/provider/net/resource.go` | New | 7a |
| `pkg/op/provider/net/resource_test.go` | New | 7a |
| `pkg/op/provider/net/provider.go` | Modify (embed ProviderBase, signature) | 7g |
| `pkg/op/provider/net/provider_test.go` | Modify | 7g |
| `pkg/op/provider/git/provider.go` | Modify (signatures) | 7b |
| `pkg/op/provider/git/provider_test.go` | Modify | 7b |
| `pkg/op/provider/service/provider.go` | Modify (signatures + returns) | 7c |
| `pkg/op/provider/service/provider_test.go` | Modify | 7c |
| `pkg/op/provider/pkg/provider.go` | Modify (signatures + returns) | 7d |
| `pkg/op/provider/pkg/provider_test.go` | Modify | 7d |
| `pkg/op/provider/archive/provider.go` | Modify (embed ProviderBase, signatures) | 7e |
| `pkg/op/provider/archive/resource.go` | New (Tombstone) | 7e |
| `pkg/op/provider/archive/resource_test.go` | New | 7e |
| `pkg/op/provider/archive/provider_test.go` | Modify | 7e |
| `pkg/op/provider/encryption/provider.go` | Modify (embed ProviderBase, signatures) | 7f |
| `pkg/op/provider/encryption/resource.go` | New (Tombstone) | 7f |
| `pkg/op/provider/encryption/resource_test.go` | New | 7f |
| `pkg/op/provider/encryption/provider_test.go` | Modify | 7f |
| `pkg/op/provider/template/provider.go` | Modify (embed ProviderBase, signatures) | 7h |
| `pkg/op/provider/template/provider_test.go` | Modify | 7h |

## Verification

1. `make build` — regenerates all gen files
2. `make vet` — no vet issues
3. `make test` — all tests pass
4. `make test-race` — no races
5. `make check` — full quality gate
6. Grep for raw string params in provider methods — only config params remain
7. Verify `net.Resource` constructor round-trip: string URL → net.Resource → URI
8. Verify `archive.Extract` returns `[]file.Resource` with correct URIs
9. Verify `encryption.CompensateDecryptSopsFile` works (no more panic)

## Related Documents

- [Phase 5](./phase-5.md) — Executor catalog integration
- [Phase 6](./phase-6.md) — Provider resource types, context injection
- [Phase 8](./phase-8.md) — Generated bridge tests
- [Architecture](../../architecture/devlore-resource-management.md)
- [Master Plan](../resource-management.md)
