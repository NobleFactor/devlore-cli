---
title: "Phase 1: Reflection Bridge"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../variadic-starlark-args.md
---

# Phase 1: Reflection Bridge

## Summary

Update `buildMethodBridge` in `receiver_reflect.go` to detect variadic
parameters marked with a `*` prefix in `MethodParams` and collect remaining
positional Starlark args into the variadic slot. After this phase, any
method whose `Params` entry uses `*name` accepts positional args in
Starlark.

## Deliverables

### 1. Variadic marker detection

In `buildMethodBridge`, scan the param name list for a `*` prefix:

```go
// pkg/op/receiver_reflect.go — inside buildMethodBridge

var variadicParam string
var namedParams []string
for _, p := range paramNames {
    if strings.HasPrefix(p, "*") {
        variadicParam = strings.TrimPrefix(p, "*")
    } else {
        namedParams = append(namedParams, p)
    }
}
```

At most one variadic param per method (enforced — Go allows only one).
The variadic param must be the last in the list.

### 2. Positional arg collection

After `starlark.UnpackArgs` processes named params, collect remaining
positional args for the variadic slot:

```go
// pkg/op/receiver_reflect.go — inside the generated bridge closure

if variadicParam != "" {
    if len(remainingArgs) > 0 {
        // Positional args → variadic list
        variadicSlot = starlark.NewList(remainingArgs)
    } else if keywordValue != nil {
        // Keyword fallback: parts=["a", "b"] still works
        variadicSlot = keywordValue
    } else {
        // No args → empty list (matches Go nil slice for variadic)
        variadicSlot = starlark.NewList(nil)
    }
}
```

### 3. CallSlice integration

The existing `CallSlice` path (`pkg/op/receiver_reflect.go:164-165`)
already handles variadic methods. The change is in how the args are
collected, not how they're dispatched to Go.

### 4. Unit tests

```go
// pkg/op/receiver_reflect_test.go

// - Positional args: join("a", "b", "c") → parts=["a", "b", "c"]
// - Keyword list:    join(parts=["a", "b"]) → same result
// - Empty:           join() → parts=[]
// - Mixed:           method with named params followed by variadic
// - Error:           variadic with both positional and keyword → reject ambiguity
```

## Tasks

- [ ] Detect `*` prefix in param names within `buildMethodBridge` in `pkg/op/receiver_reflect.go`
- [ ] Collect remaining positional args into variadic slot
- [ ] Preserve keyword list fallback for backward compatibility
- [ ] Handle empty variadic (no args → nil/empty slice)
- [ ] Add unit tests in `pkg/op/receiver_reflect_test.go`
- [ ] `make check` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/receiver_reflect.go` | Modify | Variadic detection and positional collection |
| `pkg/op/receiver_reflect_test.go` | Modify | Unit tests for variadic calling conventions |

## Exit Criteria

- `buildMethodBridge` handles `*`-prefixed params
- Positional args collected for variadic methods
- Keyword list form still works
- All existing tests pass (no params use `*` yet, so no behavior changes)
- `make check` passes
