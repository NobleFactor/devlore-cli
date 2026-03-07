---
title: "Phase 3: Fatal"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 3: Fatal

## Summary

Add the `Fatal` flow action — a terminal that halts graph execution
immediately. Accepts a fail error message that maps to a Go error,
captured as a `mem://` resource. The executor detects `FatalError` and
stops — no further nodes execute. The recovery stack unwinds normally.

## Deliverables

### 1. FatalError sentinel

```go
// pkg/op/fatal.go

// FatalError signals that the graph must halt immediately.
// The executor unwinds the recovery stack when it encounters this error.
// Unlike a regular node error, FatalError is never overridden by
// ConflictResolution — fatal is always fatal.
type FatalError struct {
    Message string
}

func (e *FatalError) Error() string { return "fatal: " + e.Message }
```

Defined in `pkg/op/` so the executor can check for it without importing
the flow package.

### 2. Action definition

```go
// internal/execution/flow/fatal.go

type Fatal struct{}

func (f *Fatal) Namespace() string { return "flow" }
func (f *Fatal) Name() string      { return "fatal" }

func (f *Fatal) Slots() []op.SlotDef {
    return []op.SlotDef{
        {Name: "message", Type: "string"},
    }
}

func (f *Fatal) Do(ctx op.ActionContext) (any, error) {
    msg := ctx.Slot("message").(string)
    ctx.Catalog().Append(op.NewMemResource("fatal", msg))
    return nil, &op.FatalError{Message: msg}
}
```

Returns `FatalError` as the error. The executor handles it.

Not compensable — the fatal node itself has no side effect to undo.
Prior nodes unwind via the existing recovery stack.

### 3. Executor integration

Update `runFlat` and `RunPhased` to check for `FatalError` after each
node's `Do`:

```go
// internal/execution/executor.go — inside runFlat and RunPhased

var fe *op.FatalError
if errors.As(err, &fe) {
    node.Status = op.StatusFailed
    node.Error = fe.Error()
    g.State = op.StateFailed
    // Unwind recovery stack (same as existing failure path)
    break
}
```

Key behavior:
- `FatalError` always halts, regardless of `ConflictResolution` setting
- Remaining nodes are marked `StatusSkipped`
- In `RunPhased`, the phased recovery/unwind path executes normally
- In `runFlat`, the recovery stack unwinds

### 4. Register in flow provider

```go
// internal/execution/flow/provider.go

reg.Register(&Fatal{})
```

### 5. Starlark binding

```starlark
# via internal/starlark/plan_root.go

plan.flow.fatal(message="database unreachable")
```

Creates a `flow.fatal` node with the `message` slot.

### 6. Unit tests

```go
// internal/execution/flow/fatal_test.go

// - Do returns FatalError
// - Do captures mem://fatal/... resource in catalog
// - errors.As matches *FatalError
```

```go
// internal/execution/executor_test.go

// - Graph with fatal node → StateFailed, remaining nodes skipped
// - Fatal overrides ConflictResolution=Skip — still halts
// - Recovery stack unwinds after fatal
```

## Tasks

- [ ] Create `pkg/op/fatal.go` — `FatalError` sentinel type
- [ ] Create `internal/execution/flow/fatal.go` — action definition
- [ ] Register `&Fatal{}` in `internal/execution/flow/provider.go`
- [ ] Update `runFlat` in `internal/execution/executor.go` — check `FatalError`
- [ ] Update `RunPhased` in `internal/execution/executor.go` — check `FatalError`
- [ ] Add `plan.flow.fatal(...)` Starlark binding in `internal/starlark/plan_root.go`
- [ ] Create `internal/execution/flow/fatal_test.go`
- [ ] Add executor tests in `internal/execution/executor_test.go`
- [ ] `make check` passes
- [ ] `make test-race` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/fatal.go` | Create | `FatalError` sentinel |
| `internal/execution/flow/fatal.go` | Create | Action definition |
| `internal/execution/flow/provider.go` | Modify | Register Fatal |
| `internal/execution/executor.go` | Modify | `FatalError` detection + halt |
| `internal/starlark/plan_root.go` | Modify | Starlark binding |
| `internal/execution/flow/fatal_test.go` | Create | Unit tests |
| `internal/execution/executor_test.go` | Modify | Fatal halt tests |

## Exit Criteria

- `flow.fatal` registered in `ActionRegistry`
- `Do` returns `*FatalError` and captures `mem://fatal/...` resource
- Executor halts on `FatalError` regardless of `ConflictResolution`
- Recovery stack unwinds normally after fatal halt
- Remaining nodes marked `StatusSkipped`
- Graph state is `StateFailed`
- `make check` and `make test-race` pass
