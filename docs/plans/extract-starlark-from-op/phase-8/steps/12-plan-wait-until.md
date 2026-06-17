---
step: 12
title: "plan.wait_until — re-evaluate a predicate-container subgraph each poll until truthy or timeout"
former_step: 15
former_title: "plan.wait_until redesign (not-started; direction TBD pending step 13)"
status: incomplete — lambda-polling impl, zero behavioral tests, container goal unmet
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 12 — plan.wait_until (formerly step 15)

**Status:** `incomplete` · the polling loop exists and is reachable, but **zero behavioral tests** prove it, **no `.star`
exercises it**, and the implementation **does not match the defined container goal**. Worse than choose (step 10): choose
at least has 13 tests against its value-picker; wait_until has none against its lambda-polling method.

## THE GOAL (defined in the design section, lines 235 / 266 / 296 / 337 / 724)

`plan.wait_until(predicate=<invocation>, timeout=…, interval=…)` — the **predicate is a container subgraph**, owning its
member invocation(s) as args. At execution the predicate subgraph is **re-evaluated (re-dispatched) each poll** until it
is truthy (return its final value) or the timeout elapses (error). Representative shape:

```python
plan.wait_until(predicate=plan.service.is_healthy(svc="db"), timeout="5m", interval="10s")
```

- Output type: the predicate's return type (line 337).
- Error contract: a missing predicate fails at plan time — `"wait_until requires a predicate invocation"` (line 724).
- `Target` is the container's subgraph (line 296) — i.e. the predicate IS the evaluated unit; there is no separate target.

## What we have right now (and how it differs)

- `flow.Provider.WaitUntil` (`pkg/op/provider/flow/provider.go:429`) — signature
  `WaitUntil(target any, predicate func(any) (bool, error), timeout, interval time.Duration) (any, error)`. A correct
  **polling loop**: timeout required, default interval 5s, immediate predicate check, then ticker; honors
  `Context.Done()` and the deadline; returns `target` on match. **Not compensable** (`(any, error)`, no `RecoveryStack`).
- `WaitUntilPlanner` (`planners.go:364`) — projects each kwarg through `projectKwargValue` into a **slot**; builds a
  one-node subgraph. It does **not** adopt the predicate as a re-dispatchable child (no gather-style `addBodyChildren`).
- **Mismatch with the goal:**
  1. The predicate is a Go **lambda** (`func(any)(bool,error)`), not an invocation/container subgraph. An invocation
     predicate projects to a single `PromiseValue` (one pre-computed result), which the slot model cannot re-evaluate —
     so the goal's per-poll re-dispatch is unrepresentable here.
  2. The shape is two params (`target` + `predicate`); the goal is a single `predicate=<invocation>`.

## Test matrix

| # | Test | Proves | Grade |
|---|---|---|---|
| 1 | `TestWaitUntilAction_DryRun` (`gen/action.gen_test.go:189`) | the generated dry-run path | ✅ |
| — | `TestWaitUntil_ImmediateMatch` | predicate already true → returns target without polling | ☐ |
| — | `TestWaitUntil_MatchAfterTicks` | predicate true on the Nth poll → returns target | ☐ |
| — | `TestWaitUntil_Timeout` | predicate never true → timeout error | ☐ |
| — | `TestWaitUntil_ContextCancel` | context cancellation → `Context.Err()` | ☐ |
| — | `TestWaitUntil_PredicateError` | predicate error → wrapped and propagated | ☐ |
| — | `test_wait_until_*.star` | the combinator is callable + correct end-to-end through the bridge | ☐ |

**Behavioral coverage: 0.** No test exercises the polling loop's five branches; no `.star` exercises the combinator.

## Proof run

```
$ go test ./pkg/op/provider/flow/... ./cmd/devlore-test/devloretest/ -run 'WaitUntil'
ok ...flow [no tests to run]   ok ...flow/gen (DryRun)   ok ...devloretest [no tests to run]
$ find . -name '*.star' -exec grep -l wait_until {} \;     # (none)
```

## To reach `complete`

1. Decide and land the goal shape: `WaitUntilPlanner` adopts the predicate **invocation as a re-dispatchable child
   subgraph** (like `GatherPlanner` adopts `body=`); `flow.WaitUntil` re-dispatches that subgraph each poll and tests
   its result for truthiness; drop the separate `target` + the lambda `predicate`.
2. Write the five behavioral Go tests (immediate-match / match-after-ticks / timeout / cancel / predicate-error).
3. Add a `test_wait_until_*.star` fixture proving the combinator end-to-end.

Until the goal shape is settled, this step is blocked the same way choose (step 10) is — the conditional/polling control
flow belongs in the subgraph topology, not in a lambda re-invoked by a method body.
