---
title: "Terminal Flow Control Nodes"
status: draft
created: 2026-03-06
updated: 2026-03-06
---

# Plan: Terminal Flow Control Nodes

## Summary

Add three terminal flow control nodes to the execution graph: `Complete`
(successful conclusion), `Degraded` (non-optimal branch, graph continues),
and `Fatal` (immediate halt). All three are flow actions registered via
`op.Announce()` alongside `Choose`, `Gather`, `Elevate`, and `WaitUntil`.
`Degraded` and `Fatal` capture their messages as errors in the
graph.

## Goals

1. **Explicit termination**: Scripts declare success, degradation, or failure
   via flow nodes rather than relying on implicit end-of-graph or raw errors.
2. **Observable outcomes**: Terminal nodes are visible in the graph and
   receipt, not hidden in error handling.
3. **User-facing messages**: `Degraded` and `Fatal` capture their messages as
   errors.
4. **Saga-safe**: `Fatal` triggers orderly unwind via the existing recovery
   stack. No new compensation mechanism.

## Current State

| Component            | Status                                | Notes                                  |
|----------------------|---------------------------------------|----------------------------------------|
| Graph terminal state | Implicit                              | Last node completes or any node errors |
| Flow actions         | Choose, Gather, Elevate, WaitUntil    | No terminal nodes                      |
| `SchemeMem`          | Defined, unused                       | `pkg/op/resource.go:115`               |
| Graph states         | Pending, Executed, Failed             | No Degraded state                      |
| Flow provider        | `internal/execution/flow/provider.go` | Handwritten descriptor, `op.Announce`  |

## Implementation Phases

|     | Phase | Name                                          | Description                                      |
|-----|-------|-----------------------------------------------|--------------------------------------------------|
| [ ] | 1     | [Complete](terminal-flow-control/phase-1.md)  | Success terminal node with optional output value |
| [ ] | 2     | [Degraded](terminal-flow-control/phase-2.md)  | Warning terminal, error, graph continues         |
| [ ] | 3     | [Fatal](terminal-flow-control/phase-3.md)     | Error terminal, executor halt, error             |
| [ ] | 4     | [E2E tests](terminal-flow-control/phase-4.md) | Statement-level Starlark tests for all three     |

## Design

### Complete

The default, healthy conclusion of a path. Accepts an optional output value
that can be captured by the graph consumer.

```go
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

Starlark: `plan.flow.complete(output=value)` or `plan.flow.complete()`.

Not compensable — a successful terminal has nothing to undo.

### Degraded

The graph continues, but marks the branch as non-optimal. Accepts a warning
message that maps to a Go error, captured as an error.

```go
type Degraded struct{}

func (d *Degraded) Do(ctx op.ActionContext) (any, error) {
    msg := ctx.Slot("message").(string)
    return fmt.Errorf("degraded: %s", errors.New(msg)), nil
}
```

Starlark: `plan.flow.degraded(message="disk space low")`.

Returns the warning as the node's output (not as an error). Downstream
nodes can inspect the output. The graph state remains `StateExecuted` but
the receipt records the `mem://degraded/...` resource.

### Fatal

The graph execution stops immediately. Accepts a fail error message that
maps to a Go error, captured as a memory resource.

```go
type Fatal struct{}

func (f *Fatal) Do(ctx op.ActionContext) (any, error) {
    msg := ctx.Slot("message").(string)
    return nil, &FatalError{Message: msg}
}
```

`FatalError` is a sentinel error type. The executor checks `errors.As` for
`*FatalError` and transitions to immediate halt — no further nodes execute.
The existing recovery stack unwinds normally (same path as any node failure
with `ConflictResolution=Stop`).

Starlark: `plan.flow.fatal(message="database unreachable")`.

### FatalError Sentinel

```go
// FatalError signals that the graph must halt immediately.
// The executor unwinds the recovery stack when it encounters this error.
type FatalError struct {
    Message string
}

func (e *FatalError) Error() string { return "fatal: " + e.Message }
```

Defined in `pkg/op/` so the executor can check for it without importing
the flow package.

### Executor Integration

The executor (`runFlat` and `RunPhased` in `internal/execution/executor.go`)
already handles node errors. `Fatal` produces a `FatalError` which the
executor treats as an unconditional stop:

```go
var fe *op.FatalError
if errors.As(err, &fe) {
g.State = op.StateFailed
// unwind recovery stack
break
}
```

This is consistent with the existing `ConflictResolution=Stop` path but
triggered explicitly by a flow node rather than an unexpected error.
`ConflictResolution=Skip` does NOT override `FatalError` — fatal is always
fatal.

### Starlark Bindings

The planned receiver for flow (`plan.flow`) gains three new methods:

- `plan.flow.complete(output=None)` — creates a Complete node
- `plan.flow.degraded(message=str)` — creates a Degraded node
- `plan.flow.fatal(message=str)` — creates a Fatal node

These follow the same pattern as `plan.flow.choose(...)` and
`plan.flow.gather(...)`.

## Related Documents

- [Provider Registration](provider-registration.md) — Registration model
- [Orchestration Primitives](orchestration-primitives.md) — Flow action design
- [Phase Execution](../architecture/devlore-phase-execution.md) — Saga pattern, recovery
- [Provider Registration follow-up](provider-registration/follow-up.md) — Origin of this item

## Open Questions

- [ ] Should `Degraded` set a new `GraphState` (e.g., `StateDegraded`) or
  annotate nodes/phases without changing graph state?

  It should annotate nodes/phases and update the graph state to `StateDegraded`. And this raises
  an interesting scenario. It is possible for me to `Complete` or exit due to a `Fatal` error while
  in a degraded state. Hence, we have two things going on: exit status (`Complete` or `Fatal`) and
  execution status. And we can trace the path between the two points.

- [ ] Should `Fatal` bypass `ConflictResolution` unconditionally, or should
  there be a "force-continue" mode for debugging?

  We should not bypass conflict resolution. It should always be possible to force-continue for debugging.

- [ ] Should `Complete` be the only way to produce a graph output, or should
  the implicit "last node result" also remain?

  Thanks for reminding me about that. The last node result should remain.
