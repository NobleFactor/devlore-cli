---
title: "Audit, Reconciliation, and Recovery in the Execution Graph"
issue: https://github.com/NobleFactor/devlore-cli/issues/156
status: draft
created: 2026-02-21
updated: 2026-02-22
---

# Plan: Audit, Reconciliation, and Recovery in the Execution Graph

## Summary

The execution framework today conflates three concerns into a single code path:
the recovery stack carries undo state, nodes carry checksum fields (now removed),
and the graph receipt carries audit metadata. This plan introduces the
**ExecutionEvent** as a unified structure that captures all three concerns at the
point of action execution, then routes them to three independent consumers with
zero logic overlap.

## Governing Principle

**The Action is the subject matter expert on *what* changed and *how* to fix it.
The Coordinator is the objective observer of *when* and *how* the process
unfolded.**

The action knows the resource, its identity, and its state. The coordinator knows
the timeline, the correlation, and the outcome. Neither trespasses on the other's
domain.

## Goals

1. **Recovery**: Preserve LIFO undo state for saga rollback (what we have today,
   but carried inside an event envelope instead of a bare `RecoveryEntry`)
2. **Audit**: Produce a durable, serializable ledger of every action executed,
   independent of recovery state lifetime
3. **Reconciliation**: Capture resource identity and state snapshots so that
   `writ reconcile` can detect drift without requiring the coordinator to
   understand resource internals

## Current State

| Concern | Mechanism | Problems |
|---------|-----------|----------|
| Recovery | `RecoveryStack` (LIFO of `RecoveryEntry{Node, UndoState}`) | Works. Transient -- destroyed after unwind. No audit trail. |
| Audit | Graph receipt (serialized `Graph` with node statuses) | Captures status but not duration, not resource identity. Mixes node metadata with execution outcome. |
| Reconciliation | **Removed** (was `ctx.TargetChecksum`/`ctx.SourceChecksum`) | Checksums were framework plumbing that violated action encapsulation. Actions know their resources; the framework does not. |
| Observation | `LifecycleHook` (fire-and-forget at node/phase boundaries) | No return channel. Cannot contribute data back to the execution record. |

### What was removed and why

`ctx.SourceChecksum` and `ctx.TargetChecksum` were fields on the execution
`Context` that every action could write to, and the executor would copy onto
the `Node` struct after `Do()` returned. This violated encapsulation: the
*framework* carried resource-specific state (file checksums) that only
content-pipeline actions cared about. The framework should not know what a
checksum is. The action should report "here is how to verify I did what I
said" in its own terms.

## Design

### 1. The Action Reports What It Knows — Closed-Loop Reconciliation

Adding reconciliation data to the `Do` return transforms actions from
one-shot commands into **managed resources**. The system moves from a simple
Command pattern to a full **Closed-Loop System** — in a world where AI agents
make changes, the system can no longer assume the state it left behind remains
untouched.

The provider method signature evolves to return reconciliation data explicitly:

```go
// Compensable provider method — the full contract
func (p *Provider) ActionX(args ...any) (result string, undo map[string]any, reconciliation any, err error)
```

The reconciliation data is the **Expected State anchor** — a fingerprint of
the resource at the moment `Do` completed:

- A content hash (SHA256 of file contents)
- A version number (installed package version, ETag)
- A status token (service enabled/running state)
- A composite (directory listing hash, git HEAD)

By returning this from `Do`, the action says: *"I have performed the action,
and as of this microsecond, this is exactly what the external resource looks
like."*

At the execution framework level, the `Action` interface adds a 4th return:

```go
Do(ctx *Context, slots map[string]any) (Result, UndoState, ReconciliationState, error)
```

Each return has a distinct consumer:

| Return | Consumer | Purpose |
|--------|----------|---------|
| Result | Graph edge system | Data flow to downstream nodes |
| UndoState | Recovery Stack | Mechanical reversal if the graph fails |
| ReconciliationState | Reconciliation Store | Source of truth for future drift checks |

### 1a. The Reconcile Method

Adding a `Reconcile` method transforms the action from a "one-and-done" task
into a **managed resource**. The full action suite becomes:

```go
// Provider method triangle — enforced by the AST generator
func (p *Provider) ActionX(args ...) (result, undo, reconciliation, error)
func (p *Provider) CompensateActionX(undo any) error
func (p *Provider) ReconcileActionX(reconciliation any) (bool, error)
```

