# Phase Execution Model for Lifecycle Pipelines

## Status

**Approved** — Implementation in progress.

## Context

The execution graph has no concept of "phase." Lifecycle pipelines
(prepare → install → provision → verify) are a build-time concept — Starlark
scripts emit nodes into a flat graph, and the executor runs them via topological
sort. This means:

- There is no error boundary between phases. A node failure is a graph failure.
- There is no retry at the phase level. Transient failures abort the whole run.
- There is no structured rollback. Partial success leaves the system in an
  indeterminate state.

This document describes **phases as first-class runtime concepts** in the
execution graph, implementing the **Saga Pattern** as a transactional state
machine. Each phase is a scoped transaction with a forward action, a compensating
action, and the execution state needed to undo itself. The executor becomes a
saga coordinator that walks phase boundaries, traps errors, retries, and unwinds.

## The Saga Pattern

Each lifecycle pipeline is a saga — a sequence of local transactions where each
transaction has a compensating action. The executor is the saga coordinator:

```
prepare ──→ install ──→ provision ──→ verify
   ↓            ↓            ↓
cleanup    uninstall    unprovision
(compensate) (compensate) (compensate)
```

On success: all phases complete, saga committed.
On failure: completed phases are compensated in LIFO order.

This is a **local saga** — the executor has full control of sequencing.
Compensating actions run synchronously in reverse order. No distributed
coordination, no eventual consistency.

## Phase as (A, C, S) Tuple

Each phase is defined by the tuple:

| Component | Role | Example |
|-----------|------|---------|
| **A** (Action) | Forward operation | Install binary, link config |
| **C** (Compensate) | Reverse operation | Remove binary, unlink config |
| **S** (State) | Metadata captured during A that C needs | Installed version, created paths |

A is obligated to populate S during forward execution. S is the receipt of A
and the input to C. If A doesn't capture enough state, C can't undo the work.

## Phase Boundary Nodes (Checkpoints)

A phase is a **dual-method node** in the graph — a stateful controller that
encapsulates both the forward and compensating actions. The executor recognizes
phase nodes as boundaries.

Phase node structure:

| Field | Type | Description |
|-------|------|-------------|
| ID | string | Unique identifier (e.g., `"phase.install"`) |
| Name | string | Phase name (e.g., `"install"`) |
| Retry | *RetryPolicy | Max retries, backoff, timeout |
| Status | PhaseStatus | pending, completed, failed, rolled_back, skipped |
| NodeIDs | []string | IDs of inner nodes belonging to this phase |
| Compensate | string | ID of compensating phase/action |
| Attempts | []Attempt | Retry history |
| State | map[string]any | Execution state captured during forward action (S) |

## Recovery Pointer Stack

When the executor enters a checkpoint phase, it pushes a **recovery pointer**
onto a local stack. The stack tracks completed phases and their compensating
actions:

```
Stack (LIFO):
  [2] provision → unprovision (← top, most recent)
  [1] install   → uninstall
  [0] prepare   → cleanup
```

On failure after exhausting retries, the executor pops the stack and executes
compensating actions in LIFO order until it reaches the bottom. Only phases
that actually completed have entries on the stack — a phase that was never
reached produces no entry.

## Executor Loop

The executor runs a **two-level loop**:

**Outer loop**: walks phases in order (the saga coordinator)
**Inner loop**: executes nodes within a phase (the current topological sort)

```
for each phase in pipeline:
    push recovery pointer
    result = phase.Execute(ctx)        // inner loop + retry
    if result == success:
        continue to next phase
    else:
        unwind(recovery_stack)         // LIFO compensating actions
        return failure
```

## Inner Node Failure

**Any inner node failure immediately fails the phase.** Inner nodes do not
have independent retry. When a node within a phase errors, execution of
that phase stops and the error bubbles to the phase boundary (the trap).

The phase's retry policy then governs: retry the entire phase, or give up and
trigger unwind.

## Rollback (Unwind)

When the executor decides to unwind:

1. Pop the recovery stack
2. For each entry (LIFO order): execute the compensating action
3. Compensating action receives the phase's captured State (S)
4. If a compensating action itself fails: log the error, continue unwinding
5. Unwinding continues until the stack is empty

## Retry Policy

```go
type RetryPolicy struct {
    MaxAttempts  int
    Backoff      BackoffStrategy  // none, linear, exponential
    InitialDelay string           // duration string, e.g. "1s"
    MaxDelay     string           // duration string, e.g. "30s"
}
```

## Graph Integration

The `Phases` field on `Graph` is optional. Graphs that don't use phases continue
to work exactly as before. Lore lifecycle graphs populate `Phases` to enable
phase-aware execution. The executor checks `graph.Phases != nil` and delegates
to the phase-aware loop when present.

## Starlark Integration (Future)

Phase scripts will gain `forward()` / `compensate()` entry points.
The phase `ctx` will expose a `state` dict for (S) capture.
A `configure(phase)` hook will allow setting retry policy per-phase.

## Files

| File | Role |
|------|------|
| `internal/execution/phase.go` | Phase, PhaseStatus, Attempt, RetryPolicy types |
| `internal/execution/recovery.go` | RecoveryStack, RecoveryEntry |
| `internal/execution/executor.go` | RunPhased method on GraphExecutor |
| `internal/execution/graph.go` | Phases field on Graph |
