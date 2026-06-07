---
title: "Phase 8 · subgraph result flow — do executors output results, or toss them?"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-lore-migration.md"
issue: TBD
status: complete
created: 2026-06-06
updated: 2026-06-06
---

# Subgraph result flow

## Concern

A subgraph executor appears to **toss** the result of the units it runs instead of **outputting** it. The intended
semantic (see the execution-result-semantics note) is that a structural subgraph's result is its terminal / final
unit's return value, which **bubbles up**: terminal node → its containing subgraph → the root subgraph → the value
the graph executor returns. If subgraph executors discard child results, that chain is broken and a graph's result
is lost.

Raised 2026-06-06 while finishing #9 (package-index update).

## Task #10 — investigate

Confirm or refute the claim with file:line evidence. Read:
- `pkg/op/subgraph.go` — `Subgraph.Execute`, especially the structural-container dispatch path (a subgraph with no
  bound action that walks its children): what does it do with each child's return value, and what does it return?
- the `GraphExecutor` — how `Graph.Execute`/the run loop captures and returns the root subgraph's result.
- `pkg/op/provider/flow/provider.go` — `flow.Provider.Completed` (and the other terminals): what value does a
  `Completed` node return, and is that value preserved up the call chain?

Deliverable: a precise statement of where a subgraph's result goes today and, if it is dropped, the minimal fix to
bubble the terminal/final unit's return up to the executor's return.

### Findings (2026-06-06) — CONFIRMED, localized to the `flow.Subgraph` verb

The toss is real, but path-specific. The `flow.Subgraph` verb drops its children's output; every other path is
correct.

- **`flow.Provider.Subgraph`** (`pkg/op/provider/flow/provider.go:350-385`) dispatches each child via
  `dispatchWithRetry` and **`return nil, stack, nil`** (`:384`) — it keeps no `lastResult`.
- **`dispatchWithRetry`** (`pkg/op/provider/flow/helpers.go:135`) is typed `… error`: it calls `DispatchChild`
  internally and **discards the result**, returning only an error. So the result is dropped twice.

Correct elsewhere (so a *structural* subgraph of a `Complete` node already works):
- structural `op.Subgraph.Execute` returns `lastResult` (`pkg/op/subgraph.go:237`);
- `GraphExecutor.Run` returns the root's result (`pkg/op/graph_executor.go:281`);
- `flow.Provider.Complete` returns its `output` (`pkg/op/provider/flow/provider.go:494`).

**Fix** (mirror the structural path): (1) change `dispatchWithRetry` to return `(any, error)`, capturing
`DispatchChild`'s result; (2) in `flow.Provider.Subgraph`, keep `lastResult` across the children loop and
`return lastResult, stack, nil` instead of `nil`.

**Bearing on #11:** a subgraph built via the **`flow.Subgraph` verb** (the Starlark / lore path) exercises the buggy
path and will **fail until the fix lands**; a bare **structural** `op.NewSubgraph` (Go API) already bubbles up and
**passes**. So the Starlark test catches this; the Go test catches it only if it routes through the `flow.Subgraph`
verb rather than a plain structural subgraph.

## Task #11 — verification tests (the guard)

Two tests proving the result flows end to end. Both build: a graph whose root contains a **subgraph** that contains
a single **`flow.Provider.Completed`** node, then execute the graph and assert the executor's returned result equals
the value `flow.Provider.Completed` returns — and that this value flows `Completed` → containing subgraph → root
subgraph → graph-executor return.

1. **Go API** — construct the graph with the Go construction API (specs / `NewGraph` / `NewSubgraph` / `NewNode`).
2. **Starlark API** — the same scenario and assertions, but plan it through the Starlark bridge, not the Go API.

These fail if subgraph executors toss results and pass once the chain is intact, so they both prove the bug (if
present) and guard the fix.

## Resolution (2026-06-06) — fixed; Go + Starlark guards green

**Fix applied** (`pkg/op/provider/flow/`): `dispatchWithRetry` (`helpers.go`) now returns `(any, error)`, capturing
`DispatchChild`'s result; `flow.Provider.Subgraph` (`provider.go`) carries `lastResult` across the children loop and
returns it instead of `nil`. So a `flow.subgraph`-bound subgraph now bubbles its terminal child's result up to
`GraphExecutor.Run`.

**Test 1 (Go API) — done, RED→GREEN.** `TestSubgraphBoundAction_FlowsLeafResult` (`result_flow_test.go`) builds root
→ `flow.subgraph` subgraph → `flow.complete` leaf (output = a sentinel), runs it, asserts `Run` returns the sentinel.
It failed (`result = nil`) before the fix and passes after; `TestStructuralSubgraph_FlowsLeafResult` is a control
that passes throughout, isolating the harness from the bug. `flow/integration_test.go`'s inventory import was swapped
to `flow/gen` to free the flow test binary from the #6-blocked inventory.

**Test 2 (Starlark API) — done, green.** `TestSubgraphBoundAction_FlowsLeafResult_Starlark` plans the same
subgraph-of-complete through the Starlark bridge (`plan.subgraph` → `flow.subgraph` → `plan.run`) and asserts the
`result` global equals the sentinel. It was `t.Skip`'d pending **#12** (a nil `op.Origin` deref in
`plan.Provider.Assemble`, `plan/provider.go:176`, under the default `origin=`); #12 is now fixed (origin defaults to the
zero `OriginBase`), the guard is un-skipped, and it passes.

## Status

- 2026-06-06 — #10 complete: **confirmed** — `flow.Subgraph` (and `dispatchWithRetry`) toss results; the structural
  path, the executor, and `flow.Complete` are correct. Fix identified (see Findings). #11 tests + the fix pending.
- 2026-06-06 — **#11 fixed**: `dispatchWithRetry` / `flow.Provider.Subgraph` now bubble the result; the Go guard is
  green (RED→GREEN), the Starlark test is `t.Skip`'d pending #12 (`plan.Assemble` nil-origin panic). See Resolution.
- 2026-06-06 — **#12 fixed**: `plan.Provider.Assemble` defaults a nil `op.Origin` to the zero `OriginBase`; the #11
  Starlark guard is un-skipped and green. #10 / #11 fully closed.
