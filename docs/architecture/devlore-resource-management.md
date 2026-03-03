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

Three core components and a shadowing mechanism:

```
                         ┌──────────────────────────────┐
                         │        ResourceManager        │
                         │                                │
                         │  ledger: []any (append-only)  │
                         │  metadata: map[ID]Metadata     │
                         └──────────┬───────────────────┘
                                    │ owns
                    ┌───────────────┴───────────────┐
                    │                               │
              ┌─────┴─────┐                 ┌───────┴───────┐
              │  Resource  │                │ NamespaceMap  │
              │            │                │               │
              │  URI       │◄───────────────│ URI → ID      │
              │  ID        │   resolves to  │ Resolve()     │
              │  OriginID  │                │ Shadow()      │
              └────────────┘                └───────────────┘
```

**Resource** (`pkg/op/resource.go:6`) — A typed handle with three identity
fields: `URI` (logical address), `ID` (unique ledger key), and `OriginNodeID`
(the node that produced it, empty if pre-existing). The current implementation:

```go
// pkg/op/resource.go:6-10
type Resource struct {
    URI          string // logical address of the resource (e.g., a file URL)
    ID           string // unique identifier in the flat ResourceLedger
    OriginNodeID string // ID of the node that created this resource
}
```

**Ledger** — An append-only collection of every `Resource` created during
planning, keyed by resource ID. Each entry records its producer
(`OriginNodeID`) or marks itself as pre-existing (discovered). The ledger
is the plan-time skeleton — URIs and relationships, no metadata. It is
portable across machines. Metadata is populated per-execution by the
executor's pre-flight resolution pass against the target machine.

**NamespaceMap** — A mutable URI-to-ResourceID lookup used during planning.
When a provider method reads a path, it calls `Resolve` to get the current
resource version. When it writes, it calls `Shadow` to create a new version
and update the map. This is how implicit dependency edges form.

**Shadowing** — When `plan.file.write_text(destination="/etc/foo", ...)`
executes during planning:

1. A new Node is created in the graph
2. `namespace.Shadow("file:///etc/foo", nodeID)` creates a new Resource in the ledger
3. The namespace updates: `file:///etc/foo` now points to the new Resource ID
4. Any later `plan.file.read(path="/etc/foo")` calls `namespace.Resolve`, gets
   the shadowed version, and the executor knows it depends on the write node
   via `OriginNodeID`

## 3. Resource Types

### 3.1 File Resource (Implemented)

The file provider embeds `op.Resource` and adds filesystem-specific metadata.
This is the reference implementation for all future resource types.

```go
// pkg/op/provider/file/resource.go:25-34
type Resource struct {
    op.Resource
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
| `op.Resource` | Embedded | URI, ID, OriginNodeID — identity tracking |
| `SourcePath` | `NewResource` arg | Filesystem path for I/O operations |
| `Inode` | `syscall.Stat_t.Ino` | Physical identity (survives rename) |
| `Device` | `syscall.Stat_t.Dev` | Partition identity (same-device rename guarantee) |
| `Size` | `os.FileInfo.Size()` | Content length |
| `Mode` | `os.FileInfo.Mode().Perm()` | Permission bits |
| `ModTime` | `os.FileInfo.ModTime()` | Last modification time |
| `Checksum` | `checksumFile()` (SHA-256) | Content integrity |

**Three resource states:**

- **Unresolved** — Source input. URI and type set, metadata empty. Created
  at plan time by `namespace.Resolve()`. Resolved by the executor's
  pre-flight pass (os.Stat against target machine).
- **Pending** — Output. URI and type set, metadata empty. Created at plan
  time by `namespace.Shadow()`. Resolved by node execution results — the
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
// pkg/op/provider/file/resource.go:36-39
type Tombstone struct {
    RecoveryPath string // Where it is now
    OriginalPath string // Where it used to be
}
```

Tombstones are the undo receipts for destructive operations. They are stored
in the compensation state map under the `"tombstone"` key and extracted via
`op.ExtractUndo[Tombstone](undo, "tombstone")`.

### 3.3 Resource Types by URI Scheme

