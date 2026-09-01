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
| `-o` rendering | ❌ one of eight | measured 2026-08-30: everything but `json` prints the human dashboard |
| `-o` validation | ✅ | `PersistentPreRunE`, #754 |
| `--store` read | ✅ | resolved in `PersistentPreRunE`, #753 |
| `--jq` / `--filter` reaching the pipeline | ❌ | they never reach `FormatterByName` because the report bypasses it |
| `--dry-run` | ❌ unverified | four `SerializeGraphs(os.Stdout, …)` dumps are the dry-run output, unrouted |

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

### Phase 2: The report through the sink (status: not started)

- [ ] `reconcile.Report` returned to the command, handed to `BuildPipeline`
- [ ] The 22 `fmt.Print*` calls deleted
- [ ] Test: `-o json`, `-o yaml`, `-o list`, `-o table`, `-o csv`, `-o value`, `-o none`, `-o template=`
      each produce their format — eight rows, eight assertions

### Phase 3: Dry-run dumps and migrate (status: not started)

- [ ] `SerializeGraphs(os.Stdout, …)` × 4 → the graph is the result under `--dry-run`
- [ ] `migrate --format` retired; `FormatMigrationPlan` returns a value
- [ ] `migrate/session.go`'s write classified and routed

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
| 2 | Each of eight `-o` values renders its format on `writ reconcile` | unit | the report bypasses the pipeline |
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