The `Reconcile` method receives the reconciliation data captured by `Do` and
probes the *current* state of the resource. If the current state differs from
the expected state, it returns `true` (drifted).

**Why a separate method is essential:**

1. **Agent consumption**: An AI agent asking "is the system still in the state
   I put it in?" gets an answer without re-running `Do`.
2. **Safety gates**: Before running `Undo`, the Coordinator can run `Reconcile`.
   If drifted, a human or another agent modified the resource — automated undo
   on a manually modified resource is dangerous. The system can halt and alert.
3. **Self-healing**: The Coordinator can periodically run `Reconcile` across
   the entire graph to detect and optionally repair drift.

### 1b. Action Capability Matrix

The interface combinations now form a triangle:

| Capability | Meaning |
|------------|---------|
| `Action` only | Forward-only, no undo, no reconciliation |
| `Action` + `Reconcilable` | Forward-only but drift-detectable (e.g., a probe) |
| `CompensableAction` only | Undoable but no drift detection |
| `CompensableAction` + `Reconcilable` | Full lifecycle: do, undo, detect drift |

### 1c. AST Generator Enforcement

The code generator enforces a **type signature triangle**:

1. Does `ActionX` exist?
2. Does `CompensateActionX` exist? (if compensable)
3. Does `ReconcileActionX` exist? (if reconcilable)

The generator verifies that the **third return type** of `ActionX` matches the
**first input type** of `ReconcileActionX`. This creates a compile-time safety
chain — if the reconciliation contract is broken, the build fails.

### 2. The Coordinator Wraps It in an Envelope

The executor produces an `ExecutionEvent` for every node it executes. This
event is the single source of truth for what happened.

```go
// ExecutionEvent is the complete record of a single node's execution.
// The coordinator populates the envelope; the action populates the payload.
type ExecutionEvent struct {
    // --- Envelope (coordinator-owned) ---

    // NodeID is the graph node that was executed.
    NodeID string

    // ActionName is the dotted action identifier (e.g., "file.copy").
    ActionName string

    // Status is the execution outcome.
    Status ResultStatus

    // StartTime is when Do() was called.
    StartTime time.Time

    // Duration is how long Do() took.
    Duration time.Duration

    // Error is the error message if Status is ResultFailed.
    Error string

    // --- Payload (action-owned) ---

    // Result is the value that flows to downstream nodes.
    Result Result

    // Recovery is the undo state for saga rollback. Nil for non-compensable
    // actions or when Do() returns nil undo state.
    Recovery *RecoveryPayload

    // Reconciliation is the resource identity and state snapshot for drift
    // detection. Nil for actions that don't implement Reconcilable.
    Reconciliation *ReconciliationPayload
}

// RecoveryPayload carries the LIFO-specific state.
type RecoveryPayload struct {
    // UndoState is the state captured by Do, passed to Undo on rollback.
    UndoState UndoState

    // Action is the CompensableAction that can perform the undo.
    Action CompensableAction
}

// ReconciliationPayload carries the expectation for drift detection.
type ReconciliationPayload struct {
    // NodeID identifies the graph node that produced this data.
    NodeID string

    // ReconciliationData is the opaque reconciliation state returned by Do().
    // Passed to ReconcileActionX() during drift detection.
    ReconciliationData any
}
```

### 3. The Coordinator's Promotion Logic

Inside `executeNode`, after `Do()` returns, the coordinator builds the event.
The 4-value return maps directly to the three consumers:

```go
start := time.Now()
result, undoState, reconciliationState, err := node.Action.Do(ctx, slots)
duration := time.Since(start)

event := ExecutionEvent{
    NodeID:     node.ID,
    ActionName: node.ActionName(),
    StartTime:  start,
    Duration:   duration,
    Result:     result,
}

if err != nil {
    event.Status = ResultFailed
    event.Error = err.Error()
} else {
    event.Status = ResultCompleted
}

// Recovery payload -- only for compensable actions with non-nil state
if comp, ok := node.Action.(CompensableAction); ok && undoState != nil {
    event.Recovery = &RecoveryPayload{
        UndoState: undoState,
        Action:    comp,
    }
}

// Reconciliation payload -- only when action returned reconciliation data
if reconciliationState != nil {
    event.Reconciliation = &ReconciliationPayload{
        NodeID:    node.ID,
        ReconciliationData: reconciliationState,
    }
}
```

### 4. Three Consumers, One Event

The same `ExecutionEvent` is routed to three destinations. Each reads only
the fields it cares about.

