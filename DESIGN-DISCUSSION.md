# Phase 5: Executor Catalog Integration — Design Discussion

**Status**: IN PROGRESS (2026-03-04)

This document captures the design decisions and open issues for Phase 5 of the resource management plan. Phase 5 wires the `ResourceCatalog` into the executor and planned-mode bridges.

---

## Design Decisions (Settled)

### D1. Executor owns the decision, provider owns the mechanism

The executor decides *which* resources need protection (by analyzing the catalog's namespace — shadowed URIs = something will be overwritten). The provider decides *how* to protect them (file: `moveToRecovery`; service: record current state; package: record current version).

### D2. `prepareWrite` stays in the provider

The original plan moved `prepareWrite` out of each file method and into the executor's pre-flight pass. This was rejected — it's counterintuitive. `prepareWrite` is provider-internal logic (discovery + backup of an existing file before overwriting). Each write method calls it internally. That stays.

### D3. Planned bridge must call Resolve/Shadow

`buildPlannedBridge` currently creates nodes and fills slots but never touches `graph.Catalog`. It needs to:

- `catalog.Resolve(uri)` for source params (inputs that should already exist)
- `catalog.Shadow(resource, originID)` for destination params (outputs that will be created/overwritten)

This is what creates the resource lineage that makes implicit edges and conflict detection work.

### D4. Ledger is sole source of truth (Decision #9 from master plan)

No node annotations for resources. The catalog tracks URI -> resource ID -> origin. The executor queries the catalog directly.

### D5. Per-graph namespace, per-phase compensation (Decision #4)

One namespace per graph. Phases are saga boundaries (compensation), not visibility boundaries.

### D6. Two return signatures

There are two action types: `Action` and `CompensableAction`. Each has its own canonical return signature:

| Action type | Provider method signature | Meaning |
|---|---|---|
| `Action` | `(result, error)` | Non-compensable. No undo state captured. |
| `CompensableAction` | `(result, undo, error)` | Compensable. Undo state captured for saga rollback. |

Rules:

- **Every CompensableAction provider method MUST be paired with a `Compensate<MethodName>` method.** A method returning `(result, undo, error)` without a companion Compensate method is an error.
- **`NoResult` sentinel type** (`struct{}`) is used at position 0 when a method produces no result (e.g., `Remove` destroys something and returns nothing). `classifyActionReturn` maps `NoResult` to `nil`.
- **Every method in the graph must return `error` as its last value.** Methods without error returns (e.g., `Exists() bool`) are not actions. To make them graph-capable, update the signature to `(bool, error)` — filesystem operations CAN fail. Swallowing errors is a bug.
- **`classifyActionReturn` uses arity to distinguish**: 2 returns (minus error) = result only; 3 returns (minus error) = result + undo state.

This replaces the previous multi-arity dispatch that accepted `(error)`, `(T, error)`, and `(T, U, error)` interchangeably.

### D7. Inputs are arguments, outputs are returns

For the planned bridge (`buildPlannedBridge`):

- **Inputs** = method arguments (parameters). Resource-typed arguments are candidates for `catalog.Resolve`.
- **Outputs** = method returns (position 0 = result). Resource-typed results are candidates for `catalog.Shadow`.

The bridge interrogates the result at execution time. When the result is a `Resource`, shadow it. When it's a `Tombstone`, extract the resource via `tombstone.Resource()` and shadow it. When it's a `[]Resource`, iterate and shadow each.

### D8. Tombstone Resource reflects physical truth

The `TombstoneBase` carries the affected `Resource`. After a destructive operation (e.g., `moveToRecovery`), the Resource's identity fields are updated to reflect where the data physically IS — the recovery location. The Tombstone's provider-specific fields record where the data CAME FROM — the restoration target.

For `file.Tombstone`:
- `Resource.SourcePath` = recovery path (where the data IS now)
- `OriginalPath` = original path (where to put it back)

The Resource always tells the truth about the physical location of the data. The Tombstone remembers where it came from. This means `file.Tombstone.RecoveryPath` is renamed to `OriginalPath` and the polarity is corrected.

---

## Open Issues

### O1. How does `buildPlannedBridge` know which params are Resources?

**Status**: RESOLVED

The bridge gets type information the same way the graph provides it at runtime: reflection on the method. `WrapPlanned` already iterates `providerType.NumMethod()` to discover methods — pass the `reflect.Method` through to `buildPlannedBridge` and inspect `method.Type.In(i+1)` for param types, `method.Type.Out(0)` for result type. No new interface, no `SlotTypes()`, no codegen.

**Sub-question: What do we do with result sets?**

After Phase 6 migration, the result types collapse to:

| Result type | Shadow strategy | Methods |
|---|---|---|
| `Resource` | `catalog.Shadow(result, nodeID)` | Most methods |
| `NoResult` | Nothing to shadow (destruction only) | Remove, RemoveAll, Unlink |
| `[]Resource` | Iterate, shadow each | pkg: Install/Remove/Upgrade |
| `any` | Not shadow-able (observation only) | WalkTree |
| `[]byte` | Not a resource | Download, Render |
| `bool` | Not a resource | Predicates |
| `string` | Not a resource | shell.Exec |

`NoResult` is a named `struct{}` type that signals a method produces no output. This applies to CompensableAction methods like `file.Provider.Remove` and `file.Provider.RemoveAll` — they can be undone (the undo state is a `Tombstone`), but they produce no result for downstream nodes. `classifyActionReturn` detects `NoResult` at position 0 and yields `nil` Result. In immediate mode, `classifyReturn` maps it to `starlark.None`.

Two patterns require shadowing: single `Resource` and `[]Resource`. Everything else is either destruction (`NoResult`), observation, or non-resource.

### O2. What does the executor pre-flight actually do now?

**Status**: RESOLVED

Resolution only. The pre-flight iterates unresolved catalog entries, `os.Stat` each on the target machine, and fails fast if a source doesn't exist. That's it.

No conflict detection — two nodes targeting the same URI in the same phase may be intentional (e.g., write then overwrite with a transform). The executor does not presume to know intent. No tombstone creation — the provider handles its own compensation (D2).

### O3. Plan-time coercion: `ResourceFromPath` vs `NewResource`

**Status**: RESOLVED

The split is required. A graph can be saved and run on any compatible device — build once, run on many (e.g., target any Debian Linux box). `os.Stat` at plan time is meaningless when the plan is built on a Mac and run on a Debian box.

- **Plan time**: `ResourceFromPath(path)` — pure, URI only, no I/O. The constructor registry must use this path during planned-mode coercion.
- **Execution time**: Resolve metadata (`os.Stat`) on the target machine during the pre-flight pass (O2).

Down the road, pre-flight will also check prerequisites (e.g., must run on systemd-managed system) and fail fast on incompatible targets.

### O4. URI construction

**Status**: RESOLVED (implemented in Phase 0 of mem-resource epic)

**Decision**: Each concrete resource type owns its URI construction via a private `buildURI()` method. The URI is cached in `ResourceBase.uri` at construction time via `SetURI()`. There is no shared dispatch (`NewURI` is removed). If `Resolve()` changes identity-bearing fields (e.g., path canonicalization), the concrete type calls `SetURI(buildURI())` to update the cache.

URIs follow RFC 3986 — hierarchical for file resources (`file:///path`), opaque for all others (`pkg:brew/jq`, `svc:nginx`, `appnet:host/path`, `git:repo-url#ref`, `mem:callable/type/name`). See `docs/architecture/4.1-resource-identity.md` for the full scheme registry.

### O5. `refreshMetadataWith` ignores the checksum parameter

**Status**: RESOLVED (fix in Phase 5)

`resource.go:215` calls `checksumFile(r.SourcePath)` instead of using the passed `checksum` parameter. Redundant disk read. Will be fixed during Phase 5 implementation.

### O6. Who calls `catalog.Shadow` for results?

**Status**: RESOLVED

The action layer (`reflectedAction.Do`), after dispatch. The provider is the model — it produces a result and has no knowledge of the catalog. The action layer is the controller — it dispatches the call, inspects the result via `classifyActionReturn`, and calls `catalog.Shadow` when the result is a Resource. It already knows the result type and the node ID, which is everything Shadow needs. This reinforces the model-controller separation.

### O7. Immediate mode vs planned mode

**Status**: RESOLVED (revises Decision #3)

Immediate mode does NOT bypass the catalog. It gets a sister controller to the action layer that mirrors its catalog integration — calling `catalog.Shadow` for results, marshaling strings and other data types as Resources when appropriate. The `RecoveryStack` is exposed to Go consumers and operative in this controller; it may be hidden from Starlark scripts. On failure within a script, the controller can rollback or not depending on the script host and its retry policy. The provider remains unaware of the catalog in both modes.

---

## Current Return Type Survey (Pre-Migration)

| Return shape | Methods |
|---|---|
| `(Resource, Tombstone, error)` | file: Backup, Copy, Link, Move, WriteBytes, WriteText |
| `(Resource, error)` | file: Mkdir, Read |
| `(Tombstone, Tombstone, error)` | file: Remove, RemoveAll, Unlink |
| `(any, *RecoveryStack, error)` | file: WalkTree |
| `(string, map[string]any, error)` | git: Clone; service: Start/Stop/Enable/Disable/Restart; archive: Extract |
| `([]string, map[string]any, error)` | pkg: Install/Remove/Upgrade |
| `(string, error)` | git: Checkout/Pull; shell: Exec/PowerShell; pkg: Update |
| `([]byte, error)` | net: Download; template: Render |
| `(bool, error)` | service: Enabled/Exists/Running; pkg: Installed/NotInstalled/VersionGTE |
| `bool` / `string` (no error) | file: Exists, IsDir, IsFile, Join, Name, Parent |
