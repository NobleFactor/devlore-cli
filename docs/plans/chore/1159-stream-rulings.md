---
title: "The execution model's specifications take the rulings of 2026-10-23 and 2026-10-24: three streams, three audiences; the command provider's interface; the logon, its account and its host; the broker remotes through SSH"
issue: https://github.com/NobleFactor/devlore-cli/issues/1159
status: complete
created: 2026-10-23
updated: 2026-10-24
---

# Plan: the rulings of 2026-10-23 and 2026-10-24 land in the specifications

## Summary

**The problem.** On 2026-10-23 and 2026-10-24 the owner ruled, in conversation with the execution-model session (schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127)), on what a command's three streams are and who they are for, on what the command provider returns and offers, on how a logon subgraph travels from the app through the broker to a subgraph-executor and back, on where the account and the host belong, on who elevates, and on how the broker remotes. None of it is in a document, and passages in fourteen specifications, and in code merged on 2026-10-22, now say otherwise. The owner asked that the design documents capture the full extent of what was discussed, that the plan cover the full design, and that issues be logged for all the work.

**The deliverable.** Each ruling in the specification that owns it, in a section headed with its date, and the body corrected to match, in fourteen documents with their status companions. The questions the owner left open are recorded as open, in one list in this plan and in the specification each belongs to. Every piece of work the rulings imply has an issue (Requirement 8). This plan changes documents only, no code.

## Issue 1159

