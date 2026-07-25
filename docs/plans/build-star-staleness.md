---
title: "Always rebuild the in-tree star generator"
issue: TBD
status: complete
created: 2026-07-25
updated: 2026-07-25
---

# Plan: Always rebuild the in-tree star generator

Chartered follow-up 2 of
[fix-windows-build-and-guide-category](fix-windows-build-and-guide-category.md).

## Summary

The `build/star:` rule had no prerequisites, so once any `build/star` binary existed,
make considered it permanently up to date and codegen silently ran a stale generator.
Observed failure: a June-27 binary rejected the `chmod` keyword argument that
`generate.star` now passes, failing `make test` until a by-hand `make star`.

## Changes

1. `Makefile` — `build/star:` becomes `build/star: FORCE` delegating to `$(MAKE) star`
   on every run, with the classic empty `FORCE:` target. The comment documents the
   staleness rationale.

The LKG escape hatch is unaffected: when `build/star.lkg` exists, `$(STAR)` resolves to
it and the `build/star` rule is never consulted.

## Verification

- `make build/star` twice: the rule fires both times; a warm, up-to-date rebuild costs
  ~2.1–2.3 s (Go build cache), matching the "seconds" cost claim.
- `make test` — pass end to end (codegen with the freshly built generator, then tests).
