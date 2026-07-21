---
step: 36
former_step: 33
title: "Control plane — the executor's bidirectional command / event surface"
status: in-progress — Slices A (in-process async plane) + B (HTTP/2 listener) landed 2026-07-20; Slice C (narrator migration) pending
proof_run: 2026-07-20 (make test green — 99 packages; gofmt + vet clean) — Slices A + B
parent: ../../phase-8.md
---

# Step 36 — Control plane (formerly 33)

**Status:** `in-progress`. The design is settled and lives in the architecture doc —
[`docs/architecture/2.7-control-plane.md`](../../../../architecture/2.7-control-plane.md) (the async `ControlPlane`
API, the HTTP/2 wire surface, the curl examples). This step doc is the task breakdown; it does not restate the design.

## Slices

1. **Slice A — the in-process async plane (landed 2026-07-20).** `pkg/op/control_plane.go`: `ControlPlane`
   (`Request(cmd) <-chan ControlResponse` — non-blocking, the channel is the future; `Subscribe() (<-chan
   ControlEvent, cancel)` — fan-out, bounded buffer, non-blocking drop, `Seq`-stamped), with the executor-side `poll`
   (non-blocking drain at the control-point) and `emit` (non-blocking fan-out). The `GraphExecutor`'s
   `pauseRequested *atomic.Bool` is replaced by a shared `control *ControlPlane`; `pausePointObserved` becomes
   `controlPoint()` — the `switch` that drains a command and answers on its own response channel: `ControlPause` →
   `ErrPaused` (preserve + resumable), `ControlStop` → `PhaseStopping` + `ErrStopped`. Run gained the `ErrStopped`
   branch: it unwinds (compensating completed work) and lands the deliberate-halt terminal `stopped × healthy ×
   stopped` (a clean stop is not a failure; a failed unwind lands `stopped × compensation_failed`), distinct from the
   `execution_failed` failure terminal. `Pause()` / `Stop()` are thin `control.Request(...)` conveniences plus a
   `Control()` accessor; the plane is shared to children via `newChildExecutor`. `EventPhaseChanged` /
   `EventError` are emitted at the run-level phase transitions and at each node's start / error. Tests:
   `control_plane_test.go` (request/response round-trip, queue-full, subscribe/emit fan-out + `Seq`, drop-slow-sub)
   and `TestGraphStop_UnwindsToStopped_ViaPublicAPI` (stop mid-run → compensate + `stopped × healthy × stopped` +
   the stopped event observed on a subscription). No wire. `make test` green (98 packages); gofmt + vet clean.
2. **Slice B — the HTTP/2 wire listener (landed 2026-07-20).** `pkg/op/controlhttp` — a `Server` that routes by run
   id to a registered plane (`Register(runID, *op.ControlPlane, status func() op.RunStatus) func()`, a stateless
   run-id → plane router). Endpoints: `POST /v1/runs/{runID}/commands` (decode `{command, request_id?, count?}` →
   `plane.Request` → the JSON ack `{status | error, request_id?}`, with a terminal-run guard → `409`),
   `GET /v1/runs/{runID}/events` (SSE — `Subscribe` → stream `event: <kind>\ndata: {seq, …RunStatus, unit?, error?}`
   frames), `GET /v1/runs/{runID}` (the current `RunStatus`). Served over cleartext HTTP/2 via `h2c` (so one
   connection multiplexes the SSE `GET` and command `POST`s as independent streams); HTTP/1.1 works too. The
   architecture doc's curl examples are now executable. Tests: `server_test.go` (status, 404 / 400 / 409 guards,
   unregister, SSE-frame shape) and `integration_test.go` (a gate fixture drives a real run: subscribe → SSE, pause
   over the command endpoint mid-run → `phase=paused` ack + `request_id` echo, and the paused event observed on the
   stream). `golang.org/x/net` promoted to a direct dep for `http2`/`h2c`. The gRPC-equivalent surface and TLS/auth
   are follow-ons. `make test` green (99 packages); gofmt + vet clean.
3. **Slice C — the event-stream / narration integration (reframed 2026-07-21).** *Not* a migration onto the plane.
   The run's steer-and-observe surface is **three orthogonal pieces** — control commands, event streams, narration —
   sharing the [`pkg/op/hooks.go`](../../../../../pkg/op/hooks.go) hook seam; see the architecture doc's
   [Framing](../../../../architecture/2.7-control-plane.md#framing-three-orthogonal-pieces). `Status`
   (`status.Narrator`) and `Result` (`result.Pipeline`) stay on `RuntimeEnvironment` and providers keep emitting via
   `p.RuntimeEnvironment().Status` unchanged. The work: (a) reconcile the hook interface's `OnSubgraph*`-vs-spec-`OnPhase*`
   drift so run-status transitions have a callback; (b) build the event stream on
   [`6-execution-topology.md` §Telemetry](../../../../architecture/6-execution-topology.md#telemetry-asynchronous-event-pipeline)
   (`Event` / `SubscriptionManager`), fed from a `LifecycleHook`; (c) split it off the command surface and retire
   Slice A's inline `emit` calls (`pkg/op/node.go:161`, `pkg/op/graph_executor.go:534`…); (d) leave narration
   orthogonal. The provider-access blocker is dissolved — the stream is fed from hooks, not from a provider reaching
   the executor.

## Touches

`graph_executor.go` (the plane field + control-point `switch` + `Pause`/`Stop`/`newChildExecutor`), a new
`control_plane.go`, the `ErrStopped` sentinel + `PhaseStopped` terminal wiring (the phase machine already has
`PhaseStopping`/`PhaseStopped` from step 21), and — Slice C only — `runtime_environment.go` and the `ui` / `service`
providers.

## Feeds

- Step 31's outstanding item 2 — the public-API pause-mid-combinator resume test needs Slice A's mid-run pause
  injection.
- Step 12's context-cancel fixture row.
