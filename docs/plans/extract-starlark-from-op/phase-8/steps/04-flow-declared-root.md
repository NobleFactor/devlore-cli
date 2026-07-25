---
step: 4
title: "flow declared a root action provider — its methods are eligible to surface flat under plan.*"
former_title: "flow.Provider declares +devlore:root=true"
status: complete — behavioral tests landed 2026-07-03 (2/2 matrix)
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 4 — flow declared a root action provider

**Status:** `complete` · **Behavioral tests: 2 / 2 landed (2026-07-03)** in `pkg/op/receiver_registry_test.go` — the registry is queried for flow's action+root partition and its RootProviders membership.

## What this step delivers

The flow provider is announced with `RoleAction|RoleRoot` — dispatch = plan-mode action, placement = root — so its
methods become eligible to appear flat (`plan.choose`, `plan.gather`) instead of nested (`plan.flow.choose`). The wiring:

- `+devlore:root=true` on the provider struct (`pkg/op/provider/flow/provider.go:30`).
- The star codegen's `struct_root` parser (`star/extensions/com.noblefactor.devlore.Actions/commands/generate.star`)
  reads the directive and sets the `RoleRoot` bit in the generated announcement: `op.AnnounceProvider(...,
  op.RoleAction|op.RoleRoot, ...)` (`pkg/op/provider/flow/gen/provider.gen.go:17`). Regeneration-stable.

This is **plumbing activation only** — the actual flat resolution under `plan.*` is wired at the plan provider
(steps 6–7); this step just gives flow the role that makes it eligible.

**Directive renamed 2026-06-15 (`fsroot` → `root`).** The directive was `+devlore:fsroot=true` while the role enum
(`RoleRoot`), registry method (`RootProviders`), and design (D12) all used plain `root` — a silent-failure trap, since
the obvious `+devlore:root=true` matched nothing. Renamed across the star parser and the source; `fsroot` now means only
the filesystem (`pkg/fsroot`). Four prose references remain in `plan/provider.go` (the other session's file) — pending.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestFlowProvider_RegisteredAsActionRoot` | flow's registered roles == `RoleAction\|RoleRoot`; `Dispatch()`==`RoleAction`; `Placement()`==`RoleRoot` | ☑ | ✅ |
| 2 | `TestRootProviders_IncludesFlow` | the receiver registry's `RootProviders()` includes flow (the directive actually reached the registry) | ☑ | ✅ |

**Behavioral coverage: 2 / 2 (verified 2026-07-03).** The former manual smoke-check ("`RootProviders()` returns flow with `roles=0x102`") is now the test pair; it was
a manual smoke-check, not a test.

## Proof run

Verified 2026-07-03: `pkg/op` passes under `make test` with both matrix tests present in
`pkg/op/receiver_registry_test.go` — flow's registered roles equal RoleAction|RoleRoot with the zone projections
asserted, and flow appears in `RootProviders()`. (The step-2 directive test additionally proves the codegen thread.)
