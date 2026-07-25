---
title: "Phase 3: Fatal"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 3: Fatal

## Summary

Add the `Fatal` flow action — a terminal leaf node that halts graph
execution immediately. Same signature as `Degraded` — a Go template
format string with `*args` and `**kwargs`, formatted at execution time
after promises resolve. The executor detects `FatalError` and stops — no
further nodes execute. The recovery stack unwinds normally.

`Fatal` is a leaf node — nothing depends on it. It ends its branch
explicitly. Branching and observation happen upstream via existing
primitives like `choose`.

## Deliverables

### 1. FatalError sentinel

```go
// pkg/op/fatal.go

// FatalError signals that the graph must halt immediately.
// The executor unwinds the recovery stack when it encounters this error.
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
        {Name: "format", Type: "string"},
        {Name: "args", Type: "list", Optional: true},
        {Name: "kwargs", Type: "dict", Optional: true},
    }
}

func (f *Fatal) Do(ctx op.ActionContext) (any, error) {
    format := ctx.Slot("format").(string)
    args, _ := ctx.Slot("args").([]any)
    kwargs, _ := ctx.Slot("kwargs").(map[string]any)
    return nil, &op.FatalError{Message: op.RenderError(format, args, kwargs).Error()}
}
```

Uses `RenderError` from `pkg/op/` (introduced in Phase 2).

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
- `FatalError` halts graph execution — the default failure path
- `ConflictPolicy` is respected: a force-continue/debug mode can
  override the halt for diagnostic purposes
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
plan.flow.fatal('{{ .service }} startup failed.', service=promise)
plan.flow.fatal('database unreachable')
```

### 6. Unit tests

```go
// internal/execution/flow/fatal_test.go

// - Do with plain format → returns FatalError with message
// - Do with template + kwargs → formats correctly in FatalError
// - errors.As matches *FatalError
// - Verify action is in ActionRegistry after InitAll
```

```go
// internal/execution/executor_test.go

// - Graph with fatal node → StateFailed, remaining nodes skipped
// - Fatal with ConflictPolicy=Skip → continues (debug override)
// - Fatal with ConflictPolicy=Stop → halts (default)
// - Recovery stack unwinds after fatal
```

## Tasks

- [ ] Create `pkg/op/fatal.go` — `FatalError` sentinel type
- [ ] Create `internal/execution/flow/fatal.go` — action definition
- [ ] Register `&Fatal{}` in `internal/execution/flow/provider.go`
- [ ] Update `runFlat` in `internal/execution/executor.go` — check `FatalError`
- [ ] Update `RunPhased` in `internal/execution/executor.go` — check `FatalError`
- [ ] Add `plan.flow.fatal(...)` Starlark binding
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
| `internal/execution/flow/fatal_test.go` | Create | Unit tests |
| `internal/execution/executor_test.go` | Modify | Fatal halt tests |

## Exit Criteria

- `flow.fatal` registered in `ActionRegistry`
- `Do` formats template and returns `*FatalError`
- Executor halts on `FatalError` (respects `ConflictPolicy` for debug override)
- Recovery stack unwinds normally after fatal halt
- Remaining nodes marked `StatusSkipped`
- Graph state is `StateFailed`
- `make check` and `make test-race` pass
