---
title: "lore: the search table, three hand-rolled flag sets, and thirteen fmt.Print adopt the convention"
issue: https://github.com/NobleFactor/devlore-cli/issues/775
status: draft
created: 2026-09-01
updated: 2026-09-01
---

# Plan: `lore` adopts the output convention

## Summary

`lore` registers the common set on `inspect` alone. Every other command invents its own output, and the
one real table in the tree is hand-rolled. This moves `runSearch` onto the shared `TableFormatter`,
retires three hand-rolled flag sets, triages thirteen `fmt.Print` calls, and registers the common set on
the root — which is what makes a program-wide fix program-wide.

This is the second item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)), after
[774-writ-reconcile.md](774-writ-reconcile.md).

## Goals

1. **`lore`'s root registers the common set**, so every subcommand accepts every flag.
2. **No `lore` command carries an output flag of its own.**
3. **The search table is the shared one**, which is a bug fix as much as a refactor.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `lore inspect` | ✅ `AddOutputFlags` | the one adopter |
| `lore search` | ❌ hand-rolled table | `commands.go:525-556`; truncates by byte count (#741) |
| `lore bundle` | ❌ `--output, -o <path>` | a destination, not a rendering — collides with the shared `-o` |
| `lore onboard` | ❌ `--output <dir>` + `--format` | own `--format`, own destination |
| `lore list` | ❌ `--format table\|manifest\|json` | own `--format`, values differ; the command is a stub |
| thirteen `fmt.Print` | ❌ untriaged | narration and results mixed on stdout |
| `TableFormatter` | ✅ in `pkg/result` | rune-aligned via `text/tabwriter`, landed in #740 phase 4 |

## Requirements

### Requirement 1: The common set on the root

`lore`'s root calls `AddOutputFlags`, and `inspect`'s own call is removed. Every leaf reaches
`BuildPipeline` — the invariant [776-output-enforcement.md](776-output-enforcement.md) will test.

### Requirement 2: `runSearch` on the shared table

The hand-rolled table at `commands.go:525-556` is deleted and the result rendered by `-o table` through
`TableFormatter`. Because that formatter is rune-aligned, this is where
[#741](https://github.com/NobleFactor/devlore-cli/issues/741) is fixed — but #741 stays its own issue and
is closed by this work only if the fix is verified, not assumed. A user-visible defect is not buried in a
refactor.

### Requirement 3: Three hand-rolled flag sets go

| Command | Today | Becomes |
| --- | --- | --- |
| `bundle` | `--output, -o <path>` | the destination is a positional or `--dest`; `-o` is the shared rendering |
| `onboard` | `--output <dir>` + `--format` | the destination is `--dest`; `--format` is the shared `-o` |
| `list` | `--format table\|manifest\|json` | the shared `-o`; `manifest` is a domain rendering and does not join the shared set |

`lore list` is a stub returning "not yet implemented", so it adapts at no cost.

### Requirement 4: Thirteen `fmt.Print` triaged

Each call is classified — narration to `cli.*` on stderr, results to the sink — and the classification
recorded in the commit, one line per call, so a reviewer can disagree with a specific one.

## Implementation Phases

### Phase 1: The root, and `inspect` (status: not started)

- [ ] `AddOutputFlags` on the root; removed from `inspect`
- [ ] Test: every `lore` subcommand accepts `-o`, `--jq`, `--filter`, `--store`

### Phase 2: `runSearch` (status: not started)

- [ ] The hand-rolled table deleted; the result rendered through `TableFormatter`
- [ ] Test: a search result with a multi-byte description renders without cutting a rune — this is the
      #741 verification, and it is written to fail on the current tree first
- [ ] #741 closed by this commit if and only if that test is green

### Phase 3: The three flag sets (status: not started)

- [ ] `bundle`'s `--output` becomes a destination flag that does not collide with `-o`
- [ ] `onboard`'s `--output` and `--format` likewise
- [ ] `list`'s `--format` retired; `manifest` handled as a domain rendering outside the shared set

### Phase 4: The thirteen calls (status: not started)

- [ ] Each `fmt.Print` classified and routed
- [ ] Test: `lore <cmd> -o none` emits nothing on stdout, for every command
- [ ] `10-command-line-interface.status.md` and 740's status updated — the lore row goes green

**Files**:

- `cmd/lore/commands.go` - Modify
- `cmd/lore/main.go` (or wherever the root is built) - Modify

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Every `lore` subcommand accepts the common set | unit | a leaf does not inherit from the root |
| 2 | `lore search -o table` renders through `TableFormatter` | unit | the hand-rolled table survives |
| 3 | A multi-byte description is not cut mid-rune | unit | byte-count truncation returns — this is #741 |
| 4 | No `lore` command registers `--output`, `--format` or `--json` | unit | a hand-rolled flag is re-added |
| 5 | `lore <cmd> -o none` writes nothing to stdout | unit | a `fmt.Print` reaches stdout |

**Write the failing test first.** Row 3 is red on the current tree by construction, and it is the test
that decides whether #741 closes.

**Not covered:** whether `lore list` should default to `table`. That is the epic's open question about
exceptions to the json-always default, and this plan does not settle it.

## Migration Path

`lore bundle --output <path>` and `lore onboard --output <dir>` change meaning: `-o` becomes the shared
rendering and the destination moves. Greenfield, no aliases, but this one is user-visible and belongs in
the release note.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/lore/commands.go` | Modify | `runSearch`, the flag sets, the thirteen calls |
| `cmd/lore/` root | Modify | `AddOutputFlags` |
| `docs/guides/lore/deploy-packages.md` | Modify | teaches `lore list --format json` and `lore inspect kubectl --format yaml` (:112, :172) |
| `docs/guides/lore/registry.md` | Modify | teaches `lore inspect kubectl --format yaml` (:41) |

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1, the epic's plan
- [774-writ-reconcile.md](774-writ-reconcile.md) — the item before this one
- [776-output-enforcement.md](776-output-enforcement.md) — the tests that certify this
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) — the convention
- Issue [#775](https://github.com/NobleFactor/devlore-cli/issues/775)
- Issue [#741](https://github.com/NobleFactor/devlore-cli/issues/741) — closed here only if verified

## Open Questions

- [ ] Does `lore list` keep a `table` default? The epic records the question; this plan carries it.