| Scheme | Provider | Type Name | Key Metadata | Status |
|--------|----------|-----------|-------------|--------|
| `file://` | file, template, archive, encryption | `file.Resource` | Path, Inode, Device, Size, Mode, ModTime, Checksum | Implemented |
| `git://` | git | `GitState` | URL, Commit, Branch | Planned |
| `pkg://` | pkg | `PackageState` | Name, Version, Manager | Planned |
| `svc://` | service | `ServiceState` | Name, Status (running/stopped/enabled/disabled) | Planned |
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

| Method | Current Input | Current Output | Status |
|--------|--------------|----------------|--------|
| `Copy` | `Resource` + `string` + `FileMode` | `Resource` | Partial |
| `WriteText` | `string` + `string` + `FileMode` | `Resource` | Partial |
| `WriteBytes` | `string` + `string` + `FileMode` | `Resource` | Partial |
| `Read` | `string` | `Resource` | Partial |
| `Exists` | `Resource` | `bool` | Migrated |
| `Link` | `string` + `string` | `string` | Not started |
| `Move` | `string` + `string` | `string` | Not started |
| `Backup` | `string` + `string` | `string` | Not started |
| `Remove` | `string` + `bool` + `string` | `Tombstone` | Not started |
| `RemoveAll` | `string` + `bool` + `string` | `Tombstone` | Not started |
| `Unlink` | `string` + `bool` + `string` | `Tombstone` | Not started |
| `WalkTree` | `string` + `Reducer` + `bool` | `any` + `*RecoveryStack` | Not started |

"Partial" means the output is `Resource` but some inputs remain raw strings.
"Not started" means both inputs and outputs use raw types.

### 4.5 Constructor Registry and Coercion

The constructor registry (`pkg/op/marshal.go:23-48`) enables automatic
type conversion from Starlark slot values to Go provider parameters. When a
Starlark script passes a string path to a method expecting `file.Resource`,
the reflection bridge coerces it automatically.

**Registration** (`pkg/op/marshal.go:27-32`):

```go
func RegisterConstructor[T any](fn func(any) (T, error)) {
    t := reflect.TypeOf((*T)(nil)).Elem()
    constructorRegistry.Store(t, func(v any) (any, error) {
        return fn(v)
    })
}
```

**File Resource registration** (`pkg/op/provider/file/resource.go:14-22`):

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

