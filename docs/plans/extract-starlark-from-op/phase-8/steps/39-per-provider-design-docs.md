---
step: 39
title: "Complete per-provider design docs — a 3.5.x document for every provider"
status: in-progress — 3.5.3 plan (the priority-1 doc) landed 2026-07-21; 13 providers remain
proof_run: n/a (documentation)
parent: ../../phase-8.md
---

# Step 39 — Complete per-provider design docs

**Status:** `in-progress`. **Landed:** `3.5.3-plan-provider.md` (2026-07-21) and `3.5.4-file-provider.md`
(2026-07-22) — each with a status companion carrying the greped per-method test matrix, a linked catalog row, and an
index entry. Next: the remaining `planned`/`both` action providers (appnet, encryption, function, git, json,
powershell, service, shell, template, yaml), then the light pair (regexp, ui).

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
