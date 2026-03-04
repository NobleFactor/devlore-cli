# Phase 5: Executor Catalog Integration

## Context

Phases 1-4 built the resource management infrastructure: `ResourceBase`,
`ResourceCatalog`, `file.Resource` embedding `ResourceBase`, the `starvalue`
marshaling interfaces, and the constructor registry for string-to-Resource
coercion.

Phase 5 wires the catalog into the executor and action layers. It completes
the model-controller separation: providers remain pure models (no catalog
knowledge), the action layer becomes the controller (dispatches calls,
classifies returns, shadows results), and the executor orchestrates
pre-flight resolution. The planned bridge gains catalog integration so
that `buildPlannedBridge` creates resource lineage at plan time.

**Repo**: devlore-cli
**Branch**: `feature/resource-management-phase-5`

## Design Decisions

Full discussion with rationale: [`DESIGN-DISCUSSION.md`](../../../DESIGN-DISCUSSION.md)

### D1. Executor owns the decision, provider owns the mechanism

The executor decides *which* resources need protection (by analyzing the
catalog's namespace — shadowed URIs = something will be overwritten). The
provider decides *how* to protect them (file: `moveToRecovery`; service:
record current state; package: record current version).

### D2. `prepareWrite` stays in the provider

`prepareWrite` is provider-internal logic (discovery + backup of an
existing file before overwriting). Each write method calls it internally.
Moving it to the executor's pre-flight pass was rejected — it's
counterintuitive. That stays.

### D3. Planned bridge must call Resolve/Shadow

`buildPlannedBridge` currently creates nodes and fills slots but never
touches `graph.Catalog`. It needs to:

- `catalog.Resolve(uri)` for source params (inputs that should already exist)
- `catalog.Shadow(resource, originID)` for destination params (outputs that
  will be created/overwritten)

This is what creates the resource lineage that makes implicit edges and
conflict detection work.

### D4. Ledger is sole source of truth

No node annotations for resources. The catalog tracks
URI → resource ID → origin. The executor queries the catalog directly.

### D5. Per-graph namespace, per-phase compensation

One namespace per graph. Phases are saga boundaries (compensation), not
visibility boundaries.

### D6. Two return signatures

There are two action types: `Action` and `CompensableAction`. Each has
its own canonical return signature:

| Action type | Provider method signature | Meaning |
|---|---|---|
| `Action` | `(result, error)` | Non-compensable. No undo state captured. |
| `CompensableAction` | `(result, undo, error)` | Compensable. Undo state captured for saga rollback. |

Rules:

- **Every CompensableAction provider method MUST be paired with a
  `Compensate<MethodName>` method.** A method returning `(result, undo,
  error)` without a companion Compensate method is an error.
- **`NoResult` sentinel type** (`struct{}`) is used at position 0 when a
  method produces no result (e.g., `Remove` destroys something and returns
  nothing). `classifyActionReturn` maps `NoResult` to `nil`.
- **Every method in the graph must return `error` as its last value.**
  Methods without error returns (e.g., `Exists() bool`) are not actions —
  they need updated signatures to be graph-capable.
- **`classifyActionReturn` uses arity to distinguish**: 2 returns = result +
  error (Action); 3 returns = result + undo + error (CompensableAction).

### D7. Inputs are arguments, outputs are returns

For the planned bridge:

- **Inputs** = method arguments (parameters). Resource-typed arguments are
  candidates for `catalog.Resolve`.
- **Outputs** = method returns (position 0 = result). Resource-typed
  results are candidates for `catalog.Shadow`.

The action layer interrogates the result at execution time:

| Result type | Shadow strategy | Methods |
|---|---|---|
| `Resource` | `catalog.Shadow(result, nodeID)` | Most methods |
| `NoResult` | Nothing to shadow (destruction only) | Remove, RemoveAll, Unlink |
| `[]Resource` | Iterate, shadow each | pkg: Install/Remove/Upgrade |
| `any` | Not shadow-able (observation only) | WalkTree |
| `[]byte` | Not a resource | Download, Render |
| `bool` | Not a resource | Predicates |
| `string` | Not a resource | shell.Exec |

## Steps

### 5a. URI construction — `Resource` interface gains component methods

**Files**: `pkg/op/resource.go`, `pkg/op/resource_test.go`

The `Resource` interface gains three component methods following
`net/url.URL` naming:

```go
type Resource interface {
    URI() string
    Scheme() string
    Host() string
    Path() string
    resourceBase() *ResourceBase
}
```

`ResourceBase.NewURI(r Resource)` builds the URI from the three
components via `net/url.URL`:

```go
func (b *ResourceBase) NewURI(r Resource) string {
    return (&url.URL{Scheme: r.Scheme(), Host: r.Host(), Path: r.Path()}).String()
}
```

Each provider implements `URI()` as a one-liner that passes itself:

```go
func (r *Resource) URI() string { return r.NewURI(r) }
```

`ResourceBase` stores nothing about scheme/host/path — those come from
the concrete type. Canonicalization happens in the provider (e.g.,
`file.Resource.Path()` returns `filepath.Abs` + `filepath.Clean`).

The standalone `ResourceURI` helper function is removed. All URIs use the
standard authority form: `scheme://host/path` (with host, e.g.,
`git://github.com/org/repo`) or `scheme:///path` (without host, e.g.,
`file:///usr/local/bin`, `svc:///nginx`).

- [ ] Add `Scheme()`, `Host()`, `Path()` to `Resource` interface
- [ ] Add `NewURI(r Resource) string` to `ResourceBase`
- [ ] Implement `Scheme()`, `Host()`, `Path()`, `URI()` on `file.Resource`
- [ ] Remove standalone `ResourceURI` helper
- [ ] Remove URI scheme constants if no longer needed externally
- [ ] Update `NewResourceBase` — URI is no longer a constructor argument
      (it's computed from the three components)
- [ ] Update tests

### 5b. `NoResult` sentinel type

**File**: `pkg/op/resource.go`

```go
// NoResult signals that a method produces no output.
// Used by CompensableAction methods like Remove and RemoveAll that
// can be undone but produce no result for downstream nodes.
type NoResult struct{}
```

- [ ] Add `NoResult` type
- [ ] Document usage in godoc

### 5c. Tombstone semantics — Resource reflects physical truth

**Files**: `pkg/op/resource.go`, `pkg/op/provider/file/resource.go`

The `TombstoneBase` carries the affected `Resource`. After a destructive
operation like `moveToRecovery`, the Resource's identity fields (e.g.,
`SourcePath`) are updated to reflect where the data physically IS — the
recovery location. The Tombstone's provider-specific fields record where
the data CAME FROM — the restoration target.

```go
// file.Tombstone — after moveToRecovery:
//   Resource.SourcePath == recovery path (where the data IS)
//   OriginalPath        == original path (where to restore it)
type Tombstone struct {
    op.TombstoneBase
    OriginalPath string
}
```

This means `file.Tombstone.RecoveryPath` is renamed to `OriginalPath`
and the polarity is corrected: the Resource always tells the truth about
the physical location of the data, and the Tombstone remembers where it
came from.

The `TombstoneBase` docstring must clarify this invariant: the embedded
Resource reflects post-operation state, not pre-operation state.

- [ ] Rename `file.Tombstone.RecoveryPath` → `OriginalPath`
- [ ] Update `moveToRecovery` to set `Resource.SourcePath` to recovery path
- [ ] Update `restoreFromRecovery` to use corrected polarity
- [ ] Update `TombstoneBase` docstring to document the invariant
- [ ] Update all `file.Tombstone` construction sites
- [ ] Update all compensation methods that read Tombstone fields
- [ ] Update tests

### 5d. Simplify `classifyActionReturn`

**File**: `pkg/op/action_reflect.go`

Replace the current multi-arity dispatch with two-arity dispatch:

- 2 returns = `(result, error)` → Action
- 3 returns = `(result, undo, error)` → CompensableAction

Detect `NoResult` at position 0 and yield `nil` Result. Reject any
other arity. Error must always be the last return.

- [ ] Rewrite `classifyActionReturn` for two-arity dispatch
- [ ] Add `NoResult` detection at position 0
- [ ] Reject methods without error returns during registration
- [ ] Update `action_reflect_test.go`

### 5e. Enforce CompensableAction pairing

**File**: `pkg/op/action_reflect.go`

During `RegisterReflectedActions`, validate that every method returning
`(result, undo, error)` has a corresponding `Compensate<MethodName>`
method on the provider. Missing pairs are a registration-time error.

- [ ] Add pairing validation in `RegisterReflectedActions`
- [ ] Add test for missing Compensate method → error

### 5f. Action layer calls `catalog.Shadow`

**File**: `pkg/op/action_reflect.go`

After `reflectedAction.Do` dispatches the call and classifies the return,
it calls `catalog.Shadow` when the result is a Resource. The action layer
already knows the result type and the node ID — everything Shadow needs.

The provider remains a pure model with no catalog knowledge. The action
layer is the controller.

| Result type | Action |
|---|---|
| `Resource` | `catalog.Shadow(result, nodeID)` |
| `Tombstone` | `catalog.Shadow(tombstone.Resource(), nodeID)` |
| `[]Resource` | Iterate, shadow each |
| `NoResult` / `bool` / `string` / `[]byte` | No shadowing |

The action layer needs access to the catalog. Options:

1. Pass catalog through the `Action.Do` signature (extend `op.Context`)
2. Store catalog reference on the reflected action at registration time

Option 1 is preferred — the catalog is part of the execution context.
Add `Catalog *ResourceCatalog` to `op.Context`.

- [ ] Add `Catalog *ResourceCatalog` to `op.Context`
- [ ] Shadow results in `reflectedAction.Do` post-dispatch
- [ ] Handle `Resource`, `Tombstone`, `[]Resource` result types
- [ ] Update tests

### 5g. Planned bridge calls Resolve/Shadow

**Files**: `pkg/op/planned_reflect.go`

`buildPlannedBridge` gets the `reflect.Method` (already available in
`WrapPlanned` via `providerType.NumMethod()`) and inspects parameter
and return types:

- **Inputs**: `method.Type.In(i+1)` — if the parameter type implements
  `Resource`, call `catalog.Resolve(uri)`.
- **Outputs**: `method.Type.Out(0)` — if the return type implements
  `Resource`, prepare for `catalog.Shadow` at execution time.

Plan-time coercion creates typed but metadata-empty Resources (URI only,
no I/O). This is the `ResourceFromPath` path.

- [ ] Pass `reflect.Method` through to `buildPlannedBridge`
- [ ] Inspect param types for Resource → `catalog.Resolve`
- [ ] Inspect result type for Resource → prepare Shadow
- [ ] Add plan-time coercion path to constructor registry
- [ ] Update tests

### 5h. Executor pre-flight — resolution pass

**File**: `internal/execution/preflight.go`

The pre-flight pass iterates unresolved catalog entries and resolves
them against the target machine:

```
for each entry in catalog where state = unresolved:
    os.Stat on target → populate metadata
    mark entry as resolved
    fail fast if source does not exist
```

No conflict detection — two nodes targeting the same URI in the same
phase may be intentional (e.g., write then overwrite with a transform).
The executor does not presume to know intent. No tombstone creation —
the provider handles its own compensation (D2).

- [ ] Implement resolution pass over catalog
- [ ] Fail-fast on missing sources
- [ ] Update tests

### 5i. Immediate mode sister controller

**File**: `pkg/op/receiver_reflect.go` (or new file)

Immediate mode does NOT bypass the catalog. It gets a sister controller
that mirrors the action layer's catalog integration:

- Calls `catalog.Shadow` for results
- Marshals strings and other data types as Resources when appropriate
- `RecoveryStack` is exposed to Go consumers and operative in this
  controller; it may be hidden from Starlark scripts
- On failure within a script, the controller can rollback or not
  depending on the script host and its retry policy

The provider remains unaware of the catalog in both modes.

Update `classifyReturn` in `receiver_reflect.go` to detect `NoResult`
at position 0 and yield `starlark.None`.

- [ ] Add catalog integration to immediate-mode dispatch
- [ ] `NoResult` → `starlark.None` in `classifyReturn`
- [ ] RecoveryStack wiring
- [ ] Update tests

### 5j. Fix `refreshMetadataWith` checksum parameter

**File**: `pkg/op/provider/file/resource.go`

`refreshMetadataWith` currently calls `checksumFile(r.SourcePath)` instead
of using the passed `checksum` parameter. Redundant disk read. Fix: use
the passed parameter.

- [ ] Fix `refreshMetadataWith` to use the passed checksum
- [ ] Update tests if any

### 5k. Plan-time coercion split

**File**: `pkg/op/starvalue_marshal.go`

The constructor registry gains a plan-time coercion path:

- **Plan time**: `ResourceFromPath(path)` — pure, URI only, no I/O.
  Called by the planned bridge during `buildPlannedBridge`.
- **Execution time**: `NewResource(path)` — does `os.Stat`, populates
  metadata. Called during immediate mode and during executor resolution.

The split is required because a graph can be saved and run on any
compatible device. `os.Stat` at plan time is meaningless when the plan
is built on a Mac and run on a Debian box.

- [ ] Add `ResourceFromPath` constructor for `file.Resource`
- [ ] Register plan-time coercion in constructor registry
- [ ] Ensure planned bridge uses plan-time coercion
- [ ] Update tests

## Files

| File | Action | Step |
|------|--------|------|
| `pkg/op/resource.go` | Modify | 5a, 5b, 5c |
| `pkg/op/resource_test.go` | Modify | 5a, 5b |
| `pkg/op/action.go` | Modify | 5f (Context) |
| `pkg/op/action_reflect.go` | Modify | 5d, 5e, 5f |
| `pkg/op/action_reflect_test.go` | Modify | 5d, 5e, 5f |
| `pkg/op/planned_reflect.go` | Modify | 5g |
| `pkg/op/receiver_reflect.go` | Modify | 5i |
| `pkg/op/receiver_reflect_test.go` | Modify | 5i |
| `pkg/op/starvalue_marshal.go` | Modify | 5k |
| `pkg/op/provider/file/resource.go` | Modify | 5a, 5j, 5k |
| `pkg/op/provider/file/resource_test.go` | Modify | 5a, 5j |
| `internal/execution/preflight.go` | Modify | 5h |

## Verification

1. `make build` — passes
2. `make vet` — passes
3. `make test` — passes
4. `make test-race`
5. Grep for standalone `ResourceURI(` calls — zero hits (replaced by `NewURI`)
6. Grep for `fmt.Sprintf("file://"` — zero hits (replaced by `NewURI`)
7. Verify `classifyActionReturn` rejects methods with arity != 2 or 3
8. Verify CompensableAction pairing validation catches missing `Compensate*` methods
9. Verify planned bridge creates catalog lineage (Resolve for inputs, Shadow for outputs)
10. Verify pre-flight fails fast on missing source resources

## Related Documents

- [DESIGN-DISCUSSION.md](../../../DESIGN-DISCUSSION.md) — Full design discussion with D1-D7 and O1-O7
- [Phase 4](./phase-4.md) — Resource type system + starvalue marshaling
- [Phase 6](./phase-6.md) — Remaining provider migration
- [Architecture](../../architecture/devlore-resource-management.md) — Master architecture document
