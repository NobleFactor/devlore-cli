---
title: "ui Takes the Root Namespace and Replaces print and fail"
issue: 698
status: approved
created: 2026-08-26
updated: 2026-08-26
---

# Plan: ui Takes the Root Namespace and Replaces print and fail

**Feature:** [#698 — ui takes the root namespace](https://github.com/NobleFactor/devlore-cli/issues/698)
**Epic:** [#445 — The Starlark authoring surface](https://github.com/NobleFactor/devlore-cli/issues/445)
**Design:** [3.5.16-ui-provider.md](../architecture/3.5.16-ui-provider.md)

## Summary

Starlark ships two output builtins — `print` and `fail` — and devlore owns neither. A script's `print` resolves
to `starlark.Universe`, is formatted by the interpreter, and is written straight to stderr by `starlark-go`'s
default. It never reaches `status.Narrator`, so it is invisible to `--quiet`, to any output shaping we choose,
and to the diagnostics stream that #507 will introduce.

`ui` already has methods that mean these things, and four more besides. Marking the provider
`+devlore:root=true` surfaces all six as top-level globals, and because Starlark resolves **predeclared before
universal**, `print` and `fail` are replaced rather than shadowed — with no interception code, no
`starlark.Universe` mutation, and no `Thread.Print` hook.

**No signature changes.** The methods keep `(msg string)`. See the ruling below.

## Goals

1. **Own the output builtins** — `print` and `fail` reach `status.Narrator` like every other message, and move
   to the diagnostics stream with everything else when #507 lands
2. **Gain four** — `note`, `warn`, `error`, and `succeed` become top-level globals, which is what a script
   author reaching for `print` actually wanted
3. **Add no mechanism** — one directive; the shadowing is a property of the resolver

## What makes this work without new machinery

**`resolve.go` checks `isPredeclared` before `isUniversal`.** The root path installs each method as
`predeclared[CamelToSnake(name)]`, so `Print` becomes `print` and `Fail` becomes `fail` — exact matches. The
runtime hands `rt.predeclared` to `ExecFileOptions`, and the builtins are never consulted.

`receiver_type.go:66` already documents this case, using `note()` as its example. This is the first real use of
the `RoleModule|RoleRoot` branch that `runtime.go:125` calls reserved.

## Ruling: no signature changes

The builtin is `print(*args, sep=" ")` — variadic, keyword separator, any value type. `ui.Print(msg string)`
is narrower on all three axes. Three candidate reconciliations were considered and all three rejected:

**`fmt.Printf` semantics — rejected on double interpretation.** Starlark's `%` is an operator that runs
*before* the call, so `print("%s" % name)` arrives as a finished string. Printf semantics would re-scan that
string for verbs, turning `50% off` into `50%!o(MISSING)ff` whenever data contains a `%`. Go avoids this by
making `Print` and `Printf` different functions; one function cannot be both. The everyday form of the same
bug is `print("100% done")`.

**Variadic space-join — rejected because the bridge has already destroyed the types.** `fillVariadicSlot` runs
`toGoInto`, so a Go-side `args ...any` receives Go natives: `True` arrives as `bool`, `None` as `nil`, a list
as `[]any`. Rendering those Go-side yields `true`, `<nil>`, `[a b]` where Starlark says `True`, `None`,
`["a"]`. Faithful output would mean reimplementing `writeValue` on the far side of a conversion that already
threw the information away.

**Keeping `(msg string)` is therefore the correct signature, not a compromise.** The script renders with
`str()` and `%` — Starlark's own rendering, correct and deterministic by construction — and hands over a
finished string. For a configuration language, requiring explicit `str()` is a feature.

The tree already works this way, universally. Every `print` and `fail` call site was surveyed:

| | Call sites | Multi-argument | Non-string argument |
| --- | --- | --- | --- |
| `print(` | 64 | 0 | 0 |
| `fail(` | 65 | 0 | 0 |

Every site passes one string, built with `%`, `+`, or an explicit `str(...)`.

### Residual failure, named

`print("a", "b")` and `print(42)` are legal against the builtin and become errors against us. Nothing in the
tree does either, but a script author with Python reflexes will write one. The errors are legible rather than
cryptic — `toGoScalar` uses `starlark.AsString` with no coercion, so `print(42)` reports `expected string, got
int`, and a multi-argument call reports an arity error. Documenting the one-string contract is step 5.

## Ruling: `error` keeps the global name

A predeclared name cannot be assigned over at module scope, so `error = compute()` becomes a resolve error
rather than a shadow. Accepted: script authors adopt `err` for the variable, the convention Go authors already
follow for exactly the same reason — the language took `error` first.

| Global | Method | | Global | Method |
| --- | --- | --- | --- | --- |
| `error` | `ui.Error` | | `print` | `ui.Print` |
| `fail` | `ui.Fail` | | `succeed` | `ui.Succeed` |
| `note` | `ui.Note` | | `warn` | `ui.Warn` |

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Fix `placeModule`'s `return`-for-`continue` defect (below) before the branch carries traffic | ☐ |
| 2 | `+devlore:root=true` on the `ui` provider struct | ☐ |
| 3 | Regenerate; verify `ui` announces `RoleModule\|RoleAction\|RoleRoot` | ☐ |
| 4 | Sweep **231** `ui.*` call sites to bare names (`star/extensions`, `cmd/star/extensions`, `devloretest/data`) | ☐ |
| 5 | Document the one-string contract and the `err`-not-`error` convention in `3.5.16-ui-provider.md` | ☐ |
| 6 | `make check`, `make test-race`, and `make test-scenario` green | ☐ |

## Defect found in the branch this activates

`placeModule`'s root path (`pkg/op/starlarkbridge/runtime.go:403`):

```go
for m := range module.Methods() {
    snake := op.CamelToSnake(m.Name())
    if refused[snake] {
        return          // <- abandons every method not yet placed
    }
    ...
    predeclared[snake] = attr
}
```

`return` should be `continue`. As written, one refused method silently drops every method the loop has not
reached — and because it ranges over `module.Methods()`, *which* methods survive depends on iteration order
rather than on what the runtime admits.

It has never fired: no provider has been `RoleModule|RoleRoot`, and `ui` claims `deterministic` on all six, so
a hermetic runtime refuses none. Putting `ui` at root arms it. The sibling assertion on line 419 was inverted
for the same reason and sat latent until #677's step 6 — the second defect in the same unexercised branch,
which is a fair signal about the rest of it.

**Fix it as step 1**, with a test that refuses one method of a root provider and asserts the others still
surface.

## What this does NOT do

**It does not give total control over the other builtins.** `dir`, `getattr`, `hasattr`, `type`, and `repr`
remain the universe's. Three of them are reflection over the surface, which is a real question for a hermetic
runtime and a separate one.

**It does not consolidate thread creation.** Eight sites construct `starlark.Thread` independently and none
sets `Thread.Print`. Irrelevant once `print` is predeclared — the builtin is never reached — but any policy
that does need the thread hook would have to set it eight times.

## Verification

- `print("a")` reaches the narrator with no stderr leak; `fail("x")` halts evaluation and narrates
- `ui.note(...)` no longer resolves — root placement replaces the provider global rather than adding to it
- A test asserts a bare `print(...)` reaches the narrator, so a regression restoring the builtin fails loudly
  instead of silently reverting to stderr
- `ui` still claims `deterministic` on all six, so a hermetic planning runtime keeps its output
- Darwin and `GOOS=linux`; the claim check across all four platform tags
