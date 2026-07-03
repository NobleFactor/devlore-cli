---
step: 12
title: "plan.wait_until — poll a body subgraph until its result is truthy or the budget expires"
former_step: 15
former_title: "plan.wait_until redesign (not-started; direction TBD pending step 13)"
status: implemented 2026-07-02 — body-subgraph polling landed; fixtures + unit tests green
proof_run: 2026-07-02
parent: ../../phase-8.md
---

# Step 12 — plan.wait_until (formerly step 15)

**Status:** `implemented 2026-07-02`. The lambda-polling stub is gone; wait_until is a quantifier over Subgraph — the
Gather side of the step-10 criterion (its poll count is runtime-unknown, so the loop lives in the method, not the
topology; the guarded back-edge loop remains step 10's anticipated future construct, not a prerequisite here).

## The surface (settled 2026-07-02)

`plan.wait_until(body=<body>, timeout=<duration>, interval=<duration>)` — the old goal's `predicate=` kwarg is renamed
`body=` per the settled direction "do what we did with choose when/then and the gather body": a body is a list of
invocations, a singleton invocation (any action works as the predicate — its result is evaluated for truthiness), or a
lambda desugared to the `function.call` leaf and archived as a content-addressed `function.Resource`. `timeout=` is
required (the mandatory runtime budget — a poll loop can no more prove termination than a topology can); `interval=`
defaults to 5s. Durations are strings in `time.ParseDuration` syntax ("60s", "2m") or Go `time.Duration` values,
parsed at plan time; bare integers are rejected (their unit would be a guess). Remaining kwargs are frame bindings,
like Subgraph.

```python
plan.wait_until(body=plan.file.exists(resource=ready_path), timeout="60s", interval="2s")
plan.wait_until(body=lambda: healthy(), timeout="30s")
```

## The execution (four decisions, settled 2026-07-02)

`flow.Provider.WaitUntil(activation, timeout, interval, kwargs) (any, *op.RecoveryStack, error)` — compensable, with
`CompensateWaitUntil` unwinding the returned stack. (The kwargs catch-all sits last in the signature; the earlier
3.5.2 sketch had it first, which method registration rejects.) Each poll runs the body via `walkSubgraphChildren` on
its own scratch child stack; the body subgraph's result (its last child's) is read with `isTruthy`.

1. **Falsy polls drop unrecorded.** Only the truthy run's stack survives — stamped and nested. The body is expected
   side-effect-free (nothing enforces it; a side-effecting poll is a plan defect, the same by-design stance as
   gather's concurrency contract). The trace reads like a subgraph that ran its body once.
2. **Resume re-enters fresh.** A completed wait_until replays its stamped result upstream like any unit; an
   interrupted one left nothing behind (falsy polls drop), so re-entry polls again with the full budget — across a
   save/reload gap the world has moved, and re-checking is the correct epistemics. Nothing new serializes.
3. **Timeout is a plain error** carrying the poll count and the last falsy result — the unit fails like any failing
   child; retry policy composes on top (another full window per attempt). A body error fails immediately (a crashed
   probe is not "not ready"). A typed `ErrWaitTimeout` sentinel is noted for later, the day something consumes it.
4. **The truthy run stamps the wait_until's own unit ID** — one surviving substack, one identity; Gather's `#i`
   iteration stamps exist to discriminate N coexisting siblings, which wait_until does not have.

`WaitUntilPlanner` adopts the body as children (`resolveBodyChildren`; lambda desugar via the same
`actionInvocationPlanner` assertion `ChoosePlanner` uses for `default=`), parses `timeout`/`interval` into typed
slots, and rejects a missing `body=` or `timeout=` at plan time.

## Test matrix

| # | Test | Proves | Grade |
|---|---|---|---|
| 1 | `TestWaitUntilAction_DryRun` (gen) | the generated dry-run path | ✅ |
| 2 | `TestWaitUntil_TimeoutOnFalsyBody` (flow) | falsy polls → timeout error; scratch stacks drop (stack stays empty) | ✅ |
| 3 | `TestWaitUntil_TimeoutRequired` (flow) | zero timeout rejected | ✅ |
| 4 | `TestCompensateWaitUntil_NilStack_NoOp` (flow) | nil-stack compensation no-op | ✅ |
| 5 | `test_wait_until.star` | end-to-end: invocation body (first-poll truthy) + lambda body; result flows downstream | ✅ |
| 6 | `test_wait_until_timeout.star` | end-to-end: budget expiry across multiple polls → "timeout after" error | ✅ |
| — | match-after-N-polls (truthiness flips mid-run) | the re-poll path returns a late truthy result | ☐ needs a mutable probe (concurrent writer or counter); no fixture mechanism today |
| — | context-cancel mid-poll | `Context.Err()` propagation | ☐ |
| — | body-error propagation fixture | a crashed probe fails immediately, not at timeout | ☐ |

## Open follow-ups

1. The three unchecked matrix rows above — the first needs a fixture-level mutable probe, which today's harness lacks.
2. `ErrWaitTimeout` sentinel (settlement 3's note) when a consumer appears.
3. If the guarded back-edge loop construct (step 10's anticipated extension) lands, wait_until stays as-is — the
   method loop is its settled shape; `plan.while` would be a sibling, not a replacement.
