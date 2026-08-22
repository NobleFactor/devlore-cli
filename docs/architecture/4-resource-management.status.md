# Status: Resource Management

**Architecture document:** [4-resource-management.md](4-resource-management.md)

**State:** rewritten 2026-07-22 (phase-8 step 51, slice 3); **revised 2026-08-20 onto the
resource-construction rulings** ([plan](../plans/resource-construction.md), feature #581): §5 "The Catalog
Travels with the Graph" added (intent-only claiming, git-model plan-space paths, rel identity with run-bound
fsroot, mandatory serialized section with hard pre-flight, graph=intent/trace=observation); §4 rewritten to
runtime-only shadowing with the two-path reconciler as §4.1 (absorbed from the retired
catalog-reconciler-logic sketch); §1/§2 corrected — ordering is the promise's job, never URI matching;
**Appendix A deleted as rejected** (products are runtime facts). The DESIGN is current, and the TREE
implements it through phase 3 (#582–#585, delivered 2026-08-20..22): the catalog section serializes and is
enforced, file identity is the rel with run-bound activation, and plan-time claiming, scoped verification,
`MissingResourcePolicy`, and the consumed-Gone guard are live — the once-red judgment pins
(`catalog_contract_test.go`, `test_graph_catalog_contract.star`) are committed green. The remaining gap is
#586 — dispatch still re-derives resources from strings — with #587 the closure. The migration-era body — string-parameter "today"
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
| Declared output specs (former Appendix A) | **Rejected 2026-08-20** — removed; products are runtime facts |
| Document rewrite onto the landed model | Complete 2026-07-22 (step 51 slice 3) |
| Catalog section serialized + enforced (mandatory even empty; hard load/pre-flight failure) | Landed 2026-08-21 (#583; PR #593) |
| File identity = rel; plan-space grammar; activation binds the run's root | Landed 2026-08-21 (#584; PRs #596/#600/#601) |
| Plan-time claiming: scoped verification, `MissingResourcePolicy`, consumed-Gone guard, resource-typed mutators, `{id, uri}` intent rows | Landed 2026-08-22 (#585; PRs #602–#606) |

## Document Discrepancies

None known — the 2026-07-22 rewrite grounds every current-system claim in the tree and marks the unimplemented
proposal explicitly.

## Outstanding Work

1. The resource-construction remainder, #586 + #587: run time consumes the catalog — dispatch resolves
   slots against the run catalog, the `buildCandidateAs` string re-parsing retires, products update the
   claimed pending entries — then closure (statuses, the transport supersession note).
2. The staged per-type `Resolve`/`Exists` rollout (step 22's ledger).
3. Remote-execution filesystem abstraction (open question §10.1).
Remaining step-51 slices are tracked in
[step 51](../plans/extract-starlark-from-op/phase-8/steps/51-documentation-debt.md).
