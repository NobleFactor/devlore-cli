# Plan: Orchestration Primitives — Architecture & Implementation Documents

## Context

Phases 1–5 established the Resource-Provider model with 34 actions, typed slots
(Immediate + Promise), phase-based saga execution, and comprehensive tests.
The `draft.md` in the worktree proposes refined orchestration primitives that
transform flow actions from passthrough stubs into first-class runtime
operations. Design decisions established in conversation:

1. **Phase is the universal boundary** — no separate Subtree type. Phase is used
   for saga steps, compensation, gather bodies, and choose branches. Distinguished
   by how they're referenced, not by structure.
2. **SlotProxy** — third SlotValue variant enabling N concurrent instances with
   isolated slot resolution. The lambda builds nodes with proxy-bound slots; it
   does not compute values. Computed values use template nodes in the subtree.
3. **Terminal node = return** — functional paradigm where the leaf node of a
   phase provides its output.
4. **ActivationState** — per-execution mutable state, separate from the Node
   (definition layer). Nodes are immutable during gather execution. Activation
   state is transient — discarded after results and undo state are captured.
5. **Concurrency via shared nodes** — gather executes phases concurrently with
   shared immutable nodes + per-iteration ActivationState maps. No node cloning.
   Undo works because GatherUndoState saves proxy contexts and recovery entries
   referencing the stable shared nodes.

The user asked to **draft an architecture update and an implementation plan**,
saving each as a **new document** in the worktree. No existing documents are
modified — the new doc supersedes specific sections by reference.

## Worktree

`/Users/david-noble/Workspace/NobleFactor/devlore-cli.resource-provider`
Branch: `feat/generator-templates`

## Deliverables

Two new files. No existing documents are modified.

### Document 1: `docs/architecture/devlore-orchestration-primitives.md`

Architecture document describing the refined orchestration primitives.

**Sections:**

1. **Preamble** — cross-references to existing docs, supersession table
2. **Design Principle** — graph remains self-contained; primitives are visible runtime ops
3. **Vocabulary** — three-layer model: Definition (Node, Phase, Edge, SlotValue),
   Activation (ActivationState), Recovery (UndoState, RecoveryEntry, RecoveryStack)
4. **Phase as Universal Boundary** — one type, four usages (saga step, compensation,
   gather body, choose branch), distinguished by reference not structure
5. **SlotProxy** — the keystone section (detailed design below)
6. **Gather as Parallel Comprehension** — loop semantics, phase reference,
   concurrency via shared nodes + ActivationState, functional return, GatherUndoState
7. **Choose as Predicate Branching** — cases with runtime.predicate, default, promise return
8. **Phase Exports** — terminal node identification, export capture
9. **Wait_Until** — event-driven sensor with polling and timeout
10. **Sidecar** — lifecycle hooks for observability
11. **Runtime Predicates** — runtime.predicate namespace, plan-time vs execution-time lambdas
12. **Serialization** — what serializes (SlotProxy) vs what doesn't (RuntimePredicate)
13. **Supersession Table** — maps which existing doc sections this supersedes or extends

### Document 2: `docs/plans/resource-provider/phase-6.md`

Implementation plan following the established phase-plan template.

**Sections:** Context, Scope, Steps (8 implementation steps), File Summary,
Test Count, Verification.

---

## Design Decisions (Detail)

### Phase as Universal Boundary

There is no Subtree type. `Phase` is the universal boundary:

| Usage | Referenced by | Executed by |
|---|---|---|
| Saga step | `Graph.Phases` order | `RunPhased` forward walk |
| Compensation | Another phase's `Compensate` field | `RunPhased` unwind |
| Gather body | Gather node's `do` slot (phase ID) | Gather's per-item loop |
| Choose branch | Choose node's `cases`/`default` slots | Choose after predicate match |

All four use `ExecutePhaseInner`. All four get retry policy. All four produce
an export (terminal node Result). Gather-body and choose-branch phases appear
in `Graph.Phases` but are skipped during the forward saga walk — same pattern
as compensating phases.

### Three-Layer Vocabulary

| Layer | Types | Lifecycle |
|---|---|---|
| **Definition** | Node, Phase, Edge, SlotValue | Plan-time, immutable during execution |
| **Activation** | ActivationState | Per-execution, mutable, transient — discarded after capture |
| **Recovery** | UndoState, RecoveryEntry, RecoveryStack | Durable until rollback completes or graph succeeds |

