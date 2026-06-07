---
title: "Phase 8 · unify subgraph execution — every subgraph runs through flow.subgraph"
parent: "docs/plans/extract-starlark-from-op/phase-8/subgraph-result-flow.md"
issue: TBD
status: draft
created: 2026-06-06
updated: 2026-06-06
---

# Unify subgraph execution

## Goal

Every subgraph — **including the root** — executes through the `flow.subgraph` action, so result-bubbling, retry, and
`errorAction` live in exactly one place (the path #11 just fixed). The root stops being special: it is built by
`NewSubgraph` from a shared spec like every other subgraph, `newRootSubgraph` is deleted, and the `action == nil`
structural branch in `Subgraph.Execute` is removed.

## Motivation

The result-flow bug (#10/#11) existed because there are **three** child-walk implementations and one
(`flow.Provider.Subgraph`) tossed results:

1. the executor's `action == nil` structural path (`Subgraph.Execute`) — root + any structural subgraph;
2. `flow.Provider.Subgraph` (the bound action);
3. `dispatchBodyChildren` (gather).

The structural path duplicates what `flow.Subgraph` does. Collapsing to one execution path removes the duplication and
the entire class of bug — there is exactly one place to get bubbling / retry / `errorAction` right.

## Resolution model — actions referenced by name (decided)

A unit references its action **by name** (`"flow.subgraph"` — the real registry name; `plan.subgraph` is only the
Starlark-surface promotion of the same action). The concrete `Action` is resolved at dispatch through
`RuntimeEnvironment.ActionByName` (`runtime_environment.go:230`), which splits the dotted name, looks the receiver up in
the `ReceiverRegistry` (`receiver_registry.go:538`, the `byName` map populated by `AnnounceProvider`), and builds the
`Action`. `pkg/op` resolves a string; it never imports `flow`.

**Name beats a resolved `Action` at the call sites.** All three `newRootSubgraph` callers build the root from a bare
spec with no registry / runtime environment in scope:

- `NewGraph` (`graph.go:119`) — takes only `*GraphSpec`.
- `dependencyview.go:447` — `&SubgraphSpec{Name:"root", Children: children}`.
- `load.go:85` — `&SubgraphSpec{}` (deserialization).

Binding a resolved `Action` would force a `*ReceiverRegistry` into all three (and change `NewGraph`'s signature).
Binding a name is a plain string on the shared spec, resolved later at dispatch. So: store the name.

## Design

### 1. Action-by-name on specs, resolved at dispatch

**Binding principle (universal).** Every action lands on a unit one of two ways: a caller that *holds* a resolved
`Action` binds via `WithAction(action)`; a caller that only has a *name* binds via `WithActionNamed(name)`. This is the
single pattern for creating actions everywhere — the `flow.Provider.*` materializers (which hold the name `"flow.subgraph"`)
use `WithActionNamed`, but it works anywhere.

- `SubgraphSpec` / `NodeSpec` gain `WithActionNamed(name string)`; the unit stores the **name**.
- `WithActionNamed` **validates the name against the global receiver registry** (`globalReceiverRegistry`, populated by
  `AnnounceProvider`) and **panics if it does not resolve** — the failure happens *in* `WithActionNamed`, per directive.
  Panic (not an error return) fits the fluent `WithX` pattern (`WithAction` returns `*Spec`, no error); a non-existent or
  un-announced action name is a programming/config error, so fail fast and loud. The call site passes no registry (it
  uses the global), preserving why by-name was the straightforward choice.
- `Subgraph.Execute` / `Node.Execute` resolve the validated name lazily via `executor.environment.ActionByName` at
  dispatch. Planner-built units may keep carrying a pre-resolved `Action`; the executor handles both.
- `NewSubgraph`'s validation (today: non-nil action required — `validate_test.go:207`) accepts an action **by name** as
  satisfying the requirement, so the root is no longer a special case.

### 2. Kill `newRootSubgraph`; one shared root spec that names `flow.subgraph`

Delete `newRootSubgraph` (subgraph.go:97). Add a single canonical source — `NewRootSubgraphSpec()` in `pkg/op` —
returning a `*SubgraphSpec` with ID `"root"` and `WithActionNamed("flow.subgraph")` (plus any root invariants). The
three call sites switch to `NewSubgraph(rootSpec)` using it:

- `NewGraph` → `NewSubgraph(&spec.Root)` where `spec.Root` was seeded from `NewRootSubgraphSpec()`.
- `dependencyview.go:447` and `load.go:85` → the same shared spec.

Sharing the one spec **guarantees** every root is identical — no call site configures it independently, and there is no
flow import (the action is a name).

### 3. Root runs through `flow.subgraph`; delete the structural path

With the root naming `flow.subgraph` (resolved at dispatch), the `action == nil` branch in `Subgraph.Execute`
(subgraph.go:205-238) is dead — delete it. Every subgraph takes the bound-action path; the executor installs the
`dispatchChild` closure for the root too (already done on the bound path, subgraph.go:248).

**Consequence (follows from the directive):** there are **no structural subgraphs** — every subgraph is bound (by name
or by a resolved action). Audit and fix any action-less subgraph creation before the deletion lands.

### 4. Converge the child-walks

`flow.Provider.Subgraph` becomes the single children-walk. Fold `dispatchBodyChildren` (gather) and `flow.Subgraph`'s
inline loop into one helper parameterized by the variable frame (gather's per-iteration frame vs `activation.Variables`)
and retry, with `dispatchWithRetry` as the per-child primitive. Settle the gather per-child-retry asymmetry (gather's
body currently dispatches without per-child retry; flow.subgraph's children retry).

## Open decision — guaranteeing `flow.subgraph` is always announced

Binding the root to `flow.subgraph` makes the flow announcement **load-bearing**: today a structural root runs without
flow announced; afterward, `RuntimeEnvironment.ActionByName("flow.subgraph")` fails (`"unknown action provider: flow"`)
for any binary/test that didn't import an inventory — and even an empty graph can't start. Announcement is currently a
per-binary, per-test convention nothing enforces (each composition root blank-imports `pkg/op/inventory` plus its app
inventory; `pkg/op` can't, by layering). Two ways to address it — **undecided, to settle before/with this work**:

- **(a) Fail-loud guard.** `NewGraph` / `GraphExecutor` asserts the core actions it depends on are announced and returns
  a clear error otherwise (*"flow.subgraph not announced — import pkg/op/inventory"*). Cheap; makes the contract
  legible; does **not** guarantee — you can still forget the import.
- **(b) Relocate the engine-control verbs into core.** Treat `flow.subgraph` (and the terminals the engine depends on)
  as core primitives announced by `pkg/op`'s own `init()` rather than an optional provider. Then importing `pkg/op`
  announces them — nothing to forget. The only *structural* guarantee; cost is splitting `flow`.

## Namespace note

The action's real registry name is `flow.subgraph` (receiver type `Name() == "flow"`; the subgraph materializer is
"bound to flow.subgraph", `plan/helpers.go:274`). `flow` is a root-planned provider whose methods are **promoted flat
into the `plan.*` namespace** for Starlark authoring (`flow/provider.go:25`; `plan/helpers.go:19`) — so plan authors
write `plan.subgraph`, but the bound name we store is `flow.subgraph`.

## Blast radius

- `newRootSubgraph` + its three callers (`NewGraph` graph.go:119, dependencyview.go:447, load.go:85).
- `SubgraphSpec` / `NodeSpec`: `WithActionNamed` + name storage.
- `NewSubgraph` validation: accept action-by-name.
- `Subgraph.Execute` / `Node.Execute`: lazy resolve-by-name; delete the `action == nil` structural branch.
- `flow` provider: converge `dispatchBodyChildren` + `flow.Subgraph`'s loop; resolve the retry asymmetry.
- Audit: any other action-less subgraph creation (Go API call sites, the planner).
- Tests: `TestStructuralSubgraph_FlowsLeafResult` (#11) retargets — a structural subgraph is no longer a valid shape.

## Sequencing

1. Add `WithActionNamed` + lazy resolve-by-name in `Execute` (via `executor.environment`); accept action-by-name in
   `NewSubgraph` validation. Structural path still present.
2. Add `NewRootSubgraphSpec`; switch `NewGraph` / dependencyview / load to `NewSubgraph` naming `flow.subgraph`. Verify
   graphs run (root through flow.subgraph) with the #11 guard + existing `pkg/op` / flow tests.
3. Settle and apply the announcement guarantee (decision above).
4. Delete the `action == nil` structural path; audit and fix any action-less subgraph creation.
5. Converge the child-walks; settle the gather retry decision.
6. Remove `newRootSubgraph`. Retarget the structural control test; add root-via-flow.subgraph coverage.

## Verification

- `pkg/op` + `pkg/op/provider/flow` tests green; the #11 guard green (now the *only* path).
- A graph with a bare node directly under the root still returns its result (root via flow.subgraph).
- No `action == nil` code path and no `newRootSubgraph` remain.
- A binary/test missing the flow announcement fails per the chosen guarantee (clear error, or impossible by §3b).

## Status

- 2026-06-06 — draft.
- 2026-06-06 — **foundation landed (steps 1–2 + `newRootSubgraph` removal)**, verified green (`pkg/op` + `flow`):
  `WithActionNamed` (validate via global registry, panic on miss, store name); `Execute` resolves by name at dispatch;
  `NewRootSubgraphSpec()` names `flow.subgraph`; the three call sites build the root via `NewSubgraph`; `newRootSubgraph`
  deleted. Side effects handled: `receipt.go` guards nil `Action()` for by-name units; a `flow_announce_test.go`
  announces flow into the `pkg/op` test binary. The `action == nil` structural path is **kept** as a fallback.
  Remaining: step 4 (audit action-less subgraphs, then delete the structural path), step 5 (converge the child-walks).
