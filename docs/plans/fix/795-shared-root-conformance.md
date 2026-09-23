---
title: "Lanes 1 to 4: a version result, a complete walk, silence before argv, and the sysexits"
issue: https://github.com/NobleFactor/devlore-cli/issues/795
status: chartered
created: 2026-09-13
updated: 2026-09-13
---

# Plan: Lanes 1 to 4 of the command line schedule

## Summary

Lanes 1 through 4 of [#887](https://github.com/NobleFactor/devlore-cli/issues/887), the schedule that closes the
command-line epic, in one pull request that closes feature
[#832](https://github.com/NobleFactor/devlore-cli/issues/832). Four defects in the root every program shares:
`version` prints text where the convention says a result, the walk that forbids direct stdout does not recognize
stdout under another name, `--silent` cannot reach a narration that happens before argv is parsed, and the exit
codes are three different answers in three places.

**This plan covers these four lanes and nothing else.** Anything found while working them stops the work and
goes to the owner for placement.

## Issue 795

Lane 1. `cli.NewVersionCmd` writes five lines with `fmt.Fprintf`, so `--output yaml` and `--output json` print
the same five lines on every program. Observed 2026-09-04 on `star version --output yaml` and
`writ version --output yaml`.

## Issue 794

Lane 2. `NoDirectStdout` reports `fmt.Print*`, writes aimed at `os.Stdout`, the print builtins and any handoff of
`os.Stdout`. It never considered `cmd.OutOrStdout()`, which is stdout under another name, so four sites reach
stdout outside the pipeline and the walk stays green. Found 2026-09-04 on a Windows machine running
`writ repo list`.

## Issue 828

Lane 3. `--silent` is documented to suppress all status messages, and two ambient-runtime lines survive it.
`starlarkbridge.Runtime.reportSelection` narrates through the one narrator, correctly; the problem is when. star
must load its extensions to register their commands before cobra can parse anything, so the runtime is assembled
inside `newRootCmd()` and `--silent` is still an unparsed string in `os.Args` at that moment.

## Issue 884

Lane 4. Three sources give three answers. §9 of the specification documents 0, 1 and 2. The constants in
`cmd/internal/cli/output.go` are the sysexits set with five gaps. The behavior is neither: `config get
nosuchkey` exits 65 and `config set notakeyvalue` exits 64, both correct, while a bad flag and an unknown command
exit 1 on all four programs where a usage error is 64. `cmd/star/main.go:147` calls `os.Exit(1)` without ever
consulting `cli.ExitCode`. Ruled 2026-09-12: the suite reports the sysexits set as `Declare-BashScript` defines
it.

## Goals

- [ ] Lane 1: `version` emits a record through `cli.Emit`, so `--output json` parses and `--output yaml` reads.
- [ ] Lane 2: the walk reports every `cmd.OutOrStdout()` outside `cli.Emit`, and the tree is green.
- [ ] Lane 3: `--silent` silences the ambient-runtime banner.
- [ ] Lane 4: the suite's exit codes are the sysexits set, with a subprocess test per program.

## Current State

Measured 2026-09-13 in the worktree, against develop at the merge of pull request #885.

| Component | Lane | Status | Notes |
| --- | --- | --- | --- |
| `cli.NewVersionCmd` | 1 | ❌ | five lines through `fmt.Fprintf`; `--output` ignored on every program |
| `cli.NoDirectStdout` | 2 | ❌ | `cmd.OutOrStdout()` passes the walk |
| `cmd/internal/cli/version.go:61` | 2 | ❌ | one of the four sites; lane 1 removes it |
| `writ repo add` / `remove` / `list` | 2 | ❌ | three text writes through `OutOrStdout`; no `cli.Emit` in the file |
| `cmd/internal/cli/output.go:324` | 2 | ✅ | `cli.Emit`, the one legitimate caller |
| `starlarkbridge.Runtime.reportSelection` | 3 | ❌ | narrates during `newRootCmd()`, before `Execute()` parses argv |
| `cli.ExitWith` / `ExitCode` | 4 | ✅ | the machinery works; `config` uses it and exits 64 and 65 correctly |
| cobra's own errors | 4 | ❌ | a bad flag and an unknown command exit 1 on all four programs |
| `cmd/star/main.go:147` | 4 | ❌ | `os.Exit(1)`, bypassing `cli.ExitCode` |
| the constants | 4 | ❌ | eight of thirteen; `EX_OSERR`, `EX_OSFILE`, `EX_TEMPFAIL`, `EX_PROTOCOL`, `EX_CONFIG` absent |
| §9 of the specification | 4 | ❌ | documents 0, 1 and 2, matching neither the constants nor the behavior |

## Requirements

### Requirement 1: `version` is a result (lane 1)

`NewVersionCmd` returns a record and emits it through `cli.Emit`: `{version, commit, built, go, os, arch}`, named
by their JSON tags. `--output json` parses, `--output yaml` reads by eye, `--output value` gives the six values,
`--output none` prints nothing. `--short` keeps its meaning and its `-s` shorthand. The `--version` flag keeps its
one-line cobra form, which is not a result.

### Requirement 2: The walk sees stdout under every name (lane 2)

`cmd.OutOrStdout()` is called in `cli.Emit` and nowhere else. `NoDirectStdout` reports every other call, shown red
on a fixture and green on the real tree. Green needs the three `writ repo` sites to emit results: `repo add`
returns the registration it made, `repo remove` the layer it unregistered, `repo list` the layers with their roots
and states.

The verbs stay `add` and `remove`. [#791](https://github.com/NobleFactor/devlore-cli/issues/791) renames them to
`set` and `unset` and belongs to the writ deployment epic. Whether that issue's work also rewrites these bodies is
unread, so a conflict is possible and is that issue's to resolve when it runs.

### Requirement 3: Silence reaches the banner (lane 3)

`--silent` is read from `os.Args` before the runtime is assembled, and the narrator is constructed accordingly, so
the documented contract that `--silent` takes effect immediately becomes true.

This closes #828's first half. Its second half, that the banner is diagnostics rather than narration and belongs
on a DEBUG stream, waits on [#507](https://github.com/NobleFactor/devlore-cli/issues/507) and is stated in #828's
own body as a separate fix.

### Requirement 4: The exit codes are the sysexits set (lane 4)

The thirteen constants of `Declare-BashScript`, Go-named, each doc comment naming the `EX_*` it is. `cli.ExitCode`
maps cobra's own errors to `ExitUsage`. `cmd/star/main.go` exits through `cli.ExitCode`. §9 states the table and
the mapping rule: a cobra usage error is 64, a missing input 66, a failed verification or drift 1, an internal
error 70.

**Ruled 2026-09-13 by the owner: a bad `--output` value exits `EX_USAGE`, 64.** The value arrives on the command
line and never from a file, and the flag layer rejects it before any command runs, which is the moment cobra
rejects an unknown flag. §9's mapping rule states it with the rest.

## Design

**Why one pull request for four lanes.** Lanes 1 and 2 are one change: the walk cannot go green until `version`
stops writing text, which is why the schedule makes lane 2 wait on lane 1. Lanes 3 and 4 touch the same two files,
`cmd/star/main.go` and `cmd/internal/cli/output.go`.

**What this plan does not touch.** The `repo` verbs (#791's). The banner as diagnostics (#507's). Anything not in
lanes 1 through 4.

## Implementation Phases

### Phase 1: The plan

- [ ] This document, the branch's first commit; `status: chartered` on the review.

### Phase 2: Lane 1

- [ ] `NewVersionCmd` returns a record through `cli.Emit`.
- [ ] A test renders it through every format and parses the json.

### Phase 3: Lane 2

- [ ] `NoDirectStdout` reports `OutOrStdout()` outside `cli.Emit`, red on a fixture.
- [ ] `repo add`, `repo remove` and `repo list` emit results; their tests assert the json.
- [ ] The walk is green on the real tree.

### Phase 4: Lane 3

- [ ] `--silent` is read from `os.Args` before the runtime is assembled.
- [ ] A subprocess test asserts `star --silent version` writes nothing to stderr.

### Phase 5: Lane 4

- [ ] The thirteen constants; `ExitCode` maps cobra's errors; star routes through it.
- [ ] §9 states the table and the mapping rule, a bad `--output` among them.
- [ ] A subprocess test per program.

### Phase 6: Installed and exercised on this machine

- [ ] `make install`, and `writ version` names the build under test.
- [ ] Each command this pull request touches, run against the real environment, output recorded verbatim in the
      pull request: `version` under each rendering on all four programs; `repo list`; `star --silent version`; a
      bad flag, an unknown command and a bad `--output` on each program, for their exit codes.
- [ ] **The owner's row, not mine:** `writ deploy` or `writ upgrade` converges the machine before the pull request
      opens. Handed over as a command, never run here.

### Phase 7: Closure

- [ ] The four issues' acceptance boxes; lanes 1 to 4 marked closed on #887; this plan `complete` in the last
      commit.

## Test Plan

| # | Lane | What it proves | Level | Fails when |
| --- | --- | --- | --- | --- |
| 1 | 1 | `version` renders through all eight formats; `--output json` parses | unit, `cli` | it writes text |
| 2 | 1 | `--short` still works, long form and short | unit, `cli` | either form breaks |
| 3 | 2 | The walk is red on a fixture using `OutOrStdout()` outside `cli.Emit` | unit, `cli` | the rule is unenforced |
| 4 | 2 | The walk is green on the real tree | unit, `cli` | a site was missed |
| 5 | 2 | `repo list`, `add`, `remove` emit parseable results | unit, `writ` | they write text |
| 6 | 3 | `star --silent version` writes nothing to stderr | subprocess, `star` | the banner survives |
| 7 | 4 | A bad flag, an unknown command and a bad `--output` exit 64 on each program | subprocess, four | the mapping is absent |
| 8 | 4 | `config get nosuchkey` still exits 65 | subprocess, `writ` | the change broke what worked |

## Files to Create/Modify

| File | Lane | Action |
| --- | --- | --- |
| `docs/plans/fix/795-shared-root-conformance.md` | — | Create |
| `cmd/internal/cli/version.go`, `version_test.go` | 1 | Modify |
| `Makefile` | 1 | Modify: its version-stamp check reads `version --short`, which lane 1 turned into a result |
| `cmd/internal/cli/invariants.go`, `invariants_test.go` | 2 | Modify |
| `cmd/writ/writ/repo_cmd.go`, `repo_cmd_test.go` | 2 | Modify |
| `cmd/star/main.go` | 3, 4 | Modify |
| `cmd/internal/cli/output.go` | 4 | Modify |
| `docs/architecture/10-command-line-interface.md` | 4 | Modify, §9, last commit |

## Related Documents

- [#887](https://github.com/NobleFactor/devlore-cli/issues/887) -- the schedule; this plan is lanes 1 to 4
- [#832](https://github.com/NobleFactor/devlore-cli/issues/832) -- the feature these four lanes close
- [#791](https://github.com/NobleFactor/devlore-cli/issues/791) -- the `repo` verbs; not this work
- [#507](https://github.com/NobleFactor/devlore-cli/issues/507) -- the diagnostics stream; #828's second half
- `docs/architecture/10-command-line-interface.md` §5, §9, §10, §14 -- the rulings applied