#### Consumer A: Recovery Stack

The recovery stack reads `event.Recovery`. It pushes `RecoveryPayload` entries
and unwinds them on failure. Once the graph completes successfully, the stack
is discarded (undo state is no longer needed).

```
executeNode() --> event.Recovery --> RecoveryStack.Push()
                                       |
                                  (on failure)
                                       |
                                  RecoveryStack.Unwind() --> CompensableAction.Undo()
```

The recovery stack becomes simpler: it stores `RecoveryPayload` instead of
`RecoveryEntry{Node, UndoState}`. No more carrying `*Node` references --
the payload has everything it needs.

#### Consumer B: Audit Ledger

The audit ledger serializes the envelope fields: NodeID, ActionName, Status,
StartTime, Duration, Error. It ignores `Recovery.UndoState` (which may be
large or sensitive). It is append-only and survives beyond the graph's
lifetime.

The audit ledger is the replacement for today's graph receipt node status
fields. Instead of the graph receipt carrying execution outcome on each
`Node` struct, the ledger carries it as a flat sequence of events.

```
executeNode() --> event --> AuditLedger.Append()
                              |
                         (receipt serialization)
                              |
                         YAML/JSON event log per graph run
```

#### Consumer C: Reconciliation Engine

The reconciliation engine reads `event.Reconciliation`. It stores the
opaque `ReconciliationData` keyed by `NodeID`. During `writ reconcile`, it replays
the stored data through the action's `Reconcile` method to detect drift.

```
executeNode() --> event.Reconciliation --> ReconciliationStore.Put(nodeID, reconciliationData)
                                              |
                                         (writ reconcile)
                                              |
                                         ReconciliationStore.Get(nodeID)
                                              |
                                         Action.Reconcile(reconciliationData) --> drifted?
```

The reconciliation engine does not compute checksums or inspect resources.
It stores the action's *assertion* about the resource state and asks the
action to verify that assertion later.

### 5. Drift Detection via Action Reconcile Methods

The key insight: **the action knows how to verify itself**. The framework
never touches checksums, file content, or resource state. The reconciliation
pipeline works like this:

1. **Deploy**: `file.Copy` executes, returns `reconciliationData` containing the
   resource path and content hash `sha256:abc123`. The reconciliation engine
   stores this keyed by node ID.

2. **Reconcile**: `writ reconcile` iterates the reconciliation store.
   For each entry, it calls `file.Provider.ReconcileCopy(reconciliationData)`. The
   Reconcile method re-reads the file, computes the current hash, and
   compares against the stored hash. Returns `(true, nil)` if drifted.

3. **Safety gate**: Before `Undo`, the Coordinator can call `Reconcile`.
   If drifted, a human or another agent modified the resource — automated
   undo is dangerous. The system halts and alerts.

4. **Repair**: If drift is detected and `--fix` is requested, the reconcile
   command re-executes the deploy graph for drifted resources.

This means:
- `file.Copy` knows its resource is verified by content hash, and
  `ReconcileCopy` re-reads the file to check
- `pkg.Install` knows its resource is verified by installed version, and
  `ReconcileInstall` re-queries the package manager to check
- `service.Enable` knows its resource is verified by enabled status, and
  `ReconcileEnable` re-queries the service manager to check
- The framework knows none of this — it just calls `Reconcile(data)`

### 6. Relationship to Writ Lifecycle

Writ manages a special kind of package with four lifecycle actions:

| Lifecycle | What happens | Event consumers |
|-----------|-------------|-----------------|
| **Deploy** | Build graph, execute, produce events | All three: recovery (for rollback), audit (receipt), reconciliation (state snapshot) |
| **Reconcile** | Read reconciliation index, probe current state, report drift | Reconciliation engine only. Recovery stack is empty (no mutations). Audit records the reconcile run itself. |
| **Upgrade** | Compare stored snapshots against new sources, re-execute changed nodes | All three. New snapshots replace old ones in reconciliation index. |
| **Decommission** | Read reconciliation index, execute undo/removal nodes | Recovery (for rollback if removal fails), audit (receipt), reconciliation (delete entries). |

### 7. Relationship to Existing Hook System

The `LifecycleHook` interface remains unchanged. Hooks are an *external
observation* mechanism -- they let callers watch execution without
participating. The execution event is an *internal coordination* mechanism
-- it carries data between the coordinator and its consumers.

