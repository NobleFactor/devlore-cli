---
title: "Resource Management: URI-Based Resource Tracking for Providers"
status: active
created: 2026-02-27
updated: 2026-03-05
---

# Plan: Resource Management

## Summary

Track external state through typed resource handles instead of raw strings.
`op.Resource` is a sealed interface backed by `ResourceBase` (URI + identity
fields). `ResourceCatalog` (append-only ledger + URI→ID namespace with
Resolve/Shadow) lives on the graph. All providers are fully migrated:
every method that identifies an external entity (file path, URL, package
name, service name) accepts typed Resource parameters. Typed Tombstones
replace `map[string]any` across all providers. The executor integrates
with the catalog for shadow/resolve during execution and pre-flight
resolution. Generated bridge tests auto-regenerate when signatures change.

Remaining gap: Phase 0 skipped tests (#164, #170, #171).

### Phase Status

| Phase | Status | Work Remaining |
| --- | --- | --- |
| 0 | ~85% done | Un-skip 13 tests: #170 (1 immediate binding), #171 (1 planned binding), #164 (11 recovery site tests — macOS SIP) |
| 1 | Done | — |
| 2 | Done | — |
| 3 | Done | — |
| 4 | Done | — |
| 5 | Done | — |
| 6 | Done | — |
| 7 | Done | — |
| 8 | Done | — |
| 9 | Done | — |
| 10 | Done | `Resolve()` is a skeleton — platform injection required for Type/Version population at execution time |
| 11 | Done | — |

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
| `op.Resource` interface | Implemented | `pkg/op/resource.go` — sealed interface with `URI()` + `resourceBase()` |
| `op.ResourceBase` | Implemented | `pkg/op/resource.go` — private `uri`, `id`, `producerID`; implements `starvalue.Marshaler` |
| `op.Tombstone` interface | Implemented | `pkg/op/resource.go` — sealed interface with `Resource()` + `tombstoneBase()` |
| `op.TombstoneBase` | Implemented | `pkg/op/resource.go` — holds `Resource`, value receivers (no post-construction mutation) |
| `op.Provider` interface | Implemented | `pkg/op/provider.go` — sealed with `providerBase() *ProviderBase` |
| `ResourceCatalog` | Implemented | `pkg/op/resource_catalog.go` — ledger + namespace (`Resolve`/`Shadow`/`Lookup`/`Current`) |
| `file.Resource` | Implemented | `pkg/op/provider/file/resource.go` — embeds `ResourceBase` + SourcePath, Inode, Device, Size, Mode, ModTime, Checksum |
| `file.Tombstone` | Implemented | `pkg/op/provider/file/resource.go` — embeds `TombstoneBase` + RecoveryPath |
| `file.Provider` | Implemented | All 19 methods accept `Resource` params; 9 compensable methods return typed `Tombstone`; `Root` is `Resource` |
| Constructor registry | Implemented | `pkg/op/starvalue_marshal.go` — `RegisterConstructor`, `Construct`, `constructorRegistry` sync.Map |
| String→Resource coercion | Implemented | `pkg/op/provider/file/resource.go:14` — `init()` registers `string → file.Resource` via `NewResource` |
| Coercion chain | Implemented | `pkg/op/action_reflect.go` — nil → assignable → convertible → map→struct → constructor → error |
| `starvalue` package | Implemented | `pkg/op/starvalue/starvalue.go` — `Marshaler`/`Unmarshaler` interfaces |
| Marshal implementation | Implemented | `pkg/op/starvalue_marshal.go` — `marshalReflect` checks `Marshaler` before reflection walk |
| `extractResource` | Implemented | `pkg/op/resource_catalog.go` — handles direct `Resource`, flat `map[string]any`, and nested `resource_base` forms |
| `FillSlot` implicit edges | Implemented | `pkg/op/output.go` — calls `extractResource`, creates edges from `producerID` |
| `Graph.Catalog` | Implemented | `pkg/op/graph.go` — `*ResourceCatalog`, initialized by `NewGraph()`, excluded from JSON/YAML |
| URI helpers | Implemented | `pkg/op/resource.go` — `SchemeFile`/`SchemeGit`/`SchemePackage`/`SchemeService`/`SchemeMem`, `ResourceURI()` |
| Same-partition recovery | Implemented | `pkg/op/provider/file/recovery.go` + `recovery_unix.go` — `os.Rename` to UUID-keyed path |
| `RecoveryStack` (pkg/op) | Implemented | `pkg/op/recovery.go` — Do/Push/Unwind/Discard with reconcile hooks |
| Test harness | Implemented | `internal/e2e/testrunner/` — Runner, TestContext (`t` namespace), Tracer; 4 baseline `.star` scripts pass |
| `devlore-test` binary | Implemented | `cmd/devlore-test/main.go` |
| Planned bridge catalog calls | Superseded | Original implementation: `resolveResourceParam` calls `catalog.Resolve` for inputs; `shadowOutputParam` shadows the last Resource param at plan time. **Current model** (see "Ledger Structure" §) shadows only at run time post-dispatch; plan time only catalogs inputs via `GetOrCreate`. |
| Executor pre-flight | Implemented | `preflight.go:ResolveResources` — iterates discovery URIs, stat checks file:// resources |
| Action layer catalog.Shadow | Implemented | `action_reflect.go:shadowResult` — shadows results after dispatch |
| Conflict detection | Implemented | `resource_catalog.go:Shadow()` returns error when two different origins target same URI |
| Other provider resources | Implemented | `net.Resource`, `git.Resource`, `service.Resource`, `pkg.Resource` all created with URI, constructors, tests |
| Provider lifecycle | Implemented | All providers embed `op.ProviderBase`, constructed via `op.NewProviderBase(ctx)` |
| Slice coercion | Implemented | `coerceSlice` in `action_reflect.go` enables `[]string` → `[]Resource` via constructors |
| CompensableAction validation | Implemented | `RegisterReflectedActions` panics on missing or orphaned compensators at registration time |
| Immediate mode catalog | Implemented | `ReflectedReceiver.SetCatalog` + `shadowResult` after dispatch in `receiver_reflect.go` |
| Skipped tests | **Gap** | `gen/integration_test.go` (#170, #171) and `gen/actions_test.go` (#164) still skipped |

## Design Decisions

Decisions 1–6 and 9 are implemented and code-grounded. Decisions 7 and 8
include aspirational design (coercion table, ParamSpec, provider
constructors with context injection) that is not yet implemented.
Decisions 10 and 11 have separate implementation plans in the
`resource-management/` subdirectory.

### 1. Non-generic Resource with Embedding

**Decision**: `op.Resource` is a sealed interface. Provider-specific types
embed `op.ResourceBase`.

`file.Resource` embeds `ResourceBase` and adds `SourcePath`, `Inode`,
`Device`, `Size`, `Mode`, `ModTime`, `Checksum`. Go generics don't allow
`[]Resource[T]` for mixed `T` in a single ledger. The interface pattern
avoids this — the ledger stores `op.Resource` values and each provider's
metadata is accessed through type assertion to the concrete type.

### 2. URI Canonicalization

**Decision**: Deferred to resolve time. Superseded by Decision #10.

The constructor stores the path as given — `filepath.Abs` and
`filepath.Clean` produce machine-specific results and cannot run at plan
time. URI canonicalization happens at `Resolve()` time on the target
machine, where the working directory and symlink layout are known.

### 3. Immediate Mode Passthrough

**Decision**: Immediate receivers use Resources via the constructor registry.
Catalog shadowing is optional — `ReflectedReceiver.SetCatalog` injects a
catalog, and `shadowResult` runs after dispatch when one is present. In
production, no catalog is wired in; the mechanism exists for testing and
future use.

The constructor registry (`file.Resource` init) already handles string→Resource
coercion for immediate calls. The `Exists(blob Resource)` method already works
this way — Starlark passes a string, the constructor converts it to a
`file.Resource`, and the method uses `blob.SourcePath`.

### 4. Per-Graph Namespace

**Decision**: One namespace per graph.

Phases are saga boundaries (compensation), not visibility boundaries. A write
in phase A should be visible to a read in phase B without explicit passing.
The namespace resets only when a new graph is created.

### 5. Split Tombstone Ownership

**Decision**: Executor owns the *decision* (when to tombstone). Provider owns
the *mechanism* (how to tombstone for its resource type).

Recovery logic has been extracted to `recovery.Site` (`pkg/op/recovery/site.go`),
a shared service on `op.Context.RecoverySite`. The Site provides `ArchiveFile` /
`RestoreFile` (zero-copy rename) and `ArchiveData` / `RestoreData` (byte
serialization). The executor's pre-flight pass decides *which* resources need
tombstones based on namespace analysis. Non-filesystem providers (service,
package) get tombstone-like behavior through their existing compensation
pairs — `service.Start` compensates via `service.Stop`, no file backup needed.

### 6. Gather + Resources (Deferred)

Each gather iteration creates nodes with unique slot values. URIs within a
gather body derive from per-iteration data and are naturally unique. If
collisions arise in practice, the namespace will report the conflict. No
special handling needed until a concrete case demands it.

### 7. Resource Coercion at the Planner/Executor Boundary

**Status**: "What Is" section is implemented. "What Will Be" section
(coercion table, `SlotTypes()`, `ParamSpec`, `go.type_embeds()`) is
aspirational design — not yet implemented.

**Decision**: Coercion from raw values (strings) to typed Resources is an
executor concern, not a provider concern. Providers always receive their own
typed `Resource` — Pending until pre-flight observes it on the target machine
(transitioning to Resolved on success, Unresolved on failure), but always
typed. See "Ledger Structure" § for the catalog state machine.

#### What Is: Current Coercion Chain

The coercion chain spans three time boundaries, with type knowledge locked
in Go reflection:

**Registration time** — Provider `init()` registers constructors:

```
file/resource.go:init()
    → op.RegisterConstructor(func(v any) (file.Resource, error) { ... })
    → constructorRegistry sync.Map: reflect.Type(file.Resource) → func
```

**Plan time** — `buildPlannedBridge` stores raw Starlark values in slots,
validates types, and resolves Resource URIs in the catalog:

```
plan.file.copy(source_file="src.txt", destination_filename="dst.txt")
    → starlark.UnpackArgs → vals[0] = starlark.String("src.txt")
    → FillSlot(node, graph, "source_file", starlark.String("src.txt"))
    → Unmarshal → node.SetSlotImmediate("source_file", "src.txt")  // string
    → validateSlotType("src.txt", file.Resource)  // passes (constructor exists)
    → resolveResourceParam → catalog.Resolve("file:///src.txt")
```

Type validation happens at plan time via `validateSlotType`. Resource URIs
are cataloged. But strings stay as strings in slots — coercion to typed
Resources still happens at execution time.

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
            → file.Resource{SourcePath: "src.txt"}  // no I/O (Decision #10)
    → Provider method calls Resolve() internally if it needs metadata
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

**Remaining problems with current design:**

1. **Late coercion** — Resource construction happens inside
   `coerceSlotValue` during execution. Slots store raw strings, not typed
   Resources. Constructor no longer does I/O (Decision #10), but coercion
   is still deferred to execution.
2. **No cross-provider coercion** — the constructor registry maps
   `target_type → func(any)`. There's no path from `mem.Resource` →
   `file.Resource` — only `string → file.Resource`.
3. **Scattered registration** — each provider's `resource.go:init()`
   registers its constructor independently. The executor has no visibility
   into what coercions are available.
4. ~~**No type validation at plan time**~~ — Resolved.
   `validateSlotType` in `buildPlannedBridge` validates that slot values
   can be coerced to the target type. Passing an `int` to a Resource slot
   now fails at plan time.

#### What Will Be: Planner Constructs, Executor Resolves

Two distinct operations, cleanly separated:

- **Construction** (plan time): Type-tagging. `string → file.Resource{URI}`,
  cataloged via `catalog.GetOrCreate`. Pure — no I/O, no `os.Stat`. The
  planner knows *what type* a slot value should be and what URI identifies
  it.
- **Resolution** (execution time): Metadata population. Pre-flight stats
  the target machine; post-dispatch `Shadow` appends output entries with
  metadata already populated. The executor knows *what exists*.

This separation is forced by a hard constraint: **a graph can be planned
once and executed on many machines**. A graph can target the local host or
any number of remote machines. `os.Stat` at plan time gives the planning
machine's metadata, not the target's. `/etc/nginx/nginx.conf` has different
inode, size, and checksum on every machine. The planner must be pure.

The catalog state model that supports this separation is documented in
"Ledger Structure" §: every entry is **Pending** at end of plan;
pre-flight transitions inputs to **Resolved** (stat succeeded) or
**Unresolved** (stat failed); post-dispatch `Shadow` appends output
entries born **Resolved**.

**Registration time** — the registry maps target Resource types to
constructor closures:

```
resourceConstructors: reflect.Type(file.Resource) → func(ctx, value any) (Resource, error)
                      reflect.Type(service.Resource) → ...
                      reflect.Type(pkg.Resource) → ...
```

**Plan time** — `NodeBuilder.fillSlot` constructs typed Resources from
string slot values via the registered constructor and interns them in the
catalog. The result is typed and cataloged but metadata-empty:

```
plan.file.copy(source_file="src.txt", destination_filename="dst.txt")
    → fillSlot knows: param 0 expects *file.Resource
    → factory := registry.resourceConstructor(*file.Resource)
    → catalog.GetOrCreate("file:///src.txt", factory)
        → first sighting: factory constructs *file.Resource{URI}, interns as Pending
        → returns canonical entry
    → node.SetSlotImmediate("source_file", *file.Resource{...})
```

The graph now contains typed Resources in its slots. The catalog contains
URIs and relationships. No metadata. Portable across machines.

**Executor pre-flight** (per execution target) — observes every Pending
discovery entry against the target machine before any node runs:

```
for each uri in catalog.DiscoveryURIs():     // entries with empty producerID
    entry := catalog.Lookup(catalog.Current(uri))
    if err := entry.Resolve(); err == nil {
        // metadata populated in place; entry transitions Pending → Resolved
    } else {
        // entry stays Pending → executor classifies it as Unresolved and halts (or
        // continues, per executor policy; consuming nodes will fail at dispatch).
    }
```

This is a flat iteration over the discovery URIs — O(unique source URIs),
not a graph traversal. The namespace deduplicates by URI: if 5 nodes
reference the same source path, the catalog has one entry and pre-flight
stat's it once.

**Execution time** — `Method.Invoke` receives typed, Resolved Resources
from slot values. `op.Convert`'s assignability path matches immediately
for Resource-typed parameters. Output Resources are returned by the
provider method and shadowed post-dispatch via `catalog.Shadow(result,
node.ID())`, born Resolved.

**Example flow:**

```
PLAN (pure, no I/O):
    plan.file.copy("source.txt", "dest.txt")
        → slot: source_file = *file.Resource{URI: "file:///src.txt"}
        → slot: destination_filename = "dst.txt"  (string, not a Resource)
        → catalog: resource-1: URI=file:///src.txt, state=Pending, producerID=""

EXECUTE on machine A:
    pre-flight:
        → resource-1.Resolve()
        → os.Stat("/src.txt") → inode=42, size=1024, checksum=abc...
        → resource-1: state=Resolved, metadata={inode:42, ...}

    Method.Invoke:
        → slot source_file: *file.Resource{Resolved} → assignable ✓
        → Provider.Copy receives (*file.Resource, string, os.FileMode)

    post-dispatch:
        → result is *file.Resource for /tmp/dst.txt with populated metadata
        → catalog.Shadow(result, copy-node-id)
        → catalog: resource-2: URI=file:///dst.txt, state=Resolved,
                   producerID="copy-node-id", metadata={inode:87, ...}

EXECUTE on machine B (same graph, different target):
    pre-flight:
        → resource-1.Resolve()
        → os.Stat("/src.txt") → inode=99, size=1024, checksum=def...
        → resource-1: state=Resolved, metadata={inode:99, ...}  // different inode, checksum
```

#### Ledger Structure

> **State model below is superseded.** The `Pending` / `Resolved` /
> `Unresolved` three-state model captured here was the original design.
> It was replaced by the **Pending / Active / Gone** model during
> 13.0(k) k.13 lifecycle integration. See
> `docs/architecture/4-resource-management.md` §3.1 (state definitions)
> and §6.2 (catalog behavior matrix) for the current authoritative
> spec. The ledger structure described below (append-only by resource
> ID, URI namespace, shadowing semantics) remains accurate; only the
> per-entry state names and transitions have changed.

The ledger is an append-only collection keyed by resource ID. The
`NamespaceMap` deduplicates by URI — multiple nodes referencing the same
path share a single ledger entry. Multiple entries per URI exist only from
shadowing (a write creates a new version).

**Three resource states.** A catalog entry's state is a function of (a)
whether observation has been attempted and (b) whether metadata has been
populated. Producer-vs-discovery (input vs output) is encoded in
`producerID` (empty for inputs, set for outputs), not in the state name.

- **Pending**: initial state. Every catalog entry is created here. URI
  and producerID are set; metadata is empty. No observation attempt has
  been made.
- **Resolved**: metadata populated. Reached by (a) pre-flight stat
  succeeding on the target machine for a discovery entry, or (b)
  post-dispatch `Shadow` appending an output entry with metadata already
  populated by the producer.
- **Unresolved**: pre-flight tried to stat the entry's URI and the
  observation failed (file does not exist on the target machine, network
  unreachable, etc.). Distinct from Pending: the catalog has tried.

State machine:

```
  Pending ──pre-flight stat fails──▶ Unresolved
     │
     └─────pre-flight stat succeeds──▶ Resolved

At run time, post-dispatch Shadow appends new entries born Resolved —
they do not transition through Pending.
```

**Snapshot points.**

- **End of plan**: every entry is Pending. The catalog contains only
  inputs (discoveries with empty producerID); outputs are not registered
  until run time.
- **End of pre-flight**: every plan-time entry is now Resolved or
  Unresolved. No entry remains Pending.
- **End of execution**: every entry is Resolved or Unresolved. New
  entries appended during execution by post-dispatch `Shadow` (outputs)
  or by run-time `GetOrCreate` (provider-internal interning) are born
  Resolved.

**Example trace.**

```
End of plan (catalog populated only with inputs):
  resource-1: URI=file:///src.txt, state=Pending, producerID=""

End of pre-flight on machine A:
  resource-1: URI=file:///src.txt, state=Resolved, producerID="",
              metadata={inode:42, size:1024, ...}

End of execution on machine A (after copy-node ran):
  resource-1: URI=file:///src.txt, state=Resolved, producerID="",
              metadata={inode:42, size:1024, ...}
  resource-2: URI=file:///dst.txt, state=Resolved, producerID="copy-node",
              metadata={inode:87, ...}     ← appended post-dispatch, born Resolved
```

If pre-flight on machine B could not find /src.txt, resource-1 would be
Unresolved at end of pre-flight, and the executor would halt before
running copy-node.

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

### 8. Provider Lifecycle: Singletons with Context Injection

**Status**: Partially implemented. Providers embed `op.ProviderBase` and
are constructed via `op.NewProviderBase(ctx)`. Context-injected
constructors (`provider.New(ctx)`) and codegen changes are not yet
implemented.

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

### 9. Ledger Is the Sole Source of Truth — No Node Annotations

**Decision**: The `ResourceManager` ledger is the single source of truth
for resource identity and lineage. Node annotations (`resource.input`,
`resource.output`) are eliminated.

The ledger already records URI, producer node ID, and version lineage
through `EnsureCataloged` (called by `NamespaceMap.Resolve` and `Shadow`).
Annotations would duplicate information the ledger already holds and cannot
represent resources produced at runtime — globs returning N files, dynamic
template expansions, gather iterations producing unknown resource sets.

The executor's pre-flight pass (Phase 4) queries the ledger directly:
resources with non-empty `producerID` whose URI matches a previously
discovered resource need tombstones. No annotation scanning required.

**Impact**: Phase 2c (Resource Annotations on Nodes) is removed from the
plan. Phase 2 consists of 2a (Graph owns Manager and Namespace) and 2b
(FillSlot detects resource identity). The `extractResource` reflection
helper moves to `resource.go` without annotation constants.

### 10. Infallible Resource Constructors — Construct / Resolve / Refresh

**Status**: Implemented. See [phase-9.md](resource-management/phase-9.md)
for the implementation plan.

**Decision**: Resource constructors are infallible pure computation. They
accept the data needed to identify the resource on a target device and
return a value with no I/O and no error. Resolution (metadata population
via `os.Stat`) is a separate explicit step owned by the executor or the
provider method that needs it.

**Rationale**: A graph can be planned on one machine and executed on
another. `filepath.Abs`, `os.Stat`, and `filepath.Clean` produce
machine-specific results — inodes, device IDs, symlink resolution, and
even absolute paths differ across hosts. The constructor must capture
only the portable identity (the path as given) and defer all
target-specific work to execution time.

This also eliminates the dual constructor registries
(`constructorRegistry` for execution-time constructors with I/O,
`planTimeConstructorRegistry` for plan-time constructors without I/O).
A single constructor suffices because it is always pure.

**Resource lifecycle** — three distinct phases:

| Phase | Method | I/O | When | Purpose |
| --- | --- | --- | --- | --- |
| Construct | `NewResource(path)` | None | Plan time | Save identity (path). Infallible. |
| Resolve | `Resolve()` | `os.Stat` | Executor pre-flight or provider internals | Populate metadata. Confirm existence. |
| Refresh | `Refresh()` | `os.Stat` + checksum | After mutation (provider internals) | Re-populate metadata. |
| Refresh | `RefreshWith(checksum, size)` | `os.Stat` only | After write with known hash | Optimized refresh. |

```go
// Construct — pure, no I/O, infallible
r := file.NewResource("/etc/nginx/nginx.conf")

// Resolve — I/O against the target machine
if err := r.Resolve(); err != nil { ... }
if r.Exists() { ... }

// After a write operation — refresh metadata
if err := r.Refresh(); err != nil { ... }

// After a write with known hash — optimized refresh
if err := r.RefreshWith(checksum, size); err != nil { ... }
```

**Existence checking**: `Resolve()` populates metadata if the file
exists and returns nil. If the file does not exist, `Resolve()` returns
nil and metadata remains empty. `Exists()` checks whether metadata was
populated — it is a pure check on the resolved state, not an I/O
operation. Callers that need to know whether a path exists call
`Resolve()` then `Exists()`. An unresolved resource always reports
`Exists() == false`.

**Impact on constructor registry**: `RegisterPlanTimeConstructor` is
eliminated. `RegisterConstructor` registers the single infallible
constructor. `coerceSlotValue` at execution time produces a constructed
but unresolved Resource. Provider methods that need metadata call
`Resolve()` internally. The executor's pre-flight pass calls `Resolve()`
on catalog discovery entries to validate source existence before
execution begins.

**Impact on URI canonicalization** (supersedes Decision #2):
`filepath.Abs` and `filepath.Clean` are NOT called in the constructor —
they produce machine-specific results. The constructor stores the path
as given. URI canonicalization is performed at resolve time on the target
machine, or deferred to the catalog's namespace which deduplicates by
URI.

### 11. Package URIs — Purl Canonical Form with Hierarchical Catalog Keys

**Status**: Implemented. See [phase-10.md](resource-management/phase-10.md)
for the implementation plan.

**Decision**: `pkg.Resource` adopts the [package-url (purl)](https://github.com/package-url/purl-spec)
specification for canonical package identification. The catalog URI
remains a `url.URL`-compatible hierarchical key; the purl is stored as
a separate canonical representation on the Resource.

**Rationale**: Purl is an ECMA-427 standard that encodes package
ecosystem, namespace, name, version, and qualifiers into a single
string: `pkg:type/namespace/name@version?qualifiers`. It is widely
adopted by SBOM tools, vulnerability databases, and dependency scanners.
Adopting purl gives devlore interoperability with the security and
supply-chain ecosystem for free.

**The problem with purl as a catalog key**: Purl is an opaque URI
(`pkg:type/...` — no `//`). Go's `net/url.URL` expects hierarchical
URIs (`scheme://authority/path`). Feeding `pkg:brew/jq@1.7` to
`url.Parse` produces `Opaque: "brew/jq@1.7"` with empty `Host` and
`Path`. This breaks `ResourceBase.NewURI` and the catalog's
URI-keyed namespace.

**Solution**: Same pattern as `file.Resource`. The catalog key is a
hierarchical URI that works with `url.URL`. The canonical external
form is stored separately on the Resource:

| | Catalog key (URI) | External representation |
| --- | --- | --- |
| file | `file:///etc/nginx/nginx.conf` | `SourcePath` field |
| pkg | `pkg://brew/jq` | `Purl()` method → `pkg:brew/jq@1.7` |
| service | `svc:///nginx` | `Name` field |
| git | `git:///path/to/repo` | `URL` field |

The purl `type` component maps to `url.URL.Host` (the authority). The
package name maps to `url.URL.Path`. Version is metadata on the
Resource, not part of the catalog URI — installing `jq@1.6` and
upgrading to `jq@1.7` is the same resource in the catalog.

```go
type Resource struct {
    op.ResourceBase
    Name    string // "jq"
    Type    string // "brew", "deb", "rpm", "port", "winget"
    Version string // populated by Resolve()
}

// Catalog key — hierarchical, url.URL-compatible.
func (r *Resource) URI() string    { return r.NewURI(r) }
func (r *Resource) Scheme() string { return op.SchemePackage }
func (r *Resource) Host() string   { return r.Type }
func (r *Resource) Path() string   { return "/" + r.Name }
// → pkg://brew/jq

// Canonical purl for external consumption.
func (r *Resource) Purl() string {
    s := "pkg:" + r.Type + "/" + r.Name
    if r.Version != "" {
        s += "@" + r.Version
    }
    return s
}
// → pkg:brew/jq@1.7
```

**Purl type mapping**: Three types are official purl types (`deb`,
`rpm`, `npm`). The rest are custom types following purl conventions.
The spec explicitly supports custom types and recommends contributing
them upstream.

| devlore source | purl type | namespace | Example |
| --- | --- | --- | --- |
| `brew` | `brew` | — | `pkg:brew/jq` |
| `brew` (cask) | `brew` | — | `pkg:brew/firefox?cask=true` |
| `port` | `port` | — | `pkg:port/jq` |
| `apt` | `deb` | distro | `pkg:deb/debian/curl` |
| `dnf`/`yum` | `rpm` | — | `pkg:rpm/nginx` |
| `winget` | `winget` | publisher | `pkg:winget/Microsoft/VisualStudioCode` |

**Winget namespace**: Winget uses reverse-domain IDs
(`Microsoft.VisualStudioCode`). The publisher portion maps to the purl
namespace: `pkg:winget/Microsoft/VisualStudioCode`.

**Auto-detected manager**: When the manager is not specified at plan
time (`manager=""`), the `Type` field is empty. `Resolve()` on the
target machine populates it from the platform's default package manager.
The catalog URI for an unresolved package is `pkg:///jq` (no authority).
After resolution, the URI gains the type: `pkg://brew/jq`. This is
consistent with Decision #10 — the constructor stores portable identity,
resolution populates target-specific data.

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

### Phase 1: ResourceManager and NamespaceMap — DONE

**DONE** — Merged in PR #176. Originally implemented as `ResourceManager` +
`NamespaceMap`. Phase 4 consolidated both into `ResourceCatalog`.

See [phase-1.md](resource-management/phase-1.md).

### Phase 2: Graph Integration — DONE

**DONE** — Merged in PR #177. `Graph.Catalog *ResourceCatalog` (was two
fields, consolidated in Phase 4). `FillSlot` creates implicit edges via
`extractResource`. `backup` annotation removed.

See [phase-2.md](resource-management/phase-2.md).

### Phase 3: File Provider — Complete Input Migration — DONE

**DONE** — Merged in PRs #177 (Phases 0–2) and #178 (Phase 3b).

All file provider methods accept `Resource` for path parameters. All
compensable methods return `Resource` results. The Reducer/Actor signatures
use `Resource`. `boundary` is `Resource`. Mode uses full `os.FileMode`.
`Provider.Root()` returns `Context().Root`.

See [phase-3.md](resource-management/phase-3.md) and
[phase-3b.md](resource-management/phase-3b.md) for details.

### Phase 4: Resource Type System + Tombstone Interface — DONE

**DONE** — Merged in PR #179.

The foundational type system is complete:

- `Resource` is a sealed interface (`URI()` + `resourceBase()`),
  `ResourceBase` is the embedded struct with private identity fields.
- `Tombstone` is a sealed interface (`Resource()` + `tombstoneBase()`),
  `TombstoneBase` is the embedded struct carrying the affected `Resource`.
- `Provider` interface is sealed (`providerBase() *ProviderBase`).
- `ResourceCatalog` replaces `ResourceManager` + `NamespaceMap` as a single
  compositor. `namespace.go` and `state.go` deleted.
- `starvalue` package with `Marshaler`/`Unmarshaler` interfaces.
  `ResourceBase.MarshalStarvalue()` serializes private fields.
- `extractResource` handles marshal round-trip (direct, flat map, nested
  `resource_base` form).
- `file.Tombstone` embeds `TombstoneBase` + `RecoveryPath`. All 9 compensable
  file methods use typed `Tombstone` instead of `map[string]any`.

See [phase-4.md](resource-management/phase-4.md) for the type system plan.

#### Files

| File | Action | Step |
| --- | --- | --- |
| `pkg/op/resource.go` | Rewrite | Resource/Tombstone interfaces, ResourceBase, TombstoneBase |
| `pkg/op/resource_catalog.go` | Create | ResourceCatalog (ledger + namespace + extractResource) |
| `pkg/op/provider.go` | Modify | Sealed Provider interface |
| `pkg/op/namespace.go` | Delete | Merged into ResourceCatalog |
| `pkg/op/state.go` | Delete | Consolidated |
| `pkg/op/graph.go` | Modify | `Catalog *ResourceCatalog` field |
| `pkg/op/output.go` | Modify | Implicit edges via extractResource |
| `pkg/op/starvalue/starvalue.go` | Create | Marshaler/Unmarshaler interfaces |
| `pkg/op/starvalue_marshal.go` | Rename | Was `marshal.go` |
| `pkg/op/provider/file/resource.go` | Modify | Embed ResourceBase, new Tombstone struct |
| `pkg/op/provider/file/provider.go` | Modify | All compensable methods use Tombstone |
| `pkg/op/provider/file/recovery.go` | Modify | Typed Tombstone, `os.Lstat` for symlinks |

### Phase 5: Executor Catalog Integration — DONE

Connects the catalog to both planning and execution. The executor
integrates with the catalog for shadow/resolve during execution and
pre-flight resolution.

**Status**: Complete. Implemented across PRs #177–#186.

**Repo**: devlore-cli

#### 5a. URI Construction (Scheme/Host/Path) — DONE

`Resource` interface gains `Scheme()`, `Host()`, `Path()` component
methods. `ResourceBase.NewURI(r)` constructs canonical URIs. All
provider resource types implement these methods.

#### 5b. NoResult Sentinel — DONE

`NoResult` type defined. `classifyActionReturn` detects it at position 0
and yields nil Result.

#### 5c. Tombstone Semantics — DONE

`TombstoneBase` holds the affected `Resource`. `Resource.SourcePath`
reflects WHERE DATA IS AFTER operation (polarity correct).

#### 5d. classifyActionReturn Arity Dispatch — DONE

Two-arity dispatch: 2 returns = Action, 3 returns = CompensableAction.
`NoResult` detection. Error must be last.

#### 5e. CompensableAction Pairing Validation — DONE

`RegisterReflectedActions` validates all compensator pairs at registration
time. Panics on missing compensators (forward method returns 3 values but
no `Compensate*` exists) and orphaned compensators (`Compensate*` exists
but forward method has wrong return arity).

#### 5f. Action Layer catalog.Shadow — DONE

`reflectedAction.Do()` calls `shadowResult()` after dispatch. Handles
direct Resources, struct values, and slices of Resources.

#### 5g. Planned Bridge Catalog Calls — DONE

`resolveResourceParam()` calls `catalog.Resolve` for input parameters
during planning. `shadowOutputParam()` calls `catalog.Shadow` for output
parameters — the last Resource-typed parameter (destination convention)
is shadowed with the node's ID. Works for both single-Resource methods
(WriteText, Append, etc.) and multi-Resource methods (Copy, Link).

#### 5h. Executor Pre-Flight Resolution — DONE

`ResolveResources()` in `preflight.go` iterates discovery URIs, stat
checks `file://` resources, skips other schemes, fails fast on missing
sources.

#### 5i. Immediate Mode Catalog Integration — DONE

`ReflectedReceiver.SetCatalog()` injects a catalog. `buildMethodBridge`
calls `shadowResult()` after successful dispatch when a catalog is
present. `RecoveryStack` wiring to Starlark is a separate concern.

#### 5j. Plan-Time Coercion Split — DONE (Superseded by Decision #10)

Per Decision #10, constructors are infallible and do no I/O. A single
`constructorRegistry` (`starvalue_marshal.go`) serves both plan-time
and execution-time use. The dual-registry approach
(`planTimeConstructorRegistry`) was eliminated. `constructResource()`
creates URI-only Resources for plan-time catalog resolution.

#### 5k. Conflict Detection — DONE

`ResourceCatalog.Shadow()` detects write-write conflicts. When two
different origins target the same URI, Shadow returns an error:
`"resource conflict: URI %q is targeted by both %q and %q"`.
Discovery entries (no origin) are silently superseded.

### Phase 6: Provider Context + Resource Types — DONE

**DONE** — All providers embed `op.ProviderBase` (codegen-enforced).
Resource types created for git (`git.Resource`), service
(`service.Resource`), pkg (`pkg.Resource`). Typed tombstones for
git/service/pkg. `output io.Writer` removed from git/service/shell
signatures. `Platform` removed as direct field from service/pkg.
Per-graph provider lifecycle via `ActionRegistrar(reg, ctx)`.
`BindingConfig` includes `Platform`. Codegen dead code removed.

See [phase-6.md](resource-management/phase-6.md).

### Phase 7: Remaining Provider Method Migration — DONE

**DONE** — Merged in PR #185. All provider methods that identify external
entities (file paths, URLs, package names, service names) now accept typed
Resource parameters. Typed Tombstones replace `map[string]any` across all
providers. `coerceSlice` enables `[]string` → `[]Resource` automatic
coercion via the constructor registry.

See [phase-7.md](resource-management/phase-7.md).

#### Provider Migration Summary

| Provider | Resource params | Config params (unchanged) |
| --- | --- | --- |
| git | `Clone(url net.Resource, dest file.Resource)`, `Checkout/Pull(repo git.Resource)` | ref |
| net | `Download(url net.Resource)` | (none) |
| pkg | `Install/Remove/Upgrade(packages []pkg.Resource)`, predicates take `pkg.Resource` | manager, cask |
| service | All methods take `service.Resource` | (none) |
| archive | `Extract(source, prefix file.Resource)` + typed `Tombstone` | (none) |
| encryption | `DecryptSopsFile(source, dest file.Resource)` + typed `Tombstone` | (none) |
| template | No change — `source`/`path`/`project` are template variables, not file paths | templateData |
| shell | No change — commands are strings, not external state | command |

#### Infrastructure

- `net.Resource` with canonical `net://` URI (RFC 3986 normalization:
  lowercase host, strip default ports, normalize percent-encoding,
  collapse slashes, sort query params)
- `coerceSlice` in `action_reflect.go` for `[]T` → `[]U` element-wise coercion
- `SchemeNet` constant
- `encryption.CompensateDecryptSopsFile` implemented (was panic)

### Phase 8: Generated Bridge Tests — DONE

**DONE** — Merged in PR #182. The star generator produces
`actions_test.gen.go` — bridge verification tests that regenerate
automatically when method signatures change.

See [phase-8.md](resource-management/phase-8.md).

### Phase 9: Resource Lifecycle — Decision #10 — DONE

**DONE** — Infallible resource constructors. Three-phase lifecycle:
Construct (pure, no I/O) → Resolve (os.Stat on target) → Refresh
(re-populate after mutation). Single unified constructor registry.
`file.NewResource` and `git.NewResource` are infallible.
`file.Resource.Resolve()` and `git.Resource.Resolve()` do I/O.

See [phase-9.md](resource-management/phase-9.md).

### Phase 10: Purl Package URIs — Decision #11 — DONE

**DONE** — `pkg.Resource` adopts purl (ECMA-427) for canonical package
identification. `Type` and `Version` fields added. Catalog URI uses Type
as authority (`pkg://brew/jq`). `Purl()` method returns canonical purl
format (`pkg:brew/jq@1.7`). Winget namespace handling. Type propagated
from resolved manager in Install/Remove/Upgrade.

See [phase-10.md](resource-management/phase-10.md).

### Phase 11: Action Interface Unification — DONE

**DONE** — Unified `Do` return signature across all three action types:
`Action`, `FallibleAction`, `CompensableAction` all return
`(Result, Complement, error)`. Eliminated the `DoAction()` type-switch
dispatcher — each reflected type normalizes internally. Codegen updated
to register and test pure actions (e.g., `file.name`, `file.parent`,
`file.join`) as first-class graph nodes.

See [phase-11.md](resource-management/phase-11.md).

## Migration Path

0. **Phase 0**: ~~Build the `star devlore test` extension.~~ **~85% DONE**
   — Harness built, baseline tests pass. Remaining: un-skip 13 tests —
   #170 (1 immediate binding), #171 (1 planned binding), #164 (11
   recovery site tests blocked by macOS SIP).
1. **Phase 1**: ~~ResourceManager and NamespaceMap.~~ **DONE** —
   Implemented as `ResourceCatalog`. See
   [phase-1.md](resource-management/phase-1.md). Merged in PR #176.
2. **Phase 2**: ~~Graph integration.~~ **DONE** — `Graph.Catalog
   *ResourceCatalog`, `FillSlot` implicit edges. See
   [phase-2.md](resource-management/phase-2.md). Merged in PR #177.
3. **Phase 3**: ~~File provider migration.~~ **DONE** — All methods accept
   `Resource`. Reducer/Actor signatures. `Provider.Root()` returns `Context().Root`.
   See [phase-3.md](resource-management/phase-3.md),
   [phase-3b.md](resource-management/phase-3b.md). Merged in PRs #177, #178.
4. **Phase 4 (type system + tombstone interface)**: ~~Resource type system,
   starvalue marshaling, Tombstone interface.~~ **DONE** — `Resource` and
   `Tombstone` are sealed interfaces. `ResourceBase` and `TombstoneBase`
   are embedded structs. `Provider` interface sealed. `ResourceCatalog`
   replaces manager + namespace. File provider uses typed `Tombstone`.
   See [phase-4.md](resource-management/phase-4.md). Merged in PR #179.
5. **Phase 5 (executor catalog integration)**: **DONE** —
   Implemented across PRs #177–#186. Catalog integrates with both
   planning (Resolve for inputs, Shadow for all output Resources) and
   execution (Shadow for results, pre-flight resolution for file://
   URIs). All substeps complete: URI construction (5a), NoResult (5b),
   tombstone semantics (5c), arity dispatch (5d), CompensableAction
   pairing (5e), action-layer shadow (5f), planned-bridge shadow (5g),
   pre-flight (5h), immediate mode catalog (5i), single constructor
   registry (5j), conflict detection (5k).
6. **Phase 6 (provider context + resource types)**: **DONE** — All
   providers embed `op.ProviderBase` (codegen-enforced). Resource types
   created for git (`git.Resource`), service (`service.Resource`), pkg
   (`pkg.Resource`). Typed tombstones for git/service/pkg. `output
   io.Writer` removed from git/service/shell signatures. `Platform`
   removed as direct field from service/pkg. Per-graph provider lifecycle
   via `ActionRegistrar(reg, ctx)`. `BindingConfig` includes `Platform`.
   Codegen dead code removed (`--extra-attrs`, `--methods`, `--package`,
   `--templates`, standard mode). See
   [phase-6.md](resource-management/phase-6.md).
7. **Phase 7 (remaining provider method migration)**: **DONE** — All
   provider methods migrated to Resource-typed parameters. Typed
   Tombstones replace `map[string]any` across all providers. `coerceSlice`
   enables `[]string` → `[]Resource` coercion. See
   [phase-7.md](resource-management/phase-7.md). Merged in PR #185.
8. **Phase 8 (generated bridge tests)**: **DONE** — Extend the star
   generator to produce `actions_test.gen.go` — bridge verification tests
   that regenerate automatically when method signatures change.
   See [phase-8.md](resource-management/phase-8.md). Merged in PR #182.
9. **Phase 9 (resource lifecycle — Decision #10)**: **DONE** — Infallible
   constructors, Construct/Resolve/Refresh lifecycle, single constructor
   registry. See [phase-9.md](resource-management/phase-9.md).
10. **Phase 10 (purl package URIs — Decision #11)**: **DONE** — `pkg.Resource`
    gains `Type`, `Version`, `Purl()`. Catalog URI uses Type as authority.
    Winget namespace handling. See [phase-10.md](resource-management/phase-10.md).
11. **Phase 11 (action interface unification)**: **DONE** — Unified `Do`
    signature `(Result, Complement, error)` across all three action types.
    `DoAction()` dispatcher eliminated. Codegen registers and tests pure
    actions as graph nodes. See [phase-11.md](resource-management/phase-11.md).

## Files to Create/Modify

### Completed

| File | Phase | Action | Status |
| --- | --- | --- | --- |
| `cmd/devlore-test/main.go` | 0 | Create | **DONE** |
| `internal/e2e/testrunner/runner.go` | 0 | Create | **DONE** |
| `internal/e2e/testrunner/runner_test.go` | 0 | Create | **DONE** |
| `internal/e2e/testrunner/test_context.go` | 0 | Create | **DONE** |
| `internal/e2e/testrunner/trace.go` | 0 | Create | **DONE** |
| `internal/e2e/testrunner/data/test_*.star` | 0 | Create | **DONE** |
| `star/extensions/com.noblefactor.devlore.Test/` | 0 | Create | **DONE** |
| `Makefile` | 0 | Modify | **DONE** |
| `pkg/op/resource.go` | 1, 4 | Rewrite | **DONE** — Resource/Tombstone interfaces, ResourceBase, TombstoneBase |
| `pkg/op/resource_catalog.go` | 1, 4 | Create | **DONE** — Replaces ResourceManager + NamespaceMap |
| `pkg/op/resource_test.go` | 1 | Create | **DONE** |
| `pkg/op/resource_catalog_test.go` | 1 | Create | **DONE** |
| `pkg/op/namespace.go` | 4 | Delete | **DONE** — Merged into ResourceCatalog |
| `pkg/op/state.go` | 4 | Delete | **DONE** — Consolidated |
| `pkg/op/graph.go` | 2 | Modify | **DONE** — `Catalog *ResourceCatalog` |
| `pkg/op/output.go` | 2 | Modify | **DONE** — Implicit edges via extractResource |
| `pkg/op/provider.go` | 4 | Modify | **DONE** — Sealed Provider interface |
| `pkg/op/starvalue/starvalue.go` | 4 | Create | **DONE** — Marshaler/Unmarshaler |
| `pkg/op/starvalue_marshal.go` | 4 | Rename | **DONE** — Was `marshal.go` |
| `pkg/op/provider/file/resource.go` | 3, 4 | Modify | **DONE** — ResourceBase embedding, Tombstone struct |
| `pkg/op/provider/file/provider.go` | 3 | Modify | **DONE** — All methods use Resource/Tombstone |
| `pkg/op/provider/file/provider_test.go` | 3 | Modify | **DONE** — All tests updated |
| `pkg/op/provider/file/recovery.go` | 4 | Modify | **DONE** — Typed Tombstone |

| `pkg/op/planned_reflect.go` | 5 | Modify | **DONE** — Catalog Resolve for inputs during planning |
| `pkg/op/action_reflect.go` | 5 | Modify | **DONE** — `coerceSlice` for `[]T` → `[]U` coercion |
| `internal/execution/preflight.go` | 5 | Rewrite | **DONE** — Resource-aware pre-flight (`file://` stat) |
| `internal/execution/executor.go` | 5 | Modify | **DONE** — Catalog passed to `op.Context`, `shadowResult` after dispatch |
| `pkg/op/provider/git/resource.go` | 6 | Create | **DONE** — `git.Resource` |
| `pkg/op/provider/service/resource.go` | 6 | Create | **DONE** — `service.Resource` |
| `pkg/op/provider/pkg/resource.go` | 6 | Create | **DONE** — `pkg.Resource` |
| `pkg/op/provider/net/resource.go` | 7 | Create | **DONE** — `net.Resource` with canonical URI normalization |
| `pkg/op/provider/archive/resource.go` | 7 | Create | **DONE** — `archive.Tombstone` |
| `pkg/op/provider/encryption/resource.go` | 7 | Create | **DONE** — `encryption.Tombstone` |
| `pkg/op/provider/*/provider.go` | 7 | Modify | **DONE** — All methods accept typed Resource params |
| noblefactor-ops templates | 6 | Modify | **DONE** — Resource-typed provider field support |
| `pkg/op/provider/*/gen/actions_test.gen.go` | 8 | Create | **DONE** — Generated bridge tests |
| noblefactor-ops star generator | 8 | Modify | **DONE** — `actions_test` template |

### Remaining

| File | Phase | Action | Purpose |
| --- | --- | --- | --- |
| `pkg/op/provider/file/gen/integration_test.go` | 0 | Modify | Un-skip #170, #171 |
| `pkg/op/provider/file/gen/actions_test.go` | 0 | Modify | Un-skip #164 |

## Relationship to Reconciliation

The [Audit, Reconciliation, and Recovery](../architecture/5.1-reconciliation.md)
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
| `RecoverySite.ArchiveFile` / `RecoverySite.RestoreFile` | Tombstone creation and restoration |

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

- [Resource Management Architecture](../architecture/4-resource-management.md)
- [Binding Unification Plan](./binding-unification.md) — Provider binding architecture
- [Compensation Plan](./compensation.md) — Compensation/recovery architecture
- [Audit, Reconciliation, and Recovery](../architecture/5.1-reconciliation.md)

### Implementation Plans

- [Resource Lifecycle (Decision #10)](resource-management/phase-9.md) — Infallible constructors, Resolve/Refresh lifecycle
- [Purl Package URIs (Decision #11)](resource-management/phase-10.md) — Purl adoption for pkg.Resource
- [Action Interface Unification](resource-management/phase-11.md) — Unified Do signature, pure actions as graph nodes
- [Gap Analysis](resource-management/gap-analysis.md) — Document review findings (2026-03-05)
