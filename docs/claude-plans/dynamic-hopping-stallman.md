# Plan: Phase Execution Model for Lifecycle Pipelines

## Context

The execution graph currently has no concept of "phase." Lifecycle pipelines
(prepare → install → provision → verify) are a build-time concept — Starlark
scripts emit nodes into a flat graph, and the executor runs them via topological
sort. This means:

- There is no error boundary between phases. A node failure is a graph failure.
- There is no retry at the phase level. Transient failures abort the whole run.
- There is no structured rollback. Partial success leaves the system in an
  indeterminate state.

This plan introduces **phases as first-class runtime concepts** in the execution
graph, implementing the **Saga Pattern** as a transactional state machine. Each
phase is a scoped transaction with a forward action, a compensating action, and
the execution state needed to undo itself. The executor becomes a saga
coordinator that walks phase boundaries, traps errors, retries, and unwinds.

## Architecture: Phase Execution Model

### The Saga Pattern

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

### Phase as (A, C, S) Tuple

Each phase is defined by the tuple:

| Component | Role | Example |
|-----------|------|---------|
| **A** (Action) | Forward operation | Install binary, link config |
| **C** (Compensate) | Reverse operation | Remove binary, unlink config |
| **S** (State) | Metadata captured during A that C needs | Installed version, created paths, backed-up files |

A is obligated to populate S during forward execution. S is the receipt of A
and the input to C. If A doesn't capture enough state, C can't undo the work.

### Phase Boundary Nodes (Checkpoints)

A phase is a **dual-method node** in the graph — a stateful controller that
encapsulates both the forward and compensating actions. The executor recognizes
phase nodes as boundaries.

Phase node structure:

| Field | Type | Description |
|-------|------|-------------|
| ID | string | Unique identifier (e.g., `"phase.install"`) |
| Name | string | Phase name (e.g., `"install"`) |
| Retry | *RetryPolicy | Max retries, backoff, timeout |
| Status | PhaseStatus | pending, completed, failed, rolled_back |
| Nodes | []string | IDs of inner nodes belonging to this phase |
| Compensate | CompensateRef | Reference to compensating action |
| Attempts | []Attempt | Retry history (populated in receipt) |
| State | map[string]any | Execution state captured during forward action (S) |

A phase with `Checkpoint: true` (all phases in a lifecycle pipeline) is a
**recovery anchor** — the point where the executor traps errors and decides
retry vs. unwind.

### Recovery Pointer Stack

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

### Executor Loop

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

### Phase.Execute — The Trap

Each phase owns its retry logic. The executor calls `Phase.Execute()` and
sees either success or final failure:

```go
func (p *Phase) Execute(ctx *Context) error {
    for attempt := 0; attempt <= p.Retry.MaxAttempts; attempt++ {
        err := p.runInnerNodes(ctx)     // topological sort of inner nodes
        if err == nil {
            return nil                   // success → next phase
        }
        if attempt < p.Retry.MaxAttempts {
            backoff(p.Retry, attempt)
            continue                     // retry the whole phase
        }
    }
    return fmt.Errorf("phase %s failed after %d attempts", p.Name, p.Retry.MaxAttempts+1)
}
```

### Inner Node Failure

**Any inner node failure immediately fails the phase.** Inner nodes do not
have independent ErrorAction. When a node within a phase errors, execution of
that phase stops and the error bubbles to the phase boundary (the trap).

The phase's retry policy then governs: retry the entire phase, or give up and
trigger unwind.

This is a deliberate simplification. No two-level retry (node retry × phase
retry). No partial-phase-success semantics. A phase either fully succeeds or
fully fails.

### Rollback (Unwind)

When the executor decides to unwind:

1. Pop the recovery stack
2. For each entry (LIFO order): execute the compensating action
3. Compensating action receives the phase's captured State (S)
4. If a compensating action itself fails: log the error, continue unwinding
   (compensate failure does not mask the original error — both are reported)
5. Unwinding continues until the stack is empty

The undo stack is **runtime bookkeeping**, not a graph structure. The graph
provides the raw materials (compensate references on each phase). The executor
assembles them into a stack as phases complete. Virtual reverse edges — emergent,
not declared.

### Retry Policy

```go
type RetryPolicy struct {
    MaxAttempts  int             // 0 = no retry (fail immediately)
    Backoff      BackoffStrategy // none, linear, exponential
    InitialDelay time.Duration
    MaxDelay     time.Duration
}
```

Retry policy is **held by** the phase node but not necessarily **owned by** it.
Policies can be:
- Declared in the lifecycle manifest (static, visible in dry-run)
- Set by the Starlark script during graph construction (dynamic)
- Inherited from a graph-level or operation-level default

### Visibility

