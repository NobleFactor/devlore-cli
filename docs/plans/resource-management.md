---
title: "Resource Management: URI-Based Resource Tracking for Providers"
status: draft
created: 2026-02-27
updated: 2026-02-27
---

# Plan: Resource Management

## Summary

Replace raw parameters (paths, URLs, package names, service names) in provider
method signatures with `Resource[T]` handles backed by a URI namespace. A
`ResourceManager` maintains a versioned ledger of every resource state. A
`NamespaceMap` resolves URIs to the current resource version during planning,
implementing **shadowing** so that sequential writes to the same path produce
distinct resource identities with implicit dependency chains. Providers work
with resources — not paths, not addresses, not names.

## Goals

1. **Lineage tracking**: Every resource state gets a unique ID. Two reads of the
   same path before and after a write resolve to different resource versions.
2. **Implicit dependencies**: Shadowing creates dependency edges automatically.
   Scripts that write then read the same URI get correct ordering without
   explicit promise passing.
3. **Unified conflict detection**: URI-keyed namespace catches conflicts across
   providers (e.g., `file.copy` and `template.render` targeting the same path).
4. **Provider simplification**: Providers receive typed resource handles instead
   of raw strings. No more ambiguity about whether a string is a source path,
   a destination path, or a URI.
5. **Tombstone unification**: The executor's pre-flight binding pass replaces
   per-provider tombstone logic with a universal mechanism.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Node slots | Working | `map[string]SlotValue` — immediate values or promises |
| Output (promise) | Working | Explicit promise passing via `*Output` return values |
| FillSlot | Working | Routes Output/Gather/primitives into slots |
| Graph edges | Working | Created manually by FillSlot when it sees an Output |
| Provider params | Raw types | Strings, modes, bools — no resource identity |
| Tombstones | Per-provider | File provider has its own recovery/tombstone; others don't |
| Dependency ordering | Explicit only | Only promise-passing creates edges; no implicit ordering |
| Conflict detection | None | Two nodes targeting the same path → silent race |

## Architecture

### Core Concepts

**Resource[T]** — A typed handle representing one version of an external entity.
The type parameter `T` carries domain metadata (Blob for files, GitState for
repos, ServiceState for daemons). Resources are values, not references — they
are immutable once created.

**URI** — The logical address of a resource: `file:///etc/hosts`, `git://github.com/org/repo`,
`pkg://homebrew/jq`, `svc://nginx`. URIs are the namespace keys.

**Ledger** — Append-only flat list of every `Resource[T]` created during planning.
Each entry records who produced it (OriginNodeID) or whether it was pre-existing
(discovered). The ledger is the audit trail.

**NamespaceMap** — Mutable lookup from URI to the *current* resource ID. During
planning, each write operation **shadows** the URI with a new resource ID.
Subsequent reads of that URI receive the shadowed version. This is how implicit
dependencies form without explicit Output passing.

**Shadowing** — When `plan.file.write_text(destination="/etc/foo", ...)` executes
during planning, it:
1. Creates a new Node in the graph
2. Calls `namespace.Shadow("file:///etc/foo", nodeID)` → creates new Resource
3. Updates the namespace: `file:///etc/foo` now points to the new resource
4. Any later `plan.file.read(path="/etc/foo")` resolves to this new resource,
   automatically creating a dependency edge

### Resource Types

```go
// Blob — file content (current file.Blob, generalized)
type Blob struct {
    Path  string
    Hash  string
    Size  int64
    Inode uint64
}

// GitState — repository at a specific commit
type GitState struct {
    URL    string
    Commit string
    Branch string
}

// ServiceState — system daemon status
type ServiceState struct {
    Name   string
    Status string // "running", "stopped", "enabled", "disabled"
}

// PackageState — installed package
type PackageState struct {
    Name    string
    Version string
    Manager string
}
```

### URI Scheme Conventions

| Scheme | Provider | Example |
| --- | --- | --- |
| `file://` | file, template, archive, encryption | `file:///home/user/.bashrc` |
| `git://` | git | `git://github.com/org/repo` |
| `pkg://` | pkg | `pkg://homebrew/jq` |
| `svc://` | service | `svc://nginx` |
| `mem://` | (internal) | `mem://template_payload_<id>` |

### Data Flow: Planning Phase

```
Starlark: plan.file.write_text(destination="/etc/foo", content="hello", mode=0o644)
    │
    ▼
PlanReceiver.write_text():
    │
    ├─ uri = "file://" + destination
    ├─ inputRes = namespace.Resolve[Blob](uri)        ← existing or discovered
    ├─ node = createNode("file.write_text")
    ├─ outputRes = namespace.Shadow[Blob](uri, node.ID) ← new version
    ├─ fillSlots(node, {destination, content, mode})
    ├─ node.SetAnnotation("resource.output", outputRes.ID)
    │
    └─ return Resource (carries Output promise internally)
```

