---
title: "template is documented pure and reads the host environment"
issue: https://github.com/NobleFactor/devlore-cli/issues/680
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: template is documented pure and reads the host environment

**Epic:** [#454 — Design and documentation debt](https://github.com/NobleFactor/devlore-cli/issues/454)
**Design:** [3.5.7-template-provider.md](../architecture/3.5.7-template-provider.md) ·
[3.6-method-classification.md](../architecture/3.6-method-classification.md)
**Related:** [method-classification.md](method-classification.md)

## Summary

`3.5-provider-catalog.md` describes the template provider as "pure" with "no filesystem."
`pkg/op/provider/template/provider.go:110` maps `"Env": os.Getenv` into the template function map, so any template
string can read any environment variable on the rendering machine. The behaviour is deliberate and documented at
`provider.go:105`; the **catalog's claim of purity is not**. This plan makes the documentation honest and hands
the code question to the classification design that owns it.

## Goals

1. **An accurate claim** — pure over its arguments, ambient when the template calls `Env`
2. **A named limit** — record that this is the first method whose purity depends on argument *content*, which no
   static classification can decide
3. **A ruling, in the right place** — the code question is ruled and owned by #683

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `Env` in the function map | ✅ Deliberate | `provider.go:110`; rationale at `provider.go:105` — graphs are transportable, so resolving at plan time would embed environment values in persisted graph documents |
| Catalog claim | ❌ "pure", "no filesystem" | `3.5-provider-catalog.md` |
| `3.5.7` claim | ❌ Same | |
| `3.6` open question | ✅ Filed | "remove `Env` from the function map, or make the map per-runtime configurable" |

## Why this matters beyond one row

`3.6` classifies a method by its signature. `template.render_text` has the signature of a pure function and the
behaviour of a query, decided by a string passed at runtime. It is the counterexample that bounds what per-method
classification can promise, and it belongs in the design record as such — not as a footnote in a catalog row.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Correct the catalog row — pure over its arguments; ambient when the template calls `Env` | ☐ |
| 2 | Correct `3.5.7-template-provider.md`'s summary to match | ☐ |
| 3 | Record the data-dependent-purity limit in `3.6` §3, citing `template.render_text` as the case | ☐ |
| 4 | **`Env` is ruled** — it consults **declared runtime variables**, the way `make` does, not the ambient process environment. Tracked as [#683](https://github.com/NobleFactor/devlore-cli/issues/683) against `op.VariableResolver`; dispatch-time resolution is unchanged, only the source | ☐ |

## Verification

- No document describes template rendering as unconditionally pure
- `3.6` §3 names the data-dependence limit
- Step 4 is ruled and carried by #683; no document leaves the source of `Env` unstated
