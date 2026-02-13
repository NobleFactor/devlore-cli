# Phase Execution Model for Lifecycle Pipelines

## Status

**Approved** — Core types, executor, and Starlark integration implemented.
Compensation ownership model under discussion.

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

## Starlark Integration

Phase scripts use three entry points:

- **`forward(package, system, plan)`** — the forward action. Emits nodes into
  the phase via plan receivers. Required.
- **`compensate(package, system, plan)`** — the compensating action. Emits nodes
  into a paired compensating phase. Optional.
- **`configure(phase)`** — sets phase-level configuration (retry policy) before
  forward execution. Optional.

The builder calls `configure()` first (if present), then `forward()`, then
checks for `compensate()`. If `compensate()` exists, the builder creates a
paired compensating phase and executes `compensate()` to populate it.

Scripts that only define `forward()` produce a phase with no compensation.
This is appropriate for idempotent phases like `verify`.

### Retry Policy from Scripts

```python
def configure(phase):
    phase.retry(max_attempts=3, backoff="exponential",
                initial_delay="1s", max_delay="30s")
```

## Compensation Ownership

### The Problem

Compensation is not the syntactic inverse of the forward action. It is the
**semantic inverse**, which depends on what actually happened at runtime.

Example: `plan.package.install("ripgrep")` creates a `package-install` node.
At execution time:

- If ripgrep was already installed → the operation is a no-op.
- If ripgrep was NOT installed → the operation installs it.

The compensating action depends on which case occurred:

- If we installed it → remove it.
- If it was already there → do nothing.

The compensating action needs the **receipt** from the forward action — what did
it actually do? That is the S in the (A, C, S) tuple. Without S, compensation
is blind.

### Return Signature: Result, State, Error

An operation produces three distinct things:

| Return | Consumer | Purpose |
|--------|----------|---------|
| **Result** | Executor | Outcome (status, checksums). Drives "continue or fail?" |
| **State** | Compensate() | Compensation receipt. The S in (A, C, S) |
| **Error** | Executor | Go error if the operation failed |

Result tells the executor what happened. State tells the compensator what to
undo. Error tells Go whether to propagate. These serve different consumers and
must not be conflated.

State is **opaque to the executor**. The executor saves it and hands it back
during compensation. Only the operation knows what its own state means.
`PackageInstallOp` writes `{"already_installed": true}` and reads it back;
the executor never looks inside.

### Where Compensation Resides

Compensation lives on the **operation implementation struct** — the same struct
that implements the forward action. The struct owns both directions because it
understands its own semantics.

```go
// PackageInstallOp — forward and reverse on the same struct
type PackageInstallOp struct{}

func (o *PackageInstallOp) Execute(ctx *Context, node Executable) error {
    // Forward: check state, install if needed, write receipt
    alreadyInstalled := pm.IsInstalled(pkg)
    node.SetState("already_installed", alreadyInstalled)
    if !alreadyInstalled {
        pm.Install(pkg)
    }
    return nil
}

func (o *PackageInstallOp) Compensate(ctx *Context, node Executable, state map[string]any) error {
    // Reverse: read receipt, undo only what we did
    if alreadyInstalled, _ := state["already_installed"].(bool); alreadyInstalled {
        return nil // we didn't install it — nothing to undo
    }
    return pm.Remove(pkg)
}
```

### Implementation Struct Contract

Today, operation structs (`PackageInstallOp`, `LinkOp`, etc.) have logic
inline — `PackageInstallOp.Execute()` calls `pm.Install()` directly. There is
no separate backing implementation. This conflates graph adaptation (slot
reading, type conversion) with business logic (is the package installed?
install it, record what we did).

The implementation struct is a **backing Go struct** that:

1. Is directly callable from Go (testable without the graph, nodes, or executor)
2. Is the single source of truth for both forward and compensating logic
3. Is the target that the auto-generated ops struct delegates to

```
Starlark script  → plan receiver  → creates Node in graph
                                        ↓
Graph executor   → ops struct     → reads slots, delegates to impl
                   (auto-generated)     ↓
                   impl struct    → actual logic (forward + compensate)
                   (hand-written,   directly callable from Go)
```

The implementation struct's methods follow a convention:

- **Forward**: `func(params...) (map[string]any, error)` — returns compensation
  state (the receipt) and error. The state is the S in (A, C, S).
- **Compensate**: `func(params..., state map[string]any) error` — receives the
  state from the forward method and returns error.

