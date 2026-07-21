# Status: Execution Topology

**Architecture document:** [6-execution-topology.md](6-execution-topology.md)

## Completion

| Component | Status | Completed | PR |
|-----------|--------|-----------|-----|
| Per-unit `ElevationOffer` + `ExecutableUnit` triplet | First cut in tree (uncommitted) | — | — |
| Elevate (privilege transition) | Stub only | 2026-02-16 | [#139](https://github.com/NobleFactor/devlore-cli/pull/139) |
| Elevation provider interface | Not implemented | — | — |
| Remote execution | Not implemented | — | — |
| Telemetry | Not implemented | — | — |

A first-cut `op.ElevationOffer` and the `ExecutableUnit` policy triplet (elevation / retry / error) are wired in
`pkg/op`; the architecture doc marks this the "shipped first cut" and keeps the fuller `pkg/elevation` / `Mode` /
`Elevator` design as the deferred "Target design." The provider, remote-execution, and telemetry mechanisms remain
"not yet approved for implementation."

## Document Discrepancies

None.

## Outstanding Work

1. **Elevation** — `op.ElevationOffer` first cut + the per-unit triplet are wired; deferred: the `Mode`/`Forbidden` disposition, op-free `pkg/elevation` packaging, the `Elevator` / `ElevationProvider` mechanism, and `flow.elevate`'s full privilege integration (still a passthrough stub)
2. **Remote execution** — SSH/RPC-based subgraph dispatch to remote nodes
3. **Telemetry** — the event-stream piece of the run's observability surface (control commands / event streams /
   narration; see [2.7-control-plane.md Framing](2.7-control-plane.md#framing-three-orthogonal-pieces)). The `Event` /
   `SubscriptionManager` spec is fed from the `pkg/op/hooks.go` seam; the control plane's first cut reinvented it and
   must be reconciled onto this spec.
4. **No implementation plan exists** — architecture doc only
