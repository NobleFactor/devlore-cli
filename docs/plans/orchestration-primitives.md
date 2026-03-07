# Plan: Orchestration Primitives

```yaml
title: "Orchestration Primitives"
issue: TBD
status: draft
created: 2026-02-16
updated: 2026-02-16
```

## Context

Phases 1–5 established the Resource-Provider model with 34 actions, typed slots
(Immediate + Promise), phase-based saga execution, and comprehensive tests. The
three flow actions (Choose, Gather, Elevate) exist as passthrough stubs that
return nil — they implement the Action interface but perform no runtime work.

Phase 6 transforms these stubs into first-class runtime operations. It adds
SlotProxy (a third SlotValue variant for gather iteration binding),
ActivationState (per-execution mutable state separated from the immutable Node),
runtime predicates (Starlark callables evaluated during execution), and two new
flow actions (WaitUntil, Sidecar). The architecture is documented in
[2.3-orchestration-primitives.md](../architecture/2.3-orchestration-primitives.md).

## Scope

| Category | Items | Strategy |
|---|---|---|
| New types | ActivationState, RuntimePredicate, GatherUndoState, IterationUndo, LifecycleHook, HookRegistry | New files + type additions |
| SlotValue extension | GatherRef, Field, IsProxy(), SetSlotProxy() | Extend existing SlotValue struct |
| ResolvedSlots extension | Optional proxyCtx parameter | Extend existing method signature |
| Gather rewrite | Phase ref, proxy ctx, concurrency, GatherUndoState | Replace stub |
| Choose rewrite | Predicate eval, phase selection, promise return | Replace stub |
| New flow action | WaitUntil (poll, timeout, predicate) | New file |
| Sidecar hooks | LifecycleHook interface, HookRegistry, executor instrumentation | New file + executor changes |
| Documentation | Architecture doc, implementation plan, index update | New files |

## Steps

### Step 1: ActivationState Type

**File:** `internal/execution/activation.go` (new)

New type that captures per-execution mutable state, separate from the immutable
Node definition layer. Not yet used by non-gather paths — the existing inlined
fields on Node continue to work for sequential execution.

```go
type ActivationState struct {
    Status         NodeStatus
    Timestamp      string
    Error          string
    SourceChecksum string
    TargetChecksum string
}
```

**Tests:**

- Construct ActivationState, verify field access
- Zero-value is valid (pending status)

### Step 2: SlotProxy on SlotValue

**Files:** `internal/execution/graph.go`, `internal/execution/executor.go`,
`internal/execution/recovery.go`

Extend SlotValue with `GatherRef` and `Field` fields. Add `IsProxy()` method.
Add `SetSlotProxy(name, gatherRef, field string)` on Node. Update
`ResolvedSlots` to accept an optional `proxyCtx ...map[string]any` parameter
and resolve proxy slots from it.

Update `RecoveryStack.Unwind` to forward proxy context when re-resolving slots
during undo.

**Tests:**

- `SlotValue.IsProxy()` returns true when GatherRef is set
- `IsProxy()`, `IsPromise()`, `IsImmediate()` are mutually exclusive
- `ResolvedSlots` with proxy context resolves proxy slots
- `ResolvedSlots` without proxy context leaves proxy slots nil
- `SetSlotProxy` creates correct SlotValue
- Proxy slot round-trips through YAML/JSON serialization

### Step 3: Runtime Predicates

**Files:** `internal/execution/predicate.go` (new),
`internal/starlark/runtime.go` (new or extend existing)

Define `RuntimePredicate` type wrapping a `starlark.Callable` with source
string. Add `Eval(thread, input) (bool, error)` method. Register
`runtime.predicate()` as a Starlark builtin that wraps a callable in a
RuntimePredicate.

**Tests:**

- `RuntimePredicate.Eval` returns true/false based on callable result
- `RuntimePredicate.Eval` propagates Starlark errors
- `runtime.predicate()` builtin creates RuntimePredicate from lambda
- RuntimePredicate `Source` field captures display string

### Step 4: Refined Gather

**Files:** `internal/execution/flow/gather.go`, `internal/execution/executor.go`,
`internal/execution/action.go`

