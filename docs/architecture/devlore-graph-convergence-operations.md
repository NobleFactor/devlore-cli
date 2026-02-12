# Graph Operations: Convergence, Control Flow, and System Interaction

This document describes the graph operations that eliminate runtime system queries by encoding all decision-making in the plan. Every system interaction, conditional branch, failure policy, and privilege transition is an explicit graph node — visible in dry-run output and recorded in the receipt.

See also: [devlore-execution-graph.md](devlore-execution-graph.md) — Core graph architecture and state machine.

Tracking issue: https://github.com/NobleFactor/devlore-cli/issues/92

## Design Principle

**The graph is self-contained.** No hidden runtime queries. Every decision the executor makes is rooted in a graph node whose inputs and outputs are traceable. The plan shows what will happen and why. The receipt shows what happened and why.

| Runtime behavior | Graph operation |
|---|---|
| Query system state | **Probe** |
| Conditional execution | **Guard** |
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
  operations: [shell]
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

## System Interaction Operations

### Probe

**Semantics**: Query system state and produce a value that flows through the graph via edges. Probes are the mechanism that feeds guard and choose nodes with inputs, making system queries explicit and traceable.

Currently `system.package.installed("docker")` and `system.service.running("nginx")` are immediate Starlark calls invisible to the graph. As probe nodes, they become explicit steps whose results are visible in both plan (dry-run) and receipt.

```
  ┌─────────────────┐
  │ probe: python    │ → version: "3.11.2"
  │                  │ → installed: true
  └────────┬────────┘
           │ result flows via edge
           ▼
  ┌─────────────────┐
  │ choose: upgrade? │
  └─────────────────┘
```

Probe is the keystone operation — without it, choose and guard still need to reach outside the graph for inputs.

#### Probe Types

| Type | Queries | Example |
|------|---------|---------|
| `package` | Installation status, version | Is docker installed? What version? |
| `service` | Running, enabled, exists | Is nginx running? Is it enabled at boot? |
| `file` | Exists, permissions, checksum | Does ~/.config/git/config exist? |
| `command` | Exit code, stdout | Does `python3 --version` succeed? |
| `platform` | OS, distro, arch, version | What Linux distro is this? |
| `disk` | Available space | Is there ≥2GB free on /? |
| `network` | Reachability | Can we reach the package registry? |

#### Execution Rules

1. Probe nodes execute before any node that depends on their output
2. Probe results are stored on the node and flow through edges as slot values
3. In dry-run mode, probes still execute (they are read-only queries)
4. Probe failures are non-fatal by default — a failed probe produces an empty/error result, not a graph failure
5. The receipt records every probe result for audit and debugging

#### Node Representation

```yaml
- id: check-python
  mode: probe
  probe:
    type: package
    name: python3
  output:
    installed: true
    version: "3.11.2"
```

---

## Control Flow Operations

### Guard

**Semantics**: Binary gate on a single path. Evaluates a condition (typically from a probe result) and either proceeds or skips/fails the downstream subgraph. Different from choose — guard is not selecting among alternatives, it's gating a single path.

```
  ┌──────────────┐
  │ probe: disk  │ → available: 4.2GB
  └──────┬───────┘
         │
         ▼
  ┌──────────────┐
  │ guard: ≥2GB  │ → pass/fail
  └──────┬───────┘
         │ (if pass)
         ▼
  ┌──────────────┐
  │ install      │
  └──────────────┘
```

#### Use Cases

- **Skip if already installed**: probe → guard (not installed?) → install
- **Version gate**: probe → guard (version < 3.0?) → upgrade
- **Dependency check**: probe → guard (no dependents running?) → decommission
- **Disk space**: probe → guard (≥2GB free?) → download large package

#### Execution Rules

1. Guard evaluates a condition against its input (from a probe or a predecessor's output)
2. If the condition passes, downstream nodes execute normally
3. If the condition fails, downstream nodes are marked `skipped` (soft guard) or the graph fails (hard guard)
4. The receipt records the guard condition, input value, and pass/fail result

#### Node Representation

```yaml
- id: check-disk-space
  mode: guard
  condition:
    input: probe-disk.available
    operator: gte
    value: 2147483648  # 2GB in bytes
  on_fail: skip  # or "fail"
```

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
  operations: [package-install]
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

**Semantics**: Paired compensating action. Associates a node with its undo operation. If the node succeeds but a later node in the graph fails, the executor can walk back through rollback associations to restore the previous state.

```
  ┌──────────────┐  rollback  ┌───────────────┐
  │ install pkg  │ ─────────→ │ remove pkg    │
  └──────────────┘            └───────────────┘
```

#### Use Cases

- **Partial manifest failure**: 3 of 5 packages installed, then the 4th fails. Rollback uninstalls the 3 that succeeded.
- **Decommission safety**: If unprovision fails, rollback re-provisions the service.
- **Configuration changes**: If new config breaks verification, rollback restores the previous config.

#### Execution Rules

1. Rollback associations are declared in the plan, not computed at runtime
2. Rollback executes in reverse topological order (last completed node rolls back first)
3. Rollback is optional — nodes without rollback associations are left as-is on failure
4. The receipt records which rollbacks executed and their results
5. Rollback failure does not mask the original error — both are reported

#### Node Representation

```yaml
- id: install-docker
  operations: [package-install]
  rollback: remove-docker

- id: remove-docker
  operations: [package-remove]
  slots:
    packages: [docker-ce, docker-ce-cli, containerd.io]
```

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

The existing `Edge` type supports `relation` (depends_on, orders). Convergence mode and control flow are properties of the **target node**, not the edge. An edge into a gather node means "this is a required input." An edge into a choose node means "this is an alternative." An edge from a probe to a guard means "this is the condition input."

## Implementation Notes

Node mode is a node-level property that determines execution semantics:

```go
type NodeMode string

const (
    NodeDefault  NodeMode = ""         // standard operation node
    NodeGather   NodeMode = "gather"   // wait for all predecessors
    NodeChoose   NodeMode = "choose"   // select one predecessor
    NodeProbe    NodeMode = "probe"    // query system state, produce value
    NodeGuard    NodeMode = "guard"    // gate downstream on condition
    NodeElevate  NodeMode = "elevate"  // privilege transition
)
```

Retry and rollback are node properties, not modes — they modify execution behavior of any node regardless of its mode:

```go
type RetryPolicy struct {
    MaxAttempts  int
    Backoff      BackoffStrategy // none, linear, exponential
    InitialDelay time.Duration
    MaxDelay     time.Duration
}

type Node struct {
    // ...existing fields...
    Mode     NodeMode
    Retry    *RetryPolicy // nil = no retry
    Rollback string       // node ID of compensating action, empty = no rollback
}
```

The executor's topological walk checks the mode of each node:

- **Default/Gather**: Wait for all predecessors, then execute operations
- **Choose**: Evaluate criteria, execute selected predecessor, skip others, then execute
- **Probe**: Execute query, store result, flow to dependents
- **Guard**: Evaluate condition from input, proceed or skip/fail downstream
- **Elevate**: Acquire/release privilege level for downstream nodes