```go
// Implementation struct — directly callable, testable without the graph.
type PackageImpl struct {
    pm host.PackageManager
}

// Forward: returns (state, error).
func (p *PackageImpl) Install(packages []string) (map[string]any, error) {
    already := p.pm.IsInstalled(packages[0])
    if !already {
        result := p.pm.Install(packages...)
        if !result.OK {
            return nil, fmt.Errorf("install failed: %s", result.Stderr)
        }
    }
    return map[string]any{"already_installed": already}, nil
}

// Compensate: receives state from Install, returns error.
func (p *PackageImpl) CompensateInstall(packages []string, state map[string]any) error {
    if already, _ := state["already_installed"].(bool); already {
        return nil // nothing to undo
    }
    result := p.pm.Remove(packages[0])
    if !result.OK {
        return fmt.Errorf("remove failed: %s", result.Stderr)
    }
    return nil
}
```

The generated ops struct delegates to the implementation struct, handling the
graph-level concerns (slot reading, state wiring, dry-run logging):

```go
// Generated — adapts the implementation to the graph execution model
type PackageInstallOp struct{}

func (o *PackageInstallOp) Execute(ctx *Context, node Executable) error {
    packages := parsePackages(node.GetSlot("packages"))
    impl := &PackageImpl{pm: resolvePMForInstall(node.GetSlot("manager"))}

    if ctx.DryRun {
        fmt.Fprintf(ctx.Logger, "[dry-run] package-install %s\n", strings.Join(packages, " "))
        return nil
    }

    state, err := impl.Install(packages)
    if err != nil {
        return err
    }
    for k, v := range state {
        node.SetState(k, v)
    }
    return nil
}

func (o *PackageInstallOp) Compensate(ctx *Context, node Executable, state map[string]any) error {
    packages := parsePackages(node.GetSlot("packages"))
    impl := &PackageImpl{pm: resolvePMForInstall(node.GetSlot("manager"))}
    return impl.CompensateInstall(packages, state)
}
```

### Method Pairing for Code Generation

The code generator inspects the implementation struct and pairs methods by
naming convention:

| Forward method | Compensate method |
|---|---|
| `Install(...)` | `CompensateInstall(...)` |
| `Link(...)` | `CompensateLink(...)` |
| `Copy(...)` | `CompensateCopy(...)` |

Forward methods return `(map[string]any, error)`. Compensate methods accept
the same parameters plus a `state map[string]any` and return `error`.

Methods without a `Compensate` pair are not compensable. The code generator
produces an ops struct with `Execute()` only — no `Compensate()` method.

Gate validation for code generation:

- **Gate 1** (existing): All parameter types must map to Starlark types
- **Gate 2** (existing): Forward return must be `(map[string]any, error)`
- **Gate 3** (new): If a `Compensate` pair exists, its return must be `error`
  and its parameters must match the forward method's parameters plus
  `state map[string]any`

### CompensableOperation Interface

```go
// CompensableOperation is implemented by operations that can undo their effects.
// Orthogonal to the execution category (Transform, Writer, Direct).
type CompensableOperation interface {
    Operation
    Compensate(ctx *Context, node Executable, state map[string]any) error
}
```

Not all operations are compensable:

| Operation | Compensable | Reason |
|-----------|-------------|--------|
| `package-install` | Yes | Can remove what was installed |
| `link` | Yes | Can unlink what was linked |
| `copy` | Yes | Can remove what was copied (with backup) |
| `render` | Yes | Same as copy (output is a file) |
| `decrypt` | No | Transform only — no side effects to undo |
| `validate` | No | Read-only probe — nothing to undo |
| `shell` | No | Arbitrary command — cannot auto-compensate |

Shell operations are not compensable at Layer 1. If a script runs
`plan.shell(command="...")`, the operation cannot know how to reverse an
arbitrary command. Compensation for shell operations belongs at Layer 2
(scripted phase-level compensation).

### State Capture Mechanism

Operations write state to the node during execution via `Executable`:

```go
type Executable interface {
    GetID() string
    GetOperation() string
    GetSlot(name string) string
    GetProject() string
    GetMode() os.FileMode
    SetState(key string, value any)   // new
    GetState(key string) any          // new
}
```

The executor reads state from the node after execution and stores it in the
Result. This avoids changing the three operation interface signatures
(Transform, Writer, Direct) — operations write state as a side effect of
execution, not as a return value.