Hooks fire at the same boundaries as before (node start, node complete,
phase start, phase complete). An audit hook could *consume* execution events,
but it would be wired as a consumer of the event stream, not as a lifecycle
hook.

## Implementation Phases

### Phase 1: Action Interface Evolution

Extend the `Action.Do` signature to return reconciliation data as a 4th value.
Define the `ExecutionEvent` and `ReconcilableAction` interface.

- [ ] Add `ReconciliationState` type alias (currently `any`)
- [ ] Update `Action.Do` to return `(Result, UndoState, ReconciliationState, error)`
- [ ] Add `ReconcilableAction` interface with `Reconcile(ReconciliationState) (bool, error)`
- [ ] Add `ExecutionEvent`, `RecoveryPayload`, `ReconciliationPayload` types
- [ ] Update `executeNode` to handle the 4-value return
- [ ] Refactor `RecoveryStack` to accept `RecoveryPayload` instead of `RecoveryEntry`

**Files**:

| File | Action | Purpose |
|------|--------|---------|
| `execution/action.go` | Modify | Extend `Action.Do` signature, add `ReconcilableAction` |
| `execution/event.go` | Create | `ExecutionEvent`, payloads |
| `execution/recovery.go` | Modify | Accept `RecoveryPayload` |
| `execution/executor.go` | Modify | Handle 4-value return, produce `ExecutionEvent` |

### Phase 2: Provider Reconcile Methods

Add `ReconcileActionX(reconciliationData any) (bool, error)` methods to providers
that manage external resources. Update `Do` to return reconciliation data
as the 3rd return value.

- [ ] `file.*` actions: Reconciliation data = path + content hash. Reconcile re-reads and compares.
- [ ] `pkg.*` actions: Reconciliation data = package names + installed versions. Reconcile re-queries.
- [ ] `service.*` actions: Reconciliation data = name + running/enabled status. Reconcile re-probes.
- [ ] `git.*` actions: Reconciliation data = repo path + HEAD commit. Reconcile re-reads HEAD.
- [ ] `archive.Extract`: Reconciliation data = prefix + directory listing hash. Reconcile re-scans.
- [ ] `net.Download`: Not reconcilable (result flows to next node, no persistent resource).
- [ ] `shell.Exec`: Not reconcilable (arbitrary side effects).
- [ ] `template.Render`, `encryption.Decrypt`: Not reconcilable (pure transforms).

**Files**:

| File | Action | Purpose |
|------|--------|---------|
| `execution/provider/{file,pkg,service,git,archive}/provider.go` | Modify | Add `ReconcileX` methods, update `Do` returns |
| `execution/provider/*/actions_gen.go` | Regenerate | Wire 4th return value and `Reconcile` method |

### Phase 3: Event Stream and Audit Ledger

Expose the event stream from the executor so consumers can subscribe.

- [ ] Add `EventSink` interface (`OnEvent(ExecutionEvent)`)
- [ ] Wire executor to emit events to registered sinks
- [ ] Implement `AuditLedger` as an `EventSink` that serializes events
- [ ] Replace node-level status in graph receipt with event-based receipt

**Files**:

| File | Action | Purpose |
|------|--------|---------|
| `execution/event.go` | Modify | Add `EventSink` interface |
| `execution/executor.go` | Modify | Wire event emission |
| `execution/audit.go` | Create | `AuditLedger` implementation |

### Phase 4: Reconciliation Engine

Build the reconciliation store and reconcile command.

- [ ] Implement `ReconciliationStore` (stores `NodeID -> ReconciliationData`)
- [ ] Wire as an `EventSink` during deploy/upgrade
- [ ] Implement `writ reconcile` using the store + action Reconcile methods
- [ ] Add safety gate: `Reconcile` before `Undo` to detect dangerous undos
- [ ] Remove the stub reconcile package created during checksum removal

**Files**:

| File | Action | Purpose |
|------|--------|---------|
| `execution/reconcile.go` | Create | `ReconciliationStore` |
| `writ/reconcile/reconcile.go` | Rewrite | Drift detection via stored reconciliation data + Reconcile methods |
| `writ/commands.go` | Modify | Wire reconciliation into deploy/reconcile/upgrade |

### Phase 5: Code Generation — The Type Signature Triangle

Update the code generator to enforce the three-method triangle: `ActionX`,
`CompensateActionX`, `ReconcileActionX`.

- [ ] Extend provider method validator: verify type signature triangle
  (3rd return of `ActionX` matches 1st param of `ReconcileActionX`)