### Data Flow: Execution Phase

```
Executor.executeNode(node):
    │
    ├─ resolvedSlots = node.ResolvedSlots(results)
    ├─ Pre-flight: if slot has resource ID, resolve to physical state
    │   ├─ Check ledger for resource metadata
    │   ├─ If shadowed (overwriting existing): PrepareTombstone()
    │   └─ Inject physical values into slots
    │
    ├─ action.Do(ctx, resolvedSlots) → result, undoState, error
    │
    ├─ Post-flight: update resource metadata in ledger
    │   ├─ Record actual hash, inode, size
    │   └─ Fulfill resource slot for downstream nodes
    │
    └─ Store result, push recovery entry
```

### Integration with Current Architecture

The resource layer sits **between** Starlark and the current Node/Slot/Action
system. It does not replace Nodes, Edges, or Actions — it enriches them:

| Current | With Resources |
| --- | --- |
| `SlotValue.Immediate = "/etc/foo"` | `SlotValue.Immediate = Resource[Blob]{ID: "res_3", URI: "file:///etc/foo"}` |
| Edge created only by explicit `Output` passing | Edge also created by shadowing (implicit dependency) |
| Provider takes `path string` | Provider takes `Resource[Blob]` (extracts path from URI) |
| Tombstone logic in each provider | Tombstone logic in executor pre-flight, keyed by resource ID |
| No conflict detection | NamespaceMap detects two nodes claiming same URI |

## Requirements

### Requirement 1: Resource and ResourceManager in `pkg/op`

The `Resource[T]` type and `ResourceManager` live in `pkg/op` alongside Graph,
Node, and Action. The ResourceManager is a Graph-level object — one per plan
session.

```go
// Resource is the handle passed between planning and execution.
type Resource[T any] struct {
    ID           string // Unique (e.g., "res_0", "res_1")
    URI          string // Logical address (e.g., "file:///etc/hosts")
    OriginNodeID string // Node that produces this ("" = pre-existing)
}

type ResourceManager struct {
    mu       sync.Mutex
    ledger   []any                // Append-only list of Resource[T]
    metadata map[string]Metadata  // Resource ID → discovery data
}

// Metadata holds physical state discovered or produced.
type Metadata map[string]any
```

**Constraint**: Go generics don't allow `[]Resource[T]` for mixed T in one
ledger. The ledger stores `any`. Type-safe access is via `LookupResource[T](id)`.

### Requirement 2: NamespaceMap

The NamespaceMap is the planning-phase lookup table. It maps URIs to the
latest resource ID. It lives on the `GraphBuilder` (or directly on `Graph`).

```go
type NamespaceMap struct {
    current map[string]string // URI → most recent Resource ID
}

// Resolve returns the current resource for a URI, or catalogs a discovery.
func (ns *NamespaceMap) Resolve(mgr *ResourceManager, uri string, nodeID string) string

// Shadow creates a new resource version and updates the namespace.
func (ns *NamespaceMap) Shadow(mgr *ResourceManager, uri string, producerNodeID string) string
```

**Shadowing creates implicit edges**: When node B calls `Resolve("file:///etc/foo")`
and gets back a resource produced by node A, the executor knows B depends on A
without any explicit Output passing. The edge is created from the resource's
`OriginNodeID`.

### Requirement 3: Provider Signature Changes

Provider methods change from raw types to resource handles for parameters that
represent external state. Configuration parameters (modes, flags, suffixes)
remain unchanged.

**Before**:
```go
func (p *Provider) Copy(destination string, blob Blob, mode os.FileMode) (string, map[string]any, error)
func (p *Provider) Link(source, path string) (string, map[string]any, error)
func (p *Provider) Read(path string) (Blob, error)
```

**After**:
```go
func (p *Provider) Copy(destination Resource[Blob], source Resource[Blob], mode os.FileMode) (Resource[Blob], map[string]any, error)
func (p *Provider) Link(source Resource[Blob], path Resource[Blob]) (Resource[Blob], map[string]any, error)
func (p *Provider) Read(path Resource[Blob]) (Resource[Blob], error)
```

**Scope of change per provider**:

