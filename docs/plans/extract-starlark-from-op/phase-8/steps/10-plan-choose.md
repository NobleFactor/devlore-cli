---
step: 10
title: "plan.choose — a conditional subgraph probed sequentially; first matching when wins"
former_step: 13
former_title: "plan.choose initial redesign (superseded; successor open)"
status: open — current implementation is a value-picker, not the goal
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 10 — plan.choose (formerly step 13)

**Status:** `open` · the current implementation does not match the goal. 13 tests pass, but they verify a value-picker, not the conditional.

## THE GOAL (what plan.choose must be)

**`plan.choose` constructs a subgraph from its case statements — each a `when` / `then` clause. At execution the subgraph
is probed sequentially: the cases' `when` clauses are evaluated in order, and the FIRST matching (truthy) `when` STOPS
the probe. Its `then` clause is executed, that result is the choose's return value, and the choose ends — no later
`when` is evaluated and no non-matching `then` runs. `flow.Provider.Choose` simply EXECUTES the subgraph that
`plan.choose` built.**

The conditional and short-circuit semantics live in the **subgraph topology** (the cases are children, probed in order),
not in a method body. A side-effecting `when` or `then` in an unchosen/after-the-match branch **must not run**.

## What we have right now (and how it differs)

Not that. The current implementation is a **value-picker**:

- `ChoosePlanner` → `planSubgraphFromParams` (`pkg/op/provider/flow/planners.go`) builds a `*op.Subgraph` bound to
  `flow.Choose`, but stores the cases as an **`ImmediateValue` slot — inert data**, with **no case-branch children**.
  (Contrast `GatherPlanner`, which *adopts* its `body=` invocations as children via `addBodyChildren`.)
- The `when`/`then` `plan.*` invocations are therefore **not** part of the choose. To run at all they must be **rooted
  separately** by the author — `test_choose_exists.star` does exactly this: `plan.assemble([…, exists_inv, …])` makes
  `exists_inv` its own top-level node (`unit_count == 4`). So every `when` producer runs **eagerly and unconditionally**.
- `flow.Choose` (`provider.go:106`) then iterates the inert cases and **reads** each `when` result from the stack
  (`resolveDispatchedValue`, `helpers.go:214`), returning the first truthy `then`. (Lambda fields are the one exception:
  invoked on demand, so they *do* short-circuit.)

**Net:** for `plan.*`-invocation cases — the real use — all branches execute; the choose only *picks*. The first-match
short-circuit the goal demands does not exist in the topology.

## Test matrix

13 tests pass; they encode the value-picker, not the goal.

| # | Tests | Prove | Grade |
|---|---|---|---|
| 1–10 | `TestChoose_*`, `TestChooseAction_*` (Go) | iterate-and-pick on literals + compensable shape | ✅ |
| 11–13 | `TestChooseExists` / `_NotExists` / `_Lambdas` / `_Literals` / `_Predicates` (`.star`) | the value-picker with externally-rooted producers, and lambda short-circuit | ✅ |
| — | `TestChoose_UnchosenInvocationBranchDoesNotRun` | **THE GOAL** — a side-effecting unchosen `when`/`then` must not execute | ❌ unbuilt; current design cannot pass |

## To reach the goal

1. `ChoosePlanner` adopts the cases' `when`/`then` clauses as **children of the choose subgraph** (like `GatherPlanner`
   adopts `body=`), not inert `ImmediateValue` data.
2. The choose subgraph's executor **probes** the `when` children in order, **short-circuits** on the first truthy one,
   executes only that case's `then`, and returns — leaving later `when`s and all non-matching `then`s unexecuted.
3. `flow.Choose` becomes a thin executor of that subgraph (no value-picking loop).
4. Add `TestChoose_UnchosenInvocationBranchDoesNotRun` (a `when`/`then` with an observable side effect that must be
   absent for unchosen branches) — the test the current design cannot pass.
