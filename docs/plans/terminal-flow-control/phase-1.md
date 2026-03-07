---
title: "Phase 1: Complete"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 1: Complete

## Summary

Add the `Complete` flow action — the default, healthy conclusion of a
graph path. Accepts an optional output value that can be captured by
the graph consumer or downstream flow actions (`choose`, `gather`).
Register in the flow provider. Add Starlark binding. Unit tests.

## Deliverables

### 1. Action definition

```go
// internal/execution/flow/complete.go

type Complete struct{}

func (c *Complete) Namespace() string { return "flow" }
func (c *Complete) Name() string      { return "complete" }

func (c *Complete) Slots() []op.SlotDef {
    return []op.SlotDef{
        {Name: "output", Type: "any", Optional: true},
    }
}

func (c *Complete) Do(ctx op.ActionContext) (any, error) {
    return ctx.Slot("output"), nil
}
```

Not compensable — a successful terminal has nothing to undo.

### 2. Register in flow provider

```go
// internal/execution/flow/provider.go

func (p *flowProvider) Register(reg *op.ActionRegistry, _ op.Context) {
    reg.Register(&Choose{})
    reg.Register(&Gather{})
    reg.Register(&Elevate{})
    reg.Register(&WaitUntil{})
    reg.Register(&Complete{})
}
```

### 3. Starlark binding

The planned receiver for flow already exists. Add `complete` method to
the flow planned receiver:

```starlark
# via internal/starlark/plan_root.go (or wherever plan.flow methods are defined)

plan.flow.complete()           # nil output
plan.flow.complete(output=42)  # captures value 42
```

The method creates a `flow.complete` node in the graph with the `output`
slot wired to the provided value (or nil).

### 4. Unit tests

```go
// internal/execution/flow/complete_test.go

// - Do with output value → returns value, nil error
// - Do with nil output → returns nil, nil error
// - Verify action is in ActionRegistry after InitAll
// - Verify Namespace() and Name() return "flow" and "complete"
```

## Tasks

- [ ] Create `internal/execution/flow/complete.go`
- [ ] Register `&Complete{}` in `internal/execution/flow/provider.go`
- [ ] Add `plan.flow.complete(...)` Starlark binding in `internal/starlark/plan_root.go`
- [ ] Create `internal/execution/flow/complete_test.go`
- [ ] `make check` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/execution/flow/complete.go` | Create | Action definition |
| `internal/execution/flow/provider.go` | Modify | Register Complete |
| `internal/starlark/plan_root.go` | Modify | Starlark binding |
| `internal/execution/flow/complete_test.go` | Create | Unit tests |

## Exit Criteria

- `flow.complete` registered in `ActionRegistry`
- `plan.flow.complete(output=value)` creates a graph node
- `Do` returns the output value with nil error
- `make check` passes
