---
step: 35
former_step: 32
title: "Retry-policy tri-state + per-type defaults"
status: COMPLETE 2026-07-20 — tri-state + full jitter + per-type defaults landed; root and combinators exempted during implementation
proof_run: 2026-07-20 (make build clean; make test green — 98 packages; gofmt + vet clean)
parent: ../../phase-8.md
---

# Step 35 — Retry-policy tri-state + per-type defaults (formerly 32)

**Status:** `COMPLETE` (2026-07-20). Design settled 2026-06-20; extracted from the phase-8 table cell (2026-07-03
audit). Landed with two scope refinements settled during implementation — see [Landed](#landed).

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

**Configuration home (settled 2026-07-06 with step 41's Q3):** the default policy lives in the op-owned
`PoliciesConfig` section as `policies.retry` — a plain `op.RetryPolicy`, not a separate defaults type — read via
`Application.Config` (the contract doc §"TransitionPolicy — Q3 settled" carries the full section shape). The
tri-state restated against it: *none* = explicit `MaxAttempts:0`; *default* = nil/unset → a **subgraph combinator**
resolves to the configured `policies.retry`, **every other executable unit resolves to none**; *specific* = an
explicit policy on the unit wins. Item 4's `MaxAttempts:3` + exponential + jitter is `policies.retry`'s builtin
floor.

## Landed

The tri-state, per-type defaults, and full jitter landed 2026-07-20. **Two scope refinements were settled during
implementation** (the design's literal "a subgraph defaults to retry" was too broad):

1. **The graph root is exempt.** The retry-then-rollback rationale is "exhaust retries, then roll back **up**" — but
   the root has no "up"; exhausting its retries just fails the run. So the root (also `flow.subgraph`) resolves to
   **none**; whole-run retry is opt-in via an explicit policy, not the default.
2. **The flow combinators are exempt.** `gather` / `choose` / `wait_until` are mechanically subgraphs, but each
   carries a deliberate failure protocol — `wait_until` *already* polls, so an outer retry would retry its retry;
   `gather` unwinds per-item. Implementing "every subgraph retries" made `TestWaitUntil_BodyErrorFailsImmediately`
   take 5.58s (retry-3×-then-fail) instead of failing fast. So only a **structural** subgraph (bound to
   `flow.subgraph`, the pure saga boundary) inherits the default; the combinators keep their own semantics.

Net resolution (`GraphExecutor.retryPolicyFor`): an explicit unit policy wins (`MaxAttempts:0` = deliberate
no-retry); a **structural nested subgraph** (`flow.subgraph`, not the root) inherits `policies.retry`; a **node**
(construction stamps an explicit `MaxAttempts:0`), the **root**, the **combinators**, and every non-subgraph unit
resolve to **none**. `retryPolicyFor` names `"flow.subgraph"` as a string (op cannot import the flow provider for
the const — the item-5 import-cycle floor).

- **Jitter** — `RetryPolicy.Jitter bool`; `ComputeDelay` treats the (MaxDelay-capped) backoff curve as a ceiling and
  draws the wait uniformly from `[0, ceiling]` via `math/rand/v2` (the non-jitter paths stay deterministic). Full
  jitter is the anti-thundering-herd choice — it spreads a correlated retry herd (a `gather`'s concurrent bodies
  failing against one downstream) across the whole window instead of releasing a synchronized spike.
- **The `policies.retry` floor** — `NewPoliciesConfig().Retry` = `MaxAttempts:3`, exponential, `1s` → `30s` cap,
  `Jitter:true`. Read from the builtin floor today, exactly as `transitionPolicyFor` reads the transition floor; the
  file / env / cli layering rides the config loader later. (The step-41 placeholder doc calling it "zero-value" was
  corrected.)
- **Node default** — `NewNode` stamps an explicit `&RetryPolicy{MaxAttempts:0}` when the spec leaves it unset, so a
  node's no-retry intent is unambiguous in the unit and in the serialized document (nodes now serialize a
  `retry: {max_attempts: 0}` block; no test hardcodes a checksum, and the round-trip checksum tests are
  consistency-based, so this is inert to the suite).
- **Setting the root's policy** — the root defaults to none, so an explicit policy is the only way to make it retry:
  `op.NewGraphSpec().WithRetryPolicy(&op.RetryPolicy{…})` (Go, delegates to the root subgraph) or
  `plan.assemble_definition(…, retry_policy={…})` (Starlark, projected to `*op.RetryPolicy` and stamped on the root).
- **Tests** — `TestRetryPolicyFor_TriState` (the resolution across structural / combinator / root / node / explicit),
  `TestNewNode_StampsExplicitNoRetry`, `TestComputeDelay_FullJitter` (bounds `[0, ceiling]` + real spread).

`make build` clean; `make test` green (98 packages); gofmt + vet clean.

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
