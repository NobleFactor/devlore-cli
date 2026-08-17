# Status: Execution Graph

**Architecture document:** [2-execution-graph.md](2-execution-graph.md)

**State:** rewritten 2026-07-21 (phase-8 step 34, slice A) onto the landed `pkg/op` model. The former body — the
pre-`op` `internal/graph` design (`ExecutionGraph`, `GraphBuilder`, `SlotValue`, `GraphState`, `graph.Run()`) — is
replaced; its old→new mapping is preserved in the document's closing table. Every code claim in the rewritten document
was verified against the tree on 2026-07-21 (`pkg/op/graph.go`, `binding.go`, `validate.go`, `graph_executor.go`,
`cmd/internal/cli/receipts.go`, `cmd/writ/writ/deploy/deploy.go`).

## Completion

| Component | Status | Completed | PR |
|-----------|--------|-----------|-----|
| Sealed `op.Graph` model (spec-based construction, unit tree, bindings, edges, toposort, checksum) | Complete | 2026-07 (phase-8) | phase-8 branch |
| `op.GraphExecutor` (per-run environment, child executors, result contract, resume paths) | Complete | 2026-07 (phase-8) | phase-8 branch |
| Graph document + trace persistence, run index | Complete | 2026-07-16 (step 47) | phase-8 branch |
| Signing (`pkg/signing`, `writ verify`) | Complete | 2026-07-16 (step 46) | phase-8 branch |
| Document rewrite onto the landed model | Complete | 2026-07-21 (step 34 slice A) | phase-8 branch |

## Document Discrepancies

None known — the 2026-07-21 rewrite grounds every claim in the current tree. The pre-rewrite discrepancy list (stale
type names, `internal/execution` paths, `Phases`/`Rollback` fields) is obsolete along with the body it described.

## Outstanding Work

None for this document. The sibling rewrites — [2.2](2.2-phase-execution.md) (step 34 slice B) and
[2.3](2.3-orchestration-primitives.md) (slice C) — and the scattered-reference sweep (slice D) are tracked in
[step 34](../plans/extract-starlark-from-op/phase-8/steps/34-architecture-docs-rewrite.md).