| Provider | Resource params | Config params (unchanged) |
| --- | --- | --- |
| file | paths → `Resource[Blob]` | mode, prune, pruneBoundary, backupSuffix, honorGitignore |
| git | url + path → `Resource[GitState]` | ref, branch |
| net | url → `Resource[Blob]` | (none) |
| pkg | names → `Resource[PackageState]` | manager, cask |
| service | name → `Resource[ServiceState]` | (none) |
| shell | (none — commands are strings) | command |
| template | source + path → `Resource[Blob]` | templateData, project |
| archive | source + prefix → `Resource[Blob]` | (none) |
| encryption | source → `Resource[Blob]` | decryptor |

### Requirement 4: Executor Pre-Flight Binding

Before executing any node, the executor performs a binding pass. For each
resource in the ledger with an OriginNodeID, the executor checks whether the
URI is currently occupied by a different resource. If so, it creates a
tombstone.

This replaces the per-provider `prepareWrite` / `moveToRecovery` pattern in
the file provider.

### Requirement 5: Starlark Surface

From Starlark, resources are opaque handles with attributes. They carry the
same promise semantics as current `Output` values — they can be passed as
arguments to create dependency edges.

```python
# Resources flow naturally between provider calls
source = plan.file.read(path="/src/data.bin")
result = plan.file.copy(destination="/dst/data.bin", source=source, mode=0o644)

# Shadowing is implicit — no explicit dependency needed
plan.file.write_text(destination="/etc/foo", content="v1", mode=0o644)
content = plan.file.read(path="/etc/foo")  # reads v1, not original
```

## Implementation Phases

### Phase 1: Core Types in `pkg/op`

Introduce `Resource`, `ResourceManager`, `NamespaceMap`, `Metadata`, and the
URI scheme constants. No integration yet — just the data structures and their
unit tests.

- [ ] `Resource[T]` struct with `ID`, `URI`, `OriginNodeID`
- [ ] `ResourceManager` with `EnsureCataloged`, `Lookup`, ledger, metadata store
- [ ] `NamespaceMap` with `Resolve`, `Shadow`
- [ ] URI scheme constants and `ParseURI(scheme, path)` helper
- [ ] Starlark integration: Resource implements `starlark.Value` and `starlark.HasAttrs`
- [ ] Unit tests for ledger append, namespace resolve/shadow, implicit dependency detection

**Files**:
- `pkg/op/resource.go` — Create
- `pkg/op/resource_test.go` — Create
- `pkg/op/namespace.go` — Create
- `pkg/op/namespace_test.go` — Create

### Phase 2: Graph Integration

Wire ResourceManager and NamespaceMap into the Graph lifecycle. The Graph
owns a ResourceManager. Planned receivers use the namespace during planning.

- [ ] Add `ResourceManager` and `NamespaceMap` fields to `Graph`
- [ ] Initialize both in `NewGraph()`
- [ ] Modify `FillSlot` to detect `Resource` values and create implicit edges
- [ ] Add resource annotations to Nodes (input/output resource IDs)
- [ ] Serialize/deserialize resources in graph YAML/JSON

**Files**:
- `pkg/op/graph.go` — Modify
- `pkg/op/output.go` — Modify FillSlot

### Phase 3: File Provider Migration

Migrate the file provider — the most complex and best-tested provider — from
raw paths to `Resource[Blob]`. This is the reference implementation for all
other providers.

- [ ] Change file.Blob to align with resource Blob metadata type
- [ ] Update compensable method signatures: Copy, Link, Move, Backup, Remove,
      RemoveAll, Unlink, WriteText, WriteBytes
- [ ] Update non-compensable method signatures: Read, Exists, IsFile, IsDir,
      Glob, Mkdir
- [ ] Extract `prepareWrite` / `moveToRecovery` logic to executor tombstone layer
- [ ] Update all file provider tests

**Files**:
- `pkg/op/provider/file/provider.go` — Modify
- `pkg/op/provider/file/blob.go` — Modify
- `pkg/op/provider/file/provider_test.go` — Modify
- `pkg/op/provider/file/recovery.go` — Modify

### Phase 4: Executor Tombstone Layer

Move tombstone/recovery logic from individual providers to the executor's
pre-flight binding pass. The executor inspects the resource ledger, identifies
shadowed URIs, and creates tombstones before any node executes.

- [ ] Pre-flight: scan graph for shadowed URIs (resource with OriginNodeID
      shadows a discovered resource at same URI)
- [ ] Create tombstones for shadowed resources
- [ ] Inject tombstone references into node undo state
- [ ] On rollback: restore from tombstones in LIFO order
- [ ] Remove provider-specific tombstone logic (file provider's `prepareWrite`,
      `moveToRecovery`, `compensateWrite`)

**Files**:
- `internal/execution/executor.go` — Modify
- `internal/execution/tombstone.go` — Create
- `pkg/op/provider/file/recovery.go` — Simplify (delegate to executor)

