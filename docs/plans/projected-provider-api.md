# The Projected Provider API — Extraction Plan

## Status: Implemented

Implemented via type ownership extraction in the `feature/binding-unification`
branch. The original plan proposed interface-based decoupling (NodeHandle,
GraphHandle). The implementation uses **type ownership** instead: `pkg/projection/`
owns the concrete graph data model types directly. Since Output, Gather, and
FillSlot are in the same package as Node and Graph, no interfaces are needed.

## Context

The binding unification (Phases 1-9) established a clean pattern: one Go
Provider struct is the source of truth, and code generation produces Starlark
bindings for both immediate execution and deferred graph construction. The
projection machinery — `Receiver`, `Output`, `FillSlot`, type conversions,
node ID generation — is generic and reusable, but was buried in
`internal/starlark/` and `internal/execution/` where no external package can
import it.

Extracting this infrastructure into `pkg/projection/` formalizes the
"Projected Provider API": one Provider projected into two Starlark namespaces
(Immediate vs. Planned), with the projection layer decoupled from the execution
engine.

## Design

### Type Ownership (Not Interfaces)

`pkg/projection/` owns the concrete graph data model types directly. There are
no interfaces — `Output`, `Gather`, and `FillSlot` operate on `*Node` and
`*Graph` in the same package.

Key design decisions:

1. **`Node.Action` is `any`.** The `Action` interface (Do/Undo) stays in
   `internal/execution/`. The projection `Node` stores `any`. The executor
   type-asserts `node.Action.(Action)` before calling Do. `ActionName()` uses
   `interface{ Name() string }` duck-type assertion.

2. **`Hydrate`/`ApplyResults` are standalone functions.** `Graph.Hydrate` needs
   `ActionRegistry` (execution type) and `Graph.ApplyResults` needs `NodeResult`
   (execution type). Both live in `internal/execution/` as `HydrateGraph()` and
   `ApplyResults()` to prevent `pkg/projection/` from importing execution.

3. **Zero back-imports.** `pkg/projection/` imports only stdlib +
   `go.starlark.net` + `gopkg.in/yaml.v3`. Never `internal/`.

### Package Boundary

```
pkg/projection/          ← public, no internal/ imports
  graph.go               ← Node, Graph, Edge, SlotValue, state/status types,
                            serialization, checksums, StubAction
  phase.go               ← Phase, RetryPolicy, BackoffStrategy, RollbackEntry
  output.go              ← Output, Gather, FillSlot, NewOutput, NewGather
  receiver.go            ← Receiver base type, MakeAttr, helpers
  convert.go             ← Starlark ↔ Go type conversions
  access.go              ← Access level constants
  nodeid.go              ← GenerateNodeID (atomic counter)

internal/execution/      ← owns Action interface, executor, registry
  graph.go               ← HydrateGraph(), ApplyResults() standalone functions
  action.go              ← Action, CompensableAction, Context (Graph: *projection.Graph)
  executor.go            ← GraphExecutor (type-asserts node.Action.(Action))
  recovery.go            ← RecoveryEntry (Node: *projection.Node)

internal/starlark/       ← imports both; devlore-specific wiring
  plan_root.go           ← PlanRoot (choose, gather, source)
  plan_registry.go       ← PlanFactory type and registry
  output.go              ← FillSlot bridge (var FillSlot = projection.FillSlot)
  plan_*_gen.go          ← generated plan receivers (use projection types)
  receiver_*_gen.go      ← generated realtime receivers
```

### What Moved

