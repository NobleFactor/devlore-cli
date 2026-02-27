# Plan: Action Interface Hierarchy Redesign

## Context

The action hierarchy in `pkg/op/action.go` conflates compensable and non-compensable
actions. Both share the same `Do()` signature returning `(Result, UndoState, error)`,
even though non-compensable actions always return `nil` for UndoState. UndoState is
noise for non-compensable actions and should only appear on CompensableAction.

Additionally, the code generator (`generate.star`) determines compensability via
**name-based pairing** — it checks whether `Compensate<MethodName>` exists. The user
wants this changed to **signature-based detection** — the return value count of the
provider method determines the action kind:

| Return signature | Action kind | Compensation |
|-----------------|-------------|--------------|
| `(T, map[string]any, error)` | CompensableAction | Expects `Compensate<Name>` method |
| `(T, error)` | Action | No compensation |
| `(T)` | Action (wraps with nil error) | No compensation |

### Verification: Current Generator Behavior

The generator in `generate.star:200` uses **name-based pairing**, not signature analysis:
```python
compensable = ("Compensate" + m.name) in all_method_names
```
The `compensable` boolean is then passed in the method descriptor to template functions
(`graphReturn`, `graphUndo`) defined in the Devlore framework's `GoReceiver`. These
functions generate different `Do()` bodies but the **same** `Do()` signature
`(op.Result, op.UndoState, error)` for all actions. Non-compensable actions just
return `nil` for UndoState.

### Current Provider Method Catalog

**Compensable** (3 returns: `T, map[string]any, error`) — 18 methods:
- file: Link, Copy, Backup, Unlink, Remove, Write, Move
- git: Clone
- pkg: Install, Remove, Upgrade
- service: Start, Stop, Restart, Enable, Disable
- archive: Extract

**Fallible non-compensable** (2 returns: `T, error`) — 23 methods:
- file: Read, Mkdir, Exists, IsDir, Glob, IsFile, RemoveAll, WalkTree
- shell: Exec, PowerShell
- template: Render
- encryption: Decrypt
- net: Download
- git: Checkout, Pull
- pkg: Update, Installed, NotInstalled, VersionGTE
- service: Exists, Running, Enabled

**Infallible** (1 return: `T`) — 7 methods (not currently graph actions):
- file: Join, Name, Parent
- ui: Error, Note, Success, Warn

**Flow actions** (internal, hand-written, not generated):
- Choose, Gather → CompensableAction (have Undo)
- Elevate, WaitUntil → Action (no real compensation)

## Design

### 1. Core Interfaces — `pkg/op/action.go`

```go
// Executable is the base interface stored on Node.Action.
type Executable interface {
    Name() string
}

// Action is a non-compensable, fallible action.
type Action interface {
    Executable
    Do(ctx *Context, slots map[string]any) (Result, error)
}

// CompensableAction is a compensable, fallible action.
// Does NOT embed Action — it has its own Do returning UndoState.
type CompensableAction interface {
    Executable
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
    Undo(ctx *Context, state UndoState) error
}
```

### 2. Dispatch Helper — `pkg/op/action.go`

Normalizes the call across action types so the executor doesn't need to type-switch:

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

Compensate methods have a specific signature:

```go
func (p *Provider) Compensate<Action>(undoState T) error
```

T is the concrete type of the undo state returned by the forward `<Action>` method.
If `Link` returns `(string, map[string]any, error)`, then `CompensateLink` accepts
`map[string]any`. The generator should validate this type agreement.

Current state: file provider uses typed parameters (`map[string]any`), other
providers use `any`. The file provider pattern is correct — the type should be
specific, not `any`.

### 4. Generator — `generate.star` (this repo)

Change compensability detection from name-pairing to signature-based:

```python
# Replace:
#   compensable = ("Compensate" + m.name) in all_method_names
# With:
returns_count = len(m.returns)
if returns_count == 3:
    action_kind = "compensable"
    if ("Compensate" + m.name) not in all_method_names:
        fail("compensable method %s returns 3 values but has no Compensate%s" % (m.name, m.name))
elif returns_count == 2:
    action_kind = "action"
elif returns_count == 1:
    action_kind = "action"  # infallible — wrap as Action with nil error
else:
    fail("method %s has unsupported return count %d" % (m.name, returns_count))
```

The method descriptor changes from `"compensable": bool` to `"action_kind": string`.

### 5. Template — `graph_actions.go.template`

The template currently hardcodes `Do(ctx *op.Context, slots map[string]any) (op.Result, op.UndoState, error)`. It needs conditional signatures:

