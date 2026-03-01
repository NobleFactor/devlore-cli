# The Projected Provider API — Extraction Plan

## Context

The binding unification (Phases 1-8) established a clean pattern: one Go
Provider struct is the source of truth, and code generation produces Starlark
bindings for both immediate execution and deferred graph construction. The
projection machinery — `Receiver`, `Output`, `FillSlot`, type conversions,
node ID generation — is generic and reusable, but currently buried in
`internal/starlark/` where no external package can import it.

Extracting this infrastructure into `pkg/projection/` formalizes the
"Projected Provider API" described in `draft-subscriber-model.md`: one Provider
projected into two Starlark namespaces (Immediate vs. Planned), with the
projection layer decoupled from the execution engine.

This is a design-now, execute-after-Part-23 effort.

## Design

### The Boundary

`pkg/projection/` provides Starlark binding infrastructure and defines
**interfaces** for graph interaction. `internal/execution/` implements those
interfaces. Generated code in `internal/starlark/` imports from both.

```
pkg/projection/          ← public, no internal/ imports
  graph.go               ← NodeHandle, GraphHandle interfaces
  receiver.go            ← Receiver base, MakeAttr, helpers
  output.go              ← Output, Gather, FillSlot
  convert.go             ← Starlark ↔ Go type conversions
  access.go              ← Access level constants + directive parsing
  nodeid.go              ← GenerateNodeID (atomic counter)

internal/execution/      ← implements projection interfaces
  graph.go               ← Node implements NodeHandle; Graph implements GraphHandle

internal/starlark/       ← imports both pkg/projection/ and internal/execution/
  plan_root.go           ← PlanRoot (devlore-specific: host, choose, gather)
  plan_registry.go       ← PlanFactory (devlore-specific factory signature)
  plan_*_gen.go          ← generated receivers (use projection base types)
  receiver_*_gen.go      ← generated realtime receivers
```

### Interfaces (`pkg/projection/graph.go`)

```go
// NodeHandle abstracts a graph node for the projection layer.
type NodeHandle interface {
    NodeID() string
    GetSlot(name string) any
    SetSlotImmediate(name string, value any)
    SetSlotPromise(name string, nodeRef string, slot string)
    SlotNames() []string
}

// GraphHandle abstracts graph edge creation for the projection layer.
type GraphHandle interface {
    AddEdge(from, to string)
}

// RetryConfig defines retry policy (set from Starlark, consumed by executor).
type RetryConfig struct {
    MaxAttempts  int
    Backoff      string // "none", "linear", "exponential"
    InitialDelay string
    MaxDelay     string
}

// RetryConfigurable allows setting retry policy on a node.
type RetryConfigurable interface {
    SetRetry(config RetryConfig)
}
```

`execution.Node` satisfies `NodeHandle` + `RetryConfigurable`.
`execution.Graph` satisfies `GraphHandle`.

### What Moves

| Source | Destination | Key changes |
|--------|-------------|-------------|
| `internal/starlark/receiver.go` | `pkg/projection/receiver.go` | Export names: `Receiver`, `MakeAttr`, `NoSuchAttrError`, `BuiltinFunc`, `ListToStringSlice` |
| `internal/starlark/output.go` (Output, Gather, FillSlot) | `pkg/projection/output.go` | Replace `*execution.Node` → `NodeHandle`, `*execution.Graph` → `GraphHandle`. Export: `Output`, `Gather`, `FillSlot`, `NewOutput`, `NewGather`, `ResolveInput` |
| `internal/starlark/output.go` (type conversions) | `pkg/projection/convert.go` | Export: `GoToStarlarkValue`, `StarlarkValueToGo`, `StarlarkListToSlice`, `StarlarkDictToMap` |
| `generateNodeID` (currently in internal/starlark/) | `pkg/projection/nodeid.go` | Export: `GenerateNodeID` |

### What Stays in `internal/starlark/`

| File | Why |
|------|-----|
| `plan_root.go` | PlanRoot is devlore-specific: `host.Host`, `choose()` creates `execution.Phase`, `source()` and `gather()` are graph primitives tied to the execution model |
| `plan_registry.go` | `PlanFactory` signature references `*execution.Graph`, `host.Host`, `*execution.ActionRegistry` — all internal types |
| `plan_*_gen.go` | Generated. Create `*execution.Node` literals (concrete type), but use `projection.FillSlot()`, `projection.NewOutput()`, `projection.Receiver` |
| `receiver_*_gen.go` | Generated realtime receivers |
| `bindings.go` | Global wiring |
| `builder.go` | Script execution environment |

### How Generated Code Changes

Before:
```go
import "github.com/NobleFactor/devlore-cli/internal/execution"

type FilePlan struct {
    Receiver                        // from internal/starlark
    graph *execution.Graph
    ...
}

func (p *FilePlan) link(...) {
    node := &execution.Node{ID: generateNodeID("file-link"), ...}
    FillSlot(node, p.graph, "source", source)     // internal/starlark.FillSlot
    return NewOutput(node, p.graph, ""), nil       // internal/starlark.NewOutput
}
```

After:
```go
import (
    "github.com/NobleFactor/devlore-cli/internal/execution"
    "github.com/NobleFactor/devlore-cli/pkg/projection"
)

type FilePlan struct {
    projection.Receiver             // from pkg/projection
    graph *execution.Graph
    ...
}

func (p *FilePlan) link(...) {
    node := &execution.Node{ID: projection.GenerateNodeID("file-link"), ...}
    projection.FillSlot(node, p.graph, "source", source)
    return projection.NewOutput(node, p.graph, ""), nil
}
```

