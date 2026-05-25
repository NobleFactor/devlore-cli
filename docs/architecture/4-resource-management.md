# Resource Management: URI-Based Resource Tracking

This document describes the resource management architecture for devlore-cli:
how providers track external state through typed resource handles, how the
namespace resolves URI-based identity across the execution graph, and how
tombstone recovery unifies under a single executor-owned mechanism.

See also:

- [Resource Management Plan](../plans/resource-management.md) — full
  implementation plan with phases, requirements, and file listings
- [4.4-root-path-triad.md](4.4-root-path-triad.md) — op.Root, op.Path,
  and op.RecoverySite interaction architecture

## 1. The Lineage Problem

Two nodes in an execution graph can target the same filesystem path with no
dependency edge between them. The graph has no way to detect this because paths
are opaque strings — the system treats `"/etc/foo"` as a value, not an identity.

Consider this Starlark plan:

```python
# Shadow-write problem: no implicit ordering
plan.file.write_text(destination="/etc/foo", content="v2", mode=0o644)
result = plan.file.read(path="/etc/foo")   # reads original, not v2
```

The write and read create separate nodes with no edge. The executor may run
them in any order. If read runs first, it gets the original content. If write
runs first, the read gets v2. The outcome depends on scheduling — a silent
race condition.

The root cause is that the current system tracks **values** but not
**identity**. Nodes communicate via slot values (strings, ints, file modes)
and explicit promise passing (`*Output`). There is no mechanism to say "this
path was modified by node A, so node B must wait."

### Current State vs. Gaps

| Aspect | Current State | Gap |
|--------|--------------|-----|
| Slot values | `map[string]any` — strings, ints, modes | No resource identity; `"/etc/foo"` is just a string |
| Dependency edges | Created by explicit `*Output` promise passing | No implicit edges from shared paths |
| Tombstone recovery | Per-provider (`file.Provider` only) | No unified mechanism; other providers have none |
| Conflict detection | None | Two nodes targeting same path = silent race |
| Provider params | Raw types (strings, modes, bools) | No distinction between source path, destination path, or URI |

## 2. Architectural Summary

The resource management architecture separates **intent** (planning) from
**reality** (execution). A graph is planned once and can be executed on
many machines — local or remote. This forces a clean separation:

- **Coercion** (plan time): Type-tagging raw values to typed Resources.
  Pure — no I/O, no `os.Stat`. The planner knows *what type* a slot
  should hold.
- **Resolution** (execution time): Metadata population against a target
  machine. The executor knows *what exists* on each target.

Two core components and a shadowing mechanism:

```
              ┌─────────────────────────────────────────────┐
              │           ResourceCatalog                     │
              │                                               │
              │  entries: []Resource  (append-only ledger)    │
              │  byID:    map[ID]int  (id → index)           │
              │  ns:      map[URI]ID  (namespace)             │
              │  nextID:  int         (monotonic counter)     │
              │                                               │
              │  Resolve(uri) → id                            │
              │  Shadow(resource, originID) → id              │
              │  Lookup(id) → Resource, bool                  │
              │  Current(uri) → id                            │
              │  Len() → int                                  │
              └─────────────────────────────────────────────┘
                                    │
                                    │ stores
                                    ▼
              ┌─────────────────────────────────────────────┐
              │  Resource (interface)                        │
              │    URI() string                              │
              │    resourceBase() *ResourceBase  (sealed)    │
              │                                               │
              │  ResourceBase (embedded struct)               │
              │    uri      string                           │
              │    id       string                           │
              │    originID string                           │
              └─────────────────────────────────────────────┘
```

**Resource** (`pkg/op/resource.go`) — An interface sealed by the unexported
`resourceBase()` method. Only types embedding `ResourceBase` can implement
it. `ResourceBase` holds three private identity fields: `uri` (logical
address), `id` (unique catalog key), and `originID` (the node that produced
it, empty if pre-existing). Provider resources embed `ResourceBase` and add
domain-specific fields (e.g., `file.Resource` adds `SourcePath`, `Inode`,
etc.).

**ResourceCatalog** (`pkg/op/resource_catalog.go`) — A single compositor
that owns both the append-only ledger and the URI→ID namespace. One per
Graph. The catalog stamps `id` and `originID` on the `ResourceBase` when
a resource is cataloged. The ledger stores `Resource` interface values,
enabling polymorphic access to the actual typed resources.

**Shadowing** — When `plan.file.write_text(destination="/etc/foo", ...)`
executes during planning:

1. A new Node is created in the graph
2. `catalog.Shadow(resource, nodeID)` catalogs the resource and updates the namespace
3. The namespace now maps `file:///etc/foo` to the new resource ID
4. Any later `plan.file.read(path="/etc/foo")` calls `catalog.Resolve`, gets
   the shadowed version, and the executor knows it depends on the write node
   via `originID`

## 3. Resource Types

### 3.1 File Resource (Implemented)

The file provider embeds `op.ResourceBase` and adds filesystem-specific
metadata. This is the reference implementation for all future resource types.

```go
// pkg/op/provider/file/resource.go
type Resource struct {
    op.ResourceBase
    SourcePath op.Path
    Inode      uint64
    Device     uint64
    Size       int64
    Mode       os.FileMode
    ModTime    time.Time
    Checksum   string
}
```

Each field serves a specific purpose:

| Field | Source | Purpose |
|-------|--------|---------|
| `op.ResourceBase` | Embedded | uri, id, originID — identity tracking (private, stamped by catalog) |
| `SourcePath` | `op.Path` via `Root.NewPath` | Filesystem path for I/O: `Rel()` for confined, `Abs()` for unconfined |
| `Inode` | `syscall.Stat_t.Ino` | Physical identity (survives rename) |
| `Device` | `syscall.Stat_t.Dev` | Partition identity (same-device rename guarantee) |
| `Size` | `os.FileInfo.Size()` | Content length |
| `Mode` | `os.FileInfo.Mode().Perm()` | Permission bits |
| `ModTime` | `os.FileInfo.ModTime()` | Last modification time |
| `Checksum` | `checksumFile()` (SHA-256) | Content integrity |

**Three resource states:**

- **Pending** — Entry exists in the namespace (URI claimed) but has not yet
  been observed (discovery) or produced (creation). Plan-time entries are
  born here; runtime constructors insert as Pending then transition to
  Active before returning. Metadata may be empty.
- **Active** — Entry has been observed-as-existing (discovery) or freshly
  created (production). Metadata is populated. Consumers can trust the
  Resource.
- **Gone** — Internal, reactive. Set by the catalog when an attempt to
  access the underlying resource via `r.Resolve()` fails. Not driven by
  explicit "delete" calls. Observed during DiscoverResource, reconciliation,
  or any other operation that touches the underlying state.

The catalog owns state transitions. Resource providers' `Resolve()` returns
success or failure; the catalog wraps the call and applies the transition
(`Active` on success, `Gone` on failure). The state field on
`op.ResourceBase` is set by catalog code only — providers never write to it
directly. This puts the burden of lifecycle management in one place.

See §6.2 for the full operation-by-operation rules.

**Discovery**: `NewResource` (`resource.go:47`) performs `os.Stat` and
populates all metadata fields. In the target architecture, `NewResource` is
called only during execution (on the target machine), never during planning.
At plan time, `ResourceFromPath` creates a typed but metadata-empty Resource.

**Post-write refresh**: After a successful mutation, `RefreshMetadataWith`
(`resource.go:146`) re-stats the file to capture kernel-assigned identity
(Inode, Device) and updates the checksum. The write path computes the checksum
during the write via `io.MultiWriter` to avoid a redundant disk read.

### 3.2 File Tombstone (Implemented)

A tombstone records where a file was moved for recovery and where it originally
lived:

```go
// pkg/op/provider/file/resource.go
type Tombstone struct {
    op.TombstoneBase          // Resource preserves true identity (SourcePath = home)
    RecoveryID string         // Opaque ID from RecoverySite (empty if nothing existed)
}
```

Tombstones are the undo receipts for destructive operations. The embedded
Resource always preserves its true identity — `SourcePath` (an `op.Path`)
is the file's home. `RecoveryID` is the opaque identifier returned by
`RecoverySite.ArchiveFile`, used by `RestoreFile` to locate the archived copy.

### 3.3 Resource Types by URI Scheme

| Scheme | Provider | Type Name | Key Metadata | Status |
|--------|----------|-----------|-------------|--------|
| `file://` | file, template, archive, encryption | `file.Resource` | Path, Inode, Device, Size, Mode, ModTime, Checksum | Implemented |
| `git://` | git | `git.Resource` | URL, Path, Ref | Implemented |
| `pkg://` | pkg | `pkg.Resource` | Name, Manager, Version | Implemented |
| `svc://` | service | `service.Resource` | Name | Implemented |
| `mem://` | (internal) | `Literal` | In-memory data (template payloads, JSON content) | Planned |

## 4. How Resource Flows Through the System Today

