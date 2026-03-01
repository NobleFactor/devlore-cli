---
title: "Resource Management: URI-Based Resource Tracking for Providers"
status: draft
created: 2026-02-27
updated: 2026-03-01
---

# Plan: Resource Management

## Summary

Add a `ResourceManager` (append-only ledger) and `NamespaceMap`
(URI-to-resource-ID lookup with Resolve/Shadow) to the execution graph so that
providers track external state through typed resource handles instead of raw
strings. The file provider already implements the resource pattern — `op.Resource`
with URI/ID/OriginNodeID, `file.Resource` with filesystem metadata, constructor
registry coercion from string to Resource. This plan extends that foundation to
the graph, the executor, and all providers.

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

## Constraints

### No Code Generation Special-Casing

No special-casing in codegen. None. No edits to generated code. None. No
accommodations to the codegen tool (`star`). The reflection bridge
(`WrapReceiver`, `marshalReflect`, `coerceSlotValue`, constructor registry)
handles all type bridging at runtime. If the codegen tool or a
`star devlore` extension needs a change to support resource types, we make
that change in noblefactor-ops as a separate PR — we do not work around it
in devlore-cli.

Generated `*.gen.go` files are outputs of the pipeline. They are regenerated,
never hand-edited. If a generated file doesn't compile after a provider
signature change, the fix is in the generator or the provider — never in the
generated file.

### Test Coverage for Every Provider and Resource Type

Every provider that gains resource parameters gets tests. Every resource type
that we create gets tests. No exceptions.

- **Resource types**: Each new resource type (`git.Resource`, `service.Resource`,
  `pkg.Resource`) gets unit tests for `NewResource`, `Exists`, constructor
  registration, and URI generation.
- **Provider methods**: Every provider method that changes signature gets its
  existing tests updated to pass `Resource` values. If no test exists, one is
  created.
- **Constructor registry**: Each registered constructor gets a round-trip test:
  `string → Resource → verify fields`.
- **Integration**: At least one end-to-end test per provider verifying the full
  path: Starlark script → planned receiver → graph node → executor dispatch →
  provider method → result.

### Exit Criteria

Test coverage for every provider and every resource type is **>85%**. This
includes graph execution — not just unit tests of provider methods in isolation.

The test suite must demonstrate the full lifecycle:

1. **Plan a graph in Starlark** — a `.star` script calls planned receiver
   methods (`plan.file.write_text`, `plan.file.copy`, etc.) and builds a graph
   with nodes, edges, and resource identity.
2. **Execute the graph** — the executor runs the planned graph, dispatching
   through the action layer to provider methods, creating and verifying
   resources.
3. **Verify compensation** — when a node fails mid-graph, completed nodes
   compensate in LIFO order, restoring previous state via tombstones.

This requires the **`star devlore test`** extension (see Phase 0 below) that
becomes the primary tool for testing graph planning and graph execution. The
extension replaces ad-hoc Go-only graph construction in tests with `.star`
scripts that exercise the same code paths as production Starlark plans.

## Current State

What exists today, grounded in code:

| Component | Status | Code |
| --- | --- | --- |
| `op.Resource` | Implemented | `pkg/op/resource.go:6` — `{URI, ID, OriginNodeID}` |
| `file.Resource` | Implemented | `pkg/op/provider/file/resource.go:25` — embeds `op.Resource` + Inode, Device, Size, Mode, ModTime, Checksum |
| `file.Tombstone` | Implemented | `pkg/op/provider/file/resource.go:36` — `{RecoveryPath, OriginalPath}` |
| Constructor registry | Implemented | `pkg/op/marshal.go:23` — `RegisterConstructor`, `Construct`, `constructorRegistry` sync.Map |
| String→Resource coercion | Implemented | `pkg/op/provider/file/resource.go:14` — `init()` registers `string → file.Resource` via `NewResource` |
| Coercion chain | Implemented | `pkg/op/action_reflect.go:82` — nil → assignable → convertible → map→struct → constructor → error |
| `NewResource` (discovery) | Implemented | `pkg/op/provider/file/resource.go:47` — `os.Stat` + `checksumFile` + `syscall.Stat_t` |
| `RefreshMetadataWith` | Implemented | `pkg/op/provider/file/resource.go:146` — post-write metadata update |
| `prepareWrite` | Implemented | `pkg/op/provider/file/provider.go:818` — discovery + preemptive recovery |
| Same-partition recovery | Implemented | `pkg/op/provider/file/recovery.go:13` + `recovery_unix.go:21` — `os.Rename` to UUID-keyed path |
| `RecoveryStack` (pkg/op) | Implemented | `pkg/op/recovery.go` — Do/Push/Unwind/Discard with reconcile hooks |
| Node slots | Working | `map[string]SlotValue` — immediate values or promises |
| Output/FillSlot | Working | `pkg/op/output.go:121` — routes Output/Gather/immediate/None into slots |
| Graph edges | Working | Created by FillSlot when it sees an Output |
| ResourceManager | **Missing** | No ledger, no ID generation, no EnsureCataloged |
| NamespaceMap | **Missing** | No Resolve/Shadow, no URI-to-ID tracking |
| Implicit edges | **Missing** | Only explicit Output passing creates edges |
| Conflict detection | **Missing** | Two nodes targeting same path = silent race |
| Executor tombstone layer | **Missing** | Only file provider has recovery; others have none |
| Other provider resources | **Missing** | Only file provider has a Resource type |

