---
title: "A Dispatch Error Names the Symbol the Author Typed"
issue: 710
status: complete
created: 2026-08-28
updated: 2026-08-28
---

# Plan: A Dispatch Error Names the Symbol the Author Typed

**Issue:** [#710](https://github.com/NobleFactor/devlore-cli/issues/710) ·
**Epic:** [#445 — The Starlark authoring surface](https://github.com/NobleFactor/devlore-cli/issues/445) ·
**Unblocked by:** [#715](https://github.com/NobleFactor/devlore-cli/issues/715)

## Summary

```
>>> print("a", "b")
ui.print: got 2 arguments, want at most 1
```

`ui.print` does not exist. `ui` is promoted, so its methods are top-level globals and the provider name is not
a symbol any script can use — an author who takes the error at its word and writes `ui.print(...)` gets
`undefined: ui`. **The error sends them toward a second, unrelated error**, which is worse than a vague message.

Pre-existing, not introduced by #708. `flow` has done this since promotion existed:
`cmd/devlore-test/devloretest/data/test_flow_fatal.star` calls `plan.failed(...)` and expects an error saying
`flow.failed`. Nobody could write `flow.failed` in a script.

## The three naming sites

Each site already knows which surface it serves, and two of them know their placement by construction — they
*are* the promoted path and the qualified path.

| Site | Surface | Placement | Emits today | Should emit |
| --- | --- | --- | --- | --- |
| `starlarkbridge/go_receiver.go:229` | script | either | `ui.print` | promoted → `print`<br>qualified → `file.copy` |
| `plan/provider.go:843` | workflow | promoted | `flow.choose` | `plan.choose` |
| `plan/adapter.go:117` | workflow | qualified | `file.copy` | `plan.file.copy` |

Only the first consults `Placement()`. The other two are unconditional: whichever site builds the builtin
determines the shape of its name.

**This is what #715 unblocked.** Before it, placement was a bit whose meaning was tangled with dispatch, and
"which name would the author have typed" had no clean answer. `flags.Placement() == op.PlacementPromoted` is
now a one-line question.

## The governing rule

> **Where a call site exists, an error names what the author typed; where none exists, it names the action.**
> The two cannot be one name because the action name is a node's identity field — it is in the serialized
> document and the checksum — so tying it to placement would rewrite every saved workflow when a provider's
> placement changed.

```python
print("a", "b")          # script evaluation -- the call site is right here
→ print: got 2 arguments, want at most 1        (today: ui.print, a symbol no script can use)

plan.failed("db down")   # runs later, possibly from a workflow loaded off disk
→ flow.failed executed: db down                 (unchanged -- there is no call site to name)
```

### Why the two names cannot simply be unified

`ActionName` is documented as *"the sole identity field"* on `nodeData`, serialized to json and yaml, and
therefore part of the canonical form the checksum covers. If the serialized name followed placement, changing a
provider from qualified to promoted would rewrite the action name in every saved workflow, change every
canonical form, and invalidate every checksum. **Placement is a call-site convenience; identity cannot move
when it changes.**

### Why an author is not owed one name

The two errors arise in different worlds, and only one of them has a call site to name:

| Error kind | Call site | Name available |
| --- | --- | --- |
| arity, type mismatch | known — the script is being evaluated | **what the author typed** |
| execution failure | **none** — the workflow may have been loaded from disk | only the action name |

An execution error genuinely cannot use a call-site name, because by then there may never have been one. A
loaded workflow has nodes, not call sites. `flow.failed executed: database unreachable` is right in that
context: you are looking at a running workflow, and the provider qualifier says who implements the operation.

So the boundary is explainable rather than arbitrary, which is what would otherwise confuse an author. Today
`print("a", "b")` reports `ui.print` **while the script is being evaluated**, with the call site right there
and ignored.

### What this narrows

Determining which fixtures depend on which name is not archaeology under this rule: script-evaluation errors
move, execution errors do not. `test_flow_fatal.star` matches an *execution* error and does not change.

## Steps

| # | Step | ✓ |
| --- | --- | --- |
| 1 | Classify each fixture's expectation: script-evaluation error, or execution error | ✅ |
| 2 | Failing tests first, one per site | ✅ |
| 3 | `go_receiver.go` — consult `Placement()`; bare name when promoted | ✅ |
| 4 | `plan/provider.go` — promoted builtins take `plan.<method>` | ✅ |
| 5 | `plan/adapter.go` — qualified builtins take `plan.<provider>.<method>` | ✅ |
| 6 | Update fixtures whose expectations name a builtin | ✅ |
| 7 | `make check`, `test-race`, `test-scenario` | ✅ |

### Step 1 result (2026-08-28)

Four fixture expectations name a provider-qualified symbol. **Exactly one changes.**

| Fixture | Expectation | Runs a graph | Kind | Verdict |
| --- | --- | --- | --- | --- |
| `test_imm_ui_print_replaces_builtin` | `ui\.print: got 2 args…` | **no** | script eval | **moves** → `print:` |
| `test_compensation` | `file.copy` | yes | execution | unchanged |
| `test_judgment_1_delete_then_copy` | `file.copy` | yes | execution | unchanged |
| `test_judgment_resolve_dangling` | `file.resolve` | yes | execution | unchanged |
| `test_flow_fatal` | `flow.failed executed:` | yes | execution | unchanged |

**"Runs a graph" is the mechanical discriminator**: every execution-error fixture calls `t.run(graph)`, and the
one script-evaluation fixture does not.

This is the rule paying for itself. Step 1 was written expecting archaeology — the risk being that a fixture's
text alone would not say which name it was seeing, so a change might either break serialized identity or
"fix" an expectation that was already correct. Under the governing rule it is a one-column check, and the three
`file.*` expectations plus the `flow.*` one are all execution errors that must stay exactly as they are.

It also confirms row 2 of the test plan is a real counterweight rather than a formality: `test_compensation`
expects `file.copy`, so a fix that made every name bare would break it — and should.

## Outcome

Complete 2026-08-28. `check`, `test-race`, `test-scenario` all pass.

| Site | Before | After |
| --- | --- | --- |
| script surface | `ui.print` | **`print`** promoted; `file.copy` qualified, unchanged |
| workflow, promoted | `flow.choose` | **`plan.choose`** |
| workflow, qualified | `file.copy` | **`plan.file.copy`** |

### `actionName` was a misnomer, and it made the change look risky

Both workflow sites named their variable `actionName` and passed it to `dispatchBuiltinBody`, which reads as
though changing it would move the serialized action name. It does not: that function uses the string **only in
error text**, and a node's action name comes from `unit.Action().Name()` at marshal. The variable made a
display string look like an identity. Renamed to `callSiteName` at both sites, with a comment saying which it
is.

That is also why the four execution-error fixtures passed untouched — they never reach these sites. Step 1's
classification predicted exactly that, and the change confirmed it rather than merely being consistent with it.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `print("a","b")` reports `print:`, not `ui.print:` | e2e | the script site ignores placement |
| 2 | `file.copy(...)` still reports `file.copy:` | unit | the fix over-reaches to qualified providers |
| 3 | A promoted workflow builtin reports `plan.choose:` | unit | the promoted plan site is missed |
| 4 | A qualified workflow builtin reports `plan.file.copy:` | unit | the adapter site is missed |
| 5 | A graph node's action name is still `flow.failed` | unit | the change leaked into serialized identity |
| 6 | Every gate | `check`, `test-race`, `test-scenario` | any of the above regresses |

**Row 2 is the counterweight.** Most providers are qualified, and `file.copy` is exactly what an author types,
so it must not change. A fix that made every name bare would pass rows 1, 3, and 4.

**Row 5 is the boundary.** The starlark-facing name and the serialized action name are different things that
happen to share a spelling today. Only one moves.

**Row 1 has a landing spot already.** `test_imm_ui_print_replaces_builtin.star` currently pins
`ui\.print: got 2 arguments, want at most 1` deliberately, with a comment saying the fix to #710 must come back
through it.

### Not covered

- **Resource receivers.** `newGoReceiver` also serves resources, whose names are type names rather than
  provider names. Placement does not apply, and nothing here changes for them — but the code path is shared,
  so the tests should confirm it by absence rather than assume it.
