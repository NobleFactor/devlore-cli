---
step: 32
former_step: 29
title: "Compiler-checked action names — op.ActionName consts emitted into provider package roots"
status: COMPLETE 2026-07-20 — all three slices landed (op.ActionName type + surface retyping, const-emitting codegen, literal migration + gen-test linkage)
proof_run: 2026-07-20 (make build clean; make test green — 98 packages; gofmt + vet clean)
parent: ../../phase-8.md
---

# Step 32 — Compiler-checked action names (formerly 29)

**Status:** `COMPLETE` (2026-07-20). Design settled 2026-06-19; extracted from the phase-8 table cell (2026-07-03
audit). All three slices landed — the `op.ActionName` type + short-name surface retyping (1), the const-emitting
codegen (2), and the literal migration + gen-test linkage (3). See [Slices](#slices).

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

### Slice 3 — migrate the literals (landed 2026-07-20)

Every action-name literal used **as an action identifier** now goes through a const. The consumer packages import the
provider (`file.WriteText`, `flow.Choose`, …); `pkg/op/provider/flow`'s own files use their bare consts (`Subgraph`,
`Complete`), and the duplicate local `completeActionName` const was folded into the generated `Complete`. Coverage,
by kind of use:

- **Call-site arguments** to the retyped surfaces (`plan.Provider.Plan`, `BuildAction`, `ActionByName`,
  `WithActionNamed`) — `cmd/writ` (adopt, deploy, migrate, decommission), `cmd/lore`, and the plan / flow test suites.
- **`function.call`** in `flow`'s `ChoosePlanner` / `WaitUntilPlanner` and `plan.Provider.Plan` — migrated to
  `function.Call`, accepting the new `flow → function` and `plan → function` imports. The dependency is real (the
  lambda desugaring emits a `function.call` invocation) and acyclic (`function` imports neither); leaving it a string
  pretended a real dependency did not exist.
- **String-typed uses** — comparisons (`entry.Action == string(file.Link)`), classification-set and `ByAction` map
  keys (`byAction[string(file.WriteText)]`, `readback`'s deploying/removing sets), `switch op.ActionName(n.Action)`
  subjects with bare-const cases, the `completed(...)` / `filterNodesByAction(...)` lookups (retyped to `op.ActionName`
  where the local surface allowed), and readback's action-name return values — all via `string(const)` so the literal
  is eliminated without rippling type changes into serialized fields or the `pkg/op` trace API.

**Gen-test linkage (B):** `action.gen_test.go.template` now emits `provider.WriteText` (not `"file.write_text"`) in the
`names` / `expected` slices and the per-method `getAction` / `getCompensable` / dry-run-substring sites, adding the
`provider` import. The gen test compile-checks that every const exists and — through `getAction`'s registry resolution
(the Go-side `CamelToSnake`, independent of the template's `to_snake`) — that its value resolves to the matching
action. This is what makes the consts load-bearing rather than dead exported identifiers.

**Deliberately left as strings (a distinct vocabulary or prose, not action identifiers):** the `pkg/op`-internal
sites (item 5, import cycle); the `cmd/writ/writ/tree` package's pipeline tokens and `deploy`'s pipeline `case` labels
(the tree's operation vocabulary — `"encryption.decrypt"`, `"file.copy"` as pipeline steps — which overlaps action
names but is a separate concept, and some like `"encryption.decrypt"` are not actions at all); doc comments, error /
transition-message text, and dry-run output substrings (prose); embedded `.star` script fragments; the black-box CLI
output assertions in `cmd/devlore-test` (asserting rendered text, alongside `"version:"`); and invocation labels
(`"file.mkdir#1"`).

`make build` clean (the new cross-provider imports are acyclic); `make test` green (98 packages); gofmt + vet clean.

## Loose end (resolved 2026-07-19)

The 2026-07-03 note claimed `BuildAction` keys on the **full** form while `plan.Plan` takes the **short** form. That is
stale: today's `BuildAction` (`receiver_registry.go:765`) already takes the short dotted label — it splits on the last
dot, resolves the receiver by short name, and snake-matches the method. No short→identity translation is needed, and
the fully-qualified `Method.ActionName()` identity is untouched.
