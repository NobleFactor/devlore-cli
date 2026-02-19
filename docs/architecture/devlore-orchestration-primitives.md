# Orchestration Primitives

This document describes the refined orchestration primitives that transform flow
actions from passthrough stubs into first-class runtime operations. The graph
remains self-contained — every decision is a visible node with traceable inputs
and outputs.

See also:

- [Execution Graph](devlore-execution-graph.md) — Core graph architecture
- [Graph Operations](devlore-graph-convergence-operations.md) — Convergence and
  control flow (superseded in part — see [Supersession Table](#supersession-table))
- [Phase Execution](devlore-phase-execution.md) — Saga pattern, phases, retry/rollback
  (superseded in part — see [Supersession Table](#supersession-table))
- [Typed Slots](devlore-typed-slots.md) — Slot model and resolution chain
  (extended — see [Supersession Table](#supersession-table))
- [Emergent System Model](devlore-emergent-system-model.md) — System-level architecture,
  dependency taxonomy (structural, functional, procedural)
- [Action Namespaces](devlore-operation-namespaces.md) — How to add action namespaces

Tracking issue: TBD

---

## Design Principle

**The graph is self-contained.** No hidden runtime queries. Every decision the
executor makes is rooted in a graph node whose inputs and outputs are traceable.
The plan shows what will happen and why. The receipt shows what happened and why.

Orchestration primitives — Gather, Choose, WaitUntil, Sidecar — are visible
runtime operations, not engine internals. Each appears as one or more nodes in
the graph, is recorded in dry-run output, and is preserved in the receipt. The
executor does not distinguish between "resource actions" and "flow actions" at
the dispatch level — both implement the Action interface and receive resolved
slots.

---

## Vocabulary

The execution model uses three layers, each with distinct lifecycle and
mutability characteristics.

### Three-Layer Model

| Layer | Types | Lifecycle |
|---|---|---|
| **Definition** | Node, Phase, Edge, SlotValue | Plan-time, immutable during execution |
| **Activation** | ActivationState | Per-execution, mutable, transient — discarded after results and undo state are captured |
| **Recovery** | UndoState, RecoveryEntry, RecoveryStack | Durable until rollback completes or graph succeeds |

**Definition layer.** The graph structure — nodes, phases, edges, and slot
values — is built at plan time by Starlark scripts and the graph builder. Once
execution begins, the definition layer is immutable. Nodes are never cloned or
mutated during execution.

**Activation layer.** Per-execution mutable state that tracks node progress
during a single execution pass. For non-gather execution, activation state is
inlined on the Node struct (the existing `Status`, `SourceChecksum`,
`TargetChecksum` fields). For gather's concurrent execution, ActivationState
lives in a per-iteration map — nodes are shared and never mutated.

```go
type ActivationState struct {
    Status         NodeStatus
    Timestamp      string
    Error          string
    SourceChecksum string
    TargetChecksum string
}
```

**Recovery layer.** State that persists beyond a single execution pass for the
purpose of rollback. UndoState is captured by each action's `Do` method and
stored in RecoveryEntry on the RecoveryStack. Recovery state is durable — it
survives until rollback completes or the graph succeeds.

---

## Phase as Universal Boundary

There is no Subtree type. **Phase is the universal boundary** — a single type
used for saga steps, compensation, gather bodies, and choose branches.
Distinguished by how they're referenced, not by structure.

| Usage | Referenced by | Executed by |
|---|---|---|
| Saga step | `Graph.Phases` order | `RunPhased` forward walk |
| Compensation | Another phase's `Compensate` field | `RunPhased` unwind |
| Gather body | Gather node's `do` slot (phase ID) | Gather's per-item loop |
| Choose branch | Choose node's `cases`/`default` slots | Choose after predicate match |

All four usages execute via `ExecutePhaseInner`. All four get retry policy.

Gather-body and choose-branch phases appear in `Graph.Phases` but are skipped
during the forward saga walk — same pattern as compensating phases. The executor
identifies them by checking whether they're referenced by a gather or choose
node rather than by position in the phase list.

### Phase Execution Contract

Regardless of usage, every phase execution follows the same contract:

1. Collect the phase's nodes from the graph (via `Phase.NodeIDs`)
2. Order nodes topologically within the phase
3. Execute nodes sequentially, resolving slots from the results map
4. Push each completed node onto the recovery stack
5. On failure, unwind the phase's recovery stack entries

---

## SlotProxy

SlotProxy is the third SlotValue variant, enabling N concurrent instances with
isolated slot resolution. It is the mechanism by which gather body phases
reference per-iteration values without mutating shared nodes.

### Go Types

```go
type SlotValue struct {
    // Immediate is the direct value (any type, known at analysis time).
    Immediate any    `json:"immediate,omitempty" yaml:"immediate,omitempty"`

    // NodeRef is the ID of the node that produces this value (promise).
    NodeRef   string `json:"node_ref,omitempty"  yaml:"node_ref,omitempty"`

    // Slot is which output slot of the referenced node (empty = default output).
    Slot      string `json:"slot,omitempty"      yaml:"slot,omitempty"`

    // GatherRef is the gather node ID for proxy resolution.
    GatherRef string `json:"gather_ref,omitempty" yaml:"gather_ref,omitempty"`

    // Field is the field name to access on the proxy item.
    Field     string `json:"field,omitempty"      yaml:"field,omitempty"`
}

func (s SlotValue) IsImmediate() bool { return !s.IsPromise() && !s.IsProxy() }
func (s SlotValue) IsPromise() bool   { return s.NodeRef != "" }
func (s SlotValue) IsProxy() bool     { return s.GatherRef != "" }
```

Three variants, mutually exclusive:

| Variant | Fields set | Resolution |
|---|---|---|
| Immediate | `Immediate` | Value used directly |
| Promise | `NodeRef` (+ optional `Slot`) | Resolved from upstream node's Result at execution time |
| Proxy | `GatherRef` + `Field` | Resolved from per-iteration proxy context at gather execution time |

### Plan-Time Recording

The `do` lambda receives a proxy object (`starlark.HasAttrs`). Field access
records the path and returns a `FieldRef` marker. The plan receiver stores it
as `SlotValue{GatherRef, Field}`. The lambda builds nodes — it does not compute
values. Computed values use template nodes in the subtree.

```python
plan.gather(
    items = servers,
    do = lambda item: plan.sequence(item, [
        plan.service.stop(item.name),     # → {gather_ref: "g1", field: "name"}
        plan.service.start(item.name),
    ]),
    policy = plan.concurrency(limit=10)
)
```

When the Starlark runtime evaluates `item.name`, the proxy object records the
access path and returns a `FieldRef` marker. The plan receiver for
`plan.service.stop()` sees the `FieldRef` and creates the node with:

```go
node.SetSlotProxy("name", gatherID, "name")
```

### Execution-Time Resolution

`ResolvedSlots` gains an optional proxy context parameter. The proxy context
maps gather IDs to the current iteration's item value:

```go
func (n *Node) ResolvedSlots(results map[string]any, proxyCtx ...map[string]any) map[string]any {
    slots := make(map[string]any, len(n.Slots))
    for name, sv := range n.Slots {
        switch {
        case sv.IsProxy():
            if len(proxyCtx) > 0 {
                if item, ok := proxyCtx[0][sv.GatherRef]; ok {
                    slots[name] = fieldAccess(item, sv.Field)
                }
            }
        case sv.IsPromise():
            if results != nil {
                if val, ok := results[sv.NodeRef]; ok {
                    slots[name] = val
                }
            }
        case sv.IsImmediate():
            slots[name] = sv.Immediate
        }
    }
    return slots
}
```

For non-gather execution, `proxyCtx` is not passed and proxy slots resolve to
nil (a validation error if encountered). This ensures proxy slots only appear
in gather body phases.

### Serialized Form

Proxy slots serialize naturally to YAML/JSON:

```yaml
- id: stop-svc
  action: service.stop
  slots:
    name: {gather_ref: patch-servers, field: name}
```

The serialized form is a complete record of the binding — the gather node that
provides the item and the field to extract. No runtime state is needed to
understand the plan.

---

## Gather as Parallel Comprehension

Gather transforms from a simple AND-join into a parallel comprehension — an
iterator that executes a phase body once per item, with configurable concurrency.

### Loop Semantics

```python
plan.gather(
    items = servers,
    do = lambda item: plan.sequence(item, [
        plan.service.stop(item.name),
        plan.service.start(item.name),
    ]),
    policy = plan.concurrency(limit=10)
)
```

The `items` slot provides a list (immediate or promise). The `do` slot
references a phase ID — the phase body to execute per item. The `policy` slot
configures concurrency limits and failure behavior.

### Phase Reference

The `do` lambda at plan time produces a phase — a set of nodes registered in
`Graph.Phases`. At execution time, the gather node's `do` slot holds the phase
ID (string). The gather action looks up the phase, collects its nodes, and
executes them per item.

Gather-body phases appear in `Graph.Phases` but are marked for skip during the
forward saga walk. They are only executed by the gather action.

### Concurrency via Shared Nodes

Gather executes phases concurrently with shared immutable nodes and
per-iteration ActivationState maps. Nodes are never cloned.

```go
func (a *Gather) executeConcurrent(ctx *Context, graph *Graph, phase *Phase,
    items []any, gatherID string, limit int) ([]any, error) {

    sem := make(chan struct{}, limit)
    phaseNodes := collectPhaseNodes(graph, phase) // shared, never mutated

    results := make([]any, len(items))
    errors  := make([]error, len(items))

    var wg sync.WaitGroup
    for i, item := range items {
        wg.Add(1)
        sem <- struct{}{}
        go func(idx int, val any) {
            defer wg.Done()
            defer func() { <-sem }()

            // Per-iteration isolation
            state    := make(map[string]*ActivationState)
            nodeResults := make(map[string]any)
            stack    := &RecoveryStack{}
            proxyCtx := map[string]any{gatherID: val}

            for _, node := range ordered(phaseNodes, phase) {
                slots := node.ResolvedSlots(nodeResults, proxyCtx)
                fillSlotsFromData(slots, ctx.Data)
                result, undoState, err := node.Action.Do(ctx, slots)
                if err != nil {
                    state[node.ID] = &ActivationState{
                        Status: StatusFailed,
                        Error:  err.Error(),
                    }
                    errors[idx] = err
                    // Unwind this iteration's stack
                    stack.Unwind(ctx, nodeResults)
                    return
                }
                if result != nil {
                    nodeResults[node.ID] = result
                }
                stack.Push(RecoveryEntry{Node: node, UndoState: undoState})
                state[node.ID] = &ActivationState{Status: StatusCompleted}
            }
            // Capture terminal node result
            results[idx] = terminalResult(phase, nodeResults)
            // state map discarded — results and undo state captured
        }(i, item)
    }
    wg.Wait()
    // ...
}
```

Key invariants:

- **Shared nodes**: `phaseNodes` is read-only. All iterations reference the same
  node objects. No cloning, no mutation.
- **Per-iteration state**: Each goroutine has its own `state`, `nodeResults`,
  `stack`, and `proxyCtx`. No shared mutable state between iterations.
- **Functional return**: The terminal node of the phase provides the iteration's
  output via its Result. The state map is discarded after results are captured.

### GatherUndoState

Gather's `Do` returns a `GatherUndoState` as its UndoState. This preserves
what's needed for rollback while the transient ActivationState is discarded:

```go
type GatherUndoState struct {
    Iterations []IterationUndo
}

type IterationUndo struct {
    ProxyCtx map[string]any  // {gatherID: item} for slot re-resolution
    Results  map[string]any  // node results for promise re-resolution
    Entries  []RecoveryEntry // nodes (shared refs) + per-node undo state
}
```

`Gather.Undo` walks iterations in reverse, re-resolves slots with each
iteration's saved proxy context, and calls `Action.Undo` on each entry:

```go
func (a *Gather) Undo(ctx *Context, slots map[string]any, state UndoState) error {
    gs, ok := state.(*GatherUndoState)
    if !ok || gs == nil {
        return nil
    }
    var errs []error
    // Reverse iteration order
    for i := len(gs.Iterations) - 1; i >= 0; i-- {
        iter := gs.Iterations[i]
        for j := len(iter.Entries) - 1; j >= 0; j-- {
            entry := iter.Entries[j]
            entrySlots := entry.Node.ResolvedSlots(iter.Results, iter.ProxyCtx)
            fillSlotsFromData(entrySlots, ctx.Data)
            if err := entry.Node.Action.Undo(ctx, entrySlots, entry.UndoState); err != nil {
                errs = append(errs, err)
            }
        }
    }
    return errors.Join(errs...)
}
```

Recovery entries reference the stable shared nodes — no lifecycle issue. The
proxy context and node results are saved per iteration, enabling slot
re-resolution during undo without access to transient activation state.

---

## Choose as Predicate Branching

Choose transforms from a static criteria-based selector into a predicate-driven
branch selector with phase-level execution.

### Structure

```python
plan.choose(
    input = server_status,
    cases = [
        (runtime.predicate(lambda s: s == "OFF"), plan.phase_start_server),
        (runtime.predicate(lambda s: s == "ON"),  plan.phase_nop),
    ],
    default = plan.phase_fail_unknown
)
```

Each case is a `(predicate, phase_id)` pair. The choose node evaluates
predicates against the resolved `input` slot. The first matching predicate's
phase is executed. If no predicate matches, the `default` phase runs.

### Execution

1. Resolve the `input` slot (immediate or promise)
2. Evaluate each case predicate against the input value
3. Execute the matched phase via `ExecutePhaseInner`
4. Return the branch phase's terminal node Result as the choose node's Result
5. All unmatched phases are skipped

### Promise Return

The choose node returns a promise — the Result of whichever branch was
selected. Downstream nodes reference the choose node's output via promise
slots, not the individual branch phases. This maintains the graph contract:
edges point to the choose node, and the choose node provides the value.

### Undo

Choose's undo delegates to the selected branch's recovery stack. Only the
executed branch has recovery entries — unmatched branches were never executed
and have nothing to undo.

---

## WaitUntil

WaitUntil is an event-driven sensor — a synchronization primitive that pauses
execution until an external condition is met.

### Structure

```python
plan.wait_until(
    target = database_node,
    predicate = runtime.predicate(lambda db: db.is_ready),
    timeout = "5m",
    interval = "10s"
)
```

### Execution

1. Resolve the `target` slot (promise — the upstream node whose Result is polled)
2. Start a polling loop with the configured `interval`
3. On each tick, evaluate the `predicate` against the target's current value
4. If the predicate returns true, the WaitUntil node completes with the
   target value as its Result
5. If `timeout` expires, the node fails with a timeout error

### Slots

| Slot | Type | Description |
|---|---|---|
| `target` | Promise (node ref) | The node whose Result is polled |
| `predicate` | RuntimePredicate | Condition to evaluate |
| `timeout` | string (duration) | Maximum wait time (e.g., `"5m"`) |
| `interval` | string (duration) | Poll interval (e.g., `"10s"`, default `"5s"`) |

### Undo

WaitUntil is not compensable — it observes state but does not modify it.
`Undo` returns nil.

---

## Sidecar

Sidecar provides lifecycle hooks for observability — non-intrusive telemetry
that attaches to nodes without altering core logic.

### LifecycleHook Interface

```go
type LifecycleHook interface {
    OnNodeStart(ctx *Context, nodeID string, slots map[string]any)
    OnNodeComplete(ctx *Context, nodeID string, result Result, err error)
    OnPhaseStart(ctx *Context, phaseID string)
    OnPhaseComplete(ctx *Context, phaseID string, err error)
}
```

### HookRegistry

```go
type HookRegistry struct {
    hooks []LifecycleHook
}

func (r *HookRegistry) Register(hook LifecycleHook)
func (r *HookRegistry) FireNodeStart(ctx *Context, nodeID string, slots map[string]any)
func (r *HookRegistry) FireNodeComplete(ctx *Context, nodeID string, result Result, err error)
func (r *HookRegistry) FirePhaseStart(ctx *Context, phaseID string)
func (r *HookRegistry) FirePhaseComplete(ctx *Context, phaseID string, err error)
```

### Executor Instrumentation

The executor fires hook events at phase and node boundaries:

```go
func (e *GraphExecutor) executeNode(ctx *Context, node *Node, ...) *NodeResult {
    e.hooks.FireNodeStart(ctx, node.ID, slots)
    result, undoState, err := node.Action.Do(ctx, slots)
    e.hooks.FireNodeComplete(ctx, node.ID, result, err)
    // ...
}
```

Hooks are fire-and-forget. A hook failure is logged but does not fail the node
or phase. Hooks run synchronously within the executor's goroutine — they must
not block.

### Starlark Integration

```python
plan.install(s, on_step=lambda msg: plan.log_to_stdout(msg))
```

At plan time, sidecar hooks registered via Starlark are stored as node
metadata. The executor reads the metadata and registers ephemeral hooks for
the node's execution lifetime.

---

## Runtime Predicates

Runtime predicates bridge plan-time Starlark lambdas and execution-time Go
evaluation. They live in the `runtime.predicate` namespace.

### RuntimePredicate Type

```go
type RuntimePredicate struct {
    Callable starlark.Callable // The Starlark lambda
    Source   string            // Source representation for display/debug
}
```

### Plan-Time vs Execution-Time

| Concern | Plan-time lambda | Runtime predicate |
|---|---|---|
| When it runs | During Starlark script execution | During Go executor execution |
| What it does | Builds graph structure (nodes, edges, slots) | Evaluates a condition against resolved data |
| Language | Pure Starlark | Starlark callable invoked from Go |
| Example | `lambda item: plan.service.stop(item.name)` | `runtime.predicate(lambda s: s == "OFF")` |

The `runtime.*` namespace is the signal that a function runs at execution time,
not plan time. Plan-time lambdas organize the graph. Runtime predicates make
decisions within the graph.

### Evaluation

The executor evaluates runtime predicates by calling the stored
`starlark.Callable` with the resolved input value:

```go
func (p *RuntimePredicate) Eval(thread *starlark.Thread, input starlark.Value) (bool, error) {
    result, err := starlark.Call(thread, p.Callable, starlark.Tuple{input}, nil)
    if err != nil {
        return false, err
    }
    return bool(result.Truth()), nil
}
```

### Usage

Runtime predicates appear in:

- **Choose**: Case predicates evaluate against the `input` slot
- **WaitUntil**: The polling predicate evaluates against the target's Result
- **Gather** (optional `where` clause): Filter predicate evaluates per item

---

## Serialization

### What Serializes

| Type | Serializes | Format |
|---|---|---|
| SlotValue (Immediate) | Yes | `{immediate: value}` |
| SlotValue (Promise) | Yes | `{node_ref: id, slot: name}` |
| SlotValue (Proxy) | Yes | `{gather_ref: id, field: name}` |
| Phase | Yes | ID, name, status, node IDs, compensate ref |
| ActivationState | No | Transient — discarded after capture |
| GatherUndoState | Yes (in recovery) | Iterations with proxy contexts and entries |
| RecoveryEntry | Yes (in recovery) | Node ref + undo state |

### What Doesn't Serialize

| Type | Reason |
|---|---|
| RuntimePredicate | Contains `starlark.Callable` — not serializable. Predicates are re-created from source when a graph is reloaded. |
| ActivationState | Transient per-execution state. Reconstructed on each execution pass. |
| LifecycleHook | Runtime callback — registered at execution time, not plan time. |

Runtime predicates are the key serialization boundary. A saved plan/receipt
records the predicate's source representation (for display) but not the
callable. Reloading a graph from YAML/JSON produces nodes with predicate
source strings that must be re-compiled from the original Starlark scripts
before execution.

---

## Supersession Table

This document supersedes or extends specific sections of existing architecture
documents. The originals remain authoritative for all sections not listed here.

| Existing Document | Section | Status | Notes |
|---|---|---|---|
| [devlore-graph-convergence-operations.md](devlore-graph-convergence-operations.md) | Gather (AND) | **Superseded** | Gather is now a parallel comprehension with phase body, proxy slots, and GatherUndoState. The original AND-join semantics are a degenerate case (single-item gather). |
| [devlore-graph-convergence-operations.md](devlore-graph-convergence-operations.md) | Choose (OR) | **Superseded** | Choose now uses runtime predicates and phase-level branching. The original criteria-based selection is replaced by predicate evaluation. |
| [devlore-graph-convergence-operations.md](devlore-graph-convergence-operations.md) | Convergence Comparison | **Superseded** | Updated comparison in this document's Gather and Choose sections. |
| [devlore-phase-execution.md](devlore-phase-execution.md) | Phase as (A, C, S) Tuple | **Extended** | Phase now serves four roles (saga step, compensation, gather body, choose branch). The (A, C, S) tuple still applies to saga-step usage. |
| [devlore-phase-execution.md](devlore-phase-execution.md) | Recovery Pointer Stack | **Extended** | Recovery stack now includes GatherUndoState with per-iteration entries. |
| [devlore-typed-slots.md](devlore-typed-slots.md) | Slot Types | **Extended** | SlotValue gains the Proxy variant (GatherRef + Field). Resolution chain adds proxy context parameter. |
| [devlore-typed-slots.md](devlore-typed-slots.md) | Serialization | **Extended** | Proxy slots serialize. RuntimePredicates do not. |

---

## Files

| File | Role |
|---|---|
| `internal/execution/action.go` | Action interface, Result, UndoState, Context |
| `internal/execution/graph.go` | SlotValue (with Proxy), Node, ResolvedSlots |
| `internal/execution/executor.go` | ExecutePhaseInner, RunPhased, hook instrumentation |
| `internal/execution/phase.go` | Phase |
| `internal/execution/recovery.go` | RecoveryStack, RecoveryEntry |
| `internal/execution/activation.go` | ActivationState (new) |
| `internal/execution/predicate.go` | RuntimePredicate (new) |
| `internal/execution/hooks.go` | LifecycleHook, HookRegistry (new) |
| `internal/execution/flow/gather.go` | Gather with phase ref, proxy ctx, concurrency, GatherUndoState |
| `internal/execution/flow/choose.go` | Choose with predicate eval, phase selection |
| `internal/execution/flow/wait_until.go` | WaitUntil (new) |
| `internal/execution/flow/register.go` | Updated registration |

## Related Documents

- [Execution Graph](devlore-execution-graph.md) — Core graph architecture
- [Graph Operations](devlore-graph-convergence-operations.md) — Original convergence and control flow
- [Phase Execution](devlore-phase-execution.md) — Saga pattern and phase model
- [Typed Slots](devlore-typed-slots.md) — Slot model and resolution chain
- [Action Namespaces](devlore-operation-namespaces.md) — Adding action namespaces
- [Receipt Integrity](devlore-receipt-integrity.md) — Checksum and signature verification