## Design Decisions (Resolved)

These were open questions in the original draft. The code has answered them.

### 1. Non-generic Resource with Embedding

**Decision**: `op.Resource` is non-generic. Provider-specific types embed it.

The code already implements this pattern: `file.Resource` embeds `op.Resource`
and adds `SourcePath`, `Inode`, `Device`, `Size`, `Mode`, `ModTime`, `Checksum`.
Go generics don't allow `[]Resource[T]` for mixed `T` in a single ledger.
The embedding pattern avoids this entirely — the ledger stores `op.Resource`
values and each provider's metadata is accessed through the concrete type.

### 2. URI Canonicalization

**Decision**: Yes. `filepath.Abs` + `filepath.Clean` before URI creation.

`NewResource` already calls `os.Stat` which resolves the path. The
`ResourceManager.EnsureCataloged` method will apply `filepath.Abs` +
`filepath.Clean` before constructing the URI to ensure `file:///etc/foo`
and `file:///etc/../etc/foo` resolve to the same resource.

### 3. Immediate Mode Passthrough

**Decision**: Immediate receivers use Resources via the constructor registry.
No namespace involvement — shadowing is planning-only.

The constructor registry (`file.Resource` init) already handles string→Resource
coercion for immediate calls. Immediate execution has no graph and nothing to
shadow. The `Exists(blob Resource)` method already works this way — Starlark
passes a string, the constructor converts it to a `file.Resource`, and the
method uses `blob.SourcePath`.

### 4. Per-Graph Namespace

**Decision**: One namespace per graph.

Phases are saga boundaries (compensation), not visibility boundaries. A write
in phase A should be visible to a read in phase B without explicit passing.
The namespace resets only when a new graph is created.

### 5. Split Tombstone Ownership

**Decision**: Executor owns the *decision* (when to tombstone). Provider owns
the *mechanism* (how to tombstone for its resource type).

The file provider retains `moveToRecovery` / `getRecoveryBase` /
`restoreFromRecovery` because same-device rename is a filesystem-specific
optimization. The executor's pre-flight pass decides *which* resources need
tombstones based on namespace analysis. Non-filesystem providers (service,
package) get tombstone-like behavior through their existing compensation
pairs — `service.Start` compensates via `service.Stop`, no file backup needed.

### 6. Gather + Resources (Deferred)

Each gather iteration creates nodes with unique slot values. URIs within a
gather body derive from per-iteration data and are naturally unique. If
collisions arise in practice, the namespace will report the conflict. No
special handling needed until a concrete case demands it.

## Implementation Phases

### Phase 0: `star devlore test` Extension

Build the test harness as a `star devlore` extension. The command
`star devlore test run --script <path>` plans a graph in Starlark, executes
it, and verifies expectations. This becomes THE tool for testing graph
planning and graph execution for the remainder of this plan and all future
work.

**Repo**: devlore-cli

#### The Problem

Today's test infrastructure has a gap between planning and execution:

- **Immediate binding tests** (`starcode/integration_test.go`) execute
  Starlark that calls provider methods directly. No graph, no nodes, no edges.
