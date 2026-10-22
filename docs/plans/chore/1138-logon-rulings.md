---
title: "The execution model's specifications take the rulings of 2026-10-22: the logon, authz as token acquisition, and plan.logon, the fifth combinator"
issue: https://github.com/NobleFactor/devlore-cli/issues/1138
status: approved
created: 2026-10-22
updated: 2026-10-22
---

# Plan: the logon rulings land in 6.1, 6.2 and 2.7

## Summary

The rulings of 2026-10-22, taken while the execution-model session walked the first of its six open questions and
then the big picture: what running code as another account is for; that it is a logon and sudo only a means to one;
that the service manager makes the logons and devlore is never a key store; that the macOS login keychain is a
GUI-session thing the runner must declare; that the `authz` package is strictly authorization for token acquisition;
that the runner owns the loop, option 2 of 2.7 § Open 1, because the serialization boundary is the price of
dispatching units to any device with a broker; that the mechanism is a new control structure, `plan.logon`, a subgraph
that produces at plan time and executes at run time a graph in a local or remote logon; that #451 comes after this
work; and that the shell provider becomes `command`. Each lands in the specification that owns it, in a dated section
the body is read against, the body corrected in place. One commit per document, one pull request, no code.

## Issue 1138

[The execution model's specifications take the logon rulings of 2026-10-22](https://github.com/NobleFactor/devlore-cli/issues/1138).
A chore under Epic [#442](https://github.com/NobleFactor/devlore-cli/issues/442), The Command Runner, touching the
specifications of [#520](https://github.com/NobleFactor/devlore-cli/issues/520), Authorization, and
[#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker.

## Goals

1. **A reader of any of the three documents reads the rulings before the body**, and the body says nothing the rulings
   retired.
2. **The holes are named where they will bite:** the keychain in 6.2, the four resources in 6.1, the broker's
   privilege in 2.7.
3. **Ruled and proposed stay distinguishable.** The one proposal, a unit's account fixed when it opens, is recorded as
   open.

## Current State

| Where | Says today |
| --- | --- |
| 6.2 § 4.1, the table | a service-account runner is "started on demand by a root broker: fork, `setuid(account)`, exec the runner; a socketpair" |
| 6.2 § 4.3, Windows, row 2 | on-demand spawn under an arbitrary account needs a token path, which #528 verifies; "until it does, the Windows pool is one service per account" |
| 6.2 § 10, the table | "the root broker spawns service-account runners by `setuid`"; the interim row is `sudo` the runner |
| 6.2 § Open 6, 10 | spike the Windows token path; accounts neither root nor the caller, supported or deferred |
| 6.1 § Ruled 2026-10-21, § Problem space | three substrates; nothing on what the account conveys versus what the token opens |
| 6.1 § Open 7 | the headless case on Unix: pause, or fall back by the offer's chain |
| 2.7 § Ruled 2026-10-21, 1 | the broker authenticates the peer; nothing on its own privilege |
| 2.7 § Open 3 | "The broker runs as root, so installing it is the one elevation that cannot go through it" |
| 2.7 § Authorization at the socket | policy in the broker's profile; nothing on credentials |
| the three status companions | no entry for 2026-10-22 |

## Requirements

The owner's words of 2026-10-22, quoted in the sections: "1 is all accessing network and operating system resources.
Grants to access these resources must be known to convey by the account I choose to use. 2 is all about protecting
network and operating system resources ... remote the test code selectively. 3 is all about account claims on the
network and operating system resources produced by a body of code. It is the user account that makes these claims.";
"It is the logon and the logon is to an account ... Sudo is a means to an end: a logon."; "A good name for a logon
that protects resources by identity is sandbox. It's a specific kind of sandbox."; "option 2. we do not want to be a
key store. we need to rely on the OS for that. option 3 is a good fallback on Unix. if the broker's service account can
logon to the requested user account, it will start a runner with sudo -u X."; "The macOS keychain is a bitch that
way."; "Option 2 is the way to go. The cost gives us what i think we ultimately want: the ability to dispatch
execution units to any device with a broker. The serialization boundary is the cost we pay for that."; "what we need
is a subgraph that is specifically designed to produce (during planning) and execute a graph that runs in a local or
remote logon. It would be the rough equivalent of sending a script to sudo. It would be another control structure.
Another use case: running a lore package lifecycle operation."; "The authz package is strictly authorization for token
acquisition. We introduce a new control structure: the logon for executing graphs anywhere there's a broker."; "Let's
do #451 after."; "Command is a better name for the shell provider."

### Requirement 1: 6.2-process-pool.md

A section **Ruled 2026-10-22: the runner is a logon, and the service manager makes it**, after § Ruled 2026-10-21, in
the owner's words, with what it settles: § 4.1's table loses the on-demand row and gains "a service registered with
the service manager, run as the account" for service accounts, with the Unix fallback `sudo -u <account>` of the runner
binary under a sudoers rule when the broker's service account may; the "root broker" is gone from § 4.1, § 4.3 and
§ 10; § 4.3's row 2 says a service per account is the design, not the interim, and #528's token-path question is
dissolved; § 10's interim row stays as iteration 1's stub. A new **§ 4.4 macOS: two kinds of runner, and the keychain**:
the LaunchAgent in `gui/<uid>` for a human account, the LaunchDaemon for a service account, declared by the runner and
stated by the account's offer; the list of commands that keep secrets in the login keychain; service accounts' secrets
as files and tokens; the temporary per-run keychain for signing; the named failure at authorization; the iteration-3
acceptance test on a Mac. § Open 6 and 10 closed by the ruling; the open question of a unit's account fixed at open
added as proposed.

### Requirement 2: 6.1-authorization.md

A section **Ruled 2026-10-22: authorization is separated from impersonation**: the three things running as an account
is for, in the owner's words (accessing, protecting, claiming); the fourth resource, devlore's own, that only the
token opens; the logon, not sudo. § Problem space gains one paragraph saying which gate opens which resource. § Open 7
takes the keychain answer for the one case it settles, a keychain-dependent command on a daemon runner fails at
authorization as a named failure. A new open item: whether a unit's account is fixed when the unit opens, which rules
out remoting on demand; proposed by the session, not yet ruled.

### Requirement 3: 2.7-control-plane.md

§ Ruled 2026-10-21 gains a dated addition: the broker is **unprivileged** and holds no credential; what it does with
an account is connect to the runner the service manager started for it, or on Unix start one with `sudo -u` under a
rule, never mint a logon itself. § Open 3 is rewritten: what is installed once under elevation is the runners, as
services, and the broker is an ordinary service; who registers a runner for a new account is the open part. § Open 4
notes the credential never being devlore's.

### Requirement 4: the status companions

6.1, 6.2, 2.7 and 2.3's `.status.md` each gain a line under their open or completion section saying what 2026-10-22
moved.

### Requirement 5: 2.3-orchestration-primitives.md, the fifth combinator

A section **Ruled 2026-10-22: `plan.logon`, the fifth combinator**, in the owner's words, then its place under the one
rule: a subgraph whose body's executor runs in a logon, local or on another machine, through that machine's broker;
the parent executor dispatches it, receives a stamped stack, pushes it nested and unwinds through it, as for any
subgraph. What it does at plan time: the boundary (inputs are bindings whose producer is outside the body; outputs
what the parent consumes), the catalog slice, and its arguments, `account` and optionally `host`, which fix the
account when the unit is planned. At run time: inputs resolved to literals, the body re-rooted as an `op.Graph`,
signed, posted to the broker, the console and events streams forked as 2.7 says, the stamped stack received;
`CompensateLogon` posts the stack back for a state-checked unwind (`ResumeUnwind`); retry inherits the structural
floor; pause makes it a trace and resume re-posts graph and trace; exit at the body's terminal, `Stop` forwarded. The
two use cases: writ's System tree as `plan.logon("root", …)`, and a lore package lifecycle operation as the package's
account or on the target host. The Starlark name `plan.logon`; the flow provider's `flow.logon` / `CompensateLogon`.
§ The Combinators gains its entry.

### Requirement 6: 2.7-control-plane.md, option 2 and the posted unit

§ Open 1 closed by the ruling, with the answers to the three questions: a remoted unit is a full graph on the wire,
the subtree re-rooted with literal inputs, the catalog slice and the coordinator's signature, and a stamped stack on
return (option 2 of three); the remote is stateless between calls, the trace is the state, undo is the stack posted
back, resume is graph plus trace posted again; exit is the run-state machine's terminal at the cut, `Stop` forwarded,
`Pause` leaving a trace and no process. § The remote leg becomes the posted-unit protocol. § Open 15 takes the
signature answer.

### Requirement 8: fd 3

Ruled 2026-10-22, while the capture was being written: "We need the event stream. We should plan on adding that. I do
not want to guess about what is being written to stderr. narrations go to stderr. events go to fd 3." 6.2 § 4.1 item 3
and § Open 9 take it; 2.8's fork paragraph and both status companions follow.

### Requirement 7: the two procedural rulings

6.1 § Open 1 closed: #451 after this work, so lane 1 of #1127 lands in today's names and the rename sweeps it. 6.3
§ Open 6 closed: the shell provider becomes `command`, on #1127's lane 8.

## Implementation Phases

### Phase 1: the plan lands

- [x] Reviewed with the owner, status `approved`, committed alone on `chore/1138-logon-rulings` -- 2026-10-22, as
      `draft` first, approved with Requirements 5 to 7 added on the big-picture ruling
- [x] #1138's documents table carries this plan's link -- 2026-10-22

### Phase 2: the documents

- [x] 6.2 per Requirement 1, with its status companion -- 2026-10-22
- [x] 6.1 per Requirement 2, and 6.3 § Open 6 per Requirement 7, with their status companions -- 2026-10-22
- [x] fd 3 per Requirement 8, in 6.2 and 2.8 and their status companions -- 2026-10-22
- [ ] 2.7 per Requirements 3 and 6, with its status companion
- [ ] 2.3 per Requirement 5, with its status companion

### Phase 3: the pull request

- [ ] `make check` passes in the worktree
- [ ] The description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is
      `~/Workspace/NobleFactor/devlore-cli.1138-logon-rulings.bash`
- [ ] Status `complete` in the last commit: nothing deploys, so the merge is the end

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1138-logon-rulings.md` | Create | This plan |
| `docs/architecture/6.2-process-pool.md`, `.status.md` | Modify | Requirement 1, 4 |
| `docs/architecture/6.1-authorization.md`, `.status.md` | Modify | Requirement 2, 4 |
| `docs/architecture/2.7-control-plane.md`, `.status.md` | Modify | Requirement 3, 4, 6 |
| `docs/architecture/2.3-orchestration-primitives.md`, `.status.md` | Modify | Requirement 4, 5 |
| `docs/architecture/6.3-command-execution.md`, `.status.md` | Modify | Requirement 7 |
| `docs/architecture/2.8-eventing-infrastructure.md`, `.status.md` | Modify | Requirement 8 |

## Related Documents

- [docs/plans/chore/1119-broker-specification.md](1119-broker-specification.md), the pull request that made these
  three documents the execution model's, 2026-10-21
- [#528](https://github.com/NobleFactor/devlore-cli/issues/528), whose question dissolves; closing it is the owner's
- Schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), whose first open question this
  conversation walks

## Open Questions

- **Whether #528 closes with this pull request** or the owner closes it on reading 6.2 § 4.3 as revised.
