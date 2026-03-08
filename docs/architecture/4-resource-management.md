# Resource Management: URI-Based Resource Tracking

This document describes the resource management architecture for devlore-cli:
how providers track external state through typed resource handles, how the
namespace resolves URI-based identity across the execution graph, and how
tombstone recovery unifies under a single executor-owned mechanism.

See also: [Resource Management Plan](../plans/resource-management.md) — full
implementation plan with phases, requirements, and file listings.

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
    SourcePath string
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
| `SourcePath` | `NewResource` arg | Filesystem path for I/O operations |
| `Inode` | `syscall.Stat_t.Ino` | Physical identity (survives rename) |
| `Device` | `syscall.Stat_t.Dev` | Partition identity (same-device rename guarantee) |
| `Size` | `os.FileInfo.Size()` | Content length |
| `Mode` | `os.FileInfo.Mode().Perm()` | Permission bits |
| `ModTime` | `os.FileInfo.ModTime()` | Last modification time |
| `Checksum` | `checksumFile()` (SHA-256) | Content integrity |

**Three resource states:**

- **Unresolved** — Source input. URI and type set, metadata empty. Created
  at plan time by `catalog.Resolve()`. Resolved by the executor's
  pre-flight pass (os.Stat against target machine).
- **Pending** — Output. URI and type set, metadata empty. Created at plan
  time by `catalog.Shadow()`. Resolved by node execution results — the
  provider populates metadata after creating the entity.
