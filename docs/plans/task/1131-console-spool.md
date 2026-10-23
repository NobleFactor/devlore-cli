---
title: "The console side of the fork in the in-process broker: no frame dropped, narration rendered as it happens, the exec's start and end on the events stream"
issue: https://github.com/NobleFactor/devlore-cli/issues/1131
status: draft
created: 2026-10-23
updated: 2026-10-23
---

# Plan: the console side of the fork

## Summary

**The problem.** Lane 2 (#1143) made the in-process broker's frames live, and three things the issue asks for are
still not true. A subscriber that falls more than 256 frames behind **misses frames**: `brokerStream.publish` drops
rather than block the child, so a chatty `apt-get install` can lose output, and if the dropped frame is the control
frame, `op.Run` waits until its context ends. **Nobody renders narration**: the broker frames each stderr line, `op.Run`
collects it, and nothing hands it to the Narrator, so a long install is silent on writ's tty until it ends. And **no
event reaches anything**: the stub never emits on the events stream, so an exec's start and end are recorded nowhere.

**The deliverable.** Three changes in `pkg/op`, no new package. Delivery that **never drops and never blocks**: each
subscriber gets an unbounded in-memory queue drained by its own goroutine, so the child's writes return at once and
every frame arrives in `seq` order; the memory-then-file spool is the runner's, iteration 3, and the broker's
append-only per-run file is iteration 2's (2.7 § The two streams). **Narration rendered as it happens**: `op.Run`
takes an option that hands each narration frame to a function as it arrives, and the shell provider and
`op.PlatformRunner` pass the environment's Narrator. **The exec's start and end on the events stream**: two new event
kinds, `EventExecStarted` and `EventExecCompleted`, carrying the exec and unit ids and the exit status, which the stub
emits on `Events(run)`; with no subscriber they are dropped, and that subscription is the sink seam iteration 4's bus
plugs into. Lane 3 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127).

## Issue 1131

