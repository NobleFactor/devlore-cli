---
title: "elevator becomes elevation — the settled name reaches the tree"
issue: https://github.com/NobleFactor/devlore-cli/issues/679
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: elevator becomes elevation — the settled name reaches the tree

**Epic:** [#520 — Elevation policy](https://github.com/NobleFactor/devlore-cli/issues/520)
**Design:** [6.1-privilege-elevation.md](../architecture/6.1-privilege-elevation.md)
**Related:** [step 38 — elevation policy](extract-starlark-from-op/phase-8/steps/38-elevation-policy.md)

## Summary

[`6.1-privilege-elevation.md:8`](../architecture/6.1-privilege-elevation.md) records the name as **settled**:
"Provider `elevation.Provider` in package `elevation` (`pkg/op/provider/elevation`)." The code is still
`elevator`, 6.1's own status companion still carries an unchecked box calling `elevator` a working name, and
6.1's body cites code paths that will not resolve after the rename. This plan carries the settled name into the
tree. **The stub stays** — it is the scaffold step 38's design builds on.

## Goals

1. **One name, everywhere** — the settled `elevation` in code, architecture, and every path citation
2. **The stub survives** — unannounced by design, correctly named, and legible as deliberate rather than dead
3. **The record closes** — 6.1's stale name checkbox ticked, its stale code links repaired
4. **The catalog stops lying** — an unannounced provider is not listed with a live role

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| The name | ✅ Settled | `6.1-privilege-elevation.md:8` — "Names — settled" |
| Package and directory | ❌ `elevator` | `pkg/op/provider/elevator/{provider,broker,config}.go` |
| Package doc comment | ❌ Calls the name provisional | contradicts 6.1's settled ruling |
| 6.1 status checkbox | ❌ "☐ Name the provider — `elevator` are working names" | stale |
| 6.1 code links | ❌ Two stale | cites `pkg/op/provider/elevator/broker.go:14` and `:53` |
| Catalog row | ❌ `elevator`, role `planned` | no `gen/`, absent from the inventory, zero importers |

**52 `elevator` references across 22 files** — 10 Go, 42 documentation. No type is named `Elevator`; the rename
is package-level only.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | `git mv pkg/op/provider/elevator pkg/op/provider/elevation` | ☐ |
| 2 | `package elevator` → `package elevation` in all three files | ☐ |
| 3 | Rewrite the package doc comment — the **name** is settled per 6.1; the **config shape** and method bodies remain provisional, and the provider stays unannounced | ☐ |
| 4 | Update the `ElevationOffer/Elevator` TODO at `pkg/platform/helpers_unix.go:70` | ☐ |
| 5 | Repair 6.1's two stale code links | ☐ |
| 6 | Tick the settled-name checkbox in `6.1-privilege-elevation.status.md` | ☐ |
| 7 | Catalog: `elevation` / `elevation.*`, moved under a **"Designed, not announced"** heading carrying no role | ☐ |
| 8 | Sweep the remaining documentation references — **rule: stale code paths are corrected, historical prose stays** | ☐ |

## Explicitly not in scope

Renaming `Config` → `ProviderConfig`, `EnvironmentConfig`, or `TokenProviderConfig`. `6.1`'s status still carries
"☐ Settle the full config shape," and step 38's open questions 1–4 are unresolved. **The package name is settled;
the config shape is not.** Renaming those types now front-runs #520's design work.

## Verification

- `make check` green
- Zero `elevator` references outside historical plan prose
- Every code-path citation to the package resolves
- The provider remains unannounced: no `gen/`, no inventory entry — verified, not assumed
