---
step: 2
title: "Provider role zones — declare dispatch mode and root placement independently, validated at announce"
former_title: "+devlore:root=true directive & ProviderRole placement zone"
status: complete — behavioral tests landed 2026-07-03 (7/7 matrix; test 7 renamed TestRootDirective_ThreadsRoleRootThroughAnnounce)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 2 — Provider role zones: orthogonal dispatch-mode + placement bits, with `+devlore:root=true` opt-in

**Status:** `complete` · **Behavioral tests: 7 / 7 landed (2026-07-03)** · rows 1–3 in `pkg/op/receiver_type_test.go`, rows 4–7 in `pkg/op/receiver_registry_test.go` (new).

## What this step delivers

A provider can declare **how** its methods dispatch and **where** they surface, as two orthogonal zones of one bitflag:

- **`ProviderRole`** (`pkg/op/receiver_type.go:46`) — partitioned into a **dispatch zone** (bits 0–7: `RoleModule` =
  immediate-mode global, `RoleAction` = plan-mode graph-node creator) and an orthogonal **placement zone** (bits 8–15:
  `RoleRoot` = surface methods flat at the namespace root instead of nested under the provider name).
- **`Dispatch()` / `Placement()`** (`receiver_type.go:81`/`:87`) — project a role onto each zone via the masks
  `0x00FF` / `0xFF00`.
- **`AnnounceProvider`** (`pkg/op/receiver_registry.go:200`) — **refuses (panics on) a provider that sets no dispatch
  bit** (`:204`).
- **`ReceiverRegistry.RootProviders()`** (`receiver_registry.go:364`) — enumerates the root-placed providers, which is
  what lets `plan.*` promote their methods.
- **`+devlore:root=true`** codegen directive — threads `RoleRoot` into a provider's generated `AnnounceProvider` call.

What we get from it: `flow`'s methods appear as `plan.choose` (flat, root-placed) rather than `plan.flow.choose`
(nested), and the framework guarantees every announced provider declares a dispatch mode.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). Files: 1–3 →
`pkg/op/receiver_type_test.go`; 4–7 → `pkg/op/receiver_registry_test.go`.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestProviderRole_ZoneBitLayout` | `RoleModule`=0x01, `RoleAction`=0x02, `RoleRoot`=0x100; masks 0x00FF / 0xFF00 | ☑ | ✅ |
| 2 | `TestProviderRole_Dispatch_ReturnsDispatchZoneOnly` | `(RoleAction\|RoleRoot).Dispatch()` == `RoleAction`; `RoleRoot.Dispatch()` == 0 | ☑ | ✅ |
| 3 | `TestProviderRole_Placement_ReturnsPlacementZoneOnly` | `(RoleAction\|RoleRoot).Placement()` == `RoleRoot`; `RoleAction.Placement()` == 0 | ☑ | ✅ |
| 4 | `TestAnnounceProvider_PanicsWithoutDispatchBit` | announcing with `RoleRoot`-only (no dispatch bit) panics | ☑ | ✅ |
| 5 | `TestAnnounceProvider_AcceptsDispatchBit` | `RoleModule` / `RoleAction` announce without panic | ☑ | ✅ |
| 6 | `TestReceiverRegistry_RootProviders_ReturnsPlacementRootOnly` | `RootProviders()` returns exactly the `RoleRoot`-placed providers, excludes non-root | ☑ | ✅ |
| 7 | `TestRootDirective_ThreadsRoleRootThroughAnnounce` (matrix name: TestGenerate_RootDirective_ThreadsRoleRoot — renamed; no generation runs, the checked-in generated announce is inspected at runtime) | `+devlore:root=true` → flow announces with RoleRoot and lands in RootProviders() | ☑ | ✅ |

**Behavioral coverage: 7 / 7 (verified 2026-07-03).** The former manual smoke-check ("`RootProviders()` returns flow
with `roles=0x102`") is now test 7, which inspects the announced roles at runtime.

## Proof run

Verified 2026-07-03: `pkg/op` passes under `make test` with all seven matrix tests present — rows 1–3 as
pure-function table tests in `receiver_type_test.go`, rows 4–6 white-box against the process registry in
`receiver_registry_test.go` (partition fixtures announced at package init, ahead of the registry singleton's
sync.OnceValue snapshot), and row 7 as the runtime inspection of flow's generated announce (renamed from the matrix's
TestGenerate_ prefix — no generation runs; the checked-in generated announce is the subject).
