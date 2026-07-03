---
step: 36
former_step: 33
title: "Control plane — the executor's bidirectional command / event surface"
status: not-started — design recorded 2026-06-21 in the step-31 doc's Control plane section
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 36 — Control plane (formerly 33)

**Status:** `not-started`. Design recorded 2026-06-21. The full design lives in the
[Control plane section](31-subgraph-executor-ownership.md#control-plane--the-executors-bidirectional-command--event-surface)
of the step-31 doc; this doc is its step-numbered home and summary.

## Summary

One surface, two directions a listener bridges a connection to — subscribe to events, issue commands.

1. **Commands in:** replace the `*atomic.Bool` pause flag (`graph_executor.go`) with a shared `*ExecutionControl`
   carrying a `ControlCommand` (`ControlNone` / `ControlPause` / `ControlStop` / `ControlStep` / …), polled at each
   control-point via a `switch`. Add **`Stop`** (`ErrStopped` → unwind + `RunStateStopped`, **not** resumable —
   distinct from pause's preserve-and-resume), then `step`. `GraphExecutor.Pause()` / `Stop()` delegate to
   `control.Request(...)`; the listener does the same for inbound connection commands.
2. **Events out:** unify the lifecycle `HookRegistry` (`graph_executor.go:44`) with the `Status *status.Narrator` and
   `Result *result.Pipeline` channels — and **move `Status` and `Result` off `RuntimeEnvironment` onto the control
   plane** (they are events-out channels, not the world; a real refactor re-threading the do-layer emission path from
   `activation.RuntimeEnvironment.Status` / `.Result`).
3. **Ownership:** the plane lives on the `GraphExecutor` — the run's driver and command target — shared to child
   executors via `newChildExecutor` like the runtime environment, **not** on `RuntimeEnvironment`.
4. **Relationship to step 31:** step 31 implemented **only pause**, but through the `*ExecutionControl` primitive
   (carrying `ControlPause`), so the plane is forward-compatible from the start; this step realizes the rest —
   `Stop` / `step`, the listener/connection, and the `Status` / `Result` migration.

Also feeds step 31's outstanding item 2 (the public-API pause-mid-combinator resume test needs this step's mid-run
pause injection) and step 12's context-cancel fixture row.
