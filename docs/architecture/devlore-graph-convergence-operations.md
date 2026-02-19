# Graph Operations: Convergence and Control Flow

> **Partial supersession notice.** The Gather, Choose, and Convergence Comparison
> sections of this document are superseded by
> [Orchestration Primitives](devlore-orchestration-primitives.md), which refines
> these operations with phase-level execution, runtime predicates, and SlotProxy.
> The Retry, Rollback, and Elevate sections remain current.

This document describes the graph operations that encode decision-making in the plan. Every conditional branch, failure policy, and privilege transition is an explicit graph node — visible in dry-run output and recorded in the receipt.

See also:

- [Execution Graph](devlore-execution-graph.md) — Core graph architecture and state machine
- [Orchestration Primitives](devlore-orchestration-primitives.md) — Refined Gather,
  Choose, WaitUntil, Sidecar (supersedes sections of this document)
- [Emergent System Model](devlore-emergent-system-model.md) — System-level architecture,
  dependency taxonomy (structural, functional, procedural)

Tracking issue: https://github.com/NobleFactor/devlore-cli/issues/92

## Design Principle

**The graph is self-contained.** No hidden runtime queries. Every decision the executor makes is rooted in a graph node whose inputs and outputs are traceable. The plan shows what will happen and why. The receipt shows what happened and why.

| Runtime behavior | Graph operation |
|---|---|
| Select among alternatives | **Choose** |
| Wait for all dependencies | **Gather** |
| Handle transient failure | **Retry** |
| Undo on failure | **Rollback** |
| Privilege transition | **Elevate** |

---

## Convergence Operations

### Gather (AND)

**Semantics**: Wait for **all** predecessors to complete. Collect all results, then proceed.

Equivalent to `Promise.all()`. Every input must succeed for the gather node to succeed.

#### Use Cases

**Install a stack, then verify the whole thing**:

```
  ┌──────────┐  ┌────────────────┐  ┌───────────┐
  │ docker   │  │ docker-compose │  │ kubectl   │
  │ install  │  │ install        │  │ install   │
  └────┬─────┘  └───────┬────────┘  └─────┬─────┘
       │                │                  │
       └────────────────┼──────────────────┘
                        │ gather
                        ▼
                ┌───────────────┐
                │ verify stack  │
                │ (all 3 ready) │
                └───────────────┘
```

**Collect artifacts from parallel builds**:

```
  ┌────────────┐  ┌────────────┐
  │ build arm64│  │ build amd64│
  └──────┬─────┘  └──────┬─────┘
         │               │
         └───────┬───────┘
                 │ gather
                 ▼
         ┌───────────────┐
         │ package bundle│
         │ (both archs)  │
         └───────────────┘
```

#### Execution Rules

1. The gather node blocks until **all** predecessors reach a terminal state (completed, skipped, or failed)
2. If any predecessor fails, the gather node fails (unless configured to tolerate partial failure)
3. The gather node receives a list of all predecessor results
4. Predecessor execution order is unspecified — they may run in parallel

#### Node Representation

```yaml
- id: verify-container-stack
  mode: gather
  action: shell.exec
  depends_on: [install-docker, install-compose, install-kubectl]
  slots:
    command: "docker compose version && kubectl version --client"
```

### Choose (OR)

**Semantics**: Evaluate **alternatives** and select one. Multiple predecessors represent options, not dependencies. The node picks based on criteria — platform, availability, preference, or runtime condition.

Only one input is selected. Unchosen branches are skipped, not executed.

#### Use Cases

**Platform-specific installation**:

```
  ┌──────────┐  ┌──────────┐  ┌──────────┐
  │ brew     │  │ apt      │  │ dnf      │
  │ (macOS)  │  │ (Debian) │  │ (Fedora) │
  └────┬─────┘  └────┬─────┘  └────┬─────┘
       │              │              │
       └──────────────┼──────────────┘
                      │ choose (platform)
                      ▼
              ┌───────────────┐
              │ python ready  │
              └───────────────┘
```

**Availability-based selection**:

