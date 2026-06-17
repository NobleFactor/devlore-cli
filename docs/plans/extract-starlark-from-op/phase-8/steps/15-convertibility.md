---
step: 15
title: "Convertibility infrastructure — SourceConverter / TargetConverter + op.Convert + typesAreInterconvertible (D8/D9)"
former_step: 18
former_title: "CanConvert on Converter + plan.Provider.CanConvertTypes"
status: complete — conversion engine directly proven; the symmetric D8 probe proven only transitively
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 15 — Convertibility infrastructure (formerly step 18)

**Status:** `complete` with two caveats. The conversion engine is directly and well tested; the D8 interconvertibility
**probe** has no direct test (transitive only); and the row title names a method — `plan.Provider.CanConvertTypes` —
**that does not exist**.

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
| — | `TestTypesAreInterconvertible` | **GAP** — the symmetric `a↔b` relation directly: source-only (a→b, not b→a), target-only, neither, identity, assignable | ☐ |
| — | direct probes for file/function/service/git/appnet/pkg/ResourceBase `CanConvert*` | 7 of 9 opt-ins are exercised only transitively | ☐ |

**Coverage:** the engine is fully proven (rows 1–8). `typesAreInterconvertible` is exercised **transitively at one of
its two call sites** — the **bubble-up** site (`subgraph.go:685`): true branch via the 53 valid `.star` fixtures
(compatible bubble-up), false branch via `TestValidateGraph_TypeCollision` ("incompatible types"). Its **other** call
site — `checkPromiseTypes` (`validate.go:234`, the Promise→slot type-check) — is **entirely untested** (see step 16),
and no test targets the symmetric `a↔b` semantics directly. (Correction: `test_writ_adopt_type_mismatch.star`'s "not
assignable to declared type" comes from `helpers.go:122`, a value-side slot-fill check — **not**
`typesAreInterconvertible`.)

## Proof run

```
$ go test ./pkg/op/ -run 'Convert' -v        # TestConvert_* x8 PASS, CanConvertTo probes PASS
$ grep -rn 'CanConvertTypes' pkg cmd          # (none — method does not exist)
$ grep -rn 'TestTypesAreInterconvertible' .   # (none — no direct probe test)
```

## Disposition

`complete` for the conversion engine — directly and thoroughly proven. Two follow-ups, neither blocking:
1. Add `TestTypesAreInterconvertible` covering the symmetric `a↔b` arms in isolation (the one feature `op.Convert`'s
   tests do not exercise, since `Convert` is one-directional).
2. Fix the row title: `plan.Provider.CanConvertTypes` does not exist; the D8 probe is `op.typesAreInterconvertible`.
