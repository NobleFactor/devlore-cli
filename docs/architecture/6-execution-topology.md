# Execution Topology

Design topic for a future plan. Not yet approved for implementation.

> **Elevation deep-dive.** The *provider* that fulfills the elevation policy below — both strategies, the
> graph/config/runtime split, the token-provider mechanism, and the failure routing — is detailed in
> [`6.1-privilege-elevation.md`](6.1-privilege-elevation.md). This document owns the **policy** (`elevation.Policy`,
> the `Elevator` contract, `ProcessSpawn`); 6.1 owns the **provider** (`elevator`, a working name).

---

## Problem

Actions like `pkg.install` (apt, dnf, pacman) and `service.start` (systemctl)
require root privileges. Others (`template.render`, `file.link`, `brew`) do not.
Running everything as root is wrong. Running nothing as root breaks
package/service management on Linux and Windows.

Elevation must be declared at two levels:

| Level | Where | Example |
|---|---|---|
| **Type** | Action implementation | `pkg.install` always needs elevation on Linux; `service.start` always does |
| **Instance** | Unit `ElevationOffer` | A specific `shell.exec` node needs elevation; another doesn't |

The executor checks both: instance-level overrides type-level.

---

## The per-unit policy triplet

Elevation is one of **three cross-cutting policies an executable unit may carry**. The other two already exist on
[`op.ExecutableUnit`](../../pkg/op/executable_unit.go); elevation joins them as the third, modeled the same way — an
optional, nil-able, serialized per-unit setting:

| Policy | Type | Question it answers | Status |
|---|---|---|---|
| **`ElevationOffer`** | `*op.ElevationOffer` | *How / over what scope / for how long should this unit elevate?* | **shipped (first cut)** |
| **`ErrorAction`** | `*Subgraph` | *When this unit fails, what handler runs?* (→ `flow.Degraded` / `flow.Failed`; see the [compensation-failure contract](../plans/extract-starlark-from-op/phase-8/compensation-failure-contract.md)) | exists |
| **`RetryPolicy`** | `*RetryPolicy` | *On failure, how many attempts and with what backoff?* | exists |

All three are set the same way — on the unit's `ExecutableUnitSpec` via a `With*` method (`WithErrorAction`,
`WithRetryPolicy`, and the new `WithElevationOffer`), read back through the matching accessor
(`ErrorAction()` / `RetryPolicy()` / `ElevationOffer()`), and serialized onto the unit. `nil` means "no policy —
defer to defaults." Because they live on `ExecutableUnit`, the triplet is uniform across **both** unit kinds: a
`Node` or a `Subgraph`. A whole phase (subgraph) can carry one elevation / error / retry policy that its child
nodes inherit unless they set their own.

### `ElevationOffer` — the shipped first cut

The shipped type is **`op.ElevationOffer`** (in `pkg/op`; the op-free `pkg/elevation` packaging is part of the
*Target design* below). It carries three dimensions of an elevation request, plus a chainable fallback:

- **`Strategy`** (`op.ElevationStrategy`, a string enum) — the *how*: `HostEscalation` (sudo / runas),
  `InteractiveChallenge`, `IdentityAssumption` (AWS STS, OAuth / JWT), `MandatedApproval`.
- **`Scope`** (`op.ElevationScope{Domain, RequiredPrivileges}`) — the *what*: the security domain and the explicit
  privileges required.
- **`Lifespan`** (`op.ElevationLifespan{Ephemeral, CacheDuration time.Duration}`) — the *how-long*.
- **`Fallback *op.ElevationOffer`** — a chained alternative attempted when this offer cannot be satisfied.

It is set through the unit's spec like the other two triplet members:

```go
spec.
    WithRetryPolicy(&op.RetryPolicy{MaxAttempts: 3}).
    WithElevationOffer(&op.ElevationOffer{
        Strategy: op.HostEscalation,
        Scope:    op.ElevationScope{Domain: "os", RequiredPrivileges: []string{"root"}},
        Lifespan: op.ElevationLifespan{CacheDuration: 15 * time.Minute},
    })
```

