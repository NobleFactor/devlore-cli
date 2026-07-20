---
step: 32
former_step: 29
title: "Compiler-checked action names — op.ActionName consts emitted into provider package roots"
status: in-progress — slices 1 (op.ActionName type + surface retyping) + 2 (const-emitting codegen) landed 2026-07-19; slice 3 (literal migration) pending
proof_run: 2026-07-19 (make test green — 98 packages; gofmt + vet clean) — slices 1 + 2
parent: ../../phase-8.md
---

# Step 32 — Compiler-checked action names (formerly 29)

**Status:** `in-progress`. Design settled 2026-06-19; extracted from the phase-8 table cell (2026-07-03 audit). Slices 1
(the `op.ActionName` type plus the short-name surface retyping) and 2 (the const-emitting codegen) landed 2026-07-19;
slice 3 (literal migration) is pending. See [Slices](#slices).

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

## Slices

### Slice 1 — the `op.ActionName` type + surface retyping (landed 2026-07-19)

`type ActionName string` in `pkg/op/action.go`, beside the `Action` interface, with a doc distinguishing it from the
fully-qualified `FullName()` identity. Every short-name surface retyped from `string` to `op.ActionName`:
`Action.Name()`, `receiverRegistry.BuildAction`, `RuntimeEnvironment.ActionByName`, `plan.Provider.Plan`,
`ExecutableUnitSpec.WithActionNamed` (plus the `Node` / `Subgraph` wrappers), and `ExecutableUnit.ActionName()` /
`setActionName`. Serialization and reporting sites convert at the boundary with `string(…)` (node / subgraph
`marshalData`, receipt `forwardAction` / `compensatingAction` stamping, the writ-migrate views). The codegen test
templates (`getAction` / `getCompensable` parameters, the `names` / `expected` slices) carry `op.ActionName`, and
`powershell` was added to the Makefile's `NEW_OP_INVENTORY` so its gen tests regenerate. Zero behavior change, zero
literal migration.

One non-obvious break: `plan.Provider.Plan`'s new `op.ActionName` first parameter silently dropped its satisfaction of
the duck-typed `flow.actionInvocationPlanner` interface (`Plan(name string, …)`), which the compiler cannot catch — a
runtime type assertion, not an assignment. The lambda-default desugaring in `ChoosePlanner` / `WaitUntilPlanner` then
fell through to "a lambda default requires a planning session host". Fixed by bringing the interface signature in step.
`make test` green (98 packages); gofmt + vet clean.

### Slice 2 — codegen emits the consts (landed 2026-07-19)

A new `action_names.gen.go.template` emits `const WriteText op.ActionName = "file.write_text"` (etc.) into each
provider's **package root** — not the `gen` subpackage — so callers write `plan.Plan(file.WriteText, …)` without
importing `…/gen` or spelling a `<package>.<function>` reference. `emit_provider_receiver` emits it gated on
`access in {planned, both}` (the same gate that gives a provider actions), reusing the sorted `methods` descriptors
(`name` + `snake_name`); `goast.render` runs `format.Source`, so the const columns align and invalid Go fails loudly.
81 consts across 16 providers.

The collision guard `validate_action_name_consts` gathers the package's `goast.funcs` + `goast.structs` and fails
generation if an action-method name matches a package-level func or struct type — a Go redeclaration the const would
otherwise cause (the compiler is the backstop for the rarer cases goast does not surface: non-struct type decls and
package-level vars / consts). No provider collided.

The Makefile lists `$(P)/<provider>/action_names.gen.go` as an output of each of the 16 planned / both grouped targets.
`make generate` idempotent; `make test` green (98 packages); gofmt + vet clean.

### Slice 3 — migrate the literals (pending)

Replace the hand-formulated string literals (~106 in production, ~213 in tests) with the emitted consts — production
first, then tests. This is where the compile-time checking starts paying off: a typo becomes a build error, and rename
/ find-references work through the consts.

## Loose end (resolved 2026-07-19)

The 2026-07-03 note claimed `BuildAction` keys on the **full** form while `plan.Plan` takes the **short** form. That is
stale: today's `BuildAction` (`receiver_registry.go:765`) already takes the short dotted label — it splits on the last
dot, resolves the receiver by short name, and snake-matches the method. No short→identity translation is needed, and
the fully-qualified `Method.ActionName()` identity is untouched.
