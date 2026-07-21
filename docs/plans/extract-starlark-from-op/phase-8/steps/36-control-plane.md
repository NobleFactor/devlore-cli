---
step: 36
former_step: 33
title: "Control plane — the executor's bidirectional command / event surface"
status: not-started — design settled 2026-07-20 (see architecture/2.7-control-plane.md); implementation pending
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 36 — Control plane (formerly 33)

**Status:** `not-started`. The design is settled and lives in the architecture doc —
[`docs/architecture/2.7-control-plane.md`](../../../../architecture/2.7-control-plane.md) (the async `ControlPlane`
API, the HTTP/2 wire surface, the curl examples). This step doc is the task breakdown; it does not restate the design.

## Slices

1. **Slice A — the in-process async plane.** Add `ControlPlane` (`Request(cmd) <-chan ControlResponse`,
   `Subscribe() (<-chan ControlEvent, cancel)`) on the `GraphExecutor`; migrate the `pauseRequested *atomic.Bool`
   (`graph_executor.go:65`) onto it; turn the pause-point (`graph_executor.go:929`) into the control-point `switch`.
   Add `ControlStop` → `ErrStopped` → `Run` unwinds → `PhaseStopped` (terminal, not resumable), alongside the existing
   pause path. `Pause()` / `Stop()` become thin `control.Request(...)` conveniences. Emit `EventPhaseChanged` /
   `EventError` at the lifecycle transitions the `HookRegistry` already fires. Shared to children via
   `newChildExecutor`. No wire.
2. **Slice B — the HTTP/2 wire listener.** The REST-commands (`POST …/commands`) + SSE-events (`GET …/events`) facade,
   and the gRPC equivalent, bridging the plane; the architecture doc's curl examples become executable. Its own step
   once Slice A lands.
3. **Slice C — the narrator migration.** Move `Status` (`status.Narrator`) and `Result` (`result.Pipeline`) off
   `RuntimeEnvironment` (`runtime_environment.go:80`/`:86`) onto the plane as event kinds. A real refactor: the
   `ui` / `service` providers emit via `p.RuntimeEnvironment().Status` today, so the emission path re-threads. **Blocked
   on a design decision** — how a provider reaches the plane without reaching the executor (likely an `activation`
   accessor mirroring `a.Transition`); see the architecture doc's open questions.

## Touches

`graph_executor.go` (the plane field + control-point `switch` + `Pause`/`Stop`/`newChildExecutor`), a new
`control_plane.go`, the `ErrStopped` sentinel + `PhaseStopped` terminal wiring (the phase machine already has
`PhaseStopping`/`PhaseStopped` from step 21), and — Slice C only — `runtime_environment.go` and the `ui` / `service`
providers.

## Feeds

- Step 31's outstanding item 2 — the public-API pause-mid-combinator resume test needs Slice A's mid-run pause
  injection.
- Step 12's context-cancel fixture row.