What the first cut does **not** carry is a `Mode`/disposition (`Auto` / `Required` / **`Forbidden`**), so it cannot
yet express *"this unit must NOT elevate"* — the case `brew` and the language managers (`npm` / `pip` / `gem` /
`cargo` / `go`) need, and the one the boolean `elevate` annotation also couldn't. Adding `Mode` (and the rest of the
*Target design* below) is the deferred work; until then the action's `ElevationAware.NeedsElevation()` default and the
`elevate` annotation remain the *whether* mechanism.

## Target design (deferred) — a declarative elevation policy

*Not yet built; this is where `ElevationOffer` goes when elevation is taken seriously.* The shipped first cut is
`op.ElevationOffer` (above); the target promotes it to an op-free `pkg/elevation` package as `elevation.Policy` and
adds what the prototype lacks. **The delta from the shipped code is exactly:** package `pkg/op` → `pkg/elevation`
and type `ElevationOffer` → `Policy`; add a `Mode`/`Forbidden` disposition (the *whether*); `Scope.RequiredPrivileges`
→ `Privileges`; `Lifespan.CacheDuration` (`time.Duration`) → `CacheTTL` (duration string); strategy `HostEscalation` →
`ProcessSpawn`; add `,omitempty` tags; and add an `Elevator` evaluator. In the target, the `Mode`
field answers **whether** a unit elevates; the rest of the `Policy` pins **how**, **over what scope**, and **for how
long** — because Elevation here is broader than OS `sudo`, spanning interactive challenges, cloud identity assumption
(AWS STS, OAuth / JWT), and gated approval.

Three finesses keep it consistent with the rest of the framework:

1. **String enums, not `iota` ints** — matches [`BackoffStrategy`](../../pkg/op/retry_policy.go) and serializes to
   self-describing JSON/YAML (an `iota` int writes as a bare `0` / `1`). The empty string is the zero value and means
   "defer to default" — so the zero value is `Auto`, *not* the first strategy.
2. **Nil-able pointer + duration strings** — the policy is a `*elevation.Policy`, like `*op.RetryPolicy` and
   `ErrorAction`'s `*op.Subgraph` (nil ⇒ no policy); cache TTLs are Go duration strings (`"15m"`) as
   `RetryPolicy.InitialDelay` is, not raw `time.Duration`.
3. **Its own op-free package** — like `pkg/result` / `pkg/status` / `pkg/platform`, `pkg/elevation` is pure data
   plus one behavior interface and imports nothing from `op`; `op.ExecutableUnit` imports *it* for the policy field.

### Types — disposition, strategy, scope, lifespan

```go
package elevation

import "context"

// Mode is the WHETHER — the per-unit override over the action's ElevationAware default.
type Mode string

const (
    Auto      Mode = ""          // defer to the action's NeedsElevation() default
    Required  Mode = "required"  // force elevation
    Forbidden Mode = "forbidden" // never elevate (brew, npm/pip/gem/cargo/go)
)

// Strategy is the HOW — the mechanism used to acquire elevation.
type Strategy string

const (
    ProcessSpawn         Strategy = "process_spawn"         // OS escalation: sudo / runas
    InteractiveChallenge Strategy = "interactive_challenge" // prompt for password / OTP
    IdentityAssumption   Strategy = "identity_assumption"   // assume a role: AWS STS, OAuth / JWT
    MandatedApproval     Strategy = "mandated_approval"     // await a third-party approver
)

// Scope is the WHAT — the security domain and the explicit privileges required.
type Scope struct {
    Domain     string   `json:"domain"               yaml:"domain"`               // "os", "aws-iam", "google-oauth"
    Privileges []string `json:"privileges,omitempty" yaml:"privileges,omitempty"` // ["root"], ["repo:write"]
}

// Lifespan is the HOW-LONG — caching semantics for the acquired privilege.
type Lifespan struct {
    Ephemeral bool   `json:"ephemeral,omitempty" yaml:"ephemeral,omitempty"` // drop immediately after the action
    CacheTTL  string `json:"cache_ttl,omitempty" yaml:"cache_ttl,omitempty"` // Go duration string, e.g. "15m"
}

// Policy is the per-unit elevation policy — the third member of the unit policy triplet. The zero value
// (and a nil *Policy) is Auto with no strategy: "defer to the action default."
type Policy struct {
    Mode     Mode     `json:"mode,omitempty"     yaml:"mode,omitempty"`
    Strategy Strategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
    Scope    Scope    `json:"scope,omitempty"    yaml:"scope,omitempty"`
    Lifespan Lifespan `json:"lifespan,omitempty" yaml:"lifespan,omitempty"`
    Fallback *Policy  `json:"fallback,omitempty" yaml:"fallback,omitempty"` // chained alternative if this fails
}
```

