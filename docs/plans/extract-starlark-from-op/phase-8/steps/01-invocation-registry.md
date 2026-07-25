---
step: 1
title: "Plan-time invocation ledger — labeled, ordered handles for every plan.* call"
former_title: "Invocation registry + options types + plan.options builder"
status: complete — behavioral tests landed 2026-06-18 (matrix 1–9 verbatim; #10 proven as TestInvocation_Binding)
proof_run: 2026-06-15
parent: ../../phase-8.md
---

# Step 1 — Plan-time invocation ledger: labeled, ordered handles for every plan.* call

**Status:** `complete` · **Behavioral tests: 10 / 10 landed (2026-06-18)** · matrix rows 1–9 exist verbatim in `pkg/op/invocation_registry_test.go`; row 10's contract is proven by `TestInvocation_Binding` (`SlotValue` was renamed `Binding`).

## What this step delivers

The session-scoped ledger of every plan-time invocation:

- **`op.Invocation`** (`pkg/op/invocation.go:18`) — the handle dispatch constructs for every `plan.*` call and the
  starlark value it returns. Carries `Target` (the `ExecutableUnit` to dispatch), `Result` (the value-side `*Promise`),
  and `Label`. `Binding()` (né `SlotValue`) delegates to the promise so a consumer slot binds to the producer by `UnitRef` (the D5
  detachment contract).
- **`op.InvocationRegistry`** (`pkg/op/invocation_registry.go:18`) — ordered creation list + label index + a
  per-`provider.method` auto-label counter, mutex-guarded: `Register` (append + index, **rejects duplicate labels**),
  `ByLabel`, `AutoLabel` (`"<provider.method>#<N>"`, monotonic per method), `All` (ordered shallow copy), `Reset`.
  Consumed by the plan-end orphan walk (step 14) and the type-check pass (step 16).

**Scope drift from the original plan:** the `Options{Label, RetryPolicy}` struct and the `plan.options(...)` builder
named in this step were **removed** — no `Options` type in the tree, not in the plan provider's announce map.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files: tests 1–9 in
`pkg/op/invocation_registry_test.go`, test 10 in `pkg/op/invocation_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| — | `"InvocationRegistry"` in the announced method list (`plan/gen/receiver_type.gen_test.go:120`) | announcement only — **not** behavior | ☑ (generated, incidental) | ✅ |
| 1 | `TestInvocationRegistry_New_IsEmpty` | a fresh ledger holds nothing | ☑ | ✅ |
| 2 | `TestInvocationRegistry_Register_AppendsInCreationOrder` | `All()` returns entries in creation order | ☑ | ✅ |
| 3 | `TestInvocationRegistry_Register_IndexesByLabel` | `ByLabel` finds a registered invocation | ☑ | ✅ |
| 4 | `TestInvocationRegistry_Register_RejectsDuplicateLabel` | duplicate label errors **and** mutates nothing | ☑ | ✅ |
| 5 | `TestInvocationRegistry_ByLabel_ReturnsNilForUnknown` | unknown label → nil | ☑ | ✅ |
| 6 | `TestInvocationRegistry_AutoLabel_IncrementsPerProviderMethod` | per-method monotonic `<pm>#N`, independent across methods | ☑ | ✅ |
| 7 | `TestInvocationRegistry_All_ReturnsIndependentCopy` | a caller can't corrupt the ledger via the returned slice | ☑ | ✅ |
| 8 | `TestInvocationRegistry_Reset_ClearsEntriesAndCounters` | `Reset` wipes entries **and** the auto-label counters | ☑ | ✅ |
| 9 | `TestInvocationRegistry_Concurrent_IsRaceFree` (`-race`) | the single mutex guards concurrent use | ☑ | ✅ |
| 10 | `TestInvocation_Binding` (matrix name: SlotValue delegation — the API was renamed) | `Binding` binds consumer→producer by `UnitRef` | ☑ | ✅ |

**Behavioral coverage: 10 / 10 (verified 2026-07-03).** The codegen name-assertion additionally proves the method is
*announced*; the matrix tests prove the behavior.

## Proof run

Verified 2026-07-03: `pkg/op` passes under `make test` with all ten matrix tests present — rows 1–9 verbatim in
`pkg/op/invocation_registry_test.go` (landed 2026-06-18), row 10's contract as `TestInvocation_Binding` in
`pkg/op/invocation_test.go` (`SlotValue` was renamed `Binding`; the matrix predates the rename). The historical
2026-06-15 proof run recorded zero tests; this doc's status lagged the 2026-06-18 landing until the 2026-07-03
consistency audit.
