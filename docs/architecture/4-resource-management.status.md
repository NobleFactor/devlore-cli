# Status: Resource Management

**Architecture document:** [4-resource-management.md](4-resource-management.md)

**State:** rewritten 2026-07-22 (phase-8 step 51, slice 3). The migration-era body — string-parameter "today"
snapshots, per-method signature-migration tables, `Tombstone` recovery, the `RegisterConstructor` /
`coerceSlotValue` coercion chain, the phase-by-phase bookkeeping — is replaced by the landed model: the catalog
surface (tree-verified method by method), the `ResourceState` machine and behavior matrix (the 2026-07-14 rulings,
kept), shadowing, observations-as-results, receipts + the recovery site, and the platform/data-provider and
lifecycle sections restated. The **declared-output-spec design (old §6.8–6.9) did not land** — verified 2026-07-22:
no `OutputSpec`, no `KnownAtExecution`, no `*Planned` companions in the tree — and is preserved as an explicitly
unimplemented appendix with its prior-art grounding; the landed alternative (planner + conversion cascade,
post-dispatch shadowing for monadic outputs) is named beside it.

## Completion

| Component | Status |
|-----------|--------|
| `Resource` sealed interface + `ResourceBase` identity | Landed |
| `ResourceCatalog` (ledger + namespace + clone/snapshot/content transport) | Landed (steps 22/25/48) |
| `ResourceState` machine + behavior matrix + `VerifyExistence` pre-flight | Landed (steps 22/41; per-type rollout staged — `file` proven) |
| Observations as results, never catalog members | Landed (ruled 2026-07-14; step 22) |
| Receipts + recovery site (Tombstones retired) | Landed (steps 40/42; [3.5.4](3.5.4-file-provider.md)) |
| Declared output specs (Appendix A) | **Not implemented** — preserved as design |
| Document rewrite onto the landed model | Complete 2026-07-22 (step 51 slice 3) |

## Document Discrepancies

None known — the 2026-07-22 rewrite grounds every current-system claim in the tree and marks the unimplemented
proposal explicitly.

## Outstanding Work

1. The staged per-type `Resolve`/`Exists` rollout (step 22's ledger).
2. Remote-execution filesystem abstraction (open question §9.1).
Remaining step-51 slices are tracked in
[step 51](../plans/extract-starlark-from-op/phase-8/steps/51-documentation-debt.md).
