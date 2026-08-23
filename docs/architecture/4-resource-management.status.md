# Status: Resource Management

**Architecture document:** [4-resource-management.md](4-resource-management.md)

**State:** rewritten 2026-07-22 (phase-8 step 51, slice 3); **revised 2026-08-20 onto the
resource-construction rulings** ([plan](../plans/resource-construction.md), feature #581): §5 "The Catalog
Travels with the Graph" added (intent-only claiming, git-model plan-space paths, rel identity with run-bound
fsroot, mandatory serialized section with hard pre-flight, graph=intent/trace=observation); §4 rewritten to
runtime-only shadowing with the two-path reconciler as §4.1 (absorbed from the retired
catalog-reconciler-logic sketch); §1/§2 corrected — ordering is the promise's job, never URI matching;
**Appendix A deleted as rejected** (products are runtime facts); §5.6–5.7 added 2026-08-22 (a string is a
key, never a constructor; explicit discovery and resolution). **CONVERGED 2026-08-22: the design and the
tree agree, and the campaign's divergence table has no surviving row.** Phases 0–4 (#582–#586, delivered
2026-08-20..22) are in the tree — the catalog section serializes and is enforced, file identity is the rel
with run-bound activation, plan-time claiming, scoped verification, `MissingResourcePolicy`, and the
consumed-Gone guard are live, dispatch resolves resource slots by identity against the run catalog with a
miss refusing, production reconciles per §3's matrix, and run-computed paths enter through
`file.discover` / `file.resolve` with kind-honest claims. Every judgment pin the campaign authored is
committed green, including the two that were authored red on purpose
(`test_judgment_preflight_fail_fast.star`, `test_judgment_discover_after_exec.star`). The migration-era body — string-parameter "today"
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
| Dispatch resolves by identity; a run-catalog miss refuses; copy-on-bind; plan-time mirrors (§5.6) | Landed 2026-08-22 (#609; PR #613) |
| Production reconciles per §3's matrix — same-URI production appends generations; `GetOrCreate` returns the canonical | Landed 2026-08-22 (#610; PR #614) |
| Explicit conversion: `file.discover` / `file.resolve`, the runtime grammar, `op.OrderingEdge`, kind-honest `Exists`, `file.move_directory` (§5.7) | Landed 2026-08-22 (#611; PR #615) |

## Document Discrepancies

None known — the 2026-07-22 rewrite grounds every current-system claim in the tree and marks the unimplemented
proposal explicitly.

## Outstanding Work

1. The staged per-type `Resolve`/`Exists` rollout (step 22's ledger) — `file` is proven and now
   kind-honest; the other eight resource-bearing providers await their per-type step.
2. Remote-execution filesystem abstraction (open question §10.1).
3. **Run-start claiming for variable-fed resource slots** (ruled 2026-08-22, sequenced after the
   resource-construction campaign): a variable resolves the way a promise does — only after execution
   begins — so its claim belongs to the run's pre-flight, minted into the run clone at the consuming
   subgraph's start. The interim posture is a plan-time refusal of plain variables into resource-typed
   slots (the reserved gather `item` frame excepted).
4. Judgment scenario 2 (relocate the tree, reconcile at the new root) stays a recorded prediction until
   there is a drivable reconcile surface — the direct payoff of rel identity, unexercised.
Remaining step-51 slices are tracked in
[step 51](../plans/extract-starlark-from-op/phase-8/steps/51-documentation-debt.md).