The generated code still creates concrete `*execution.Node` values (it imports
`internal/execution/`). But it passes them to `projection.FillSlot()` and
`projection.NewOutput()` which accept `NodeHandle`/`GraphHandle` interfaces.
This works because `*execution.Node` satisfies `NodeHandle`.

### Access Level Directives (`pkg/projection/access.go`)

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
builds on this foundation. `access.go` defines the constants and provides a
`ParseAccessDirective(docComment string) string` utility for generators to use
when the method-level directives are implemented.

### Interface Implementation (changes to `internal/execution/graph.go`)

Add methods to satisfy `projection.NodeHandle`:
```go
func (n *Node) NodeID() string                                        { return n.ID }
func (n *Node) GetSlot(name string) any                               { /* existing logic */ }
func (n *Node) SlotNames() []string                                   { /* keys of n.Slots */ }
// SetSlotImmediate and SetSlotPromise already exist
```

Add method to satisfy `projection.GraphHandle`:
```go
func (g *Graph) AddEdge(from, to string) {
    g.Edges = append(g.Edges, Edge{From: from, To: to})
}
```

Add method to satisfy `projection.RetryConfigurable`:
```go
func (n *Node) SetRetry(config projection.RetryConfig) {
    n.Retry = &RetryPolicy{
        MaxAttempts:  config.MaxAttempts,
        Backoff:      parseBackoff(config.Backoff),
        InitialDelay: config.InitialDelay,
        MaxDelay:     config.MaxDelay,
    }
}
```

### Template Changes

All three templates updated:

**`plan_receiver.go.template`:**
- Add `"github.com/NobleFactor/devlore-cli/pkg/projection"` import
- `Receiver` → `projection.Receiver`
- `NewReceiver` → `projection.NewReceiver`
- `MakeAttr` → `projection.MakeAttr`
- `NoSuchAttrError` → `projection.NoSuchAttrError`
- `FillSlot` → `projection.FillSlot`
- `NewOutput` → `projection.NewOutput`
- `generateNodeID` → `projection.GenerateNodeID`

**`realtime_receiver.go.template`:**
- Same import/prefix changes for `Receiver`, `NewReceiver`, `MakeAttr`,
  `NoSuchAttrError`, `ListToStringSlice`

**`graph_actions.go.template`:**
- No changes (works with `execution` types directly, no projection dependency)

## Migration Steps

### Step 1: Create `pkg/projection/` package

Create the 6 source files. Move code from `internal/starlark/receiver.go` and
`internal/starlark/output.go`, adapting to use interfaces and exported names.

### Step 2: Implement interfaces on `execution` types

Add `NodeID()`, `SlotNames()`, `AddEdge()`, `SetRetry()` methods to
`execution.Node` and `execution.Graph`.

### Step 3: Update `internal/starlark/` to import projection

- Delete `receiver.go` and `output.go` from `internal/starlark/`
- Update `plan_root.go` to import and use `projection.*`
- Update `plan_registry.go` if needed (PlanFactory signature stays, but
  internal references to moved types change)

### Step 4: Update templates

Add `projection` import, prefix all moved symbols. Rebuild `star` binary.

### Step 5: Regenerate all `_gen.go` files

Regenerate all 10 `plan_*_gen.go` and all realtime receivers with updated
templates.

### Step 6: Update `internal/lore/builder.go` and other consumers

Any code that references moved types (e.g., `starlark.NewOutput`,
`starlark.FillSlot`) must switch to `projection.*`.

### Step 7: Build and test

```
go build ./...
go test ./...
star devlore knowledge extract --domain all
```

## Files Modified

| File | Action |
|------|--------|
| `pkg/projection/graph.go` | **Create** — interfaces |
| `pkg/projection/receiver.go` | **Create** — from `internal/starlark/receiver.go` |
| `pkg/projection/output.go` | **Create** — from `internal/starlark/output.go` |
| `pkg/projection/convert.go` | **Create** — type conversions from output.go |
| `pkg/projection/access.go` | **Create** — access level constants |
| `pkg/projection/nodeid.go` | **Create** — GenerateNodeID |
| `internal/starlark/receiver.go` | **Delete** |
| `internal/starlark/output.go` | **Delete** |
| `internal/execution/graph.go` | **Modify** — add interface methods |
| `internal/starlark/plan_root.go` | **Modify** — import projection |
| `internal/starlark/plan_registry.go` | **Modify** — import projection |
| `internal/starlark/plan_*_gen.go` (10) | **Regenerate** |
| `internal/starlark/receiver_*_gen.go` | **Regenerate** |
| `star/.../templates/plan_receiver.go.template` | **Modify** — projection prefix |
| `star/.../templates/realtime_receiver.go.template` | **Modify** — projection prefix |

## Future Enhancements (not in scope)

1. **Method-level access directives**: `//devlore:access=immediate|planned|both`
   per method, parsed by generator, controlling which projection surface each
   method appears on
2. **Explicit Bind API**: `registry.Bind("pkg", pkg.Provider, Immediate|Planned)`
   replacing init-based self-registration with consumer-driven subscription
3. **Provider registration unification**: Single `Registry` that drives both
   plan factory and realtime receiver construction from the same Provider
