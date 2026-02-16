# Phase Execution Model for Lifecycle Pipelines

## Status

**Approved** — Core types, executor, and Starlark integration implemented.
Activity model (Forward/Backward on Provider structs) formalized. Compensation
not yet implemented — pending Provider struct extraction and code generation.

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

## Activities

An Activity is a unit of work on a Provider — the Saga pattern's term for a
local transaction. Each Activity comprises up to two methods on a Provider struct:

| Method | Role | Naming | Signature |
|--------|------|--------|-----------|
| **Forward** | Execute the operation | The method itself: `Copy`, `Install` | `func(params...) (...result, map[string]any, error)` |
| **Backward** | Compensate the operation | `Compensate` prefix: `CompensateCopy` | `func(state map[string]any) error` |

Activity is a design concept, not a Go type. The generator detects Activities
by finding `Compensate<MethodName>` pairs on the Provider struct. No annotation,
no interface — naming convention is the contract.

### Forward

Forward executes the business logic. Its return signature encodes three things:

| Output | Position | Consumer | Purpose |
|--------|----------|----------|---------|
| **result** | Before state | Graph/executor | Content, checksums — flows to downstream nodes via Result |
| **state** | `map[string]any` | Backward | Compensation receipt — the S in (A, C, S). Opaque to the executor |
| **error** | Last | Go | Whether the action failed |

The generator determines the content model from what precedes state and error:

| Returns (after stripping state + error) | Content model |
|---|---|
| Nothing | No content (direct op) |
| `string` | Content consumer (string = checksum) |
| `[]byte` | Content transformer (bytes = transformed content) |

Non-compensable methods (no `Compensate` pair) omit the `map[string]any` state
return. Their signatures follow the existing convention: `error`,
`(string, error)`, or `([]byte, error)`.

### Backward

Backward receives the state saved by Forward and undoes the operation:

```
Forward(params...)  → (result, state, error)
                         ↓        ↓
                       graph    saved on node
                                   ↓
Backward(state)     → error
```

- Backward has no result. Compensation is terminal — nothing downstream
  consumes its output.
- Backward has no state. There is no "compensate the compensation" — the saga
  pattern does not nest.
- If Backward needs Forward's result, Forward saves it to state. State is the
  single channel from Forward to Backward.
- If Backward needs Forward's params (e.g., which packages to remove), Forward
  saves them to state.

### Example: file.Provider

```go
// provider/file/provider.go
type Provider struct{}

// --- Copy Activity ---

// Forward: write content to path. Returns checksum (result), state, error.
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, map[string]any, error) {
    existed := fileExists(path)
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return "", nil, fmt.Errorf("create parent dirs: %w", err)
    }
    if err := os.WriteFile(path, content, mode); err != nil {
        return "", nil, err
    }
    checksum := ChecksumBytes(content)
    return checksum, map[string]any{"path": path, "existed": existed}, nil
}

// Backward: undo Copy.
func (p *Provider) CompensateCopy(state map[string]any) error {
    path, _ := state["path"].(string)
    existed, _ := state["existed"].(bool)
    if existed {
        return nil // file was already there — don't remove it
    }
    return os.Remove(path)
}

// --- Move Activity ---

// Forward: rename source to path. Returns state, error.
func (p *Provider) Move(source, path string) (map[string]any, error) {
    if err := os.Rename(source, path); err != nil {
        return nil, err
    }
    return map[string]any{"source": source, "path": path}, nil
}

// Backward: undo Move.
func (p *Provider) CompensateMove(state map[string]any) error {
    source, _ := state["source"].(string)
    path, _ := state["path"].(string)
    return os.Rename(path, source)
}
```

### Generated Actions

The generator reads the Provider, finds the Activity pairs, and produces
action structs that implement the unified `Action` interface (Do + Undo):

```go
// provider/file/actions_gen.go — generated, nuke-safe
type Copy struct{ Impl *Provider }

func (o *Copy) Name() string { return "file.copy" }

func (o *Copy) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
    path := slots["path"].(string)
    mode, _ := slots["mode"].(os.FileMode)
    content, _ := slots["content"].([]byte)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
        ctx.TargetChecksum = checksumBytes(content)
        return nil, nil, nil
    }
    checksum, state, err := o.Impl.Copy(path, mode, content)
    if err != nil {
        return nil, nil, err
    }
    ctx.TargetChecksum = checksum
    return nil, state, nil
}

func (o *Copy) Undo(ctx *execution.Context, slots map[string]any, state execution.UndoState) error {
    s, _ := state.(map[string]any)
    return o.Impl.CompensateCopy(s)
}
```

The generated `Undo` method is trivial — it passes state through to the
Provider's Backward method. All compensation logic lives in the Provider.

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

