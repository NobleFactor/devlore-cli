# Phase 5: Comprehensive Action Testing

```yaml
title: "Phase 5: Comprehensive Action Testing"
issue: TBD
status: complete
created: 2026-02-16
updated: 2026-02-16
```

## Context

Phases 1–4 restructured the execution layer into the Resource-Provider model
with 31 provider actions + 3 flow actions. The architecture docs are current.
But only 12 of 34 actions had direct Do() tests, there were no flow action
tests, and the graph lifecycle (build → save → load → run) had never been
tested end-to-end. Phase 5 closes these gaps.

## Scope

| Category | Items | Strategy |
|---|---|---|
| Missing provider Do() tests | 19 actions | Real filesystem for file ops, dry-run for external tools |
| Flow action tests | 3 actions | Stub verification + graph integration |
| Graph.Hydrate(reg) | 1 new method | Replace stubActions with real actions from registry |
| Graph lifecycle tests | build/save/load/hydrate/run | End-to-end with YAML/JSON round-trip |

## Changes

### Graph.Hydrate(reg)

**File:** `internal/execution/graph.go`

New method that walks graph nodes and replaces stubActions with real actions
from the registry. This enables loaded/deserialized graphs to be executed.

### File action tests (move, mkdir, source)

**File:** `internal/execution/execution_test.go`

Added Do() tests for the 3 untested file actions using `t.TempDir()` with
real filesystem (same pattern as existing file tests):

- `TestMoveOperation` — source gone, target exists
- `TestMoveOperationCreatesParentDirs` — parent dirs created
- `TestMkdirOperation` — directory created with mode
- `TestMkdirOperationDefaultMode` — default 0755
- `TestSourceOperation` — result contains file content

### External-tool action tests (dry-run)

**File:** `internal/execution/provider_test.go` (new)

Dry-run tests for all actions that call external tools. Each test verifies
slot parsing, dry-run logging, and empty/missing slot errors:

- pkg: install, upgrade, remove, update
- shell: exec (including empty command error), powershell
- service: start, stop, restart, enable, disable (including empty name error)
- git: clone (including empty URL error), checkout, pull

### In-memory / testable action tests

**File:** `internal/execution/provider_test.go` (same file)

Tests with real execution (no external tools needed):

- content.literal — verify Result = []byte, dry-run log
- net.download — httptest.NewServer, to-file, dry-run
- archive.extract — tar.gz and zip with t.TempDir(), dry-run

### Flow action tests

**File:** `internal/execution/flow_test.go` (new)

- Do() and Undo() for all 3 flow actions (choose, gather, elevate)
- Name() verification for all 3
- Gather integration: graph with 3 predecessors → gather → successor

### Graph lifecycle tests

**File:** `internal/execution/lifecycle_test.go` (new)

Full graph lifecycle: build → save → load → hydrate → run.

- Build programmatically, verify structure
- Serialize/deserialize YAML and JSON
- Round-trip YAML and JSON
- Hydrate with real registry, verify actions work
- Hydrate unknown action returns error
- Full lifecycle: build → serialize → deserialize → hydrate → run → verify files
- Pipeline lifecycle: source → render → copy with promise slots

## Test Count

52 new test functions. After Phase 5: all 34 actions have Do() tests, graph
lifecycle is verified end-to-end.

## File Summary

| File | Change |
|---|---|
| `internal/execution/graph.go` | Add `Graph.Hydrate(reg)` method (~15 lines) |
| `internal/execution/execution_test.go` | Add 5 file action tests (move, mkdir, source) |
| `internal/execution/provider_test.go` | **New**: 25 tests for pkg, shell, service, git, content, net, archive |
| `internal/execution/flow_test.go` | **New**: 11 tests for flow actions |
| `internal/execution/lifecycle_test.go` | **New**: 11 tests for graph build/save/load/hydrate/run |
| `docs/plans/resource-provider.md` | Update Phase 5 → Completed |
| `docs/plans/resource-provider/phase-5.md` | **New**: this file |