```
  ┌──────────────┐  ┌──────────────┐
  │ pyenv        │  │ system python│
  │ (if present) │  │ (fallback)   │
  └──────┬───────┘  └──────┬───────┘
         │                 │
         └────────┬────────┘
                  │ choose (availability)
                  ▼
          ┌───────────────┐
          │ python ready  │
          └───────────────┘
```

#### Selection Criteria

| Criteria | Description | Example |
|----------|-------------|---------|
| `platform` | Select based on OS/distro | brew on macOS, apt on Debian |
| `availability` | Select first available option | pyenv if installed, else system python |
| `preference` | Select based on user/manifest preference | user prefers nvm over fnm |
| `condition` | Select based on runtime evaluation | version check, feature flag |

#### Execution Rules

1. The choose node evaluates selection criteria **before** executing predecessors
2. Only the selected predecessor is executed; all others are marked `skipped`
3. If the selected predecessor fails, the choose node fails (no automatic fallback to other options)
4. Selection criteria can be static (platform, known at graph build time) or dynamic (availability, evaluated at execution time)

#### Static vs Dynamic Selection

**Static choose** (resolved at graph build time):
- Platform selection — the OS is known before execution
- The graph builder prunes unchosen branches entirely
- No choose node appears in the final graph; only the selected branch remains

**Dynamic choose** (resolved at execution time):
- Availability — requires runtime probing (is pyenv installed?)
- The choose node and all alternatives appear in the graph
- The executor evaluates criteria and skips unchosen branches

#### Node Representation

```yaml
- id: python-ready
  mode: choose
  criteria: platform
  alternatives:
    - node: install-python-brew
      when: { os: darwin }
    - node: install-python-apt
      when: { os: linux, distro: debian }
    - node: install-python-dnf
      when: { os: linux, distro: fedora }
```

### Convergence Comparison

| Property | Gather | Choose |
|----------|--------|--------|
| Fan-in edges | All consumed | One selected |
| Predecessor execution | All run (potentially parallel) | One runs, rest skipped |
| Failure mode | Fails if any input fails | Fails if selected input fails |
| Analogy | `Promise.all()` | `switch/case` |
| Graph optimization | Can parallelize predecessors | Can prune unchosen branches |
| Typical use | Verify a composed stack | Platform/availability dispatch |

---

## Control Flow Operations

### Retry

**Semantics**: Declarative retry policy attached to a node. If the node fails, re-execute according to the policy. Without retry in the graph, transient failure handling lives in Starlark scripts. With it, the plan declares the policy and the receipt records every attempt.

#### Use Cases

- **Package manager failures**: Mirror timeout, lock contention, index refresh race
- **Network operations**: Registry unreachable, download interrupted
- **Service startup**: Service takes time to become healthy after start

#### Execution Rules

1. On failure, the executor re-executes the node according to the retry policy
2. Each attempt is recorded in the receipt with its result and timestamp
3. If all attempts fail, the node is marked `failed` with the last error
4. Backoff strategy prevents hammering a failing resource

#### Node Representation

```yaml
- id: install-docker
  action: pkg.install
  retry:
    max_attempts: 3
    backoff: exponential  # none, linear, exponential
    initial_delay: 1s
    max_delay: 30s
```

Receipt output:

```yaml
- id: install-docker
  status: completed
  attempts:
    - { attempt: 1, status: failed, error: "mirror timeout", timestamp: "..." }
    - { attempt: 2, status: failed, error: "lock held by apt", timestamp: "..." }
    - { attempt: 3, status: completed, timestamp: "..." }
```

### Rollback

**Semantics**: State-based compensating action. Each action's `Do` method returns rollback state that the executor stores on the recovery stack. If a later node fails, the executor unwinds the stack in reverse order, passing the stored state to each action's `Undo` method.

No separate rollback nodes. The action that performed the work provides both `Do` and `Undo`, and `Do` captures exactly the state needed for compensation.

```
  Do:   install-docker → returns rollback state {packages: [...]}
  Do:   configure-docker → returns rollback state {config_backup: "..."}
  FAIL: start-docker
  Undo: configure-docker(state) → restores config
  Undo: install-docker(state) → removes packages
```

#### Use Cases

