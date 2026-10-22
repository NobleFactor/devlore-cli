---
title: "The execution model's specifications take the logon rulings of 2026-10-22"
issue: https://github.com/NobleFactor/devlore-cli/issues/1138
status: draft
created: 2026-10-22
updated: 2026-10-22
---

# Plan: the logon rulings land in 6.1, 6.2 and 2.7

## Summary

Four rulings of 2026-10-22, taken while the execution-model session walked the first of its six open questions: what
running code as another account is for, that it is a logon and sudo only a means to one, that the service manager
makes the logons and devlore is never a key store, and that the macOS login keychain is a GUI-session thing the runner
must declare. Each lands in the specification that owns it, in a dated section the body is read against, the body
corrected in place. One commit per document, one pull request, no code.

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

6.1, 6.2 and 2.7's `.status.md` each gain a line under their open or completion section saying what 2026-10-22 moved.

## Implementation Phases

### Phase 1: the plan lands

- [ ] Reviewed with the owner, status `approved`, committed alone on `chore/1138-logon-rulings`
- [ ] #1138's documents table carries this plan's link

### Phase 2: the documents

- [ ] 6.2 per Requirement 1, with its status companion
- [ ] 6.1 per Requirement 2, with its status companion
- [ ] 2.7 per Requirement 3, with its status companion

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
| `docs/architecture/2.7-control-plane.md`, `.status.md` | Modify | Requirement 3, 4 |

## Related Documents

- [docs/plans/chore/1119-broker-specification.md](1119-broker-specification.md), the pull request that made these
  three documents the execution model's, 2026-10-21
- [#528](https://github.com/NobleFactor/devlore-cli/issues/528), whose question dissolves; closing it is the owner's
- Schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), whose first open question this
  conversation walks

## Open Questions

- **Whether #528 closes with this pull request** or the owner closes it on reading 6.2 § 4.3 as revised.
