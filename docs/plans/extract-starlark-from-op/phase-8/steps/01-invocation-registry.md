---
step: 1
title: "Plan-time invocation ledger — labeled, ordered handles for every plan.* call"
former_title: "Invocation registry + options types + plan.options builder"
status: incomplete — pending tests
proof_run: 2026-06-15
parent: ../../phase-8.md
---

# Step 1 — Plan-time invocation ledger: labeled, ordered handles for every plan.* call

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 10 written** · deliverable compiles but is unverified.

## What this step delivers

The session-scoped ledger of every plan-time invocation:

- **`op.Invocation`** (`pkg/op/invocation.go:18`) — the handle dispatch constructs for every `plan.*` call and the
  starlark value it returns. Carries `Target` (the `ExecutableUnit` to dispatch), `Result` (the value-side `*Promise`),
  and `Label`. `SlotValue()` delegates to the promise so a consumer slot binds to the producer by `UnitRef` (the D5
  detachment contract).
- **`op.InvocationRegistry`** (`pkg/op/invocation_registry.go:18`) — ordered creation list + label index + a
  per-`provider.method` auto-label counter, mutex-guarded: `Register` (append + index, **rejects duplicate labels**),
  `ByLabel`, `AutoLabel` (`"<provider.method>#<N>"`, monotonic per method), `All` (ordered shallow copy), `Reset`.
  Consumed by the plan-end orphan walk (step 18) and the type-check pass (step 20).

**Scope drift from the original plan:** the `Options{Label, RetryPolicy}` struct and the `plan.options(...)` builder
named in this step were **removed** — no `Options` type in the tree, not in the plan provider's announce map.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files: tests 1–9 in
`pkg/op/invocation_registry_test.go`, test 10 in `pkg/op/invocation_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| — | `"InvocationRegistry"` in the announced method list (`plan/gen/receiver_type.gen_test.go:120`) | announcement only — **not** behavior | ☑ (generated, incidental) | ✅ |
| 1 | `TestInvocationRegistry_New_IsEmpty` | a fresh ledger holds nothing | ☐ | — |
| 2 | `TestInvocationRegistry_Register_AppendsInCreationOrder` | `All()` returns entries in creation order | ☐ | — |
| 3 | `TestInvocationRegistry_Register_IndexesByLabel` | `ByLabel` finds a registered invocation | ☐ | — |
| 4 | `TestInvocationRegistry_Register_RejectsDuplicateLabel` | duplicate label errors **and** mutates nothing | ☐ | — |
| 5 | `TestInvocationRegistry_ByLabel_ReturnsNilForUnknown` | unknown label → nil | ☐ | — |
| 6 | `TestInvocationRegistry_AutoLabel_IncrementsPerProviderMethod` | per-method monotonic `<pm>#N`, independent across methods | ☐ | — |
| 7 | `TestInvocationRegistry_All_ReturnsIndependentCopy` | a caller can't corrupt the ledger via the returned slice | ☐ | — |
| 8 | `TestInvocationRegistry_Reset_ClearsEntriesAndCounters` | `Reset` wipes entries **and** the auto-label counters | ☐ | — |
| 9 | `TestInvocationRegistry_Concurrent_IsRaceFree` (`-race`) | the single mutex guards concurrent use | ☐ | — |
| 10 | `TestInvocation_SlotValue_DelegatesToResultPromise` | `SlotValue` binds consumer→producer by `UnitRef` | ☐ | — |

**Behavioral coverage: 0 / 10.** The lone written reference is a codegen name-assertion that proves the method is
*announced*, not that the registry behaves.

## Proof run

```
$ go test ./pkg/op/ -run 'Invocation' -v -count=1
ok  github.com/NobleFactor/devlore-cli/pkg/op  [no tests to run]
```

No test names or exercises the registry. Re-run this after each test lands; the step reaches `complete` when all ten
rows are ☑ and ✅.

## Remaining to reach `complete`

Write tests 1–10 above. Each maps to a specific contract: ordering (`ordered`), label index + dedup (`byLabel`),
per-method auto-label counter (`counts`), copy-safety of `All`, full clear by `Reset`, mutex-safety under `-race`, and
the `Invocation.SlotValue` → producer-`UnitRef` delegation.
