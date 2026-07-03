---
step: 35
former_step: 32
title: "Retry-policy tri-state + per-type defaults"
status: not-started — design settled 2026-06-20, implementation pending
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 35 — Retry-policy tri-state + per-type defaults (formerly 32)

**Status:** `not-started`. Design settled 2026-06-20; extracted here from the phase-8 table cell (2026-07-03 audit).

## Problem — not tri-state today

The policy is carried as `*RetryPolicy` (nil when unset, `executable_unit.go:240`); `DispatchChild` treats nil as one
attempt (`activation_record.go:172-177`), and the field contract says `MaxAttempts:0` = "no retry, fail immediately"
(`retry_policy.go:13`) — so **nil ≡ `MaxAttempts:0` ≡ no-retry** are conflated, and both a node (`node.go:59`) and a
subgraph (`subgraph.go:719`) default to no-retry identically.

## The tri-state to realize

1. **No retry** — explicit `MaxAttempts:0`, fail immediately.
2. **Default retry** — unset/nil, defer to a framework default.
3. **Non-default retry** — explicit `MaxAttempts:N>0`.

The change is that **nil stops meaning no-retry and starts meaning *default*** (an explicit `MaxAttempts:0` carries
"no retry").

## Per-type defaults (settled 2026-06-20)

1. A **node** carries an explicit `MaxAttempts:0` (no retry) by default — a leaf is one provider call, so fail fast
   and let the enclosing boundary decide.
2. A **subgraph** carries **nil** by default, resolving to the **graph's default retry policy** — the subgraph *is*
   the saga boundary, and step 31's rollback rule is gated on its retry policy, so defaulting it to retry is what
   makes "exhaust retries, then roll back up" the actual default.
3. The two unset-defaults are deliberately different representations: a node stamps an explicit no-retry policy; a
   subgraph stays nil and inherits the graph default at resolution time.
4. **The graph default retry policy is `MaxAttempts:3` with exponential backoff and jitter.**

## New work this implies

1. **Nil-resolution** — `DispatchChild` must resolve nil to the graph default (today nil → 1 attempt,
   `activation_record.go:172-177`), so nil stops meaning no-retry.
2. **Jitter** — `RetryPolicy` (`retry_policy.go`) today has `Backoff` (none/linear/exponential), `InitialDelay`,
   `MaxDelay` but **no jitter**; add a jitter component and apply it in `ComputeDelay`.
3. **Node default** — node construction must stamp `MaxAttempts:0` (today `node.go:59` leaves it nil).
4. **Graph-level default policy** — a carrier must exist to resolve nil against.

## Touches

`DispatchChild`, node/subgraph construction (`node.go:59`, `subgraph.go:719`), `RetryPolicy` (jitter) + its doc
contract, the graph default-policy carrier, and the step-31 saga rollback gating. Pairs with step 31.