[The execution model's specifications take the rulings of 2026-10-23](https://github.com/NobleFactor/devlore-cli/issues/1159). A chore under Epic [#442](https://github.com/NobleFactor/devlore-cli/issues/442), The Command Runner, touching the specifications of [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker, [#449](https://github.com/NobleFactor/devlore-cli/issues/449), Eventing and Telemetry, and [#520](https://github.com/NobleFactor/devlore-cli/issues/520), Authorization.

## The rulings

1. **Three streams, three audiences.** Narrations target users. Filtering them is the program's business, usually set by an option on the command line and sometimes in configuration. Events target observers: the people and machines that monitor activity. Observers subscribe to events, there will be many of them, and the Eventing and Telemetry specification describes how. Outputs target users and machines, and they are what people spend time and money for. The output pipeline transforms them for either audience, as JSON, YAML, Markdown and the other formats.
2. **Standard error is narration.** Everything a program writes to standard error is narration, and all narration goes through the narration stream. The command provider takes it from the child's standard error and writes it directly to the caller's narration sink.
3. **Standard output is the result.** Everything a program writes to standard output is accumulated and returned, together with its exit code. A non-zero exit code is an error. A command has no compensating action, so the command provider's action returns a result and an error, and nothing more elaborate is called for.
4. **Events come from every component and have subscribers.** The app, the broker and, sometimes, a command all emit events onto the event bus, and anyone subscribes, the components themselves included (2026-10-24). A command emits only if it is one of our programs, through OpenTelemetry (ruling 12); the programs the command provider runs write nothing there, and the provider emits their start and end on their behalf. Nothing a request returns travels on the bus: narration, the result and the exit status have lines of their own, so an absent or slow subscriber never changes a run. Telemetry is a kind of event: a diagnostic, a metric, and so on.
5. **The logon pipeline.** A logon subgraph is the source of a three-tier request that runs from the app, through the broker, to a subgraph-runner standing for a user account. The app sends the subgraph through the broker to the subgraph-runner, which runs it the way the graph executor runs any graph. The subgraph-runner writes outputs to standard output, narration to standard error, and events to the event stream. The broker streams the narration back to the app as it arrives and returns the result to the app when the run ends, which is what writ already does in-process.
6. **The broker remotes through SSH.** No broker is installed on the machines devlore reaches. SSH is the standard for remote access: it is installed, or can be installed, nearly everywhere. Over SSH the broker ships a bundle that ensures three things on the far machine: the subgraph-runner is present, installed if it is not, so that any device can run one; the subgraph is present; and the subgraph runs under the account's credentials. How results return over SSH is to be determined.
7. **The command provider's interface (2026-10-24).** Every command devlore runs is ultimately an `exec.Cmd` from Go's `os/exec`. A bare fork is not available to a Go program, which is always multithreaded, so the interface offers what Go supports: running a program to completion, and replacing the current process with a program (exec), after a graceful shutdown that flushes narration, writes the trace and releases locks. Running a program synchronously comes with governance while it runs: a deadline that ends gracefully, the whole process tree killed on cancel, resource limits where the platform supports them, and reliable signalling. Asynchronous execution is ruled out for now; Start-Claude, ported to star, needs only synchronous commands and an exec into tmux. Options are built from Starlark by a method on the provider, on both the immediate and the planned surface.
8. **The account belongs to the logon, and the broker elevates (2026-10-24).** The account is named by the author, in a call to the command provider or its planner, and it is a property of the logon, the unit of execution; every command inside runs as it, and a call that names an account is a logon of that one command. Elevating to the account is the broker's: the app sends the subgraph and the account, and the broker makes sure a subgraph-executor is running as that account, starting one with its own privileges or reusing one. The broker therefore runs under an account with elevation privileges, and the app need not. Automatic elevation is recommended but not required, and the documentation spells out how to configure such an account. This amends the ruling of 2026-10-22 that the broker is unprivileged; devlore still holds no credential.
9. **A command processor is identified by its host and account (2026-10-24).** The subgraph is the work a processor runs, not part of its identity: one elevated executor runs many subgraphs, and runs them in parallel. The executor is a version requirement the broker satisfies, matching the app's, not a path the app chooses. What names a processor is the account and the host it lives on, written `account@host` when shown to a person; with no host it is this machine, and `caller` with no host is the direct path, with no broker hop.
10. **The logon takes the account and the host as two arguments (2026-10-24).** The account is policy and belongs in the graph; the host is the environment's and usually comes from configuration, an inventory or a variable. The account a subgraph runs as is not the account SSH logs in with: SSH may log in as `deploy` and the broker there elevate to `root`. So `plan.logon(account, host=None)` keeps them apart, and the broker combines them only as a processor's identity.
11. **SSH's configuration is the user's (2026-10-24).** How to reach a host, its user, key, port and jump host, belongs in `~/.ssh/config`, which writ already deploys from the personal layer; devlore names a host by its SSH alias and does not own that file.
12. **Events flow through OpenTelemetry natively, and the programs we run are invited (2026-10-24).** Every devlore program emits its events through OpenTelemetry natively: the subgraph-executor, the broker, and each app, lore, star and writ. A program devlore runs takes part only by its own OpenTelemetry configuration, if it wishes to, and devlore assists it: the command provider sets the standard variables in every child's environment, the Collector's endpoint, the program's service name and resource attributes (run, unit, exec, host, account) and `TRACEPARENT`, the exec's span, so a program that reads them nests its spans under the exec; a program without OpenTelemetry ignores them, and an option turns them off (#1171). Shell scripts take part through a utility that makes it easy, `otel-cli`, shipped or installed once devlore is installed (#1172). Programs that devlore does not run, but that use OpenTelemetry, are configured from outside, by their own operators.

## Outstanding design questions

Each is the owner's to rule. The documents record them as open, not as proposals.

1. **Where a logon subgraph's trace and recovery stack live**, locally and remotely: centralized or distributed. The trace must be part of the record on the app's side and also available to the subgraph-executor, together with the recovery site, which lives inside the run's authority boundary on the machine where the effect happened. #1158.
2. **The shipment back over SSH**, and with it how a remote executor's events reach the bus. SSH carries file descriptors 0, 1 and 2, and a remote executor's events are OpenTelemetry (ruling 12). The recommendation: the broker reaches the far machine with Go's SSH package, `golang.org/x/crypto/ssh`, whose one connection carries any number of channels, the session and a channel of our own for OTLP, on every platform including Windows. The alternative, with the OpenSSH client, is a remote port forward, `ssh -R 4317:localhost:4317`, so the far executor's export to its own `localhost:4317` reaches the Collector on the app's side; where the far machine can reach a Collector directly, it exports there and SSH carries nothing for the bus. Either way events stay off the request's reply path. The shipment back of the result and the trace is open with it. #1160.
3. **Input from the user while a command runs.** It comes in three kinds: a program reading standard input, which wants keystrokes forwarded to its file descriptor 0 as a byte stream, the console stream in reverse; a program that wants a terminal, such as `sudo` asking for a password, which needs a pseudo-terminal and so merges standard output and standard error, ruling it out for any command whose output is a result; and our own programs asking the user a question, best carried as a structured request and answer relayed by the broker and asked through the app's Narrator, where `--unattended` answers with the default. The recommendation is to design the third first, add the first when a command needs it, and keep the second to the direct path's `Exec`. No issue yet.
4. **A package or service command that needs root, written without an account.** Under ruling 8 it runs as root only inside a root logon. Either the author must put it in one, and preflight refuses it otherwise, or the planner puts package and service nodes whose manager needs root into a root logon for them. The recommendation is the planner, with lane 7 (#527) reframed: the backend declares that apt needs root, and the logon makes it so.
5. **The SSH client.** The owner favours Go's SSH package for its channels (question 2). It reads no `~/.ssh/config`, so devlore reads a host's resolved configuration from `ssh -G <host>`, `Match` and `Include` included, and uses `x/crypto/ssh/agent` for the agent and `x/crypto/ssh/knownhosts` for `known_hosts`; a jump host is one dial through another client. What it would still lack is the long tail, Kerberos, `ProxyCommand` scripts and hardware-key prompts. The system's OpenSSH client honours all of it but has no multiplexing on Windows. To rule.
6. **The local leg of the broker.** As this plan reads ruling 6, the HTTP/2-over-TLS remote leg and its token are withdrawn and the local leg, HTTP/2 over a Unix socket or named pipe, stays. To confirm.
7. **File descriptor 3, now that our programs emit OpenTelemetry natively (ruling 12).** Ruled 2026-10-22, events from a devlore-aware child go to fd 3. If every devlore program exports OTLP to the endpoint in its environment, fd 3 has no traffic of its own. Either it is retired, or it becomes the local carrier of a child's OTLP to the broker, for a child that cannot reach the Collector. The recommendation is to retire it and let the endpoint be the channel. To rule.
8. **Three issues left from the rulings:** #824 asks for the opposite of ruling 3 and is superseded by #1153; #1135 names an offer on a unit, a surface retired on 2026-10-22; #892 carries no epic label.

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
| 2.6 § Elevation | the requirement a `NeedsSudo()` boolean on the manager; `ElevationOffer` | 8 |
| 6.1 § Ruled 2026-10-21 item 3; § Problem space | "a unit's command declares the account it needs"; the backend declares its account | 8 |
| 6.2 § Ruled 2026-10-21; § Ruled 2026-10-22; § 4 | the manager backend declares the account; the broker unprivileged; the pool keyed by account alone | 8, 9 |
| 10-command-line-interface § 5 | "Narration — progress, warnings, errors, diagnostics" | 1 |
| 3.5.8, 3.5.9 | `shell.exec` runs `sh -c` of a string and returns both streams; PowerShell a provider of its own | 3, 7 |
| 6-execution-topology | its SSH remote design replaced on 2026-10-21 | 6 |
| 3.4; configuration.md | package leaves declare the need; `providers.elevation.brokers.ssh` | 8 |
| `pkg/platform` leaves, `op.PlatformRunner` (#1143) | each manager command names `AccountRoot` or `AccountCaller` and the broker elevates per command | 8 |

## Requirements

Each specification states the rulings it takes in a section headed with the date: in plain prose, attributed to the owner and dated, the way this plan's § The rulings states them, not as quotations.

### Requirement 1: 2.8, the eventing specification

A section **Ruled 2026-10-23: three streams, three audiences** at the head of § The three output streams, rulings 1, 2, 3 and 4 stated. The body: the three streams are **narrations** (users; stderr; filtered by the program that writes them, by its own flags and config, `--silent`, `-v`, `verbosity`), **outputs** (users and machines; stdout; shaped by the result pipeline, `-o` and its formatters, `--filter`, `--jq`), and **events** (observers; emitted through OpenTelemetry natively by the app, the broker and the subgraph-executor (ruling 12), a program devlore runs taking part by its own configuration with the command provider's assistance; subscribed to by anyone; diagnostics, metrics and telemetry are event types; never on a request's reply path). § The three output streams is restated on that footing, results no longer inside narrations. "A program's own stderr, ruled 2026-10-21" is replaced: a program's stderr is narration, all of it, written into the caller's narration sink as it was written, never read, labelled or split. § Narration vs. diagnostics keeps the two-API rule and moves diagnostics to the event stream; its OpenTelemetry table rows follow, diagnostics a Logs-signal *event* for observers, not narration. § The fork's table: `narration` carries stderr's bytes; `event` carries what fd 3 and the provider emit. Design decisions 2 and 6 amended, dated. The `diagnose-ignored-error` marker stands: a diagnostic is an event, and the sweep emits it as one.

### Requirement 2: 2.7, the broker

A section **Ruled 2026-10-23: the pipeline, the streams on the wire, and SSH**, rulings 2, 3, 5, 6 and 7 stated. The body:

- § The wire, in detail and § Message reference: a narration frame is `{"exec","unit","len","bytes"}`, stderr's bytes as they arrived, the same shape as an output frame; no `level`, no `text`.
- § Open 10 closed: a frame carries what was written; the caller writes it into its narration sink.
- § The two streams: the caller writes narration bytes into its narration sink, where its own `--silent` governs them.
- A new § **The logon pipeline**: app ⇒ broker ⇒ subgraph-runner, as ruling 5; the broker streams the runner's narration back as it comes and returns the runner's result, its stdout, with the exit status at the end, as writ does in-process (`env.Status` while the executor runs, `cli.Emit` once when `Run` returns); the app decodes the result into the logon unit's output, and the app's own `-o` shapes the final product; the app's narration flags travel with the request.
- § The remote leg rewritten: the broker remotes through SSH, as ruling 6; the bundle ensures the subgraph-runner is present, installing it if absent, the subgraph is present, and it executes under the account's credentials; no broker on the far machine. The HTTP/2-over-TLS remote leg and its token are withdrawn; the local leg, HTTP/2 over the socket or pipe, stands. § Open 5 (remote authorization) and § Open 8 (version skew) restated for SSH and the bundle. § Ruled 2026-10-22's "any device with a broker" amended: any device SSH reaches.
- § Open gains: the shipment back (ruling 6); where the trace and recovery stack live (ruling 7, #1158).
- The broker's own events: each exec's start and end, grants and refusals, runner connections, on the bus beside the app's and the relayed fd 3 (ruling 4); none on a request's reply path.
- The logon request is the account, the host and the subgraph (rulings 8 to 10); the broker keys its processors by host and account, displays them as `account@host`, supplies the executor at the app's version, and elevates with its own privileges.
- The remote leg's SSH: the connection is the user's `~/.ssh/config`, which devlore does not own (ruling 11); the login account it names is not the logon's account.
- § Open gains user input while a command runs (outstanding question 3) and the SSH client (outstanding question 5); the shipment back records outstanding question 2 with its recommendation, a remote executor's events exported as OTLP over an SSH remote port forward.

### Requirement 3: 6.2, the command runner

A section **Ruled 2026-10-23: the subgraph-runner, and what it writes where**, rulings 2, 4 and 5 stated. § 4.1 item 3 rewritten: `narration` is stderr's bytes, pumped unconditionally like stdout, no line splitting, no line limit, no UTF-8 replacement; `event` is fd 3, from devlore's own children only. § 7 Framing follows. The **subgraph-runner** is named as what the runner is when it runs a posted logon subgraph: devlore running it through the graph executor as its account, narration to its stderr under the flags the app passes, its result once at the end to stdout, its events to fd 3 (#1156). § Open questions gains the bundle's contents and installation (#1160).
- The pool key becomes the host and the account (ruling 9): a remote processor is an address whose host is not this one, so the local and remote cases are one rule. The executor's version is a property of the processor, checked when the broker connects, not part of its name.
- Subgraphs run in parallel on one elevated executor (ruling 9), so the executor keeps every piece of per-run state, the run registry, each run's console and events streams, its recovery site, keyed by run, never global to the process.

### Requirement 3a: 6.2, the broker's account

§ Ruled 2026-10-22 gains a dated amendment stating ruling 8: the broker runs under an account with elevation privileges, the app need not, and automatic elevation is recommended, not required; the broker, not the app, elevates to the logon's account. A new section, **Configuring the broker's account**, says how, per platform: on Linux and macOS a sudoers rule for the broker's account, scoped to the runner binary and the accounts it may stand for, `NOPASSWD` for automatic elevation; on Windows a service account in Administrators, or `LocalSystem` for the root analogue. In iteration 1 the account configured is the app's user, since the in-process broker runs as it (#1167).

### Requirement 4: 6.3, command execution

A section **Ruled 2026-10-23: the command provider's result**, ruling 3 stated: the command provider's actions are fallible, not compensable, `(string, error)`, Go's `exec.Cmd.Output` shape; the string is stdout, the error on a non-zero exit carries the code and the argv and no stderr, which went to narration; no accepted-exit-codes parameter until a use appears (#1153). § 6's exit-code and result questions closed by it; #824's premise recorded as superseded.

### Requirement 4a: 6.3, the command provider's interface

A section **Ruled 2026-10-24: the command provider's interface**, rulings 7 and 8 stated, with the interface itself:

- `Run(argv, options) (string, error)` and `Script(dialect, body, args, options) (string, error)`: synchronous; stderr streamed to the narration sink as bytes; stdout the result; a non-zero exit an `ExitError` carrying the argv, the code and any signal, and no stderr.
- `Exec(argv, options) error`: replaces the current process after a graceful shutdown; the direct path only, never through a broker; unavailable on Windows, where `syscall.Exec` returns `EWINDOWS`; returns only on failure.
- `Which(name) (string, error)`: where a program resolves, for probing before running.
- Every child's environment carries the OpenTelemetry variables of ruling 12 unless an option turns them off.
- `Options(kwargs) (Options, error)`: the constructor Starlark calls on both surfaces, validating keys, durations and limits where the author writes them; a dict hydrates into the same type wherever an `Options` is expected. Fields: environment, directory, stdin (bytes or auto-confirm), timeout, grace, limits; no account, which belongs to the logon (ruling 8). A call that names an account runs as a logon of that one command, in either surface.
- Governance, concretely: `CommandContext` with `Cmd.Cancel` sending SIGTERM and `Cmd.WaitDelay` as the grace before SIGKILL; `Setpgid` and a signal to the group on Unix, a Job Object killed on close on Windows; cgroups through `SysProcAttr.CgroupFD` on Linux and Job Object limits on Windows, none on macOS; `Pdeathsig` on Linux; pidfd signalling on Linux since Go 1.23; start and end events carrying the exit code, the duration and the child's user and system time.
- In the planned surface, options are a slot value in the graph document: they serialize (durations and sizes as text), a field may be a promise, and they are covered by the graph's checksum and signature.
- Why not fork, and why exec, stated for a reader who knows Windows better than Unix: fork copies the calling process and returns twice; exec replaces the program inside a process, keeping its pid, descriptors and terminal; Unix starts a program with both, Windows with `CreateProcess`; Go offers `ForkExec` (behind `exec.Cmd.Start`) and `Exec`, and not fork alone, because fork copies only the calling thread of a multithreaded runtime.

### Requirement 5: 2.3, `plan.logon`

§ Ruled 2026-10-22 gains a dated addition, rulings 8 to 10 stated: the logon carries the account, and every command inside runs as it; `plan.logon(account, host=None)` keeps the two arguments, the account a role in the graph and the host the environment's, a variable, a promise or a configured default, with no host meaning this machine; a command call that names an account is a logon of one command; run time follows 2.7 § The logon pipeline (narration streamed back, result decoded at the end) and § The remote leg (SSH, the bundle); "forks the console and events streams" and "the stamped stack back" are replaced by a reference to outstanding question 1 (#1158). Its two use cases stand. #1157 is the task.

### Requirement 6: 5.3, the recovery site

A section **Open 2026-10-23: the recovery site across a logon**, ruling 7 stated, with what is known: the site is inside the run's authority boundary, file archival a zero-copy rename, so an archived file and its recovery id live on the machine and partition where the effect happened, and a receipt's undo runs there; writ keeps the trace app-side in the store; a resume or a state-checked unwind needs the trace where it runs. Centralize or distribute is the owner's, #1158.

### Requirement 7: the status companions

Every document this plan changes gains, in its `.status.md`, a dated line saying what 2026-10-23 and 2026-10-24 moved and which issue carries the work.

### Requirement 8: the issues, verified

Every piece of work the rulings imply has an issue, typed and linked as a sub-issue, and the schedule names the iteration-1 ones. The plan's last phase checks the list against the documents as written:

| Work | Issue | Iteration |
| --- | --- | --- |
| Narration frames are stderr's bytes, into the caller's narration sink | #1151, under #1128 | 1 |
| The console stream never drops a frame | #1131, rescoped, under #1128 | 1 |
| The command provider returns `(string, error)` | #1153, under #799 | 1 (with lane 8, #1136) |
| The command provider emits exec start and end as events | #1154, under #799 | 1 (with lane 8) |
| `command.options` on both surfaces, with no account | #1164, under #799 | 1 (with lane 8) |
| A running command is governed: deadline, tree kill, limits | #1165, under #799 | 1 (with lane 8) |
| `command.which` | #1166, under #799 | 1 (with lane 8) |
| `command.exec`: replace the process after a graceful shutdown | #802, under #799, ruling recorded on it | 1 (with lane 8) |
| The platform's leaves and `op.PlatformRunner` stop choosing an account | #1168, under #1128 | 1, after outstanding question 4 |
| The broker emits its own events | #1169, under #1128 | 1 |
| `writ repo set` writes git's narration past the Narrator | #1152, bug, under #892 | 1 |
| The command provider sets the OpenTelemetry environment on every child | #1171, under #799 | 1 (with lane 8) |
| Shell scripts emit through `otel-cli`, installed with devlore | #1172, under #799 | 1 |
| How to configure the broker's account to elevate | #1167, under #1149 | 1 and 2 |
| The broker relays narration live and returns the result at the end | #1155, under #1149 | 2 |
| The subgraph-runner | #1156, under #1150 | 3 |
| `plan.logon(account, host)` | #1157, under #1150 | 3 |
| The broker remotes through SSH, the bundle | #1160, under #1150 | 3 |
| Where the trace and recovery stack live | #1158, under #1150, for ruling | 3 |
| These documents | #1159, this chore | — |

The plan for #1131, committed on `feature/1131-console-spool`, is rewritten to its rescoped task on that branch when lane 3 resumes; it is not this branch's file.

### Requirement 9: 2.6, execution policies

§ Elevation's table, its `NeedsSudo()` and its `ElevationOffer` predate ruling 8 and the retirement of `ElevationOffer` on 2026-10-22. It is restated: the requirement is the logon's account, the realization is the broker's elevation, and the per-manager boolean goes with outstanding question 4.

### Requirement 10: 6.1, authorization

§ Ruled 2026-10-21 item 3, "a unit's command declares the account it needs", and § Problem space's backend-declared account are amended by ruling 8: the account belongs to the logon, the elevation to the broker, and the `authz` package stays strictly token acquisition (2026-10-22).

### Requirement 11: 10-command-line-interface § 5

The output model's narration bullet, "progress, warnings, errors, diagnostics", becomes narration without diagnostics, which are events (ruling 1); the result and the documents bullets stand.

### Requirement 12: 3.5.8 and 3.5.9, the shell and PowerShell providers

3.5.8 is superseded by the command provider: `sh -c` of a string and a result holding both streams give way to 6.3's tiers and ruling 3's `(string, error)`. It is rewritten as the command provider's specification, or replaced by a pointer to 6.3, with the provider's name `command` (ruled 2026-10-22). 3.5.9 becomes the PowerShell dialect of the script tier.

### Requirement 13: 6, execution topology

Its pointer that the SSH remote design was replaced on 2026-10-21 is corrected: ruling 6 brings SSH back as the broker's remote transport, with a bundle in place of document 6's upload of a runner binary.

### Requirement 14: 3.4 and configuration.md

3.4's contract in which "package leaves declare the need" follows outstanding question 4. 3.2's router, built from `providers.elevation.brokers`, is renamed with it, found by Phase 4's check. configuration.md's example path `providers.elevation.brokers.ssh` is renamed to the `authz` provider's, and its use of "broker" is reconciled with the broker of 2.7.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed alone on `chore/1159-stream-rulings` -- 2026-10-24, on the owner's instruction to write the plan's decisions into the design documents
- [x] #1159's documents table carries this plan's link -- 2026-10-23

### Phase 2: the streams and the interface (Requirements 1, 3, 3a, 4, 4a, 11, 12)

- [x] 2.8, 6.2, 6.3, 10, 3.5.8, 3.5.9 and their status companions, one commit per document -- 2026-10-24

### Phase 3: the logon, the broker and the remote leg (Requirements 2, 5, 6, 9, 10, 13, 14)

- [x] 2.7, 2.3, 5.3, 2.6, 6.1, 6, 3.4, configuration.md and their status companions, one commit per document -- 2026-10-24; 3.2's `authz` router added, found by the check

### Phase 4: the check and the pull request

- [x] Requirement 8: each ruling's passages read back against § Current State, every contradicted passage gone; every outstanding question recorded where it belongs; every issue in the table open and linked -- 2026-10-24: thirteen retired phrases searched across `docs/architecture`; the hits left are citations of the old text, dated
- [x] `make check` passes; the description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1159-stream-rulings.bash` -- 2026-10-24; 2.8 also gains a worked example, a logon to a shell script that reports through `otel-cli`, on the owner's word
- [x] Status `complete` in the last commit: nothing deploys -- 2026-10-24

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1159-stream-rulings.md` | Create | This plan |
| `docs/architecture/2.8-eventing-infrastructure.md`, `.status.md` | Modify | Requirement 1 |
| `docs/architecture/2.7-control-plane.md`, `.status.md` | Modify | Requirement 2 |
| `docs/architecture/6.2-process-pool.md`, `.status.md` | Modify | Requirements 3 and 3a |
| `docs/architecture/6.3-command-execution.md`, `.status.md` | Modify | Requirements 4 and 4a |
| `docs/architecture/2.3-orchestration-primitives.md`, `.status.md` | Modify | Requirement 5 |
| `docs/architecture/5.3-recovery-site.md`, `.status.md` | Modify | Requirement 6 |
| `docs/architecture/2.6-execution-policies.md`, `.status.md` | Modify | Requirement 9 |
| `docs/architecture/6.1-authorization.md`, `.status.md` | Modify | Requirement 10 |
| `docs/architecture/10-command-line-interface.md`, `.status.md` | Modify | Requirement 11 |
| `docs/architecture/3.5.8-shell-provider.md`, `3.5.9-powershell-provider.md`, their `.status.md` | Modify | Requirement 12 |
| `docs/architecture/6-execution-topology.md`, `.status.md` | Modify | Requirement 13 |
| `docs/architecture/3.4-platform-package-managers.md`, `configuration.md`, `3.2-projected-provider-api.md`, their `.status.md` | Modify | Requirement 14 |

## Related Documents

- [1138-logon-rulings.md](1138-logon-rulings.md), the rulings of 2026-10-22 these build on
- [1130-in-process-stub.md](../task/1130-in-process-stub.md), the stub whose narration frames #1151 corrects
- NobleFactor/noblefactor-engprod#399, the documentation standards this plan already follows: rulings stated, not quoted, and no hard-wrapped prose
