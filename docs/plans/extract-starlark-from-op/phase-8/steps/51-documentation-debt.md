---
step: 51
title: "Documentation debt — rewrite the remaining pre-op docs; architecture coherence gate"
status: in-progress — slices 1–7 landed 2026-07-22 (decisions (a) + (c) settled: 3.3 archived; package docs compact hand-maintained, generation deferred); slice 8 pending
parent: ../../phase-8.md
---

# Step 51 — Documentation debt: rewrite the remaining pre-`op` docs

**Status:** `not-started` — chartered 2026-07-21. Source: the
[step 34](34-architecture-docs-rewrite.md) slice-D inventory — 13 docs whose bodies describe the pre-`pkg/op` model
(hit counts recorded there). Sequenced **after step 39** (the per-provider docs) and **before the PR to develop**.

## The bar

The step-34 standard, uniformly:

1. **Full rewrite where the body is stale** — never spot-patches that misrepresent a doc as current.
2. Every code claim **tree-verified** at time of writing.
3. `.status.md` companions rewritten to match (discrepancies list emptied or made current).
4. Banned-term-free (`complement`, `Tombstone`-as-datum, pre-`op` vocabulary only as marked history).
5. 120-column wrapping; relative links and anchors checked.

## Slices (priority order)

1. **`2.1-typed-slots.md` (landed 2026-07-22).** Rewritten onto the sealed `Binding` set, the variable surface
   (replacing `Context.Data`), signature-driven slot typing, and announce-time classification with reflect-once
   adapters. Preserved: Plan-Time Conversion, The Converter, Calling into Starlark — the Invoker text re-verified
   against the tree (per-resource construction; never a process singleton). `.status.md` rewritten.
2. **`1-system-model.md` (landed 2026-07-22).** Vision preserved and explicitly marked (*vision* tags on the
   package planner, distributed orchestration, the global record graph); current-system claims restated onto the
   landed model (units + saga receipts, the Binding set, retained records instead of Tombstones, the trace/run-index,
   a corrected §12 table). `.status.md` rewritten.
3. **`4-resource-management.md` + `4.3-resource-registration.md` (landed 2026-07-22).** The resource pair
   rewritten: 4's migration-era bookkeeping replaced by the landed catalog model (tree-verified surface, the
   `ResourceState` machine + 2026-07-14 behavior matrix kept, shadowing, observations, receipts + recovery site) —
   with the **declared-output-spec design preserved as an explicitly unimplemented appendix** (verified: no
   `OutputSpec` / `KnownAtExecution` / `*Planned` in the tree; the landed alternative named). 4.3's lazy-descriptor
   + callable-extraction body replaced by the eager generated announcements into the one receiver registry, the
   three construction paths, and callables-as-`function.Resource`; the source-type declaration section kept
   (verified current). Both `.status.md` companions rewritten. (4.1 / 4.2 / 4.4 current — untouched.)
4. **`5.1-reconciliation.md` (landed 2026-07-22).** The issue-#156 draft plan's machinery (4-value `Do`,
   `Reconcile<X>` triangle, `ExecutionEvent`/`EventSink`/`AuditLedger`/`ReconciliationStore`, `ErrDrifted`) never
   landed — tree-verified. Rewritten onto the landed realization: trace-as-audit; `writ status` + Etag/Digest drift
   attribution (steps 47–48) replacing `writ reconcile`; the distributed recovery-safety gates (receipt precision,
   recovery-digest tamper check, state-checked `ResumeUnwind`); the `EventSink` idea mapped to the 2.8 observability
   design. Proposal preserved as an unimplemented appendix with a where-its-ideas-went mapping. `.status.md`
   rewritten (its own "ErrDrifted Complete" row no longer matched the tree).
5. **The provider-authoring trio (landed 2026-07-22).** 5a: `3-operation-namespaces.md` rewritten onto the landed
   workflow (provider contract → `make generate` → tests → catalog + design doc; stale namespace tables dropped —
   3.5 owns the inventory). 5b: `3.2-projected-provider-api.md` rewritten — the dead announce-and-callback /
   `pkg/projection` / generated-receiver mechanics replaced by the runtime projection (goReceiver + announced
   metadata, the three-tier surface, one process-global registry); the current **Pluggable brokers** and **Member
   Projection** sections preserved; consumers restated (lore's dual door, writ's rewritten deploy family).
   **Decision (a) settled: `3.3-static-starlark-codegen.md` ARCHIVED** (user decision) with a banner naming the
   landed alternative and crediting its one shipped idea (`+devlore:property`).
6. **`7.1-llm-integration.md` + `7.2-e2e-testing.md` (landed 2026-07-22).** Surgical re-grounding (the docs were
   structurally current): 7.1's response-parsing example corrected to the real flow — the LLM proposes analysis +
   a flat operation list, and the sealed graph is Go-built through `planBuilder` → `AssembleDefinition`, so LLM
   proposals pass the same validation as hand-written plans; the provider-selection example moved onto
   `model.EnsureProvider`'s chain. 7.2's correctness evaluator corrected to the live
   analysis-plus-sealed-graph shape. Status companions updated.
7. **`docs/package-hierarchy.md` + `docs/package-reference.md` (landed 2026-07-22).** Both rewritten from the
   live tree: hierarchy = the three-layer map + dependency rule + tree; reference = a one-line-per-package roster
   (the former per-symbol element listing was unmaintainable by hand and described the deleted layout — symbol
   reference is `go doc`'s job). **Decision (c) settled:** compact hand-maintained; a generator is a possible
   future charter if churn warrants.
8. **`8-rust-migration.md`** — last: re-ground or explicitly re-mark as historical vision (open decision (b)); its
   premise is "describe the system to port," so it is only worth rewriting after everything else settles.

## Exit gate — architecture coherence verification (directed 2026-07-21)

After steps 39 and 51 both complete, a closing verification pass over `docs/architecture`:

1. **Hangs together** — every cross-reference resolves to a doc that agrees with it; no doc contradicts a sibling;
   the index describes what each doc actually contains.
2. **Precisely describes the code** — spot re-verification of each doc's load-bearing claims against the tree.
3. **In-flight design inventory** — enumerate the design documents that are in flight (designed but not fully
   implemented), in a table: summary, status, links to the design doc and the owning plan/step doc.

## Open decisions (settle at the owning slice)

- **(a)** `3.3-static-starlark-codegen.md` — rewrite vs. archive as historical.
- **(b)** `8-rust-migration.md` — re-ground vs. explicit historical marker.
- **(c)** `package-hierarchy.md` / `package-reference.md` — stay hand-maintained vs. become generated.

## Relationship to other steps

- **Step 34** — delivered 2/2.2/2.3 + the sweep; this step consumes its recorded inventory.
- **Step 39** — disjoint (per-provider `3.5.x` docs vs. framework/system docs); 39 runs first by direction.
- **The PR to develop** — this step (including the exit gate) completes before the PR.
