---
step: 32
former_step: 29
title: "Compiler-checked action names — op.ActionName consts emitted into provider package roots"
status: not-started — design settled 2026-06-19, implementation pending
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 32 — Compiler-checked action names (formerly 29)

**Status:** `not-started`. Design settled 2026-06-19; extracted here from the phase-8 table cell (2026-07-03 audit).

## Problem

Stringly-typed action names — `"file.write_text"` — are hand-formulated across tests and production code. A typo is
a runtime lookup failure, not a compile error.

## Design (settled 2026-06-19)

1. `type ActionName string` in `pkg/op/action.go`, beside the `Action` interface.
2. Codegen emits `const WriteText op.ActionName = "file.write_text"` (etc.) into the **provider package root** — not
   the `gen` subpackage — so callers write `plan.Plan(file.WriteText, …)` without importing
   `pkg/op/provider/<package>/gen` or spelling a `<package>.<function>` reference.
3. `op.ActionName` covers **only the short starlark action name** — the `Action.Name()` / `plan.Provider.Plan(name)`
   / const surfaces that carry `"file.write_text"`.
4. The **fully-qualified type identity** — `Action.FullName()` / `Method.ActionName()` =
   `<pkg-path>.<receiverName>.<methodName>` (`method.go:328`), the receipt-stamp form — **stays `string`**: it is a
   different concept (reflect's `(PkgPath, Name)` identity, for which the Go spec has no term), not the short name.

## Loose end for implementation

`BuildAction` (`receiver_registry.go:538`) keys lookups on the **full** form while `plan.Plan` takes the **short**
form — trace the short→identity resolution before retyping signatures.
