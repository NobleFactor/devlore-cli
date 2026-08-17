# Status: System Model

**Architecture document:** [1-system-model.md](1-system-model.md)

**State:** vision document, restated 2026-07-22 (phase-8 step 51, slice 2) onto the landed `pkg/op` model. The
vision content (dependency taxonomy, package planner, distributed orchestration, the emergent record graph) is
preserved and explicitly marked *vision*; every current-system claim now describes the landed model — units + saga
receipts instead of runtime phases, the sealed `Binding` set instead of `Proxy`/`Context.Data`, retained
receipts/journal instead of Tombstones, the trace/run-index instead of the receipt-YAML sketch, and a corrected §12
status table (`pkg/op`, `pkg/signing`, `cmd/internal/cli`; the landed drift attribution of steps 47–48). The pre-rewrite
discrepancy list (Sidecar cross-reference, `internal/execution` paths, provider count, `actions_gen.go`) is obsolete
with the body it described.

## Completion

| Component | Status |
|-----------|--------|
| Local execution pipeline (§6.1) as described | Landed (phase-8) |
| Record contents + drift detection (§7.2–7.3) as described | Landed locally (steps 46–49) |
| DAG-by-construction + recorded retries (§8.1, 8.3) | Landed |
| Package planner (§5) | Vision — not started |
| Distributed orchestration (§6.2–6.4) | Vision — not started |
| Global record graph (§7 at fleet scale) | Vision — not started |
| Document restatement onto the landed model | Complete 2026-07-22 (step 51 slice 2) |

## Document Discrepancies

None known — the 2026-07-22 restatement grounds every current-system claim in the tree and marks vision content
explicitly.

## Outstanding Work

The vision items above graduate to their own designs when chartered. Document-wise, none; remaining step-51 slices
are tracked in [step 51](../plans/extract-starlark-from-op/phase-8/steps/51-documentation-debt.md).
