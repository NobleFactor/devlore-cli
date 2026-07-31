---
title: "Class a: range-value copies"
issue: TBD
status: complete
created: 2026-07-31
updated: 2026-07-31
---

# Plan: Class a — range-value copies

First rung of the 4b-3 ladder (per-class discuss-then-fix, ruled 2026-07-30; this session
owns the ladder). Class a's original mechanical checks (octalLiteral, sloppyReassign,
paramTypeCombine, emptyStringTest, misspell, unlambda, sprintfQuotedString, errorlint,
goimports) were retired by a parallel session; the residual is rangeValCopy.

## Site survey

All seventeen bodies read: every one is read-only (grouping appends, report formatting,
tallies, plan construction, stack decode) — zero mutating-copy cases. Three of the
seventeen range over `Inventory.Entries`, a `map[string]Entry`, whose values are
unaddressable — the pointer transform is impossible there.

## Changes

1. Fourteen slice loops: `for i := range s` with `entry := &s[i]` (or direct indexing for
   one-field bodies) — the per-iteration copy goes away; appends and by-value calls
   dereference (`*entry`), netting one copy where the old form made two.
2. Three map loops (`decommission.go` selectEntries, `status.go` report build,
   `upgrade.go` copied-entry filter): justified suppression —
   `//nolint:gocritic // rangeValCopy: map values are unaddressable; the per-iteration
   copy is the read.`
3. Value storage stays value storage (settled in discussion 2026-07-31: identity-as-data;
   interning is reserved for multi-referent identities, which the ResourceCatalog already
   implements).

## Verification

- rangeValCopy: 17 → 0 (uncapped).
- `make vet` pass; full `make test` pass; `gofmt -l` clean.
