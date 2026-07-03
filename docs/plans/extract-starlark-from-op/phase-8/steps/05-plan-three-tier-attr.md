---
step: 5
title: "plan.Provider three-tier attribute resolution with construction-time collision detection"
former_step: 6
former_title: "plan.Provider discovers root-planned peers; three-tier Attr with collision detection"
status: complete — behavioral tests landed 2026-07-03 (9/9 matrix + a third-collision companion)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 5 — plan.Provider three-tier attribute resolution (formerly step 6)

**Status:** `complete` · **Behavioral tests: 9 / 9 landed (2026-07-03)** in `pkg/op/provider/plan/provider_test.go` (the package's API suites — gather/lifecycle — predate this; the resolution/collision coverage is new). `buildPromotedBuiltins` gained the `promoteRootMethods` seam so the collision contract is testable against synthetic receiver types without polluting the process registry; a third-collision companion test (duplicate root method) rides along.

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
| 1 | `TestProvider_ResolveAttr_Tier2_PromotedBuiltin` | `plan.choose` / `plan.gather` resolve to a `*starlark.Builtin` | ☑ | ✅ |
| 2 | `TestProvider_ResolveAttr_Tier1_SubNamespaceAdapter` | `plan.file` / `plan.git` resolve to a `*adapter` | ☑ | ✅ |
| 3 | `TestProvider_ResolveAttr_Tier3_OwnMethod` | `plan.assemble` / `plan.variable` resolve (own method) | ☑ | ✅ |
| 4 | `TestProvider_ResolveAttr_RootProviderExcludedFromTier1` | `plan.flow` returns nil (root providers not nested) | ☑ | ✅ |
| 5 | `TestProvider_ResolveAttr_UnknownReturnsNil` | `plan.<unknown>` → nil | ☑ | ✅ |
| 6 | `TestProvider_ResolveAttr_TierOrder` | Tier 2 wins over Tier 1 wins over Tier 3 for resolution order | ☑ | ✅ |
| 7 | `TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsOwn` | construction panics; message names both offenders | ☑ | ✅ |
| 8 | `TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsSubNamespace` | construction panics | ☑ | ✅ |
| 9 | `TestAdapter_Attr_RoutesToMethod` | `adapter.Attr("<method>")` returns a builtin; unknown → nil/error | ☑ | ✅ |

**Behavioral coverage: 9 / 9 (verified 2026-07-03).** Realization notes: row 3 proves both halves of the Tier-3
design — ResolveAttr declines own-method names AND the announced plan receiver type serves them (the goReceiver
path); row 6 proves precedence by white-box injection, since real cross-tier collisions panic at construction and
cannot exist to be observed.

## Proof run

Verified 2026-07-03: `pkg/op/provider/plan` passes under `make test` with all nine matrix tests (plus the
duplicate-root companion) in `provider_test.go`; `plan_announce_test.go` blank-imports plan/gen from the external
test package (an internal import would cycle) so registry.Type("plan") resolves for the Tier-3 row.
