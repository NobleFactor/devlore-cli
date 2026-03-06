# Resource Management Design Document — Gap Analysis

## Date: 2026-03-05

Analysis of `docs/plans/resource-management.md` for clarity, internal
consistency, consistency with the code, and gaps.

## 1. Stale Gap Claims (Internal Inconsistency)

The document claims three Phase 5 gaps that are already implemented:

| Claimed Gap | Status | Evidence |
| --- | --- | --- |
| **5e** (CompensableAction pairing) | Done | `action_reflect.go:347-398` — `RegisterReflectedActions` panics on missing AND orphaned compensators |
| **5i** (Immediate mode catalog) | Done | `receiver_reflect.go:176-186` — `SetCatalog()` + `shadowResult()` after dispatch |
| **5k** (Conflict detection) | Done | `resource_catalog.go:57-71` — `Shadow()` returns conflict error for different origins on same URI |

These stale claims appear in seven locations:
- Phase 5 detail sections (5e, 5i, 5k)
- Migration Path Phase 5 summary
- Summary paragraph
- Current State table
- Remaining files table

## 2. Stale 5j Description

Line 1107 describes `RegisterPlanTimeConstructor` as a separate registry.
Code has a single `constructorRegistry` (`starvalue_marshal.go:25`). No
`planTimeConstructorRegistry` exists. Decision #10 (line 553) correctly
says the dual registries are eliminated — contradicting 5j in the same
document.

## 3. 5g Gap — Imprecise Description

The 5g description says "No `catalog.Shadow` for output/result types in
the planned bridge." The code **does** shadow outputs — `shadowOutputParam`
exists. The actual gap: methods with a single Resource parameter (WriteText,
Append, WriteJSON, etc.) are not shadowed because `shadowOutputParam`
requires 2+ Resource params to distinguish source from destination
(line 239: `len(resourceParamIndices) < 2` guard).

## 4. Decision #10 — Implemented But Written As Future

Decision #10 is written in future tense but fully implemented:
- `file.NewResource` is infallible (`file/resource.go:67-69`)
- `Resolve()` is on the `Resource` interface (`resource.go:30`)
- `ResourceBase.Resolve()` is a no-op default (`resource.go:100`)
- `file.Resource.Resolve()` and `git.Resource.Resolve()` do I/O
- `Refresh()` and `RefreshWith()` exist on `file.Resource`
- Single constructor registry — no dual registries

No cross-reference to `phase-9.md` (the implementation plan).

## 5. Decision #11 — No Implementation Status

Decision #11 describes purl adoption. `pkg.Resource` still has only
`Name` — no `Type`, `Version`, or `Purl()`. This is correctly future
work, but the document doesn't mark it as unimplemented. No
cross-reference to `phase-10.md` (the implementation plan).

## 6. Decision #8 Out of Order

Decision #8 appears after Decisions #10 and #11 in the document.
Sequential numbering is broken, confusing for readers.

## 7. Decision #7 "What Will Be" — Aspirational

The "What Will Be" section describes:
- Coercion table replacing constructor registry
- `SlotTypes()` on Action interface
- `ParamSpec` replacing `[]string` in MethodParams
- `go.type_embeds()` introspection in noblefactor-ops
- Provider constructors with context injection

None are implemented. The document doesn't distinguish implemented
from aspirational at the section level.

## 8. Phase 6 Missing From Detailed Breakdown

Implementation Phases jump from Phase 5 to Phase 7. Phase 6 appears
in the Migration Path but has no detailed section header. The detail
exists only in `phase-6.md`.

## 9. Missing Cross-References

- Decision #10 → `phase-9.md` (not linked)
- Decision #11 → `phase-10.md` (not linked)
- Related Documents section doesn't reference implementation plans

## 10. Decision #3 Contradicts 5i

Decision #3 said "No namespace involvement — shadowing is planning-only."
But 5i adds `SetCatalog` + `shadowResult` to immediate mode. The mechanism
exists but is only called from tests — not wired into production immediate
mode. Decision #3 needed updating to reflect the opt-in catalog.

## 11. Decision #7 "What Is" Has Stale Claims

The "What Is" coercion trace showed `NewResource → os.Stat` at execution
time. Per Decision #10, `NewResource` no longer does I/O. Also, the
"Problems" list claimed "No type validation at plan time" — but
`validateSlotType` in `buildPlannedBridge` now validates at plan time.
The Current State table row for planned bridge also understated the
shadowing that exists.

## Recommended Fixes

1. Mark 5e, 5i, 5k as DONE everywhere they appear
2. Fix 5j — remove dual-registry description, note single unified registry
3. Narrow 5g — describe single-Resource output shadowing limitation
4. Reorder Decision #8 before #10 so numbering is sequential
5. Add cross-references from Decisions #10/#11 to implementation plans
6. Add implementation status to each Decision
7. Add Phase 6 section header or cross-reference to phase-6.md
8. Update Decision #3 to reflect opt-in catalog via SetCatalog
9. Update Decision #7 "What Is" traces to reflect Decision #10 (no I/O
   in constructor) and plan-time validation via validateSlotType
10. Update Current State table row for planned bridge to note
    shadowOutputParam for multi-Resource methods

## Fix Status

All recommended fixes have been applied to `resource-management.md`.