A `Fallback` chain lets a unit say "assume the IAM role; if it is unavailable, fall back to an interactive
challenge."

### Behavior — the evaluator

`Policy` is data; an `Elevator` acts on it, bracketing the unit's dispatch. It composes with the
`ElevationProvider` mechanism (below): `ProcessSpawn` resolves to an `ElevationProvider`, while
`IdentityAssumption` / `MandatedApproval` resolve to their own providers (an STS client, an approval gateway).

```go
// Elevator acquires the elevation a Policy requires, runs the payload, then releases per the Lifespan.
type Elevator interface {
    Elevate(ctx context.Context, policy *Policy, payload func() error) error
}
```

### On the unit, beside the other two

The triplet is set through the unit's `ExecutableUnitSpec`, each member the same shape — a nil-able pointer:

```go
spec.
    WithErrorAction(abortHandler).                    // *op.Subgraph      (exists)
    WithRetryPolicy(&op.RetryPolicy{MaxAttempts: 3}). // *op.RetryPolicy   (exists)
    WithElevationOffer(&elevation.Policy{            // *elevation.Policy (new)
        Mode:     elevation.Required,
        Strategy: elevation.ProcessSpawn,
        Scope:    elevation.Scope{Domain: "os", Privileges: []string{"root"}},
        Lifespan: elevation.Lifespan{CacheTTL: "15m"}, // cache the sudo session for 15 minutes
    })
```

### Why it fits

- **Flat and serializable** — strings, bools, and slices only, so it round-trips to JSON/YAML config and onto the
  unit's receipt with no custom marshaling (the property the purl identities rely on too).
- **Zero-value sane** — an empty `elevation.Policy{}` (and a nil `*elevation.Policy`) is `Auto` with no strategy:
  "defer to the action default." Nothing to remember to clear.
- **Open beyond `sudo`** — `IdentityAssumption` and `MandatedApproval` are first-class strategies, so the same
  triplet slot models cloud-role assumption and human-gated approval, matching Elevation's broader-than-security
  scope.

---

## Approach: Privileged Child Process + Action-Declared Elevation

Combine a persistent elevated child process (mechanism) with action/node
declarations (policy).

### Privileged Child Process

```
┌──────────────────────────┐
│  devlore (unprivileged)  │
│                          │
│  executor ──pipe──► ┌────────────────────────┐
│                     │  worker (elevated)      │
│                     │  receives: cmd, args    │
│                     │  returns: stdout, rc    │
│                     └────────────────────────┘
│                          │
│  flow.elevate starts it  │
│  flow.elevate undo stops │
└──────────────────────────┘
```

The worker is a generic command executor. Actions that need elevation don't exec
directly — they send `(command, args, env)` to the worker and get back
`(stdout, stderr, exit_code)`. File operations that need root (writing to `/etc`)
go through the same channel.

One elevated child, kept alive for the phase lifetime. This solves the core
problem on all three platforms:

- **Linux**: sudo credential caching is unreliable (configurable timeout, might
  require tty). A persistent child avoids repeated prompts.
- **macOS**: Same sudo issues, plus Keychain complications. Brew explicitly
  rejects root — elevation must be selective.
- **Windows**: UAC is per-process. You cannot "cache" it. An elevated process
  is the only mechanism.

### Platform Divergence

| Platform | Spawn mechanism | IPC | Credential UX |
|---|---|---|---|
| Linux/macOS | `sudo ./devlore-worker` | stdin/stdout pipe | One sudo prompt, child stays alive |
| Windows | `runas` / ShellExecute with `runas` verb | Named pipe | One UAC dialog, child stays alive |

