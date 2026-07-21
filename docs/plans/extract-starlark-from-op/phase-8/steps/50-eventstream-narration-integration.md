---
step: 50
title: "Event-stream / narration / hook integration — the run's observability surface"
status: not-started — chartered 2026-07-21 (split from step 36 Slice C)
parent: ../../phase-8.md
---

# Step 50 — Event-stream / narration / hook integration (split from step 36 Slice C)

**Status:** `not-started` — chartered 2026-07-21. An **integration + refactoring** exercise, not a green field: both the
event-stream spec and the hook emission seam pre-date the control plane, and step 36's first cut grew past both. The
design is on record; this step doc is the task breakdown.

## Framing — three orthogonal pieces

A run's outward-facing surface is three **orthogonal** subsystems (none downstream of another), sharing one seam:

1. **Control commands** — inbound; `pause` / `stop` / `step` at the control-point. **Done** (step 36 — the control
   plane's command surface).
2. **Event streams** — outbound, structured, machine-facing. The async event pipeline
   ([`6-execution-topology.md` §Telemetry](../../../../architecture/6-execution-topology.md#telemetry-asynchronous-event-pipeline)):
   `Event` + `SubscriptionManager` (non-blocking / drop-on-slow-client `Publish`), the three event categories,
   ndjson-over-SSH, and the elevation-and-eventing rule (initialize the event sinks **before** privilege drop).
3. **Narration** — outbound, human. `status.Narrator`'s categorized terminal render, one per process, shared with the
   hosting application; **stays orthogonal** — providers keep emitting via `p.RuntimeEnvironment().Status` unchanged.

The shared seam is the lifecycle hooks in [`pkg/op/hooks.go`](../../../../../pkg/op/hooks.go) (`LifecycleHook` /
`HookRegistry`), from [`orchestration-primitives.md` §Step 7](../../../orchestration-primitives.md). Full framing:
[architecture 2.7 §Framing](../../../../architecture/2.7-control-plane.md#framing-three-orthogonal-pieces). Terminology:
`Observation` is a resource fact (`op.ObservationBase`, an observe-action result), **not** the event channel —
observations ride events as results.

## Work items

1. **Reconcile the hook interface with its spec.** `pkg/op/hooks.go` ships `OnSubgraphStart` / `OnSubgraphComplete`
   where the spec ([Step 7](../../../orchestration-primitives.md)) had `OnPhaseStart` / `OnPhaseComplete`, so run-status
   transitions have no callback — the reason step 36 emits them inline. Add the run-status-transition callback the event
   stream needs (the current phase model is the `RunStatus` triplet from step 41, not the older `Phase`).
2. **Build the event stream on the §Telemetry spec.** `Event` + `SubscriptionManager` (or the `op` equivalent), fed
   from a `LifecycleHook` that translates each boundary + transition into a structured, identity-tagged event.
3. **Split the event stream off the command surface.** Move `Subscribe` / `emit` / `ControlEvent` / `Seq` off
   `ControlPlane`; retire step 36's bespoke inline `emit` calls beside each `Fire*` (`pkg/op/node.go:161`,
   `pkg/op/graph_executor.go:534`…) in favor of the hook-fed stream; move the SSE endpoint off `controlhttp`'s command
   server onto the event-stream surface. `ControlPlane` keeps `Request` / response — commands only.
4. **Execution identity + subscriber filtering.** Every graph execution carries an identity; events are tagged with it;
   a consumer subscribes and filters by execution (some / none / all).
5. **Leave narration orthogonal.** No migration onto the plane; `Status` / `Result` stay on `RuntimeEnvironment`;
   emission sites unchanged.

## Touches

`pkg/op/hooks.go` (the interface reconciliation), a new event-stream / telemetry surface fed from a `LifecycleHook`,
`pkg/op/control_plane.go` + `pkg/op/graph_executor.go` + `pkg/op/node.go` (retire the inline `emit`; `ControlPlane` →
commands only), and `pkg/op/controlhttp` (SSE moves off the command server). Narration (`status.Narrator`,
`result.Pipeline`) is untouched.

## Depends on / relates to

- **Step 36** — the control plane's command surface (done); this step unwinds its fused first-cut event stream.
- **Step 41** — the `RunStatus` machine whose transitions the new hook callback must carry.
- **Step 38 (elevation policy)** — the §Telemetry "initialize the event sinks before privilege drop" rule interlocks
  with the elevation model.