Rewrite Gather from stub to full implementation:

- `Do` receives `items` (list), `do` (phase ID), and `policy` (concurrency config) slots
- Looks up the referenced phase from the graph
- Executes the phase body once per item with:
  - Shared immutable nodes (no cloning)
  - Per-iteration ActivationState map
  - Per-iteration results map
  - Per-iteration RecoveryStack
  - Proxy context `{gatherID: item}` passed to ResolvedSlots
- Collects terminal node Result per iteration as gather's Result (list)
- Builds `GatherUndoState` with per-iteration undo entries

Define `GatherUndoState` and `IterationUndo` types. Implement `Undo` that
walks iterations in reverse, re-resolves slots with saved proxy context, and
calls `Action.Undo` per entry.

Gather needs access to the Graph and Phase during execution. Add `Graph *Graph`
to `Context` (or pass via a new mechanism — e.g., a `FlowContext` wrapper that
Gather extracts from `Context.Data`).

**Tests:**

- Gather with 3 items executes phase body 3 times
- Gather with concurrency limit=1 runs sequentially
- Gather with concurrency limit=N runs up to N concurrent
- Gather collects terminal results in order
- Gather per-iteration failure unwinds only that iteration
- Gather undo walks iterations in reverse
- Gather undo re-resolves proxy slots correctly
- Proxy slots resolve to per-item values in phase body
- GatherUndoState serialization round-trip

### Step 5: Refined Choose

**Files:** `internal/execution/flow/choose.go`

Rewrite Choose from stub to full implementation:

- `Do` receives `input` (any), `cases` (list of predicate+phase pairs),
  and `default` (phase ID) slots
- Evaluates each case predicate against the resolved input
- Executes the first matching phase via `ExecutePhaseInner`
- If no predicate matches, executes the default phase
- Returns the branch phase's terminal node Result as its own Result

Implement `Undo` that delegates to the selected branch's recovery stack.

**Tests:**

- Choose selects first matching predicate
- Choose falls through to default when no match
- Choose returns branch phase's terminal node Result
- Choose undo delegates to selected branch
- Choose with promise input resolves before evaluation
- Choose skips unmatched branches (no execution)

### Step 6: WaitUntil

**Files:** `internal/execution/flow/wait_until.go` (new),
`internal/execution/flow/register.go`

New flow action implementing a poll-based synchronization primitive:

- `Do` receives `target` (promise), `predicate` (RuntimePredicate),
  `timeout` (string duration), `interval` (string duration, default "5s")
- Starts a polling loop that evaluates the predicate against the target value
- Returns the target value when the predicate returns true
- Returns a timeout error if the deadline expires

Register `flow.wait_until` in the flow action registry.

**Tests:**

- WaitUntil completes when predicate returns true
- WaitUntil times out when predicate never returns true
- WaitUntil respects interval between polls
- WaitUntil returns target value on success
- WaitUntil undo is no-op (returns nil)
- WaitUntil with default interval uses 5s

### Step 7: Sidecar Hooks

**Files:** `internal/execution/hooks.go` (new),
`internal/execution/executor.go`

Define `LifecycleHook` interface with `OnNodeStart`, `OnNodeComplete`,
`OnPhaseStart`, `OnPhaseComplete` methods. Define `HookRegistry` that holds
registered hooks and provides `Fire*` methods.

Add `HookRegistry` field to `GraphExecutor` (optional, nil-safe). Instrument
`executeNode` and `ExecutePhaseInner` to fire hook events at boundaries.

Hook failures are logged but do not fail execution. Hooks run synchronously —
they must not block.

**Tests:**

- Hook receives OnNodeStart before Do
- Hook receives OnNodeComplete after Do (with result or error)
- Hook receives OnPhaseStart/OnPhaseComplete at phase boundaries
- Hook failure does not fail node execution
- Multiple hooks fire in registration order
- Nil HookRegistry is safe (no-op)

### Step 8: Documentation

**Files:**
- `docs/architecture/2.3-orchestration-primitives.md` (new)
- `docs/plans/orchestration-primitives.md` (new — this file)
- `docs/architecture/index.md` (update — add link to orchestration primitives)
- `docs/plans/resource-provider.md` (update — add orchestration primitives reference)

