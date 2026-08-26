---
title: "ui Takes the Root Namespace and Replaces print and fail"
issue:
status: draft
created: 2026-08-26
updated: 2026-08-26
---

# Plan: ui Takes the Root Namespace and Replaces print and fail

**Epic:** [#445 — The Starlark authoring surface](https://github.com/NobleFactor/devlore-cli/issues/445)
**Design:** [3.5.16-ui-provider.md](../architecture/3.5.16-ui-provider.md) ·
[3.6-method-classification.md](../architecture/3.6-method-classification.md)

## Summary

Starlark ships two output builtins — `print` and `fail` — and devlore currently owns neither. A script's `print`
goes to raw stderr through `starlark-go`'s default, formatted by the interpreter, invisible to the narrator, and
unreachable by the diagnostics stream that #507 will introduce.

`ui` already has methods that mean the same things. Marking the provider `+devlore:root=true` surfaces all six as
top-level globals, and because Starlark resolves **predeclared before universal**, `print` and `fail` are replaced
rather than shadowed — with no interception code, no `starlark.Universe` mutation, and no `Thread.Print` hook.

The prerequisite is that the replacements accept what the builtins accept. `ui.Print(msg string)` takes one
string; `print` is variadic with a keyword separator. Narrowing that would break every `print(a, b)` in the tree.

## Goals

1. **Own the output builtins** — `print` and `fail` reach `status.Narrator` like every other message, and move to
   the diagnostics stream with everything else when #507 lands
2. **Lose nothing** — a variadic `ui.Print` accepts what the builtin accepted, including `sep`
3. **Gain four** — `note`, `warn`, `error`, and `succeed` become top-level globals, which is what a script author
   reaching for `print` actually wanted
4. **Add no mechanism** — one directive and a signature change; the shadowing is a property of the resolver

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `ui` provider, six methods | Landed | `Error`, `Fail`, `Note`, `Print`, `Succeed`, `Warn`; all claim `deterministic` |
| `ui` roles | `RoleModule\|RoleAction` | reachable as `ui.note(...)` and `plan.ui.note(...)` |
| Signatures | `(msg string)` | one string; `Fail` also returns `error` |
| `print` / `fail` builtins | **unowned** | resolve to `starlark.Universe`; `print` writes to stderr via `starlark-go` |
| `RoleModule\|RoleRoot` path | **never used** | `runtime.go:125` calls it reserved; its assertion sat inverted until step 6's tests |

## What makes this work without new machinery

**`resolve.go:437` checks `isPredeclared` before `:441` checks `isUniversal`.** The root path installs each method
as `predeclared[CamelToSnake(name)]`, so `Print` becomes `print` and `Fail` becomes `fail` — exact matches. The
runtime hands `rt.predeclared` to `ExecFileOptions` (`runtime.go:243`), and the builtins are never consulted.

`receiver_type.go:66` already documents this case, using `note()` as its example.

## The builtin's contract, which the replacement must honor

`library.go:799`:

- **variadic** over positional arguments
- **`sep` keyword**, defaulting to `" "`
- strings written as-is; `Bytes` written raw; everything else through `writeValue`, i.e. `repr`
- returns `None`

`fail` is simpler: it raises, halting evaluation. `ui.Fail` returns an `error`, which the bridge turns into a
Starlark error — same observable behavior, so `fail` needs no compatibility work beyond the variadic change.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Make all six `ui` methods variadic — `(args ...any)`, and `Fail(args ...any) error` | ☐ |
| 2 | Join arguments the way the builtin does: strings as-is, everything else through the value's Starlark representation | ☐ |
| 3 | Decide `sep` — a `sep` keyword parameter matching the builtin, or a documented narrowing. **Ruling needed** | ☐ |
| 4 | Update `3.5.16-ui-provider.md`'s method table and its status companion | ☐ |
| 5 | `+devlore:root=true` on the `ui` provider struct | ☐ |
| 6 | Regenerate, and verify `ui` announces `RoleModule\|RoleAction\|RoleRoot` | ☐ |
| 7 | Confirm the six names install as top-level globals and that `print` / `fail` resolve to them, not to the universe | ☐ |
| 8 | Confirm `ui.note(...)` still works — root placement removes the provider global, so every existing `ui.*` call site changes | ☐ |
| 9 | Sweep `.star` call sites: `ui.note(...)` becomes `note(...)`, across `star/extensions`, `cmd/star/extensions`, and `cmd/devlore-test/devloretest/data` | ☐ |
| 10 | `make check`, `make test-race`, and `make test-scenario` green | ☐ |

## Ruling needed before step 1

**Does `error` belong in the global namespace?** `note`, `warn`, `succeed`, `print`, and `fail` are safe. `error`
is an ordinary word for a script to want as a variable, and a predeclared name cannot be assigned over at module
scope — `error = compute()` becomes a resolve error rather than a shadow. Options: accept it, rename the global to
something else while keeping `ui.Error`, or leave `Error` off the root surface.

## What this does NOT do

**It does not give total control over the other builtins.** `dir`, `getattr`, `hasattr`, `type`, and `repr` remain
the universe's. Owning those is a separate question — and a real one for a hermetic runtime, since three of them
are reflection over the surface.

**It does not consolidate thread creation.** Eight sites construct `starlark.Thread` independently and none sets
`Thread.Print`. That does not matter once `print` is predeclared, because the builtin is never reached — but any
policy that *does* need the thread hook would have to be set eight times. Tracked separately.

## Verification

- `print("a", "b")` reaches the narrator, joined, with no stderr leak
- `fail("x")` halts evaluation and reports through the narrator
- Every `.star` file in the tree parses and runs
- `ui` still claims `deterministic` on all six, so a hermetic planning runtime keeps its output
- Darwin and `GOOS=linux`; the claim check across all four platform tags
