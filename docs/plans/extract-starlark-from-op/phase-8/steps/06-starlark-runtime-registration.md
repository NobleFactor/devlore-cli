---
step: 6
title: "StarlarkRuntime predeclared-globals registration branches on access × placement"
former_step: 7
former_title: "StarlarkRuntime access×root registration branches"
status: complete — behavioral tests landed 2026-07-03 (3/3 matrix); fixed the reserved branch's inverted Attr assertion
proof_run: 2026-07-03
parent: ../../phase-8.md
---

# Step 6 — StarlarkRuntime predeclared-globals registration (formerly step 7)

**Status:** `complete` · **Behavioral tests: 3 / 3 landed (2026-07-03)** in `runtime_test.go`, driven by announced
fixture providers selected per-test via `env.Modules`. Writing them caught a production bug: the reserved
RoleModule|RoleRoot branch asserted `err != nil` on the Attr lookup — inverted, so a SUCCESSFUL resolution would
have panicked; fixed to assert success (runtime.go, 2026-07-03).

## What this step delivers

`NewRuntime` (`pkg/op/starlarkbridge/runtime.go:44`) builds the predeclared starlark globals by branching each module on
its **dispatch** zone (`RoleModule`?) × **placement** zone (`RoleRoot`?), per D12 (`:78-128`):

- **planned-only** (`dispatch & RoleModule == 0`) → **skipped** (`:81`); methods surface via `plan.*` instead (flow, git).
- **RoleModule + non-root** → registered as a **top-level global under the provider name** (`:89`): `plan`, `ui`,
  `file`, `template`, …
- **RoleModule + root** → each method installed as **its own top-level global** (`:94-128`); a collision against an
  existing predeclared **panics** via `assert.Failf` (`:111`). Reserved — no phase-8 provider uses this row.

This is what makes `plan` / `ui` / `file` appear as starlark globals while `flow` / `git` do not.

## Test matrix

Legend — Written: ☑ present · ☐ to write. Grade: ✅ pass · ❌ fail · — not gradable (unwritten). File:
`pkg/op/starlarkbridge/runtime_test.go` (exists; add to it).

| # | Test | Proves | Written | Grade |
|---|---|---|---|---|
| 1 | `TestNewRuntime_PlannedOnlyProvider_NotRegistered` | `flow` (RoleAction+RoleRoot) and `git` (planned, non-root) are **absent** from predeclared | ☑ | ✅ |
| 2 | `TestNewRuntime_ModuleNonRoot_RegisteredUnderName` | `plan` / `ui` / `file` present under their `Name()` | ☑ | ✅ |
| 3 | `TestNewRuntime_ModuleRoot_InstallsEachMethodAndPanicsOnCollision` | a synthetic `RoleModule+RoleRoot` provider installs each method top-level; a name collision triggers the `assert.Failf` panic | ☑ | ✅ |

**Behavioral coverage: 3 / 3 (verified 2026-07-03).** The registration branches are proven with fixtures (planned-only
and planned+root absent; module-non-root under its name with methods NOT top-level; module+root installs each method
as its own global, provider itself absent, and a two-fixture collision panics). The denial-mechanism tests remain
separate coverage.

## Proof run

Verified 2026-07-03: `pkg/op/starlarkbridge` passes under `make test` with the three registration tests present.
The fixtures are announced at package init (ahead of the registry singleton's snapshot) because `buildOne` resolves
module instances through the global registry (`env.ModuleByName` → `ReceiverRegistry`); they stay inert to every
other test because only an explicit `env.Modules` selection reaches them.

## Findings

- ~~Untested branching~~ — closed 2026-07-03 (rows 1–3 landed).
- ~~Residual `fsroot` references~~ — verified absent 2026-07-03: `runtime.go` greps clean for `fsroot`; the
  registration body says "Immediate + root:" and the panic message "(root immediate)". Fixed between the 2026-06-16
  audit and today.

Nothing remains; the 2026-07-03 registration tests also caught and fixed the reserved branch's inverted Attr
assertion (see Status).
