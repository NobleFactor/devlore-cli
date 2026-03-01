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
**reality** (execution) using three core components and a shadowing mechanism.

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

**Ledger** — An append-only list of every `Resource` created during planning.
Each entry records its producer (`OriginNodeID`) or marks itself as
pre-existing (discovered). The ledger is the audit trail for the entire plan.
It stores `any` because Go generics do not allow `[]Resource[T]` for mixed `T`
in a single slice.

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

**Discovery**: `NewResource` (`resource.go:47`) performs `os.Stat` and
populates all metadata fields. If the file does not exist, it returns a
partial resource with only `URI` and `SourcePath` set — a "known path but no
data" state that the executor must fulfill via a node later.

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

Step 5 is where string-to-Resource conversion happens. The coercion is
transparent to Starlark scripts and provider code — Starlark passes a string
path, the constructor calls `NewResource`, and the provider method receives a
fully populated `file.Resource` with Inode, Size, Mode, and Checksum already
filled in.

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
`ResourceManager` per plan session. The ledger stores `any` because Go
generics do not allow mixed type parameters in a single slice.

```
ResourceManager
├── ledger: []any             ← append-only, mixed Resource types
├── metadata: map[ID]Metadata ← physical state (hash, inode, size)
└── methods:
    ├── EnsureCataloged(uri, producerID) → Resource
    └── LookupResource[T](id) → T, bool
```

Type-safe access is via `LookupResource[T]`:

```go
// Target API — type-safe access to the any-typed ledger
func LookupResource[T any](mgr *ResourceManager, id string) (T, bool) {
    // Iterate ledger, type-assert to Resource wrapper containing T metadata
}
```

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

### 6.4 Planning Data Flow

```
Starlark call: plan.file.write_text(destination="/etc/foo", content="v2", mode=0o644)
    │
    ▼
PlannedReceiver.write_text()
    ├─ uri = "file://" + destination
    ├─ namespace.Shadow(uri, node.ID)    ← new Resource in ledger
    ├─ createNode("file.write_text")     ← graph node with slots
    ├─ fillSlots(node, {destination, content, mode})
    └─ return Resource (carries Output promise internally)
```

### 6.5 Execution Data Flow

```
Executor.executeNode(node)
    │
    ├─ Pre-flight: tombstone scan
    │   ├─ For each resource slot with OriginNodeID:
    │   │   ├─ URI occupied by different resource? → create tombstone
    │   │   └─ URI unoccupied? → no action
    │   └─ Inject physical state into slots
    │
    ├─ action.Do(ctx, resolvedSlots) → result, undoState, error
    │
    ├─ Post-flight: metadata update
    │   ├─ Re-stat file for kernel-assigned identity
    │   ├─ Record actual hash, inode, size in ledger
    │   └─ Fulfill resource slot for downstream nodes
    │
    └─ Push recovery entry onto stack
```

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
(`marshal.go:27-48`, `action_reflect.go:82-112`) already handles
string-to-Resource coercion. When a Starlark script passes `"/etc/foo"` to a
provider method expecting `file.Resource`, the constructor calls `NewResource`
and returns a fully populated resource.

Resources formalize what the constructor registry does informally. Today, the
constructor converts a path string to a `file.Resource` with discovery
metadata. With the full resource architecture, the constructor becomes the
entry point for namespace resolution — converting a string path into a
namespace-tracked resource handle with lineage.

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

## 10. Open Questions

1. **Go generics constraint** — `Resource[T]` with mixed `T` in a single
   ledger requires `any`-typed storage. Current code leans toward non-generic
   `Resource` with embedding (as `file.Resource` already does).

2. **URI canonicalization** — Should `file:///etc/foo` and
   `file:///etc/../etc/foo` resolve to the same resource? Current leaning:
   yes — `filepath.Clean` before URI creation (already effectively done by
   `os.Stat` in `NewResource`).

3. **Immediate mode** — Immediate receivers (non-plan) have no graph and
   nothing to shadow. Resources may be unnecessary for
   `file.exists(path="/tmp/foo")`. Current leaning: immediate mode passes
   through raw values; resources are planning-only.

4. **Namespace scope** — One namespace per graph or one per phase? Per-graph
   means phase boundaries don't reset visibility. Per-phase means phase A's
   writes aren't visible to phase B unless explicitly passed. Current leaning:
   per-graph, since phases are saga boundaries (compensation), not isolation
   boundaries (visibility).

5. **Tombstone ownership** — Should the executor own ALL tombstone logic, or
   should providers retain the option for domain-specific compensation?
   `service.Stop` → `service.Start` has no filesystem tombstone. Current
   leaning: executor owns the *decision* of when to tombstone; providers own
   the *mechanism* for their resource type.

6. **Gather + resources** — When a gather produces N outputs at the same URI
   scheme, how does the namespace handle uniqueness? Current leaning: each
   iteration gets a unique URI suffix (e.g., `file:///etc/foo.0`,
   `file:///etc/foo.1`).

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
extracts tombstone logic from the file provider to the executor. Phases 5-6
propagate the pattern to all providers and generated code. During migration,
the system runs in mixed mode: resource-aware providers coexist with raw-type
providers.