The file provider is the first provider to adopt resources. This section
documents the current state of the migration — what uses `Resource`, what
still uses raw strings, and how the two coexist.

### 4.1 Write Path: Copy

`Copy` (`provider.go:133`) demonstrates partial migration. The source is
already a `Resource`, but the destination is still a raw string:

```go
// pkg/op/provider/file/provider.go:133
func (p *Provider) Copy(sourceFile Resource, destinationFilename string,
    destinationFileMode os.FileMode) (result Resource, undo map[string]any, err error) {

    result, undo, err = p.prepareWrite(destinationFilename)
    if err != nil {
        return Resource{}, nil, err
    }
    // ...
    if _, err := sourceFile.WriteTo(f); err != nil {
        return result, nil, err
    }
    return result, undo, nil
}
```

The source `Resource` carries metadata from its origin node. The result
`Resource` is created by `prepareWrite` via `NewResource` and carries metadata
for the destination path after the write.

### 4.2 Write Path: WriteText/WriteBytes

The internal `write` method (`provider.go:855`) shows the full write pipeline:

```go
// pkg/op/provider/file/provider.go:855-897
func (p *Provider) write(path string, data []byte, mode os.FileMode) (
    result Resource, undo map[string]any, err error) {

    result, undo, err = p.prepareWrite(path)
    if err != nil {
        return result, nil, err
    }
    // ...
    hasher := sha256.New()
    mw := io.MultiWriter(f, hasher)
    var size int
    size, err = mw.Write(data)
    // ...
    err = result.RefreshMetadataWith(hex.EncodeToString(hasher.Sum(nil)), int64(size))
    return result, undo, err
}
```

The pattern is: `prepareWrite` (discovery + preemptive recovery) then
`io.MultiWriter` (write + hash in one pass) then `RefreshMetadataWith`
(update resource metadata with known checksum and size, re-stat for
kernel-assigned Inode/Device).

### 4.3 Read Path

`ReadText` and `ReadBytes` (`provider.go:714-746`) return file contents as a string or byte
slice respectively. Both delegate to a shared `read` helper that uses `Resource.WriteTo` to
stream the file into a buffer:

```go
// pkg/op/provider/file/provider.go:714-746
func (p *Provider) ReadBytes(resource Resource) (result []byte, err error) {
    buffer, err := p.read(resource)
    if err != nil {
        return nil, err
    }
    return buffer.Bytes(), nil
}

func (p *Provider) ReadText(resource Resource) (result string, err error) {
    buffer, err := p.read(resource)
    if err != nil {
        return "", err
    }
    return buffer.String(), nil
}
```

`Exists` (`provider.go:646`) already takes a `Resource` — the first fully
migrated read method:

```go
// pkg/op/provider/file/provider.go:646-649
func (p *Provider) Exists(blob Resource) bool {
    _, err := os.Lstat(blob.SourcePath)
    return err == nil
}
```

### 4.4 Migration Status

Every file provider method and its current migration state:

| Method | Action Type | Current Input | Current Output | Status |
|--------|-------------|--------------|----------------|--------|
| `Copy` | Compensable | `Resource` + `string` + `FileMode` | `Resource` | Partial |
| `WriteText` | Compensable | `string` + `string` + `FileMode` | `Resource` | Partial |
| `WriteBytes` | Compensable | `string` + `string` + `FileMode` | `Resource` | Partial |
| `ReadBytes` | Fallible | `Resource` | `[]byte` | Migrated |
| `ReadText` | Fallible | `Resource` | `string` | Migrated |
| `Exists` | Fallible | `Resource` | `bool` | Migrated |
| `IsDir` | Fallible | `Resource` | `bool` | Migrated |
| `IsFile` | Fallible | `Resource` | `bool` | Migrated |
| `Glob` | Fallible | `string` + `bool` | `[]string` | Not started |
| `Mkdir` | Fallible | `string` + `FileMode` | `Resource` | Not started |
| `Link` | Compensable | `string` + `string` | `string` | Not started |
| `Move` | Compensable | `string` + `string` | `string` | Not started |
| `Backup` | Compensable | `string` + `string` | `string` | Not started |
| `Remove` | Compensable | `string` + `bool` + `string` | `Tombstone` | Not started |
| `RemoveAll` | Compensable | `string` + `bool` + `string` | `Tombstone` | Not started |
| `Unlink` | Compensable | `string` + `bool` + `string` | `Tombstone` | Not started |
| `Name` | Pure | `string` | `string` | N/A |
| `Parent` | Pure | `string` | `string` | N/A |
| `Join` | Pure | `...string` | `string` | N/A |
| `WalkTree` | (flagged) | `string` + `Reducer` + `bool` | `any` + `*RecoveryStack` | Not started |

"Partial" means the output is `Resource` but some inputs remain raw strings.
"Not started" means both inputs and outputs use raw types.
"N/A" means the method takes no resource-typed parameters (pure computation).

### 4.5 Constructor Registry and Coercion

The constructor registry (`pkg/op/starvalue_marshal.go`) enables automatic
type conversion from Starlark slot values to Go provider parameters. When a
Starlark script passes a string path to a method expecting `file.Resource`,
the reflection bridge coerces it automatically.

**Registration** (`pkg/op/starvalue_marshal.go`):

```go
func RegisterConstructor[T any](fn func(any) (T, error)) {
    t := reflect.TypeOf((*T)(nil)).Elem()
    constructorRegistry.Store(t, func(v any) (any, error) {
        return fn(v)
    })
}
```

**File Resource registration** (`pkg/op/provider/file/resource.go`):

```go
func init() {
    op.RegisterConstructor(func(v any) (Resource, error) {
        s, ok := v.(string)
        if !ok {
            return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
        }
        return NewResource(s)
    })
}
```

**Coercion chain** — When the reflection bridge in `action_reflect.go`
encounters a slot value that doesn't directly match the target parameter type,
it walks through five coercion steps:

```
 1. nil           → zero value of target type
 2. Assignable    → assign directly (exact type match)
 3. Convertible   → convert (int → os.FileMode, int → int64, etc.)
 4. Map → struct  → coerce map[string]any to struct fields recursively
 5. Constructor   → call registered constructor (string → file.Resource)
 6. Error         → "cannot coerce %T to %s"
```

Step 5 is where string-to-Resource conversion happens today. The coercion is
transparent to Starlark scripts and provider code — Starlark passes a string
path, the constructor calls `NewResource` (which does `os.Stat`), and the
provider method receives a fully populated `file.Resource`.

**Target architecture — planner coerces, executor resolves:**

The constructor registry evolves into a coercion table with two kinds of
entries:

- **Plan-time coercions** (pure, no I/O): `string → file.Resource{URI
  only}`. Called by `buildPlannedBridge` when a Starlark string is passed
  to a slot expecting `file.Resource`. Creates a typed but unresolved
  Resource — URI and type set, metadata empty. The planner is pure because
  a graph is planned once and executed on many machines; `os.Stat` at plan
  time gives the wrong machine's metadata.
- **Execution-time resolvers**: `file.Resource{unresolved} →
  file.Resource{resolved}`. Called by the executor's pre-flight pass
  against the target machine. These do I/O (os.Stat, service status
  check, package version query).

The coercion table also supports cross-provider conversions:
`(mem.Resource, file.Resource) → converter`. The provider never sees a
foreign resource type — coercion is the only place where cross-provider
type bridging happens.

## 5. The Recovery Model Today

### 5.1 Same-Partition Recovery

The file provider's recovery model is built on a single optimization:
`os.Rename` is O(1) when source and destination are on the same filesystem
partition (it updates directory entries without copying data).

Recovery is handled by `op.RecoverySite` (`pkg/op/recovery_site.go`), a
context-aware service holding `Context` (same pattern as `ProviderBase`).
It is instantiated by the executor and stored in `op.Context.RecoverySite`.
The RecoverySite places all recovery files under `.devlore/recovery/` within
the `op.Root` authority boundary (same partition guaranteed):

```go
// pkg/op/recovery_site.go (reduced)
func (s *RecoverySite) ArchiveFile(p Path) (string, error) {

    recoveryDir := s.ctx.Root.NewPath(".devlore/recovery")
    s.ctx.Root.MkdirAll(recoveryDir, 0o700)
    recoveryID := uuid.New().String()
    recoveryPath := s.ctx.Root.NewPath(".devlore/recovery/" + recoveryID)
    s.ctx.Root.Rename(p, recoveryPath) // O(1) — same partition
    return recoveryID, nil
}
```

Providers access the RecoverySite through the standard context path:

```go
recoveryID, err := p.Context().RecoverySite.ArchiveFile(resource.SourcePath)
```

### 5.2 Compensation via Tombstone

`RestoreFile` reverses the rename:

```go
// pkg/op/recovery_site.go (reduced)
func (s *RecoverySite) RestoreFile(original Path, recoveryID string) error {

    recoveryPath := s.ctx.Root.NewPath(".devlore/recovery/" + recoveryID)
    if _, err := s.ctx.Root.Lstat(recoveryPath); errors.Is(err, fs.ErrNotExist) {
        return fmt.Errorf("recovery source not found: %s", recoveryID)
    }
    parentDir := s.ctx.Root.NewPath(filepath.Dir(original.Rel()))
    s.ctx.Root.MkdirAll(parentDir, 0o755)
    return s.ctx.Root.Rename(recoveryPath, original)
}
```

