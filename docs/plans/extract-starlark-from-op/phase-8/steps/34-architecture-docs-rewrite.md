---
step: 34
former_step: 31
title: "Architecture docs — rewrite 2, 2.2, 2.3 onto the pkg/op model in full"
status: in-progress — all four slices landed 2026-07-21 (A: 2, B: 2.2, C: 2.3, D: reference sweep); closure pending the structural-debt disposition (13 inventoried docs)
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
3. **Slice C — `2.3-orchestration-primitives.md` (landed 2026-07-21).** The dated 2026-06-20 section promoted to
   the document's spine, updated where its deviation paragraphs have since landed (step 31: executors own stacks;
   step 35: retry tri-state realized; step 42 3a: one stamped substack per Gather iteration — the `[]*RecoveryStack`
   shape and ride-on-audit-receipt nesting superseded). New body: the seven verified flow actions
   (`Fatal`→`flow.failed`; the `Elevate` flow-action stub replaced by the policy pointer), the four combinators with
   live signatures, terminals + error actions, variables/field projection replacing `SlotProxy`/`GatherRef`, current
   hooks + the step-50 pointer, and a verified Files table. `.status.md` rewritten to match.
4. **Slice D — scattered-reference sweep (landed 2026-07-21).** Grep census over every non-rewritten doc for the
   pre-`op` vocabulary (`SlotValue`, `internal/execution`, `RunPhased`, `Tombstone`, `op.Context`, `flow.fatal`,
   banned `*Complement` forms, …). **Spot-fixed** the passing references in otherwise-current docs:
   `index.md` (engine → `pkg/op`), `6-execution-topology.md` (no `flow.elevate`/`flow.fatal` actions exist — the
   sketch reframed as pre-`op`), `5.2-recovery-serialization.md` (recovery-entry vocabulary, `GatherComplement`
   residuals, the resolved gather-restart open question), `3.5.1-archive-provider.md` ("the complement" → the
   compensating action), `3.5.2-flow-provider.md` (live terminal signatures; `op.FatalError` gone),
   `4.2-mem-resource.md` (`op.Context.Thread` → `starlarkbridge.Invoker` session service),
   `5.3-recovery-site.md` (proposed mem receipt in receipt vocabulary). **Structurally stale docs inventoried, not
   half-patched** (their bodies are built on the pre-`op` model; spot edits would misrepresent them as current):
   `4-resource-management.md` (38 hits), `8-rust-migration.md` (23), `5.1-reconciliation.md` (21),
   `1-system-model.md` (12), `3.2-projected-provider-api.md` (8), `2.1-typed-slots.md` (8),
   `3-operation-namespaces.md` (7), `3.3-static-starlark-codegen.md` (4), `4.3-resource-registration.md` (the
   deleted callable-slot machinery), `7.1-llm-integration.md` + `7.2-e2e-testing.md` (pre-rewrite migrate examples),
   `docs/package-hierarchy.md` + `docs/package-reference.md` (`internal/execution` sections). Disposition of the
   structural set is a chartering decision — see the closure note in the phase-8 row.

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
