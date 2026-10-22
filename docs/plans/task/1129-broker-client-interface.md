---
title: "The broker's client interface: exec by account, the console stream, commands by run id, in pkg/op with a test double"
issue: https://github.com/NobleFactor/devlore-cli/issues/1129
status: approved
created: 2026-10-22
updated: 2026-10-22
---

# Plan: the broker's client interface

## Summary

**The problem.** Every command devlore runs today is run in the caller's own process and as the caller's own account:
the shell provider's `sh -c`, `pkg/platform`'s `runCommand` with its inlined `sudo -n`, `pkg/process.Runner`. There is
no one thing a caller asks to run a command as an account, no stream on which a command's narration, output and exit
come back as they happen, and no way to reach a run by its id from outside the executor that owns it. The execution
model ruled on 2026-10-21 and 2026-10-22 puts a broker between the caller and a pool of runners, one per account, and
later on another machine; but writ, the `authz` provider, the `command` provider and `plan.logon` cannot be written
against a broker that does not exist, and must not be rewritten when it does.

**The deliverable.** A Go interface in `pkg/op`, `Broker`, that is 2.7's wire with no wire: `Exec` by account
returning an exec id or a named refusal; `Post` for the unit `plan.logon` dispatches; `Cancel`; `Console` and `Events`
as per-run subscriptions with one `seq` each; `Command` and `Status` by run id. The request and frame types carry the
wire's JSON tags and marshal to 2.7's examples byte for byte, pinned by test, so the HTTP/2 facade of iteration 2
carries them unchanged. A test double in a non-test file lets every package above it test without a broker. Nothing
behind the interface is built here: that is lane 2, the in-process stub, and lane 3, the console side of the fork.
Lane 1 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), in today's names (#451 after).

## Issue 1129

[The broker's client interface: exec by account, the console stream, commands by run id](https://github.com/NobleFactor/devlore-cli/issues/1129).
A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128), the client interface and its
in-process stub, of Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. **One interface, written once.** What writ, the `authz` provider, the `command` provider and `plan.logon` are
   written against in iteration 1 is byte-for-byte what the HTTP/2 facade carries in iteration 2: the frames marshal
   to the wire's JSON as 2.7 § The wire, in detail shows it, pinned by test.
2. **Nothing blocks.** Every subscription is a channel and a cancel, as the control plane already is; a refusal is a
   named error a caller can `errors.As`, never a dialog.
3. **Testable above it.** A test double in a non-test file, so a provider's tests drive it without a broker.

## Current State

