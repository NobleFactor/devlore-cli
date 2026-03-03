---
title: "Resource Management: URI-Based Resource Tracking for Providers"
status: draft
created: 2026-02-27
updated: 2026-03-03
---

# Plan: Resource Management

## Summary

Add a `ResourceManager` (append-only ledger) and `NamespaceMap`
(URI-to-resource-ID lookup with Resolve/Shadow) to the execution graph so that
providers track external state through typed resource handles instead of raw
strings. The file provider already implements the resource pattern — `op.Resource`
with URI/ID/OriginNodeID, `file.Resource` with filesystem metadata. This plan
extends that foundation to the graph, the executor, and all providers. Resource
coercion (e.g., `string → file.Resource`) moves from provider init-time
registration to the executor's planner boundary — providers always receive their
own typed Resource, possibly in an unresolved state.

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
| Constructor registry | Implemented (moving) | `pkg/op/marshal.go:23` — `RegisterConstructor`, `Construct`, `constructorRegistry` sync.Map. Per Decision #7, ownership moves from provider init() to the executor's coercion layer. |
| String→Resource coercion | Implemented (moving) | `pkg/op/provider/file/resource.go:14` — `init()` registers `string → file.Resource` via `NewResource`. Will move to executor boundary. |
| Coercion chain | Implemented | `pkg/op/action_reflect.go:82` — nil → assignable → convertible → map→struct → constructor → error |
| `NewResource` (discovery) | Implemented | `pkg/op/provider/file/resource.go:47` — `os.Stat` + `checksumFile` + `syscall.Stat_t` |
| `RefreshMetadataWith` | Implemented | `pkg/op/provider/file/resource.go:146` — post-write metadata update |
| `prepareWrite` | Implemented | `pkg/op/provider/file/provider.go:818` — discovery + preemptive recovery |
| Same-partition recovery | Implemented | `pkg/op/provider/file/recovery.go:13` + `recovery_unix.go:21` — `os.Rename` to UUID-keyed path |
| `RecoveryStack` (pkg/op) | Implemented | `pkg/op/recovery.go` — Do/Push/Unwind/Discard with reconcile hooks |
| Node slots | Working | `map[string]SlotValue` — immediate values or promises |
| Output/FillSlot | Working | `pkg/op/output.go:121` — routes Output/Gather/immediate/None into slots |
| Graph edges | Working | Created by FillSlot when it sees an Output |
| ResourceManager | Implemented | `pkg/op/resource.go` — append-only ledger, `EnsureCataloged`/`Lookup`/`LedgerLen`, mutex-guarded monotonic `res-<N>` IDs |
| NamespaceMap | Implemented | `pkg/op/namespace.go` — `Resolve`/`Shadow`/`Current`, URI→ResourceID mapping |
| URI helpers | Implemented | `pkg/op/resource.go` — `SchemeFile`/`SchemeGit`/`SchemePackage`/`SchemeService`/`SchemeMem`, `ResourceURI()` with file path canonicalization |
| Implicit edges | **Missing** | Only explicit Output passing creates edges |
| Conflict detection | **Missing** | Two nodes targeting same path = silent race |
| Executor tombstone layer | **Missing** | Only file provider has recovery; others have none |
| Other provider resources | **Missing** | Only file provider has a Resource type |
| Provider lifecycle | **Gap** | Providers are zero-value structs (`&Provider{}`). No constructor, no context injection. Context arrives per-call via `op.Context` in `Do()`. See Decision #8. |

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

### 7. Resource Coercion at the Planner/Executor Boundary

