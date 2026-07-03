---
step: 34
former_step: 31
title: "Architecture docs — rewrite 2, 2.2, 2.3 onto the pkg/op model in full"
status: not-started — documentation work; sequence after the execution-core changes so the docs describe landed reality
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 34 — Architecture docs rewrite (formerly 31)

**Status:** `not-started` (charter set 2026-06-20; extracted here from the phase-8 table cell, 2026-07-03 audit).
Documentation work, not a code deliverable.

## Problem — the stale core

The execution-model architecture docs describe the superseded pre-`pkg/op` design and must be **rewritten**, not
patched:

1. [`2-execution-graph.md`](../../../architecture/2-execution-graph.md) — `internal/graph/builder.go`,
   `ExecutionGraph`, `graph.Run()`, the Command-Layer→GraphBuilder pipeline.
2. [`2.2-phase-execution.md`](../../../architecture/2.2-phase-execution.md) — the saga/phase model.
3. The body of [`2.3-orchestration-primitives.md`](../../../architecture/2.3-orchestration-primitives.md) from
   `## Vocabulary` down — `Phase`, `Graph.Phases`, `RunPhased`, `ExecutePhaseInner`, `ActivationState`, `SlotValue`,
   the `internal/execution/*` + `pkg/op/recovery.go` file map.

## Rewrite target

The sealed `op.Graph` of `op.Subgraph`/`op.Node` units; `op.Binding` slots (`Immediate`/`Promise`/`Variable`)
replacing `SlotValue`; dispatch via each unit's bound action (no `Phase`/`RunPhased`); `op.GraphExecutor` +
`op.RecoveryStack` as the runtime; and the per-subgraph-executor recovery-stack-ownership model recorded in 2.3's
dated section (landed 2026-06-20) as the authoritative principle. The dated section and its staleness blockquote are
the seed; this step finishes the job by replacing the historical body rather than leaving it fenced off.

## Scope note

Scattered `SlotValue`→`Binding` and `internal/execution`→`pkg/op` references also appear in `2.1`, `3.2`, `8`, and
others (grep 2026-06-20); whether they fold into this step or a follow-on documentation-debt sweep is open.