Because phases are nodes, everything is visible:
- **Plan (dry-run)**: shows phases, their inner nodes, retry policies, and
  compensating action references
- **Receipt**: shows phase statuses, retry attempts with timestamps, and
  which compensating actions executed during rollback
- **Logging**: "Phase install: attempt 2/3", "Rollback initiated: unwinding
  to boundary prepare"

## Starlark Integration

### Phase Declaration

Phase scripts declare both forward and compensating actions. Currently
lifecycle scripts are single-function. They would gain a second entry point:

```python
# install.star

def forward(ctx):
    """Forward action: install the package."""
    pkg.install("ripgrep")

def compensate(ctx):
    """Compensating action: undo the install."""
    pkg.remove("ripgrep")
```

The lifecycle runner looks for both `forward` and `compensate` functions.
If `compensate` is absent, the phase has no compensating action (acceptable
for idempotent phases like `verify`).

### State Capture

The `ctx` object passed to both `forward` and `compensate` provides access to
the phase's State (S). Forward actions write state; compensating actions read it:

```python
def forward(ctx):
    result = pkg.install("ripgrep")
    ctx.state["installed_version"] = result.version
    ctx.state["binary_path"] = result.path

def compensate(ctx):
    pkg.remove("ripgrep", version=ctx.state["installed_version"])
```

### Inner Node Emission

Within `forward()`, the script emits inner nodes via plan receivers (the
existing mechanism). These nodes become the phase's inner nodes:

```python
def forward(ctx):
    plan.file.link(source="completions/_rg", target=".zsh/completions/_rg")
    plan.file.copy(source="config.toml", target=".config/ripgrep/config.toml")
```

### Retry Policy Declaration

Retry policy can be set per-phase in the Starlark script:

```python
def configure(phase):
    """Called during graph construction to configure the phase."""
    phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s")
```

Or in the lifecycle manifest (YAML):

```yaml
phases:
  install:
    retry:
      max_attempts: 3
      backoff: exponential
      initial_delay: 1s
```

## Type Design

### New Types (`internal/execution/phase.go`)

```go
type PhaseStatus string

const (
    PhasePending    PhaseStatus = "pending"
    PhaseCompleted  PhaseStatus = "completed"
    PhaseFailed     PhaseStatus = "failed"
    PhaseRolledBack PhaseStatus = "rolled_back"
    PhaseSkipped    PhaseStatus = "skipped"
)

type Phase struct {
    ID         string            `json:"id" yaml:"id"`
    Name       string            `json:"name" yaml:"name"`
    Status     PhaseStatus       `json:"status" yaml:"status"`
    Retry      *RetryPolicy      `json:"retry,omitempty" yaml:"retry,omitempty"`
    NodeIDs    []string          `json:"nodes,omitempty" yaml:"nodes,omitempty"`
    Compensate string            `json:"compensate,omitempty" yaml:"compensate,omitempty"`
    Attempts   []Attempt         `json:"attempts,omitempty" yaml:"attempts,omitempty"`
    State      map[string]any    `json:"state,omitempty" yaml:"state,omitempty"`
}

type Attempt struct {
    Number    int       `json:"number" yaml:"number"`
    Status    string    `json:"status" yaml:"status"`
    Error     string    `json:"error,omitempty" yaml:"error,omitempty"`
    Timestamp string    `json:"timestamp" yaml:"timestamp"`
}

type BackoffStrategy string

const (
    BackoffNone        BackoffStrategy = "none"
    BackoffLinear      BackoffStrategy = "linear"
    BackoffExponential BackoffStrategy = "exponential"
)

type RetryPolicy struct {
    MaxAttempts  int             `json:"max_attempts" yaml:"max_attempts"`
    Backoff      BackoffStrategy `json:"backoff" yaml:"backoff"`
    InitialDelay time.Duration   `json:"initial_delay" yaml:"initial_delay"`
    MaxDelay     time.Duration   `json:"max_delay" yaml:"max_delay"`
}
```

### RecoveryStack (`internal/execution/recovery.go`)

```go
type RecoveryEntry struct {
    PhaseID    string
    PhaseName  string
    Compensate func(ctx *Context) error
    State      map[string]any
}

type RecoveryStack struct {
    entries []RecoveryEntry
}

func (s *RecoveryStack) Push(entry RecoveryEntry)
func (s *RecoveryStack) Unwind(ctx *Context) []error  // LIFO, returns all errors
func (s *RecoveryStack) Len() int
```

### Graph Changes (`internal/execution/graph.go`)

```go
type Graph struct {
    // ...existing fields unchanged...

    // Phases defines the ordered lifecycle phases (nil for non-phased graphs).
    // When present, the executor uses phase-aware execution.
    // When nil, the executor falls back to flat node execution (current behavior).
    Phases []*Phase `json:"phases,omitempty" yaml:"phases,omitempty"`
}
```