Compensable file operations that archive to the recovery directory (Remove,
RemoveAll, Unlink, Copy, WriteText, WriteBytes) use the same pattern: extract
the `Tombstone` from the undo state, then call
`p.Context().RecoverySite.RestoreFile(resource.SourcePath, tombstone.RecoveryID)`.
Note: `Backup` and `Move` do NOT use RecoverySite — they perform direct renames
to peer locations, and their compensation uses `p.rename` directly.

### 5.3 The prepareWrite Pattern

`prepareWrite` combines discovery and preemptive archival into a single
operation. Every write method calls it before mutating the filesystem:

```go
// pkg/op/provider/file/provider.go (reduced)
func (p *Provider) prepareWrite(resource Resource) (result Resource, undo Tombstone, err error) {

    result = NewResource(resource.SourcePath)
    result.Resolve(p.Context().Root)        // Discovery (root-scoped via op.Root)

    if !result.Exists() {
        parentDir := p.Context().Root.NewPath(filepath.Dir(result.SourcePath.Rel()))
        p.Context().Root.MkdirAll(parentDir, 0o750)
        undo = Tombstone{TombstoneBase: op.NewTombstoneBase(&result)}
        return result, undo, nil            // New file — no RecoveryID
    }

    tombstone, _, err := p.Remove(result, false, Resource{})
    return result, tombstone, err           // Existing file — tombstone with RecoveryID
}
```

Two branches:
- **New file**: Creates a tombstone with no `RecoveryPath`. Resource identity
  is the destination. Compensation deletes the newly created file.
- **Existing file**: Moves the existing file to recovery via `Remove`, creating
  a tombstone with `RecoveryPath`. Compensation removes the new file and
  restores from `RecoveryPath` back to `Resource.SourcePath`.

### 5.4 Gap

Only the file provider has tombstone recovery. The service, package, git, net,
and archive providers have compensation pairs (e.g., `CompensateInstall` calls
`Remove`), but none have a pre-flight discovery + backup mechanism equivalent
to `prepareWrite`. A failed `service.Start` compensates by calling `Stop`, but
there is no "snapshot the previous state" step.

## 6. Target Architecture

This section describes what the system will look like when the resource
management plan is fully implemented.

### 6.1 ResourceCatalog

The `ResourceCatalog` (`pkg/op/resource_catalog.go`) is a graph-level
compositor that owns both the append-only ledger and the URI→ID namespace.
One `ResourceCatalog` per Graph. The ledger is the plan-time skeleton —
URIs and relationships, but no target-machine-specific metadata. Metadata
is populated per-execution by the executor's pre-flight resolution pass.

```
ResourceCatalog
├── entries: []Resource         ← append-only ledger (interface values)
├── byID:    map[string]int     ← id → index
├── ns:      map[string]string  ← URI → current id (namespace)
├── nextID:  int                ← monotonic counter
└── methods:
    ├── Resolve(r Resource) → (Resource, id)            ← discover or return canonical
    ├── Shadow(r Resource, originID) → (id, error)      ← new version, update namespace, detect conflicts
    ├── Transition(resolved Resource, originID) → error ← in-place pending → resolved
    ├── Lookup(id) → (Resource, bool)
    ├── Current(uri) → id
    ├── DiscoveryURIs() → []string                       ← preflight input set
    └── Len() → int
```

The catalog stores `Resource` interface values, enabling polymorphic
access to actual typed resources (e.g., `file.Resource`). It deduplicates
by URI — if 5 nodes reference the same source path, `Resolve` returns
the same canonical entry. Pre-flight resolution stats each unique URI once.

**Typed resources in, typed resources out.** Both `Resolve` and `Shadow`
take a caller-constructed `Resource` rather than a bare URI string. The
caller coerces its input (a string path, a package identifier, a URL)
into a typed resource via the resource type's registered constructor
*before* reaching the catalog. The catalog never fabricates a concrete
resource type itself — the concrete type always flows in from the
caller. This matters because `entries` holds interface values, which
require a concrete type to exist. A catalog that took only URIs would
have no way to discover a new entry of the right concrete type.

**Plan-time vs execution-time ownership:**

| Concern | Owner | When |
| --- | --- | --- |
| URIs and relationships | Planner (ResourceCatalog) | Plan time |
| Implicit edges via shadowing | Planner | Plan time |
| Resource state (Pending/Active/Gone) | ResourceCatalog | Both — catalog owns transitions |
| Metadata (inode, size, checksum) | Executor pre-flight + node execution | Execution time, per target |
| State transitions on success/failure | ResourceCatalog (wraps r.Resolve) | Execution time |

### 6.2 Catalog Operations

The catalog mediates two provider-side operations: **DiscoverResource**
(observation, no production claim) and **NewResource** (production —
producer creates the underlying resource and registers it). Both go
through the catalog's internal `Discover` / `GetOrCreate` methods, which
in turn read or update the namespace and append entries to the ledger.

The catalog owns state transitions. Provider `Resolve()` returns
success or failure; the catalog applies the resulting state change to
the entry. The eight rules below define the full behavior matrix.

#### State machine

```
            ┌────────────┐
            │  Pending   │  ◀── initial state on insert
            └────────────┘
              │        │
   Resolve OK │        │ Resolve fails
              ▼        ▼
       ┌──────────┐ ┌────────┐
       │  Active  │ │  Gone  │
       └──────────┘ └────────┘
              │        ▲
              └────────┘
       (any later Resolve failure
        on an Active entry → Gone)
```

`Gone` is reactive — set whenever an operation that touches the
underlying state via `r.Resolve()` fails. `Active` is reached on either
a successful observation (discovery path) or fresh creation (production
path). No path transitions out of `Gone` automatically; reviving a
Gone URI requires a subsequent `NewResource` call (Rule 7).

#### Catalog behavior matrix

| Op | Cache state | `r.Resolve()` | Content-addressable | Location-based |
|---|---|---|---|---|
| `DiscoverResource` | miss | success | append `Pending` → `Active`; no `producerID` | same |
| `DiscoverResource` | miss | failure | append `Pending` → `Gone`; return error | same |
| `DiscoverResource` | hit, `Pending` | success | in-place `Pending` → `Active`; discard input | same |
| `DiscoverResource` | hit, `Pending` | failure | in-place `Pending` → `Gone`; return error | same |
| `DiscoverResource` | hit, `Active` | (not called) | return existing | same |
| `DiscoverResource` | hit, `Gone` | (not called) | return error | same |
| `DiscoverResource` | (any) | (any) | **never shadows** | **never shadows** |
| `NewResource` | miss | (not called) | append `Pending` → `Active`; stamp `producerID` | same |
| `NewResource` | hit, `Pending`/`Active` | (not called) | return existing (singleton); no shadow, no state change, `producerID` preserved | **shadow** with new entry born `Pending` → `Active`, stamp `producerID`; old entry stays in ledger |
| `NewResource` | hit, `Gone` | (not called) | **shadow** with new entry born `Pending` → `Active` (revives the URI); old Gone entry stays in ledger as history | same — **shadow** with new entry; Gone is terminal for the existing entry regardless of addressing |

**Where addressing matters:** only `NewResource` cache hits on
`Pending` / `Active`. `DiscoverResource` is shape-agnostic across
addressing types because it never shadows. `NewResource` on cache
miss is also identical because both addressing types append once.
`NewResource` on `Gone` hit is the same for both — shadow with a new
entry — because `Gone` is terminal regardless of addressing.

**Where state matters:** `DiscoverResource` branches on cache state to
decide whether to call `r.Resolve()` and how to apply the result.
`NewResource` reads state to decide: on `Pending`/`Active`, CAS returns
existing while location shadows; on `Gone`, both shadow (revive via
new entry).

**Gone is terminal.** No catalog operation transitions an entry out of
`Gone`. Reviving a Gone URI requires a subsequent `NewResource` call,
which appends a fresh entry via shadow rather than mutating the
existing Gone entry. The Gone entry stays in the ledger as history.

**Catalog owns transitions:** the state field on `op.ResourceBase` is
mutated by catalog code only. Resource providers' `r.Resolve()`
returns nil or an error; the catalog interprets the result and applies
the state change. Lifecycle management is centralized rather than
fragmented across every provider implementation.

#### Why this shape

The asymmetry between Rules 6 and 7 — return existing vs shadow —
matches the asymmetry already established by `Resolve`'s addressing-
aware cascade (§6.1). Content-addressable URIs encode their identity
in the URI itself, so re-producing the same URI is provably the same
resource. Location-based URIs do not encode content, so a new
production at the same URI is genuinely a new version that downstream
consumers need to see.