`BackupOp` already does this by dirty-casting `node.(*Node)` to write
`Annotations["backup_path"]`. The `SetState`/`GetState` API replaces that
pattern with a clean interface.

```go
// Node gains a State field for compensation receipts
type Node struct {
    // ...existing fields...
    State map[string]any `json:"state,omitempty" yaml:"state,omitempty"`
}

func (n *Node) SetState(key string, value any) {
    if n.State == nil {
        n.State = make(map[string]any)
    }
    n.State[key] = value
}

func (n *Node) GetState(key string) any {
    if n.State == nil {
        return nil
    }
    return n.State[key]
}
```

Result gains a State field:

```go
type Result struct {
    NodeID         string
    Status         ResultStatus
    Error          error
    State          map[string]any  // compensation receipt from node
    SourceChecksum string
    TargetChecksum string
}
```

### Executor State Flow

```
Forward execution:
  1. executor allocates node
  2. op.Execute(ctx, node)
     → operation writes node.SetState("already_installed", false)
  3. executor captures node.State into Result.State
  4. Result.State pushed to recovery stack

Compensation (on failure):
  5. executor pops recovery stack (LIFO)
  6. for each completed node (reverse order within phase):
     → op.Compensate(ctx, node, savedState)
     → operation reads state, decides what to undo
```

### Two Layers

There are two distinct layers of compensation that serve different purposes.

**Layer 1 — Operation-level (automatic, fine-grained)**

Each operation knows its own inverse. The executor calls `Compensate()` on
each completed node within a failed phase, in reverse order. No Starlark
involvement. The operation is the single source of truth.

During phase unwind, the executor:
1. Iterates completed nodes in reverse order.
2. Looks up the operation in the registry.
3. If the operation implements `CompensableOperation`, calls `Compensate()`
   with the node's saved state.
4. If the operation does not implement `CompensableOperation`, skips it (logged).

**Layer 2 — Phase-level (scripted, coarse-grained)**

For orchestration logic beyond what individual operations provide: cleaning up
temporary artifacts, sending notifications, restoring backups, coordinating
multi-step rollback sequences that span concerns.

This is where the Starlark `compensate()` function lives.

**Layer interaction**: Layer 1 runs first (reverse-order node compensation
within the phase). Layer 2 runs after (phase-level scripted compensation).
Layer 2 does not subsume Layer 1 — they are complementary. A phase can have
Layer 1 only, Layer 2 only, both, or neither.

### Build Time vs Runtime

The current implementation runs `compensate()` at build time to emit nodes
into the graph. This gives dry-run visibility but cannot handle
state-dependent compensation.

Resolution:

- **Build time (dry-run)**: The graph shows compensation **capability** — which
  operations are compensable (interface check), which phases have scripted
  compensation (compensate function exists). This is metadata, not a plan.
- **Runtime**: The executor determines actual compensation from execution
  state. Node results carry compensation receipts. The recovery stack carries
  these receipts for unwind.
- **Transition**: The builder's current `compensate()` call at build time
  should be replaced with a compensation capability flag on the phase. The
  Starlark `compensate()` function moves to runtime execution during unwind.

### Resolved: State Serialization

Node `State` is `map[string]any` with no shape constraints. The implementation
owns its state and decides what it needs. Implementers are responsible for
ensuring their state round-trips through JSON and YAML marshalers with full
fidelity.

### Resolved: Non-Compensable Operations

The builder does not reject phases containing non-compensable operations. The
executor skips non-compensable operations during unwind (logged, not an error).
Shell operations are the primary example — they are an escape hatch that comes
with the cost of no automatic undo.

A structured return contract for shell compensation (JSON on stdout conforming
to a message type) is tracked as a future enhancement: issue #105.

## Files

| File | Role |
|------|------|
| `internal/execution/phase.go` | Phase, PhaseStatus, Attempt, RetryPolicy types |
| `internal/execution/recovery.go` | RecoveryStack, RecoveryEntry |
| `internal/execution/executor.go` | RunPhased, node compensation during unwind |
| `internal/execution/graph.go` | Node.State, Result.State, Executable.SetState/GetState |
| `internal/execution/operation.go` | CompensableOperation interface |
| `internal/execution/ops.go` | Compensate() on file operations (link, copy, etc.) |
| `internal/execution/ops_package.go` | Compensate() on package operations |
| `internal/starlark/phase_config.go` | PhaseConfig Starlark bindings for configure() |
| `internal/lore/builder.go` | Phase-aware graph builder |
