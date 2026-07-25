---
step: 37
former_step: 34
title: "RecoveryStack.Unwind takes the runtime environment; Push/PushNested drop it"
status: complete 2026-07-01 — pulled forward, ahead of flow.gather
proof_run: 2026-07-01
parent: ../../phase-8.md
---

# Step 37 — Unwind takes the runtime environment (formerly 34)

**Status:** `complete 2026-07-01`, pulled forward ahead of the gather-resume work it unblocked.

## What landed

1. `RecoveryStack.Unwind(runtimeEnvironment *RuntimeEnvironment) error` — the environment is bound once, at rollback,
   and threaded down into every nested substack's own `Unwind`, rather than captured at push time.
2. `Push` / `PushNested` drop their runtime-environment parameters — an entry's undo closure receives the environment
   from the `Unwind` call that runs it.
3. Every combinator compensator (`CompensateSubgraph` / `CompensateGather` / `CompensateChoose` /
   `CompensateWaitUntil`) collapses to `stack.Unwind(activation.RuntimeEnvironment)`.

## History

Landed with the step-31 arc (commit `903864cb`, under the pre-renumber label "step 34"); recorded in the
[step-31 doc](31-subgraph-executor-ownership.md) and the
[3.5.2 status table](../../../architecture/3.5.2-flow-provider.status.md) ("env-deferred unwind" row).
