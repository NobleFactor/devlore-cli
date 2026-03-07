---
title: "Phase 2: Degraded"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../terminal-flow-control.md
---

# Phase 2: Degraded

## Summary

Add the `Degraded` flow action — a terminal leaf node that marks a branch
as non-optimal while allowing graph execution to continue. Accepts a Go
template format string with `*args` and `**kwargs` that may include
promises from upstream nodes. The message is formatted at execution time
after promises resolve.

`Degraded` is a leaf node — nothing depends on it. It ends its branch
explicitly. Branching and observation happen upstream via existing
primitives like `choose`.

## Deliverables

### 1. RenderError helper

Shared by both `Degraded` and `Fatal`. Lives in `pkg/op/`.

```go
// pkg/op/render.go

// RenderError formats a Go template string with positional args and
// keyword args, returning the result as an error.
func RenderError(format string, args []any, kwargs map[string]any) error {
    // Build template data: kwargs + "Args" key for positional access.
    data := make(map[string]any, len(kwargs)+1)
    for k, v := range kwargs {
        data[k] = v
    }
    data["Args"] = args

    tmpl, err := template.New("msg").Parse(format)
    if err != nil {
        return fmt.Errorf("render: %w", err)
    }
    var buf strings.Builder
    if err := tmpl.Execute(&buf, data); err != nil {
        return fmt.Errorf("render: %w", err)
    }
    return errors.New(buf.String())
}
```

If the format string contains no template directives, it passes through
as a plain string.

### 2. Action definition

```go
// internal/execution/flow/degraded.go

type Degraded struct{}

func (d *Degraded) Namespace() string { return "flow" }
func (d *Degraded) Name() string      { return "degraded" }

func (d *Degraded) Slots() []op.SlotDef {
    return []op.SlotDef{
        {Name: "format", Type: "string"},
        {Name: "args", Type: "list", Optional: true},
        {Name: "kwargs", Type: "dict", Optional: true},
    }
}

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
- Returns the rendered warning as the node's output (`error` return is
  nil — graph continues)
- Sets graph state to `StateDegraded` (done by the executor when this
  node completes without error)
- Not compensable — a warning has no side effect to undo

### 3. Register in flow provider

```go
// internal/execution/flow/provider.go

reg.Register(&Degraded{})
```

### 4. Starlark binding

The planned receiver packs Starlark `*args` into the `args` list slot
and `**kwargs` into the `kwargs` dict slot. Promise values create edges.

```starlark
plan.flow.degraded('disk space low on {{ .volume }}', volume=disk_check)
plan.flow.degraded('simple message with no templates')
```

### 5. Unit tests

```go
// pkg/op/render_test.go

// - Plain string → pass-through
// - Template with kwargs → formatted
// - Template with args → {{ index .Args 0 }}
// - Template with both args and kwargs → merged data
// - Invalid template → error
// - Nil args/kwargs → no panic
```

```go
// internal/execution/flow/degraded_test.go

// - Do with plain format string → returns "degraded: <message>" as output
// - Do with template + kwargs → formats correctly
// - Do returns nil error (graph continues)
// - Verify action is in ActionRegistry after InitAll
```

## Tasks

- [ ] Create `pkg/op/render.go` — `RenderError` helper
- [ ] Create `pkg/op/render_test.go` — unit tests for `RenderError`
- [ ] Create `internal/execution/flow/degraded.go` — action definition
- [ ] Register `&Degraded{}` in `internal/execution/flow/provider.go`
- [ ] Add `plan.flow.degraded(...)` Starlark binding
- [ ] Create `internal/execution/flow/degraded_test.go`
- [ ] `make check` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/render.go` | Create | `RenderError` helper |
| `pkg/op/render_test.go` | Create | RenderError tests |
| `internal/execution/flow/degraded.go` | Create | Action definition |
| `internal/execution/flow/provider.go` | Modify | Register Degraded |
| `internal/execution/flow/degraded_test.go` | Create | Unit tests |

## Exit Criteria

- `RenderError` formats templates with args/kwargs
- `flow.degraded` registered in `ActionRegistry`
- `Do` returns rendered warning as output, nil error — graph continues
- `plan.flow.degraded(format, *args, **kwargs)` creates a graph node
- Plain string messages work (no template directives)
- `make check` passes