```go
type ActivationState struct {
    Status         NodeStatus
    Timestamp      string
    Error          string
    SourceChecksum string
    TargetChecksum string
}
```

Today, activation state is inlined on the Node struct. For non-gather execution,
this continues to work. For gather's concurrent execution, ActivationState lives
in a per-iteration map — nodes are never mutated.

### SlotProxy

**Go types:**

```go
type SlotValue struct {
    Immediate any    `json:"immediate,omitempty" yaml:"immediate,omitempty"`
    NodeRef   string `json:"node_ref,omitempty"  yaml:"node_ref,omitempty"`
    Slot      string `json:"slot,omitempty"      yaml:"slot,omitempty"`
    GatherRef string `json:"gather_ref,omitempty" yaml:"gather_ref,omitempty"`
    Field     string `json:"field,omitempty"      yaml:"field,omitempty"`
}

func (s SlotValue) IsProxy() bool { return s.GatherRef != "" }
```

**Plan-time recording:**

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

**Execution-time resolution:**

`ResolvedSlots` gains an optional proxy context parameter:

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

**Serialized form:**

```yaml
- id: stop-svc
  action: service.stop
  slots:
    name: {gather_ref: patch-servers, field: name}
```

### Gather Concurrency

Shared immutable nodes + per-iteration ActivationState maps:

```go
func (a *Gather) executeConcurrent(ctx *Context, graph *Graph, phase *Phase,
    items []any, gatherID string, limit int) ([]any, error) {

    sem := make(chan struct{}, limit)
    phaseNodes := collectPhaseNodes(graph, phase) // shared, never mutated

    for i, item := range items {
        go func(idx int, val any) {
            // Per-iteration isolation
            state := make(map[string]*ActivationState)
            results := make(map[string]any)
            stack := &RecoveryStack{}
            proxyCtx := map[string]any{gatherID: val}

            for _, node := range ordered(phaseNodes, phase) {
                slots := node.ResolvedSlots(results, proxyCtx)
                fillSlotsFromData(slots, ctx.Data)
                result, undoState, err := node.Action.Do(ctx, slots)
                if result != nil {
                    results[node.ID] = result
                }
                stack.Push(RecoveryEntry{Node: node, UndoState: undoState})
                state[node.ID] = &ActivationState{Status: StatusCompleted, ...}
            }
            // Capture terminal node result; state map discarded
        }(i, item)
    }
}
```

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
iteration's saved proxy context, and calls `Action.Undo` on each entry.
Recovery entries reference the stable shared nodes — no lifecycle issue.

---

## Implementation Steps (Phase 6 plan content)

| Step | Scope | Files |
|---|---|---|
| 1. ActivationState type | New type, not yet used by non-gather paths | `activation.go` (new) |
| 2. SlotProxy on SlotValue | Add GatherRef/Field, IsProxy(), update ResolvedSlots | `graph.go`, `executor.go`, `recovery.go` |
| 3. Phase exports | TerminalNodeID method, Export field, capture in ExecutePhaseInner | `phase.go`, `executor.go` |
| 4. Runtime predicates | RuntimePredicate type, runtime.predicate() Starlark builtin | `predicate.go` (new), `starlark/runtime.go` (new) |
| 5. Refined Gather | Rewrite: loop, phase ref, proxy ctx, concurrency, GatherUndoState | `flow/gather.go`, `executor.go`, `action.go` |
| 6. Refined Choose | Rewrite: predicate eval, phase selection, promise return | `flow/choose.go` |
| 7. Wait_Until | New flow action: poll loop, timeout, predicate eval | `flow/wait_until.go` (new), `flow/register.go` |
| 8. Sidecar hooks | LifecycleHook interface, HookRegistry, executor instrumentation | `hooks.go` (new), `executor.go` |
| 9. Documentation | Architecture doc + phase-6 plan doc + index update | `docs/architecture/`, `docs/plans/` |

## Verification

After creating both documents:
- Both files parse as valid Markdown
- Architecture doc contains all 13 sections listed above
- Phase-6 plan follows the established template (frontmatter, context, scope, steps, file summary)
- SlotProxy design includes Go types, plan-time recording, execution-time resolution, serialization
- Gather section covers concurrency (ActivationState), undo (GatherUndoState), phase reference
- Phase-as-universal-boundary table shows all four usages
- Cross-references to existing docs are correct (file paths exist)
- No existing documents are modified
