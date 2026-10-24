---
title: "The execution model's specifications take the rulings of 2026-10-23: three streams, three audiences; the command provider's result; the subgraph-runner pipeline; the broker remotes through ssh"
issue: https://github.com/NobleFactor/devlore-cli/issues/1159
status: draft
created: 2026-10-23
updated: 2026-10-23
---

# Plan: the rulings of 2026-10-23 land in the specifications

## Summary

**The problem.** On 2026-10-23 the owner ruled, in conversation with the execution-model session (schedule
[#1127](https://github.com/NobleFactor/devlore-cli/issues/1127)), on what a command's three streams are and who they
are for, on what the command provider returns, on how a logon subgraph travels from the app through the broker to a
subgraph-runner and back, and on how the broker remotes. None of it is in a document, and six passages the specs carry
now say otherwise; two of them are in code merged the day before (#1140, #1143). The owner: "we must make sure that we
capture the full extent of what we've discussed in the design documents. we must make sure that the plan cover the
full design spec and that we have issues logged to cover all the work."

**The deliverable.** Each ruling in the specification that owns it, in a "Ruled 2026-10-23" section in the owner's
words, and the body corrected to match, in 2.8, 2.7, 6.2, 6.3, 2.3 and 5.3, with each status companion. The one
question the owner left open, where a logon subgraph's trace and recovery stack live, recorded as open in 2.7 and 5.3
with what is known. Every piece of work the rulings imply already has an issue (Requirement 8); this plan changes
documents only, no code.

## Issue 1159

[The execution model's specifications take the rulings of 2026-10-23](https://github.com/NobleFactor/devlore-cli/issues/1159).
A chore under Epic [#442](https://github.com/NobleFactor/devlore-cli/issues/442), The Command Runner, touching the
specifications of [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker, and
[#449](https://github.com/NobleFactor/devlore-cli/issues/449), Eventing and Telemetry.

## The rulings, in the owner's words

1. **Three streams, three audiences.** "Narrations target users. Narration filtering is handled at the program level.
   You typically set it with an option on the command line, though it can be set in config." "Events target
   observers. These are people and machines that monitor activities. Observers subscribe to events. We will have many
   observers. We write about this in the Eventing and Telemetry spec." "Outputs target users and machines. This is what
   people pay time and money for. Our output pipeline enables transformations that target users and/or machines in
   different ways: case, json, yaml, markdown,…"
2. **stderr is narration.** "Anything written to stderr is assumed to be a narration and all narrations go through our
   narration stream: stderr." "Narrations are sourced from stderr and written directly from that stream and written to
   our narration sink."
3. **stdout is the result.** "Anything written to stdout is accumulated and returned in a struct that includes the
   output to stdout and an exit code." "Results are sourced from stdout and buffered as the return value together with
   the exit code… Non-zero exit codes ⇒ error. There is no compensating action, so the return value in pseudo-code is
   (string, error, nil)."
4. **fd 3 is events, from our own programs.** "Events are sourced from the event stream that we create. I don't expect
   any activity on that stream when a program other than one we write executes… The command provider might produce
   events (logs, metrics, and such). The programs we run will not." And, earlier the same day: "fd3 for events. We have
   a spec for eventing and telemetry. In my mind, telemetry is an event type. It might be a diagnostic, a metric, …"
5. **The pipeline.** "The 3-tier request 'pipeline' from app ⇒ broker ⇒ subgraph-runner(user-account) is sourced from
   a logon subgraph. We send that subgraph from the app through the broker to the subgraph-runner which runs the graph
   exactly as you would expect: the way the graph executor does. The subgraph-runner writes outputs to stdout,
   narrations to stderr, and events to the event stream. It is the job of the broker to stream narrations back to the
   app and return results to the app at the end of the execution. We do that now in writ."
6. **The broker remotes through ssh.** "when we come to remoting, we rely on ssh. I do not want to worry about
   installing brokers on every machine i touch. ssh is the gold standard for that sort of thing. It's installed or can
   be installed everywhere and typically is." And: "we will remote through the broker using ssh. this means have a
   bundle that we can ship over the wire that ensure the graph runner is present (meaning that we can install it on any
   device), the subgraph is present, and we can execute under the account credentials. The shipment back is to be
   determined."
7. **Open.** "the trace must be part of the record app-side and must also be available to the subgraph runner together
   with the recovery site… There are two scenarios: local and remote. both are tricky. the question that pops for me
   is whether we centralize or distribute the traces and the recovery stacks?" With: "We don't need to settle it here.
   I will think about that." Also open: the shipment back (ruling 6).

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

### Requirement 1: 2.8, the eventing specification

A section **Ruled 2026-10-23: three streams, three audiences** at the head of § The three output streams, rulings 1, 2,
3 and 4 quoted. The body: the three streams are **narrations** (users; stderr; filtered by the program that writes
them, by its own flags and config, `--silent`, `-v`, `verbosity`), **outputs** (users and machines; stdout; shaped by
the result pipeline, `-o` and its formatters, `--filter`, `--jq`), and **events** (observers; fd 3 for our own
programs; diagnostics, metrics and telemetry are event types). § The three output streams is restated on that
footing, results no longer inside narrations. "A program's own stderr, ruled 2026-10-21" is replaced: a program's
stderr is narration, all of it, written into the caller's narration sink as it was written, never read, labelled or
split. § Narration vs. diagnostics keeps the two-API rule and moves diagnostics to the event stream; its OpenTelemetry
table rows follow, diagnostics a Logs-signal *event* for observers, not narration. § The fork's table: `narration`
carries stderr's bytes; `event` carries what fd 3 and the provider emit. Design decisions 2 and 6 amended, dated. The
`diagnose-ignored-error` marker stands: a diagnostic is an event, and the sweep emits it as one.

### Requirement 2: 2.7, the broker

A section **Ruled 2026-10-23: the pipeline, the streams on the wire, and ssh**, rulings 2, 3, 5, 6 and 7 quoted. The
body:

- § The wire, in detail and § Message reference: a narration frame is `{"exec","unit","len","bytes"}`, stderr's bytes
  as they arrived, the same shape as an output frame; no `level`, no `text`.
- § Open 10 closed: a frame carries what was written; the caller writes it into its narration sink.
- § The two streams: the caller writes narration bytes into its narration sink, where its own `--silent` governs them.
- A new § **The logon pipeline**: app ⇒ broker ⇒ subgraph-runner, as ruling 5; the broker streams the runner's
  narration back as it comes and returns the runner's result, its stdout, with the exit status at the end, as writ
  does in-process (`env.Status` while the executor runs, `cli.Emit` once when `Run` returns); the app decodes the
  result into the logon unit's output, and the app's own `-o` shapes the final product; the app's narration flags
  travel with the request.
- § The remote leg rewritten: the broker remotes through ssh, as ruling 6; the bundle ensures the subgraph-runner is
  present, installing it if absent, the subgraph is present, and it executes under the account's credentials; no
  broker on the far machine. The HTTP/2-over-TLS remote leg and its token are withdrawn; the local leg, HTTP/2 over the
  socket or pipe, stands. § Open 5 (remote authorization) and § Open 8 (version skew) restated for ssh and the bundle.
  § Ruled 2026-10-22's "any device with a broker" amended: any device ssh reaches.
- § Open gains: the shipment back (ruling 6); where the trace and recovery stack live (ruling 7, #1158).

### Requirement 3: 6.2, the command runner

A section **Ruled 2026-10-23: the subgraph-runner, and what it writes where**, rulings 2, 4 and 5 quoted. § 4.1 item 3
rewritten: `narration` is stderr's bytes, pumped unconditionally like stdout, no line splitting, no line limit, no
UTF-8 replacement; `event` is fd 3, from devlore's own children only. § 7 Framing follows. The **subgraph-runner** is
named as what the runner is when it runs a posted logon subgraph: devlore running it through the graph executor as its
account, narration to its stderr under the flags the app passes, its result once at the end to stdout, its events to
fd 3 (#1156). § Open questions gains the bundle's contents and installation (#1160).

### Requirement 4: 6.3, command execution

A section **Ruled 2026-10-23: the command provider's result**, ruling 3 quoted: the command provider's actions are
fallible, not compensable, `(string, error)`, Go's `exec.Cmd.Output` shape; the string is stdout, the error on a
non-zero exit carries the code and the argv and no stderr, which went to narration; no accepted-exit-codes parameter
until a use appears (#1153). § 6's exit-code and result questions closed by it; #824's premise recorded as superseded.

### Requirement 5: 2.3, `plan.logon`

§ Ruled 2026-10-22 gains a dated addition: run time follows 2.7 § The logon pipeline (narration streamed back,
result decoded at the end) and § The remote leg (ssh, the bundle); "forks the console and events streams" and "the
stamped stack back" are replaced by a reference to the open question of 2.7 (#1158). Its two use cases stand. #1157
named as the task.

### Requirement 6: 5.3, the recovery site

A section **Open 2026-10-23: the recovery site across a logon**, ruling 7 quoted, with what is known: the site is
inside the run's authority boundary, file archival a zero-copy rename, so an archived file and its recovery id live on
the machine and partition where the effect happened, and a receipt's undo runs there; writ keeps the trace app-side in
the store; a resume or a state-checked unwind needs the trace where it runs. Centralize or distribute is the owner's,
#1158.

### Requirement 7: the status companions

2.8, 2.7, 6.2, 6.3, 2.3 and 5.3's `.status.md` each gain a dated line saying what 2026-10-23 moved and which issue
carries the work.

### Requirement 8: the issues, verified

Every piece of work the rulings imply has an issue, typed and linked as a sub-issue, and the schedule names the
iteration-1 ones. The plan's last phase checks the list against the documents as written:

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
| The broker remotes through ssh, the bundle | #1160, under #1150 | 3 |
| Where the trace and recovery stack live | #1158, under #1150, for ruling | 3 |
| These documents | #1159, this chore | — |

The plan for #1131, committed on `feature/1131-console-spool`, is rewritten to its rescoped task on that branch when
lane 3 resumes; it is not this branch's file.

## Implementation Phases

### Phase 1: the plan lands

- [ ] Reviewed with the owner, status `approved`, committed alone on `chore/1159-stream-rulings`
- [ ] #1159's documents table carries this plan's link

### Phase 2: the streams (Requirements 1, 3, 4)

- [ ] 2.8, 6.2, 6.3 and their status companions, one commit per document

### Phase 3: the pipeline and the remote leg (Requirements 2, 5, 6)

- [ ] 2.7, 2.3, 5.3 and their status companions, one commit per document

### Phase 4: the check and the pull request

- [ ] Requirement 8: each ruling's passages read back against the table of Current State, every contradicted passage
      gone; every issue in the table open and linked
- [ ] `make check` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude
      Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1159-stream-rulings.bash`
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
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) § 5, the output model the three
  streams agree with

## Open Questions

- Rulings 6 and 7 leave the shipment back and the trace's home to the owner; the documents record them as open, not
  as proposals.
