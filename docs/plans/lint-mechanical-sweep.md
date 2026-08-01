---
title: "Class a proper: the orphaned mechanical sweep, landed"
issue: TBD
status: complete
created: 2026-07-31
updated: 2026-07-31
---

# Plan: Class a proper — the orphaned mechanical sweep, landed

Ownership finding (2026-07-31): the original class-a mechanical fixes — believed already
retired — were sitting **uncommitted in the working tree since 2026-07-28**, produced by a
parallel session and never landed. The ladder's earlier "re-baseline" unknowingly measured
the dirty tree. Per the full-ownership ruling, this change inventories, verifies, and
lands that work.

## Inventory (35 files, diffed against origin/develop via the API)

- **sloppyReassign** — `if err = …` → `if err := …` shadowing fixes across the file
  provider, archive provider, deploy, and tests.
- **paramTypeCombine** — `(a string, b string)` → `(a, b string)` in eight signatures
  (config accessor, file helpers/provider, function literals, plan provider,
  resource catalog, starlarkbridge runtime).
- **emptyStringTest** — canonical empty-string checks in config sync, goast, shellcheck,
  snapshot.
- **misspell** — `cancelled` → `canceled` in graph_executor, node, flow tests.

## Verification

- The tree containing exactly this work passed `make vet`, the full `make test` suite,
  and `gofmt -l` clean earlier today.
- Uncapped lint with this work present: sloppyReassign, paramTypeCombine,
  emptyStringTest, and misspell all at zero.

## Sequencing

Lands after PR #304 (class a residual: rangeValCopy) merges — the commit script guards on
#304's state. Never accumulate PRs.
