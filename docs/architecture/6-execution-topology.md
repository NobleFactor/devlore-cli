# Execution Topology

Design topic for a future plan. Not yet approved for implementation.

> **Revised 2026-10-21.** This document keeps the **problem**, the **per-unit policy triplet** and the **topology**. What
> it once designed as mechanism has moved, and the sections below that carried it are now a paragraph each saying where:
> the provider to [`6.1-authorization.md`](6.1-authorization.md) (`authz`; elevation is its process-running case); the
> privileged child, the interface shape, the Windows notes and the PowerShell prior art to
> [`6.2-process-pool.md`](6.2-process-pool.md) (the command runner, a daemon per account); remote execution, SSH
> multiplexing and the telemetry transport to [`2.7-control-plane.md`](2.7-control-plane.md) (the DevLore Broker) and
> [`2.8-eventing-infrastructure.md`](2.8-eventing-infrastructure.md). The policy triplet's `ElevationOffer` is dormant in
> the tree and its fate is 6.1 § Open 12; the *Target design* below, with its `Mode`, is kept as design history: ruled
> 2026-10-21, there is no "whether" — a command declares the account it needs ([`6.2`](6.2-process-pool.md) § Ruled).
> Plan: [`1119-broker-specification.md`](../plans/chore/1119-broker-specification.md).

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

---

## Approach: Privileged Child Process + Action-Declared Elevation

**Moved 2026-10-21.** The persistent elevated child this section described — a generic command executor over a pipe,
one per phase, through which "file operations that need root go through the same channel" — is the **command runner**
of [`6.2-process-pool.md`](6.2-process-pool.md) § 4.1: a daemon standing for an account, holding no shell, a fresh
child per command, many commands at once. Platform divergence is 6.2 § 10 and § 4.3. Action-declared elevation, the
`ElevationAware` interface and the `Mode` tri-state, is superseded by the account a command declares; the dormant
`op.ElevationOffer`'s fate is [`6.1-authorization.md`](6.1-authorization.md) § Open 12.

---

## Interface Shape

**Retired 2026-10-21.** `ElevationProvider{Start, Exec, WriteFile, Stop}` is the runner's API in another form
([`6.2`](6.2-process-pool.md) § 13): `Start` and `Stop` are the runner's lifetime, `Exec` is the request, `WriteFile` is
a command. The `flow.elevate` sketch that followed predated the sealed model; the provider that fulfills the policy is
[`6.1-authorization.md`](6.1-authorization.md), whose `Authorize` takes the activation record and returns the lease as
its receipt. There is no `flow.elevate` action in the tree, and `flow.Provider` has no `Elevate`.

---

## Windows-Specific Considerations

**Moved 2026-10-21** to [`6.2-process-pool.md`](6.2-process-pool.md) § 4.3: a service per account started by the
Service Control Manager, a named pipe with a DACL and the client token as the peer identity, CRT argv encoding for tier
1, `pwsh -File` for tier 2, a Job Object for cancel, an inherited handle for the third channel, and the one unverified
row, the token path for on-demand spawn, which [#528](https://github.com/NobleFactor/devlore-cli/issues/528) settles.
The UAC dialog is never raised from a service.

---

## Prior Art: Persistent PowerShell Session

**Moved 2026-10-21** to [`6.2-process-pool.md`](6.2-process-pool.md) § 8, where the PowerShell host is modeled on
runspaces rather than a persistent `pwsh` process, and § 3, where persistent interpreter state is separated from
persistent privilege. `internal/pwsh`, which demonstrated the shape, was deleted 2026-08-17.

---

## Open Questions

**Dispersed 2026-10-21.** Worker protocol: frames by tag with a request id, [`6.2`](6.2-process-pool.md) § 7. Worker
binary: the runner daemon, the same binary for every account; `devlore` behind a flag or its own is
[`2.7`](2.7-control-plane.md) § Open 2. Timeout and keepalive: 6.2 § 9. Nested elevation and concurrency: 6.2 § 6,
multiplexing by request id. Dry-run: 6.2 § 9, allocating nothing. Platform detection: 6.2 § 10, the platform decides
how the runner for root exists. Credential forwarding for remote execution: the token on the broker's remote leg,
[`2.7`](2.7-control-plane.md) § Authorization at the socket.

---

## SSH Multiplexing for Remote Elevation

**Superseded 2026-10-21; SSH restored 2026-10-24.** On 2026-10-21 the remote leg became the broker's API over TLS with a token, and SSH at most a tunnel. On 2026-10-24 the owner ruled the other way: the broker remotes through SSH, shipping a bundle that ensures the executor and the subgraph are present on the far machine and run under the account's credentials, with no broker installed there ([`2.7`](2.7-control-plane.md) § The remote leg). Whether one SSH connection carries many sessions, by Go's SSH package's channels or OpenSSH's multiplexing, is [`2.7`](2.7-control-plane.md) § Open 17 and 18. The 2026-10-21 text follows for the record: The property this
section wanted — one authenticated connection carrying many isolated sessions — is HTTP/2's multiplexing of streams on
one connection, and the per-session fresh process is the runner's child per command.

---

## Remote Execution: The Executor Abstraction

**Superseded 2026-10-21.** `Executor{Upload, Execute(cmd string, elevated bool), Download}` and the graph travelling
with a runner binary over SSH are replaced by posting the graph or the command to the remote machine's broker
([`2.7`](2.7-control-plane.md) § The remote leg): the string loses to [`6.3`](6.3-command-execution.md)'s argv rule,
the boolean to the account. What survives unchanged is the insight that **the graph is the artifact that travels** and
must be idempotent, and the distributed flow of [`1-system-model.md`](1-system-model.md) § 6.2, where the coordinator
collects each machine's trace. Long-running workloads re-attach by `seq` (2.7 § The two streams).

---

## Telemetry: Asynchronous Event Pipeline

**Moved 2026-10-21.** The event-stream spec this section seeded — `Event`, the non-blocking `SubscriptionManager`, the
three categories, ndjson, reconnection and history, the elevation-and-eventing rule to initialize sinks before a
privilege transition — is [`2.8-eventing-infrastructure.md`](2.8-eventing-infrastructure.md)'s bus, fed from the hook
seam, and its wire is [`2.7-control-plane.md`](2.7-control-plane.md) § The two streams: operational events on
`/events` for the logger and monitors, narration and output on `/console` for the caller, each `seq`-numbered with SSE
`id:` as the resume point. The vision summary stands with one substitution: the broker's TLS listener where it said
"SSH tunnel".
