---
step: 28 (prerequisite)
title: "Subgraphs own their executors — recovery-stack ownership moves to a per-subgraph executor"
status: design draft 2026-06-20 — awaiting approval, no code yet
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 28 (prerequisite) — Subgraphs own their executors

**Status:** design draft (2026-06-20), awaiting approval. This is the execution-core prerequisite that unblocks the
step-28 pause/resume work; resume cannot skip already-completed units while flow combinators re-mint empty recovery
stacks on every dispatch.

## The model (settled)

Every subgraph executes via its own executor, and that executor owns the subgraph's recovery stack — together with the
subgraph's variable scope, pause signal, trace, and catalog scope. `Gather`, `Choose`, and `Subgraph` are not special
combinators; each *is* a subgraph with its own executor. One rule, applied recursively down the subgraph tree. (Recorded
as the authoritative principle in
[2.3-orchestration-primitives.md](../../../architecture/2.3-orchestration-primitives.md#subgraph-execution--recovery-stack-ownership-current-model--2026-06-20).)

## Current deviation

There is a single shared `op.GraphExecutor`. `Subgraph.Execute` hands that same executor to children via the
`dispatchChild` closure (`activation_record.go`), and the flow combinators hand-roll their own stacks:

- **Forward:** `flow.Subgraph` mints `op.NewRecoveryStack()` (`provider.go:369`), `flow.Gather` mints a per-iteration
  `iterStack` (`:234`) plus a `gathered` stack (`:276`), `flow.Choose` returns an empty stack (`:115`). The minted stack
  is the method's *complement* — returned as the middle value of `(any, *op.RecoveryStack, error)` and `PushNested` onto
  the parent stack by `pushAuditReceipt`.
- **Undo:** `CompensateSubgraph` / `CompensateGather` / `CompensateChoose` (`provider.go:391`/`:299`/`:133`) exist only
  to unwind that combinator-supplied complement-stack.

So the combinator owns the stack on **both** sides — it mints it forward and unwinds it back.

## The change — ownership moves to the executor, symmetric on both sides

1. **Child executor.** `Subgraph.Execute` constructs a child executor that **shares** the parent's runtime environment,
   variable frame, and pause signal, but **owns its own recovery stack**. This is a construction path distinct from
   `GraphExecutor.Run`: it does **not** rebuild the environment, clone the catalog, or rebind variables — those stay
   `Run`'s one-time top-of-tree responsibilities. Pause is run-global: the child observes the parent's
   `pauseRequested`.
2. **Forward signatures — every combinator keeps its complement.** Each combinator's forward action returns its
   compensation state as its complement; none drops to `(any, error)`. `Subgraph` drops only its vestigial `items`
   parameter (iteration is Gather's job): `Subgraph(activation, kwargs) (any, *op.RecoveryStack, error)` and
   `Choose(...) (any, *op.RecoveryStack, error)` return a single stack; `Gather(activation, items, kwargs)
   (any, []*op.RecoveryStack, error)` returns the **slice** of per-iteration stacks (one per iteration). What changes vs
   today is the *source* of the stack — the per-subgraph executor owns and creates it; `Do()` no longer mints it via
   `op.NewRecoveryStack()`. Regenerates the flow provider.
3. **Every combinator keeps its compensate companion.** `CompensateSubgraph(stack *op.RecoveryStack)`,
   `CompensateChoose(stack *op.RecoveryStack)`, and `CompensateGather(stacks []*op.RecoveryStack)` each consume the
   complement their forward returned and unwind it — Gather undoes the slice (each iteration's stack, LIFO / reverse
   completion order). **No companion is removed.** The deviation being fixed is `Do()` *minting* the stack, not the
   companion's existence.
4. **Gather calls Subgraph once per item.** Gather iterates its `items`, calling `Subgraph` for each — each call runs the
   body once under its own executor with its own stack (created in that iteration's goroutine, never shared, so no race).
   Gather collects the N stacks and returns `(results, []*op.RecoveryStack)` — the slice of per-iteration stacks; its
   companion `CompensateGather` undoes the slice (item 3). Gather no longer folds them into one `gathered` stack
   (`provider.go:276,281`). (Stack count was always "many"; this fixes who owns them and how they are returned/undone.)
5. **`DispatchChild` drops its `stack` parameter (settled).** The param exists today only to scope receipts to a saga
   boundary in the absence of per-subgraph executors — the combinator mints a stack and threads it down. Once the
   dispatching executor owns its stack, the param can only ever carry the stack that executor already holds, so it is
   redundant: `DispatchChild(ctx, child, variables)`. Retry semantics are unchanged.

## Combinator signatures (confirmed in review — 2026-06-20)

Every flow combinator keeps **both** an action and a compensation companion: the action returns its compensation state as
its complement, the companion undoes it. `WaitUntil` joins the combinators (it was a leaf flow-action) — it is `Subgraph`
plus "re-run the body until it returns true or we time out." Receivers are all `func (p *Provider) …`.

| Combinator | Action signature | Compensation signature |
|---|---|---|
| `Choose` | `Choose(activation *op.ActivationRecord, defaultCase any, cases ...Case) (any, *op.RecoveryStack, error)` | `CompensateChoose(stack *op.RecoveryStack) error` |
| `Gather` | `Gather(activation *op.ActivationRecord, items []any, kwargs map[string]any) (any, []*op.RecoveryStack, error)` | `CompensateGather(stacks []*op.RecoveryStack) error` |
| `Subgraph` | `Subgraph(activation *op.ActivationRecord, kwargs map[string]any) (any, *op.RecoveryStack, error)` | `CompensateSubgraph(stack *op.RecoveryStack) error` |
| `WaitUntil` | `WaitUntil(activation *op.ActivationRecord, kwargs map[string]any, timeout, interval time.Duration) (any, *op.RecoveryStack, error)` | `CompensateWaitUntil(stack *op.RecoveryStack) error` |

- **`Subgraph`** binds `kwargs` → the subgraph's parameters (`subgraph.Parameters()`), runs the children under that
  frame, and returns the final executable unit's result. Single stack.
- **`WaitUntil`** is `Subgraph` plus a poll loop: it binds `kwargs` → the body subgraph's parameters, runs the body,
  evaluates the result for truthiness, and re-runs at `interval` until true or `timeout`. Single stack, single companion.
  (Was a leaf `(any, error)` polling a bare predicate; the body subgraph is now the predicate.)
- **`Choose`** returns the chosen branch's single stack.
- **`Gather`** calls `Subgraph` once per item and returns the **slice** of per-iteration stacks; `CompensateGather`
  undoes the slice.

## Saga-boundary semantics (settled 2026-06-20)

The saga boundary **is maintained** — rollback is a per-boundary unwind that propagates outward, **not** a single
root-level sweep. Each subgraph executor is a saga boundary and respects its retry policy:

- **Retry budget at the boundary.** No retries → one attempt; retry count N → N+1 attempts (the existing
  `DispatchChild` budget, now read as the boundary's — the subgraph's `RetryPolicy`, honored when the boundary is
  dispatched).
- **Retries exhaust before rollback propagates.** On failure the boundary runs its full retry budget first. No retries →
  rollback continues up the stack immediately; retry count N → all N are executed, then rollback continues up the stack.
- **Rollback continues up the stack** = the failure reaches the next outer saga boundary, which applies *its own* retry
  policy before unwinding its own stack and propagating further. Each executor unwinds its own stack — **replacing the
  current single top-level `Run` unwind** (`graph_executor.go:273`), which becomes one boundary among many (the root's).

- **Each failed attempt unwinds before it retries — forced by atomicity, not a choice.** A boundary is atomic, so a
  retry must run against the boundary's entry precondition. A failed attempt's completed children carry real side
  effects (a dir created, a resource allocated); re-running the body without first unwinding them double-applies
  non-idempotent operations and accrues duplicate receipts — the boundary stops being atomic. So each failed attempt
  compensates its own stack LIFO back to the entry precondition, then the next attempt runs clean. When the budget
  exhausts, the last attempt has already unwound (stack empty) and the bare failure propagates up, where the parent
  unwinds its own prior work per its own policy. (This is a behavior addition: today's `DispatchChild` re-dispatches
  without unwinding between attempts.) The "keep completed work, re-run from the failure point" model is **resume-after-
  pause** (sequence (b), skip-completed) — a different feature, not retry-on-failure; for an atomic boundary there is no
  no-undo retry.

## Files touched

- `pkg/op/graph_executor.go` — child-executor construction path; stack ownership.
- `pkg/op/subgraph.go` — `Subgraph.Execute` creates the child executor, nests its stack.
- `pkg/op/activation_record.go` — `DispatchChild` stack parameter (decision 5).
- `pkg/op/provider/flow/provider.go` + `flow/helpers.go` — combinator signatures `(any, error)`; remove
  `CompensateSubgraph/Gather/Choose`; `walkSubgraphChildren` no longer threads a minted stack.
- `pkg/op/provider/flow/gen/*` — regenerate (signature + companion changes).
- Tests: `flow`, `plan`, `cmd/devlore-test/devloretest` (gather/choose/compensation coverage).

## Sequencing within step 28

(a) **this prerequisite** → (b) resume re-entry + skip-completed (executor accepts `RunStatePaused`, preserves
`trace.Stack`, skips already-receipted units) → (c) catalog capture/restore in `op.Trace`. Step 28 does not close until
(c).

## Implementation verification note

Confirm during implementation that the named `CompensateX` companions are not on a live path I have not traced — the
observed live subgraph-compensation path is the nested-substack auto-unwind (`RecoveryStack.PushNested`), with the
subgraph's own receipt being audit-only (`&ReceiptBase{}`, no resource). If a companion is live, it changes rather than
vanishes; the ownership conclusion (executor unwinds its own stack) is unchanged.
