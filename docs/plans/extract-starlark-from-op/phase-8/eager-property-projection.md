---
title: "Eager property projection — the MethodModifiers bit-flag"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-graph-immutability.md"
status: planned
created: 2026-05-31
updated: 2026-05-31
---

## Problem statement

The phase-8 "Row 4" reds — `cmd/star/star`'s 9 `TestLintCopyright_*` cases and `TestSourceFile_StarlarkIntegration`
— are not stale scripts. They fail because the `.star` consumers read zero-arg getters as **properties**
(`config.get`, `ast.package_name`) while the current reflection projection surfaces every method as a **callable
builtin**:

- `lint-copyright.star:303`: `cfg = config.get` then `cfg.lint` → `builtin_function_or_method has no .lint field`.
- `sourcefile_integration_test.go:116`: `ast.package_name != "example"` → compared against `<built-in function
  SourceFile.package_name>`.

The intended contract is documented in [3.3 Static Starlark Codegen](../../../architecture/3.3-static-starlark-codegen.md)
(§"StarAttrs — Attribute Dispatch", lines 260, 290): **"Fields become direct access. Zero-arg methods become direct
calls."** Its `StarAttr` example eager-calls `uri`/`id`/`scheme`/`exists` while leaving `Save()` a builtin. So the
scripts are written to the documented design; the **reflection `goReceiver` never implemented eager getters**. These
tests are *ahead of the implementation*, not a regression.