- **Planned binding tests** (`file/gen/integration_test.go`) are **skipped**
  (issues #170, #171). They build a graph via Starlark but never execute it.
- **Execution tests** (`execution/compensation_test.go`) build graphs in Go
  code, not Starlark. They execute and verify compensation, but the graph is
  hand-constructed — it doesn't prove the Starlark→Graph→Executor pipeline.
- **Builder tests** (`lore/builder_test.go`) plan via Starlark using lore
  package entry points, but verify graph structure only — they don't execute.

No test currently does the full loop: **Starlark plan → graph → execute →
verify results → verify compensation**.

#### Extension Structure

The extension follows the same pattern as the existing four extensions in
`star/extensions/`:

```
star/extensions/com.noblefactor.devlore.Test/
├── extension.yaml
└── commands/
    └── run.star
```

**extension.yaml**:

```yaml
extension: com.noblefactor.devlore.Test
description: Test harness for Starlark graph planning and execution

receivers:
  - name: test
    builtin: true
    type: TestReceiver
    description: Graph planning, execution, and expectation verification
  - name: ui
    builtin: true
    type: UiReceiver
    description: Status output (note, warn, success, fail)

commands:
  - name: devlore.test.run
    help: Run a Starlark test script that plans and executes a graph
    implementation: commands/run.star
    flags:
      - name: script
        type: string
        required: true
        help: Path to the test script (.star)
      - name: provider
        type: string
        default: ""
        help: "Restrict to a specific provider (default: all)"
      - name: dry-run
        type: bool
        default: "false"
        help: Execute in dry-run mode (plan only, no side effects)
```

#### TestReceiver

A builtin Go receiver that wraps `BindingSet` (planned receivers) +
`GraphExecutor`. It implements the full plan→execute→verify pipeline as
a single `test.run()` call.

```go
// TestReceiver implements the star devlore test harness.
// It loads a test script, sets up the plan namespace via BindingSet,
// executes the script to build a graph, runs the graph through
// GraphExecutor, and checks expectations registered during script
// execution.
type TestReceiver struct {
    // ...
}
```

`test.run(script, opts)` does the following:

1. Creates a temp directory for filesystem operations
2. Sets up `BindingSet` with all planned receivers → `plan` namespace
3. Injects a `t` namespace into the script environment (see below)
4. Executes the test script — `plan.*` calls build graph nodes,
   `t.expect_*` calls register post-execution expectations
5. Executes the graph through `GraphExecutor`
6. Checks all queued expectations against actual results
7. Returns a result struct with pass/fail, assertion details, node count

#### The `t` Namespace

The test script receives a `t` global with:

- `t.tmp(relative)` — returns an absolute path under the test's temp
  directory. All file operations should target paths under `t.tmp()`.
- `t.expect_file(path, content=None)` — registers an expectation that
  `path` exists after execution. If `content` is provided, also checks
  file contents match.
- `t.expect_no_file(path)` — registers an expectation that `path` does
  NOT exist after execution (for testing compensation/removal).
- `t.expect_node_count(n)` — registers an expectation on the graph's
  node count.
- `t.expect_error(pattern)` — registers an expectation that execution
  fails with an error matching `pattern`.

Expectations are **queued** during script execution and **checked** after
graph execution. This avoids the plan-vs-verify sequencing problem — the
script is sequential Starlark, but planning and verification are clearly
separated by the `plan.*` vs `t.expect_*` convention.

#### The Star Command

The command (`commands/run.star`) is a thin wrapper:

```starlark
def run(ctx):
    result = test.run(
        script = ctx.args["script"],
        provider = ctx.args.get("provider", ""),
        dry_run = ctx.args.get("dry-run", False),
    )
    if result.passed:
        success(
            "All expectations met ("
            + str(result.expectation_count) + " expectations, "
            + str(result.node_count) + " nodes)"
        )
    else:
        for f in result.failures:
            error(f.expectation + ": " + f.message)
        fail(str(len(result.failures)) + " expectation(s) failed")
```

#### Test Script Convention

Test scripts live in `testdata/` directories. They follow a naming
convention: `test_<scenario>.star`.

```starlark
# testdata/test_write_and_read.star
#
# Plans a write followed by a read of the same path.
# Verifies that the executor runs them in the correct order
# and the file has the expected content.

dest = t.tmp("foo.txt")

# Planning phase — these calls build graph nodes
plan.file.write_text(destination=dest, content="hello", mode=0o644)
plan.file.read(path=dest)

# Expectations — checked after graph execution
t.expect_file(dest, content="hello")
t.expect_node_count(2)
```

```starlark
# testdata/test_compensation.star
#
# Plans a write followed by a node that will fail.
# Verifies that compensation restores the original state.

dest = t.tmp("recover_me.txt")
plan.file.write_text(destination=dest, content="original", mode=0o644)
# ... node that fails ...

t.expect_no_file(dest)  # compensation should have restored pre-write state
t.expect_error(".*")    # execution should fail
```

#### Un-skip Planned Binding Tests

Fix the skipped tests at `pkg/op/provider/file/gen/integration_test.go`
(issues #170, #171). These are exercised by the harness through test
scripts that use the file provider's planned receivers.

#### Baseline Coverage

Before any resource management work begins, the harness proves the existing
infrastructure works end-to-end:

- `test_write_text.star` — plan file.write_text → execute → expect file
  with correct content
- `test_copy.star` — plan file.copy → execute → expect destination matches
  source
- `test_write_and_read.star` — plan write then read on same path → execute
  → expect correct ordering and content
- `test_compensation.star` — plan write + failing node → execute → expect
  compensation restored original state

These are the baseline tests. Every subsequent phase adds test scripts for
its new functionality.

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `star/extensions/com.noblefactor.devlore.Test/extension.yaml` | Create | Extension spec with TestReceiver and run command |
| `star/extensions/com.noblefactor.devlore.Test/commands/run.star` | Create | Thin wrapper calling `test.run()` |
| `internal/starlark/test_receiver.go` | Create | `TestReceiver` — BindingSet + GraphExecutor + expectation checking |
| `internal/starlark/test_receiver_test.go` | Create | Unit tests for TestReceiver |
| `internal/starlark/testdata/test_write_text.star` | Create | Baseline: write text |
| `internal/starlark/testdata/test_copy.star` | Create | Baseline: copy file |
| `internal/starlark/testdata/test_write_and_read.star` | Create | Baseline: write then read |
| `internal/starlark/testdata/test_compensation.star` | Create | Baseline: write + fail → compensate |
| `pkg/op/provider/file/gen/integration_test.go` | Modify | Un-skip #170, #171 |

### Phase 1: ResourceManager and NamespaceMap

Pure additions. No existing code changes. The `op.Resource` struct already
exists — this phase adds the manager that assigns IDs, the ledger that tracks
versions, and the namespace that resolves/shadows URIs.

**Repo**: devlore-cli

#### 1a. ResourceManager

Extend `pkg/op/resource.go` with the manager. The existing `Resource` struct
gains automatic ID assignment via the manager.

```go
// ResourceManager owns the append-only ledger of all resources created
// during a single planning session. One per Graph.
type ResourceManager struct {
    mu     sync.Mutex
    ledger []Resource  // Append-only; index = sequence number
    nextID int         // Monotonic counter for ID generation
}

// EnsureCataloged creates a new Resource in the ledger with a unique ID.
// The URI is canonicalized (filepath.Abs + filepath.Clean for file:// URIs).
// Returns the assigned resource ID.
func (m *ResourceManager) EnsureCataloged(uri string, originNodeID string) string

// Lookup returns the Resource with the given ID, or false if not found.
func (m *ResourceManager) Lookup(id string) (Resource, bool)

// LedgerLen returns the number of resources in the ledger.
func (m *ResourceManager) LedgerLen() int
```

#### 1b. NamespaceMap

Create `pkg/op/namespace.go` with Resolve and Shadow.

```go
// NamespaceMap maps URIs to the most recent resource ID during planning.
type NamespaceMap struct {
    current map[string]string // URI → resource ID
}

// NewNamespaceMap creates an empty namespace.
func NewNamespaceMap() *NamespaceMap

// Resolve returns the current resource ID for a URI. If the URI has never
// been seen, it catalogs a discovery in the manager (originNodeID = "")
// and returns the new ID. If the URI was previously shadowed, returns the
// shadowed version's ID.
func (ns *NamespaceMap) Resolve(mgr *ResourceManager, uri string) string

// Shadow creates a new resource version in the manager, updates the
// namespace to point to it, and returns the new resource ID. The new
// resource's OriginNodeID is set to producerNodeID.
func (ns *NamespaceMap) Shadow(mgr *ResourceManager, uri string, producerNodeID string) string

// Current returns the resource ID currently mapped to a URI, or "" if
// the URI has never been resolved or shadowed.
func (ns *NamespaceMap) Current(uri string) string
```

#### 1c. URI Helpers

Add URI scheme constants and a builder to `resource.go`:

```go
const (
    SchemeFile    = "file"
    SchemeGit     = "git"
    SchemePackage = "pkg"
    SchemeService = "svc"
    SchemeMem     = "mem"
)

// ResourceURI builds a canonicalized URI from a scheme and path.
// For file:// URIs, the path is resolved via filepath.Abs + filepath.Clean.
func ResourceURI(scheme, path string) string
```

#### 1d. Tests

- Ledger append, ID monotonicity
- Namespace Resolve: first access catalogs discovery, second returns same ID
- Namespace Shadow: creates new version, subsequent Resolve returns shadowed
- Implicit dependency detection: Shadow by node A, then Resolve by node B →
  returned resource has `OriginNodeID = A`
- URI canonicalization: `file:///etc/../etc/foo` → `file:///etc/foo`
- Concurrent access safety (ResourceManager is mutex-protected)

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/resource.go` | Modify | Add ResourceManager, URI helpers |
| `pkg/op/resource_test.go` | Create | Unit tests for manager and URI helpers |
| `pkg/op/namespace.go` | Create | NamespaceMap with Resolve/Shadow |
| `pkg/op/namespace_test.go` | Create | Unit tests for namespace operations |

### Phase 2: Graph Integration

Wire ResourceManager and NamespaceMap into the Graph lifecycle. After this
phase, planned receivers can call Resolve/Shadow during planning, and FillSlot
creates implicit edges from resource identity.

**Repo**: devlore-cli

#### 2a. Graph Owns the Manager and Namespace

Add fields to `Graph` (`pkg/op/graph.go:20`):

```go
type Graph struct {
    // ... existing fields ...
    Resources *ResourceManager  // Ledger of all resources (nil for legacy graphs)
    Namespace *NamespaceMap     // URI → current resource ID (nil for legacy graphs)
}
```

Initialize both in `NewGraph` (`graph.go:39`):

```go
func NewGraph(tool string) *Graph {
    return &Graph{
        // ... existing fields ...
        Resources: NewResourceManager(),
        Namespace: NewNamespaceMap(),
    }
}
```

#### 2b. FillSlot Detects Resource Identity

Extend `FillSlot` (`output.go:121`) to handle the case where a slot value
carries resource identity. When an `Output`'s producer node has a resource
annotation (`resource.output`), the consumer node gets an implicit edge
even without explicit Output passing.

The current flow is unchanged — `Output` still creates edges via
`output.FillSlot`. The new behavior adds a check: if a Starlark value
resolves to a `file.Resource` (or any type embedding `op.Resource`), and
that resource's `OriginNodeID` is non-empty, create an edge from the origin
node to the consumer.

This requires adding a resource-aware path to `FillSlot`:

```go
func FillSlot(node *Node, graph *Graph, slotName string, value starlark.Value) error {
    // ... existing Output/Gather/None cases ...

    // Resource identity: if the immediate value embeds op.Resource with
    // a non-empty OriginNodeID, create an implicit edge.
    var goVal any
    if err := Unmarshal(value, &goVal); err != nil {
        return fmt.Errorf("slot %q: %w", slotName, err)
    }
    if res, ok := extractResource(goVal); ok && res.OriginNodeID != "" {
        graph.Edges = append(graph.Edges, Edge{From: res.OriginNodeID, To: node.ID})
    }
    node.SetSlotImmediate(slotName, goVal)
    return nil
}
```

`extractResource` uses reflection to check if the value embeds `op.Resource`.

#### 2c. Resource Annotations on Nodes

Add annotation helpers so planned receivers can tag nodes with resource IDs:

```go
const (
    AnnotationResourceInput  = "resource.input"   // Comma-separated input resource IDs
    AnnotationResourceOutput = "resource.output"   // Output resource ID
)
```

These annotations are metadata — they don't affect execution, but they enable
the executor's pre-flight binding pass (Phase 4) to know which nodes produce
which resources.

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/graph.go` | Modify | Add Resources/Namespace fields, init in NewGraph |
| `pkg/op/output.go` | Modify | Resource-aware FillSlot with implicit edges |
| `pkg/op/resource.go` | Modify | Add `extractResource` reflection helper, annotation constants |
| `pkg/op/graph_test.go` | Modify | Tests for resource fields on Graph |
| `pkg/op/output_test.go` | Modify | Tests for implicit edge creation in FillSlot |

### Phase 3: File Provider — Complete Input Migration

Finish migrating all file provider method inputs to `Resource`. The outputs
are already `Resource` for Copy, WriteText, WriteBytes, and Read. This phase
converts the remaining string inputs. The constructor registry ensures
backward compatibility — Starlark scripts passing string paths still work
because `coerceSlotValue` calls the registered `string → file.Resource`
constructor.

**Repo**: devlore-cli

#### Current Signatures vs. Target

| Method | Current Input | Target Input | Output (unchanged) |
| --- | --- | --- | --- |
| `Copy` | `Resource, string, FileMode` | `Resource, Resource, FileMode` | `Resource` |
| `WriteText` | `string, string, FileMode` | `Resource, string, FileMode` | `Resource` |
| `WriteBytes` | `string, string, FileMode` | `Resource, string, FileMode` | `Resource` |
| `Read` | `string` | `Resource` | `Resource` |
| `Exists` | `Resource` | No change | `bool` |
| `Link` | `string, string` | `Resource, Resource` | `Resource` |
| `Move` | `string, string` | `Resource, Resource` | `Resource` |
| `Backup` | `string, string` | `Resource, string` | `Resource` |
| `Remove` | `string, bool, string` | `Resource, bool, string` | `Tombstone` |
| `RemoveAll` | `string, bool, string` | `Resource, bool, string` | `Tombstone` |
| `Unlink` | `string, bool, string` | `Resource, bool, string` | `Tombstone` |

The pattern: every parameter identifying an external entity (a path) becomes
`Resource`. Configuration values (mode, prune, pruneBoundary, backupSuffix)
stay unchanged.

#### Migration Strategy

Each method change follows the same steps:

1. Change the parameter type from `string` to `Resource`
2. Replace internal `path` usage with `param.SourcePath`
3. For compensable methods returning `string`: change return to `Resource`
4. Update the corresponding `params.gen.go` parameter names (via code gen)
5. Update tests

For methods that currently return `string` (Link, Move, Backup): the result
becomes `Resource` with discovery metadata from the destination path. This
is the same pattern Copy already uses — `prepareWrite` returns a `Resource`.

#### Output Migration for Link, Move, Backup

These three methods currently return `(string, map[string]any, error)`. The
result `string` is the destination path. They change to return
`(Resource, map[string]any, error)`, calling `NewResource` on the destination
after the operation succeeds:

```go
// Move: before
func (p *Provider) Move(source, destination string) (result string, undo map[string]any, err error)

// Move: after
func (p *Provider) Move(source, destination Resource) (result Resource, undo map[string]any, err error)
```

#### Tests

Every method that changes signature gets its tests updated. Specifically:

- **Go-level**: Each compensable method tested with Resource input (Copy, Link,
  Move, Backup, Remove, RemoveAll, Unlink, WriteText, WriteBytes). Each
  non-compensable method tested with Resource input (Read, Exists).
- **Constructor round-trip**: `string → file.Resource → verify SourcePath,
  URI, Inode, Size, Checksum`.
- **Coercion path**: Starlark string → `coerceSlotValue` → `file.Resource`
  (verifies the constructor registry fires for the new parameter positions).
- **Compensation round-trip**: Forward with Resource input → compensate →
  verify original state restored.

#### Code Generation

Generated files (`gen/*.gen.go`) are regenerated via `star devlore actions
generate`. No hand edits. If the generator doesn't handle the new signatures,
the fix is in the generator (noblefactor-ops), not in the generated output.

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider/file/provider.go` | Modify | Convert all method inputs to Resource |
| `pkg/op/provider/file/resource.go` | Modify | Any helpers needed for the migration |
| `pkg/op/provider/file/provider_test.go` | Modify | Update all test call sites |
| `pkg/op/provider/file/gen/*.gen.go` | Regenerate | No hand edits — regenerate via `star` |

### Phase 4: Executor Tombstone Layer

Extract the tombstone *decision* from the file provider to the executor.
The provider retains the *mechanism* (same-device rename via
`moveToRecovery`/`restoreFromRecovery`). The executor's pre-flight pass
uses namespace analysis to determine which resources need tombstones.

**Repo**: devlore-cli

#### 4a. ResourceTombstone Type

Create `pkg/op/tombstone.go` as a peer of `resource.go`:

```go
// ResourceTombstone records pre-write state for a resource that will be
// overwritten by a graph node. Created by the executor's pre-flight pass.
type ResourceTombstone struct {
    ResourceID string // Which resource was shadowed
    URI        string // Logical address
    // Provider-specific recovery data. For file resources, this is
    // file.Tombstone{RecoveryPath, OriginalPath}. For non-filesystem
    // resources, this carries the state needed for compensation.
    Recovery any
}
```

#### 4b. Executor Pre-Flight Binding

Before executing any node, the executor scans the graph's resource ledger.
For each resource with a non-empty `OriginNodeID` whose URI matches a
previously discovered resource, the executor:

1. Calls the provider's recovery mechanism (e.g., `moveToRecovery` for files)
2. Stores a `ResourceTombstone` keyed by resource ID
3. On rollback, restores from the tombstone

This replaces the per-method `prepareWrite` pattern. Currently, every write
method in the file provider calls `prepareWrite` internally. After this phase,
write methods receive a pre-cleared destination — the executor has already
backed up what was there.

#### 4c. File Provider Simplification

`prepareWrite` (`provider.go:818`) becomes a thin wrapper that the executor
calls, not each method internally. The method flow changes from:

```
method → prepareWrite → discovery + backup → write
```

to:

```
executor pre-flight → prepareWrite (once per shadowed URI)
executor dispatch → method → write (no backup logic)
```

The file provider's `moveToRecovery`, `getRecoveryBase`, `findMountPoint`,
`restoreFromRecovery` remain unchanged — they are the *mechanism*. Only the
call site moves from inside each method to the executor.

#### 4d. Backward Compatibility

During the transition, the executor detects whether a graph has a
`ResourceManager` (non-nil `graph.Resources`). Legacy graphs without resources
skip the pre-flight pass, and providers retain their existing `prepareWrite`
behavior. This allows mixed-mode operation.

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/tombstone.go` | Create | `ResourceTombstone` type |
| `internal/execution/executor.go` | Modify | Pre-flight binding pass |
| `pkg/op/provider/file/provider.go` | Modify | Remove `prepareWrite` from method bodies |
| `pkg/op/provider/file/recovery.go` | Modify | Expose `moveToRecovery`/`restoreFromRecovery` as public API for executor |
| `internal/execution/executor_test.go` | Modify | Tests for pre-flight tombstone creation |
| `pkg/op/provider/file/provider_test.go` | Modify | Verify methods work without internal prepareWrite |

### Phase 5: Remaining Providers and Code Generation

Migrate remaining providers and update the code generator. Each provider
creates a resource type embedding `op.Resource` and registers a constructor.

**Repo**: devlore-cli + noblefactor-ops (templates)

#### 5a. Provider Resource Types

Each provider that manages external state creates its own resource type:

```go
// pkg/op/provider/git/resource.go
type Resource struct {
    op.Resource
    URL    string
    Commit string
    Branch string
}

// pkg/op/provider/service/resource.go
type Resource struct {
    op.Resource
    Name   string
    Status string // "running", "stopped", "enabled", "disabled"
}

// pkg/op/provider/pkg/resource.go
type Resource struct {
    op.Resource
    Name    string
    Version string
    Manager string
}
```

Each registers a constructor in `init()` following the file provider pattern:

```go
func init() {
    op.RegisterConstructor(func(v any) (Resource, error) {
        s, ok := v.(string)
        if !ok {
            return Resource{}, fmt.Errorf("service.Resource: expected string name, got %T", v)
        }
        return NewResource(s)
    })
}
```

#### 5b. Provider Method Migration

| Provider | Resource params | Config params (unchanged) |
| --- | --- | --- |
| git | url, path → `git.Resource` | ref, branch |
| net | url → `file.Resource` (downloaded to filesystem) | (none) |
| pkg | packages → `pkg.Resource` | manager, cask |
| service | name → `service.Resource` | (none) |
| template | source, path → `file.Resource` | templateData, project |
| archive | source, prefix → `file.Resource` | (none) |
| encryption | source → `file.Resource` | decryptor |
| shell | No change (commands are strings, not external state) | command |

Note: `net`, `template`, `archive`, and `encryption` all produce files, so
they use `file.Resource`. They don't need their own resource types.

#### 5c. Tests for Every Resource Type

Each new resource type gets a dedicated test file:

| Type | Test File | Test Coverage |
| --- | --- | --- |
| `git.Resource` | `pkg/op/provider/git/resource_test.go` | `NewResource`, constructor round-trip (`string → git.Resource`), URI generation (`git://...`), field population |
| `service.Resource` | `pkg/op/provider/service/resource_test.go` | `NewResource`, constructor round-trip (`string → service.Resource`), URI generation (`svc://...`), field population |
| `pkg.Resource` | `pkg/op/provider/pkg/resource_test.go` | `NewResource`, constructor round-trip (`string → pkg.Resource`), URI generation (`pkg://...`), field population |

Each provider that changes method signatures gets updated tests:

| Provider | Test Coverage |
| --- | --- |
| git | Clone, Pull with `git.Resource` input; compensation round-trip |
| service | Start, Stop, Enable, Disable, Restart with `service.Resource` input; compensation round-trip |
| pkg | Install, Remove, Upgrade with `pkg.Resource` input; compensation round-trip |
| net | Download with `file.Resource` output; no resource input (URL is a string) |
| template | Render with `file.Resource` input/output |
| archive | Extract with `file.Resource` input/output |
| encryption | DecryptSopsFile with `file.Resource` input/output |

#### 5d. Code Generation Updates

No special-casing in codegen. No hand edits to generated files. If the
generator needs changes to support resource types in planned/immediate
receivers, those changes are made in noblefactor-ops as separate PRs to the
`star devlore` tool or its extensions.

The reflection bridge already handles Resource types through the constructor
registry and `coerceSlotValue`. The generated code dispatches through the
same bridge — no template changes are needed unless the bridge itself is
insufficient (in which case the bridge is fixed, not the templates).

Generated `*.gen.go` files are regenerated via `star devlore actions generate`
after provider signatures change. That is the only interaction with codegen.

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider/git/resource.go` | Create | `git.Resource` type + constructor |
| `pkg/op/provider/git/resource_test.go` | Create | Resource type tests |
| `pkg/op/provider/service/resource.go` | Create | `service.Resource` type + constructor |
| `pkg/op/provider/service/resource_test.go` | Create | Resource type tests |
| `pkg/op/provider/pkg/resource.go` | Create | `pkg.Resource` type + constructor |
| `pkg/op/provider/pkg/resource_test.go` | Create | Resource type tests |
| `pkg/op/provider/git/provider.go` | Modify | Resource params |
| `pkg/op/provider/git/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/service/provider.go` | Modify | Resource params |
| `pkg/op/provider/service/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/pkg/provider.go` | Modify | Resource params |
| `pkg/op/provider/pkg/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/net/provider.go` | Modify | Resource params (uses `file.Resource`) |
| `pkg/op/provider/net/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/template/provider.go` | Modify | Resource params (uses `file.Resource`) |
| `pkg/op/provider/template/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/archive/provider.go` | Modify | Resource params (uses `file.Resource`) |
| `pkg/op/provider/archive/provider_test.go` | Modify | Updated tests |
| `pkg/op/provider/encryption/provider.go` | Modify | Resource params (uses `file.Resource`) |
| `pkg/op/provider/*/gen/*.gen.go` | Regenerate | No hand edits — regenerate via `star` |

## Migration Path

0. **Phase 0**: Build the `star devlore test` extension. Un-skip planned
   binding tests. Establish baseline coverage: plan → execute → verify for
   file provider. This extension is used by every subsequent phase.
1. **Phase 1**: Pure additions. `ResourceManager` and `NamespaceMap` exist
   alongside the current Graph/Node/Slot model. No existing tests break.
   Test scripts verify manager/namespace don't interfere with existing flow.
2. **Phase 2**: Graph gains Resources/Namespace fields. `FillSlot` gains
   resource-aware edge creation. Legacy graphs (nil Resources) work unchanged.
   Test scripts verify implicit edge creation via shadowing.
3. **Phase 3**: File provider completes its input migration. Constructor
   registry ensures Starlark scripts passing strings still work. Generated
   code regenerated. Harness tests verify every file provider method with
   Resource inputs, including compensation round-trips.
4. **Phase 4**: Executor gains pre-flight tombstone pass. File provider's
   internal `prepareWrite` calls migrate to the executor. Legacy graphs
   without resources skip the pre-flight pass. Test scripts verify tombstone
   creation and restoration via executor.
5. **Phase 5**: Remaining providers + code generation. Each provider is
   independently testable. Test scripts for every provider that gains
   resource parameters. The system runs in mixed mode throughout — resource
   and raw-type providers coexist.

## Files to Create/Modify

| File | Phase | Action | Purpose |
| --- | --- | --- | --- |
| `star/extensions/com.noblefactor.devlore.Test/` | 0 | Create | Extension spec + star command |
| `internal/starlark/test_receiver.go` | 0 | Create | TestReceiver (plan + execute + verify) |
| `internal/starlark/test_receiver_test.go` | 0 | Create | TestReceiver unit tests |
| `internal/starlark/testdata/test_*.star` | 0 | Create | Baseline test scripts |
| `pkg/op/provider/file/gen/integration_test.go` | 0 | Modify | Un-skip #170, #171 |
| `pkg/op/resource.go` | 1 | Modify | ResourceManager, URI helpers |
| `pkg/op/resource_test.go` | 1 | Create | Manager and URI tests |
| `pkg/op/namespace.go` | 1 | Create | NamespaceMap |
| `pkg/op/namespace_test.go` | 1 | Create | Namespace tests |
| `pkg/op/graph.go` | 2 | Modify | Resources/Namespace fields |
| `pkg/op/output.go` | 2 | Modify | Resource-aware FillSlot |
| `pkg/op/provider/file/provider.go` | 3 | Modify | Complete input migration |
| `pkg/op/provider/file/provider_test.go` | 3 | Modify | Updated test calls |
| `pkg/op/tombstone.go` | 4 | Create | ResourceTombstone type |
| `internal/execution/executor.go` | 4 | Modify | Pre-flight binding pass |
| `pkg/op/provider/file/recovery.go` | 4 | Modify | Public API for executor |
| `pkg/op/provider/git/resource.go` | 5 | Create | git.Resource |
| `pkg/op/provider/service/resource.go` | 5 | Create | service.Resource |
| `pkg/op/provider/pkg/resource.go` | 5 | Create | pkg.Resource |
| `pkg/op/provider/*/provider.go` | 5 | Modify | Resource params |

## Relationship to Reconciliation

The [Audit, Reconciliation, and Recovery](../architecture/devlore-audit-reconciliation-and-recovery.md)
plan depends on resource management. Reconciliation adds a 4th return value
(`ReconciliationState`) to `Action.Do` — a fingerprint of the resource at
completion. That fingerprint *is* the resource's post-write metadata (hash,
version, status). Resource management provides the identity model;
reconciliation provides the drift detection.

Resource management must complete before reconciliation begins. The two plans
touch the same files in conflicting ways — resources change input types,
reconciliation changes output arity. Sequential execution avoids merge
conflicts in `action.go`, every `provider.go`, and `executor.go`.

## Debugging Strategy: JetBrains + Starlark

The `star devlore test` extension must be debuggable in GoLand (or IntelliJ
with the Go plugin). The goal: set a breakpoint in a `.star` test script and
step through into the graph builder and executor Go code.

### The Challenge

Starlark is interpreted by `go.starlark.net`. GoLand doesn't natively
understand `.star` breakpoints — it debugs Go code. But the interpreter
exposes `DebugFrame` with local variable access and source positions, giving
us the building blocks.

### Layer 1: Go Test Entry Point (Day 1)

The `TestReceiver` has a Go test file (`test_receiver_test.go`) with tests
that call `TestReceiver.Run()` directly. Run these in GoLand's debugger:

```go
func TestWriteAndRead(t *testing.T) {
    r := NewTestReceiver(/* ... */)
    result := r.Run("testdata/test_write_and_read.star", TestOptions{})
    // ...
}
```

Set breakpoints at:

| Breakpoint location | What you see |
| --- | --- |
| `PlannedReceiver.CallInternal` | Every `plan.file.*` call from Starlark |
| `FillSlot` (`output.go`) | Slot value assignment, edge creation |
| `GraphExecutor.runFlat` | Node-by-node execution |
| `reflectedAction.Do` | Go method dispatch with coerced args |
| `coerceSlotValue` (`action_reflect.go`) | String→Resource constructor coercion |
| `Provider.WriteText` (etc.) | The actual provider method |
| `moveToRecovery` / `restoreFromRecovery` | Tombstone creation and restoration |

This gives full visibility into the graph builder and executor from day 1.

### Layer 2: Starlark Trace Mode (Phase 0)

Add a `--trace` flag to `star devlore test run`. When enabled, the
`TestReceiver` installs a step callback on the Starlark `Thread` that logs
each statement with file, line, and local variables:

```
[trace] test_write_and_read.star:4  dest = t.tmp("foo.txt")
        dest = "/tmp/test-xxx/foo.txt"
[trace] test_write_and_read.star:7  plan.file.write_text(...)
        → node: file.WriteText #1
[trace] test_write_and_read.star:8  plan.file.read(...)
        → node: file.Read #2, edge: #1→#2
[trace] test_write_and_read.star:11 t.expect_file(dest, content="hello")
        → expectation queued: file_exists(/tmp/test-xxx/foo.txt)
[exec]  Executing graph: 2 nodes, 1 edge
[exec]  Node #1 file.WriteText → OK
[exec]  Node #2 file.Read → OK
[verify] file_exists(/tmp/test-xxx/foo.txt) → PASS
[verify] file_content("hello") → PASS
```

The step callback uses `thread.CallStack()` for position and
`thread.DebugFrame(0)` for local variable inspection. The trace output
goes to `ui.note()` so it appears in the star command's output.

In GoLand, combine trace mode with a conditional breakpoint on the step
callback — break when `pos.Filename` and `pos.Line` match a specific
`.star` location. This gives you "breakpoints in Starlark" via Go.

### Layer 3: Bazel Plugin Starlark Support

The [Bazel for IntelliJ](https://plugins.jetbrains.com/plugin/8609-bazel-for-jetbrains)
plugin provides Starlark language support: syntax highlighting, code
navigation, and structure view for `.star` / `.bzl` files. Install it for
`.star` editing comfort.

The plugin also includes a Starlark debug adapter, but it speaks Bazel's
debug protocol (not go.starlark.net). To bridge this gap, the `TestReceiver`
can optionally expose a **DAP (Debug Adapter Protocol)** server using
`DebugFrame`:

- `--debug-port <port>` flag on `star devlore test run`
- The receiver starts a DAP server before executing the script
- GoLand connects to the DAP server as a "Remote Debug" run configuration
- Breakpoints set in `.star` files are honored via the step callback
- Variable inspection uses `DebugFrame.Local(i)` to report Starlark locals

This is a stretch goal — Layer 1 + Layer 2 provide full debuggability
without it. But if we want `.star` breakpoints in the GoLand gutter, DAP
is the path. The `DebugFrame` API provides everything DAP needs:

| DAP capability | go.starlark.net API |
| --- | --- |
| Set breakpoint (file:line) | Step callback checks `CallStack().Position()` |
| Continue / Step Over / Step In | Step callback state machine |
| Inspect locals | `DebugFrame(0).Local(i)` |
| Call stack | `thread.CallStack()` |
| Evaluate expression | `starlark.Eval(thread, expr, locals)` |

### Implementation Priority

| Layer | When | Effort | Value |
| --- | --- | --- | --- |
| Go test entry point | Phase 0 (built-in) | None — it's the test file | Full Go-side debugging |
| Trace mode (`--trace`) | Phase 0 | Small — step callback + formatting | Starlark execution visibility |
| DAP server (`--debug-port`) | Post-Phase 5 | Medium — DAP protocol impl | Native `.star` breakpoints in GoLand |

Layers 1 and 2 ship with Phase 0. Layer 3 is a follow-on after the core
resource management work is complete.

## Related Documents

- [Resource Management Architecture](../architecture/devlore-resource-management.md)
- [Binding Unification Plan](./binding-unification.md) — Provider binding architecture
- [Compensation Plan](./compensation.md) — Compensation/recovery architecture
- [Audit, Reconciliation, and Recovery](../architecture/devlore-audit-reconciliation-and-recovery.md)
