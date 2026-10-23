---
title: "The in-process stub behind the broker's interface: the caller's account through pkg/process, root through one sudo -n, every other sudo in pkg/platform deleted"
issue: https://github.com/NobleFactor/devlore-cli/issues/1130
status: draft
created: 2026-10-22
updated: 2026-10-22
---

# Plan: the in-process stub

## Summary

**The problem.** `op.Broker` landed in #1140 with nothing behind it. Every command devlore runs still runs the old
way: the shell provider's `sh -c` in-process, `pkg/platform`'s `runCommand(argv, sudo bool)` prefixing `sudo -n` at
twenty-two of its seventy call sites under a `TODO(elevation)`, the Windows leaves' `cmd /c` strings with
`Start-Process -Verb RunAs`. Nothing asks for an account; a boolean says whether to be root. Until something stands
behind the interface, no provider can be written against it, and the `sudo` the interface exists to centralize stays
inlined.

**The deliverable.** `op.InProcessBroker`, iteration 1's stub, in the caller's process behind `op.Broker`: an exec
for the caller's own account runs through `pkg/process.Runner`, so dry-run and the Narrator keep working; an exec for
`root` runs through **one** `sudo -n` prefix, the standing TODO done literally, in the one place the word now appears;
any other account is refused with a named `Refusal`. Frames reach the console at the exec's end, narration per stderr
line, one output chunk, one control frame; lane 3 makes them live and adds the event sink. `op.Run` is the synchronous
convenience every provider needs now: subscribe, exec, collect until control. `pkg/platform` takes a `CommandRunner`
through its `Spec`, every leaf's boolean becomes the account it meant, `runCommand` and its `sudo` go, and the runtime
environment hands the platform the runner its broker backs. The shell, pkg and service providers run through it.
Windows keeps `runWindowsCommand`, which has no `sudo`, as 6.2 § 10's desktop interim row until lane 8 reshapes the
Windows leaves to argv. Lane 2 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127).

**What outlives iteration 3.** The owner, 2026-10-22: "we launch commands from the app process space. and this is a
good optimization for when you're not in a login." The caller's path is 2.7's **direct path**: a command for the
caller's own account needs no logon and no policy, so it is forked from the app's process with the same frames, and
no broker hop is paid. That path stays, and `InProcessBroker` is its name. What iteration 3 retires is the `sudo -n`
root path, replaced by the runner standing for root reached through the broker daemon.

## Issue 1130