[The console side of the fork in the stub: narration, output and exit reach the caller; events reach a sink](https://github.com/NobleFactor/devlore-cli/issues/1131).
A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic
[#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. **No frame is lost and no child waits.** A subscriber that reads slowly receives every frame, late; the child's
   write never blocks on it.
2. **Narration reaches writ's tty as the command speaks**, through the caller's Narrator, as 2.7 § The two streams
   puts it: "In the caller: narration unbuffered, straight to the `Narrator`."
3. **An exec's start and end are events**, on the events stream, not the console.

## Current State

| Where | Says or does today |
| --- | --- |
| `pkg/op/broker_stream.go` `publish` | a non-blocking send per subscriber into a 256-slot channel; a full subscriber misses the frame, the gap visible in `seq` |
| `pkg/op/broker_run.go` `Run` | collects narration into `Outcome.Narration`; renders nothing; waits for the control frame until its context ends |
| `pkg/op/broker_stub.go` | delivers narration, output and control to the console; emits nothing on `Events`; the `events` streams exist and are empty |
| `pkg/op/control_plane.go` | `ControlEventKind`: phase changed, error, result; `ControlEvent` has no exec id |
| `pkg/op/provider/shell`, `op.PlatformRunner` | `op.Run` and fold the outcome; stderr reaches the caller only at the end, as `Result.Stderr` |
| 2.7 § The two streams | the broker keeps an append-only file per run and stream and a memory ring over it; the caller renders narration unbuffered |

## Requirements

### Requirement 1: delivery never drops, `pkg/op/broker_stream.go`

`subscribe` gives each subscriber a queue and a goroutine that drains it into the subscriber's channel; `publish`
appends to every subscriber's queue under the broker's lock and returns. The queue is unbounded memory: the child is
never blocked, and a subscriber that stops reading holds memory until it cancels, which `op.Run` always does on return.
`cancel` stops the goroutine and closes the channel; frames queued after it are discarded. The double and the stub
share it unchanged. The test that pinned the drop becomes the test that a slow subscriber receives every frame, in
order, with `seq` contiguous.

### Requirement 2: narration rendered, `pkg/op/broker_run.go`

`Run(ctx, broker, request, options ...RunOption)`, with `WithNarration(func(Narration))` calling the function for each
narration frame of the exec as it arrives, before the exec ends; `Outcome.Narration` still collects them.
`op.PlatformRunner` and the shell provider pass `runtimeEnvironment.Status.Warn` of the line, the level today's
`process.Runner.Run` renders a child's stderr at. The variadic option leaves every existing caller unchanged.

### Requirement 3: the exec's events, `pkg/op/control_plane.go` and the stub

`EventExecStarted` and `EventExecCompleted` join `ControlEventKind`; `ControlEvent` gains `Exec ExecID` and
`ExitStatus int`. The stub emits `EventExecStarted` before it forks and `EventExecCompleted` after the control frame,
on the run's events stream, with the unit id. The double's `Emit` is unchanged. A test subscribes to `Events` and sees
the two, in order, around an exec; another proves an exec with no events subscriber runs as before.

### Requirement 4: the documents

2.7's status companion says the console side landed and what stays for iteration 2, the per-run file and the ring;
this plan's status.

## Implementation Phases

### Phase 1: the plan lands

- [ ] Reviewed with the owner, status `approved`, committed alone on `feature/1131-console-spool`
- [ ] #1131's documents table carries this plan's link

### Phase 2: delivery (Requirement 1)

- [ ] `broker_stream.go` and its tests; `make lint-all`, `make test-race`

### Phase 3: narration and events (Requirements 2 and 3)

- [ ] `Run`'s option, the two callers, the event kinds, the stub's emits, the tests; `make lint-all`, `make test-race`

### Phase 4: the documents and the pull request

- [ ] Requirement 4
- [ ] `make check`, `make lint-all` and `make test-race` pass; the description says AI assisted and ends with
      `Signed-off-by:` and `Assisted-by: Claude Code`; the script is
      `~/Workspace/NobleFactor/devlore-cli.1131-console-spool.bash`
- [ ] Status `active` in the last commit, with Phase 5 named as the reason

### Phase 5: after the merge (rule 5)

- [ ] `make install`; on Media-Server, a `writ` run whose package manager runs a command shows that command's stderr
      on the tty as it happens; status `complete`; #1131 closed

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1131-console-spool.md` | Create | This plan |
| `pkg/op/broker_stream.go`, `pkg/op/broker_double_test.go` | Modify | Requirement 1 |
| `pkg/op/broker_run.go`, `_test.go` | Modify | Requirement 2 |
| `pkg/op/broker_platform.go`, `pkg/op/provider/shell/provider.go` | Modify | Requirement 2, the two callers |
| `pkg/op/control_plane.go`, `pkg/op/broker_stub.go`, `_test.go` | Modify | Requirement 3 |
| `docs/architecture/2.7-control-plane.status.md` | Modify | Requirement 4 |

## Related Documents

- [1130-in-process-stub.md](1130-in-process-stub.md), the broker this completes
- [2.7-control-plane.md](../../architecture/2.7-control-plane.md) § Dispatch and the fork, § The two streams;
  [2.8-eventing-infrastructure.md](../../architecture/2.8-eventing-infrastructure.md), sinks and the bus

## Open Questions

- **Narration's level.** The plan renders a child's stderr at `Warn`, as `process.Runner.Run` does today; a package
  manager's progress lines then read as warnings. `Note` reads as what most of it is, and is what
  `process.Runner.RunNarrating` chose for children that narrate on stderr. For ruling.
- **fd 3** (ruled 2026-10-22: events go to fd 3) is not in this lane: it needs a devlore-aware flag on the request and
  a record format for the child to write. The plan proposes it as its own task under #1128, after this one.
- **A file sink now**, the run's `events.ndjson`, or the events stream alone until the bus. The plan takes the stream
  alone: the stub has no run directory to write under until the executor gives runs ids.
