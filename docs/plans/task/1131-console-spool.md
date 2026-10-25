---
title: "The console stream never drops a frame: a slow subscriber receives every one, late, and the child is never blocked on it"
issue: https://github.com/NobleFactor/devlore-cli/issues/1131
status: complete
created: 2026-10-23
updated: 2026-10-24
---

# Plan: the console stream never drops a frame

## Summary

**The problem.** The in-process broker (#1143) and its test double deliver a run's frames through `brokerStream.publish`, which sends to each subscriber's 256-slot channel without blocking and drops the frame when the channel is full. A subscriber that falls more than 256 frames behind loses narration or output, which breaks the rulings of 2026-10-23 that everything a program writes to stderr and stdout reaches the caller. If the frame it loses is the control frame, `op.Run` waits until its context ends. A chatty command, such as `apt-get install` printing thousands of lines while the caller is busy, reaches that limit.

**The deliverable.** Delivery that never drops and never blocks the child. Each subscriber gets a queue of its own in memory, drained into its channel by a goroutine of its own, so `publish` returns at once and every frame arrives in `seq` order, contiguous. The change is in one file, `pkg/op/broker_stream.go`, shared by the in-process broker and the double, and it covers both per-run streams, the console and the events. The spill to a file and the ring for reconnects are the broker's of iteration 2 (2.7 § The two streams); this lane is the in-process delivery.

## Issue 1131

[The console stream never drops a frame](https://github.com/NobleFactor/devlore-cli/issues/1131), rescoped on 2026-10-23. A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker. Lane 3 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), first in the order the owner set on 2026-10-24.

## What changed since the first plan

The first plan, committed as c6a059b0, also rendered narration and emitted exec events. The rulings of 2026-10-23 and 2026-10-24 (#1173) moved both out of this lane. Narration frames carry stderr's bytes, written into the caller's narration sink unaltered, which is #1151. An exec's start and end are events, emitted through OpenTelemetry, which is #1154 for the command provider and #1169 for the broker. This plan replaces the first.

## Goals

1. No frame is lost: a subscriber that reads slowly receives every frame of a run, in order, with `seq` contiguous.
2. No child waits: the broker's `deliver` and the double's `Deliver` and `Emit` return at once, however slow a subscriber is.
3. Cancel still ends a subscription: the channel closes, and frames still queued for it are discarded.

## Current State

| Where | Does today |
| --- | --- |
| `pkg/op/broker_stream.go` | `publish` sends to each subscriber's 256-slot channel with a `select … default`, dropping the frame for a full subscriber; `subscribe` registers the channel; `cancel` closes it under the broker's lock |
| `pkg/op/broker_stub.go`, `broker_double.go` | `deliver`, `Deliver` and `Emit` stamp `seq` and call `publish` under the broker's mutex |
| `pkg/op/broker_double_test.go` | `TestRecordingBroker_SlowSubscriberMisses` pins the drop, the behavior this lane removes |
| `pkg/op/broker_run.go` | `op.Run` subscribes before it execs and cancels when it returns |
| 2.7 § The two streams | the broker of iteration 2 keeps an append-only file per run and stream and a memory ring for reconnects; the runner's spool, memory then a file, never drops |

## Requirements

### Requirement 1: a queue per subscriber

`subscribe` creates, for each subscriber, an unbounded queue in memory and a goroutine that moves frames from the queue into the subscriber's channel in order. `publish` appends the frame to every subscriber's queue and signals its goroutine; it never blocks. `cancel` stops the goroutine, closes the channel, and discards whatever the queue still holds; it is idempotent. The queue and its goroutine are guarded by a lock of their own, so a slow channel never holds the broker's mutex.

### Requirement 2: the tests

`TestRecordingBroker_SlowSubscriberMisses` becomes `TestRecordingBroker_SlowSubscriberReceivesEverything`: far more frames than the old buffer are delivered to a subscriber that is not reading, each `Deliver` returns at once, and the subscriber then reads every frame with `seq` contiguous. A test of the in-process broker runs a command that writes many thousands of lines to stderr and stdout through `op.Run` and checks the outcome holds every byte and the control frame. A test proves cancel discards the queue and closes the channel, and that a cancelled subscriber's goroutine ends. All of it under `make test-race`.

### Requirement 3: the documents

2.7's status companion says the in-process delivery never drops, with the file and the ring still iteration 2's. This plan's status.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed on `feature/1131-console-spool`, replacing the plan of c6a059b0 -- 2026-10-24
- [x] #1131's documents table carries this plan's link -- 2026-10-23

### Phase 2: delivery and its tests (Requirements 1 and 2)

- [x] `pkg/op/broker_stream.go` and the tests; `make lint-all` and `make test-race` -- 2026-10-24

### Phase 3: the documents and the pull request

- [x] Requirement 3 -- 2026-10-24, after develop was merged in, since #1173 and #1175 had changed the same file
- [x] `make check`, `make lint-all` and `make test-race` pass; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1131-console-spool.bash` -- 2026-10-24
- [x] Status `complete` in the last commit: nothing deploys that a run can observe until #1151 lands -- 2026-10-24

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1131-console-spool.md` | Modify | This plan, replacing the first |
| `pkg/op/broker_stream.go` | Modify | Requirement 1 |
| `pkg/op/broker_double_test.go`, `pkg/op/broker_stub_test.go`, `pkg/op/broker_stream_test.go` | Modify, Create | Requirement 2 |
| `docs/architecture/2.7-control-plane.status.md` | Modify | Requirement 3 |

## Related Documents

- [1130-in-process-stub.md](1130-in-process-stub.md), the broker this delivery belongs to
- [1159-stream-rulings.md](../chore/1159-stream-rulings.md), the rulings that rescoped this lane
- [2.7-control-plane.md](../../architecture/2.7-control-plane.md) § The two streams

## Open Questions

1. **Memory.** The queue is unbounded: a subscriber that stops reading and never cancels holds every frame in memory until the run ends. `op.Run` always cancels when it returns, and the file of iteration 2 bounds it later. The plan accepts the risk for the in-process broker; a cap with a spill to a file is the alternative.
2. **The control plane's own events.** `ControlPlane.emit` drops events for a slow subscriber in the same way. It is not a console stream and is outside this lane; if it should never drop either, that is its own task.