[The in-process stub: the caller's account through pkg/process, root through one sudo -n behind the interface](https://github.com/NobleFactor/devlore-cli/issues/1130).
A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic
[#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. **`grep -rn sudo pkg/platform` finds nothing**, and `grep -rn 'sudo -n' pkg` finds the stub and nothing else.
2. **No boolean says whether.** Every command names the account it runs as, `caller` or `root`, where today a `true`
   or `false` sits, and the platform's leaves receive their runner from the caller that owns it (6.2 § Open 2).
3. **Providers run through the interface** and keep their behavior: the shell provider's result shape, the package
   and service providers' receipts, dry-run narrating and skipping, the Narrator seeing stderr.
4. **What is interim is named**: the `sudo -n` root path, `op.Run`'s collect-at-end until lane 3, the Windows
   interim row. The caller's path is not interim: it is the direct path, kept.

## Current State

| Where | Says or does today |
| --- | --- |
| `pkg/platform/helpers_unix.go:58` | `var runCommand = func(argv []string, sudo bool) Result`: 30-minute timeout, `DEBIAN_FRONTEND=noninteractive`, 4096 `y\n` on stdin, `sudo -n` when the bool is true; a package var so two tests in `update_unix_test.go` substitute it |
| the Unix leaves | 70 calls: 48 `false`, 22 `true`, across `linux_managers_linux.go`, `darwin_managers_darwin.go`, `cross_distro_managers_linux.go`, `detect_linux.go` |
| `pkg/platform/windows_managers_windows.go:268` | `runWindowsCommand(command string, elevated bool)`: `cmd /c` of a string, `Start-Process -Wait -Verb RunAs` when elevated; 14 calls, 4 elevated; no `sudo` |
| `pkg/platform/platform.go` | `Spec` carries the leaves and the service manager; `New` seals them; no runner |
| `pkg/op/runtime_environment.go:221-227` | builds the platform from `platform.Detect()` when the spec has none; `runner()` builds a `process.Runner` from the environment's context, dry-run flag, pipeline and Narrator |
| `pkg/op/provider/shell/provider.go:49` | `Exec(command string)` runs `sh -c` in-process, both streams captured into `Result{Command, ExitCode, Stdout, Stderr}` |
| `pkg/op/provider/pkg`, `service` | reach `p.RuntimeEnvironment().Platform` and call its managers; nothing of theirs runs a command |
| `cmd/internal/lorepackage`, `cmd/devlore-test` | call `platform.New(spec)` themselves, for queries; nothing elevated |
| `pkg/op/broker.go` | the interface, #1140; `RecordingBroker` the double |
| 6.2 § 10 | the interim row: "`sudo` the runner, as the stub of iteration 1 of the schedule does with `sudo -n`"; the Windows desktop row, `runas`, one UAC dialog |

## Requirements

### Requirement 1: the stub, `pkg/op/broker_stub.go`

`InProcessBroker` implements `Broker`, in the app's process, forking commands from it. `Exec`: validates the request;
`caller` runs the argv, or the script written to a `0o600` file under a scratch directory and run by its dialect's
interpreter with the args, through the environment's `process.Runner`, so dry-run narrates and skips and stderr lines
reach the Narrator as today, which is the direct path of 2.7 and is kept; `root` runs the same argv with `sudo -n`
prefixed, the one site, which iteration 3 retires; any other account returns `&Refusal{Reason: "account X is not available
without a broker", Account: X}`. Env and Cwd apply; `Stdin` bytes feed the child, `AutoConfirm` feeds the `y\n`
stream `pkg/platform` fed. On exit the stub delivers to the run's console, in order: one narration frame per stderr
line, one output frame with stdout, one control frame with the exit status and any launch error; the exec id is
minted before the child starts, so a subscriber who subscribed before `Exec` sees everything. `Post` is refused with
a named failure until the logon lane. `Cancel` kills the child. `Console`, `Events`, `Command` and `Status` are the
`RecordingBroker`'s fan-out and a run registry the executor fills; `Command` for a run this process holds goes to
its `ControlPlane`. Windows: the same, with `root` refused as a named failure, since the interim elevation on Windows
is a UAC dialog the stub never raises (6.2 § 10); the Windows leaves keep their own path for now (Requirement 3).

### Requirement 2: `op.Run` and the environment

`Run(ctx, broker, request) (Outcome, error)`: subscribes to the run's console, execs, collects frames for the exec id
until its control frame, returns `Outcome{ExitStatus, Output []byte, Narration []Narration, Error string}`; a refusal
or validation error returns as is. `RuntimeEnvironmentSpec.Broker` and `WithBroker`; `RuntimeEnvironment.Broker`,
defaulting to an `InProcessBroker` bound to the environment when the spec gave none; the environment's platform,
when it detects one, is built `WithCommandRunner` of a runner that goes through its broker (Requirement 3).

### Requirement 3: `pkg/platform` takes its runner

`type CommandRunner func(ctx context.Context, account string, argv []string) Result` in `manager.go`;
`Spec.WithCommandRunner`; `New` hands it to every Unix leaf and the service manager through an unexported setter each
leaf implements, and refuses a spec with none. `runCommand` and its three knobs go: the timeout and the `y\n` stream
become the runner's (the stub's `AutoConfirm`), and `DEBIAN_FRONTEND=noninteractive` moves to the apt leaf's calls as
Env, which is where 6.2 § Open 4 says it belongs. Every `true` becomes `"root"` and every `false` `"caller"` at its
call site; the two test seams become fakes passed `WithCommandRunner`, one of which records the account per call and
proves `apt install` asks for `root` and `brew info` for `caller`. `cmd/internal/lorepackage` and `cmd/devlore-test`
pass `platform.LocalRunner()`, a runner in `pkg/platform` for the caller's account only, refusing any other with a
named error, since they query and never elevate. `runWindowsCommand` stays, untouched, with a comment naming it 6.2
§ 10's desktop interim row and lane 8 as its end.

### Requirement 4: the providers

The shell provider's `Exec` becomes `op.Run` of a `caller` script, dialect `sh`, body the command, `Result` unchanged
in shape, `Stderr` the narration lines joined. The pkg and service providers change nothing: their platform's leaves
now run through the environment's broker. Each provider's existing tests pass; the shell provider gains one that
proves its command ran through the environment's broker, by a `RecordingBroker` on the spec seeing the request.

### Requirement 5: the documents

6.2 § 10's interim row cites the stub by name; 2.7's status companion says the stub landed, lane 3 pending; this
plan's status.

## Implementation Phases

### Phase 1: the plan lands

- [ ] Reviewed with the owner, status `approved`, committed alone on `feature/1130-in-process-stub`
- [ ] #1130's documents table carries this plan's link

### Phase 2: the stub and the environment (Requirements 1 and 2)

- [ ] `pkg/op/broker_stub.go`, `pkg/op/broker_run.go`, their tests, the spec and environment fields; `make lint`,
      `make test`

### Phase 3: the platform (Requirement 3)

- [ ] `CommandRunner`, `WithCommandRunner`, `LocalRunner`, every leaf, `runCommand` deleted, the tests; the two
      `cmd` callers; `grep -rn sudo pkg/platform` empty; `make lint`, `make test`

### Phase 4: the providers (Requirement 4)

- [ ] The shell provider through `op.Run`; the pkg and service providers' tests pass unchanged; the new shell test

### Phase 5: the documents and the pull request

- [ ] Requirement 5
- [ ] `make check` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude
      Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1130-in-process-stub.bash`
- [ ] Status `active` in the last commit, with Phase 6 named as the reason

### Phase 6: after the merge (rule 5)

- [ ] `make install`; `writ deploy` on Media-Server from an unprivileged shell runs its Home scope through the stub,
      and `writ --version` reports the merge; status `complete`; #1130 closed

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1130-in-process-stub.md` | Create | This plan |
| `pkg/op/broker_stub.go`, `_test.go` | Create | Requirement 1 |
| `pkg/op/broker_run.go`, `_test.go` | Create | Requirement 2, `op.Run` |
| `pkg/op/runtime_environment.go` | Modify | Requirement 2, the broker on the spec and the environment, the platform's runner |
| `pkg/platform/manager.go`, `platform.go`, `helpers_unix.go`, the Unix leaves, `update_unix_test.go`, `windows_managers_windows.go` | Modify | Requirement 3 |
| `cmd/internal/lorepackage/package.go`, `search.go`, `cmd/devlore-test/devloretest/runner.go`, `test_context.go` | Modify | Requirement 3, `LocalRunner` |
| `pkg/op/provider/shell/provider.go`, `_test.go` | Modify | Requirement 4 |
| `docs/architecture/6.2-process-pool.md`, `2.7-control-plane.status.md` | Modify | Requirement 5 |

## Related Documents

- [1129-broker-client-interface.md](1129-broker-client-interface.md), the interface this stands behind
- [6.2-process-pool.md](../../architecture/6.2-process-pool.md) § 4.1, § 10, § Open 4; [2.7-control-plane.md](../../architecture/2.7-control-plane.md) § Dispatch and the fork
- [6.3-command-execution.md](../../architecture/6.3-command-execution.md): argv or a script file, never a string
- Lane 3, [#1131](https://github.com/NobleFactor/devlore-cli/issues/1131), the console side made live; lane 7,
  [#527](https://github.com/NobleFactor/devlore-cli/issues/527), where each backend declares its account through authz
  and `NeedsSudo` goes

## Open Questions

- **Frames at exit, not as they happen.** Lane 2 delivers the console when the child exits; lane 3 makes it live.
  A long `apt install` therefore narrates at its end under this lane alone. Accepted, since lane 3 follows at once.
- **The scratch directory for tier-2 scripts** under the stub: `os.MkdirTemp` per exec, removed after, or the
  environment's recovery site. The plan takes the temp directory; the runner of iteration 3 owns the real scratch root.
- **Whether `LocalRunner` should exist at all**, or the two `cmd` callers should build a runtime environment and take
  its broker. The plan takes the runner, as the smaller change for callers that only query.
