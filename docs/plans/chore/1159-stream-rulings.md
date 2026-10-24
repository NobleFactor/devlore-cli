---
title: "The execution model's specifications take the rulings of 2026-10-23: three streams, three audiences; the command provider's result; the subgraph-runner pipeline; the broker remotes through SSH"
issue: https://github.com/NobleFactor/devlore-cli/issues/1159
status: draft
created: 2026-10-23
updated: 2026-10-23
---

# Plan: the rulings of 2026-10-23 land in the specifications

## Summary

**The problem.** On 2026-10-23 the owner ruled, in conversation with the execution-model session (schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127)), on what a command's three streams are and who they are for, on what the command provider returns, on how a logon subgraph travels from the app through the broker to a subgraph-runner and back, and on how the broker remotes. None of it is in a document, and six passages the specs carry now say otherwise; two of them are in code merged the day before (#1140, #1143). The owner asked that the design documents capture the full extent of what was discussed, that the plan cover the full design, and that issues be logged for all the work.

**The deliverable.** Each ruling in the specification that owns it, in a section headed Ruled 2026-10-23, and the body corrected to match, in 2.8, 2.7, 6.2, 6.3, 2.3 and 5.3, with each status companion. The one question the owner left open, where a logon subgraph's trace and recovery stack live, recorded as open in 2.7 and 5.3 with what is known. Every piece of work the rulings imply already has an issue (Requirement 8); this plan changes documents only, no code.

## Issue 1159

