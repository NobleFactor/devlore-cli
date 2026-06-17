---
step: 2
title: "Provider role zones — declare dispatch mode and root placement independently, validated at announce"
former_title: "+devlore:root=true directive & ProviderRole placement zone"
status: incomplete — pending tests
proof_run: 2026-06-15
parent: ../../phase-8.md
---

# Step 2 — Provider role zones: orthogonal dispatch-mode + placement bits, with `+devlore:root=true` opt-in

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 7 written** · deliverable compiles but is unverified.

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
`pkg/op/receiver_type_test.go`; 4–6 → `pkg/op/receiver_registry_test.go` (new); 7 → the `cmd/star` codegen tests.

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestProviderRole_ZoneBitLayout` | `RoleModule`=0x01, `RoleAction`=0x02, `RoleRoot`=0x100; masks 0x00FF / 0xFF00 | ☐ | — |
| 2 | `TestProviderRole_Dispatch_ReturnsDispatchZoneOnly` | `(RoleAction\|RoleRoot).Dispatch()` == `RoleAction`; `RoleRoot.Dispatch()` == 0 | ☐ | — |
| 3 | `TestProviderRole_Placement_ReturnsPlacementZoneOnly` | `(RoleAction\|RoleRoot).Placement()` == `RoleRoot`; `RoleAction.Placement()` == 0 | ☐ | — |
| 4 | `TestAnnounceProvider_PanicsWithoutDispatchBit` | announcing with `RoleRoot`-only (no dispatch bit) panics | ☐ | — |
| 5 | `TestAnnounceProvider_AcceptsDispatchBit` | `RoleModule` / `RoleAction` announce without panic | ☐ | — |
| 6 | `TestReceiverRegistry_RootProviders_ReturnsPlacementRootOnly` | `RootProviders()` returns exactly the `RoleRoot`-placed providers, excludes non-root | ☐ | — |
| 7 | `TestGenerate_RootDirective_ThreadsRoleRoot` | `+devlore:root=true` → generated `AnnounceProvider` call includes `\|op.RoleRoot` | ☐ | — |

**Behavioral coverage: 0 / 7.** The runtime smoke-check recorded in the plan ("`RootProviders()` returns flow with
`roles=0x102`") was a manual observation, not a test.

## Proof run

```
$ go test ./pkg/op/ -run 'Role|Dispatch|Placement|RootProvider|AnnounceProvider' -v -count=1
--- PASS: TestActivationRecord_DispatchChild_NotInstalled   # unrelated — matched "Dispatch" by accident
ok  github.com/NobleFactor/devlore-cli/pkg/op
```

No test exercises the zones, the accessors, the dispatch-bit validation, or `RootProviders`. The step reaches
`complete` when all seven rows are ☑ and ✅.

## Remaining to reach `complete`

Write tests 1–7. 1–3 are pure-function table tests over `ProviderRole`. 4–6 are white-box (`package op`) tests against
the process registry — 4/5 assert the `assert.Truef` panic boundary, 6 announces a root and a non-root provider and
checks the partition. 7 is a codegen golden/inspection test that the `+devlore:root=true` directive emits `op.RoleRoot`.