The Gone state is recorded rather than suppressed because it carries
useful information: "we expected this URI to exist and it doesn't"
becomes input to compensation, recovery, and reconciliation paths. A
caller seeing a Gone entry knows the catalog has already verified the
miss; no redundant probe is needed.

### 6.3 Shadowing Walkthrough

Step-by-step catalog state for a write-then-read of the same path:

```
Initial state:
  catalog.entries = []
  catalog.ns = {}

Step 1: plan.file.write_text(destinationPath="/etc/foo", content="v2", mode=0o644)
  ├─ Node A created
  ├─ Planned companion: pending = WriteTextPlanned(...) → *file.Resource{uri:"file:///etc/foo"}
  ├─ catalog.Shadow(pending, nodeA.ID) → (id:"res-1", nil)
  │   ├─ entries = [*file.Resource{uri:"file:///etc/foo", id:"res-1", originID:"A"}]
  │   └─ ns = {"file:///etc/foo" → "res-1"}

Step 2: plan.file.read(path="/etc/foo")
  ├─ Node B created
  ├─ Coerce "/etc/foo" → *file.Resource{uri:"file:///etc/foo"} (fresh, no origin)
  ├─ catalog.Resolve(fresh) → (canonical, "res-1")
  │   └─ canonical is the entry from Step 1; fresh is discarded
  ├─ Node B reads canonical.originID = "A" → implicit edge Node A → Node B
  └─ Executor guarantees: A runs before B

Execution time (Node A runs, producing a real file with populated metadata):
  ├─ action.Do(...) → resolved *file.Resource{uri:"file:///etc/foo", Inode:..., Size:..., Checksum:...}
  ├─ catalog.Transition(resolved, "A") → nil
  │   ├─ locates the pending entry by URI
  │   ├─ verifies origin matches
  │   └─ copies Inode / Size / Checksum / … onto the existing entry in place,
  │       preserving id="res-1" and originID="A"
  └─ Node B (and all other holders of the pending pointer) now sees populated metadata

Result: write happens before read. No explicit Output passing needed. The
catalog's pending entry survived from plan time into execution time and
transitioned to resolved in place — the same object, now with metadata.
```

### 6.4 Planning Data Flow (Pure — No I/O)

The planner is pure. It coerces strings to typed Resources (URI only, no
metadata) and builds the ledger skeleton. No `os.Stat`, no filesystem
access. This is required because a graph is planned once and executed on
many machines.

```
Starlark call: plan.file.write_text(destinationPath="/etc/foo", content="v2", mode=0o644)
    │
    ▼
planner.dispatch("file.write_text", args)
    ├─ createNode("file.write_text")
    ├─ fillSlots(node, {destinationPath:"/etc/foo", content, mode})
    ├─ method.HasPlanned? yes — WriteTextPlanned
    │   ├─ pending := WriteTextPlanned(destinationPath, content, mode)
    │   │   └─ *file.Resource{uri:"file:///etc/foo"} (pure, no I/O)
    │   └─ catalog.Shadow(pending, node.ID) → (id, nil)
    └─ return Promise(node.ID)

Starlark call: plan.file.read(resource="/etc/foo")
    │
    ▼
planner.dispatch("file.read", args)
    ├─ Resource-typed slot: coerce "/etc/foo" → *file.Resource (fresh, type-tag only)
    ├─ canonical, _ := catalog.Resolve(fresh)
    │   └─ canonical is the pending entry from write_text's Shadow;
    │      its originID is the write node
    ├─ createNode("file.read"); implicit edge write → read via canonical.originID
    ├─ fillSlots(node, {resource: canonical})
    └─ return Promise(node.ID)
```

### 6.5 Execution Data Flow (Per Target Machine)

The executor resolves the plan-time skeleton against a specific target
machine. Pre-flight resolution populates metadata for unresolved entries.
Node execution populates metadata for pending entries.

```
Executor.Run(ctx, graph)  — on target machine
    │
    ├─ Pre-flight: resolution pass over catalog.DiscoveryURIs()
    │   ├─ For each discovery URI (entry with originID == "", metadata empty):
    │   │   ├─ Lookup the typed entry, call its Resolve() — os.Stat / version
    │   │   │   query / etc. — populating metadata in place
    │   │   └─ Fail-fast if source does not exist
    │   └─ Pending entries skipped (they have non-empty originID; their
    │       producer will create them at execution time)
    │
    ├─ Pre-flight: tombstone scan
    │   ├─ For each resource slot with originID:
    │   │   ├─ URI occupied by different resource? → create tombstone
    │   │   └─ URI unoccupied? → no action
    │   └─ Inject physical state into slots
    │
    ├─ For each node (topological order):
    │   ├─ action.Do(ctx, resolvedSlots) → result, complement, error
    │   ├─ Post-dispatch: catalog reconciliation
    │   │   ├─ If result is a Resource (and not KnownAtExecution):
    │   │   │   ├─ catalog has a pending entry for this URI owned by this node?
    │   │   │   │   └─ catalog.Transition(result, node.ID)
    │   │   │   │       → in-place metadata copy onto pending entry, preserving id/originID
    │   │   │   └─ otherwise (monadic / late shadow):
    │   │   │       └─ catalog.Shadow(result, node.ID)
    │   │   │           → new ledger entry, namespace points to it
    │   │   └─ Shadow/Transition failure → push action to recovery stack,
    │   │       then fail the node (side effect already happened)
    │   └─ Push recovery entry onto stack
    │
    └─ Same graph can be executed again on a different target
```

### 6.6 Platform Provider — Data Provider

The platform provider (`pkg/op/provider/platform/`) is a data provider, not
an action provider. It is the Starlark surface for `op.Context.Platform` —
no independent state, no side effects, no compensation pairs.

Access type is `both`:

- **Immediate** — `platform.distro` returns a string from the local
  machine's Platform. Useful for single-machine local plans.
- **Planned** — `plan.platform.distro` returns a promise (`Output`) that
  the executor resolves against the target machine's Platform at execution
  time. This is the mechanism by which a single graph targets many machines.

```python
# Immediate: branch on local machine's distro (single-target plans)
if platform.distro == "Debian":
    plan.pkg.install(packages=["nginx"])

# Planned: branch at execution time (multi-target graphs)
distro = plan.platform.distro
plan.choose(
    plan.pkg.install(packages=["nginx"]),  # default
    plan.case(when=distro == "Debian", then=plan.pkg.install(packages=["nginx"])),
    plan.case(when=distro == "Fedora", then=plan.pkg.install(packages=["nginx"])),
)
```

The executor populates `op.Context.Platform` before running any node. For
remote targets, a different `*op.Platform` is constructed with the remote
machine's OS, Arch, Distro, and its package/service manager implementations.
The planned projection's promises resolve against whichever Platform the
executor provides — the graph itself is target-agnostic.

Because the provider is read-only, it requires no codegen (no actions, no
params, no compensation). It will evolve as the Starlark receiver surface
takes shape.

### 6.7 Provider Lifecycle and Context Injection

Providers are singletons. In a graph, a provider follows the lifetime of
the graph. In a Starlark script, a provider follows the lifetime of the
script. Every provider needs context — and context is provided at
construction time. Every provider needs a constructor that accepts a
context object by reference.

**Current state** — Every provider embeds `op.ProviderBase` and receives
context at construction time. The codegen enforces this as a hard
requirement — `generate.star` fails if ProviderBase is not embedded.

In **immediate mode**, the generated `ImmediateFactory` creates a provider
with a partial `op.Context` populated from `BindingConfig`:

```go
ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
    return NewPkgReceiver(&provider.Provider{
        ProviderBase: op.NewProviderBase(op.Context{
            Writer:   cfg.Writer,
            Platform: cfg.Platform,
        }),
    })
}
```

In **action/graph mode**, the `ActionRegistrar` creates a provider with
the full execution `op.Context`:

```go
ActionRegistrar: func(reg *op.ActionRegistry, ctx op.Context) {
    p := &provider.Provider{
        ProviderBase: op.NewProviderBase(ctx),
    }
    provider.RegisterReflectedActions(reg, p)
}
```

The provider reads `p.Context().Writer`, `p.Context().Platform`, etc.
for the duration of the graph or script. No per-method context parameters.

### 6.8 Output Specs

