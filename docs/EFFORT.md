# Project Effort Report

Generated: 2026-03-04

## Code Quality Assessment

The architecture is a 9, the implementation is an 8, dragged down by type
erasure that the language forces on the codebase.

**Architecture discipline is high.** The provider/action/resource/graph
separation is clean. The three-mode split (action/immediate/planned) is
consistent across all 20 providers. The compensation pattern is
well-executed — `moveToRecovery` / `restoreFromRecovery` with
platform-specific recovery bases, same-device rename guarantees,
UUID-named recovery paths. This isn't accidental — someone thought about
failure modes.

**The reflection code is the best it can be given Go's constraints.**
`coerceSlotValue` has a clear coercion chain with well-ordered fallbacks.
`classifyActionReturn` and `classifyReturn` are the same logic split
correctly for two contexts (Go-native vs Starlark). `shadowResult` handles
value types, pointer types, and slices of resources — all the cases that
actually arise. The code comments explain *why*, not *what*.

**Test ratio is exceptional.** 32,791 test LOC against 34,533 source LOC —
nearly 1:1. The W08 refactoring (source dropped 3,400 lines, tests grew
6,000) shows willingness to delete code and backfill coverage. That's rare.

**The codegen boundary is respected.** Generated files are never hand-edited.
The `star` tool is the source of truth. `Compensate*` methods are required
to exist (panic at registration if missing). These are guardrails that
prevent drift.

**The "no legacy" principle is enforced.** No backward compatibility shims,
deprecated paths, or fallback behavior. The code does one thing.

**Where it's weaker:**

- **The `Result = any` / `UndoState = any` problem.** Type information is
  destroyed at the reflection boundary and recovered via runtime assertions
  that can fail silently or with unhelpful errors. This is the single
  biggest quality issue — and it's forced by Go's type system, not by
  design intent.

- **The `init()` registration pattern** is invisible coupling. Constructor
  registrations in `resource.go` `init()` functions are
  action-at-a-distance — if someone forgets to import a package, the
  constructor silently doesn't register, and the error surfaces later as a
  coercion failure. Go has no way to make this visible.

- **`sync.Map` for registries** is correct but obscures intent. The
  type-erased `func(any) (any, error)` stored in `constructorRegistry` is
  a consequence of Go's generics limitations — the `RegisterConstructor[T]`
  generic wrapper hides it, but the underlying storage is still untyped.

These three issues are all manifestations of the same root cause: Go's type
system forces runtime workarounds for things that should be compile-time
guarantees. The code quality is high *within* those constraints.

---

## Repositories

- **noblefactor** — Product design documentation (ADRs, strategy, PRDs, architecture)
- **devlore-cli** — Go implementation (source, tests, codegen, CLI)

## Combined Weekly Effort

| Week | Dates | noblefactor (design docs) | devlore-cli (source) | devlore-cli (tests) | Total LOC delta | Commits |
|---|---|---|---|---|---|---|
| W02 | Jan 6–12 | 13,269 (new) | — | — | +13,269 | 5 |
| W03 | Jan 13–19 | +16,468 → 29,737 | 7,660 (new) | 403 (new) | +24,531 | 24 |
| W04 | Jan 20–26 | +5,297 → 35,034 | +13,332 → 20,992 | +5,286 → 5,689 | +23,915 | 70 |
| W05 | Jan 27–Feb 2 | +374 → 35,408 | +10,315 → 31,307 | +3,533 → 9,222 | +14,222 | 42 |
| W06 | Feb 3–9 | — | +48 → 31,355 | +1,480 → 10,702 | +1,528 | 13 |
| W07 | Feb 10–16 | −1,349 → 34,059 | +3,128 → 34,483 | +3,955 → 14,657 | +5,734 | 39 |
| W08 | Feb 17–23 | — | −3,387 → 31,096 | +5,942 → 20,599 | +2,555 | 36 |
| W09 | Feb 24–Mar 2 | — | +2,333 → 33,429 | +9,125 → 29,724 | +11,458 | 9 |
| W10 | Mar 3–5 | — | +1,104 → 34,533 | +3,067 → 32,791 | +4,171 | 7 |

## Summary

| Metric | noblefactor | devlore-cli | Combined |
|---|---|---|---|
| Period | Jan 11 – Feb 12 | Jan 14 – Mar 5 | Jan 11 – Mar 5 |
| Calendar days | 33 | 50 | 54 |
| Active days | 17 | 30 | ~35 |
| Total commits | 87 | 158 | 245 |
| Total PRs | 61 | 181 | 242 |
| Final LOC | 34,059 (docs) | 34,533 (src) + 32,791 (test) | 101,383 |
| Author | 1 | 1 | 1 |

## Phase Narrative

### W02–W03 (Jan 6–19): Product Design

Product design from scratch — 30K words of ADRs, strategy, PRDs,
architecture docs in `noblefactor`. First 7,600 LOC of Go code scaffolded
in `devlore-cli`.

### W04 (Jan 20–26): Peak Velocity

Peak velocity — 70 commits. Design docs finalized. Go codebase exploded
from 7,600 to 21,000 LOC source. Providers, Starlark integration, codegen
pipeline built.

### W05 (Jan 27–Feb 2): Core Build Sprint

Core build sprint — 31,300 LOC source. Execution engine, model providers,
signing, CLI infrastructure.

### W06–W07 (Feb 3–16): Refinement

Design docs consolidated to GitHub issues, source grew modestly, tests
ramped up (10K → 15K).

### W08 (Feb 17–23): Major Refactoring

Source LOC dropped by 3,400 (cleanup/coalescing) while tests grew by 6K.
Net quality improvement.

### W09–W10 (Feb 24–Mar 5): Resource Management

New resource management architecture layered in. Test LOC nearly doubled
(20K → 33K). 58 generated test files added.

## Tooling

All development performed with Claude Code (AI-assisted). Single human
author reviewing, steering, and approving all changes.
