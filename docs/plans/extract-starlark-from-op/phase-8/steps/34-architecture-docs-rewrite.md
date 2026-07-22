---
step: 34
former_step: 31
title: "Architecture docs — rewrite 2, 2.2, 2.3 onto the pkg/op model in full"
status: in-progress — slices A (2-execution-graph.md) + B (2.2-phase-execution.md) landed 2026-07-21; slices C (2.3), D (reference sweep) pending
proof_run: n/a (documentation)
parent: ../../phase-8.md
---

# Step 34 — Architecture docs rewrite (formerly 31)

**Status:** `in-progress`. Documentation work, not a code deliverable (charter set 2026-06-20; extracted here from the
phase-8 table cell, 2026-07-03 audit). Slice plan and progress:

1. **Slice A — `2-execution-graph.md` (landed 2026-07-21).** Full rewrite onto the sealed `op.Graph` model: the
   unit tree (`Node`/`Subgraph`, every combinator a subgraph with its own child executor), spec-based setter-free
   construction (edge materialization from promises + toposort + checksum at seal), the sealed `Binding` set with the
   plan-time type-check, the two front doors (Starlark planning API / direct Go), the `GraphExecutor` contract
   (result = final dispatch's output; halt vs health; pause/stop/resume/`ResumeUnwind`), the graph-vs-trace document
   split with the `internal/cli` store + run index, the thin command layer, and the old→new mapping table. Every
   claim verified against the tree; `.status.md` rewritten to match.
2. **Slice B — `2.2-phase-execution.md` (landed 2026-07-21).** Rewrite onto the saga-over-units model: the
   compensable-action contract ((A, C, S) as action / `Compensate<Name>` / receipt, activation-first, receipt names
   its own undo through the `compensatingActionIndex`), per-executor recovery stacks + the `Compensator` interface,
   the forward-path failure adjudication (retry tri-state → error-action verdict → `TransitionPolicy`), and the
   serialization/resume paths. Preserved: Prior art (step 42 — its serialization bullet corrected from the
   superseded `kind` tag to the structural `entries` discriminator), Run Status and Terminals (step 41), Compensation
   Failure Has No Forward Continuation. Replaced body carried two banned `savedComplement` residuals (step-40 purge
   misses) and a Files table naming seven deleted files. `.status.md` rewritten to match.
3. **Slice C — `2.3-orchestration-primitives.md` body.** Pending.
4. **Slice D — scattered-reference sweep** (`2.1`, `3.2`, `8`, others by grep). Pending.

## Problem — the stale core

The execution-model architecture docs describe the superseded pre-`pkg/op` design and must be **rewritten**, not
patched:

1. [`2-execution-graph.md`](../../../../architecture/2-execution-graph.md) — `internal/graph/builder.go`,
   `ExecutionGraph`, `graph.Run()`, the Command-Layer→GraphBuilder pipeline.
2. [`2.2-phase-execution.md`](../../../../architecture/2.2-phase-execution.md) — the saga/phase model.
3. The body of [`2.3-orchestration-primitives.md`](../../../../architecture/2.3-orchestration-primitives.md) from
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