For the catalog to be built at plan time, the planner must know the
identity of every resource each node will produce — **before** the node
runs. The types of a method's input parameters tell the planner which
parameters are resources (inputs), but not which input parameter becomes
the output URI. Name-based heuristics ("last resource param is the
output") are fragile — they handle the `Copy(source, destination)` case
but fail for `Remove(path, prune, boundary)` and similar shapes.

The solution is a **declared output** pattern, borrowed from Bazel and
Terraform: every resource-producing method declares a pure function that
computes its output resource identity from the method's input slot
values. This is an **output spec**. The framework calls it at plan time;
the forward method calls it at execution time. One source of truth.

This matches established prior art:

- **Bazel** — rule implementations call `ctx.actions.declare_file(name)`
  during the analysis phase (before execution) to pre-declare output
  files. Output filenames are computed from attrs as pure Starlark.
- **Terraform** — providers implement `PlanResourceChange`. Attributes
  knowable at plan time are computed; unknown attributes are marked
  "known after apply". The provider surfaces identity upfront where
  possible and uses a sentinel for dynamic cases.
- **Nix derivations** — every derivation has a deterministic output path
  computed from inputs before the build runs. Identity is a pure function
  of inputs.
- **Mokhov, Mitchell, Peyton Jones (ICFP 2018), "Build Systems à la
  Carte"** — formalizes the distinction between *applicative* tasks
  (outputs knowable at plan time) and *monadic* tasks (outputs depend on
  runtime values).

Our output spec is applicative by default with a monadic escape hatch via
an unknown sentinel.

#### OutputSpec type

```go
// OutputSpec computes the identity of a resource that a method will
// produce, from the method's input slot values. Pure: no I/O, no
// target-machine state, no mutation. Called at plan time.
//
// The returned Resource has identity (URI) set but metadata empty —
// equivalent to a pending-state resource. The catalog stamps id and
// originID when it shadows the spec's output.
//
// Returns (KnownAtExecution, nil) when the output identity depends on
// runtime values (e.g., target machine's package manager). The planner
// skips plan-time shadowing for such outputs; the executor shadows the
// real return value post-dispatch.
type OutputSpec func(slots map[string]any) (Resource, error)

// KnownAtExecution is the sentinel that an output spec returns when
// the output identity cannot be determined at plan time but will be
// available after the forward method runs. Analogous to Terraform's
// "known after apply" marker.
//
// The name is temporal, not uncertain: the value is a legitimate
// resource identity that exists once the producing node has executed.
var KnownAtExecution Resource = knownAtExecution{}
```

#### Authoring convention

The output spec for a forward method is authored as a sibling Go method
with the same input signature (less the `error` return) and a single
resource return type. By naming convention, `Copy`'s output spec is
`CopyPlanned`, `WriteText`'s is `WriteTextPlanned`, `Clone`'s is
`ClonePlanned`.

The "`Planned`" suffix is deliberate: it names **what** the function
returns (the planned identity of the resource that the forward method
will produce) and **when** the function runs (the plan phase, pure, no
side effects). It pairs with the `KnownAtExecution` sentinel: the
planned identity is either knowable at plan time or explicitly deferred
until execution.

The sibling method is **pure**: it constructs the identity without
touching the filesystem, network, or package manager. It is authored by
the provider developer; the codegen discovers it by naming convention and
generates the `OutputSpec` closure that unmarshals slot values into the
sibling's parameter types.

Methods that produce no resource (or whose resource output is dynamic
and deferred) have no sibling. The codegen emits no output spec entry.
Such methods rely on execution-time shadowing.

#### Companion triplet

Every provider method that mutates external state is a member of a
**companion triplet**: the forward method, its planned output spec, and
its compensation companion. Not every triplet is complete — the shape
depends on the method's action kind.

| Member | Required when | Purity | Runs when |
|---|---|---|---|
| `X` (forward) | Always | Side-effecting | Execution phase, on the target machine |
| `XPlanned` (output spec) | `X` returns a resource | Pure — no I/O, no target state | Plan phase, plus invoked internally by `X` for identity construction |
| `CompensateX` (compensation) | `X` is compensable (returns `(T, U, error)`) | Side-effecting — restores prior state | Rollback phase, given the tombstone `X` returned |

Only `X` is registered as an action and exposed to starlark.
`XPlanned` and `CompensateX` are companions: the codegen emits them
into the provider's metadata (output spec map, compensation companion
pointer) but they do not appear as methods on the starlark receiver.
The naming convention is also what excludes them from the starlark
method enumeration.

**Source order convention.** Members of a triplet are placed adjacent
in the provider's source file in this order: forward, planned,
compensate.

```go
func (p *Provider) Copy(...)         { /* ... */ }
func (p *Provider) CopyPlanned(...)  { /* ... */ }
func (p *Provider) CompensateCopy(...) { /* ... */ }
```

This does not match alphabetical order (alphabetical puts
`CompensateCopy` first), but it matches the reading order a developer
wants when scanning the triplet: what the method does, what it produces,
how it rolls back. The codegen emits generated sections in the same
order. The style guide enforces the convention for hand-written
provider code.

**Static checks the codegen enforces:**

1. A method whose first non-error return is a resource type must have
   a `Planned` sibling.
2. A `Planned` sibling whose corresponding forward method's first
   non-error return is NOT a resource is a codegen error.
3. A `Planned` sibling's input signature must match its forward
   method's input signature exactly.
4. A `Planned` sibling's return type must match its forward method's
   first non-error return type.
5. A compensable method must have a `Compensate*` companion (already
   enforced by `NewMethod` today).
6. A `Compensate*` companion for a non-compensable method is a codegen
   error (already enforced).

Either the developer satisfies all six rules and the codegen generates
a working provider, or codegen fails with a specific message. There is
no runtime grey area.

#### The forward method uses its own output spec

The critical property: **the forward method calls its own output spec**
as the first step. Identity construction is factored out of the forward
method into the spec, so the developer writes it once and both the
planner and the forward method call the same function.

Example — `file.Copy`:

```go
// Copy writes source to destinationPath and returns the destination
// resource with its metadata populated.
func (p *Provider) Copy(source *Resource, destinationPath string, mode os.FileMode) (*Resource, Tombstone, error) {
    // 1. Identity construction — reuse the output spec so Copy and
    //    CopyPlanned agree on URI construction rules.
    dest, err := p.CopyPlanned(source, destinationPath, mode)
    if err != nil {
        return nil, Tombstone{}, err
    }

    // 2. Pre-write hook: archive any existing file at the destination.
    tombstone, err := p.prepareWrite(dest)
    if err != nil {
        return nil, Tombstone{}, err
    }

    // 3. Do the work.
    if err := p.copyContents(source.SourcePath, dest.SourcePath, mode); err != nil {
        return nil, tombstone, err
    }

    // 4. Post-write metadata refresh — transition pending to resolved.
    if err := dest.Resolve(); err != nil {
        return dest, tombstone, err
    }

    return dest, tombstone, nil
}

// CopyPlanned is the output spec for Copy. Pure: no I/O.
// Identity-only; metadata is zero-valued until Resolve().
func (p *Provider) CopyPlanned(source *Resource, destinationPath string, _ os.FileMode) (*Resource, error) {
    return NewResource(p.Context(), destinationPath)
}

// CompensateCopy undoes a Copy by restoring the original file from recovery.
func (p *Provider) CompensateCopy(undo Tombstone) error {
    return p.compensateWrite(undo)
}
```

Identity construction lives in one place. Bug fixes to URI rules
propagate automatically to both planning and execution. The framework
and the implementation cannot drift.

Source order is forward, planned, compensate — the companion-triplet
convention.

#### Benefits beyond planning

If the output spec existed only for the planner, it would be duplicated
work and developers would resent it. The sell is that the forward method
*uses* it, and several framework capabilities require it:

1. **Single source of identity construction.** One function owns URI
   rules; planning and execution share it.
2. **Unit testability.** Output specs are pure and trivially testable
   without filesystem or network setup.
3. **Dry-run support for free.** In dry-run mode, the executor calls
   the output spec instead of the forward method. Dry-run reports what
   would be produced without running side effects. No per-method dry-run
   branch.
4. **Receipt hydration.** When a graph is loaded from disk (receipt
   replay, cross-machine execution), slot values come back as raw data.
   `Rebind` walks nodes and calls each method's output spec to
   reconstruct typed output resources from the recorded input slots.
   The catalog rebuilds itself from pure data — no stored resource
   objects to deserialize.
5. **Plan-time conflict detection.** Before any node executes, the
   planner has shadowed every known-at-plan output. Two nodes writing
   to the same URI collide immediately with a clear error. No silent
   race.
6. **Implicit edges via URI matching.** A downstream node that reads
   `/etc/foo` calls `catalog.Resolve` with the typed file resource. The
   catalog has a shadowed entry from an upstream
   `plan.file.write_text(destinationPath="/etc/foo")`. Resolve returns
   the shadowed version; the planner adds an edge writer → reader.
7. **Compensation cleanup knows what to undo.** The output spec is the
   same function both the writer and the compensator rely on to
   identify the affected URI.
8. **Speculative: skip-if-unchanged.** Once identity is decoupled from
   side effects, preflight can compare the pending output's content
   hash (if the spec computes one) against the existing file and mark
   the node completed without running. Deferred — not required for
   correctness.

#### Planning with output specs

