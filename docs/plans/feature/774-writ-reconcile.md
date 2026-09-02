---
title: "writ reconcile: the command is renamed, and its renderings go through the sink"
issue: https://github.com/NobleFactor/devlore-cli/issues/774
status: draft
created: 2026-09-01
updated: 2026-09-01
---

# Plan: `writ reconcile`

## Summary

`writ status` is retired and `writ reconcile` takes its place. The 30 stdout call sites in writ route
through the sink, and the five globals the root already registers are actually consumed. This is one
piece of work, not a rename followed by a refactor: the renderings being rewritten live in the package
being renamed, so doing them separately would mean touching the same 22 lines twice.

This is the first item of thread 1 ([#740](https://github.com/NobleFactor/devlore-cli/issues/740),
[740-cli-output-conventions.md](740-cli-output-conventions.md)) and phase 2 of thread 3
([#762](https://github.com/NobleFactor/devlore-cli/issues/762),
[762-lifecycle-scopes.md](762-lifecycle-scopes.md)). Both plans record that these are the same work.

## Goals

1. **The command is `writ reconcile`**, and `writ status` stops existing. No alias.
2. **Every byte writ writes to stdout goes through the sink**, so `-o`, `--jq`, `--filter` and the
   result/narration split apply to all of it.
3. **The globals are consumed, not merely registered.** A flag on a root that no leaf reads is worse
   than an absent one, because it looks like compliance.

## The settled surface

Settled in [#772](https://github.com/NobleFactor/devlore-cli/issues/772) and recorded in
[772-reconcile-surface.md](../doc/772-reconcile-surface.md):

`writ reconcile` **produces a report.** It has **no command flags.** It takes the globals — `--output` /
`-o`, `--filter`, `--jq`, `--store`, `--dry-run` — and must utilize all of them faithfully. The selector
between fix, diff and summary behaviors is deferred to the reconciliation epic.

The report is one JSON document: `entries[]` (the delta, eight states, each naming its `Repair`),
`layers[]`, `packages[]`, `health`.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| The command | ✅ `writ reconcile` | `cmd/writ/writ/reconcile/`, renamed with history under phase 1 |
| `--json` boolean | ✅ retired | done in #740 phase 3a |
| `-o` rendering | ✅ all eight | phase 2: the report is the result; the presenters are deleted |
| `-o` validation | ✅ | `PersistentPreRunE`, #754 |
| `--store` read | ✅ | resolved in `PersistentPreRunE`, #753 |
| `--jq` / `--filter` reaching the pipeline | ✅ | phase 2: `emitResult` builds the pipeline from the bound options |
| `--dry-run` | ✅ | phase 3: the plan is the result; `Execute` returns it and the pipeline renders it |

The 30 stdout call sites, from 740's measurement:

| Location | Calls | What it is |
| --- | --- | --- |
| `cmd/writ/writ/status/report.go` | 22 `fmt.Print*` | the human report |
| `deploy`, `decommission`, `upgrade`, `secret` | 4 `SerializeGraphs(os.Stdout, …)` | the dry-run plan dump |
| `cmd/writ/writ/migrate/session.go` | 1 `os.Stdout` | TUI session output |
| `cmd/writ/writ/migrate_cmd.go` | 1 `os.Stdout` | `FormatMigrationPlan` |
| `cmd/writ/writ/verify/verify.go` | 2 `fmt.Print*` | already removed with `presentReport` |

## Requirements

### Requirement 1: The rename, with history

`git mv cmd/writ/writ/status cmd/writ/writ/reconcile` and `status.go` → `reconcile.go`, so `git log
--follow` works. `status.Report` → `reconcile.Report` and the rest of the package surface. The scenario
tests and every plan document naming `writ status` follow.

### Requirement 2: The report is a value, and the sink renders it

`reconcile.Report` is returned to the command and handed to `BuildPipeline`. The 22 `fmt.Print*` calls
that hand-format it are deleted, not routed — the human rendering is `-o table` or `-o list` applied to
the JSON, per the two-stage model in
[10-command-line-interface.md](../../architecture/10-command-line-interface.md) §7.

### Requirement 3: The dry-run dumps are a result

`SerializeGraphs(os.Stdout, …)` in four commands is the dry-run output. Under `--dry-run` the graph *is*
the result, so it goes to the sink and renders by `-o` like any other. `-o yaml` on a dry run is then a
parseable plan rather than a dump.

### Requirement 4: `migrate` joins the convention

`migrate`'s own `--format json|yaml|text` is retired in favor of `-o`. `FormatMigrationPlan` returns a
value. `session.go`'s one write is classified — narration if it is progress, result if it is output —
and routed accordingly.

### Requirement 5: The globals are consumed

Each global is exercised end-to-end and its effect observed, not assumed from registration:

| Global | Observed effect |
| --- | --- |
| `-o <fmt>` | every one of the eight formats produces its format |
| `--jq '.entries'` | the delta alone |
| `--filter` | the report filtered |
| `--store <dir>` | the report reads that store's runs |
| `--dry-run` | the graph, not the mutation |

## Implementation Phases

### Phase 1: The rename (status: complete)

- [x] `git mv cmd/writ/writ/status cmd/writ/writ/reconcile`; `status.go` → `reconcile.go`
- [x] `status.Report` → `reconcile.Report`; the package surface follows
- [x] `writ status` → `writ reconcile` in the scenario tests
- [x] Every plan document naming `writ status` as live — the eight sites in 740 already changed under
      #772; the remaining plan documents are found by search and corrected

### Phase 2: The report through the sink (status: complete)

- [x] `reconcile.Report` returned to the command, handed to `BuildPipeline` — `BuildReport` is the entry;
      `Execute` and `Config.JSON` are deleted, and with them the one-bool bridge at `config.go:160`
- [x] The 22 `fmt.Print*` calls deleted, along with `State.String()`, the glyph they rendered; the help
      text's "Entry states" now lists the labels JSON emits rather than glyphs no output carries
- [x] Test: every format, at two levels. `TestReport_EveryFormatRenders` (unit, in the package) pins the
      Report's shape under the shared pipeline — State marshals as its label, the four sections survive
      normalization. `TestWritDeployScenario_Deploy` (scenario, `make test-scenario`) runs the real binary
      through all seven non-JSON formats and fails on the human dashboard or on silence.

**On "write the failing test first."** The scenario assertion is red on the tree before this phase by
construction: `presentText`'s first statement was `fmt.Println("Layers:")`, and the assertion fails on
exactly that string. It was written and the wiring changed in one pass, so red was argued rather than
observed. Recorded so the claim is the right size.

### Phase 3: Dry-run dumps and migrate (status: complete)

- [x] `SerializeGraphs(os.Stdout, …)` × 4 → the graph is the result under `--dry-run`. Each `Execute`
      returns `([]*op.Graph, error)` — the plan under `--dry-run`, nil otherwise — following
      `verify.Execute`, and the command hands a non-nil plan to `emitResult`. No new helper: `Graph` has
      its own `MarshalJSON`/`MarshalYAML`, so the value's document form is `SerializeGraphs`'s, as a JSON
      array or YAML sequence rather than a multi-document stream. 28 test call sites now discard the plan
      they never asserted on; two dry-run tests assert it comes back.
- [x] `migrate --format` retired; `FormatMigrationPlan` and its eleven text helpers deleted;
      `NewMigrationView` builds the value the pipeline renders. `Options.Format` was read by nothing and
      goes with it.
- [x] `migrate/session.go`'s write is a result — the session's analysis and graph at completion — and
      is emitted through an `Options.Emit` hook the command supplies, since the session has no writer.
- [x] Test: `TestWritDeployScenario_Deploy` runs `deploy --dry-run -o json` against the real binary and
      requires a non-empty JSON array of graphs — red before, when `-o json` yielded YAML.

**Found on the way, logged where each belongs.** `manage-environments.md` teaches `writ inspect --format`,
a command that does not exist — [#777](https://github.com/NobleFactor/devlore-cli/issues/777), not this
work. Two `lore` guides teach `--format` — recorded in [775-lore-adoption.md](775-lore-adoption.md)'s
file list, resolved there.

### Phase 4: The globals proved (status: not started)

- [ ] One test per global, observing its effect rather than its registration
- [ ] `10-command-line-interface.status.md` and 740's status updated — the writ row goes green
- [ ] `docs/cli` regenerated

**Files**:

- `cmd/writ/writ/status/` → `cmd/writ/writ/reconcile/` - Rename
- `cmd/writ/writ/reconcile/report.go` - Modify
- `cmd/writ/writ/{deploy,decommission,upgrade,secret}/` - Modify
- `cmd/writ/writ/migrate/session.go`, `cmd/writ/writ/migrate_cmd.go` - Modify
- `cmd/writ/writ/config.go` - Modify (the one-bool bridge at `:160` goes)

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `writ status` does not exist | unit | an alias or the old command survives |
| 2 | Each of eight `-o` values renders its format on `writ reconcile` | unit + scenario | the report bypasses the pipeline |
| 3 | `--jq '.entries'` returns the delta alone | unit | `--jq` never reaches the pipeline |
| 4 | `--store <dir>` reads that store | unit | `readback` folds the default store regardless |
| 5 | `--dry-run` on `deploy` yields the graph, rendered by `-o` | unit | `SerializeGraphs` still writes stdout |
| 6 | No `fmt.Print*` remains in `cmd/writ/writ/reconcile` | unit | one is re-added |
| 7 | The scenario suite passes with the new name on all five platforms | scenario | a fixture still invokes `writ status` |

**Write the failing test first.** Rows 2–5 are red on the current tree by construction.

**Not covered:** the S4 rendering question below. `-o table` on a sectioned object will render one row
of four cells, which is correct per §7 and useless to a human. Whether the presenters need a case for
that shape is a design question, and it is left open rather than answered in passing.

## Migration Path

Greenfield. `writ status` stops existing; `migrate --format` stops existing. No aliases. Release note
only.

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `cmd/writ/writ/status/` | Rename → `reconcile/` | the command |
| `cmd/writ/writ/reconcile/report.go` | Modify | 22 `fmt.Print*` deleted; `Report` returned |
| `cmd/writ/writ/deploy/`, `decommission/`, `upgrade/`, `secret/` | Modify | dry-run dumps become results |
| `cmd/writ/writ/migrate/session.go` | Modify | one write classified |
| `cmd/writ/writ/migrate_cmd.go` | Modify | `--format` retired; a value returned |
| `cmd/writ/writ/config.go` | Modify | the one-bool bridge goes |
| `cmd/devlore-test/devloretest/data/*.star` | Modify | fixtures naming `writ status` |

## Related Documents

- [740-cli-output-conventions.md](740-cli-output-conventions.md) — thread 1, the epic's plan
- [762-lifecycle-scopes.md](762-lifecycle-scopes.md) — thread 3; this is its phase 2
- [772-reconcile-surface.md](../doc/772-reconcile-surface.md) — the surface ruling
- [775-lore-adoption.md](775-lore-adoption.md) — the next item in thread 1
- [776-output-enforcement.md](776-output-enforcement.md) — the tests that certify this
- [5.1-reconciliation.md](../../architecture/5.1-reconciliation.md) — the design of record
- [10-command-line-interface.md](../../architecture/10-command-line-interface.md) — the convention
- Issue [#774](https://github.com/NobleFactor/devlore-cli/issues/774)

## Open Questions

- [ ] `Report` is shape S4 — three slices and a struct. `-o list` renders four lines of compact JSON,
      `-o table` one row of four cells. Legible for `list`, useless for `table`. Do the presenters need a
      case for a sectioned object, or is `--jq '.entries' -o table` the answer and the docs say so?
