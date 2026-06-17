---
step: 6
title: "StarlarkRuntime predeclared-globals registration branches on access × placement"
former_step: 7
former_title: "StarlarkRuntime access×root registration branches"
status: incomplete — pending tests
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 6 — StarlarkRuntime predeclared-globals registration (formerly step 7)

**Status:** `incomplete — pending tests` · **Behavioral tests: 0 / 3 written** · the branching is present and the package
has tests, but they cover the *denial* mechanism, not the registration branches.

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
| 1 | `TestNewRuntime_PlannedOnlyProvider_NotRegistered` | `flow` (RoleAction+RoleRoot) and `git` (planned, non-root) are **absent** from predeclared | ☐ | — |
| 2 | `TestNewRuntime_ModuleNonRoot_RegisteredUnderName` | `plan` / `ui` / `file` present under their `Name()` | ☐ | — |
| 3 | `TestNewRuntime_ModuleRoot_InstallsEachMethodAndPanicsOnCollision` | a synthetic `RoleModule+RoleRoot` provider installs each method top-level; a name collision triggers the `assert.Failf` panic | ☐ | — |

**Behavioral coverage: 0 / 3.** Existing `runtime_test.go` tests (`TestFilteredReceiver_*`, `TestDenyAttributes_*`,
`TestRuntime_applyDenials`) cover denials/filtering, not registration. The plan-doc's "plan → global, flow → not
registered, …" was a smoke check.

## Proof run

```
$ go test ./pkg/op/starlarkbridge/...   # ok — but the 4 runtime tests are denial-mechanism, not registration
$ grep -n "func Test" runtime_test.go   # FilteredReceiver_Attr/_AttrNames, DenyAttributes, applyDenials
```

The step reaches `complete` when rows 1–3 are ☑ and ✅.

## Findings

- **Untested branching.** Rows 1–3 above — the access×placement registration is unverified by any Go test.
- **Residual `fsroot` the rename missed** (placement sense, not the filesystem package): `runtime.go:94`
  ("Immediate + fsroot:") and `runtime.go:112` — the **`assert.Failf` panic message** "(fsroot immediate)". The earlier
  `fsroot`→`root` pass fixed the matrix comment block but missed these two in the registration body.

## Remaining to reach `complete`

Write rows 1–3, and fix the two residual `fsroot` references at `runtime.go:94`/`:112`.