- **Partial manifest failure**: 3 of 5 packages installed, then the 4th fails. Each successful install's `Do` returned rollback state; `Undo` uses it to remove installed packages.
- **Configuration changes**: `Do` backs up the original config and returns it as rollback state. `Undo` restores from that state.

#### Execution Rules

1. `Do` returns rollback state (`any`) — the executor stores it on the recovery stack
2. On failure, the executor unwinds the recovery stack in reverse order (last completed first)
3. `Undo` receives the rollback state captured by `Do` for that node
4. Actions with no rollback return `nil` state from `Do`; their `Undo` is a no-op
5. The receipt records which rollbacks executed and their results
6. Rollback failure does not mask the original error — both are reported

### Elevate

**Semantics**: Explicit privilege boundary in the graph. Makes the transition from unprivileged to privileged execution (and back) visible as a graph node. Currently `sudo` is embedded in shell commands — invisible to the plan. With elevate, dry-run shows "root required here" and the receipt records when privilege was acquired and released.

```
  ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
  │ download │ →   │ elevate  │ →   │ install  │ →   │ drop     │
  │ (user)   │     │ (→ root) │     │ (root)   │     │ (→ user) │
  └──────────┘     └──────────┘     └──────────┘     └──────────┘
```

#### Use Cases

- **Package installation**: Download as user, elevate for install, drop for config
- **Service management**: Elevate for systemd/launchd operations
- **Audit**: Receipt shows exactly which operations ran with elevated privileges
- **Dry-run clarity**: Plan shows "this step requires root" before execution

#### Execution Rules

1. Elevate acquires the requested privilege level (typically root via sudo)
2. All downstream nodes execute at the elevated level until a drop node or the subgraph ends
3. Elevate may prompt the user (interactive) or use cached credentials (non-interactive)
4. The receipt records the privilege level of every node
5. Dry-run does not actually elevate — it reports the requirement

#### Node Representation

```yaml
- id: elevate-for-install
  mode: elevate
  privilege: root
  reason: "Package installation requires root"

- id: drop-privileges
  mode: elevate
  privilege: user
```

---

## Relationship to Existing Patterns

### Platform Resolution

The lifecycle phase discovery system (`PlatformResolutionOrder()` in `internal/lorepackage/lifecycle.go`) already performs implicit static choosing — it returns the most-specific platform match. Making choose an explicit graph operation:

1. Makes selection logic visible in the graph (not hidden in the resolver)
2. Supports runtime decisions beyond platform (availability, version, preference)
3. Allows the executor to track and report what was chosen and what was skipped
4. Enables dry-run output to show all alternatives with the selected one highlighted

### Edge Types

The existing `Edge` type supports `relation` (depends_on, orders). Convergence mode and control flow are properties of the **target node**, not the edge. An edge into a gather node means "this is a required input." An edge into a choose node means "this is an alternative."

## Implementation Notes

Node mode is a node-level property that determines execution semantics:

```go
type NodeMode string

const (
    NodeDefault  NodeMode = ""         // standard action node
    NodeGather   NodeMode = "gather"   // wait for all predecessors
    NodeChoose   NodeMode = "choose"   // select one predecessor
    NodeElevate  NodeMode = "elevate"  // privilege transition
)
```

Retry is a node property, not a mode — it modifies execution behavior of any
node regardless of its mode. Rollback is handled by the Action interface's `Undo`
method — each action's `Do` returns `UndoState` that the executor stores on the
recovery stack and passes back to `Undo` during unwind:

```go
type RetryPolicy struct {
    MaxAttempts  int
    Backoff      BackoffStrategy // none, linear, exponential
    InitialDelay time.Duration
    MaxDelay     time.Duration
}

type Node struct {
    // ...existing fields...
    Action   Action       // action to execute (has Do + Undo)
    Retry    *RetryPolicy // nil = no retry
}
```

The executor's topological walk checks the mode of each node:

- **Default/Gather**: Wait for all predecessors, then execute the action
- **Choose**: Evaluate criteria, execute selected predecessor, skip others, then execute
- **Elevate**: Acquire/release privilege level for downstream nodes