- **Resolved** — Metadata populated (inode, device, size, mode, modtime,
  checksum). Either resolved by pre-flight or by node execution.

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
    RecoveryPath string       // Where data was temporarily moved (empty if nothing existed)
}
```

Tombstones are the undo receipts for destructive operations. The embedded
Resource always preserves its true identity — `SourcePath` is the file's home.
`RecoveryPath` records where data was temporarily moved during the operation.

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

`Read` (`provider.go:772`) creates a `Resource` from a path:

```go
// pkg/op/provider/file/provider.go:772-774
func (p *Provider) Read(path string) (result Resource, err error) {
    return NewResource(path)
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
| `Read` | Fallible | `string` | `Resource` | Partial |
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

`moveToRecovery` (`recovery.go:13-60`) moves a file to a UUID-keyed path
in a recovery directory guaranteed to be on the same partition:

```go
// pkg/op/provider/file/recovery.go (reduced)
func (p *Provider) moveToRecovery(resource Resource, prune bool, pruneBoundary string) (Tombstone, error) {

    originalPath, _ := filepath.Abs(resource.SourcePath)
    recoveryBase, _ := p.getRecoveryBase(originalPath)
    id := uuid.New().String()
    recoveryPath := filepath.Join(recoveryBase, id)
    os.MkdirAll(recoveryBase, 0700)
    os.Rename(originalPath, recoveryPath) // O(1) — same partition

    // Resource identity is NOT modified. RecoveryPath records temporary location.
    return Tombstone{
        TombstoneBase: op.NewTombstoneBase(&resource),
        RecoveryPath:  recoveryPath,
    }, nil
}
```

`getRecoveryBase` (`recovery_unix.go:21-56`) ensures the recovery directory
is on the same device as the source file. It first tries
`os.UserCacheDir()/devlore/recovery` and verifies same-device via
`syscall.Stat_t.Dev`. If that fails, it walks mount points upward via
`findMountPoint` until the device ID changes, then places the recovery
directory at the mount root:

```go
// pkg/op/provider/file/recovery_unix.go:21 (reduced)
func (p *Provider) getRecoveryBase(absolutePath string) (string, error) {
    sourcePath, sourceInfo, _ := getFirstExistingAncestor(absolutePath)

    if cacheDir, err := os.UserCacheDir(); err == nil {
        recoveryDir := filepath.Join(cacheDir, "devlore", "recovery")
        _, targetInfo, _ := getFirstExistingAncestor(recoveryDir)
        if sameDevice, _ := isSameDevice(sourceInfo, targetInfo); sameDevice {
            return recoveryDir, nil
        }
    }

    mountPoint, _ := findMountPoint(sourcePath, sourceInfo)
    return filepath.Join(mountPoint, ".devlore_recovery"), nil
}
```

### 5.2 Compensation via Tombstone

`restoreFromRecovery` (`recovery.go:71-100`) reverses the rename:

```go
// pkg/op/provider/file/recovery.go (reduced)
func (p *Provider) restoreFromRecovery(tombstone Tombstone) error {
    originalPath := tombstone.Resource().(*Resource).SourcePath  // true home
    recoveryPath := tombstone.RecoveryPath                       // temporary location
    if _, err := os.Stat(recoveryPath); errors.Is(err, fs.ErrNotExist) {
        return fmt.Errorf("recovery source not found: %s", recoveryPath)
    }
    os.MkdirAll(filepath.Dir(originalPath), 0755)
    return os.Rename(recoveryPath, originalPath)
}
```

All compensable file operations (Remove, RemoveAll, Unlink, Copy, WriteText,
WriteBytes) use the same pattern: extract the `Tombstone` from the undo state
via `undo["tombstone"].(Tombstone)`, then call `restoreFromRecovery`.

### 5.3 The prepareWrite Pattern

`prepareWrite` (`provider.go:818-846`) combines discovery and preemptive
recovery into a single operation. Every write method calls it before mutating
the filesystem:

```go
// pkg/op/provider/file/provider.go (reduced)
func (p *Provider) prepareWrite(resource Resource) (result Resource, undo Tombstone, err error) {

    result = NewResource(resource.SourcePath)
    result.Resolve()                        // Discovery

    if !result.Exists() {
        os.MkdirAll(filepath.Dir(result.SourcePath), 0o750)
        undo = Tombstone{TombstoneBase: op.NewTombstoneBase(&result)}
        return result, undo, nil            // New file — no RecoveryPath
    }

    tombstone, _, err := p.Remove(result, false, Resource{})
    return result, tombstone, err           // Existing file — tombstone with RecoveryPath
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
    ├── Resolve(uri) → id       ← discover or return existing
    ├── Shadow(resource, originID) → id  ← new version, update namespace
    ├── Lookup(id) → Resource, bool
    ├── Current(uri) → id
    └── Len() → int
```

The catalog stores `Resource` interface values, enabling polymorphic
access to actual typed resources (e.g., `file.Resource`). It deduplicates
by URI — if 5 nodes reference the same source path, `Resolve()` returns
the same resource ID. Pre-flight resolution stats each unique URI once.

**Plan-time vs execution-time ownership:**

| Concern | Owner | When |
| --- | --- | --- |
| URIs and relationships | Planner (ResourceCatalog) | Plan time |
| Implicit edges via shadowing | Planner | Plan time |
| Resource state (unresolved/pending/resolved) | ResourceCatalog | Both |
| Metadata (inode, size, checksum) | Executor pre-flight | Execution time, per target |
| Pending → resolved transitions | Node execution results | Execution time |

### 6.2 Catalog Operations

The `ResourceCatalog` provides two core operations during planning:

- **Resolve(uri)** — Returns the current resource ID for a URI. If the URI
  has never been seen, catalogs a discovery (`ResourceBase` with URI only,
  no `originID`). If the URI was previously shadowed, returns the shadowed
  version.

- **Shadow(resource, originID)** — Catalogs a new resource version in the
  ledger, stamps its `id` and `originID`, updates the namespace to point
  to it. Any subsequent `Resolve` for this URI returns the shadowed version.

### 6.3 Shadowing Walkthrough

Step-by-step catalog state for a write-then-read of the same path:

```
Initial state:
  catalog.entries = []
  catalog.ns = {}

Step 1: plan.file.write_text(destination="/etc/foo", content="v2", mode=0o644)
  ├─ Node A created
  ├─ uri = "file:///etc/foo"
  ├─ catalog.Shadow(resource, nodeA.ID)
  │   ├─ entries = [file.Resource{uri:"file:///etc/foo", id:"res-1", originID:"A"}]
  │   └─ ns = {"file:///etc/foo" → "res-1"}
  └─ return id "res-1"

Step 2: plan.file.read(path="/etc/foo")
  ├─ Node B created
  ├─ uri = "file:///etc/foo"
  ├─ catalog.Resolve("file:///etc/foo")
  │   └─ returns "res-1" (produced by node A)
  ├─ Node B depends on Node A (via originID = "A")
  └─ Executor guarantees: A runs before B

Result: write happens before read. No explicit Output passing needed.
```

### 6.4 Planning Data Flow (Pure — No I/O)

The planner is pure. It coerces strings to typed Resources (URI only, no
metadata) and builds the ledger skeleton. No `os.Stat`, no filesystem
access. This is required because a graph is planned once and executed on
many machines.

```
Starlark call: plan.file.write_text(destination="/etc/foo", content="v2", mode=0o644)
    │
    ▼
buildPlannedBridge.write_text()
    ├─ coerce "/etc/foo" → file.Resource{uri: "file:///etc/foo", state: pending}
    ├─ catalog.Shadow(resource, node.ID)             ← new entry in ledger
    ├─ createNode("file.write_text")                 ← graph node
    ├─ fillSlots(node, {destination: file.Resource{pending}, content, mode})
    └─ return Output (promise)

Starlark call: plan.file.read(path="/etc/foo")
    │
    ▼
buildPlannedBridge.read()
    ├─ coerce "/etc/foo" → file.Resource (type-tag only)
    ├─ catalog.Resolve("file:///etc/foo")
    │   └─ returns id from Shadow above → originID = write node
    ├─ createNode("file.read") with implicit edge from write node
    └─ return Output (promise)
```

### 6.5 Execution Data Flow (Per Target Machine)

The executor resolves the plan-time skeleton against a specific target
machine. Pre-flight resolution populates metadata for unresolved entries.
Node execution populates metadata for pending entries.

```
Executor.Run(ctx, graph)  — on target machine
    │
    ├─ Pre-flight: resolution pass (flat iteration over ledger)
    │   ├─ For each entry with state=unresolved:
    │   │   ├─ os.Stat on target machine → populate metadata
    │   │   ├─ Mark entry as resolved
    │   │   └─ Fail-fast if source does not exist
    │   └─ Pending entries skipped (don't exist yet)
    │
    ├─ Pre-flight: tombstone scan
    │   ├─ For each resource slot with originID:
    │   │   ├─ URI occupied by different resource? → create tombstone
    │   │   └─ URI unoccupied? → no action
    │   └─ Inject physical state into slots
    │
    ├─ For each node (topological order):
    │   ├─ action.Do(ctx, resolvedSlots) → result, complement, error
    │   ├─ Post-flight: metadata update
    │   │   ├─ Re-stat file for kernel-assigned identity
    │   │   ├─ Record actual hash, inode, size in ledger
    │   │   ├─ Mark pending resource as resolved
    │   │   └─ Fulfill resource slot for downstream nodes
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
plan.choose(distro, {
    "Debian": lambda: plan.pkg.install(packages=["nginx"]),
    "Fedora": lambda: plan.pkg.install(packages=["nginx"]),
})
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
| **Recovery** (compensation) | Per-provider tombstone (`moveToRecovery`) | Executor-owned tombstone layer; providers retain same-device logic |

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
pruneBoundary, backupSuffix, honorGitignore) are unchanged.

| Method | Type | Before | After |
|--------|------|--------|-------|
| `Copy` | Compensable | `(Resource, string, FileMode)` → `Resource` | `(Resource, Resource, FileMode)` → `Resource` |
| `WriteText` | Compensable | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `WriteBytes` | Compensable | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `Read` | Fallible | `(string)` → `Resource` | `(Resource)` → `Resource` |
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
| file | paths → `Resource` | mode, prune, pruneBoundary, backupSuffix, honorGitignore |
| git | url + path → `Resource` | ref, branch |
| net | url → `Resource` | (none) |
| pkg | package names → `Resource` | manager, cask |
| service | service name → `Resource` | (none) |
| template | source + path → `Resource` | templateData, project |
| archive | source + prefix → `Resource` | (none) |
| encryption | source → `Resource` | decryptor |
| shell | (none — commands are strings, not external state) | command |

## 9. Executor Tombstone Unification

### 9.1 Current: Per-Provider Recovery

Today, tombstone logic lives entirely inside the file provider:

- `moveToRecovery` (`recovery.go:13`) — moves file to UUID-keyed recovery path
- `restoreFromRecovery` (`recovery.go:71`) — renames recovery copy back
- `getRecoveryBase` (`recovery_unix.go:21`) — finds same-partition directory
- `prepareWrite` (`provider.go:818`) — combines discovery + preemptive recovery

No other provider has equivalent recovery logic.

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
├── RecoveryPath  string   ← where the backup lives (filesystem resources)
├── RecoveryState any      ← provider-specific recovery data (non-filesystem)
└── Metadata      Metadata ← physical state at time of tombstone
```

### 9.3 Migration Path

1. The file provider's `prepareWrite` pattern delegates to the executor's
   tombstone layer. The provider retains its same-device recovery logic
   (`getRecoveryBase`, `findMountPoint`) because that is filesystem-specific
   optimization.

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