```
Starlark call: plan.file.copy(source=src, destinationPath="/etc/foo", mode=0o644)
    │
    ▼
planner.dispatch("file.copy", args)
    ├─ Unpack input slots:
    │   └─ source (*file.Resource), destinationPath (string), mode (os.FileMode)
    ├─ Coerce resource-typed inputs via the registered constructor
    │   └─ source → *file.Resource; routed through catalog.Resolve as a discovery
    ├─ Create the node and fill input slots with typed values
    ├─ method.HasPlanned()?  Yes — CopyPlanned auto-discovered on the
    │                        provider type by methodFromReflectedMethod,
    │                        symmetric with Compensate<Name>.
    │   ├─ receiver := rt.Construct()(ctx)
    │   ├─ pending := method.Plan(receiver, args)
    │   │   └─ invokes CopyPlanned via reflection with the filled slot values
    │   │     → *file.Resource{uri:"file:///etc/foo"} (pure, no I/O)
    │   ├─ catalog.Shadow(pending, node.ID) → (id, nil)
    │   │   └─ stamps id and originID on pending's ResourceBase in place
    │   └─ pending now lives in the ledger, addressable by URI
    ├─ Return Promise(node.ID)
```

Subsequent `plan.file.read_text(resource="/etc/foo")` at plan time:

```
planner.dispatch("file.read_text", args)
    ├─ Coerce resource to *file.Resource
    ├─ catalog.Resolve(res) → canonical (the pending entry from Copy!)
    ├─ canonical has originID set → planner creates implicit edge
    │   from Copy node to ReadText node
    └─ Fill slot with canonical resource
```

The implicit edge is the payoff. The developer didn't wire it
explicitly. The catalog did it because both nodes referenced the same
URI and the output spec put it there at plan time.

#### Execution with output specs

```
Executor.Run(ctx, graph)
    │
    ├─ Preflight: resolve discovery entries against target
    │   (pending entries skipped because they have non-empty originID)
    │
    ├─ For each node in execution order:
    │   ├─ Resolve slot values (pending entries already in slots)
    │   ├─ Call method.Do(ctx, slots) → result, complement, error
    │   ├─ Post-dispatch: if result is a Resource (and not KnownAtExecution):
    │   │   ├─ catalog has a pending entry for result.URI() owned by this node?
    │   │   │   → catalog.Transition(result, node.ID)
    │   │   │     in-place metadata copy onto the pending entry,
    │   │   │     preserving catalog id and originID
    │   │   └─ otherwise (monadic case — Planned returned KnownAtExecution,
    │   │      or no Planned companion exists):
    │   │      → catalog.Shadow(result, node.ID)
    │   │        (late shadow — first time the URI is seen)
    │   └─ Push recovery entry, continue.
```

#### Monadic outputs (unknown at plan time)

Some methods cannot compute their output identity at plan time because
it depends on runtime values. The canonical example is `pkg.Install`
on a cross-platform graph: the installed package's URI depends on which
package manager the target machine has (`pkg:apt/foo` vs
`pkg:brew/foo`), and the planner doesn't know the target.

These methods have an output spec that returns `KnownAtExecution`:

```go
func (p *Provider) InstallPlanned(name string, _ string, _ bool) (*Resource, error) {
    return op.KnownAtExecution, nil
}
```

The planner sees `KnownAtExecution` and skips plan-time shadowing. The
executor shadows the real result after dispatch. Implicit edges via
URI matching don't work for these outputs at plan time, but explicit
promise passing still does. Plan-time conflict detection is skipped
for these outputs too — the system does not know what URI they will
claim.

The phrasing mirrors Terraform's user-facing
`(known after apply)` marker and its underlying semantics: the value
is a legitimate resource identity that exists once the producing node
has run, just not before. It is a temporal statement, not an
uncertainty statement.

In the build-systems literature (Mokhov, Mitchell, Peyton Jones, ICFP
2018, "Build Systems à la Carte") this is the applicative-vs-monadic
split: applicative tasks have outputs knowable at plan time; monadic
tasks have outputs that depend on runtime values. Our default is
applicative; `KnownAtExecution` is the exit.

#### Codegen

The codegen scans provider source for method pairs `X`/`XPlanned` where
the signature of `XPlanned` matches `X`'s input signature (modulo the
error return and with a single resource return). For each pair it
emits an entry in the provider's registration:

```go
op.AnnounceProvider(
    reflect.TypeFor[provider.Provider](),
    op.RoleAction,
    func(ctx *op.ExecutionContext) (any, error) { return provider.NewProvider(ctx), nil },
    map[string][]string{
        "Copy": {"source_file", "destination_filename", "destination_file_mode"},
        // ...
    },
    map[string]op.OutputSpec{
        "Copy": func(slots map[string]any) (op.Resource, error) {
            p := mustProvider()  // cached provider instance
            return p.CopyPlanned(
                slots["source_file"].(*Resource),
                slots["destination_filename"].(string),
                slots["destination_file_mode"].(os.FileMode),
            )
        },
        // ...
    },
)
```

No annotation language. No struct tags. The naming convention is the
only authoring contract.

#### Summary

| Aspect | Before (heuristics) | After (output specs) |
|---|---|---|
| Output parameter identification | Name matching (fragile) | Sibling method with typed inputs (deterministic) |
| Identity construction | Duplicated in forward method and planner | Single function called by both |
| Plan-time shadowing | Heuristic-gated, incorrect for Remove-family | Explicit, type-safe, authored per method |
| Dry-run | Separate code path per method | Call the output spec instead of the forward method |
| Receipt hydration | Not supported | Output spec reconstructs pending resources from slots |
| Runtime-dependent outputs | Undefined behavior | `KnownAtExecution` sentinel, execution-time shadowing |
| Prior art | None | Bazel declared outputs, Terraform planned state |

### 6.9 Comparison to Bazel Declared Outputs

Our closest analogue is Bazel's analysis-phase output declaration. The
pattern is similar in intent — "describe what a rule will produce
before running it" — but diverges in authoring syntax, call timing,
and reuse.

#### Side-by-side: a compile-like rule

**Bazel rule (Starlark):**

```python
def _my_compile_impl(ctx):
    # Analysis phase. Pure Starlark, no subprocesses yet.
    # Declare the output file at a computed path:
    out = ctx.actions.declare_file(ctx.label.name + ".o")

    # Register an action that will populate `out` at execution time:
    ctx.actions.run(
        executable = ctx.executable._compiler,
        inputs = [ctx.file.src],
        outputs = [out],                 # <-- framework now knows this path
        arguments = [ctx.file.src.path, out.path],
    )

    return [DefaultInfo(files = depset([out]))]

my_compile = rule(
    implementation = _my_compile_impl,
    attrs = {
        "src": attr.label(allow_single_file = True),
        "_compiler": attr.label(executable = True, cfg = "exec",
                                default = Label("//tools:compiler")),
    },
)
```

**Our equivalent (Go):**

```go
// Compile is the forward method. Calls its own output spec to
// construct the result, then does the work.
func (p *Provider) Compile(src *file.Resource) (*file.Resource, Tombstone, error) {
    out, err := p.CompilePlanned(src)
    if err != nil {
        return nil, Tombstone{}, err
    }

    tombstone, err := p.prepareWrite(out)
    if err != nil {
        return nil, Tombstone{}, err
    }

    if err := exec.Command(compilerPath, src.SourcePath.Abs(), out.SourcePath.Abs()).Run(); err != nil {
        return nil, tombstone, err
    }

    if err := out.Resolve(); err != nil {
        return out, tombstone, err
    }
    return out, tombstone, nil
}

// CompilePlanned is the output spec for Compile. Pure: no I/O.
func (p *Provider) CompilePlanned(src *file.Resource) (*file.Resource, error) {
    outPath := strings.TrimSuffix(src.SourcePath.Rel(),
        filepath.Ext(src.SourcePath.Rel())) + ".o"
    return file.NewResource(p.Context(), outPath)
}

// CompensateCompile removes the compiled output on rollback.
func (p *Provider) CompensateCompile(undo Tombstone) error {
    return p.compensateWrite(undo)
}
```

Source order: forward, planned, compensate.

#### Structural comparison