Architecture document covers: design principle, three-layer vocabulary,
phase as universal boundary, SlotProxy and gather concurrency, choose
with predicate branching, WaitUntil, sidecar hooks, runtime predicates,
serialization boundaries, and a supersession table mapping changes to
existing docs.

## File Summary

| File | Change |
|---|---|
| `internal/execution/activation.go` | **New**: ActivationState type |
| `internal/execution/predicate.go` | **New**: RuntimePredicate type, Eval method |
| `internal/execution/hooks.go` | **New**: LifecycleHook interface, HookRegistry |
| `internal/execution/graph.go` | Extend SlotValue (GatherRef, Field, IsProxy), update ResolvedSlots |
| `internal/execution/executor.go` | Hook instrumentation |
| `internal/execution/recovery.go` | Forward proxy context in Unwind |
| `internal/execution/action.go` | GatherUndoState, IterationUndo types |
| `internal/execution/flow/gather.go` | Full rewrite: phase ref, proxy ctx, concurrency, undo |
| `internal/execution/flow/choose.go` | Full rewrite: predicate eval, phase selection, undo |
| `internal/execution/flow/wait_until.go` | **New**: WaitUntil flow action |
| `internal/execution/flow/register.go` | Register WaitUntil |
| `internal/starlark/runtime.go` | **New** (or extend): runtime.predicate() builtin |
| `docs/architecture/2.3-orchestration-primitives.md` | **New**: Architecture document |
| `docs/architecture/index.md` | Add orchestration primitives link |
| `docs/plans/orchestration-primitives.md` | **New**: This plan |
| `docs/plans/resource-provider.md` | Add orchestration primitives reference |

## Test Count

Estimated 40–50 new test functions across steps 1–7. After Phase 6: all flow
actions have real implementations with full test coverage.

## Verification

```bash
# All new types compile
go build ./internal/execution/...

# SlotProxy fields present
grep -n 'GatherRef\|IsProxy' internal/execution/graph.go

# ResolvedSlots accepts proxy context
grep -n 'proxyCtx' internal/execution/graph.go

# Flow actions are not stubs (no bare nil returns in Do)
grep -A5 'func.*Gather.*Do(' internal/execution/flow/gather.go
grep -A5 'func.*Choose.*Do(' internal/execution/flow/choose.go

# WaitUntil registered
grep -n 'wait_until\|WaitUntil' internal/execution/flow/register.go

# Hook instrumentation in executor
grep -n 'FireNodeStart\|FireNodeComplete' internal/execution/executor.go

# Tests pass
go test ./internal/execution/... -count=1

# No legacy/compat/deprecated terms
grep -rn 'legacy\|backward\|compat\|deprecated' internal/execution/

# Architecture doc contains all sections
grep '^## ' docs/architecture/2.3-orchestration-primitives.md
```

## Dependencies

| Step | Depends on | Reason |
|---|---|---|
| 1 (ActivationState) | — | Standalone type |
| 2 (SlotProxy) | — | Standalone extension to SlotValue |
| 3 (Runtime predicates) | — | Standalone type + Starlark builtin |
| 4 (Gather) | 1, 2, 3 | Uses ActivationState, proxy slots, optional predicates |
| 5 (Choose) | 3 | Uses runtime predicates |
| 6 (WaitUntil) | 3 | Uses runtime predicates |
| 7 (Sidecar hooks) | — | Standalone; executor instrumentation is additive |
| 8 (Documentation) | All | Documents all changes |

Steps 1–3 and 7 can proceed in parallel. Steps 4–6 require their
dependencies. Step 8 follows all others.

## Related Documents

- [Orchestration Primitives Architecture](../architecture/2.3-orchestration-primitives.md)
- [Graph Operations Architecture](../architecture/2.3-orchestration-primitives.md)
- [Phase Execution Architecture](../architecture/2.2-phase-execution.md)
- [Typed Slots Architecture](../architecture/2.1-typed-slots.md)
- [Resource-Provider Plan](resource-provider.md)