| Where | Says or does today |
| --- | --- |
| `pkg/op/control_plane.go` | `ControlPlane`: `Request(ControlCommand) <-chan ControlResponse`, `Subscribe() (<-chan ControlEvent, cancel)`; one per run, created in `GraphExecutor`; no run id, since the executor holds its own |
| `pkg/op/provider/shell/provider.go` | `Exec(command string)` runs `sh -c` in-process, captures both streams into a `Result`; no account, no frames, no broker |
| `pkg/platform/helpers_unix.go:58-70` | `runCommand(argv, sudo bool)` inlines `sudo -n` under a `TODO(elevation)`; eight `sudo` sites in the package (lane 2's) |
| `pkg/process/runner.go` | `Runner` runs an `exec.Cmd` relaying stderr lines to the `Narrator`; the caller's own account only |
| 2.7 § Dispatch and the fork, § The two streams, § The wire, in detail, § Message reference | the exec request, the `202 {exec_id}` response, the named refusal, the console frames and their JSON, the two streams with one `seq` each |
| 2.7 § The remote leg (2026-10-22) | the posted unit: a re-rooted signed graph in, a stamped stack back |
| 6.2 § 4.1, § 7 | `{rid, argv | script, env, cwd, stdin}` in, `{rid, channel, payload}` out; stdin an attribute, bytes or `auto_confirm` |
| 6.3 | a command is an argv vector or a script file with arguments, never a string to `-c` |
| `pkg/op` | no test-double convention; test helpers live in `_test.go` files of their own package |

## Requirements

### Requirement 1: the interface, `pkg/op/broker.go`

```go
type Broker interface {
    Exec(ctx context.Context, request ExecRequest) (ExecID, error)      // 202 {exec_id}, or a *Refusal
    Post(ctx context.Context, request PostRequest) (ExecID, error)      // the posted unit of plan.logon (Open 1)
    Cancel(ctx context.Context, exec ExecID) error                      // {rid, cancel}: SIGTERM, then SIGKILL
    Console(run RunID) (frames <-chan ConsoleFrame, cancel func())      // narration, output, control; seq per run
    Events(run RunID) (events <-chan ControlEvent, cancel func())       // the run's events stream
    Command(run RunID, command ControlCommand) <-chan ControlResponse   // pause, stop, step, by run id
    Status(run RunID) (RunStatus, error)                                // the plain poll
}
```

`RunID` and `ExecID` are string types. `ExecRequest` is 6.2 § 7's request with 2.7's routing: `Run`, `Unit`,
`Account` (a string: `root`, `caller`, or a name; its final form is 6.2 § Open 8, and the interface does not decide
it), one of `Argv []string` or `Script{Dialect, Body, Args}`, `Env`, `Cwd`, `Stdin{Bytes, AutoConfirm}`. `PostRequest`
is `Run`, `Unit`, `Account`, `Host`, and the graph document bytes and signature of the re-rooted unit. `Refusal` is
the named policy failure, `{Account, Offer, Reason}`, an `error` whose text is the sentence 2.7 shows. Every request
and response type carries the JSON tags the wire uses, so the facade of iteration 2 marshals them unchanged.

### Requirement 2: the console frames, `pkg/op/broker_console.go`

`ConsoleFrame{Seq, Exec, Unit, Channel, Narration, Output, Control}` with `Channel` one of `narration`, `output`,
`control` as strings (ruled: enums are strings), and exactly one payload set: `Narration{Level, Text}`,
`Output{Bytes}`, `Control{ExitStatus, Error}`; for a posted unit the control payload also carries the stamped stack's
trace document. The three examples of 2.7 § The wire, in detail marshal from these types to those bytes, by test.
`event` is not a console channel: events are `ControlEvent`, on `Events`.

### Requirement 3: the test double, `pkg/op/broker_double.go`

`RecordingBroker` implements `Broker` in a non-test file: it records every `Exec`, `Post`, `Cancel` and `Command`
call; a test enqueues frames per exec and events per run and they are delivered in order with the per-run `seq`
stamped; `Command` answers with a status the test set; `Status` returns it. No goroutine of its own beyond delivery;
cancel closes. Its own tests prove the delivery order, the `seq`, the cancel, and that it satisfies the interface at
compile time.

### Requirement 4: the documents

The doc comment on `Broker` names the 2.7 sections it carries and says it is the one thing fixed in iteration 1.
`2.7-control-plane.status.md` gains "The client interface — landed, #1129" under Completion, with the facade still
not built. #1129's documents table carries this plan.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed alone on `feature/1129-broker-client-interface` --
      2026-10-22, `Post` on the interface now
- [x] #1129's documents table carries this plan's link -- 2026-10-22

### Phase 2: the interface and the frames

- [x] `pkg/op/broker.go` and `pkg/op/broker_console.go` per Requirements 1 and 2, with the JSON pins in
      `pkg/op/broker_test.go`; `gofmt`; `make lint` and `make test` -- 2026-10-22

### Phase 3: the double

- [x] `pkg/op/broker_double.go` and its test per Requirement 3; `make lint` and `make test` -- 2026-10-22

### Phase 4: the documents and the pull request

- [ ] Requirement 4
- [ ] `make check` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude
      Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1129-broker-client-interface.bash`
- [ ] Status `complete` in the last commit: nothing deploys

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1129-broker-client-interface.md` | Create | This plan |
| `pkg/op/broker.go` | Create | The interface, the requests, the refusal |
| `pkg/op/broker_console.go` | Create | The console frames |
| `pkg/op/broker_test.go` | Create | The JSON pins and the refusal |
| `pkg/op/broker_double.go`, `pkg/op/broker_double_test.go` | Create | The test double and its proof |
| `docs/architecture/2.7-control-plane.status.md` | Modify | Requirement 4 |

## Related Documents

- [2.7-control-plane.md](../../architecture/2.7-control-plane.md) § Dispatch and the fork, § The two streams,
  § The wire, in detail, § Message reference, § The remote leg
- [6.2-process-pool.md](../../architecture/6.2-process-pool.md) § 4.1, § 7; [6.3-command-execution.md](../../architecture/6.3-command-execution.md)
- [2.3-orchestration-primitives.md](../../architecture/2.3-orchestration-primitives.md) § Ruled 2026-10-22, `plan.logon`
- [1138-logon-rulings.md](../chore/1138-logon-rulings.md), the rulings this interface follows
- Lanes 2 and 3, [#1130](https://github.com/NobleFactor/devlore-cli/issues/1130) and
  [#1131](https://github.com/NobleFactor/devlore-cli/issues/1131), the first implementations behind it

## Open Questions

- **`Post` now, or when `plan.logon` is built — ruled 2026-10-22: now.** #1128 says the interface is fixed in
  iteration 1 and never moved, and a method added later moves it; the stub of lane 2 refuses it with a named failure
  until the logon lane.
- **`Events` on the interface, or the plane's `Subscribe` alone.** The plan puts it on the interface, because by run
  id is how a caller that does not hold the executor reaches a run, which is every caller once the broker is remote.
- **The account's form** stays a string, as 6.2 § Open 8 leaves it; the interface documents the two role words,
  `root` and `caller`, and nothing else.
