---
title: "Phase 1: Action.Do 4-value return"
parent: ../reconciliation.md
status: draft
---

# Phase 1: Action.Do 4-value return

## Summary

Extend the `Action.Do` signature from 3-value to 4-value return, adding
`ReconciliationState` as the third positional return. Update the executor,
flow actions, reflected actions, all provider implementations, all test
mocks, and regenerate all `actions_gen.go` files.

## Rationale

The architecture document (§1) identifies three distinct consumers of the
data produced by `Do`:

| Return | Consumer | Purpose |
| --- | --- | --- |
| `Result` | Graph edge system | Data flow to downstream nodes |
| `UndoState` | Recovery Stack | Mechanical reversal if the graph fails |
| `ReconciliationState` | Reconciliation Store | Source of truth for future drift checks |

Today, `Do` returns only the first two. Reconciliation data has no channel.
Providers that want to report verifiable state (content hash, installed
version, service status) have nowhere to put it. Adding the 4th return
creates that channel.

## Changes

### Type alias

```go
// pkg/op/action.go
type ReconciliationState = any
```

### Interface change

```go
// Before
Do(ctx *Context, slots map[string]any) (Result, UndoState, error)

// After
Do(ctx *Context, slots map[string]any) (Result, UndoState, ReconciliationState, error)
```

### Executor

`executeNode` captures the 4th return. For now, it is unused — Phase 2
wires it into `PushAction`, and Phase 3 wraps it in the `ExecutionEvent`.

```go
result, undoState, _, err := action.Do(ctx, slots)
```

### Flow actions

`Choose.Do` and `Gather.Do` (specifically `executeIteration`) call
`node.Action.Do` in their inner loops. These gain the 4th return,
currently ignored.

### Reflected actions

`action_reflect.go` dispatches `Do` via reflection. The reflected call
must handle 4-value returns. The reconciliation state is passed through
as `any`.

### Provider methods

Every `Do` implementation across all providers gains a `nil` 4th return:

```go
// Before
return result, undoState, nil

// After
return result, undoState, nil, nil
```

This is a mechanical change. Providers that produce reconciliation data
will populate the 3rd return in Phase 2.

### Test mocks

All test mock actions (`echoAction`, `failAction`, `countAction`,
`testUndoAction`, etc.) gain the 4th return.

### Code generation

Star templates that generate `Do` method calls and return handling must
be updated. All `actions_gen.go` files are regenerated.

## Tasks

- [ ] Add `ReconciliationState = any` type alias in `pkg/op/action.go`
- [ ] Change `Action.Do` signature to return `(Result, UndoState, ReconciliationState, error)`
- [ ] Update `executeNode` in `internal/execution/executor.go` to capture 4th return
- [ ] Update `Choose.Do` inner loop in `internal/execution/flow/choose.go`
- [ ] Update `executeIteration` in `internal/execution/flow/gather.go`
- [ ] Update `action_reflect.go` to handle 4-value return in reflected actions
- [ ] Update all provider `Do` implementations to return 4 values (nil reconciliation for now)
- [ ] Update all test mock actions to return 4 values
- [ ] Regenerate all `actions_gen.go` files via star

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/action.go` | Modify | Add `ReconciliationState`, update `Action.Do` |
| `internal/execution/executor.go` | Modify | Capture 4th return |
| `internal/execution/flow/choose.go` | Modify | 4-value inner Do call |
| `internal/execution/flow/gather.go` | Modify | 4-value inner Do call |
| `pkg/op/action_reflect.go` | Modify | Handle 4-value reflected return |
| `pkg/op/provider/*/provider.go` | Modify | Add `nil` 4th return to all Do methods |
| `pkg/op/provider/*/actions_gen.go` | Regenerate | Wire 4th return |
| `internal/execution/compensation_test.go` | Modify | 4-value mock actions |
| `internal/execution/flow/flow_test.go` | Modify | 4-value mock actions |
