---
title: "Terminal Flow Control Nodes"
status: draft
created: 2026-03-06
updated: 2026-03-06
---

# Plan: Terminal Flow Control Nodes

## Summary

Add three terminal flow control nodes to the execution graph: `Complete`
(successful conclusion), `Degraded` (non-optimal branch, other branches
continue), and `Fatal` (immediate halt). All three are **leaf nodes** —
nothing depends on them. They end their branch explicitly.

All three are flow actions registered via `op.Announce()` alongside
`Choose`, `Gather`, `Elevate`, and `WaitUntil`.

`Degraded` and `Fatal` accept a Go template format string with `*args`
and `**kwargs` that may include promises from upstream nodes. The message
is formatted at execution time after promises resolve.

## Goals

1. **Explicit termination**: Scripts declare success, degradation, or failure
   via terminal leaf nodes rather than relying on implicit end-of-graph or
   raw errors.
2. **Observable outcomes**: Terminal nodes are visible in the graph and
   receipt, not hidden in error handling.
3. **Templated messages**: `Degraded` and `Fatal` accept Go template format
   strings with args/kwargs that may reference upstream promises. Messages
   are formatted at execution time after promises resolve.
4. **Saga-safe**: `Fatal` triggers orderly unwind via the existing recovery
   stack. No new compensation mechanism.
5. **Concurrent branches**: `Degraded` ends one branch while independent
   branches continue executing. The graph is not halted.

## Current State

| Component            | Status                                | Notes                                  |
|----------------------|---------------------------------------|----------------------------------------|
| Graph terminal state | Implicit                              | Last node completes or any node errors |
| Flow actions         | Choose, Gather, Elevate, WaitUntil    | No terminal nodes                      |
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

### Terminal Leaf Model

All three terminals — `Complete`, `Degraded`, and `Fatal` — are **leaf
nodes**. Nothing depends on them. They end their branch explicitly.

Branching and observation happen upstream via existing primitives like
`choose`. The terminal is the conclusion of a branch, not a link in a
chain.

```starlark
health = plan.service.check(service="db")

# Branch A: conditional termination
plan.flow.choose(
    condition=health,
    if_true=plan.flow.complete(output=health),
    if_false=plan.flow.degraded('{{ .svc }} unhealthy', svc=health),
)

# Branch B: independent work — no edge to the terminal
plan.file.write_text(destination=log, content="proceeding...", mode=0o644)
```

- **Interrogation**: `choose` inspects the upstream result, routes to
  `degraded` or `complete`
- **Messaging**: `degraded` renders the template, writes to stderr,
  sets `StateDegraded`
- **Concurrency**: Branch B has no edge to `degraded` — runs independently
- **Terminal**: `degraded` is a leaf. Its branch ends. Other branches
  continue.

### Complete

The default, healthy conclusion of a branch. Leaf node. Accepts an
optional output value that can be captured by the graph consumer.

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

Leaf node. Ends its branch, marks the graph as non-optimal, and writes
the rendered message to stderr. Other branches continue executing.

Signature: `def degraded(format, *args, **kwargs)`

```starlark
plan.flow.degraded('disk space low on {{ .volume }}', volume=disk_check)
plan.flow.degraded('simple warning with no templates')
```

```go
func (d *Degraded) Do(ctx op.ActionContext) (any, error) {
    format := ctx.Slot("format").(string)
    args, _ := ctx.Slot("args").([]any)
    kwargs, _ := ctx.Slot("kwargs").(map[string]any)
    rendered := op.RenderError(format, args, kwargs)
    fmt.Fprintln(os.Stderr, "degraded:", rendered)
    return rendered, nil
}
```

- Writes the rendered message to stderr (observable side effect)
- Returns the rendered error as the node's output (nil error — graph
  continues)
- Sets graph state to `StateDegraded`
- Not compensable — a warning has no side effect to undo

### Fatal

Leaf node. Halts graph execution immediately. Same signature as
`Degraded` — a Go template format string with args/kwargs.

Signature: `def fatal(format, *args, **kwargs)`

```starlark
plan.flow.fatal('{{ .service }} startup failed.', service=promise)
plan.flow.fatal('database unreachable')
```

```go
func (f *Fatal) Do(ctx op.ActionContext) (any, error) {
    format := ctx.Slot("format").(string)
    args, _ := ctx.Slot("args").([]any)
    kwargs, _ := ctx.Slot("kwargs").(map[string]any)
    return nil, &FatalError{Message: op.RenderError(format, args, kwargs).Error()}
}
```

- `FatalError` is a sentinel error type
- The executor checks `errors.As` for `*FatalError` and halts
- The existing recovery stack unwinds normally
- Not compensable — prior nodes unwind via the recovery stack

### RenderError

```go
// pkg/op/render.go

// RenderError formats a Go template string with positional args and
// keyword args, returning the result as an error.
func RenderError(format string, args []any, kwargs map[string]any) error
```

Builds a template data map: kwargs merged with an `Args` key for
positional access. Executes `text/template` and wraps the result in
an error.

```go
// Template data available to the format string:
// {{ .key }}          — kwargs value
// {{ index .Args 0 }} — positional arg by index
```

If the format string contains no template directives, it passes through
as-is (plain string messages still work).

### FatalError Sentinel

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
`ConflictResolution` is respected — a force-continue/debug mode can
override the halt for diagnostic purposes.

### Starlark Bindings

The planned receiver for flow (`plan.flow`) gains three new methods:

- `plan.flow.complete(output=None)` — leaf, creates a Complete node
- `plan.flow.degraded(format, *args, **kwargs)` — leaf, creates a Degraded node
- `plan.flow.fatal(format, *args, **kwargs)` — leaf, creates a Fatal node

These follow the same pattern as `plan.flow.choose(...)` and
`plan.flow.gather(...)`.

For `degraded` and `fatal`, the planned receiver packs the Starlark
`*args` into a list slot and `**kwargs` into a dict slot on the graph
node. Promise values in args/kwargs create edges — resolved at execution
time before the template is formatted.

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