**Decision (2026-05-31): fix the bridge, not the scripts.** Eager-getter projection is documented design, not legacy,
so the prime directive is neutral here; retreating the scripts to the callable contract would regress the API away
from its own docs and incur re-touch churn when codegen lands. The bridge is brought up to the 3.3 contract via an
explicit, author-declared per-method signal. This is its own scoped framework step; phase-8's PR gate (full `make
test` green) gates on it, so Row 4 cannot be parked red.

> Resolves the Row-4 item carried as **step 25 / F3** in [phase-8.md](../phase-8.md) and the F3 pre-work item in
> [lore-command-rewrites.md](./lore-command-rewrites.md).

## Why a signal is required (and why it can't be inferred)

"Zero-arg" is syntactic; "accessor vs. action" is semantic (command-query separation). The two don't line up, and the
distinguishing fact — does the method have side effects? — is invisible to the type system, the AST, and reflection.
So it cannot be inferred from behavior, and name heuristics are brittle (`config.Get()` is a verb-named getter;
`Resolve`/`Load`/`Cleanup` are genuinely ambiguous). It must be **declared** by the author.

The existing per-method `MethodKind` (method.go:53, auto-classified from the return signature) does the heavy lifting
and constrains *eligibility*:

| Kind | Return | CQS | Property-eligible |
|---|---|---|---|
| `MethodAction` | `()` | command | no — no value to return |
| `MethodFallibleAction` | `(error)` | command | no — no value to return |
| `MethodFunction` | `(T)` | query | **yes** |
| `MethodFallibleFunction` | `(T, error)` | query | **yes** (fallible getter; error propagates as an attr error) |
| `MethodCompensableFunction` | `(T, U, error)` | command-ish | no |

An action returns no value, so it is *structurally* incapable of being an eager property — "actions stay callable" is
a consequence of the kind, not a rule. This also auto-resolves the `(T, error)` ambiguity (`config.Get` is a fallible
*function*, `Save` is a fallible *action*). But not every zero-arg `MethodFunction` is an accessor (`Snapshot() State`,
`Build() Graph` are operations), so the author's signal is still needed. `MethodKind` gives *eligibility*; the signal
gives *intent*. They are orthogonal axes — a property is a *modifier on* a Function, not a sixth kind.

## The design

**Signal:** `+devlore:property` — a presence-flag directive in the method's doc comment (same family as
`+devlore:access=`, `+devlore:defaults`). Default unset → callable; opt-in eager. The hazardous behavior (code firing
on attribute access) is the thing you ask for, never the default.

**Representation:** a per-method bit-flag, parallel to the provider-level `ProviderRole`:

- Type `MethodModifiers` (`uint` bit-flag).
- First bit `ModifierProperty MethodModifiers = 1 << iota` (stem from the type word, like `RoleModule`; avoids
  colliding with the `MethodKind` family `MethodAction`/`MethodFunction`).
- Field `modifiers MethodModifiers` on `Method` (beside `kind`); accessor `Method.Modifiers()`.
- Field `Modifiers MethodModifiers` on `MethodMetadata` (today only `ParameterNames` + `Planner`).
- Flat for now (no dispatch/placement zones); zoning can come if a second orthogonal modifier axis appears.

**Codegen validation:** `ModifierProperty` is legal only on a zero-arg `MethodFunction`/`MethodFallibleFunction` —
tagging an action or a parameterized method is a compile-time error. The kind does the gatekeeping.

## Plumbing — confirmed by trace (2026-05-31)

The metadata channel and its routing **already exist**; only the type's registration and a metadata field are
missing.

- The result-wrap path `toStarlarkReflect` resolves a returned value's receiver type via
  `ReceiverRegistry.TypeByReflectionOrDerive(ptr.Type())` (`go_receiver.go:417`). That looks up `byType` first and
  derives via reflection only for unregistered types. So a **registered** type's codegen `MethodMetadata` is already
  delivered to the `goReceiver` — no marshaling rework needed.
- **`SourceFile` is already registered** — via `AnnounceType` (`goast/gen/source_file.gen.go`), the value-type
  category for "Go structs that need method dispatch but are neither providers nor resources." `FuncDecl`,
  `GenDeclNode`, and `starcode.Sources` ride the same path, so it is found in `byType` (not the derive branch). **But
  `AnnounceType` carries only `map[string][]string`** (parameter names), *not* `MethodMetadata` — so value types have
  no channel for the `Modifiers` bit.
- **Providers are registered** with full `MethodMetadata` (via `AnnounceProvider(..., map[string]MethodMetadata)`), so
  `config.get` already has a metadata record to extend.

Consequence: there is no registration to add and no resource masquerade — the gap is that **`AnnounceType` lacks a
`MethodMetadata` channel**. Upgrading it to mirror `AnnounceProvider` gives value-type getters (`ast.package_name`)
the same modifier path providers already have.

## Work items

1. **`MethodModifiers` type + `ModifierProperty`** (new bit-flag, `pkg/op`). Flat `uint`, first bit, doc-commented per
   the `ProviderRole` precedent.
2. **`MethodMetadata.Modifiers`** field; plumb it through `AnnounceProvider`/`NewReceiverType` → `NewMethod` →
   `Method.modifiers`, with a `Method.Modifiers()` accessor.
3. **`goReceiver.Attr` honors it.** When the resolved `*op.Method` has `ModifierProperty`, eager-call and marshal the
   result instead of returning the builtin. `Attr` already holds the `*op.Method` from the method index — localized.
4. **Codegen** (goast / staranalysis): read `+devlore:property`, emit `Modifiers`, and **validate** the kind/arity
   pairing (zero-arg `MethodFunction`/`MethodFallibleFunction` only).
5. **Upgrade `AnnounceType` to carry `MethodMetadata`** (from `map[string][]string`), mirroring `AnnounceProvider`,
   so value types get the `Modifiers` channel — `SourceFile`/`FuncDecl`/`GenDeclNode` are *already* `AnnounceType`'d,
   so no new registration. The `AnnounceType`-emitting codegen (item 4) emits `MethodMetadata` instead of bare param
   maps. Tag the getter methods (`PackageName`, `Types`, `CheckCompliance`, …).
6. **Tag `config.Provider.Get`** (`+devlore:property`) — provider path, already registered.

Scripts and tests are **not** edited — they already read property semantics; once the bridge honors the bit they pass
as authored.

## Registration — settled (2026-05-31): `AnnounceType`, already in use

There is no A-vs-B decision. The registry already has a **value-type category** — `AnnounceType(goType, …)`, for "Go
structs that need method dispatch but are neither providers nor resources (e.g., the goast AST types)" — and the
codegen already emits it for `SourceFile`, `FuncDecl`, and `GenDeclNode` (`goast/gen/*.gen.go`). Modeling `SourceFile`
as a `ResourceReceiverType` was never necessary: it would force a meaningless `ResourceConstructor` and
`URI`/`Digest`/`Etag` semantics onto a transient AST that is *produced by a method call*, not coerced from a literal.

The only gap is metadata reach: `AnnounceType` takes `map[string][]string` (parameter names) where `AnnounceProvider`
takes `map[string]MethodMetadata`. So the settled work is **upgrade `AnnounceType` to `map[string]MethodMetadata`**
(work item 5) — value types then carry `Modifiers` exactly as providers do. Nothing else about registration changes.

## Affected sites (scope)

- **Failing tests (must go green):** `cmd/star/star` `TestLintCopyright_*` (×9, via `config.get`),
  `TestSourceFile_StarlarkIntegration` (`ast.package_name`).
- **Latent breaks (no test today):** `LintAll/lint-all.star:32` (`config.get`),
  `LintGoStyle/lint-go-style.star:47` (`for v in ast.check_compliance`). Repaired for free by the bridge fix.
- **3.3-listed getters** (`uri`/`id`/`scheme`/`exists`/…): align as their types are tagged/registered.

## Exit criteria

- `MethodModifiers`/`ModifierProperty` landed; `MethodMetadata.Modifiers` plumbed to `Method`; `goReceiver` eager-calls
  property methods; codegen emits + validates the modifier.
- `SourceFile` (and peers) registered; getter methods tagged; `config.Get` tagged.
- `cmd/star/star` Row-4 tests green **without editing the scripts/tests**; full `make test` green (modulo the
  separately-tracked, sanctioned `TestWalkTreePlanned` step-24 deferral and the pre-seal lore/writ consumer builds).
- 3.3 reconciled: note that the reflection path now honors the documented eager-getter contract via `ModifierProperty`.

## See also

- [3.3 Static Starlark Codegen](../../../architecture/3.3-static-starlark-codegen.md) — the documented eager-getter
  contract this implements on the reflection path.
- [3.2 Projected Provider API](../../../architecture/3.2-projected-provider-api.md) — the reflection projection.
- [lore-command-rewrites.md](./lore-command-rewrites.md) — the working set; this is box 2.