| Concern | Bazel | Ours |
|---|---|---|
| **Phase separation** | Hard. Analysis (Starlark) and execution (subprocess) are distinct phases. `_impl` runs in analysis; declared files are populated later by actions. | Soft. Output spec is a sibling Go method; forward method calls it synchronously before doing work. Framework can call the spec independently without ever running the forward method. |
| **Authoring location** | `ctx.actions.declare_file(name)` inline in the rule impl. Output naming is a string expression over `ctx.label.name` / `ctx.attr.*`. | Sibling method `XPlanned` next to forward method `X`. Input signature is identical to `X` so the compiler type-checks the connection. |
| **Language** | Starlark — interpreted, dynamic-typed, no compiler checks that the declared output matches what the action produces. | Go — static typing. `CompilePlanned`'s return type is the same resource type that `Compile` returns. Codegen enforces the signature pairing. |
| **Discovery mechanism** | Bazel inspects the rule's return value (providers) plus the actions registered via `ctx.actions.*`. `outputs` are tracked in the action graph. | Codegen scans provider source for the `X` + `XPlanned` method pair by naming convention. Emits an entry in the generated `provider.gen.go`. |
| **Separation of identity and work** | Explicit: `declare_file` creates the `File` handle; `ctx.actions.run` registers the action that populates it. The action doesn't own identity. | Explicit: `XPlanned` creates the resource identity; `X` populates metadata after running the work. The forward method calls the spec internally so both agree. |
| **Reuse between framework and action** | Framework uses `File.path` to wire downstream consumers. The action's subprocess is opaque to Bazel — it just reads the declared path. | Framework calls `CompilePlanned` at plan time to populate the catalog. The forward method calls `CompilePlanned` at execution time to construct the result. Same function, same rules. |
| **Dynamism (deferred outputs)** | **Tree artifacts** (`declare_directory`): output is a directory whose contents are discovered at execution time. Plus experimental **dynamic dependencies** where an action can discover additional inputs/outputs mid-execution. | `KnownAtExecution` sentinel returned by the output spec. The planner skips plan-time shadowing; the executor shadows the real return value post-dispatch. Phrasing and semantics borrowed from Terraform's `(known after apply)`. |
| **Conflict detection** | At analysis time. Two rules declaring the same output file fail the build immediately. | At plan time via `catalog.Shadow`. Two nodes shadowing the same URI return a conflict error from the planner. |
| **Implicit edges via shared identity** | Not really. Bazel uses explicit `inputs`/`outputs` lists on every action. Dependencies are declared, not discovered. | Yes. A reader calling `catalog.Resolve` on an already-shadowed URI gets the canonical entry with `originID` set. The planner creates the edge automatically. |
| **Pure-function guarantee** | Starlark analysis is enforced pure: no `os.stat`, no file reads. The Starlark interpreter literally rejects I/O during analysis. | By convention only — `XPlanned` is documented as pure, but Go doesn't enforce it. Code review and unit tests are the enforcement. Could be strengthened with a linter. |
| **Receipt / replay** | Bazel's action cache and remote execution replay actions by hashing their declared inputs and outputs. The declaration is what makes the hash deterministic. | `Rebind` at graph load walks nodes, calls each method's output spec with the recorded slot values, and reconstructs the catalog's pending entries. Same function, different caller. |

#### Where we diverge intentionally

**Bazel declares outputs via a framework call (`declare_file`) inside
the rule impl.** The rule impl is imperative — it can do arbitrary
computation to build the filename, but the act of declaring is explicit.
Our output spec is a separate pure function with a typed signature; the
"declaration" is the function itself, and the typed input signature is
what the codegen uses to wire the planner.

**Bazel's rules produce actions; our methods ARE the actions.** Bazel
has a rule impl (`_my_compile_impl`) that registers actions as a side
effect. We have a method (`Compile`) that *is* the side-effecting
operation. Bazel's analysis/execution split is reified in the data
(actions are data structures); ours is reified in the function (the
spec is a function, the forward method is a function, they share input
types).

**Bazel's analysis is Starlark-interpreted; ours is Go-compiled.** Our
static typing catches bugs Bazel would catch only at analysis runtime.
The tradeoff: Bazel rules are easier to author dynamically (string
manipulation, dict attrs), ours require named Go functions with
declared parameter types.

**Our output spec is callable by the forward method; Bazel's
`declare_file` is not reusable by the action.** The Bazel action is a
subprocess — it can't invoke `declare_file`, that's a Starlark-only
operation. Our sibling-method pattern lets the forward method reuse
identity construction, eliminating drift. This is the sell to the
provider developer: you write the identity logic once and the framework
uses it for free.

**Bazel has no direct equivalent to our implicit-edges-via-URI-matching.**
Bazel edges are declared explicitly in rule `inputs`. Our planner reads
the catalog and creates edges from shared URIs. The output spec is what
puts the URIs in the catalog at plan time so reads later can discover
them.

#### Terminology adopted

From Bazel: "**declared output**" (the catalog-level concept), "**plan
phase**" (our analog of Bazel's analysis phase, same pure-function
discipline).

From Terraform: "**known after apply**" semantics and the temporal
framing, adopted as `KnownAtExecution` for our sentinel — the output is
a legitimate resource identity that exists once the producing node has
run, not an uncertain value.

From Mokhov et al., "Build Systems à la Carte" (ICFP 2018): "**applicative
tasks**" (outputs knowable at plan time) and "**monadic tasks**" (outputs
depend on runtime values). Our default is applicative; `KnownAtExecution`
is the exit for genuinely monadic outputs.

## 7. Integration with Current Architecture

### 7.1 Relationship to Output/FillSlot

Resources enrich the existing slot model — they do not replace it. Nodes,
edges, slots, and actions remain unchanged. Resources add identity tracking
on top.

| Aspect | Current | With Resources |
|--------|---------|----------------|
| Slot values | `"/etc/foo"` (string) | `file.Resource{uri:"file:///etc/foo", id:"res-3"}` |
| Edge creation | Explicit `*Output` passing only | Also implicit via shadowing (`originID`) |
| Provider params | `path string` | `Resource` (extracts path from `SourcePath`) |
| Tombstone logic | Per-provider (`prepareWrite`) | Executor pre-flight, keyed by resource ID |
| Conflict detection | None | ResourceCatalog detects two nodes claiming same URI |

### 7.2 Relationship to Compensation Model

The phase execution model defines three layers: Definition, Activation, and
Recovery. Resources map cleanly to each:

| Layer | Current | With Resources |
|-------|---------|----------------|
| **Definition** (planning) | Nodes created with string slots | Nodes created with Resource slots; catalog tracks shadowing |
| **Activation** (execution) | Provider method called with slot values | Provider method called with Resource; metadata updated post-write |
| **Recovery** (compensation) | Shared `op.RecoverySite` (`ArchiveFile`/`RestoreFile`) | Executor-owned tombstone layer; shared recovery service |

The compensation pairs (`Forward`/`Compensate*`) are unchanged. What changes
is *who decides* when to create tombstones: the executor's pre-flight pass
instead of each provider's `prepareWrite`.

### 7.3 Action Type Model

Provider methods are classified into three action types based on their Go
return signature. All three share a unified `Do` interface:

```go
Do(ctx *Context, slots map[string]any) (Result, Complement, error)
```

The three types differ in how the reflected adapter normalizes the
provider method's actual return values into this unified signature:

| Action Type | Provider Return Signature | Normalization | Undo |
|-------------|--------------------------|---------------|------|
| **Action** (pure) | `(T)` or `()` | `(result, nil, nil)` | None — side-effect-free |
| **FallibleAction** | `(T, error)` | `(result, nil, err)` | None — fails cleanly |
| **CompensableAction** | `(T, U, error)` | `(result, complement, err)` | `Undo(ctx, complement) error` |

**Pure actions** — Methods like `file.Name`, `file.Parent`, `file.Join`
return a value with no error and no side effects. They are registered as
graph-mode `Action` nodes and participate in the execution graph like any
other action. In dry-run mode they log and return nil (consistent with
fallible and compensable actions). The reflected adapter
(`reflectedPureAction`) panics on coercion errors since these indicate a
framework bug, not a runtime failure.

**FallibleAction** — Methods like `file.Read`, `file.Exists`, `file.Glob`
perform I/O and can fail but have no compensation pair. They return
`(result, nil, err)` — the `Complement` is always nil.

**CompensableAction** — Methods like `file.WriteText`, `file.Remove` have
a `Compensate*` companion method. The reflected adapter extracts the
complement from the second return value and the executor pushes it onto
the recovery stack for LIFO unwinding on failure.

**Classification happens at registration time** in
`RegisterReflectedActions` (`action_reflect.go`). The codegen
(`generate.star`) includes all three types in the generated action tests.
There is no runtime type-switch dispatcher — each reflected type's `Do`
method handles its own normalization internally.

### 7.4 Relationship to Constructor Registry

The existing `RegisterConstructor`/`Construct`/`coerceSlotValue` chain
(`starvalue_marshal.go`, `action_reflect.go`) handles string-to-Resource
coercion at execution time. When a Starlark script passes `"/etc/foo"` to a
method expecting `file.Resource`, the constructor calls `NewResource` (which
does `os.Stat`) and returns a fully populated resource.

**Target architecture**: The constructor registry evolves into a coercion
table with two concerns:

1. **Plan-time coercion** (pure, no I/O) — `string → file.Resource{URI
   only}`. Called by `buildPlannedBridge` during planning. Creates typed
   but metadata-empty Resources. This is the entry point for namespace
   resolution — the coerced Resource is tracked in the ledger via
   `catalog.Resolve()` or `catalog.Shadow()`.

2. **Execution-time resolution** — `file.Resource{unresolved} →
   file.Resource{resolved}`. Called by the executor's pre-flight pass on
   the target machine. Populates metadata via `os.Stat`. This cannot
   happen at plan time because a graph is planned once and executed on
   many machines with different filesystem state.

The coercion table also supports cross-provider conversions:
`(mem.Resource, file.Resource) → converter`. Provider methods are
monomorphic in their resource types — they never see a foreign resource
type. The coercion table is the only place where cross-provider type
bridging happens.

## 8. Provider Signature Migration

### 8.1 File Provider (Reference Implementation)