**Coercion chain** — When the reflection bridge in `action_reflect.go:82-112`
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
// pkg/op/provider/file/recovery.go:13 (reduced)
func (p *Provider) moveToRecovery(path string, prune bool, pruneBoundary string) (
    result Tombstone, undoState map[string]any, err error) {

    absolutePath, _ := filepath.Abs(path)
    recoveryBase, _ := p.getRecoveryBase(absolutePath)
    id := uuid.New().String()
    recoveryPath := filepath.Join(recoveryBase, id)
    os.MkdirAll(recoveryBase, 0700)
    os.Rename(absolutePath, recoveryPath) // O(1) — same partition

    result = Tombstone{
        OriginalPath: absolutePath,
        RecoveryPath: recoveryPath,
    }
    return result, map[string]any{"tombstone": result}, nil
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
// pkg/op/provider/file/recovery.go:71 (reduced)
func (p *Provider) restoreFromRecovery(tombstone Tombstone) error {
    // Verify recovery source still exists
    if _, err := os.Stat(tombstone.RecoveryPath); errors.Is(err, fs.ErrNotExist) {
        return fmt.Errorf("recovery source not found: %s", tombstone.RecoveryPath)
    }
    os.MkdirAll(filepath.Dir(tombstone.OriginalPath), 0755)
    return os.Rename(tombstone.RecoveryPath, tombstone.OriginalPath)
}
```

All compensable file operations (Remove, RemoveAll, Unlink, Copy, WriteText,
WriteBytes) use the same pattern: extract the `Tombstone` from the undo state
via `op.ExtractUndo[Tombstone](undo, "tombstone")`, then call
`restoreFromRecovery`.

### 5.3 The prepareWrite Pattern

`prepareWrite` (`provider.go:818-846`) combines discovery and preemptive
recovery into a single operation. Every write method calls it before mutating
the filesystem:

```go
// pkg/op/provider/file/provider.go:818-846
func (p *Provider) prepareWrite(path string) (
    result Resource, undo map[string]any, err error) {

    result, err = NewResource(path)         // Discovery
    if err != nil {
        return Resource{}, nil, err
    }

    if !result.Exists() {
        err = os.MkdirAll(filepath.Dir(result.SourcePath), 0o750)
        if err != nil {
            return Resource{}, nil, errors.Join(os.ErrNotExist, err)
        }
        undo = map[string]any{"tombstone": Tombstone{OriginalPath: result.SourcePath}}
        return result, undo, nil            // New file — tombstone with no recovery path
    }

    tombstone, _, err := p.Remove(result.SourcePath, false, "")
    if err != nil {
        return Resource{}, nil, fmt.Errorf("failed to backup existing file: %w", err)
    }
    undo = map[string]any{"tombstone": tombstone}
    return result, undo, nil                // Existing file — tombstone with recovery path
}
```

Two branches:
- **New file**: Creates a tombstone with `OriginalPath` only (no `RecoveryPath`).
  Compensation deletes the newly created file.
- **Existing file**: Moves the existing file to recovery via `Remove`, creating
  a full tombstone. Compensation removes the new file and renames the recovery
  copy back.

### 5.4 Gap

Only the file provider has tombstone recovery. The service, package, git, net,
and archive providers have compensation pairs (e.g., `CompensateInstall` calls
`Remove`), but none have a pre-flight discovery + backup mechanism equivalent
to `prepareWrite`. A failed `service.Start` compensates by calling `Stop`, but
there is no "snapshot the previous state" step.

## 6. Target Architecture

This section describes what the system will look like when the resource
management plan is fully implemented.

### 6.1 ResourceManager

The `ResourceManager` is a graph-level object that owns the ledger. One
`ResourceManager` per plan session. The ledger is the plan-time skeleton —
URIs, relationships, and resource states, but no target-machine-specific
metadata. Metadata is populated per-execution by the executor's pre-flight
resolution pass.

```
ResourceManager
├── ledger: map[ID]Resource   ← append-only, keyed by resource ID
└── methods:
    ├── EnsureCataloged(uri, producerID) → Resource
    ├── Lookup(id) → Resource, bool
    ├── Unresolved() → []Resource   ← entries needing pre-flight resolution
    └── Resolve(id, metadata)       ← called by executor after stat
```

The ledger stores `Resource` values (with `op.Resource` embedded). The
`NamespaceMap` deduplicates by URI — if 5 nodes reference the same source
path, `namespace.Resolve()` returns the same resource ID. Pre-flight
resolution stat's each unique URI once.

**Plan-time vs execution-time ownership:**

| Concern | Owner | When |
| --- | --- | --- |
| URIs and relationships | Planner (ResourceManager + NamespaceMap) | Plan time |
| Implicit edges via shadowing | Planner | Plan time |
| Resource state (unresolved/pending/resolved) | ResourceManager | Both |
| Metadata (inode, size, checksum) | Executor pre-flight | Execution time, per target |
| Pending → resolved transitions | Node execution results | Execution time |

### 6.2 NamespaceMap

The `NamespaceMap` provides two operations during planning:

- **Resolve(uri)** — Returns the current resource for a URI. If the URI has
  never been seen, catalogs a discovery (pre-existing resource with no
  `OriginNodeID`). If the URI was previously shadowed, returns the shadowed
  version.

- **Shadow(uri, producerNodeID)** — Creates a new resource version in the
  ledger, updates the namespace to point to it. The new resource's
  `OriginNodeID` is set to the producer node. Any subsequent `Resolve` for
  this URI returns the shadowed version.

### 6.3 Shadowing Walkthrough

Step-by-step namespace state for a write-then-read of the same path:

```
Initial state:
  namespace = {}
  ledger = []

Step 1: plan.file.write_text(destination="/etc/foo", content="v2", mode=0o644)
  ├─ Node A created
  ├─ uri = "file:///etc/foo"
  ├─ Shadow("file:///etc/foo", nodeA.ID)
  │   ├─ ledger = [Resource{ID:"res_0", URI:"file:///etc/foo", OriginNodeID:"A"}]
  │   └─ namespace = {"file:///etc/foo" → "res_0"}
  └─ return Resource{ID:"res_0"} as Output

Step 2: plan.file.read(path="/etc/foo")
  ├─ Node B created
  ├─ uri = "file:///etc/foo"
  ├─ Resolve("file:///etc/foo")
  │   └─ returns "res_0" (produced by node A)
  ├─ Node B depends on Node A (via res_0.OriginNodeID = "A")
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
    ├─ coerce "/etc/foo" → file.Resource{URI: "file:///etc/foo", state: pending}
    ├─ namespace.Shadow("file:///etc/foo", node.ID)  ← new entry in ledger
    ├─ createNode("file.write_text")                 ← graph node
    ├─ fillSlots(node, {destination: file.Resource{pending}, content, mode})
    └─ return Output (promise)

Starlark call: plan.file.read(path="/etc/foo")
    │
    ▼
buildPlannedBridge.read()
    ├─ coerce "/etc/foo" → file.Resource (type-tag only)
    ├─ namespace.Resolve("file:///etc/foo")
    │   └─ returns resource from Shadow above → OriginNodeID = write node
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
    │   ├─ For each resource slot with OriginNodeID:
    │   │   ├─ URI occupied by different resource? → create tombstone
    │   │   └─ URI unoccupied? → no action
    │   └─ Inject physical state into slots
    │
    ├─ For each node (topological order):
    │   ├─ action.Do(ctx, resolvedSlots) → result, undoState, error
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

**Current state** — Providers are zero-value structs with no constructor:

```go
// Generated code — no context, no lifecycle
op.RegisterReflectedActions(reg, "pkg", &provider.Provider{}, Params)
NewPkgReceiver(&provider.Provider{})
```

Context arrives per-call via `op.Context` in `action.Do(ctx, slots)`, but
providers have no access to it at construction time. This forces providers
that need platform info (pkg, service) to take manager arguments on every
method call or to call `host.NewHost()` on every invocation.

**Target state** — Every provider has a constructor that accepts context:

```go
// Target — context-injected singleton
p := provider.New(ctx)
op.RegisterReflectedActions(reg, "pkg", p, Params)
NewPkgReceiver(p)
```

The provider holds a reference to the context and reads from it for the
duration of the graph or script. For the platform provider, this means
reading `ctx.Platform` directly. For file, pkg, and service providers,
this means accessing Platform's managers without method-level arguments.

This is a codegen change: the generated binding registrars and receiver
factories must call a provider constructor with context instead of
creating zero-value structs.

## 7. Integration with Current Architecture

### 7.1 Relationship to Output/FillSlot

Resources enrich the existing slot model — they do not replace it. Nodes,
edges, slots, and actions remain unchanged. Resources add identity tracking
on top.

| Aspect | Current | With Resources |
|--------|---------|----------------|
| Slot values | `"/etc/foo"` (string) | `Resource{ID:"res_3", URI:"file:///etc/foo"}` |
| Edge creation | Explicit `*Output` passing only | Also implicit via shadowing (`OriginNodeID`) |
| Provider params | `path string` | `Resource` (extracts path from `SourcePath`) |
| Tombstone logic | Per-provider (`prepareWrite`) | Executor pre-flight, keyed by resource ID |
| Conflict detection | None | NamespaceMap detects two nodes claiming same URI |

### 7.2 Relationship to Compensation Model

The phase execution model defines three layers: Definition, Activation, and
Recovery. Resources map cleanly to each:

| Layer | Current | With Resources |
|-------|---------|----------------|
| **Definition** (planning) | Nodes created with string slots | Nodes created with Resource slots; namespace tracks shadowing |
| **Activation** (execution) | Provider method called with slot values | Provider method called with Resource; metadata updated post-write |
| **Recovery** (compensation) | Per-provider tombstone (`moveToRecovery`) | Executor-owned tombstone layer; providers retain same-device logic |

The compensation pairs (`Forward`/`Compensate*`) are unchanged. The undo state
map still carries `"tombstone"` entries. What changes is *who decides* when to
create tombstones: the executor's pre-flight pass instead of each provider's
`prepareWrite`.

### 7.3 Relationship to Constructor Registry

The existing `RegisterConstructor`/`Construct`/`coerceSlotValue` chain
(`marshal.go:27-48`, `action_reflect.go:82-112`) handles string-to-Resource
coercion at execution time. When a Starlark script passes `"/etc/foo"` to a
method expecting `file.Resource`, the constructor calls `NewResource` (which
does `os.Stat`) and returns a fully populated resource.

**Target architecture**: The constructor registry evolves into a coercion
table with two concerns:

1. **Plan-time coercion** (pure, no I/O) — `string → file.Resource{URI
   only}`. Called by `buildPlannedBridge` during planning. Creates typed
   but metadata-empty Resources. This is the entry point for namespace
   resolution — the coerced Resource is tracked in the ledger via
   `namespace.Resolve()` or `namespace.Shadow()`.

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

| Method | Before | After |
|--------|--------|-------|
| `Copy` | `(Resource, string, FileMode)` → `Resource` | `(Resource, Resource, FileMode)` → `Resource` |
| `WriteText` | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `WriteBytes` | `(string, string, FileMode)` → `Resource` | `(Resource, string, FileMode)` → `Resource` |
| `Read` | `(string)` → `Resource` | `(Resource)` → `Resource` |
| `Exists` | `(Resource)` → `bool` | No change (already migrated) |
| `Link` | `(string, string)` → `string` | `(Resource, Resource)` → `Resource` |
| `Move` | `(string, string)` → `string` | `(Resource, Resource)` → `Resource` |
| `Backup` | `(string, string)` → `string` | `(Resource, string)` → `Resource` |
| `Remove` | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |
| `RemoveAll` | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |
| `Unlink` | `(string, bool, string)` → `Tombstone` | `(Resource, bool, string)` → `Tombstone` |

The pattern: every parameter that identifies an external entity (a path, a
URL, a package name) becomes a `Resource`. Parameters that are configuration
values (modes, flags, suffixes) remain unchanged.

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
URIs (a resource with `OriginNodeID` shadows a discovered resource at the same
URI). For each shadowed URI, it creates a tombstone before any node executes.

The target tombstone type lives in `pkg/op/tombstone.go` as a peer of
`resource.go`:

```
pkg/op/
├── resource.go     ← Resource (identity)
├── tombstone.go    ← ResourceTombstone (recovery)
├── namespace.go    ← NamespaceMap (planning)
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

1. **Go generics constraint** — Non-generic `Resource` with embedding.
   `file.Resource` embeds `op.Resource`. The ledger stores `Resource` values
   keyed by ID.

2. **URI canonicalization** — Yes. `filepath.Abs` + `filepath.Clean` before
   URI creation. Applied at plan time (no `os.Stat` needed for
   canonicalization).

3. **Immediate mode** — Immediate receivers pass through raw values;
   resources are planning-only.

4. **Namespace scope** — Per-graph. Phases are saga boundaries
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

| Phase | Focus | Key Files |
|-------|-------|-----------|
| 1 | Core types | `pkg/op/resource.go`, `pkg/op/namespace.go` (new) |
| 2 | Graph integration | `pkg/op/graph.go`, `pkg/op/output.go` |
| 3 | File provider migration | `pkg/op/provider/file/provider.go`, `resource.go` |
| 4 | Tombstone layer | `internal/execution/executor.go`, `pkg/op/tombstone.go` (new) |
| 5 | Remaining providers | `pkg/op/provider/*/provider.go` |
| 6 | Code generation | Templates and generator |

Phases 1-2 are pure additions (no existing code changes). Phase 3 is the
pilot migration using the file provider as reference implementation. Phase 4
extracts tombstone logic from the file provider to the executor. Phase 5
propagates the pattern to all providers. Phase 6 updates the codegen pipeline
(`MethodParams` → `ParamSpec`, `generate.star` resource detection,
`params.go.template`). During migration, the system runs in mixed mode:
resource-aware providers coexist with raw-type providers.
