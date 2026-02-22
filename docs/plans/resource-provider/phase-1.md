# Phase 1: Interface Rename

## Context

Phase 1 is a mechanical rename across the codebase. No structural changes, no
new packages, no file moves (other than `operation.go` to `action.go`). The goal
is to establish the Action interface with `Do`/`Undo` signatures so subsequent
phases can build on the saga-ready contract.

**Repo**: devlore-cli
**Branch**: `feat/action-interface`

## Changes

### Core types (`internal/execution/action.go`)

Rename `operation.go` to `action.go`. Replace the `Operation` interface with:

```go
type Result = any
type UndoState = any

type Action interface {
    Name() string
    Do(ctx *Context, node *Node) (Result, UndoState, error)
    Undo(ctx *Context, node *Node, state UndoState) error
}
```

All 20 existing op structs: rename `Execute` to `Do` (return `nil, nil, err`),
add no-op `Undo` (return `nil`). This preserves current behavior while
establishing the interface contract.

### Registry (`internal/execution/registry.go`)

- `OperationRegistry` to `ActionRegistry`
- `NewOperationRegistry()` to `NewActionRegistry()`
- Internal `operations` map to `actions` map

### Node field (`internal/execution/graph.go`)

- `Node.Operation` to `Node.Action` (JSON/YAML tag: `action`)
- `GetOperation()` to `GetAction()` (or delete)
- Add `Retry *RetryPolicy` field to Node
- `Result` struct to `NodeResult` (avoids collision with new `Result` type alias)

### Executor (`internal/execution/executor.go`)

- Dispatch via `action.Do(ctx, node)` returning `(Result, UndoState, error)`
- `Result` references to `NodeResult`

### Registry catalog (`internal/execution/ops_registry.go`)

- `AllOps()` to `AllActions()`
- Delete `ValidateOp` (count drops from 21 to 20)

### Generated ops (6 `ops_*_gen.go` files)

Each file: rename `Execute` method to `Do` with triple return, add no-op `Undo`.

### Plan receivers (`internal/execution/plan.go`, `internal/starlark/plan*.go`, `internal/starlark/platform/*.go`)

All `Operation:` struct field initializers to `Action:`.

### Callers (`internal/writ/`, `internal/lore/`, `internal/manifest/`, `internal/cli/`)

- `execution.NewOperationRegistry()` to `execution.NewActionRegistry()`
- `execution.AllOps()` to `execution.AllActions()`
- `node.Operation` to `node.Action`
- `Operation:` to `Action:` in Node struct literals

### State view (`internal/execution/stateview.go`)

- `HistoryRecord.Operation` to `HistoryRecord.Action`
- `isPackageNode` and `isTransformOnlyNode` switch on `node.Action`

## Exclusions

These `Operation` references are in separate domains and must NOT be renamed:

| Location | Field | Reason |
|---|---|---|
| `writ/reconcile/reconcile.go` | `Entry.Operation` | Reconcile package's own type |
| `lorepackage/action.go` | `NativePMAction.Command` | PMCommand type, not execution.Action |
| `writ/tree/operation.go` | `Operation` type | Tree package's own type |
| `writ/migrate/format.go` | `nodeView.Operation` | Serialization view struct field |

## Files changed (40)

| File | Change |
|---|---|
| `execution/action.go` | Renamed from `operation.go`; Action interface with Do/Undo |
| `execution/registry.go` | ActionRegistry |
| `execution/graph.go` | Node.Action, NodeResult, Retry field |
| `execution/executor.go` | Dispatch via Do(), NodeResult |
| `execution/builder.go` | node.Action |
| `execution/preflight.go` | node.Action |
| `execution/stateview.go` | HistoryRecord.Action, node.Action |
| `execution/plan.go` | Action: in struct literals |
| `execution/ops_registry.go` | AllActions(), ValidateOp deleted |
| `execution/ops_file_gen.go` | Do/Undo |
| `execution/ops_encryption_gen.go` | Do/Undo |
| `execution/ops_package_gen.go` | Do/Undo |
| `execution/ops_shell_gen.go` | Do/Undo |
| `execution/ops_service_manager_gen.go` | Do/Undo |
| `execution/execution_test.go` | All references |
| `execution/phase_test.go` | All references, testRetryOp mock |
| `execution/stateview_test.go` | All references |
| `execution/dependencyview_test.go` | All references |
| `starlark/plan.go` | Action: in struct literals |
| `starlark/plan_file.go` | Action: in struct literals |
| `starlark/plan_package.go` | Action: in struct literals |
| `starlark/plan_root.go` | Action: in struct literals |
| `starlark/plan_archive_gen.go` | Action: in struct literals |
| `starlark/plan_git_gen.go` | Action: in struct literals |
| `starlark/platform/common.go` | Action: in struct literals |
| `starlark/platform/darwin.go` | Action: in struct literals |
| `starlark/platform/linux.go` | Action: in struct literals |
| `starlark/platform/windows.go` | Action: in struct literals |
| `writ/graph_builder.go` | ActionRegistry, AllActions |
| `writ/commands.go` | ActionRegistry, AllActions, node.Action |
| `writ/graph_test.go` | All references |
| `writ/migrate/session.go` | ActionRegistry, node.Action |
| `writ/migrate/execute.go` | node.Action |
| `writ/migrate/format.go` | node.Action (value only, not struct field) |
| `lore/builder.go` | node.Action |
| `lore/builder_test.go` | All references, NodeResult |
| `lore/commands.go` | ActionRegistry, AllActions |
| `manifest/builder.go` | Action: in struct literal |
| `manifest/manifest_test.go` | node.Action |
| `cli/receipts_test.go` | Action: in struct literal |

## Verification

```bash
cd /path/to/devlore-cli.resource-provider

go build ./...
go test ./... -count=1
go vet ./...

# Confirm zero remaining Operation in execution package
grep -rn '\bOperation\b' internal/execution/*.go  # expect 0 matches
```