Before/after for each method. Configuration parameters (mode, prune,
boundary, backupSuffix, honorGitignore) are unchanged.

| Method | Type | Before | After |
|--------|------|--------|-------|
| `Copy` | Compensable | `(Resource, string, FileMode)` → `Resource` | `(Resource, Resource, FileMode)` → `Resource` |
| `WriteText` | Compensable | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `WriteBytes` | Compensable | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `ReadBytes` | Fallible | — | `(Resource)` → `[]byte` |
| `ReadText` | Fallible | — | `(Resource)` → `string` |
| `Exists` | Fallible | `(Resource)` → `bool` | No change (already migrated) |
| `IsDir` | Fallible | `(Resource)` → `bool` | No change |
| `IsFile` | Fallible | `(Resource)` → `bool` | No change |
| `Glob` | Fallible | `(string, bool)` → `[]string` | No change |
| `Mkdir` | Fallible | `(string, FileMode)` → `Resource` | `(Resource, FileMode)` → `Resource` |
| `Link` | Compensable | `(string, string)` → `string` | `(Resource, Resource)` → `Resource` |
| `Move` | Compensable | `(string, string)` → `string` | `(Resource, Resource)` → `Resource` |
| `Backup` | Compensable | `(string, string)` → `string` | `(Resource, string)` → `Resource` |
| `Remove` | Compensable | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |
| `RemoveAll` | Compensable | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |
| `Unlink` | Compensable | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |
| `Name` | Pure | `(string)` → `string` | No change (no resource params) |
| `Parent` | Pure | `(string)` → `string` | No change (no resource params) |
| `Join` | Pure | `(...string)` → `string` | No change (no resource params) |

The pattern: every parameter that identifies an external entity (a path, a
URL, a package name) becomes a `Resource`. Parameters that are configuration
values (modes, flags, suffixes) remain unchanged. Pure methods take and
return primitive values — they have no resource parameters to migrate but
participate in the graph as `Action` nodes.

### 8.2 Other Providers

| Provider | Resource Params | Config Params (unchanged) |
|----------|----------------|---------------------------|
| file | paths → `Resource` | mode, prune, boundary, backupSuffix, honorGitignore |
| git | url + path → `Resource` | ref, branch |
| net | url → `Resource` | (none) |
| pkg | package names → `Resource` | manager, cask |
| service | service name → `Resource` | (none) |
| template | source + path → `Resource` | templateData, project |
| archive | source + prefix → `Resource` | (none) |
| encryption | source → `Resource` | decryptor |
| shell | (none — commands are strings, not external state) | command |

## 9. Executor Tombstone Unification

### 9.1 Current: Shared Recovery Site

Recovery logic lives in `op.RecoverySite` (`pkg/op/recovery_site.go`), a
context-aware service holding `Context` (same pattern as `ProviderBase`),
used by all providers through `op.Context.RecoverySite`. All I/O goes
through `Context.Root` (the `op.Root` interface):

- `ArchiveFile(p Path) → recoveryID` — moves file to UUID-keyed recovery via zero-copy rename
- `ArchiveData(data []byte) → recoveryID` — writes arbitrary bytes to recovery for non-file state
- `RestoreFile(original Path, recoveryID string)` — renames recovery copy back to original path
- `RestoreData(recoveryID string) → []byte` — reads bytes back from recovery

The file provider uses `ArchiveFile`/`RestoreFile` for all compensable
operations. `prepareWrite` combines discovery + preemptive archival.

### 9.2 Target: Executor-Owned Tombstone Layer

The executor's pre-flight binding pass scans the resource ledger for shadowed
URIs (a resource with `originID` shadows a discovered resource at the same
URI). For each shadowed URI, it creates a tombstone before any node executes.

The target tombstone type lives in `pkg/op/tombstone.go` as a peer of
`resource.go`:

```
pkg/op/
├── resource.go          ← Resource interface, ResourceBase (identity)
├── resource_catalog.go  ← ResourceCatalog (ledger + namespace)
├── tombstone.go         ← ResourceTombstone (recovery) [planned]
└── ...
```

The `ResourceTombstone` type generalizes the file provider's `Tombstone`:

```
ResourceTombstone
├── ResourceID    string   ← which resource was shadowed
├── URI           string   ← logical address
├── RecoveryID    string   ← opaque ID from RecoverySite (filesystem resources)
├── RecoveryState any      ← provider-specific recovery data (non-filesystem)
└── Metadata      Metadata ← physical state at time of tombstone
```

### 9.3 Migration Path

1. The file provider's `prepareWrite` pattern delegates to the executor's
   tombstone layer. Recovery uses `op.RecoverySite` which places all archived
   files within `.devlore/recovery/` under the authority boundary (same
   partition guaranteed).

2. The decision of *when* to tombstone moves from the provider to the
   executor. Today `prepareWrite` decides; in the target architecture, the
   executor's pre-flight pass decides based on namespace analysis.

3. Non-filesystem providers get tombstone support for free. A `service.Start`
   that shadows a `svc://nginx` resource causes the executor to capture the
   current service state before the node executes.

## 10. Open Questions and Resolved Decisions

### Resolved

1. **Go generics constraint** — `Resource` is a sealed interface.
   `file.Resource` embeds `op.ResourceBase`. The catalog stores `Resource`
   interface values, enabling polymorphic access to typed resources.

2. **URI canonicalization** — Yes. `filepath.Abs` + `filepath.Clean` before
   URI creation. Applied at plan time (no `os.Stat` needed for
   canonicalization).

3. **Immediate mode** — Immediate receivers pass through raw values;
   resources are planning-only.

4. **Catalog scope** — Per-graph. Phases are saga boundaries
   (compensation), not visibility boundaries.

5. **Tombstone ownership** — Executor owns the *decision*; providers own
   the *mechanism*.

6. **Coercion vs resolution** — Planner coerces (pure type-tagging, no
   I/O). Executor resolves (metadata population per target machine).
   Required because a graph is planned once and executed on many machines.

7. **Resolution timing** — Executor pre-flight pass resolves all unresolved
   entries as a flat iteration over the ledger before any node executes.
   Fail-fast: missing source files detected before partial execution.
   Pending entries are resolved by node execution results.

### Open

1. **Gather + resources** — When a gather produces N outputs at the same URI
   scheme, how does the namespace handle uniqueness? Current leaning: each
   iteration gets a unique URI suffix (e.g., `file:///etc/foo.0`,
   `file:///etc/foo.1`).

2. **Remote execution transport** — The graph is portable (planned once,
   executed on many machines). The platform provider (section 6.6) resolves
   target-machine identity via `op.Context.Platform`, but the execution
   transport itself — how the executor runs nodes and stats files on a
   remote machine — is not yet defined. The pre-flight resolution pass
   needs a filesystem abstraction for remote targets (Platform carries
   package/service managers but not filesystem operations).

## 11. Implementation Phases

See [Resource Management Plan](../plans/resource-management.md) for full
details, requirements, and file listings.

| Phase | Focus | Key Files | Status |
|-------|-------|-----------|--------|
| 0–2 | Core types + graph integration | `pkg/op/resource.go`, `pkg/op/graph.go`, `pkg/op/output.go` | Done |
| 3 | File provider migration + catalog | `pkg/op/provider/file/resource.go`, `pkg/op/resource_catalog.go` | Done |
| 4 | Resource type system + starvalue | `pkg/op/resource.go`, `pkg/op/starvalue/`, `pkg/op/starvalue_marshal.go` | Done |
| 4.5 | Action interface unification | `pkg/op/action.go`, `pkg/op/action_reflect.go`, `generate.star` | Done |
| 5 | Tombstone layer | `internal/execution/executor.go`, `pkg/op/tombstone.go` (planned) | Planned |
| 5.5 | Provider context + resource types | `pkg/op/provider/*/provider.go`, `*/resource.go` | Done |
| 6 | Remaining provider method migration | `pkg/op/provider/*/provider.go` | Planned |
| 7 | Code generation | Templates and generator | Planned |

Phases 0–4, 4.5, and 5.5 are complete. Phase 4 established the `Resource`
interface with `ResourceBase`, consolidated `ResourceManager` +
`NamespaceMap` into `ResourceCatalog`, and extracted
`starvalue.Marshaler`/`Unmarshaler` interfaces for custom Starlark
serialization. Phase 4.5 unified the `Do` return signature to
`(Result, Complement, error)` across all three action types (pure,
fallible, compensable), eliminated the `DoAction` type-switch dispatcher,
moved normalization into each reflected type's `Do` method, and updated
the codegen to register and test pure actions as graph-mode `Action`
nodes. Phase 5.5 embedded `op.ProviderBase` in all providers,
created resource types for git/service/pkg, introduced typed tombstones,
removed `output io.Writer` and direct `Platform` fields, and established
per-graph provider lifecycle via `ActionRegistrar`. Phase 5 extracts
tombstone logic from the file provider to the executor. Phase 6 migrates
remaining provider method signatures to accept Resource-typed parameters.
