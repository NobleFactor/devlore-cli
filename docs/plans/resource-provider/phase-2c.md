# Phase 2C: Typed Action Contract + Executor Simplification

```yaml
title: "Phase 2C: Typed Action Contract + Executor Simplification"
issue: https://github.com/NobleFactor/devlore-cli/issues/127
status: complete
created: 2026-02-14
updated: 2026-02-15
```

## Context

Phase 1 (PR #128) renamed Operation to Action with Do/Undo interface.
Phase 2A (PR #129) extracted providers into subpackages.
Phase 2B (PR #130) extracted template/manifest providers.

The executor still does runtime registry lookup to find an Action by string name.
Node.Action is a string. The content pipeline (ContentFor/StoreContent) is a
side-channel that duplicates what the executor should handle natively via edges
and slots. RecoveryEntry uses closures instead of real Action.Undo methods.

This PR makes Action a first-class object on Node, changes the Action contract
to accept resolved slots, eliminates runtime dispatch, and deletes the content
pipeline. Dotted names (already partially implemented in the worktree) are
completed as part of the builder changes.

## Prerequisites (already done in worktree)

Flow package created: `flow/choose.go`, `flow/gather.go`, `flow/elevate.go`,
`flow/register.go`. All 29 action `Name()` methods already return dotted names.
Plan receivers, platform bindings, tree builder, ComputeSummary, Preflight
updated to dotted names. Tests not yet updated.

## Design

### Action Contract

```go
type Action interface {
    Name() string
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
    Undo(ctx *Context, slots map[string]any, state UndoState) error
}
```

Actions receive resolved slots -- they never touch `*Node`. The executor resolves
all promise slots before calling Do.

### Node.Action Field

```go
type Node struct {
    Action Action `json:"-" yaml:"-"`   // was: Action string
    // ...existing fields unchanged...
}
```

- `ActionName() string` helper returns `Action.Name()` (or `""` if nil)
- Node owns serialization via `MarshalJSON`/`UnmarshalJSON` (and YAML equivalents)
  using the `type Alias Node` trick to avoid infinite recursion. The outer struct
  injects `"action"` as a plain string from `ActionName()`. On unmarshal, the
  action name string creates a `stubAction`.
- `stubAction` implements Action with just a name (for receipt deserialization).
  Do/Undo return errors — receipt nodes are not executable.
- No ActionRef wrapper type, no redundant field. The Action interface is the
  single source of truth for the name.

### RecoveryEntry

```go
type RecoveryEntry struct {
    Node      *Node
    UndoState UndoState
}
```

One entry per successfully executed node. Executor pushes after each Do succeeds.
Unwind calls `entry.Node.Action.Undo(ctx, resolvedSlots, entry.UndoState)` in
LIFO order. Phase-level compensation (compensating phases) is a separate concern
that stays unchanged.

### Executor Flow

Per node, in topological order:
1. Resolve promise slots: substitute upstream Results from `results[sv.NodeRef]`
2. Fill slots from Context.Data (defaults for unfilled slots)
3. `result, undoState, err := node.Action.Do(ctx, resolvedSlots)`
4. Store Result in `results[node.ID]` for downstream edge resolution
5. Push `RecoveryEntry{Node: node, UndoState: undoState}`
6. On failure: unwind stack

GraphExecutor drops the `registry` field. No runtime lookup.

### Content Pipeline Removal

Delete from Context:
- `Edges []Edge`
- `Outputs map[string][]byte`
- `ContentFor(node *Node) ([]byte, error)`
- `StoreContent(node *Node, data []byte)`

The executor replaces this: it stores each node's Result (keyed by node ID) and
resolves promise slots from those stored Results before calling downstream Do.
Actions that previously called `ctx.ContentFor(node)` (Copy, Render) now receive
content as a resolved `"content"` slot. Actions that called `ctx.StoreContent`
(Source, Literal) now return content as their Result.

### Graph Builder Slot Promises

The graph builder (`writ/graph_builder.go`) must set slot promises on downstream
nodes in multi-op pipelines, just as the starlark layer does. Currently, the
builder creates edges but relies on the content pipeline for data flow. With the
content pipeline removed, the builder must explicitly set:

```go
// For multi-op pipelines (e.g., source → decrypt → render → copy):
// Node 0 (source): SetSlotImmediate("source", f.Source)
// Node 1 (decrypt): SetSlotPromise("content", prevNode.ID, "")
// Node 2 (render): SetSlotPromise("content", prevNode.ID, "")
// Node 3 (copy): SetSlotPromise("content", prevNode.ID, "")
//                 SetSlotImmediate("path", f.Target)
```

The same pattern applies to `writ/commands.go` upgrade pipeline construction.

### Mode as a Slot

`node.Mode` (currently `os.FileMode`, `json:"-"`) becomes a slot.
One caller sets it: `writ/commands.go:512` (`node.Mode = 0600`).
Actions that read Mode (Copy, Write, Mkdir) read `slots["mode"]`.
Delete `Node.Mode` field and `Node.GetMode()` method.

### Annotations

`file.Backup` currently writes `node.Annotations["backup_path"]`. With the new
contract, the backup path is returned as Result. The executor stores it. Tests
and display code read it from stored results instead of node.Annotations.
Node.Annotations stays for receipt metadata (not action-writable at runtime).

### Checksums

`ctx.SourceChecksum` and `ctx.TargetChecksum` stay on Context for now. Actions
set them as side effects. The executor reads them after Do and applies to the
node. This is a pragmatic choice -- a future PR can move them into a structured
Result type.

### ActionRegistry

Stays as a builder dependency. Builders call `reg.MustGet("file.link")` to get
the Action instance when constructing nodes. Add `MustGet(name string) Action`
that panics if the action is not registered (safe: all actions are pre-registered
before any builder runs).

### Manifest Reader + PlanSequence (replaces Delegate + manifest.Resolve)

The delegate expansion mechanism (`ExpandDelegates`, `SubgraphBuilder`,
`isDelegateNode`) and the manifest execution action (`manifest.Resolve`) are
both removed. Manifest reading is a build-time concern, not an execution-time
action. The replacement is two pieces:

**manifest.Load** already exists (`internal/manifest/manifest.go`) and returns
`*PackagesManifest` with `Packages []PackageEntry`. Each `PackageEntry` is a
YAML object (Name + optional With features per the packages-manifest.json schema).
No wrapper needed.

**lore.Planner** — owns the full resolve → plan cycle for manifest entries:

```go
// in internal/lore/builder.go (replaces Build() as the primary API)
type Planner struct {
    Registry *lorepackage.Registry
    Host     host.Host
    Platform string
    Config   BuildConfig
}

// PlanSequence creates a deployment pipeline from manifest entries.
// Each entry is resolved against the DevLore Registry, then its lifecycle
// phases (prepare, install, provision, verify) are built by buildPackageNodes.
// Per-package features from the manifest entry flow into the Starlark scripts.
func (p *Planner) PlanSequence(g *execution.Graph, pipeline string, entries []manifest.PackageEntry) bool {
    if len(entries) == 0 {
        return false
    }
    for _, entry := range entries {
        pkg, err := p.Registry.Resolve(entry.Name, p.Platform)
        if err != nil {
            return false
        }
        cfg := p.Config
        cfg.Features = entry.With
        if err := buildPackageNodes(g, pkg, p.Host, p.Platform, cfg); err != nil {
            return false
        }
    }
    return true
}
```

`PlanSequence` lives in `lore` (not `manifest`) because it needs the registry
client, host, and platform context to resolve packages and run Starlark scripts.
The existing `buildPackageNodes` IS the phase builder — it already creates
lifecycle phases with forward/compensate nodes via Starlark scripts.

`Build()` becomes a thin wrapper around `Planner.PlanSequence`.

**Shared by writ and lore.** Writ's migration subsystem already imports lore.
When the writ graph builder encounters a `packages-manifest.yaml` in the file
tree, it calls `manifest.Load()` then `planner.PlanSequence()` to add package
deployment phases to the graph. The `*lore.Planner` is passed into the writ
graph builder at construction time.

**Caller pattern** (lore):

```go
m, err := manifest.Load(cfg.ManifestPath)
if err != nil { return nil, err }
planner := &Planner{Registry: regClient, Host: h, Platform: plat, Config: cfg}
if !planner.PlanSequence(graph, "deployment", m.Packages) {
    return nil, fmt.Errorf("failed to plan deployment")
}
```

**Caller pattern** (writ graph builder, when encountering packages-manifest.yaml):

```go
m, err := manifest.Load(manifestPath)
if err != nil { return err }
if !planner.PlanSequence(graph, "deployment", m.Packages) {
    return fmt.Errorf("empty manifest: %s", manifestPath)
}
```

**Removals:**

- `internal/execution/provider/manifest/` — entire package (Resolve action + Provider)
- `internal/manifest/builder.go` — current `Builder` implementing `SubgraphBuilder`
  (replaced by `PlanSequence` in same file)
- `internal/execution/builder.go` — `ExpandDelegates()`, `SubgraphBuilder` interface,
  `isDelegateNode()`, `BuildOptions`
- `manifest.Register(reg)` from `register_gen.go`
- No `flow.Delegate` action (delegate concept is eliminated)

## Steps

### 1. Change Action interface signature

**File**: `action.go`

- `Do(ctx *Context, slots map[string]any) (Result, UndoState, error)`
- `Undo(ctx *Context, slots map[string]any, state UndoState) error`
- Delete `ContentFor`, `StoreContent`, `Edges`, `Outputs` from Context

### 2. Change Node struct

**File**: `graph.go`

- `Action` field: `string` -> `Action` (interface), tag `json:"-" yaml:"-"`
- Add `ActionName() string` helper
- Add `MarshalJSON`/`UnmarshalJSON` (and YAML) that serialize Action as name string
- Add `stubAction` type for deserialization
- Delete `Mode` field and `GetMode()` method
- Delete `GetAction() string` accessor (replaced by `ActionName()`)

### 3. Update RecoveryEntry + RecoveryStack

**File**: `recovery.go`

- `RecoveryEntry{Node *Node, UndoState UndoState}`
- `Unwind`: resolve slots for each entry's node, call
  `entry.Node.Action.Undo(ctx, slots, entry.UndoState)`

### 4. Update executor

**File**: `executor.go`

- Remove `registry` field from `GraphExecutor`
- Remove `registry` parameter from `NewGraphExecutor`
- `executeNode`: no lookup -- call `node.Action.Do(ctx, resolvedSlots)` directly
- Slot resolution lives on Node: `node.ResolvedSlots(results map[string]any) map[string]any`
  walks node.Slots, substitutes promise refs from results map, returns flat map.
  Both the executor and recovery stack call this method.
- `executeNode` sets `node.Status` directly (`StatusCompleted`/`StatusFailed`) --
  needed because phased execution does not call `ApplyResults` per-phase
- Store Result in `results` map for downstream resolution
- Push RecoveryEntry per node on success
- `runFlat`: create `results` map, pass to executeNode, build RecoveryStack
- `ExecutePhaseInner`: same pattern with per-phase results + recovery entries
- `fillSlotsFromData`: operates on resolved map instead of node

### 5. Add MustGet to ActionRegistry

**File**: `registry.go`

```go
func (r *ActionRegistry) MustGet(name string) Action {
    a, ok := r.actions[name]
    if !ok { panic("unregistered action: " + name) }
    return a
}
```

### 6. Update all 28 provider actions (new Do/Undo signatures)

**Files**: All 10 `provider/*/actions_gen.go` (manifest provider is deleted)

Each action's Do changes from:
```go
func (o *Link) Do(ctx *execution.Context, node *execution.Node) (...) {
    source := node.GetSlot("source").(string)
```
To:
```go
func (o *Link) Do(ctx *execution.Context, slots map[string]any) (...) {
    source := slots["source"].(string)
```

Content producers (file.Source, content.Literal) return content as Result
instead of calling ctx.StoreContent.

Content consumers (file.Copy, template.Render, encryption.Decrypt) read
`slots["content"]` instead of calling ctx.ContentFor.

Mode consumers (file.Copy, file.Write, file.Mkdir) read `slots["mode"]`.

### 7. Update flow actions

**Files**: `flow/choose.go`, `flow/gather.go`, `flow/elevate.go`

Same signature change. These are stubs that return nil.

### 8. Thread ActionRegistry through all builders + complete dotted names

All node construction sites change from:
```go
node := &execution.Node{ID: id, Action: "link"}
```
To:
```go
node := &execution.Node{ID: id, Action: reg.MustGet("file.link")}
```

This simultaneously completes dotted names (the registry key IS the dotted name).

**Graph builder** (`writ/graph_builder.go`):

- Thread `reg *ActionRegistry` into `BuildTree`, `ConfigureEngine`, all builder types
- `Action: ops[0]` -> `Action: reg.MustGet(ops[0])`
- Multi-op pipelines: add `SetSlotPromise("content", prevNode.ID, "")` on
  downstream nodes so content flows via slot resolution instead of the deleted
  content pipeline
- `node.Mode = f.Mode` -> `node.SetSlotImmediate("mode", f.Mode)` (when non-zero)

**writ/commands.go** upgrade pipeline:

- Same pattern: thread registry, set content slot promises between pipeline nodes
- `node.Mode = 0600` -> `node.SetSlotImmediate("mode", os.FileMode(0600))`

**Starlark plan bindings** (add `reg *ActionRegistry` to constructors):

| File | Changes |
|---|---|
| `starlark/plan.go` | Add `reg` to `NewPlanBindings`; ~17 Action strings -> `reg.MustGet()` |
| `starlark/plan_file.go` | ~4 Action strings -> `reg.MustGet()` |
| `starlark/plan_package.go` | ~4 Action strings -> `reg.MustGet()` |
| `starlark/plan_template.go` | ~1 Action string -> `reg.MustGet()` |
| `starlark/plan_encryption.go` | ~1 Action string -> `reg.MustGet()` |
| `starlark/plan_root.go` | ~5 Action strings -> `reg.MustGet()` |
| `starlark/plan_git_gen.go` | ~3 Action strings -> `reg.MustGet()` |
| `starlark/plan_archive_gen.go` | ~1 Action string -> `reg.MustGet()` |

**Platform bindings** (thread `reg` through base):

| File | Changes |
|---|---|
| `platform/common.go` | Add `reg` to `basePlanBindings`; 6 Action -> `reg.MustGet()` |
| `platform/darwin.go` | Thread `reg`; ~12 Action -> `reg.MustGet()` |
| `platform/linux.go` | Thread `reg`; ~11 Action -> `reg.MustGet()` |
| `platform/windows.go` | Thread `reg`; ~11 Action -> `reg.MustGet()` |

**Writ callers**:

| File | Changes |
|---|---|
| `writ/commands.go` | Thread registry; string comparisons -> `ActionName()` |
| `writ/graph_builder.go` | Remove registry from executor constructor |
| `writ/reconcile/reconcile.go` | String comparisons -> `ActionName()` |
| `writ/migrate/session.go` | Thread registry; "move" -> `reg.MustGet("file.move")` |
| `writ/migrate/execute.go` | "move" -> `ActionName() == "file.move"` |
| `writ/migrate/format.go` | Comparisons -> `ActionName()` |

**Lore callers**:

| File | Changes |
|---|---|
| `lore/commands.go` | Thread registry; remove from executor constructor |
| `lore/builder.go` | Thread registry; "package-install" -> `reg.MustGet("pkg.install")` |

**Execution callers**:

| File | Changes |
|---|---|
| `execution/stateview.go` | "copy"/"link" -> `ActionName()` comparisons |
| `execution/plan.go` | Switch on `ActionName()` (strings already dotted) |
| `execution/preflight.go` | Switch to `ActionName()` (strings already dotted) |
| `execution/graph.go` | `ComputeSummary` switch to `ActionName()` |

### 9. Remove delegate + manifest action, add Planner.PlanSequence

**Delete**: `internal/execution/provider/manifest/` (entire package)

**Delete**: `internal/manifest/builder.go` (SubgraphBuilder, BuildSubgraph, etc.)

**Edit**: `internal/execution/builder.go`
- Delete `SubgraphBuilder` interface, `ExpandDelegates()`, `isDelegateNode()`,
  `BuildOptions` struct

**Edit**: `internal/execution/provider/register_gen.go`
- Remove `manifest.Register(reg)` call and import

**Edit**: `internal/lore/builder.go`
- Add `Planner` struct (Registry, Host, Platform, Config)
- Add `Planner.PlanSequence(g, pipeline, entries)` — resolves each entry
  against the DevLore Registry, passes `entry.With` as features, calls
  existing `buildPackageNodes` for each resolved package
- `Build()` becomes a thin wrapper: creates Planner, calls PlanSequence

**Edit**: `internal/writ/graph_builder.go` (or tree builder)
- Accept `*lore.Planner` at construction time
- When encountering `packages-manifest.yaml`: call `manifest.Load()` then
  `planner.PlanSequence()` instead of creating delegate nodes

### 10. Update NewGraphExecutor call sites (remove registry param)

17 call sites drop the registry argument:

| File | Lines |
|---|---|
| `writ/graph_builder.go` | ~150 |
| `writ/commands.go` | ~524 |
| `writ/graph_test.go` | ~342 |
| `writ/migrate/session.go` | ~486 |
| `lore/commands.go` | ~240 |
| `lore/builder_test.go` | ~183, ~214 |
| `execution/phase_test.go` | ~135, ~228, ~363, ~429, ~491 |
| `execution/execution_test.go` | ~540, ~578, ~635, ~690, ~715, ~748, ~791 |

### 11. Update string comparison sites

All `node.Action == "..."` -> `node.ActionName() == "..."`:

| File | Comparisons |
|---|---|
| `writ/commands.go:704,707,711` | file.link, template.render |
| `execution/preflight.go:80` | file.link |
| `execution/stateview.go:99,108` | file.copy, file.link |
| `writ/migrate/execute.go:37,98` | file.move |
| `writ/migrate/session.go:347` | file.move |

### 12. Update tests

- `execution_test.go`: Construct nodes with real Action instances via registry
- `lore/builder_test.go`: Thread registry, update assertions
- `writ/graph_test.go`: Thread registry, update backup annotation test
- `writ/reconcile/reconcile_test.go`: "link"/"copy" -> "file.link"/"file.copy"
- `phase_test.go`: Construct nodes with Action instances, update recovery assertions
- `recovery_test.go`: Update RecoveryEntry construction (Node + UndoState)

### 13. Build and test

```bash
cd /path/to/worktree
go build ./...
go vet ./...
go test ./internal/execution/... -count=1
go test ./internal/starlark/... -count=1
go test ./internal/writ/... -count=1
go test ./internal/lore/... -count=1
```

## Order of Operations

1. **Steps 1-5**: Core type changes (Action interface, Node struct, RecoveryEntry, executor, MustGet)
2. **Steps 6-7**: Update all provider actions to new signatures
3. **Steps 8-9**: Thread registry through builders + remove delegate/manifest action + add Reader/PlanSequence
4. **Steps 10-11**: Update executor call sites and string comparisons
5. **Steps 12-13**: Tests, build, verify

## What This PR Does NOT Touch

- Phase-level compensation (compensating phases stay as is)
- No engine/build subpackage restructuring
- Checksums stay on Context (move to structured Result is a future PR)
- No new Starlark builtins or plan receiver APIs

## Critical Files

| File | Role |
|---|---|
| `execution/action.go` | Action interface -- contract change |
| `execution/graph.go` | Node struct -- Action field type + custom marshal |
| `execution/recovery.go` | RecoveryEntry redesign |
| `execution/executor.go` | Core dispatch + slot resolution + content pipeline removal |
| `execution/registry.go` | MustGet addition |
| `starlark/output.go` | SetSlotPromise / FillSlot -- drives executor slot resolution |
| `starlark/plan.go` | Largest plan bindings -- registry threading template |
| `platform/common.go` | Base platform bindings -- registry threading template |
| `provider/file/actions_gen.go` | Largest provider -- signature change template |
| `lore/builder.go` | Planner + PlanSequence replaces Build() loop |
| `writ/graph_builder.go` | Graph builder -- slot promise wiring for content pipelines |

## Deviations from Plan

### Unplanned additions

- **`Node.ResolvedSlots(results map[string]any) map[string]any`** — slot resolution
  lives on Node, not as a private executor helper. Both executor and recovery call it.
- **`StubAction(name string) Action`** — exported constructor for test packages that
  need to create stub actions outside the `execution` package.
- **`Build()` auto-defaults `ActionRegistry`** — `lore/builder.go` creates and populates
  a registry via `provider.RegisterAll` if `cfg.ActionRegistry` is nil, matching the
  existing pattern for `RegistryClient`.
- **`executeNode` sets `node.Status` directly** — phased execution does not call
  `ApplyResults` between phases, so the executor must mark nodes completed/failed
  as it goes.

### Scope changes

- **`flow.delegate` never created.** The plan's step 9 described creating it; the user
  directed that delegate should not exist. The delegate concept is fully eliminated:
  `nodeIsDelegate` removed from reconcile, all `flow.delegate` comparisons removed.
- **Step 9 (Planner.PlanSequence) deferred to Phase 2D.** The deletions from step 9
  (manifest provider, SubgraphBuilder, ExpandDelegates, isDelegateNode) were already
  done in prior work. The remaining piece -- adding `Planner` struct and
  `PlanSequence` method -- is deferred.

## Related Documents

- [Resource Provider Plan](../resource-provider.md) -- Parent plan
- [Phase 2A](./phase-2a.md) -- Provider extraction
- [Phase 2B](./phase-2b.md) -- Template/manifest providers
- [Convergence Operations](../../architecture/devlore-graph-convergence-operations.md) -- Flow action specs
