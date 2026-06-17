---
step: 5
title: "plan.Provider three-tier attribute resolution with construction-time collision detection"
former_step: 6
former_title: "plan.Provider discovers root-planned peers; three-tier Attr with collision detection"
status: incomplete — pending tests
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 5 — plan.Provider three-tier attribute resolution (formerly step 6)

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 9 written** · the entire `plan` provider package has **no test files**.

## What this step delivers

`plan.Provider` resolves `plan.<name>` across three tiers (`ResolveAttr`, `pkg/op/provider/plan/provider.go:537`):

- **Tier 2 — promoted builtins** from root-placed providers: `plan.choose`, `plan.gather`, … surface **flat** (built by
  `buildPromotedBuiltins`, `:688`, from `op.ReceiverRegistry.RootProviders`; stored write-once in `promotedBuiltins`).
- **Tier 1 — sub-namespace adapters**: `plan.file.<method>`, `plan.git.<method>` route through a lazily-minted
  `*adapter` (`plan/adapter.go:50`/`:111`). Root providers are **excluded** from Tier 1 (`plan.flow` → nil).
- **Tier 3 — own methods**: `plan.assemble`, `plan.variable`, `plan.save` via the executing receiver path.

`buildPromotedBuiltins` **panics at construction** on any name collision across the tiers — three cases
(`provider.go:720/726/732`): promoted vs. plan's own method; promoted vs. a sub-namespace provider name; the same
promoted method declared on two root providers. This is the dispatch surface that makes `plan.*` work in starlark.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). New files:
`pkg/op/provider/plan/provider_test.go`, `pkg/op/provider/plan/adapter_test.go` (the package has none today).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestProvider_ResolveAttr_Tier2_PromotedBuiltin` | `plan.choose` / `plan.gather` resolve to a `*starlark.Builtin` | ☐ | — |
| 2 | `TestProvider_ResolveAttr_Tier1_SubNamespaceAdapter` | `plan.file` / `plan.git` resolve to a `*adapter` | ☐ | — |
| 3 | `TestProvider_ResolveAttr_Tier3_OwnMethod` | `plan.assemble` / `plan.variable` resolve (own method) | ☐ | — |
| 4 | `TestProvider_ResolveAttr_RootProviderExcludedFromTier1` | `plan.flow` returns nil (root providers not nested) | ☐ | — |
| 5 | `TestProvider_ResolveAttr_UnknownReturnsNil` | `plan.<unknown>` → nil | ☐ | — |
| 6 | `TestProvider_ResolveAttr_TierOrder` | Tier 2 wins over Tier 1 wins over Tier 3 for resolution order | ☐ | — |
| 7 | `TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsOwn` | construction panics; message names both offenders | ☐ | — |
| 8 | `TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsSubNamespace` | construction panics | ☐ | — |
| 9 | `TestAdapter_Attr_RoutesToMethod` | `adapter.Attr("<method>")` returns a builtin; unknown → nil/error | ☐ | — |

**Behavioral coverage: 0 / 9.** The plan-doc's "verified 2026-06-15: `plan.choose` → builtin, `plan.flow` → nil, …" was
a grep/smoke check this session, not a Go test.

## Proof run

```
$ go test ./pkg/op/provider/plan/...
?   github.com/NobleFactor/devlore-cli/pkg/op/provider/plan        [no test files]
ok  github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen    (codegen artifacts only — not resolution/collision)
```

The step reaches `complete` when rows 1–9 are ☑ and ✅.

## Remaining to reach `complete`

Write rows 1–9 (new test files). Construction tests (7, 8) build a `plan.Provider` against a registry seeded with
colliding root providers and assert the panic + message; resolution tests (1–6) assert each tier and the exclusion of
root providers from Tier 1; row 9 covers the Tier-1 adapter's `Attr`. No production change needed — the code is present.
