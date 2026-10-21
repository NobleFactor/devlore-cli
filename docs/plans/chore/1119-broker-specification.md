---
title: "The execution model's specifications: 2.7 becomes the broker's, 6.2 the command runner's, 2.8 gains the fork, and 6.1-authorization lands"
issue: https://github.com/NobleFactor/devlore-cli/issues/1119
status: approved
created: 2026-10-21
updated: 2026-10-21
---

# Plan: the execution model's specifications

## Summary

On 2026-10-21 the owner ruled the execution model: a pool of command-runner daemons, one per account, and a broker
that unifies eventing, telemetry and the command runner; `authz` as the provider that fulfils a unit's authorization,
with elevation its process-running case; strings for enums; no "whether"; HTTP/2. The rulings, the gap analysis of
the 2026-10-16 authorization draft against the tree, and the sequence and buffering designs that followed are the
record on [#520](https://github.com/NobleFactor/devlore-cli/issues/520) (its first comment) and the drawing "writ and
the Runner Daemon". None of it is in a design document. This plan carries it into four: `2.7-control-plane.md`
becomes the broker's specification; `6.2-process-pool.md` becomes the command runner's; `2.8-eventing-infrastructure.md`
gains the fork and the console; and `6.1-authorization.md` lands from the draft, corrected, retiring
`6.1-privilege-elevation.md`. One pull request, four issues, one plan.

The documents record what is ruled as ruled and what is open as open, by name. A document that says "settled" about a
question the owner has not ruled is the defect this plan exists to avoid.

## Issue 1119

[2.7 becomes the broker's specification](https://github.com/NobleFactor/devlore-cli/issues/1119): the control plane as
its core, the listener, the runner selector, the fork, authorization at the socket, the per-run buffers, the remote
leg. Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118).

## Issue 1120

[6.2 becomes the command runner's specification](https://github.com/NobleFactor/devlore-cli/issues/1120): the account
axis, the runner daemon, the pool of them; document 6 keeps the topology. Epic
[#442](https://github.com/NobleFactor/devlore-cli/issues/442).

## Issue 1121

[2.8 gains the fork and the console](https://github.com/NobleFactor/devlore-cli/issues/1121): what the bus carries,
what the caller's stream carries, and where diagnostics go. Epic
[#449](https://github.com/NobleFactor/devlore-cli/issues/449).

## Issue 1122

[6.1-authorization.md lands from the 2026-10-16 draft](https://github.com/NobleFactor/devlore-cli/issues/1122),
corrected to the rulings, and 6.1-privilege-elevation retires. Feature
[#1116](https://github.com/NobleFactor/devlore-cli/issues/1116) under epic
[#815](https://github.com/NobleFactor/devlore-cli/issues/815); the policy side is epic
[#520](https://github.com/NobleFactor/devlore-cli/issues/520).

## Goals

1. **Four documents say what was ruled.** Each carries a "Ruled 2026-10-21" section quoting the ruling in the owner's
   words, and its body is revised to match rather than carrying the ruling as an appendix to text that contradicts it.
2. **Open questions are named, not settled.** The gap analysis has eighteen questions, five ruled. The thirteen open
   ones appear in the document that owns each, under "Open", in the form the owner rules from: the question, the
   options, what each costs.
3. **Nothing cites what the tree does not have.** The draft's stale citations (`authorization_policy.go`,
   `helpers.go:77`, `runShellCommand`, `flow/provider.go:507`) are repaired against develop at `4992f580`.
4. **The cross-references close.** `index.md`, `STATUS.md`, document 6, 6.3, and every document that linked
   `6.1-privilege-elevation.md` point at what exists after this change.
5. **The status companions tell the truth.** Each says what is designed, what is implemented (nothing new), and what
   is open, with the date.

## Current State

At develop `4992f580`, 2026-10-21.

| Document | Says today | Becomes |
| --- | --- | --- |
| `2.7-control-plane.md` | An in-process control plane; a listener bridges a remote connection; HTTP/2 ruled; narration never a wire channel; three orthogonal pieces | The broker's specification, the control plane its core |
| `2.7-control-plane.status.md` | Slices A and B landed a fused first cut; Slice C pending | Adds the broker's parts and their state |
| `2.8-eventing-infrastructure.md` | OTel as the bus, five decisions settled 2026-07-23; three output streams | Gains the fork, the console stream as outside the bus, the diagnostics audience |
| `2.8-eventing-infrastructure.status.md` | Design settled; nothing implemented | Adds the fork as designed |
| `6-execution-topology.md` | Policy triplet, privileged child process, interface shape, remote execution over SSH, telemetry | Keeps problem, triplet, topology; § Approach, § Interface Shape, § Remote Execution, § Telemetry become pointers |
| `6-execution-topology.status.md` | Design topic; Elevate stub only | Says what moved where |
| `6.1-privilege-elevation.md` + status | The elevation provider, `elevation` settled as the name, a superseded mechanism | Removed |
| `6.1-authorization.md` + status | Do not exist; the draft is in the owner's Downloads | Land, corrected |
| `6.2-process-pool.md` | Hosts keyed by interpreter × privilege × pooling × session; no implementation | The command runner's specification, on the account axis |
| `6.2-process-pool.status.md` | Design draft; open questions 1–7 | Adds the runner daemon; closes "who acquires" and the worker binary's shape; keeps the rest |
| `6.3-command-execution.md` | Two tiers; open question 3 is stdin | Records stdin as a request attribute; nothing else changes |
| `docs/architecture/index.md` | Lists 6.1 as "Privilege Elevation: The Elevator Provider" | Lists 6.1-authorization, retitles 2.7 and 6.2 |
| `docs/architecture/STATUS.md` | Row 6: "Design topic for a future plan — Elevate stub only" | Row 6 names the three documents and their state |

No code changes. `pkg/op/provider/elevator` stays until #679's own pull request.

## Requirements

### Requirement 1: `2.7-control-plane.md`, the broker (#1119)

Revised in place; the command and event design, the HTTP/2 ruling, run-id addressing and `seq` stand. The title
becomes "The DevLore Broker: the control plane and its wire". Sections, in this order after the status block:

1. **Ruled 2026-10-21.** The broker, what it unifies, what it never does (runs a command, holds an interpreter), the
   transports, HTTP/2 against HTTP/3 with the three reasons.
2. **Framing**, revised: the three orthogonal pieces stand; narration becomes a wire channel because the hosting
   application is on the other end of the wire; outputs join as a fourth thing, a result, not an event.
3. **The listener:** HTTP/2 over a Unix domain socket or a Windows named pipe, TLS remotely, the peer's uid from the
   socket, the token on the remote leg; h2c and [#689](https://github.com/NobleFactor/devlore-cli/issues/689).
4. **The runner selector**, a plug point: `account → runner session`; the four policies named; spawn on demand only
   from a root broker.
5. **Dispatch and the fork:** `POST /v1/exec`; one frame stream per exec from the runner, as the response body of the
   broker's `POST /exec` to it; the switch on the tag; `event` to the bus, `narration` / `output` / `control` to the
   console.
6. **The two streams:** `GET /v1/runs/{id}/events` for the logger and monitors; `GET /v1/runs/{id}/console` for the
   caller; one `seq` each; SSE `id:` as `seq` and `Last-Event-ID` as the resume point; a memory ring over an
   append-only file for each; drop-oldest with the gap visible on the ring, never on the file; the octet-stream
   endpoint for bulk output.
7. **The wire, in detail:** the HTTP/2 streams a caller holds, the SSE event shape (`event:` channel, `id:` seq,
   `data:` payload), base64 for bytes, live stdin's three options with the recommendation.
8. **Authorization at the socket:** the peer, the account policy from the broker's own profile, refusal as a named
   failure; the authz provider as the client side.
9. **The remote leg**, replacing document 6's SSH transfer: the same API over TLS; the direct, unbrokered path.
10. **Open:** cross-run subscriptions with a filter; authorization of an outside actor; command runner or graph
    executor (the daemon's question 1); one binary or two; installation and the first elevation; local policy
    source; per-user agent beside the system daemon; session and lifetime; version skew; where traces are written;
    narration frames rendered or categorized; the binary-output threshold; whether the gRPC twin is in the first
    cut; the per-run buffer policy under a chatty command.

### Requirement 2: `6.2-process-pool.md`, the command runner (#1120)

Revised in place; the thesis, prior art, two kinds of persistence, pooling modes, concurrency and lazy allocation
stand. The title becomes "The Command Runner: a pool of daemons, one per account". Changes:

1. **Ruled 2026-10-21** at the top: the runner stands for an account; there is always an account; no "whether".
2. **§ 4 identity:** privilege becomes **account**; the zero value is the caller's own; a runner under the caller's
   account needs no grant.
3. **A new section, the runner daemon:** one process per account, the same binary; lifetime by kind (system daemon
   for root, user agent for the caller, on demand for a service account); the request mux and request ids; the
   per-exec child by argv (tier 1) or `sh` on a static file (tier 2), never a string, never a long-lived shell; the two
   spools and their policies (lines with a flush timer for narration, chunks for output; memory then file; never
   dropped; a full disk fails the exec); fd 3 for devlore-aware children, recommended over sniffing stderr; exit as a
   control frame; cancel as a signal to one child; the child never blocked on its consumer.
4. **The case for sh**, as a section: POSIX sh on every Unix; static body with values as arguments; one child per
   command; nothing to escalate inside; a kill that lands on one process.
5. **§ 10 platform divergence:** a Windows service with `CreateProcessAsUser` as the headless path, unverified, with
   [#528](https://github.com/NobleFactor/devlore-cli/issues/528) kept as the spike that verifies it; the UAC row kept
   for the desktop.
6. **§ 11 relationship to authorization:** `ProcessSpawn`'s grant is a runner session; the lease is the session;
   revocation releases it.
7. **Absorbed from document 6:** § Approach (the worker is a runner; file operations under root become commands, with
   `devlore run --graph` as the proposed form and the alternatives named), § Interface Shape (retired: `{Start, Exec,
   WriteFile, Stop}` is the runner's API in another form), § Remote Execution (retired onto 2.7's wire;
   `Execute(cmd string, elevated bool)` named as what 6.3 and the account rule out).
8. **Open questions** revised: "who acquires" closed (the manager backend declares its account and obtains the
   runner); the worker binary closed as the runner daemon, with one-binary-or-two left to 2.7; the package-manager
   knobs, retirement thresholds, pause and resume across a restart, the PowerShell serialization boundary, JEA, kept;
   added: naming the account (name, uid, role), who declares it, accounts that are neither root nor the caller,
   remote runners, where the runner type lives.

### Requirement 3: `2.8-eventing-infrastructure.md`, the fork (#1121)

Revised in place; the five settled decisions stand. Changes:

1. **Ruled 2026-10-21:** one exec's frames are forked at the broker; operational events to the bus and the logger;
   narration and output to the caller's console stream, which is not on the bus.
2. **§ The three output streams:** where each goes after the fork; outputs as a result, the fourth thing.
3. **§ Narration vs. diagnostics:** diagnostics, a program's own stderr, go to the caller as quiet narration; the
   caller sees everything the command said, the logger everything the system did; fd 3 or `source` as the way a
   devlore-aware child's records are told from raw lines, with 6.2's recommendation.
4. **The in-process tap and the console stream distinguished:** the tap (#509) serves in-process consumers; the
   console serves the caller over the wire.
5. **The hook seam feeding the bus from inside the broker:** telemetry as the union of the runner's exec events and
   the executor's unit events.

### Requirement 4: `6.1-authorization.md` (#1122)

The draft of 2026-10-16, `devlore_authorization_specification.md`, enters the tree at
`docs/architecture/6.1-authorization.md` with `6.1-authorization.status.md`, and `6.1-privilege-elevation.md` with
its status leaves. Corrections applied in place, each marked "Ruled 2026-10-21" where it is a ruling and "Corrected
against the tree" where it is a citation:

1. **Names:** `authz` as the package, `authz.Provider`, `authz.ProviderConfig`, `authz.Broker`, `authz.BrokerConfig`;
   the term authorization; elevation as the process-running case; the elevator dead (#679).
2. **Enums:** `AuthorizationStrategy` and `GrantKind` as string enums, the empty string the zero value meaning defer;
   the int constants and `None` removed.
3. **No `Mode`:** the unit's command declares an account; the runner standing for it runs it; the "whether" dissolves.
4. **`ProcessSpawn`:** no worker; its grant is a runner session from the pool (6.2); the `{Start, Exec, WriteFile,
   Stop}` text removed.
5. **Action signatures:** `Authorize(activationRecord *op.ActivationRecord, req OfferRequest) (Grant, *Receipt,
   error)` and `CompensateAuthorize(activationRecord, *Receipt)`; the lease is the receipt, committed and pushed by
   the executor; "registered on the RecoveryStack" rewritten.
6. **Preflight:** the graph-only check in `op.ValidateGraph`; the config cross-check in `GraphExecutor.Run`'s
   preflight, with `ReasonPreflightFailed`.
7. **Config:** `providers.authz`, not top-level; the `op.PoliciesConfig` pattern (`AnnounceSection`,
   `devconfig.SectionBase`) until the recursive tree lands, with the gate named.
8. **The provider as the client side** under the broker: it requests; the broker holds the policy. Where offers and
   brokers are configured, open.
9. **The router is a `Broker`** and the pattern is recursive, per 3.2.
10. **The grant on the edge:** for the process case nothing crosses an edge; for identity assumption, the open
    question that a promise result is retained on the stack until unwind.
11. **Citations repaired:** `pkg/op/elevation_policy.go`; `runCommand` in `pkg/platform/helpers_unix.go:58` with the
    TODO at `:70-71`; `flow.Provider` has no `Elevate`; `elevator/{provider,broker,config}.go` at their lines.
12. **Open:** before or after the workflow rename (#451); where the fallback chain lives; the config gate; the grant's
    slot type and `Zero()` for the token case; which substrates in the first cut; Windows; the headless case on Unix
    (Windows is answered by 6.2 § 10: refuse, never block); the default host-escalation offer's name and whether writ
    asks by name; writ's interim; file operations under root.

The status companion lists the draft's "resolved decisions" against #521's tasks #522–#525, which close against this
document when the owner rules it adopted; the plan does not close them.

### Requirement 5: the cross-references

- `6-execution-topology.md`: § Approach, § Interface Shape, § Remote Execution and § Telemetry reduced to a paragraph
  each naming where the content went; the status block and "Relationship" lines revised; the status companion says
  what moved.
- `6.3-command-execution.md`: open question 3 recorded as closed by the request attribute, with the date; nothing else.
- `index.md`: the 2.7, 6.1 and 6.2 lines retitled and described.
- `STATUS.md`: row 6 revised.
- Every link to `6.1-privilege-elevation.md` in `docs/` repointed; `grep -rn "6.1-privilege-elevation" docs/` is
  empty at the last commit.

## Implementation Phases

### Phase 1: the plan lands

- [x] This document is reviewed with the owner and its status set to `approved` -- 2026-10-21
- [x] Committed on `chore/1119-broker-specification`, alone, before any document changes
- [x] The four issues' "Plan and design documents" tables carry this plan's link on `develop`

### Phase 2: the documents

- [x] `2.7-control-plane.md` and its status companion (Requirement 1)
- [x] `6.2-process-pool.md` and its status companion (Requirement 2)
- [x] `2.8-eventing-infrastructure.md` and its status companion (Requirement 3)
- [x] `6.1-authorization.md` and its status companion land; `6.1-privilege-elevation.md` and its status are removed
      with `git rm` (Requirement 4)

Each document is its own commit, so review can take them one at a time, and each commit ticks its box above.

### Phase 3: the cross-references

- [ ] `6-execution-topology.md`, `6.3-command-execution.md`, `index.md`, `STATUS.md` and every link to the retired
      file (Requirement 5)
- [ ] `grep -rn "6.1-privilege-elevation" docs/` is empty

### Phase 4: the pull request

- [ ] `make quality-gate` passes in the worktree (`Test-GuideFrontmatter.sh` is the check that reads documents; the
      rest confirms nothing else changed)
- [ ] Every relative link in the five changed documents resolves to a file in the tree (`scripts` are not written for
      this; a one-line shell loop in the pull request's test plan)
- [ ] The pull request's description says AI assisted and ends with `Signed-off-by:` and `Assisted-by: Claude Code`
- [ ] This document's status is set to `complete` in the last commit, every box above ticked

No phase runs after the merge: this is a documentation change with no deploy and no live path.

## What follows: the specs stay in front, and the iterations become a schedule

Ruled 2026-10-21, on the owner's suggestion. The implementation is four iterations, filed after this pull request
merges as one schedule issue whose lanes cross five epics:

1. **The authz provider and the command provider against a stubbed broker, in process.** The broker's client
   interface is fixed here and never moves. The stub runs commands the only way the tree can: `pkg/process` for the
   caller's account and `sudo -n` for root, which is the standing TODO in `helpers_unix.go` done literally: every
   inlined `sudo -n` in `pkg/platform` becomes one line behind the interface, deleted in iteration 3. The console side
   of the fork is in the interface from the start, so narration and output reach writ's Narrator; `event` frames go to
   a sink that logs or drops them. Acceptance: a `System/` deploy from an unprivileged shell through the stub.
2. **The broker is remoted:** the daemon, the socket, the wire of 2.7. For one iteration the daemon runs commands
   itself, which contradicts "the broker never runs a command"; the contradiction is temporary and named.
3. **The command runner** is written and plugged in behind the selector: 6.2.
4. **Eventing and telemetry** are plugged in behind the fork's other branch: 2.8, the bus, the logger, OpenTelemetry.

2.7 and 6.1-authorization are what iteration 1 builds against, 6.2 is iteration 3's, 2.8 is iteration 4's. The
schedule's deliverable is the proof #1118 names: `shell.run` as a client of the broker on a runner under an account,
and a `System/` deploy from an unprivileged shell through the runner standing for root.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1119-broker-specification.md` | Create | This plan |
| `docs/architecture/2.7-control-plane.md` | Modify | The broker's specification |
| `docs/architecture/2.7-control-plane.status.md` | Modify | Its state |
| `docs/architecture/6.2-process-pool.md` | Modify | The command runner's specification |
| `docs/architecture/6.2-process-pool.status.md` | Modify | Its state |
| `docs/architecture/2.8-eventing-infrastructure.md` | Modify | The fork and the console |
| `docs/architecture/2.8-eventing-infrastructure.status.md` | Modify | Its state |
| `docs/architecture/6.1-authorization.md` | Create | From the 2026-10-16 draft, corrected |
| `docs/architecture/6.1-authorization.status.md` | Create | Its state; the draft's resolved decisions against #522–#525 |
| `docs/architecture/6.1-privilege-elevation.md` | Remove (`git rm`) | Superseded |
| `docs/architecture/6.1-privilege-elevation.status.md` | Remove (`git rm`) | Superseded |
| `docs/architecture/6-execution-topology.md` | Modify | Pointers where sections moved |
| `docs/architecture/6-execution-topology.status.md` | Modify | What moved where |
| `docs/architecture/6.3-command-execution.md` | Modify | Open question 3 closed |
| `docs/architecture/index.md` | Modify | Titles and descriptions |
| `docs/architecture/STATUS.md` | Modify | Row 6 |

## Related Documents

- [#520, first comment](https://github.com/NobleFactor/devlore-cli/issues/520#issuecomment-6327141123): the gap
  analysis, the rulings, §§ 5–8.3; the source for every "Ruled 2026-10-21" section
- The drawing "writ and the Runner Daemon", the owner's artifact; its two figures are the shape Requirements 1 and 2
  describe in prose
- [`elevation-provider-rename.md`](../elevation-provider-rename.md): withdrawn by #679's ruling; it stays as history
  and says so in #679
- [step 38](../extract-starlark-from-op/phase-8/steps/38-elevation-policy.md): superseded by #520; untouched here
- [3.2 § Pluggable brokers](../../architecture/3.2-projected-provider-api.md): the router rule 6.1-authorization follows
- [configuration.md](../../architecture/configuration.md): the config tree 6.1-authorization's gate names

## Open Questions

Rulings this plan needs before a document can say "settled" rather than "open"; absent a ruling, the document records
the question.

- [ ] The daemon's question 1: command runner or graph executor. It decides the broker's size and whether the System
      graph reaches root as `devlore run --graph` on the root runner (the proposal drawn) or as file operations.
- [ ] fd 3 for devlore-aware children, against a `source` field with sniffing. 6.2 recommends fd 3.
- [ ] The daemon's name and the socket paths, drawn as `devlore broker`, `/var/run/devlore/broker.sock`,
      `runner-root.sock`; illustrative until ruled.
- [ ] Whether the draft is adopted as written apart from the corrections listed, which closes #522–#525.
- [ ] The shell provider's name. Feature #799 calls it "one command-execution surface for every platform"; 6.3's tier 1
      runs no shell, and PowerShell is a dialect of its tier 2. Proposed: `command`, with `plan.command.run(argv=[...])`
      and `plan.command.script(body=..., args=[...])` and the dialect a parameter, `shell.exec` kept one release as a
      deprecated alias. A breaking change to an authoring surface, as 6.3 weighs for `command=`. Recorded in 6.3 as
      open unless ruled first.
