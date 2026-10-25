# Status: Execution Topology

**Architecture document:** [6-execution-topology.md](6-execution-topology.md)

**Revised 2026-10-24** (plan [`1159-stream-rulings.md`](../plans/chore/1159-stream-rulings.md)): the remote leg is SSH again, with a bundle in place of the upload of a runner binary.
**State:** revised 2026-10-21. The document keeps the problem, the per-unit policy triplet and the topology; its
mechanism sections are a paragraph each naming where the content now lives. Plan
[`1119-broker-specification.md`](../plans/chore/1119-broker-specification.md).

## Completion

| Component | Status | Where it lives now |
|-----------|--------|--------------------|
| Per-unit `ElevationOffer` + `ExecutableUnit` triplet | In the tree, dormant: `WithElevationOffer` called by nothing, `ElevationOffer()` read by nothing, not serialized | here; its fate is [6.1](6.1-authorization.md) § Open 12 |
| The "whether" (`Mode` / `Forbidden`) | Superseded 2026-10-21: a command declares its account | [6.2](6.2-process-pool.md) § Ruled |
| Elevate (privilege transition) | No `flow.elevate` in the tree; `flow.Provider` has no `Elevate` | — |
| The provider | Designed, not implemented; the `elevator` stub is dead (#679) | [6.1](6.1-authorization.md), the `authz` provider (#1116) |
| The privileged child / the worker | Designed, not implemented | [6.2](6.2-process-pool.md) § 4.1, the runner daemon |
| Windows | Designed, one row unverified (#528) | [6.2](6.2-process-pool.md) § 4.3 |
| Remote execution | Superseded by the broker's remote leg; designed, not implemented | [2.7](2.7-control-plane.md) |
| Telemetry | Designed; the bus settled 2026-07-23, the fork 2026-10-21; not implemented | [2.8](2.8-eventing-infrastructure.md), [2.7](2.7-control-plane.md) § The two streams |

## Document Discrepancies

None after the 2026-10-21 revision. Before it, § Interface Shape cited `internal/execution/flow/elevate.go`, which
does not exist, and § Approach designed a worker that 6.2 had already replaced.

## Outstanding Work

1. **The policy triplet's third member.** Whether `op.ElevationOffer` becomes the unit's offer reference for
   [6.1](6.1-authorization.md) or is retired with the elevator — 6.1 § Open 12.
2. **An implementation plan** — the four-iteration schedule the plan names, filed after it merges; nothing here is
   implemented beyond the dormant triplet member.
