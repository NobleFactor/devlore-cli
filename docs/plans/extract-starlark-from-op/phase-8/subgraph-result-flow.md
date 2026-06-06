---
title: "Phase 8 · subgraph result flow — do executors output results, or toss them?"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-lore-migration.md"
issue: TBD
status: draft
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

## Task #11 — verification tests (the guard)

Two tests proving the result flows end to end. Both build: a graph whose root contains a **subgraph** that contains
a single **`flow.Provider.Completed`** node, then execute the graph and assert the executor's returned result equals
the value `flow.Provider.Completed` returns — and that this value flows `Completed` → containing subgraph → root
subgraph → graph-executor return.

1. **Go API** — construct the graph with the Go construction API (specs / `NewGraph` / `NewSubgraph` / `NewNode`).
2. **Starlark API** — the same scenario and assertions, but plan it through the Starlark bridge, not the Go API.

These fail if subgraph executors toss results and pass once the chain is intact, so they both prove the bug (if
present) and guard the fix.

## Status

- 2026-06-06 — queued. Investigation (#10) not started; tests (#11) not written.
