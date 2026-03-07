# Execution Topology

Design topic for a future plan. Not yet approved for implementation.

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
| **Instance** | Node annotation | A specific `shell.exec` node needs elevation; another doesn't |

The executor checks both: instance-level overrides type-level.

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

**Instance-level** — node annotation overrides the type-level default.
`Node.Annotations` (`map[string]string`) is already implemented and serialized
to JSON/YAML receipts:

```go
node.Annotations["elevate"] = "true"   // force elevation for this node
node.Annotations["elevate"] = "false"  // suppress elevation for this node
```

The executor's decision:

1. Check `node.Annotations["elevate"]` for instance-level override
2. If no override, check if action implements `ElevationAware`
3. If elevation needed, route through `ElevationProvider`

This covers the `shell.exec` case: the action type doesn't declare elevation
by default, but a specific node can opt in via annotation.

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
  [devlore-recovery-serialization](devlore-recovery-serialization.md) — planned).

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
