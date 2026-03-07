---
title: "Phase 2: Degraded"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 2: Degraded

## Summary

Add the `Degraded` flow action — a terminal that marks a branch as
non-optimal while allowing graph execution to continue. Accepts a warning
message that maps to a Go error, captured as a `mem://` resource in the
graph's resource catalog.

## Deliverables

### 1. Memory resource helper

Add a constructor for `mem://` resources. `SchemeMem` is already defined
at `pkg/op/resource.go:115` but unused.

```go
// pkg/op/resource.go

// NewMemResource creates a mem:// resource for in-memory artifacts.
// kind identifies the category (e.g., "degraded", "fatal").
// message is the content captured in the resource.
func NewMemResource(kind, message string) Resource {
    return NewResourceBase(SchemeMem, kind, message)
}
```

This may require adjusting `NewResourceBase` or using the existing
`Resource` constructor pattern. Match whatever constructor pattern
the other schemes use (file, git, pkg, etc.).

### 2. Action definition

```go
// internal/execution/flow/degraded.go

type Degraded struct{}

func (d *Degraded) Namespace() string { return "flow" }
func (d *Degraded) Name() string      { return "degraded" }

func (d *Degraded) Slots() []op.SlotDef {
    return []op.SlotDef{
        {Name: "message", Type: "string"},
    }
}

func (d *Degraded) Do(ctx op.ActionContext) (any, error) {
    msg := ctx.Slot("message").(string)
    ctx.Catalog().Append(op.NewMemResource("degraded", msg))
    return fmt.Errorf("degraded: %s", msg), nil
}
```

Returns the warning as the node's output (not as an error — `error` return
is nil). The graph continues executing. Downstream nodes can inspect the
warning output if wired via edges.

Not compensable — a warning has no side effect to undo.

### 3. Register in flow provider

```go
// internal/execution/flow/provider.go

reg.Register(&Degraded{})
```

### 4. ActionContext catalog access

`Do` needs access to the graph's `ResourceCatalog`. Verify that
`ActionContext` exposes `Catalog()`. If not, add it — the executor
already has the catalog (it lives on the graph).

### 5. Starlark binding

```starlark
# via internal/starlark/plan_root.go

plan.flow.degraded(message="disk space low")
```

Creates a `flow.degraded` node with the `message` slot.

### 6. Unit tests

```go
// internal/execution/flow/degraded_test.go

// - Do captures mem://degraded/... resource in catalog
// - Do returns warning error as output, nil error
// - Verify message slot is required (not optional)
```

## Tasks

- [ ] Add `NewMemResource` helper to `pkg/op/resource.go`
- [ ] Verify `ActionContext` exposes `Catalog()` — add if missing
- [ ] Create `internal/execution/flow/degraded.go`
- [ ] Register `&Degraded{}` in `internal/execution/flow/provider.go`
- [ ] Add `plan.flow.degraded(...)` Starlark binding in `internal/starlark/plan_root.go`
- [ ] Create `internal/execution/flow/degraded_test.go`
- [ ] `make check` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/resource.go` | Modify | `NewMemResource` helper |
| `internal/execution/flow/degraded.go` | Create | Action definition |
| `internal/execution/flow/provider.go` | Modify | Register Degraded |
| `internal/starlark/plan_root.go` | Modify | Starlark binding |
| `internal/execution/flow/degraded_test.go` | Create | Unit tests |

## Exit Criteria

- `flow.degraded` registered in `ActionRegistry`
- `Do` captures `mem://degraded/...` resource in catalog
- `Do` returns warning as output, nil error — graph continues
- `plan.flow.degraded(message=...)` creates a graph node
- `make check` passes