| Source | Destination | Notes |
|--------|-------------|-------|
| `internal/execution/graph.go` types | `pkg/projection/graph.go` | Node, Graph, Edge, SlotValue, state/status constants, serialization, checksums. `Node.Action` field: `Action` interface → `any` |
| `internal/execution/phase.go` types | `pkg/projection/phase.go` | Phase, RetryPolicy, BackoffStrategy, RollbackEntry, Attempt |
| `internal/starlark/output.go` (Output, Gather, FillSlot) | `pkg/projection/output.go` | Operate on `*Node` and `*Graph` directly (same package) |
| `internal/starlark/output.go` (type conversions) | `pkg/projection/convert.go` | GoToStarlarkValue, StarlarkValueToGo, etc. |
| `internal/starlark/receiver.go` | `pkg/projection/receiver.go` | Receiver, MakeAttr, NoSuchAttrError, BuiltinFunc |
| `internal/starlark/plan.go` (generateNodeID) | `pkg/projection/nodeid.go` | Exported as GenerateNodeID |
| Access constants | `pkg/projection/access.go` | AccessImmediate, AccessPlanned, AccessBoth |

### What Stays in `internal/execution/`

| Type | Why |
|------|-----|
| `Action`, `CompensableAction` | Define the Do/Undo contract — execution concern |
| `Context` | Provides DryRun, Writer, Graph to actions — execution concern |
| `ActionRegistry` | Maps action names to Action implementations |
| `GraphExecutor` | Runs the graph — orchestration logic |
| `RecoveryStack`, `RecoveryEntry` | Saga rollback (Node field is `*projection.Node`) |
| `NodeResult`, `ResultStatus` | Execution outcomes |
| `HydrateGraph()` | Replaces stubs with real Actions from registry |
| `ApplyResults()` | Updates graph nodes with execution results |

### How Generated Code Works

```go
import (
    "github.com/NobleFactor/devlore-cli/internal/execution"
    "github.com/NobleFactor/devlore-cli/pkg/projection"
)

type FilePlan struct {
    projection.Receiver             // from pkg/projection
    graph *projection.Graph         // concrete type, same package as Output
    reg   *execution.ActionRegistry // stays in execution
}

func (p *FilePlan) link(...) {
    node := &projection.Node{
        ID:     projection.GenerateNodeID("file-link"),
        Action: p.reg.MustGet("file.link"),
    }
    FillSlot(node, p.graph, "source", source)           // bridge alias
    return projection.NewOutput(node, p.graph, ""), nil
}
```

Generated code creates concrete `*projection.Node` values and calls
`projection.FillSlot()` / `projection.NewOutput()` on them. No interfaces
needed — everything is in the same package.

The `FillSlot` bridge alias in `internal/starlark/output.go` exists because the
code generator template (`planFillSlots`) emits bare `FillSlot()` calls. The
alias delegates to `projection.FillSlot`.

### Template Changes

**`plan_receiver.go.template`:**
- `Receiver` → `projection.Receiver`
- `NewReceiver` → `projection.NewReceiver`
- `MakeAttr` → `projection.MakeAttr`
- `NoSuchAttrError` → `projection.NoSuchAttrError`
- `FillSlot` → `projection.FillSlot` (via bridge alias)
- `NewOutput` → `projection.NewOutput`
- `generateNodeID` → `projection.GenerateNodeID`
- `execution.Node` → `projection.Node`
- `execution.Graph` → `projection.Graph`

**`realtime_receiver.go.template`:**
- Same prefix changes for Receiver, NewReceiver, MakeAttr, NoSuchAttrError

**`graph_actions.go.template`:**
- No changes — works with execution types directly

## Access Level Directives

Current state: struct-level `//devlore:plannable` (binary: plannable or not).

Future evolution (method-level granularity):
```go
const (
    AccessImmediate = "immediate"  // execute during script eval, return value
    AccessPlanned   = "planned"    // defer to graph, return handle
    AccessBoth      = "both"       // available in both projections (default)
)
```

The initial extraction preserves the existing `//devlore:plannable` directive.
Method-level `//devlore:access=immediate|planned|both` is a future phase that
builds on this foundation.

## Future Enhancements (not in scope)

1. **Method-level access directives**: `//devlore:access=immediate|planned|both`
   per method, parsed by generator, controlling which projection surface each
   method appears on
2. **Explicit Bind API**: `registry.Bind("pkg", pkg.Provider, Immediate|Planned)`
   replacing init-based self-registration with consumer-driven subscription
3. **Provider registration unification**: Single `Registry` that drives both
   plan factory and realtime receiver construction from the same Provider