### Phase 5: Remaining Provider Migration

Apply the same pattern to all other providers. Each migration is independent.

- [ ] git: url, path → `Resource[GitState]`, `Resource[Blob]`
- [ ] net: url → `Resource[Blob]` (downloaded content)
- [ ] pkg: package names → `Resource[PackageState]`
- [ ] service: service name → `Resource[ServiceState]`
- [ ] template: source, path → `Resource[Blob]`
- [ ] archive: source, prefix → `Resource[Blob]`
- [ ] encryption: source → `Resource[Blob]`
- [ ] shell: no resource params (commands are strings, not external state)

**Files**:
- `pkg/op/provider/*/provider.go` — Modify (each provider)
- `pkg/op/provider/*/provider_test.go` — Modify (each provider)

### Phase 6: Code Generation Updates

Update the code generation pipeline to produce planned receivers and graph
actions that work with resources. Planned receivers call `Resolve`/`Shadow`
on the namespace. Graph actions extract physical state from resource metadata.

- [ ] Planned receiver template: resource params call namespace Resolve/Shadow
- [ ] Graph actions template: resource slots read resource metadata
- [ ] Immediate receiver template: resource params create resources from literal URIs
- [ ] Struct constructor: `file.blob(...)` creates Resource[Blob] from kwargs

**Files**:
- Templates and generator (dependent on codegen rewrite — details TBD)

## Migration Path

1. **Phase 1-2**: Pure additions. No existing code changes. ResourceManager and
   NamespaceMap exist alongside current Graph/Node/Slot model.
2. **Phase 3**: File provider is the pilot. All file provider tests rewritten.
   Generated code for file provider regenerated.
3. **Phase 4**: Tombstone extraction. File provider tests verify executor-level
   tombstones produce identical behavior to provider-level tombstones.
4. **Phase 5**: Other providers follow file's pattern. Each is independently
   testable.
5. **Phase 6**: Code generator learns about resources. All generated code
   regenerated.

During migration, the system can run in mixed mode: some providers use
resources, others use raw types. The executor handles both — resource slots
resolve to metadata, raw slots pass through unchanged.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/resource.go` | Create | Resource[T], ResourceManager, Metadata |
| `pkg/op/resource_test.go` | Create | Unit tests for resource types |
| `pkg/op/namespace.go` | Create | NamespaceMap with Resolve/Shadow |
| `pkg/op/namespace_test.go` | Create | Unit tests for namespace operations |
| `pkg/op/graph.go` | Modify | Add ResourceManager, NamespaceMap fields |
| `pkg/op/output.go` | Modify | FillSlot handles Resource values |
| `internal/execution/executor.go` | Modify | Pre-flight tombstone binding |
| `internal/execution/tombstone.go` | Create | Unified tombstone mechanism |
| `pkg/op/provider/file/provider.go` | Modify | Resource[Blob] signatures |
| `pkg/op/provider/file/blob.go` | Modify | Align with Blob metadata |
| `pkg/op/provider/file/recovery.go` | Modify | Delegate to executor tombstones |
| `pkg/op/provider/*/provider.go` | Modify | Resource signatures (each provider) |

## Related Documents

- [draft-resource-management.md](../../draft-resource-management.md) — Original design spec
- [Binding Unification Plan](./binding-unification.md) — Provider binding architecture
- [Compensation Plan](./compensation.md) — Compensation/recovery architecture

## Open Questions

- [ ] **Go generics constraint**: `Resource[T]` with mixed T in a single ledger
      requires `any`-typed storage. Is the ergonomic cost acceptable, or should
      Resource be non-generic with a `Kind` discriminator?
- [ ] **URI canonicalization**: Should `file:///etc/foo` and `file:///etc/../etc/foo`
      resolve to the same resource? (Probably yes — filepath.Clean before URI creation.)
- [ ] **Immediate mode**: Do immediate receivers (non-plan) use resources at all,
      or only planned receivers? Immediate execution has no graph — there's nothing
      to shadow. Resources may be overkill for `file.exists(path="/tmp/foo")`.
- [ ] **Namespace scope**: One namespace per graph, or one per phase? Per-graph
      means phase boundaries don't reset visibility. Per-phase means phase A's
      writes aren't visible to phase B unless explicitly passed.
- [ ] **Tombstone ownership**: Should the executor own ALL tombstone logic, or
      should providers retain the option to handle their own compensation (e.g.,
      service.Stop → service.Start has no filesystem tombstone)?
- [ ] **Gather + resources**: When a gather produces N outputs at the same URI
      scheme, how does the namespace handle it? (Probably: each iteration gets
      a unique URI suffix.)