### Action-Declared Elevation

Two levels of declaration:

**Type-level** — optional interface on Action:

```go
// ElevationAware — actions implement this to declare whether they need
// elevation by default. The executor checks this when no instance-level
// override is present.
type ElevationAware interface {
    NeedsElevation() bool
}
```

**Instance-level** — the unit's `ElevationOffer` overrides the type-level default
(see [The per-unit policy triplet](#the-per-unit-policy-triplet) above). It replaces
the earlier `node.Annotations["elevate"]` string hack with a typed tri-state:

```go
spec.WithElevationOffer(&elevation.Policy{Mode: elevation.Required})  // force on
spec.WithElevationOffer(&elevation.Policy{Mode: elevation.Forbidden}) // force off (e.g. brew)
```

The executor's decision:

1. Check `unit.ElevationOffer().Mode` — `Required` → elevate; `Forbidden` → never; `Auto`/nil → step 2
2. Check if the action implements `ElevationAware` (its type-level default)
3. If elevation is needed, route through `ElevationProvider`

This covers the `shell.exec` case: the action type doesn't declare elevation by
default, but a specific node opts in via `elevation.Required` — and `elevation.Forbidden`
keeps root-hostile units (brew, language managers) unprivileged even inside an
elevated phase.

---

## Interface Shape

```go
// ElevationProvider manages a privileged execution context.
// Platform-specific implementations handle the spawn/IPC mechanics.
type ElevationProvider interface {
    // Start spawns the privileged child process.
    Start(ctx context.Context) error

    // Exec runs a command in the elevated context.
    Exec(name string, args ...string) (stdout, stderr []byte, err error)

    // WriteFile writes content to a file with elevated privileges.
    WriteFile(path string, content []byte, mode os.FileMode) error

    // Stop terminates the privileged child process.
    Stop() error
}
```

### flow.elevate Integration

`flow.elevate` exists as a registered flow action (`internal/execution/flow/elevate.go`)
alongside `flow.choose`, `flow.gather`, `flow.complete`, `flow.degraded`,
`flow.fatal`, and `flow.wait_until`. It is currently a passthrough stub that
makes privilege boundaries visible in the graph. The full privilege provider
wiring is the subject of this design.

The action signatures use `Complement` (the current undo state type alias):

```go
func (a *Elevate) Do(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
    provider := elevationProviderFromContext(ctx)
    if provider == nil {
        return nil, nil, fmt.Errorf("elevation required but no provider configured")
    }
    if err := provider.Start(ctx); err != nil {
        return nil, nil, fmt.Errorf("failed to acquire elevation: %w", err)
    }
    // Provider is the complement — Undo calls Stop
    return nil, provider, nil
}

func (a *Elevate) Undo(ctx *op.Context, complement op.Complement) error {
    if provider, ok := complement.(ElevationProvider); ok {
        return provider.Stop()
    }
    return nil
}
```

The `flow.elevate` node in the graph makes privilege acquisition visible in the
plan and receipt. Phase boundary = child lifetime. The receipt records when
elevation was acquired and released.

### Context Integration

`op.Context` currently carries: `context.Context` (embedded), `Catalog`,
`Data map[string]any`, `DryRun`, `Graph`, `NodeID`, `Platform`, and `Writer`.
The elevation provider would be accessed via `ctx.Data` or a dedicated field.
The `Platform` field (added for host abstraction) already provides OS detection
needed for platform-specific elevation dispatch.

---

## Windows-Specific Considerations

- **UAC prompt**: Desktop-modal dialog. Cannot run headless without
  pre-configuration (auto-elevation policies, scheduled tasks, or admin accounts
  without UAC).
- **Service control**: Windows services use `SC_HANDLE` / service control
  manager — a different elevation path than command-line tools.
- **MSI installers**: `msiexec` may need its own elevation path.
- **PowerShell remoting**: `Enter-PSSession` is an option for server/headless
  scenarios.
- **Named pipes**: Native IPC mechanism on Windows — better fit than
  stdin/stdout pipes for the elevated child.

---

## Prior Art: Persistent PowerShell Session

The `internal/pwsh` package implements the persistent child process + pipe IPC
pattern for PowerShell on Windows. This is the same architectural shape proposed
for elevation:

- A single `pwsh -NoProfile -NonInteractive -Command -` process is spawned once
- Commands are streamed to stdin; output is read from stdout until a marker
- Variables, module imports, and session state persist across calls
- The session is closed explicitly (or on process exit)

This exists because PowerShell modules (Az, ActiveDirectory, PackageManagement)
establish authenticated sessions on import, and COM/.NET objects (WMI, Registry,
IIS) are expensive to instantiate. Spawning `pwsh -Command` per operation loses
all state.

The elevation worker generalizes this pattern: instead of PowerShell-specific
stdin/marker protocol, the worker uses a structured IPC protocol (ndjson) for
arbitrary `(command, args, env) → (stdout, stderr, exit_code)` exchanges.

---

## Open Questions

1. **Worker protocol**: What serialization for the IPC channel? JSON lines?
   Protobuf? Gob? JSON lines is simplest and debuggable.
2. **Worker binary**: Same binary with a `--worker` flag, or a separate
   `devlore-worker` binary? Same binary is simpler to distribute.
3. **Timeout/keepalive**: Should the elevated child auto-terminate after idle
   timeout? Or only on explicit `Stop()`?
4. **Nested elevation**: Can a gather body phase contain an elevate node? If so,
   the child process must handle concurrent requests (one per iteration).
5. **Dry-run**: In dry-run mode, `flow.elevate` should report "elevation
   required here" without actually prompting for credentials.
6. **Platform detection**: How does the executor know which `ElevationProvider`
   to use? Compile-time build tags, runtime detection, or configuration?
7. **Credential forwarding**: For remote execution scenarios, how do credentials
   propagate? SSH agent forwarding? Kerberos?

---

## SSH Multiplexing for Remote Elevation

SSH provides a natural transport for the elevated child model on remote hosts.
A single TCP connection (the "tunnel") hosts multiple independent channels
(the "sessions"). This is **multiplexing**.

### One Tunnel, Many Sessions

1. **The Tunnel (TCP/22):** Established once. Handles key exchange,
   authentication, and encryption.
2. **The Sessions (Channels):** Inside that one tunnel, `client.NewSession()`
   can be called many times. Each session is logically isolated.

```go
// 1. Create the Tunnel (The Client)
client, _ := ssh.Dial("tcp", "server:22", config)

// 2. Elevated task
s1, _ := client.NewSession()
s1.Run("sudo apt update")
s1.Close()

// 3. Non-elevated task — fresh start, no sudo leak
s2, _ := client.NewSession()
s2.Run("rm -rf ~/old_configs")
s2.Close()
```

### Why This Matters

- **Concurrency:** Multiple sessions over one connection — tail a log, run
  `yum install`, and monitor CPU simultaneously.
- **Isolation:** If one session crashes, others remain unaffected.
- **Elevation isolation:** Each session starts a fresh process on the remote
  side. Permissions do not leak between sessions.

### Connection Limits

The remote `sshd` typically defaults `MaxSessions` to **10**. If the tool needs
to exceed this (e.g., a gather with 50 concurrent iterations), options are:

1. Increase `MaxSessions` on the target.
2. Manage a pool of clients (multiple tunnels) in the Go tool.

---

## Remote Execution: The Executor Abstraction

The elevation model extends naturally to remote execution. The core insight:
**the graph is the artifact that travels** — coordinator to target and back.

### The Provider Interface

```go
// Executor abstracts local vs remote command execution.
// The orchestrator doesn't care whether it's talking to a local shell or
// an SSH tunnel.
type Executor interface {
    Upload(src string, dest string) error
    Execute(cmd string, elevated bool) ([]byte, error)
    Download(src string, dest string) error
}
```

- **Local implementation:** Uses `os/exec` and `io.Copy`.
- **SSH implementation:** Uses `ssh.Client` and `sftp` for file transfer.

### Graph Lifecycle Over SSH

1. **Transfer:** Coordinator SCP/SFTPs the graph (JSON or YAML) and the runner
   binary to the target.
2. **Trigger:** Coordinator calls `session.Start("./runner --graph=task.json")`.
3. **Hydration:** On the target, the runner loads the graph, reconciles against
   local system state (using the `RecoveryStack` with drift detection and
   reconcile functions), and begins execution.
4. **Persistence:** The runner saves the result graph (receipt) to a local file.
5. **Retrieval:** Coordinator detects process exit, downloads the result graph.

### Local-to-Remote Transition

Start by using SSH even for localhost. This forces handling authentication,
latency, and serialization immediately. If it works over `ssh localhost`, it
works over `ssh 10.0.0.50` with zero code changes.

### Graph Idempotency

Because the graph is uploaded and reused for reconciliation, nodes must be
**idempotent**. Example: an `Install-Nginx` node checks whether Nginx exists
before attempting installation. This is already the design — actions check
target state before acting.

### Long-Running Workloads

For "chunky" workloads (minutes to hours):

- **Heartbeats:** The runner emits periodic "alive" signals to stderr that the
  coordinator monitors.
- **Detached pattern:** For very long workloads, start via `nohup` or a systemd
  transient unit. Disconnect and re-attach later to check exit status. This
  maps to the Recovery Stack restart capability (see
  [devlore-recovery-serialization](5.2-recovery-serialization.md) — planned).

---

## Telemetry: Asynchronous Event Pipeline

Chunky, distributed workloads cannot use synchronous request-response. The
coordinator needs an **asynchronous event bus** that bridges the remote SSH
runner and UI/API consumers.

### Event Categories

1. **Orchestration events:** "Graph Started," "Node 5 Scheduled," "Tunnel
   Connection Lost."
2. **Execution events:** "Installing Package X," "Step 2/10 Complete,"
   "Retrying Connection."
3. **System telemetry:** "CPU Spike on Node B," "Disk Full during Upgrade."

### Structured Stream over SSH

The runner emits **newline-delimited JSON (ndjson)** to stdout. The coordinator
demuxes the stream into Go types.

**Remote side (runner):**

```go
type Event struct {
    Timestamp time.Time `json:"ts"`
    NodeID    string    `json:"node_id"`
    Status    string    `json:"status"`
    Message   string    `json:"msg"`
}

json.NewEncoder(os.Stdout).Encode(Event{
    Timestamp: time.Now(),
    NodeID:    "provision-db",
    Status:    "IN_PROGRESS",
    Message:   "Allocating tablespace...",
})
```

**Local side (coordinator):** Reads from `session.StdoutPipe()`, decodes JSON,
broadcasts to a pub/sub system (NATS, Redis, or internal Go channel bus).

### Observer Pattern

```go
type SubscriptionManager struct {
    mu          sync.RWMutex
    subscribers map[string]chan Event
}

func (s *SubscriptionManager) Publish(ev Event) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, ch := range s.subscribers {
        // Non-blocking send — slow client doesn't hang the orchestrator
        select {
        case ch <- ev:
        default:
        }
    }
}
```

### Reconnection and History

SSH tunnel drops lose the stream. Mitigations:

- **Log buffer:** Runner writes events to a local file (`/tmp/execution.log`)
  while streaming to stdout.
- **Re-sync:** On reconnection, coordinator asks "give me everything since
  timestamp X."
- **WebSocket bridge:** Go backend bridges internal channels to WebSockets for
  real-time UI — the graph "lights up" node-by-node.

### Elevation and Eventing

Subtle trap: if the runner is executing an elevated task (`sudo`), it may lose
access to telemetry sockets or log files. The event logger must initialize its
sinks **before** privilege transitions, or the log directory
(`/var/log/devlore/`) must have correct permissions for non-elevated tasks to
still write telemetry.

### Vision Summary

1. **Coordinator** sends the Graph.
2. **SSH Tunnel** stays open for the live stream of JSON events.
3. **Go Channels** broadcast events to connected UI clients.
4. **Result Graph** provides the final post-mortem state.