### Where Compensation Resides

Compensation lives on the **Provider struct** — the same struct that implements
the forward method. The Provider owns both directions because it understands its
own semantics. See [Activities](#activities) for the full contract.

The three-layer delegation:

```
Starlark script  → plan receiver  → creates Node in graph
                                        ↓
Graph executor   → action struct  → reads slots, delegates to Provider
                   (generated)          ↓
                   Provider struct → actual logic (Forward + Backward)
                   (hand-written,  directly callable from Go)
```

### Undo on Action

Every action implements both `Do` and `Undo`. Non-compensable actions
return nil from `Undo` — the executor skips them during unwind:

| Action | Undo behavior | Reason |
|--------|---------------|--------|
| `pkg.install` | Removes installed packages | Can remove what was installed |
| `file.link` | Unlinks created symlink | Can unlink what was linked |
| `file.copy` | Removes copied file | Can remove what was copied |
| `template.render` | Same as copy | Output is a file |
| `encryption.decrypt` | No-op (nil) | Transform only — no side effects to undo |
| `shell.exec` | No-op (nil) | Arbitrary command — cannot auto-compensate |

Shell actions are not compensable at Layer 1. Compensation for shell
actions belongs at Layer 2 (scripted phase-level compensation).

### Gate Validation for Code Generation

The generator validates Activity pairs at discovery time:

- **Gate 1**: All parameter types must map to Starlark types
- **Gate 2**: Forward return must follow the content model convention
  (`error`, `(string, error)`, `([]byte, error)`, or their compensable variants
  with `map[string]any` state)
- **Gate 3**: If a `Compensate<Method>` pair exists, its signature must
  be `func(state map[string]any) error`

### State Capture

The generated action captures state from the Provider's Forward return and
returns it as `UndoState` from `Do`. The executor stores UndoState in a
`RecoveryEntry` on the recovery stack.

State is **opaque to the executor**. The executor stores it and passes it
back to `Undo` during compensation. Only the Provider knows what its own
state means.

### Executor State Flow

```
Forward execution:
  1. executor resolves promise slots → flat map[string]any
  2. executor calls action.Do(ctx, slots)
  3. generated action reads slots, calls provider.Forward(params...)
  4. Forward returns (result, state, error)
  5. Do returns (Result, UndoState, error) to executor
  6. executor stores Result for downstream promise resolution
  7. executor pushes RecoveryEntry{Node, UndoState} to recovery stack

Compensation (on failure):
  8. executor pops recovery stack (LIFO)
  9. for each completed node (reverse order within phase):
     → executor resolves slots again via node.ResolvedSlots(results)
     → executor calls action.Undo(ctx, slots, savedState)
     → generated action passes state to provider.Compensate<Method>(state)
     → Provider reads state, decides what to undo
```

### Two Layers

There are two distinct layers of compensation that serve different purposes.

**Layer 1 — Action-level (automatic, fine-grained)**

Each action knows its own inverse via `Undo`. The executor calls `Undo()` on
each completed node within a failed phase, in reverse order. No Starlark
involvement. The action is the single source of truth.

During phase unwind, the executor:
1. Iterates completed nodes in reverse order.
2. Calls `node.Action.Undo(ctx, slots, savedState)`.
3. If Undo returns nil (no-op), the executor logs and continues.
4. If Undo returns an error, the executor logs the error and continues unwinding.

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

Node `State` is `map[string]any` with no shape constraints. The Provider owns
its state and decides what it needs. Implementers are responsible for ensuring
their state round-trips through JSON and YAML marshalers with full fidelity.

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
| `internal/execution/graph.go` | Node, ResolvedSlots, ActionName, custom marshal |
| `internal/execution/action.go` | Action interface, Result, UndoState, Context |
| `internal/execution/registry.go` | ActionRegistry (Get, MustGet, Register, Names) |
| `internal/execution/provider/register_gen.go` | RegisterAll — calls all provider Register functions |
| `internal/execution/provider/*/actions_gen.go` | Generated action structs (Do, Undo, Register) |
| `internal/execution/provider/*/provider.go` | Hand-written Provider structs (business logic) |
| `internal/starlark/phase_config.go` | PhaseConfig Starlark bindings for configure() |
| `internal/lore/builder.go` | Phase-aware graph builder |

## Related Documents

- [Typed Slots and Context Data](devlore-typed-slots.md) — Terminology, slot
  model, Provider structs, generated code patterns
- [Graph Operations](devlore-graph-convergence-operations.md) — Convergence,
  control flow, and system interaction (probe, guard, choose, retry, rollback)
- [Action Namespaces](devlore-operation-namespaces.md) — How to add new
  action namespaces