- [ ] Update `graph_actions.go.template` to emit `Reconcile` method on
  the generated action struct
- [ ] Update provider method discovery to detect `ReconcileX` methods
- [ ] Generator refuses to build if reconciliation chain is broken
- [ ] Regenerate all `actions_gen.go` files

## Open Questions

- [x] ~~Should `Reconcilable` be implemented on the action struct or on the
  provider?~~ **Resolved**: On the provider. `ReconcileActionX` is a provider
  method alongside `ActionX` and `CompensateActionX`. The AST generator
  enforces the triangle.
- [ ] How should the reconciliation store be persisted? Options: embedded in
  the graph receipt, separate index file, or SQLite database.
- [ ] Should `ReconciliationData` be typed per action (e.g., `FileCopyRecon{Path, Hash}`)
  or remain opaque `any`? Typed gives compile-time safety; opaque gives
  flexibility and simpler generator output.
- [ ] What is the TTL for recovery data? Should `RecoveryPayload` carry an
  expiry hint so that old undo state can be garbage collected?
- [ ] Should the audit ledger be the graph receipt (replacing the current
  `Node.Status` approach), or should it be a separate artifact?
- [ ] Can the `EventSink` pattern replace `LifecycleHook` entirely, or do hooks
  serve a distinct purpose (external observation vs internal coordination)?
- [ ] Should the `Reconcile` safety gate before `Undo` be opt-in or default?
  Default is safer but adds latency to rollback. Opt-in risks silent data loss.
- [ ] Self-healing loop: should the Coordinator run periodic `Reconcile` sweeps
  automatically, or only on explicit `writ reconcile` invocation?

## Architecture Analysis: The Receipt + Sidecar Model

The design creates a **Stateful Receipt** — a complete, immutable snapshot of
execution that serves as the permanent record of truth, while offloading
system-wide awareness to a sidecar.

### The Receipt as a Distributed Trace

By bundling Audit and Reconciliation data into a single receipt that augments
the original graph, we create a "Self-Documenting Execution." If the receipt
moves to another system, it contains everything needed to understand the
Intent (the graph) and the Result (the audit/reconciliation). When an AI agent
debugs a failure, it reads the Receipt — the Receipt is the Context Window for
the entire lifecycle of that action.

### Ephemeral Recovery: The Safety Pin

Disposing of Recovery Data upon completion handles state bloat and security.
Recovery data often contains sensitive snapshots (e.g., a user's old
configuration or a temporary token). Once the transaction boundary is crossed
and the system is committed, that undo state becomes a liability rather than an
asset. Purging it keeps the hot memory footprint lean.

### The Sidecar: Separation of Knowing and Doing

A sidecar service replicates receipt information to a global state store or
long-term data lake. This solves the Observer Effect:

- **Zero impact on throughput**: The Execution Coordinator finishes the graph
  as fast as possible, hands the receipt to the sidecar, and moves on.
- **Centralized reconciliation**: While the Receipt is a local record, the
  Sidecar aggregates receipts to detect **Cross-Graph Drift**. If Action A in
  Graph 1 and Action B in Graph 2 both touch Resource X, the sidecar spots
  the conflict.

### Receipt Structure

The Receipt augments the graph as an envelope around the AST-generated nodes:

| Component | Role | Persistence |
|-----------|------|-------------|
| Original Graph | The Intent (Nodes and Edges) | Permanent |
| Audit Metadata | The History (Who, When, Duration) | Permanent |
| Reconciliation Snapshots | The Anchor (The observed hash/version) | Permanent |
| Recovery Pointer | A temporary ID to the LIFO stack | Deleted on Finish |

### Forensic Readiness

1. **System crashes during execution**: The Recovery Stack (still in existence)
   handles the undo.
2. **System drifts two weeks later**: The Reconciliation data in the Receipt
   (stored by the Sidecar) detects it.
3. **Auditor asks for proof**: The Receipt provides the full narrative.

### Design Constraint: Resource-Keyed Indexing

When replicating to the Sidecar, Reconciliation Data must be indexed by the
**External Resource ID**, not just the Graph ID. The system must answer
"What is the current expected state of Server-7?" rather than just
"What did Graph-452 do?"

## Related Documents

- [Binding Unification](./binding-unification.md) -- parent plan
- [Phase 8](./binding-unification/phase-8.md) -- compensation classification, provider contracts
- Issue #156 -- Audit, Reconciliation, and Recovery in the Execution Graph