[The execution model's specifications take the rulings of 2026-10-23](https://github.com/NobleFactor/devlore-cli/issues/1159). A chore under Epic [#442](https://github.com/NobleFactor/devlore-cli/issues/442), The Command Runner, touching the specifications of [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker, and [#449](https://github.com/NobleFactor/devlore-cli/issues/449), Eventing and Telemetry.

## The rulings

1. **Three streams, three audiences.** Narrations target users. Filtering them is the program's business, usually set by an option on the command line and sometimes in configuration. Events target observers: the people and machines that monitor activity. Observers subscribe to events, there will be many of them, and the Eventing and Telemetry specification describes how. Outputs target users and machines, and they are what people spend time and money for. The output pipeline transforms them for either audience, as JSON, YAML, Markdown and the other formats.
2. **Standard error is narration.** Everything a program writes to standard error is narration, and all narration goes through the narration stream. The command provider takes it from the child's standard error and writes it directly to the caller's narration sink.
3. **Standard output is the result.** Everything a program writes to standard output is accumulated and returned, together with its exit code. A non-zero exit code is an error. A command has no compensating action, so the command provider's action returns a result and an error, and nothing more elaborate is called for.
4. **File descriptor 3 carries events, from our own programs only.** The event stream is one we create. The programs the command provider runs write nothing to it; only programs we write do. The command provider itself may emit events, such as logs and metrics. Telemetry is a kind of event: a diagnostic, a metric, and so on.
5. **The logon pipeline.** A logon subgraph is the source of a three-tier request that runs from the app, through the broker, to a subgraph-runner standing for a user account. The app sends the subgraph through the broker to the subgraph-runner, which runs it the way the graph executor runs any graph. The subgraph-runner writes outputs to standard output, narration to standard error, and events to the event stream. The broker streams the narration back to the app as it arrives and returns the result to the app when the run ends, which is what writ already does in-process.
6. **The broker remotes through SSH.** No broker is installed on the machines devlore reaches. SSH is the standard for remote access: it is installed, or can be installed, nearly everywhere. Over SSH the broker ships a bundle that ensures three things on the far machine: the subgraph-runner is present, installed if it is not, so that any device can run one; the subgraph is present; and the subgraph runs under the account's credentials. How results return over SSH is to be determined.
7. **Open.** The trace must be part of the record on the app's side, and it must also be available to the subgraph-runner, together with the recovery site. Both the local and the remote case are hard. The question is whether traces and recovery stacks are centralized or distributed. The owner will decide it; this plan does not.

## Current State

| Where | Says today | Contradicts |
| --- | --- | --- |
| 2.8 § The three output streams | three streams: narrations (status to stderr, results to stdout), diagnostics, operational events; results inside "narrations" | 1, 3 |
| 2.8 § Narration vs. diagnostics, "A program's own stderr, ruled 2026-10-21" | a program's stderr "is diagnostics… It goes to the caller, as narration at a quiet level" | 2 |
| 2.8 § Mapping to OpenTelemetry, § Design decisions 2 and 6 | diagnostics with narration on Logs, split by severity; "a program's own stderr to the caller as quiet narration" | 2, 4 |
| 2.7 § The wire, in detail; § Message reference | a narration frame `{"exec","unit","level","text"}` | 2 |
| 2.7 § Open 10 | the rendered line or the categorized record, open | 2 |
| 2.7 § Ruled 2026-10-22; § The remote leg; § Open 5, 8 | "dispatch execution units to any device with a broker"; the remote leg the broker's API over TLS with a token, to a broker on the far machine, SSH "at most a tunnel or a bootstrap"; the stamped stack back on the control frame | 5, 6, 7 |
| 6.2 § 4.1 item 3; § 7 | narration "one frame per line, a maximum line length, a flush timer… invalid UTF-8 replaced" | 2 |
| 6.2 § 4.1, § 13 | the runner runs commands; a posted graph is the broker's (2.7) | 5 |
| 6.3 | the shell provider's result shape; § 6 open questions on exit codes and the command string | 3 |
| 2.3 § Ruled 2026-10-22, `plan.logon` | posted through the broker; "forks the console and events streams as 2.7 § The remote leg says"; the stamped stack back | 5, 6, 7 |
| 5.3 | the recovery site within one authority boundary, one process; nothing on a logon subgraph | 7 |
| `pkg/op/broker_console.go`, `broker_stub.go` (#1140, #1143) | `Narration{Exec, Unit, Level, Text}`; the stub stamps `"warn"` | 2; corrected by #1151 |

## Requirements

Each specification states the rulings it takes in a section headed Ruled 2026-10-23: in plain prose, attributed to the owner and dated, the way this plan's § The rulings states them, not as quotations.

### Requirement 1: 2.8, the eventing specification

A section **Ruled 2026-10-23: three streams, three audiences** at the head of § The three output streams, rulings 1, 2, 3 and 4 stated. The body: the three streams are **narrations** (users; stderr; filtered by the program that writes them, by its own flags and config, `--silent`, `-v`, `verbosity`), **outputs** (users and machines; stdout; shaped by the result pipeline, `-o` and its formatters, `--filter`, `--jq`), and **events** (observers; fd 3 for our own programs; diagnostics, metrics and telemetry are event types). § The three output streams is restated on that footing, results no longer inside narrations. "A program's own stderr, ruled 2026-10-21" is replaced: a program's stderr is narration, all of it, written into the caller's narration sink as it was written, never read, labelled or split. § Narration vs. diagnostics keeps the two-API rule and moves diagnostics to the event stream; its OpenTelemetry table rows follow, diagnostics a Logs-signal *event* for observers, not narration. § The fork's table: `narration` carries stderr's bytes; `event` carries what fd 3 and the provider emit. Design decisions 2 and 6 amended, dated. The `diagnose-ignored-error` marker stands: a diagnostic is an event, and the sweep emits it as one.

### Requirement 2: 2.7, the broker

A section **Ruled 2026-10-23: the pipeline, the streams on the wire, and SSH**, rulings 2, 3, 5, 6 and 7 stated. The body:

- § The wire, in detail and § Message reference: a narration frame is `{"exec","unit","len","bytes"}`, stderr's bytes as they arrived, the same shape as an output frame; no `level`, no `text`.
- § Open 10 closed: a frame carries what was written; the caller writes it into its narration sink.
- § The two streams: the caller writes narration bytes into its narration sink, where its own `--silent` governs them.
- A new § **The logon pipeline**: app ⇒ broker ⇒ subgraph-runner, as ruling 5; the broker streams the runner's narration back as it comes and returns the runner's result, its stdout, with the exit status at the end, as writ does in-process (`env.Status` while the executor runs, `cli.Emit` once when `Run` returns); the app decodes the result into the logon unit's output, and the app's own `-o` shapes the final product; the app's narration flags travel with the request.
- § The remote leg rewritten: the broker remotes through SSH, as ruling 6; the bundle ensures the subgraph-runner is present, installing it if absent, the subgraph is present, and it executes under the account's credentials; no broker on the far machine. The HTTP/2-over-TLS remote leg and its token are withdrawn; the local leg, HTTP/2 over the socket or pipe, stands. § Open 5 (remote authorization) and § Open 8 (version skew) restated for SSH and the bundle. § Ruled 2026-10-22's "any device with a broker" amended: any device SSH reaches.
- § Open gains: the shipment back (ruling 6); where the trace and recovery stack live (ruling 7, #1158).

### Requirement 3: 6.2, the command runner

A section **Ruled 2026-10-23: the subgraph-runner, and what it writes where**, rulings 2, 4 and 5 stated. § 4.1 item 3 rewritten: `narration` is stderr's bytes, pumped unconditionally like stdout, no line splitting, no line limit, no UTF-8 replacement; `event` is fd 3, from devlore's own children only. § 7 Framing follows. The **subgraph-runner** is named as what the runner is when it runs a posted logon subgraph: devlore running it through the graph executor as its account, narration to its stderr under the flags the app passes, its result once at the end to stdout, its events to fd 3 (#1156). § Open questions gains the bundle's contents and installation (#1160).

### Requirement 4: 6.3, command execution

A section **Ruled 2026-10-23: the command provider's result**, ruling 3 stated: the command provider's actions are fallible, not compensable, `(string, error)`, Go's `exec.Cmd.Output` shape; the string is stdout, the error on a non-zero exit carries the code and the argv and no stderr, which went to narration; no accepted-exit-codes parameter until a use appears (#1153). § 6's exit-code and result questions closed by it; #824's premise recorded as superseded.

### Requirement 5: 2.3, `plan.logon`

§ Ruled 2026-10-22 gains a dated addition: run time follows 2.7 § The logon pipeline (narration streamed back, result decoded at the end) and § The remote leg (SSH, the bundle); "forks the console and events streams" and "the stamped stack back" are replaced by a reference to the open question of 2.7 (#1158). Its two use cases stand. #1157 named as the task.

### Requirement 6: 5.3, the recovery site

A section **Open 2026-10-23: the recovery site across a logon**, ruling 7 stated, with what is known: the site is inside the run's authority boundary, file archival a zero-copy rename, so an archived file and its recovery id live on the machine and partition where the effect happened, and a receipt's undo runs there; writ keeps the trace app-side in the store; a resume or a state-checked unwind needs the trace where it runs. Centralize or distribute is the owner's,
#1158.

### Requirement 7: the status companions

2.8, 2.7, 6.2, 6.3, 2.3 and 5.3's `.status.md` each gain a dated line saying what 2026-10-23 moved and which issue carries the work.

### Requirement 8: the issues, verified

Every piece of work the rulings imply has an issue, typed and linked as a sub-issue, and the schedule names the iteration-1 ones. The plan's last phase checks the list against the documents as written:

| Work | Issue | Iteration |
| --- | --- | --- |
| Narration frames are stderr's bytes, into the caller's narration sink | #1151, under #1128 | 1 |
| The console stream never drops a frame | #1131, rescoped, under #1128 | 1 |
| The command provider returns `(string, error)` | #1153, under #799 | 1 (with lane 8, #1136) |
| The command provider emits exec start and end as events | #1154, under #799 | 1 (with lane 8) |
| `writ repo set` writes git's narration past the Narrator | #1152, bug, under #892 | 1 |
| The broker relays narration live and returns the result at the end | #1155, under #1149 | 2 |
| The subgraph-runner | #1156, under #1150 | 3 |
| `plan.logon` | #1157, under #1150 | 3 |
| The broker remotes through SSH, the bundle | #1160, under #1150 | 3 |
| Where the trace and recovery stack live | #1158, under #1150, for ruling | 3 |
| These documents | #1159, this chore | — |

The plan for #1131, committed on `feature/1131-console-spool`, is rewritten to its rescoped task on that branch when lane 3 resumes; it is not this branch's file.

## Implementation Phases

### Phase 1: the plan lands

- [ ] Reviewed with the owner, status `approved`, committed alone on `chore/1159-stream-rulings`
- [ ] #1159's documents table carries this plan's link

### Phase 2: the streams (Requirements 1, 3, 4)

- [ ] 2.8, 6.2, 6.3 and their status companions, one commit per document

### Phase 3: the pipeline and the remote leg (Requirements 2, 5, 6)

- [ ] 2.7, 2.3, 5.3 and their status companions, one commit per document

### Phase 4: the check and the pull request

- [ ] Requirement 8: each ruling's passages read back against the table of Current State, every contradicted passage gone; every issue in the table open and linked
- [ ] `make check` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1159-stream-rulings.bash`
- [ ] Status `complete` in the last commit: nothing deploys

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1159-stream-rulings.md` | Create | This plan |
| `docs/architecture/2.8-eventing-infrastructure.md`, `.status.md` | Modify | Requirement 1 |
| `docs/architecture/2.7-control-plane.md`, `.status.md` | Modify | Requirement 2 |
| `docs/architecture/6.2-process-pool.md`, `.status.md` | Modify | Requirement 3 |
| `docs/architecture/6.3-command-execution.md`, `.status.md` | Modify | Requirement 4 |
| `docs/architecture/2.3-orchestration-primitives.md`, `.status.md` | Modify | Requirement 5 |
| `docs/architecture/5.3-recovery-site.md`, `.status.md` | Modify | Requirement 6 |

## Related Documents

- [1138-logon-rulings.md](1138-logon-rulings.md), the rulings of 2026-10-22 these build on
- [1130-in-process-stub.md](../task/1130-in-process-stub.md), the stub whose narration frames #1151 corrects
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) § 5, the output model the three streams agree with

## Open Questions

- Rulings 6 and 7 leave the shipment back and the trace's home to the owner; the documents record them as open, not as proposals.