**Decision**: Coercion from raw values (strings) to typed Resources is an
executor concern, not a provider concern. Providers always receive their own
typed `Resource` — possibly unresolved (path exists but not yet stat'd) or
pending (destination that doesn't exist yet), but always typed.

#### What Is: Current Coercion Chain

The coercion chain spans three time boundaries, with type knowledge locked
in Go reflection:

**Registration time** — Provider `init()` registers constructors:

```
file/resource.go:init()
    → op.RegisterConstructor(func(v any) (file.Resource, error) { ... })
    → constructorRegistry sync.Map: reflect.Type(file.Resource) → func
```

**Plan time** — `buildPlannedBridge` stores raw Starlark values in slots:

```
plan.file.copy(source_file="src.txt", destination_filename="dst.txt")
    → starlark.UnpackArgs → vals[0] = starlark.String("src.txt")
    → FillSlot(node, graph, "source_file", starlark.String("src.txt"))
    → Unmarshal → node.SetSlotImmediate("source_file", "src.txt")  // string
```

No type checking. No Resource creation. Strings stay as strings.

**Execution time** — `reflectedAction.Do()` coerces slots to method types:

```
reflectedAction.Do(ctx, slots{"source_file": "src.txt", ...})
    → paramType := method.Type.In(1)  // file.Resource (via reflection)
    → coerceSlotValue("src.txt", file.Resource)
        Level 1: nil?                No
        Level 2: assignable?         No (string ≠ file.Resource)
        Level 3: convertible?        No
        Level 4: map → struct?       No
        Level 5: constructor?        YES → NewResource("src.txt")
            → os.Stat("src.txt") → inode, size, checksum, mode, modtime
            → file.Resource{SourcePath: "src.txt", Inode: ..., ...}
    → Provider.Copy receives file.Resource
```

**Generated code carries NO type metadata.** `MethodParams` is
`map[string][]string` — just param names. The 4-file codegen pattern:

| File | Type info | What it does |
| --- | --- | --- |
| `params.gen.go` | None — names only | `MethodParams{"Copy": {"source_file", "destination_filename", ...}}` |
| `actions.gen.go` | Implicit — Go reflection | `RegisterReflectedActions` reflects on `Provider` struct at runtime |
| `planned.gen.go` | None | `WrapPlanned` → `buildPlannedBridge` → `FillSlot` stores raw values |
| `immediate.gen.go` | Implicit — Go reflection | `WrapReceiver` → `buildMethodBridge` → `unmarshalValue` at call time |

**Problems with current design:**

1. **Late coercion** — `os.Stat` discovery happens inside `coerceSlotValue`
   during execution. Errors surface at execution time, not planning time.
2. **No cross-provider coercion** — the constructor registry maps
   `target_type → func(any)`. There's no path from `mem.Resource` →
   `file.Resource` — only `string → file.Resource`.
3. **Scattered registration** — each provider's `resource.go:init()`
   registers its constructor independently. The executor has no visibility
   into what coercions are available.
4. **No type validation at plan time** — passing an `int` to a Resource
   slot silently stores it. The error surfaces in `coerceSlotValue` during
   execution.

#### What Will Be: Planner Coerces, Executor Resolves

Two distinct operations, cleanly separated:

- **Coercion** (plan time): Type-tagging. `string → file.Resource{URI,
  state=unresolved}`. Pure — no I/O, no `os.Stat`. The planner knows
  *what type* a slot value should be.
- **Resolution** (execution time): Metadata population. `file.Resource
  {unresolved} → file.Resource{resolved, inode, size, checksum}`. I/O
  against the *target machine*. The executor knows *what exists*.

This separation is forced by a hard constraint: **a graph can be planned
once and executed on many machines**. A graph can target the local host or
any number of remote machines. `os.Stat` at plan time gives the planning
machine's metadata, not the target's. `/etc/nginx/nginx.conf` has different
inode, size, and checksum on every machine. The planner must be pure.

**Registration time** — the registry evolves from a constructor registry
to a **coercion table** that maps `(source_type, target_type)` pairs:

```
// Current: target_type → constructor (does I/O via NewResource → os.Stat)
constructorRegistry: reflect.Type(file.Resource) → func(any) (any, error)

// Target: (source_type, target_type) → coercion_func (pure, no I/O)
coercionTable:
    (string, file.Resource)        → file.ResourceFromPath(s)  // URI only, no stat
    (string, service.Resource)     → service.ResourceFromName(s)
    (string, pkg.Resource)         → pkg.ResourceFromName(s)
    (mem.Resource, file.Resource)  → file.ResourceFromMem(m)
```

**Plan time** — `buildPlannedBridge` coerces slot values to typed
Resources using the coercion table. This requires `buildPlannedBridge` to
know param types — either via `reflect.Method` (passed from `WrapPlanned`)
or via enriched `MethodParams`. The result is typed but metadata-empty:

```
plan.file.copy(source_file="src.txt", destination_filename="dst.txt")
    → buildPlannedBridge knows: param 0 expects file.Resource
    → coercionTable.Coerce("src.txt", file.Resource)
        → file.Resource{URI: "file:///src.txt", state: unresolved}  // no os.Stat
    → namespace.Resolve("file:///src.txt") → resource-1 (cataloged, unresolved)
    → node.SetSlotImmediate("source_file", file.Resource{...})
```

The graph now contains typed Resources in its slots. The ledger contains
URIs and relationships. No metadata. Portable across machines.

**Executor pre-flight** (NEW, per execution target) — resolves all
unresolved resources against the target machine before any node runs:

```
for each entry in resourceManager.Ledger():
    if entry.State == unresolved:
        metadata := resolve(entry)  // os.Stat on target machine
        if err:
            fail-fast: "source file not found: /etc/foo"
        entry.State = resolved
        entry.Metadata = metadata
    // pending entries (outputs) skip — they don't exist yet
```

This is a flat iteration over the ledger — O(unique source URIs), not a
graph traversal. The namespace deduplicates by URI: if 5 nodes reference
the same source path, `namespace.Resolve()` returns the same resource ID,
and pre-flight stat's it once.

**Execution time** — `reflectedAction.Do()` receives typed, resolved
Resources. `coerceSlotValue` Level 2 (assignable) matches immediately.
Level 5 (constructor) never fires. Pending Resources (outputs) are
populated by node results and flow downstream via promise resolution.

**Example flow:**

```
PLAN (pure, no I/O):
    plan.file.copy("source.txt", "dest.txt")
        → slot: source_file = file.Resource{URI: "file:///src.txt", state: unresolved}
        → slot: destination_filename = "dst.txt"  (string, not a Resource)
        → ledger: [resource-1: file:///src.txt, unresolved]

EXECUTE on machine A:
    pre-flight:
        → resolve resource-1 against machine A
        → os.Stat("/src.txt") → inode=42, size=1024, checksum=abc...
        → resource-1.State = resolved, resource-1.Metadata = {...}

    reflectedAction.Do():
        → slot source_file: file.Resource{resolved} → Level 2: assignable ✓
        → Provider.Copy receives (file.Resource, string, os.FileMode)

EXECUTE on machine B (same graph, different target):
    pre-flight:
        → resolve resource-1 against machine B
        → os.Stat("/src.txt") → inode=99, size=1024, checksum=def...
        → resource-1.State = resolved, resource-1.Metadata = {...}  // different inode, checksum
```

#### Ledger Structure

The ledger is an append-only collection keyed by resource ID. The
`NamespaceMap` deduplicates by URI — multiple nodes referencing the same
path share a single ledger entry. Multiple entries per URI exist only from
shadowing (a write creates a new version).

**Three resource states:**
- **Unresolved**: Source input — the external entity should exist but
  hasn't been stat'd. URI and type are set, metadata is empty. Created by
  `namespace.Resolve()` when the planner encounters a source path.
  Resolved by the executor's pre-flight pass against the target machine.
- **Pending**: Output — the external entity does not yet exist. URI and
  type are set, metadata is empty. Created by `namespace.Shadow()` when
  the planner encounters a destination. Resolved by the node's execution
  result — the provider populates metadata after creating the entity.
- **Resolved**: Metadata populated. For files: inode, device, size, mode,
  modtime, checksum. For services: status. For packages: version.

```
Ledger (plan-time skeleton — portable):
  resource-1: URI=file:///src.txt,  state=unresolved, origin=""
  resource-2: URI=file:///dst.txt,  state=pending,    origin=copy-node

Namespace:
  file:///src.txt → resource-1
  file:///dst.txt → resource-2

After pre-flight on target machine (execution-scoped):
  resource-1: URI=file:///src.txt,  state=resolved, metadata={inode:42, ...}
  resource-2: URI=file:///dst.txt,  state=pending   (still pending — not yet created)

After copy-node executes:
  resource-2: URI=file:///dst.txt,  state=resolved, metadata={inode:87, ...}
```

#### Codegen Tool Changes

Three layers of change, in dependency order:

**Layer 1: Action interface — `SlotTypes()` (devlore-cli, no codegen)**

Add `SlotTypes() map[string]reflect.Type` to the `Action` interface.
`reflectedAction` implements it by reflecting on `method.Type.In(i+1)` for
each param name. `WrapPlanned` passes the `reflect.Method` to
`buildPlannedBridge` so it can coerce at plan time.

```go
func (a *reflectedAction) SlotTypes() map[string]reflect.Type {
    types := make(map[string]reflect.Type, len(a.paramNames))
    for i, name := range a.paramNames {
        types[name] = a.method.Type.In(i + 1) // skip receiver
    }
    return types
}
```

**Layer 2: Coercion table (devlore-cli, replaces constructor registry)**

Evolve `constructorRegistry` in `marshal.go` to a coercion table. Two
kinds of entries:

- **Plan-time coercions** (pure, no I/O): `string → file.Resource{URI
  only}`, `mem.Resource → file.Resource`. Called by `buildPlannedBridge`.
- **Execution-time resolvers**: `file.Resource{unresolved} → file.Resource
  {resolved}`. Called by the executor's pre-flight pass. These do I/O
  (os.Stat, service status check, package version query).

**Layer 3: `MethodParams` type evolution (devlore-cli + noblefactor-ops)**

Extend `MethodParams` to carry type metadata:

```go
// Current:
type MethodParams map[string][]string

// Target:
type ParamSpec struct {
    Name       string
    TypeName   string // Go type as string: "string", "file.Resource", "os.FileMode"
    IsResource bool   // true if type embeds op.Resource
}
type MethodParams map[string][]ParamSpec
```

This requires changes to:

| Component | Repo | Change |
| --- | --- | --- |
| `MethodParams` type | devlore-cli `pkg/op/receiver_reflect.go` | `[]string` → `[]ParamSpec` |
| `params.go.template` | devlore-cli `star/extensions/.../templates/` | Emit `ParamSpec` structs |
| `generate.star` | devlore-cli `star/extensions/.../commands/` | Detect Resource types, set `IsResource` |
| `go.type_embeds()` | noblefactor-ops GoReceiver | New introspection: does type T embed type U? |
| `WrapPlanned` | devlore-cli `pkg/op/planned_reflect.go` | Pass `reflect.Method` to `buildPlannedBridge` for plan-time coercion |
| `WrapReceiver` | devlore-cli `pkg/op/receiver_reflect.go` | Extract param names from `ParamSpec` |
| `RegisterReflectedActions` | devlore-cli `pkg/op/action_reflect.go` | Extract param names from `ParamSpec` |

**Layer 4: Provider constructors (devlore-cli + codegen)**

Per Decision #8, every provider needs a constructor that accepts context.
The generated binding code must change from zero-value struct creation to
constructor calls:

| Component | Repo | Change |
| --- | --- | --- |
| `actions.gen.go` | devlore-cli (generated) | `&provider.Provider{}` → `provider.New(ctx)` |
| `immediate.gen.go` | devlore-cli (generated) | `&provider.Provider{}` → `provider.New(ctx)` |
| `graph_actions.go.template` | devlore-cli templates | Emit constructor call with context |
| `immediate_receiver.go.template` | devlore-cli templates | Emit constructor call with context |
| `ProviderBinding` | devlore-cli `pkg/op/binding.go` | `ActionRegistrar` and `ImmediateFactory` closures receive context |
| Each `provider.go` | devlore-cli `pkg/op/provider/*/` | Add `func New(ctx) *Provider` constructor |

#### `generate.star` Changes

`build_method_descriptors()` already receives param type strings from
`go.methods()` AST introspection. It currently stores `p.type` but only
uses it for struct_param and callable detection. Changes:

1. **Resource detection** — for each param, check if its Go type embeds
   `op.Resource`. This requires a new `go.type_embeds(path, type_name,
   target_type)` introspection method on the GoReceiver (noblefactor-ops).
   Alternatively, maintain a known-Resource-types list in the generator,
   since all Resource types are defined in devlore-cli and their names are
   predictable (`file.Resource`, `git.Resource`, `service.Resource`,
   `pkg.Resource`).

2. **Param descriptor annotation** — add `is_resource: true` to param
   descriptors for Resource-typed params. The `params.go.template` uses
   this to emit `IsResource: true` in the generated `ParamSpec`.

3. **Plan-time type name** — include the Go type name in the param
   descriptor so `params.go.template` can emit `TypeName: "file.Resource"`.

#### Implementation Order

| Step | What | Where | Depends on |
| --- | --- | --- | --- |
| 1 | `Action.SlotTypes()` + pass `reflect.Method` to planned bridge | devlore-cli `pkg/op/` | Nothing |
| 2 | Coercion table (plan-time coercions + execution-time resolvers) | devlore-cli `pkg/op/marshal.go` | Step 1 |
| 3 | Plan-time coercion in `buildPlannedBridge` | devlore-cli `pkg/op/planned_reflect.go` | Steps 1-2 |
| 4 | Executor pre-flight resolution pass | devlore-cli `internal/execution/` | Step 2 |
| 5 | `go.type_embeds()` | noblefactor-ops GoReceiver | Nothing |
| 6 | `ParamSpec` type + template | devlore-cli + templates | Step 5 |
| 7 | `generate.star` annotations | devlore-cli extension | Steps 5-6 |
| 8 | Provider constructors + context injection | devlore-cli providers + templates | Steps 6-7 |

Steps 1-4 can ship independently (reflection-only, no codegen changes).
Steps 5-7 ship together (codegen pipeline update).
Step 8 ships after codegen — it changes every provider and every generated
binding.

### 9. Ledger Is the Sole Source of Truth — No Node Annotations

**Decision**: The `ResourceManager` ledger is the single source of truth
for resource identity and lineage. Node annotations (`resource.input`,
`resource.output`) are eliminated.

The ledger already records URI, origin node ID, and version lineage through
`EnsureCataloged` (called by `NamespaceMap.Resolve` and `Shadow`).
Annotations would duplicate information the ledger already holds and cannot
represent resources produced at runtime — globs returning N files, dynamic
template expansions, gather iterations producing unknown resource sets.

The executor's pre-flight pass (Phase 4) queries the ledger directly:
resources with non-empty `OriginNodeID` whose URI matches a previously
discovered resource need tombstones. No annotation scanning required.

**Impact**: Phase 2c (Resource Annotations on Nodes) is removed from the
plan. Phase 2 consists of 2a (Graph owns Manager and Namespace) and 2b
(FillSlot detects resource identity). The `extractResource` reflection
helper moves to `resource.go` without annotation constants.

### 8. Provider Lifecycle: Singletons with Context Injection

**Decision**: Providers are singletons. In a graph, a provider follows the
lifetime of the graph. In a Starlark script, a provider follows the lifetime
of the script. Every provider needs context, and context is provided at
construction time — every provider needs a constructor that accepts a
context object by reference.

**Current state** — Providers are zero-value structs with no constructor:

```go
// Generated actions.gen.go — zero-value, no context
op.RegisterReflectedActions(reg, "pkg", &provider.Provider{}, Params)

// Generated immediate.gen.go — zero-value, no context
NewPkgReceiver(&provider.Provider{})
```

Context arrives per-call via `op.Context` in `action.Do(ctx, slots)`. The
provider cannot access Platform, Writer, or Data at construction time.
Providers that need platform info (pkg, service) take manager arguments on
every method call or call `host.NewHost()` on every invocation.

**Target state** — Every provider has a constructor that accepts context:

```go
// Target — context-injected singleton
p := provider.New(ctx)
op.RegisterReflectedActions(reg, "pkg", p, Params)
NewPkgReceiver(p)
```

The provider holds a reference to the context and reads from it for its
entire lifetime. For the platform provider, this means reading
`ctx.Platform` directly — the provider is just a Starlark surface for
Platform data. For file, pkg, and service providers, this means accessing
Platform's managers without method-level arguments.

**Codegen impact**: The generated `actions.gen.go` and `immediate.gen.go`
files currently create `&provider.Provider{}`. These must change to call a
provider constructor with context. The `ProviderBinding` struct's
`ActionRegistrar` and `ImmediateFactory` closures both need access to a
context parameter. This affects every generated binding for every provider.

**Relationship to Design Decision #7**: Context injection and resource
coercion are complementary. The provider receives context at construction
(Decision #8) and receives typed Resources in its method params (Decision
#7). Together, they eliminate both the "providers can't access platform"
problem and the "providers receive raw strings" problem.

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

#### Architecture

`devlore-test` is a **separate binary** (`cmd/devlore-test/`) with
process-level isolation, not an embedded receiver. It runs under `star`
command control via an extension that shells out to the binary (CLI args +
stdout + exit code). No noblefactor-ops spec changes — the `.star` command
resolves the binary path by convention.

```
star devlore test run --script path.star
        |
        v
   run.star (extension command)
        |  shells out to devlore-test binary
        v
   devlore-test --script path.star [--dry-run] [--trace]
        |
        |  1. Creates temp dir
        |  2. Sets up BindingSet -> plan namespace
        |  3. Injects `t` namespace (expectations)
        |  4. Executes .star script (plan.* builds graph)
        |  5. Wraps nodes in a Phase for RunPhased compensation
        |  6. Executes graph via GraphExecutor
        |  7. Checks queued expectations
        |  8. Writes structured results to stdout
        |  9. Exit code: 0 = pass, 1 = fail, 2 = error
        v
   run.star reads stdout, reports via ui.*
```

#### Extension Structure

```
star/extensions/com.noblefactor.devlore.Test/
├── extension.yaml
└── commands/
    └── run.star
```

**extension.yaml** declares the command with flags for `--script`,
`--dry-run`, `--trace`, and `--tool-path` (path to the `devlore-test`
binary, resolved via flag → env var → config default `build/devlore-test`).

#### Runner Package (`internal/e2e/testrunner/`)

Core orchestration lives in `internal/e2e/testrunner/`, sibling to the
existing LLM accuracy harness in `internal/e2e/`. The LLM harness tests
migrate/onboard via provider configs and F1 metrics. This package tests
the graph pipeline: Starlark plan → graph → execute → verify.

```go
type Runner struct {
    script   string
    dryRun   bool
    trace    bool
    provider string
    writer   io.Writer
}

func New(script string, opts ...Option) *Runner
func (r *Runner) Start(ctx context.Context) (*Result, error)
```

`Runner.Start()` does the following:

1. Creates a temp directory for filesystem operations
2. Creates `BindingSet` with `op.BindingConfig{}` + `.With("plan", "file")`
3. Creates `ActionRegistry` via `bs.NewPopulatedRegistry()`
4. Creates `Platform` via `platform.New()`
5. Creates `op.Graph("devlore-test")`
6. Builds Starlark globals from `bs.BuildGlobals(graph, project, reg)`
7. Injects a `t` namespace into the script environment (see below)
8. Configures Starlark `Thread` with trace handler
9. Executes the test script — `plan.*` calls build graph nodes,
   `t.expect_*` calls register post-execution expectations
10. Wraps all nodes in a single `phase.test` Phase for RunPhased
    compensation semantics (without phases, `runFlat` executes but does
    not unwind the recovery stack on failure)
11. Executes the graph through `GraphExecutor`
12. Checks all queued expectations against actual results
13. Returns a `Result` struct with pass/fail, assertion details, node count

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

The command (`commands/run.star`) resolves the `devlore-test` binary via a
3-tier lookup: `--tool-path` flag → `DEVLORE_TEST_TOOL_PATH` env var →
extension config `test.tool_path` (default: `build/devlore-test` relative
to the git worktree root). It shells out with flags, parses JSON stdout,
and reports via `ui.*`.

Note: Direct exec from Starlark requires a future `host.exec()` receiver
in noblefactor-ops. Until then, the `run.star` command documents the
intended flow but cannot shell out natively.

#### Stdout Protocol

JSON object on stdout:

```json
{
  "passed": true,
  "node_count": 3,
  "expectation_count": 2,
  "failures": [],
  "trace": ["script: test.star", "tmpdir: /tmp/...", "..."]
}
```

Exit codes: `0` = all pass, `1` = expectation failure, `2` = harness error.

#### Test Script Convention

Test scripts live in `internal/e2e/testrunner/data/`. They follow a naming
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
| `cmd/devlore-test/main.go` | Create | Binary entry point (flags, JSON output, exit codes) |
| `internal/e2e/testrunner/runner.go` | Create | Runner orchestration (BindingSet + GraphExecutor + phase wrapping) |
| `internal/e2e/testrunner/runner_test.go` | Create | Go-level test entry points (GoLand-debuggable) |
| `internal/e2e/testrunner/test_context.go` | Create | `t` namespace (Starlark receiver for expectations) |
| `internal/e2e/testrunner/trace.go` | Create | Starlark trace mode (step callback + variable inspection) |
| `internal/e2e/testrunner/data/test_write_text.star` | Create | Baseline: write text |
| `internal/e2e/testrunner/data/test_copy.star` | Create | Baseline: copy file |
| `internal/e2e/testrunner/data/test_write_and_read.star` | Create | Baseline: write then read |
| `internal/e2e/testrunner/data/test_compensation.star` | Create | Baseline: write + fail → compensate |
| `star/extensions/com.noblefactor.devlore.Test/extension.yaml` | Create | Extension spec with run command |
| `star/extensions/com.noblefactor.devlore.Test/commands/run.star` | Create | Star command (shells out to devlore-test binary) |
| `Makefile` | Modify | Add devlore-test build target |
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
carries resource identity. When a slot value embeds `op.Resource` with a
non-empty `OriginNodeID` (set by `NamespaceMap.Shadow`), the consumer node
gets an implicit edge from the origin node — even without explicit Output
passing. The ledger is the sole source of truth (Decision #9); no node
annotations are needed.

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

#### Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/graph.go` | Modify | Add Resources/Namespace fields, init in NewGraph; remove dead `backup` annotation check and `BackedUp` summary field |
| `pkg/op/output.go` | Modify | Resource-aware FillSlot with implicit edges |
| `pkg/op/resource.go` | Modify | Add `extractResource` reflection helper |
| `pkg/op/graph_test.go` | Modify | Tests for resource fields on Graph |
| `pkg/op/output_test.go` | Modify | Tests for implicit edge creation in FillSlot |

### Phase 3: File Provider — Complete Input Migration

Finish migrating all file provider method inputs to `Resource`. The outputs
are already `Resource` for Copy, WriteText, WriteBytes, and Read. This phase
converts the remaining string inputs. Per Design Decision #7, the executor's
coercion layer converts strings to typed `file.Resource` values at the
planner/executor boundary — provider methods always receive `file.Resource`,
either resolved (existing file with metadata) or pending (destination path
with empty metadata).

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

Each registers a resource constructor in `init()` following the file
provider pattern:

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

Each provider also gains a provider constructor per Decision #8:

```go
// New creates a Provider with context injected at construction time.
// The provider is a singleton — one per graph or script lifetime.
func New(ctx *op.Context) *Provider {
    return &Provider{ctx: ctx}
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
3. **Phase 3**: File provider completes its input migration. The executor's
   coercion layer converts strings to `file.Resource` at the planner boundary
   (Design Decision #7). Generated code regenerated. Harness tests verify
   every file provider method with Resource inputs, including compensation
   round-trips.
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
| `cmd/devlore-test/main.go` | 0 | Create | Binary entry point |
| `internal/e2e/testrunner/runner.go` | 0 | Create | Runner orchestration (plan + execute + verify) |
| `internal/e2e/testrunner/runner_test.go` | 0 | Create | Go-level test entry points |
| `internal/e2e/testrunner/test_context.go` | 0 | Create | `t` namespace |
| `internal/e2e/testrunner/trace.go` | 0 | Create | Starlark trace mode |
| `internal/e2e/testrunner/data/test_*.star` | 0 | Create | Baseline test scripts |
| `star/extensions/com.noblefactor.devlore.Test/` | 0 | Create | Extension spec + star command |
| `Makefile` | 0 | Modify | Add devlore-test build target |
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

The `devlore-test` harness must be debuggable in GoLand (or IntelliJ
with the Go plugin). The goal: set a breakpoint in a `.star` test script and
step through into the graph builder and executor Go code.

### The Challenge

Starlark is interpreted by `go.starlark.net`. GoLand doesn't natively
understand `.star` breakpoints — it debugs Go code. But the interpreter
exposes `DebugFrame` with local variable access and source positions, giving
us the building blocks.

### Layer 1: Go Test Entry Point (Day 1)

The runner package has a Go test file (`runner_test.go`) with tests
that call `Runner.Start()` directly. Run these in GoLand's debugger:

```go
func TestWriteAndRead(t *testing.T) {
    runner := testrunner.New(filepath.Join(testdataDir, "test_write_and_read.star"))
    result, err := runner.Start(context.Background())
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

The `--trace` flag on `devlore-test` enables the `Tracer` which records
each Starlark statement with file, line, and local variables via the
thread's print handler:

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

The trace uses `thread.CallStack()` for position and
`thread.DebugFrame(0)` for local variable inspection. The trace output
appears in the JSON result's `trace` array.

In GoLand, combine trace mode with a conditional breakpoint on the
`Tracer.RecordThread` method — break when position matches a specific
`.star` location. This gives you "breakpoints in Starlark" via Go.

### Layer 3: Bazel Plugin Starlark Support

The [Bazel for IntelliJ](https://plugins.jetbrains.com/plugin/8609-bazel-for-jetbrains)
plugin provides Starlark language support: syntax highlighting, code
navigation, and structure view for `.star` / `.bzl` files. Install it for
`.star` editing comfort.

The plugin also includes a Starlark debug adapter, but it speaks Bazel's
debug protocol (not go.starlark.net). To bridge this gap, the `Runner`
can optionally expose a **DAP (Debug Adapter Protocol)** server using
`DebugFrame`:

- `--debug-port <port>` flag on `devlore-test`
- The runner starts a DAP server before executing the script
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
