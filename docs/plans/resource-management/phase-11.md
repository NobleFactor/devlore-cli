# Action Interface Unification — Three Action Types with Unified Do

## Context

The resource management plan requires pure provider methods (e.g.,
`file.Name`, `file.Parent`, `file.Join`) to participate as graph-mode
actions. Previously, the codegen excluded methods without error returns
from action registration and bridge tests, treating them as
immediate-only Starlark builtins. This was the direct consequence of
an earlier design where the three action flavors had incompatible `Do`
return signatures, requiring a `DoAction()` type-switch dispatcher to
normalize.

This phase unifies the `Do` return signature across all three action
types, eliminates the dispatcher, and updates the codegen to register
and test pure actions as first-class graph nodes.

Supersedes the working plan at `docs/plans/normalize-action-do.md`.

## Goals

1. **Unified Do signature**: All three action types share
   `Do(ctx, slots) (Result, Complement, error)`.
2. **Eliminate DoAction dispatcher**: Each reflected type normalizes
   internally — no external type switch.
3. **Pure actions in the graph**: Methods like `file.Name`,
   `file.Parent`, `file.Join` are registered as `Action` nodes with
   generated bridge tests.

## Steps Completed

### 1. Unified Action interfaces

**File**: `pkg/op/action.go`

Three interfaces, all sharing the same `Do` signature:

```go
type Action interface {
    Name() string
    Params() []ParamInfo
    Do(ctx *Context, slots map[string]any) (Result, Complement, error)
}

type FallibleAction interface {
    Action
}

type CompensableAction interface {
    Action
    Undo(ctx *Context, complement Complement) error
}
```

Deleted: `NodeAction` (renamed to `Action`), old per-type `Do`
signatures, `DoAction()` function.

### 2. Reflected type normalization

**File**: `pkg/op/action_reflect.go`

Each reflected type's `Do` method normalizes its provider method's
actual return values into the unified signature:

| Reflected Type | Provider Signature | Normalized Return |
|---|---|---|
| `reflectedPureAction` | `(T)` or `()` | `(result, nil, nil)` |
| `reflectedFallibleAction` | `(T, error)` | `(result, nil, err)` |
| `reflectedCompensableAction` | `(T, U, error)` | `(result, complement, err)` |

`reflectedPureAction.Do` panics on coercion errors (framework bug,
not runtime failure) and respects dry-run mode.

`InitActionProvider` simplified from 3-case type switch to
`hasProvider` interface assertion on `actionBase`.

### 3. Dry-run guard for pure actions

**File**: `pkg/op/action_reflect.go`

Added dry-run handling to `reflectedPureAction.Do` for consistency
with fallible and compensable actions. Pure actions log
`[dry-run] provider.method_name` and return `(nil, nil, nil)`.

### 4. DoAction call sites replaced

Three call sites became direct `action.Do()` calls:

| File | Before | After |
|---|---|---|
| `internal/execution/executor.go` | `op.DoAction(node.Action, ctx, slots)` | `node.Action.Do(ctx, slots)` |
| `internal/execution/flow/gather.go` | `op.DoAction(node.Action, ctx, nodeSlots)` | `node.Action.Do(ctx, nodeSlots)` |
| `internal/execution/flow/choose.go` | `op.DoAction(node.Action, ctx, nodeSlots)` | `node.Action.Do(ctx, nodeSlots)` |

### 5. Flow actions updated

**Files**: `internal/execution/flow/elevate.go`,
`internal/execution/flow/wait_until.go`

Both flow action types updated from 2-return `Do` to 3-return
`(Result, Complement, error)`.

### 6. Codegen — include pure actions

**File**: `star/extensions/.../commands/generate.star`

Removed the error-return filter that excluded pure methods from
`actions_gen_test.go`. Previously:

```python
# REMOVED — incorrectly excluded pure actions
action_methods = []
for d in provider_method_descs:
    if "error" in d.get("returns", ""):
        action_methods.append(d)
```

Now all methods (pure, fallible, compensable) are passed to the
`actions_test` template. Added `pure` field to method descriptors
for template conditionals.

### 7. Test updates

All test files updated for unified 3-return `Do` signature:

| File | Changes |
|---|---|
| `pkg/op/graph_test.go` | `NodeAction` → `Action`, `testAction.Do` 3-return, `DoAction` → direct calls |
| `pkg/op/registry_test.go` | `registryTestAction.Do` 3-return |
| `pkg/op/action_reflect_test.go` | `DoAction(action, ctx, ...)` → `action.Do(ctx, ...)` |
| `pkg/op/planned_reflect_test.go` | Stub action `Do` signatures 3-return |
| `internal/execution/compensation_test.go` | `failAction`, `noopAction`, `conditionalFailAction` 3-return |
| `internal/execution/flow_test.go` | `NodeAction` → `Action`, WaitUntil/Elevate test calls 3-return |
| `internal/execution/execution_test.go` | `NodeAction` → `Action` |
| `internal/execution/phase_test.go` | `NodeAction` → `Action` |
| `internal/execution/flow/flow_test.go` | `NodeAction` → `Action`, WaitUntil/Elevate test calls 3-return |
| `internal/starlark/binding_set_test.go` | `testAction.Do` 3-return |
| `pkg/op/provider/file/gen/actions_test.go` | `op.DoAction(action, ctx, slots)` → `action.Do(ctx, slots)` |

All `_gen_test.go` files regenerated via `make generate`.

## New Pure Actions Registered

After this phase, the following methods are registered as graph-mode
`Action` nodes (previously immediate-only):

| Provider | Method | Action Name |
|---|---|---|
| file | `Name` | `file.name` |
| file | `Parent` | `file.parent` |
| file | `Join` | `file.join` |

Additional methods newly included in generated bridge tests (were
already registered but lacked generated tests):

| Provider | Method | Action Name | Type |
|---|---|---|---|
| file | `Exists` | `file.exists` | Fallible |
| file | `IsDir` | `file.is_dir` | Fallible |
| file | `IsFile` | `file.is_file` | Fallible |
| file | `Glob` | `file.glob` | Fallible |
| file | `Mkdir` | `file.mkdir` | Fallible |
| file | `Read` | `file.read` | Fallible |

## Verification

- `make build` — compiles, regenerates all gen files
- `make test` — all tests pass (only pre-existing `walk_tree` `fn`
  param failure remains — #170)
- `grep -r "NodeAction\|DoAction" pkg/ internal/` — no references
  in non-generated Go files

## Files Modified

| File | Nature of change |
|---|---|
| `pkg/op/action.go` | Interface unification, delete DoAction/NodeAction |
| `pkg/op/action_reflect.go` | Unified Do on all 3 types, dry-run for pure, simplify InitActionProvider |
| `pkg/op/recovery.go` | NodeAction → Action |
| `pkg/op/registry.go` | NodeAction → Action |
| `pkg/op/graph.go` | NodeAction → Action, stubAction Do signature |
| `internal/execution/executor.go` | DoAction → direct call |
| `internal/execution/flow/gather.go` | DoAction → direct call |
| `internal/execution/flow/choose.go` | DoAction → direct call |
| `internal/execution/flow/elevate.go` | Do 2-return → 3-return |
| `internal/execution/flow/wait_until.go` | Do 2-return → 3-return |
| `star/.../commands/generate.star` | Remove error-return filter, add `pure` field |
| `docs/architecture/devlore-resource-management.md` | Document action type model |
