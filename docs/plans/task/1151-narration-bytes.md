---
title: "A narration frame is the child's stderr bytes, written into the caller's narration sink unaltered: no level, no line splitting"
issue: https://github.com/NobleFactor/devlore-cli/issues/1151
status: active
created: 2026-10-24
updated: 2026-10-24
---

# Plan: narration as stderr's bytes

## Summary

**The problem.** The owner ruled on 2026-10-23 that everything a program writes to stderr is narration, and that it reaches the caller's narration sink as it was written: not read, not labelled, not split, not filtered, since filtering is the program's own business. The code merged in #1140 and #1143 does all four. The narration frame carries a `Level` and a `Text`. The in-process broker stamps every line `"warn"`. `process.Runner.Stream` splits stderr into lines and strips a trailing `\r`, so a progress line that redraws without a newline reaches no one until the child exits. And `op.Run` collects narration and shows none of it, so a long command is silent on the terminal until it ends.

**The deliverable.** A narration frame carries stderr's bytes, chunked as they arrive, in exactly the shape of an output frame: `{exec, unit, len, bytes}`, as 2.7's wire example has shown since #1173. `process.Runner.Stream` hands stderr through as bytes. The broker frames each write as it comes. `op.Run` writes each frame's bytes into the caller's narration sink as it arrives, through a raw write `status.Narrator` gains, so the caller's own `--silent` governs a child's narration exactly as it governs the caller's own. The command provider and `op.PlatformRunner` pass the environment's Narrator. Lane 12 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), item 2 of the order the owner set on 2026-10-24.

## Issue 1151

[A narration frame is the child's stderr bytes, written into the caller's narration sink unaltered](https://github.com/NobleFactor/devlore-cli/issues/1151). A task under feature [#1128](https://github.com/NobleFactor/devlore-cli/issues/1128) of Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. A child's stderr appears on the caller's stderr byte for byte, `\r` progress lines included, as it is written.
2. Under the caller's `--silent`, a child's stderr appears nowhere.
3. Nothing in `pkg/op` reads, labels or splits a child's stderr.

## Current State

| Where | Does today |
| --- | --- |
| `pkg/op/broker_console.go` | `Narration{Exec, Unit, Level, Text}`; `NarrationFrame(exec, unit, level, text)`; JSON `{"exec","unit","level","text"}` |
| `pkg/op/broker_stub.go:346` | the stderr callback delivers `NarrationFrame(id, unit, "warn", line)` per line |
| `pkg/process/runner.go` `Stream` | `stderr func(string)`, fed by `lineWriter`, which splits on newlines, strips a trailing `\r`, and flushes a partial line at exit |
| `pkg/op/broker_run.go` | `Outcome.Narration []Narration`; `Outcome.Stderr()` joins the texts with newlines; nothing rendered |
| `pkg/op/broker_platform.go`, `pkg/op/provider/shell/provider.go` | fold `Outcome.Stderr()` into their results; render nothing |
| `pkg/status/narrator.go` | six categorized methods, `Note`, `Warn`, `Error`, `Succeed`, `Fail`, `Print`; no way to write bytes as they are |
| 2.7 § The wire, in detail (#1173) | the narration frame is `{"exec","unit","len","bytes"}` |

## Requirements

### Requirement 1: the frame

`Narration` becomes `{Exec, Unit, Bytes}`, marshalled as `{"exec","unit","len","bytes"}` with the bytes base64 and the length beside them, as `Output` is; unmarshalling refuses a length that disagrees. `NarrationFrame(exec, unit, bytes)`. No level, no text. The JSON pins in `broker_test.go` follow 2.7's example.

### Requirement 2: the stub and the runner

`process.Runner.Stream(cmd, stdout, stderr io.Writer)`: stderr passes straight through to the writer as the child writes it, with no `lineWriter`. The in-process broker's stderr writer delivers one narration frame per write, as its stdout writer does for output. Dry-run and the audit line stand.

### Requirement 3: the caller writes it

`status.Narrator` gains `Write(p []byte) (int, error)`: the bytes, unaltered, onto the Narrator's sink, which is `Discard` under `--silent`. `op.Run` gains the option `WithNarrator(*status.Narrator)`, writing each narration frame's bytes through it as the frame arrives, before the exec ends. `Outcome.Narration` becomes the bytes collected, and `Outcome.Stderr()` returns them as a string. `op.PlatformRunner` and the shell provider pass the environment's Narrator, so a package manager's or a shell command's stderr shows on the terminal as it runs.

### Requirement 4: the documents

2.7's status companion says narration frames are bytes in the code; this plan's status.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed alone on `feature/1151-narration-bytes` -- 2026-10-24
- [x] #1151's documents table carries this plan's link -- 2026-10-24

### Phase 2: the frame, the runner and the caller (Requirements 1 to 3)

- [x] The code and the tests; `make lint-all` and `make test-race` -- 2026-10-24

### Phase 3: the documents and the pull request

- [x] Requirement 4 -- 2026-10-24
- [x] `make check`, `make lint-all` and `make test-race` pass; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1151-narration-bytes.bash` -- 2026-10-24
- [x] Status `active` in the last commit, with Phase 4 named as the reason -- 2026-10-24

### Phase 4: after the merge (rule 5)

- [ ] `make install`; on Media-Server a `writ` run whose package manager prints progress shows it on the terminal as it happens, and shows nothing under `--silent`; status `complete`

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/task/1151-narration-bytes.md` | Create | This plan |
| `pkg/op/broker_console.go`, `broker_test.go` | Modify | Requirement 1 |
| `pkg/process/runner.go`, `pkg/op/broker_stub.go`, their tests | Modify | Requirement 2 |
| `pkg/status/narrator.go`, its test; `pkg/op/broker_run.go`, `broker_platform.go`, `provider/shell/provider.go`, their tests | Modify | Requirement 3 |
| `docs/architecture/2.7-control-plane.status.md` | Modify | Requirement 4 |

## Related Documents

- [1159-stream-rulings.md](../chore/1159-stream-rulings.md), rulings 1 and 2
- [2.7-control-plane.md](../../architecture/2.7-control-plane.md) § The wire, in detail; [2.8-eventing-infrastructure.md](../../architecture/2.8-eventing-infrastructure.md) § Ruled 2026-10-23 and 2026-10-24
- [1130-in-process-stub.md](1130-in-process-stub.md), the stub this corrects

## Open Questions

1. **A visible change.** Every package manager command devlore runs, queries included, now shows its stderr on the terminal as it runs, where today it shows nothing unless the command fails. That is the ruling, and `--silent` hides it all; it is named so it surprises no one.
2. **`process.Runner`'s other methods.** `Run`, `Capture` and `RunNarrating` still relay a child's stderr line by line through `Warn` or `Note`, which labels and splits it. Their callers, `devlore self upgrade` and the runtime environment's own `Run`, are outside the broker. The plan leaves them; #1177 carries them.
