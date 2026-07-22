---
step: 39
title: "Complete per-provider design docs — a 3.5.x document for every provider"
status: COMPLETE 2026-07-22 — all 14 docs landed (3.5.3–3.5.16), each with a greped test-matrix status companion, catalog row, and index entry; surfaced coverage gaps intaken by step 52 (rows 1–18)
proof_run: n/a (documentation)
parent: ../../phase-8.md
---

# Step 39 — Complete per-provider design docs

**Status:** `COMPLETE` 2026-07-22. All 14 docs landed: `3.5.3-plan` (2026-07-21), `3.5.4-file`, batch 1
(`3.5.5-json`, `3.5.6-yaml`, `3.5.7-template`), batch 2 (`3.5.8-shell`, `3.5.9-powershell` — the catalog's only
zero-coverage provider; step-52 rows 11–12), batch 3 (`3.5.10-git` — gaps → rows 14–15, `3.5.11-service`), batch 4
(`3.5.12-appnet`, `3.5.13-encryption`, `3.5.14-function` — gaps → rows 16–18), and the final batch
(`3.5.15-regexp`, `3.5.16-ui` — whose four stale `[status.UI]` code doclinks were fixed en route). Every doc has a
status companion carrying the greped per-method test matrix, a linked catalog row, and an index entry; every
surfaced coverage gap is intaken by [step 52](52-test-backfill-round-2.md).

Chartered from the 2026-07-03 reference audit: the provider catalog
([3.5-provider-catalog.md](../../../../architecture/3.5-provider-catalog.md)) lists **14 of 18 providers with no design
doc** — every announced method of those providers is documented only in code comments and scattered step docs.

## The gap

Providers with design docs today: archive (3.5.1), flow (3.5.2), pkg/platform (3.4), elevator/elevation (6.1),
mem-as-resource (4.2). Without: **plan** (the entire planning API — `Plan`, `Case`, `Variable`,
`AssembleDefinition`, `SaveDefinition`/`LoadDefinition`, `Spec`, `Run`, `Origin`, `Clear`, `ResolveAttr`), **file**
(the largest action surface and owner of the unified file-mutation core), appnet, encryption, function (action side —
`function.call`), git, json, powershell, regexp, service, shell, template, ui, yaml.

## Deliverable

One `3.5.x` design doc **plus its `.status.md` companion** per provider, on the 3.5.2 pattern:

1. Thesis and role; every announced method summarized (the completeness bar: a doc that covers 4 of 7 surface
   methods is the defect this step exists to prevent).
2. An API-surface section with the Go signatures.
3. A usage section showing both planning APIs where the provider is `planned`/`both` (the 3.5.2 §11 shape).
4. The status doc carries the per-method **test matrix** — Go vs Starlark, named tests and fixtures, open gaps —
   verified against the tree, not asserted (the 2026-07-03 audit bar).
5. A linked catalog row.

## Priority order

1. **plan** (3.5.3) — the planning API is the product's front door and has no design doc.
2. **file** (3.5.4) — the largest surface; the unified mutation-receipt core deserves its record.
3. The remaining `planned`/`both` action providers (appnet, encryption, function, git, json, powershell, service,
   shell, template, yaml).
4. The light surfaces (regexp, ui).

## Relationship to other steps

Step 34 rewrites the *execution-model* architecture docs (2/2.2/2.3); this step covers the *provider* documents —
complementary, not overlapping. The test-matrix gaps these docs surface are intaken by
[step 52](52-test-backfill-round-2.md) (step 24 — the original backfill — closed 2026-07-18 with its enumerated
scope delivered; it does not reopen).