**Option A — Framework updates `graphReturn`/`graphUndo`** (clean, requires framework change):
Template stays the same; framework reads `action_kind` and generates the correct
signature, return statement, and optional `Undo()`.

**Option B — Inline conditionals in template** (self-contained, no framework change):
Replace `{{graphReturn . $.ImplType}}` and `{{graphUndo .}}` with Go template
conditionals that branch on `.ActionKind`. More verbose but doesn't block on
framework release.

Recommend: **Option A** since the framework already owns `graphReturn`/`graphUndo`.
The framework change is small — `graphReturn` already inspects `compensable` to
generate 2-value vs 3-value provider calls. It just needs to also conditionally
emit the `Do()` signature and dry-run return.

### 6. Executor — `internal/execution/executor.go`

One call site changes: `action.Do(ctx, slots)` → `op.Execute(node.Action, ctx, slots)`.
The rest (CompensableAction check, recovery push) stays the same.

### 7. Flow Actions — `internal/execution/flow/`

| File | Change |
|------|--------|
| `choose.go` | Implement `CompensableAction` (Do returns `(Result, UndoState, error)`) — no change needed |
| `gather.go` | Implement `CompensableAction` — no change needed |
| `elevate.go` | Implement `Action` — `Do` returns `(Result, error)`, drop `UndoState` return |
| `wait_until.go` | Implement `Action` — `Do` returns `(Result, error)`, drop `UndoState` return |

Flow actions also call `node.Action.Do(ctx, nodeSlots)` internally (choose.go,
gather.go). These become `op.Execute(node.Action, ctx, nodeSlots)`.

### 8. Node.Action Type — `pkg/op/graph.go`

`Node.Action` field type changes from `Action` to `Executable`. The `stubAction`
type implements `Executable` only (Name() + String()), no `Do`.

### 9. ActionRegistry — `pkg/op/` (binding_registry.go or wherever it lives)

Stores `Executable` instead of `Action`. Register/Get/MustGet accept/return `Executable`.

### 10. Infallible Provider Methods — No Separate Interface

Provider methods with a single return value (e.g., `file.Join`, `file.Name`,
`file.Parent`) are wrapped as `Action` with `Do()` returning `(result, nil)`.
No separate `InfallibleAction` interface is needed. The generator detects the
1-return signature and generates `return o.Impl.Foo(...), nil`.

The `Execute()` dispatch stays at two cases: `CompensableAction` and `Action`.

## Files to Modify

### Core types — `pkg/op/`
- `action.go` — Executable, Action (2-return Do), CompensableAction (3-return Do), Execute()
- `graph.go` — Node.Action: Action → Executable; stubAction
- `binding_registry.go` — ActionRegistry stores Executable

### Executor — `internal/execution/`
- `executor.go` — action.Do → op.Execute (1 call site)
- `recovery.go` — no change (already type-asserts CompensableAction)

### Flow actions — `internal/execution/flow/`
- `choose.go` — inner Do calls → op.Execute
- `gather.go` — inner Do calls → op.Execute
- `elevate.go` — Do returns (Result, error)
- `wait_until.go` — Do returns (Result, error)

### Generator — `star/extensions/com.noblefactor.devlore.Actions/`
- `commands/generate.star` — signature-based detection, `action_kind` descriptor field
- `templates/graph_actions.go.template` — conditional Do signature (depends on framework approach)

### Generated actions — `pkg/op/provider/*/actions_gen.go` (regenerate all)
- Non-compensable (2-return): Do returns `(Result, error)`, no Undo
- Non-compensable (1-return): Do returns `(Result, error)` with `return result, nil`
- Compensable (3-return): Do returns `(Result, UndoState, error)`, has Undo — unchanged

### Tests
- `internal/execution/flow/flow_test.go` — mock action signatures
- `internal/execution/compensation_test.go` — mock compensable action signatures
- `internal/execution/provider_test.go` — if calling Do directly
- `pkg/op/action_test.go` (new) — Execute dispatch, type assertions

### Framework (separate work)
- `GoReceiver` template functions: `graphReturn`, `graphUndo` — support `action_kind`

## Verification

1. `make check` passes
2. Generated actions match their provider method signatures:
   - 3-return provider methods → CompensableAction with Undo
   - 2-return provider methods → Action without Undo
3. Executor correctly dispatches via `op.Execute()`
4. Recovery stack only pushes CompensableAction entries
5. Flow actions (choose, gather) still compensate correctly
6. Elevate and WaitUntil work as non-compensable Actions
