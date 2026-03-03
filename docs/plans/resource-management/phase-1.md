# Phase 1: ResourceManager, NamespaceMap, and URI Helpers

## Context

Phase 1 adds the core resource-tracking types that all subsequent phases
depend on: the `ResourceManager` (append-only ledger), `NamespaceMap`
(URI→ResourceID with Resolve/Shadow), and URI helpers with scheme constants.

These are **pure additions** — no existing code changes, no existing tests
break. The existing `op.Resource` struct (`pkg/op/resource.go`) has three
fields (`URI`, `ID`, `OriginNodeID`) but no manager to assign IDs and no
namespace to track versions. `file.Resource` already embeds `op.Resource`
and sets `URI` as `file://<path>` — the pattern is established but not yet
connected to a ledger.

Phase 0 (`devlore-test` harness) shipped in commit `b2c7834`.

**Repo**: devlore-cli
**Branch**: `feature/resource-management`

## Changes

### URI Helpers (`pkg/op/resource.go`)

Scheme constants and a URI builder:

- `SchemeFile`, `SchemeGit`, `SchemePackage`, `SchemeService`, `SchemeMem`
- `ResourceURI(scheme, path) string` — for `file://`, canonicalizes via
  `filepath.Abs` + `filepath.Clean`; other schemes pass through as-is

### ResourceManager (`pkg/op/resource.go`)

Append-only ledger with mutex-guarded monotonic counter:

- `NewResourceManager() *ResourceManager`
- `EnsureCataloged(uri, originNodeID) string` — creates new entry, returns
  `res-<N>` ID. Does NOT deduplicate by URI (that's the NamespaceMap's job).
- `Lookup(id) (Resource, bool)` — retrieve by ID
- `LedgerLen() int` — entry count

### NamespaceMap (`pkg/op/namespace.go`)

URI→ResourceID mapping with version tracking:

- `NewNamespaceMap() *NamespaceMap`
- `Resolve(mgr, uri) string` — returns current ID for URI; catalogs
  discovery (OriginNodeID="") on first access
- `Shadow(mgr, uri, producerNodeID) string` — creates new version,
  updates namespace to point to it
- `Current(uri) string` — returns current ID or ""

### Tests

**`pkg/op/resource_test.go`**:
- `TestResourceURI_File` — canonicalization via Abs+Clean
- `TestResourceURI_OtherSchemes` — git, pkg, svc, mem pass through
- `TestResourceManager_EnsureCataloged` — monotonic IDs (res-1, res-2, ...)
- `TestResourceManager_Lookup` — correct Resource by ID; false for unknown
- `TestResourceManager_LedgerLen` — starts at 0, increments
- `TestResourceManager_ConcurrentAccess` — 50 goroutines, all unique IDs

**`pkg/op/namespace_test.go`**:
- `TestNamespace_Resolve_FirstAccess` — catalogs discovery, empty OriginNodeID
- `TestNamespace_Resolve_Idempotent` — same URI twice → same ID, 1 entry
- `TestNamespace_Shadow` — new version, 2 entries, correct OriginNodeID
- `TestNamespace_Shadow_OverwritesResolve` — Resolve after Shadow returns shadow's ID
- `TestNamespace_ImplicitDependency` — Shadow by nodeA → Resolve returns nodeA's resource
- `TestNamespace_Current_Empty` — unknown URI → ""
- `TestNamespace_Current_AfterResolve` — returns ID
- `TestNamespace_Current_AfterShadow` — returns new ID
- `TestNamespace_MultipleURIs` — independent URIs

## Files

| File | Action | Purpose |
|------|--------|---------|
| `docs/plans/resource-management/phase-1.md` | Create | This document |
| `pkg/op/resource.go` | Modify | Add ResourceManager, URI scheme constants, ResourceURI |
| `pkg/op/resource_test.go` | Create | Tests for ResourceManager and URI helpers |
| `pkg/op/namespace.go` | Create | NamespaceMap with Resolve/Shadow/Current |
| `pkg/op/namespace_test.go` | Create | Tests for namespace operations |

## Design Decisions

### Ledger Is the Sole Source of Truth (Decision #9)

Node annotations (`resource.input`, `resource.output`) were originally
planned for Phase 2c to tag nodes with resource IDs so the executor's
pre-flight pass could identify which nodes produce which resources.

**Decision**: Drop annotations entirely. The `ResourceManager` ledger
already records URI, origin node ID, and version lineage through
`Resolve`/`Shadow`. Annotations would duplicate what the ledger already
knows and cannot represent resources produced at runtime (globs, dynamic
template expansions, gather iterations). There should always be one source
of truth — the ledger is it.

**Impact on Phase 2**: Phase 2c (Resource Annotations on Nodes) is removed.
The executor's pre-flight pass (Phase 4) queries the ledger directly via
`OriginNodeID` to determine which resources were shadowed and by which
node.

## What This Does NOT Touch

- `graph.go` — Graph gets Resources/Namespace fields in Phase 2
- `output.go` — FillSlot gets resource-aware edges in Phase 2
- `provider/file/resource.go` — continues using `fmt.Sprintf` for now; updated in Phase 3
- No generated code changes
- No existing tests modified
