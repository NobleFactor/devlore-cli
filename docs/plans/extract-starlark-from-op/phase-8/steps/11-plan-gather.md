---
step: 11
title: "plan.gather — concurrent per-item dispatch of the body subgraph, bounded concurrency, per-dispatch frame minting"
former_step: 14
former_title: "plan.gather redesign (not-started; direction TBD pending step 13)"
status: complete — core proven; compensation-unwind path under-tested
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 11 — plan.gather (formerly step 14)

**Status:** `complete` (core) · **7 tests pass, ~16 `.star` sub-scenarios** · the iteration combinator already reached
the subgraph-child goal that `plan.choose` (step 10) has not. One flagged gap: the compensation-unwind path.

**Row drift:** phase-8.md marks this `not-started — direction TBD pending successor redesign for step 13`. Stale. The
redesign landed (Phase 5 of 13.0(n), the ForEach-Object shape) and is thoroughly proven; it does not wait on choose.

## What this step delivers

`plan.gather` — `GatherPlanner` (`pkg/op/provider/flow/planners.go:115`) materializes a `*op.Subgraph` bound to
`flow.Gather`, **adopting `body=` invocations as iteration-template children** via `addBodyChildren` (the right
pattern — exactly what `ChoosePlanner` fails to do). `flow.Gather` (`provider.go:181`) then dispatches that one
materialized body subgraph **once per item, concurrently**:

- **Per-dispatch frame minting** — each goroutine gets `buildIterationFrame(activation.Variables, item)`, a fresh frame
  binding `item` over the inherited variables. The body subgraph is *not* duplicated per iteration (goroutine-safe
  template, per the execution-units contract).
- **Bounded concurrency** — a `limit`-sized semaphore; `limit<=0` falls back to `Platform.DefaultConcurrency()`.
- **Result collection by index** — `results[c.index] = c.result`; returns the ordered slice.
- **Failure semantics** — first iteration error cancels the shared `gatherCtx` and, after `wg.Wait()`, unwinds the
  completed iterations' recovery stacks **LIFO**, joining compensation errors.
- **Per-iteration recovery** — each iteration's `*op.RecoveryStack` is nested into one returned `gathered` stack;
  `CompensateGather` (`:299`) unwinds them LIFO on parent rollback.

## Test matrix

Legend — ✅ pass · ❌ fail · ☐ to write.

| # | Test | Proves | Grade |
|---|---|---|---|
| 1 | `test_gather_basic.star` (6 cases) | per-item dispatch, `item`→variable binding, bounded concurrency, empty-items short-circuit + downstream canary, empty-body no-op | ✅ |
| 2 | `test_gather_concurrency.star` (6 cases) | every limit mode — serial(1), parallel(2), over-count(100), default(0→`DefaultConcurrency`), batching(10/3), isolation(6/4) | ✅ |
| 3 | `test_gather_advanced.star` (4 cases) | frame sees `item`+outer flags; frame hygiene (`items` not leaked); multi-node body (`write`+`shell.exec`); sequencing with non-gather siblings | ✅ |
| 4 | `TestCompensateGather_NilStack_NoOp` (`provider_test.go:161`) | compensate no-ops on a nil stack | ✅ |
| 5 | `TestCompensateGather_EmptyStack_NoOp` (`:170`) | compensate no-ops on an empty stack | ✅ |
| 6 | `TestGatherAction_DryRun` (`gen/action.gen_test.go:143`) | dry-run path | ✅ |
| 7 | `TestGatherAction_CompensableInterface` (`gen:217`) | generated action satisfies the compensable interface | ✅ |
| — | `TestGather_FailureUnwindsCompletedIterations` | **GAP** — a mid-flight iteration error cancels the rest AND LIFO-unwinds the already-completed iterations' side effects | ☐ |

**Coverage:** the core combinator (concurrent per-item dispatch, frame minting, all limit modes, frame hygiene,
empty-items/empty-body, multi-node body, sequencing) is fully proven. The **failure/compensation path is not** — rows
4–5 only exercise the nil/empty no-op guards, not the real `firstErr → gatherCancel → LIFO Unwind` of completed work.

## Proof run

```
$ go test ./pkg/op/provider/flow/... ./cmd/devlore-test/devloretest/ -run 'Gather' -count=1
ok  ...flow   ok  ...flow/gen   ok  ...devloretest
```

## Disposition

`complete` for the core deliverable — `plan.gather` is real, matches the iteration goal, and is thoroughly proven for
the success paths. Close the one gap by adding `TestGather_FailureUnwindsCompletedIterations` (a body whose Nth
iteration fails while earlier iterations produced observable, compensable side effects; assert the rest are cancelled
and the completed side effects are undone LIFO).
