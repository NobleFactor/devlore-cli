---
title: "Variadic Starlark Args"
status: draft
created: 2026-03-06
updated: 2026-03-06
---

# Plan: Variadic Starlark Args

## Summary

Make variadic Go parameters present as multiple positional arguments in
Starlark. Today `file.join(parts=["a", "b"])` requires a keyword list.
After this plan, `file.join("a", "b")` works — the variadic parameter
collects remaining positional args. The keyword form continues to work.

## Goals

1. **Natural calling convention**: Variadic Go methods accept positional
   args in Starlark without keyword wrapping.
2. **Backward compatible**: `file.join(parts=["a", "b"])` still works.
3. **Code generator awareness**: `star` marks variadic params so the
   reflection bridge handles them correctly.

## Current State

| Component                   | Status                                 | Notes                      |
|-----------------------------|----------------------------------------|----------------------------|
| `file.Provider.Join`        | `parts ...string`                      | Only variadic method today |
| `Params` entry              | `"Join": {"parts"}`                    | No variadic marker         |
| `buildMethodBridge`         | Single-slot unpacking                  | `receiver_reflect.go:117`  |
| Starlark calling convention | `file.join(parts=[...])`               | Keyword-only               |
| `starlark.UnpackArgs`       | Separates positional from keyword args | Available in bridge        |

## Implementation Phases

|     | Phase | Name                                                   | Description                                                               |
|-----|-------|--------------------------------------------------------|---------------------------------------------------------------------------|
| [ ] | 1     | [Reflection bridge](variadic-starlark-args/phase-1.md) | Update `buildMethodBridge` to collect positional args for variadic params |
| [ ] | 2     | [Codegen + tests](variadic-starlark-args/phase-2.md)   | Mark variadic params in templates, regenerate, e2e tests                  |

## Design

### Variadic Marker in MethodParams

Mark variadic params with a `*` prefix in the params list:

```go
"Join": {"*parts"}, // variadic — collects positional args
```

The `*` prefix is an internal convention in `MethodParams`. It tells
`buildMethodBridge` to handle this parameter differently.

### buildMethodBridge Changes

In `receiver_reflect.go`, `buildMethodBridge` detects the `*` prefix:

1. Strip the `*` prefix and record the param as variadic
2. Non-variadic params are unpacked via `starlark.UnpackArgs` as today
3. Remaining positional args (`args` after UnpackArgs consumes named params)
   are collected into a Starlark list and assigned to the variadic slot
4. If no positional args remain and the caller passed the keyword form
   (`parts=[...]`), use the keyword value (existing behavior)

```go
// Pseudocode for variadic handling in the bridge:
if isVariadic {
// Positional args not consumed by named params → variadic slice
if len(remainingArgs) > 0 {
variadicSlot = starlark.NewList(remainingArgs)
}
// Keyword fallback: parts=["a", "b"] still works
}
```

### starlark.UnpackArgs Integration

`UnpackArgs` already returns unused positional args when the param list
doesn't consume them all. The bridge currently treats leftover positional
args as an error. After this change, leftover positional args are consumed
by the variadic parameter.

### Planned Action Reflection

`action_reflect.go` has similar slot-based unpacking for planned actions.
Planned actions use named slots, not positional args, so variadic handling
does not apply there. The variadic parameter appears as a single list-valued
slot in planned mode.

## Related Documents

- [Provider Registration](provider-registration.md) — Parent epic
- [Provider Registration follow-up](provider-registration/follow-up.md) — Origin of this item

## Open Questions

- [ ] Should the `*` prefix convention live in `MethodParams` or should `star`
  emit a separate `VariadicParams` set alongside `Params`?

  I say use the `*` prefix convention. It's easier to comprehend in code.

- [ ] If more variadic methods are added later, should `star` auto-detect
  them from Go signatures or require explicit annotation?

  The code generator should not require explicit annotations that can be inferred. Go syntax makes it
  clear what's a variadic method. We should have NO variadic annotation.

- [ ] What if both `parts` and `*args` are present?
  ```python
  path = file.join(parts=['a', 'b', 'c'], 'd', 'e', 'f')
  ```
  Is there a winner?

  I say that raising the equivalent of a Python `TypeError`. In Python, it is common to provide this
  feature. I like having the option. I do not think its wise to merge or accept one over another. The
  call should fail.