The `Phases` field is optional. Writ graphs that don't use phases continue to
work exactly as before. Lore lifecycle graphs populate `Phases` to enable
phase-aware execution.

## Graph Serialization

### Plan (before execution)

```yaml
tool: lore
state: pending
phases:
  - id: phase.prepare
    name: prepare
    status: pending
    nodes: [probe-brew, probe-disk]
  - id: phase.install
    name: install
    status: pending
    retry:
      max_attempts: 3
      backoff: exponential
      initial_delay: 1s
    nodes: [package-install-ripgrep]
    compensate: phase.install.compensate
  - id: phase.provision
    name: provision
    status: pending
    nodes: [link-completions, copy-config]
    compensate: phase.provision.compensate
  - id: phase.verify
    name: verify
    status: pending
    nodes: [verify-rg-version]
nodes:
  - id: probe-brew
    operation: probe
    status: pending
  # ...
```

### Receipt (after execution with rollback)

```yaml
tool: lore
state: failed
phases:
  - id: phase.prepare
    name: prepare
    status: completed
  - id: phase.install
    name: install
    status: completed
    attempts:
      - { number: 1, status: failed, error: "mirror timeout", timestamp: "..." }
      - { number: 2, status: completed, timestamp: "..." }
  - id: phase.provision
    name: provision
    status: rolled_back
    attempts:
      - { number: 1, status: failed, error: "permission denied", timestamp: "..." }
  - id: phase.verify
    name: verify
    status: skipped
rollback:
  - { phase: provision, compensate: phase.provision.compensate, status: completed }
  - { phase: install, compensate: phase.install.compensate, status: completed }
```

## Implementation Plan

### Step 1: Architecture Document

Create `docs/architecture/devlore-phase-execution.md` capturing the design
above. This is the reference document for the execution model.

### Step 2: Core Types

Create `internal/execution/phase.go`:
- `Phase`, `PhaseStatus`, `Attempt` structs
- `RetryPolicy`, `BackoffStrategy`

Create `internal/execution/recovery.go`:
- `RecoveryStack`, `RecoveryEntry`
- `Push()`, `Unwind()`, `Len()`

### Step 3: Phase-Aware Executor

Modify `internal/execution/executor.go`:
- Add `RunPhased(ctx, graph)` method (phase-aware loop)
- Existing `Run()` checks `graph.Phases != nil` → delegates to `RunPhased`
- `RunPhased` implements: outer phase loop, inner node execution, recovery
  stack management, retry logic, LIFO unwind on failure
- Inner node failure immediately fails the phase (no ErrorAction within phases)

### Step 4: Graph Serialization

Modify `internal/execution/graph.go`:
- Add `Phases []*Phase` to `Graph`
- Update `CanonicalContent()` to include phases
- Update `ComputeSummary()` to account for phase statuses
- Update `ApplyResults()` for phase-level results

### Step 5: Starlark Phase Interface

Modify lifecycle Starlark integration:
- Phase scripts gain `forward()` / `compensate()` entry points
- Phase `ctx` exposes `state` dict for (S) capture
- `configure(phase)` hook for retry policy
- Plan receivers emit nodes that are associated with the current phase

### Step 6: Lifecycle Builder Integration

Modify `internal/lorepackage/lifecycle.go` and lore graph builders:
- `DiscoverPhaseScripts` returns scripts that the builder wraps in Phase nodes
- Graph builder creates Phase entries in the graph
- Phase ordering from `DeployPhaseOrder` etc. maps to graph phase sequence

## Files

| File | Action |
|------|--------|
| `docs/architecture/devlore-phase-execution.md` | Create |
| `internal/execution/phase.go` | Create |
| `internal/execution/phase_test.go` | Create |
| `internal/execution/recovery.go` | Create |
| `internal/execution/recovery_test.go` | Create |
| `internal/execution/executor.go` | Modify (add RunPhased) |
| `internal/execution/executor_test.go` | Modify (phase execution tests) |
| `internal/execution/graph.go` | Modify (add Phases field) |
| `internal/lorepackage/lifecycle.go` | Modify (phase script dual entry point) |

## Verification

1. `go test ./internal/execution/...` — all existing tests pass (non-phased graphs unchanged)
2. Unit tests for `RecoveryStack`: push, unwind LIFO, compensate error handling
3. Unit tests for `Phase.Execute`: success, retry-then-succeed, retry-exhausted
4. Integration test: 4-phase pipeline, failure at phase 3, verify phases 1-2 compensated in reverse
5. Integration test: retry succeeds on attempt 2, verify receipt records both attempts
6. Serialization round-trip: phased graph → YAML → parse → verify phases intact
