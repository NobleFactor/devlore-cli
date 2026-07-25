---
step: 15
title: "Convertibility infrastructure — SourceConverter / TargetConverter + op.Convert + typesAreInterconvertible (D8/D9)"
former_step: 18
former_title: "CanConvert on Converter + plan.Provider.CanConvertTypes"
status: complete — conversion engine + symmetric D8 probe directly proven (probe test landed 2026-07-03 with step 16)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 15 — Convertibility infrastructure (formerly step 18)

**Status:** `complete`. The conversion engine is directly and well tested, and — as of 2026-07-03, landed with the
step-16 parcel — the D8 interconvertibility **probe** has a direct test covering every symmetric arm. The historical
row-title defect (`plan.Provider.CanConvertTypes`, a method that never existed) has been corrected in the plan row.

## What this step delivers

The original D9 "single `op.Converter`" spec refined into two opt-in interfaces (`pkg/op/interfaces.go`):

- `op.SourceConverter` — value-side: `CanConvertTo(target)` + `ConvertTo(target)`.
- `op.TargetConverter` — target-side: `CanConvertFrom(source)` + `ConvertFrom(value)`.

Two consumers:

- `op.Convert` (`convert.go`) — the **conversion engine**: identity → assignability → `SourceConverter` →
  `TargetConverter` → resource-constructor cascade.
- `op.typesAreInterconvertible(a, b)` (`convert.go:355`) — the **D8 plan-time probe**: returns whether a value of type
  `a` can fill a slot typed `b` **or vice versa** (symmetric). The row's `plan.Provider.CanConvertTypes` never
  materialized as a method — this unexported helper is the actual landing. Consumed at `validate.go:235` (type-check
  pass), `subgraph.go:685` (bubble-up merge dedup), `cmd/writ/adopt/plan.go:36` (variable interconvertibility);
  `planner.go:316` probes `SourceConverter.CanConvertTo` directly in slot-fill.

Nine Resource types opt in (`CanConvertTo`/`CanConvertFrom`): `ResourceBase`, `envValue`, file, function, service, git,
mem, appnet, pkg.

## Test matrix

| # | Test | Proves | Grade |
|---|---|---|---|
| 1–8 | `TestConvert_{Identity,Assignability,Slice,Map,SourceConverter,TargetConverter,ResourceConstructor,ResourceConstructor_ErrOnNilContext}` (`convert_test.go`) | the conversion engine across all cascade arms | ✅ |
| 9 | `TestEnvValue_CanConvertTo` (`env_value_test.go:48`) | the `envValue` source probe (nil/interface/string cases) | ✅ |
| 10 | `TestCanConvertTo` (`mem/resource_test.go:359`) | the mem Resource source probe | ✅ |
| 11 | `TestTypesAreInterconvertible` (`convert_test.go`, landed 2026-07-03) | the symmetric `a↔b` relation directly: identity, both assignability orders, source-only and target-only opt-ins in **both argument orders** (the symmetric-acceptance arms), neither-direction, nil guards | ✅ |
| — | direct probes for file/function/service/git/appnet/pkg/ResourceBase `CanConvert*` | 7 of 9 opt-ins are exercised only transitively — non-blocking backfill | ☐ |

**Coverage:** the engine is fully proven (rows 1–8), and row 11 now targets the symmetric `a↔b` semantics directly.
Both `typesAreInterconvertible` call sites are behaviorally covered: the **bubble-up** site (`subgraph.go:685`) — true
branch via the 53 valid `.star` fixtures, false branch via `TestValidateGraph_TypeCollision` — and the
**`checkPromiseTypes`** site (`validate.go:265`) via step 16's `TestValidateGraph_PromiseType_*` trio + the
`test_promise_type_mismatch.star` fixture. (Historical correction stands: `test_writ_adopt_type_mismatch.star`'s "not
assignable to declared type" comes from `helpers.go:122`, a value-side slot-fill check — **not**
`typesAreInterconvertible`.)

## Proof run

Verified 2026-07-03: `pkg/op` passes under `make test` with `TestTypesAreInterconvertible` present (ten subtests over
the relation's arms, reusing `convert_test.go`'s `sourceConverter`/`targetConverter` fixtures). The symmetric-vs-
directional consequence for D8 is pinned by step 16's `TestValidateGraph_PromiseType_ReverseOnlyConvertible_Passes`
and documented there.

## Disposition

`complete`. The 2026-06-16 follow-ups both closed: (1) `TestTypesAreInterconvertible` landed 2026-07-03 with the
step-16 parcel; (2) the plan-row title now names `typesAreInterconvertible` instead of the never-existent
`plan.Provider.CanConvertTypes`. Remaining non-blocking backfill (matrix final row): direct `CanConvert*` probes for
the 7 transitively-exercised Resource opt-ins.
