---
title: "Phase 3: ExecutionEvent and audit ledger"
parent: ../reconciliation.md
status: draft
---

# Phase 3: ExecutionEvent and audit ledger

## Summary

Introduce the `ExecutionEvent` envelope that wraps every node execution
with coordinator-owned metadata (timing, status, node ID) alongside
action-owned payloads (result, undo state, reconciliation state). Implement
the `AuditLedger` as the first `EventSink` consumer.

## Rationale

Today, execution outcome is recorded by mutating `Node.Status` on the
graph struct. This conflates the intent (the graph) with the history
(what happened). The `ExecutionEvent` separates these concerns:

- **The graph is the intent** — nodes, edges, phases, slots
- **The event stream is the history** — what ran, when, how long, what
  state was captured

The audit ledger serializes the envelope fields into a durable log.
It ignores `UndoState` (which may be large or sensitive) and captures
everything else.

## Types

```go
// pkg/op/event.go

type ExecutionEvent struct {
    // Envelope (coordinator-owned)
    NodeID     string
    ActionName string
    Status     ResultStatus
    StartTime  time.Time
    Duration   time.Duration
    Error      string

    // Payload (action-owned)
    Result         Result
    Recovery       *RecoveryPayload
    Reconciliation *ReconciliationPayload
}

type RecoveryPayload struct {
    UndoState UndoState
    Action    CompensableAction
}

type ReconciliationPayload struct {
    NodeID             string
    ReconciliationData any
}

type EventSink interface {
    OnEvent(ExecutionEvent)
}
```

## Executor changes

`executeNode` builds an `ExecutionEvent` after `Do` returns:

1. Record `start := time.Now()` before `Do`
2. Compute `Duration` after `Do`
3. Populate envelope fields from node metadata
4. Populate recovery payload if `CompensableAction` with non-nil undo state
5. Populate reconciliation payload if non-nil reconciliation state
6. Emit event to registered sinks
7. Feed `RecoveryStack.Push` from the event payloads

The existing `LifecycleHook` system is preserved — hooks are external
observation; events are internal coordination. A future phase may unify
them if `EventSink` proves sufficient for all hook use cases.

## AuditLedger

The `AuditLedger` implements `EventSink`. It captures a flat sequence of
events per graph run and serializes them as YAML or JSON alongside the
graph receipt.

Fields captured per event:
- `NodeID`, `ActionName`, `Status`, `StartTime`, `Duration`, `Error`
- `Reconciliation.NodeID`, `Reconciliation.ReconciliationData`

Fields excluded:
- `Result` (flows to downstream nodes; not audit-relevant)
- `Recovery.UndoState` (transient, may be sensitive)

## Tasks

- [ ] Add `ExecutionEvent`, `RecoveryPayload`, `ReconciliationPayload` types in `pkg/op/event.go`
- [ ] Add `EventSink` interface in `pkg/op/event.go`
- [ ] Add event sink registry to `GraphExecutor` (slice of `EventSink`)
- [ ] Refactor `executeNode` to build `ExecutionEvent` with timing
- [ ] Emit events to registered sinks after each node execution
- [ ] Feed `RecoveryStack.Push` from event payloads
- [ ] Implement `AuditLedger` as an `EventSink`
- [ ] Register `AuditLedger` with executor during graph execution
- [ ] Serialize audit log alongside graph receipt

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/event.go` | Create | `ExecutionEvent`, payloads, `EventSink` |
| `internal/execution/executor.go` | Modify | Build events, emit to sinks, timing |
| `internal/execution/audit.go` | Create | `AuditLedger` implementation |
