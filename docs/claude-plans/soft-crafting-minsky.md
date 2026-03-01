# Plan: Action Interface Hierarchy Redesign

**Status**: Parked — blocked by [Reflection-based Starlark Marshaler](../../docs/plans/reflection-marshaler.md)

Execute after the marshaler lands. The marshaler's Phase 2 (return type classification)
and Phase 3 (updated codegen) handle detection and generation. This plan covers only
the Go interface split and its consumers.

## Context

The action hierarchy in `pkg/op/action.go` conflates compensable and non-compensable
actions. Both share the same `Do()` signature returning `(Result, UndoState, error)`,
even though non-compensable actions always return `nil` for UndoState. UndoState is
noise for non-compensable actions and should only appear on CompensableAction.

## Design

### 1. Core Interfaces — `pkg/op/action.go`

```go
// Executable is the base interface stored on Node.Action.
type Executable interface {
    Name() string
}

// Action is a non-compensable action.
type Action interface {
    Executable
    Do(ctx *Context, slots map[string]any) (Result, error)
}

// CompensableAction is a compensable action.
// Does NOT embed Action — it has its own Do returning UndoState.
type CompensableAction interface {
    Executable
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
    Undo(ctx *Context, state UndoState) error
}
```

### 2. Dispatch Helper — `pkg/op/action.go`

```go
func Execute(exec Executable, ctx *Context, slots map[string]any) (Result, UndoState, error) {
    switch a := exec.(type) {
    case CompensableAction:
        return a.Do(ctx, slots)
    case Action:
        result, err := a.Do(ctx, slots)
        return result, nil, err
    default:
        return nil, nil, fmt.Errorf("not executable: %T (%s)", exec, exec.Name())
    }
}
```

### 3. Compensate Method Contract

```go
func (p *Provider) Compensate<Action>(undoState T) error
```

T is the concrete type of the undo state returned by the forward method.
If `Link` returns `(string, map[string]any, error)`, then `CompensateLink`
accepts `map[string]any`. The generator validates this type agreement.

### 4. Infallible Provider Methods

Provider methods with a single return value (e.g., `file.Join`) are wrapped as
`Action` with `Do()` returning `(result, nil)`. No separate interface needed.

## Files to Modify

### Core types — `pkg/op/`
- `action.go` — Executable, Action (2-return Do), CompensableAction (3-return Do), Execute()
- `graph.go` — Node.Action: `Action` → `Executable`; stubAction implements Executable only
- `binding_registry.go` — ActionRegistry stores Executable

### Executor — `internal/execution/`
- `executor.go` — `action.Do(ctx, slots)` → `op.Execute(node.Action, ctx, slots)`
- `recovery.go` — no change (already type-asserts CompensableAction)

### Flow actions — `internal/execution/flow/`
- `choose.go` — inner Do calls → `op.Execute`
- `gather.go` — inner Do calls → `op.Execute`
- `elevate.go` — Do returns `(Result, error)`
- `wait_until.go` — Do returns `(Result, error)`

### Generated actions — `pkg/op/provider/*/gen/actions.gen.go` (regenerate all)
- Non-compensable: Do returns `(Result, error)`, no Undo
- Compensable: Do returns `(Result, UndoState, error)`, has Undo

### Tests
- `internal/execution/flow/flow_test.go` — mock action signatures
- `internal/execution/compensation_test.go` — mock compensable action signatures
- `pkg/op/action_test.go` (new) — Execute dispatch, type assertions

## Verification

1. `make check` passes
2. Generated actions match their provider method signatures
3. Executor dispatches via `op.Execute()`
4. Recovery stack only pushes CompensableAction entries
5. Flow actions (choose, gather) still compensate correctly

## Removed from This Plan (handled by reflection-marshaler)

- Generator detection mechanism (`generate.star`) — marshaler Phase 2
- Template changes (`graph_actions.go.template`) — marshaler Phase 3
- Framework `GoReceiver` template function updates — no longer needed
- Provider method catalog — marshaler auto-classifies at runtime
