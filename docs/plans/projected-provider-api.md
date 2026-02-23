# The Projected Provider API — Full Realization

## Status: In Progress

Phase 1 (type ownership extraction to `pkg/projection/`) was completed in the
`feature/binding-unification` branch. This plan covers the remaining work:
moving Action types, providers, and the registry into the projection library,
and replacing the struct-level `//devlore:plannable` directive with method-level
`//+devlore:access=[immediate|planned|both]`.

## Context

The initial extraction moved `Node`, `Graph`, `Phase`, `Output`, `Gather`,
`FillSlot`, `Receiver`, type conversions, and node ID generation to
`pkg/projection/`. However, the Action interface, ActionRegistry, execution
Context, and all 10 providers still live in `internal/execution/`. This forces
`Node.Action` to be typed `any` (duck-typed at runtime), keeps HydrateGraph in
the execution layer, and prevents `pkg/projection/` from being a self-contained
projection library.

The struct-level `//devlore:plannable` directive is binary — the entire provider
is either plannable or not. Method-level `//+devlore:access=` gives per-method
control over which projection surface each method appears on.

## What Changes

### 1. Move Action types to `pkg/projection/`

**From** `internal/execution/action.go` **to** `pkg/projection/action.go`:
- `Action` interface (Name, Do)
- `CompensableAction` interface (Action + Undo)
- `Context` struct (DryRun, Writer, Data, Graph, NodeID)
- `Result`, `UndoState` type aliases
- `NotCompensableError`

**From** `internal/execution/registry.go` **to** `pkg/projection/registry.go`:
- `ActionRegistry` (unchanged API)

**From** `internal/execution/provider_registry.go` **to** `pkg/projection/provider_registry.go`:
- `ProviderRegistrar`, `RegisterProvider`, `RegisterAllProviders`

**Consequence**: `Node.Action` changes from `any` to `Action`. The duck-type
assertion in `ActionName()` is replaced with direct `Action` interface usage.
`HydrateGraph()` moves into `pkg/projection/` since it no longer needs
execution-layer types. `ApplyResults()` remains in `internal/execution/`
because it depends on `NodeResult`/`ResultStatus` (execution concerns).

### 2. Move providers to `pkg/projection/provider/`

Move 9 provider packages (ui stays in `internal/execution/provider/`):

| From | To |
|------|-----|
| `internal/execution/provider/file/` | `pkg/projection/provider/file/` |
| `internal/execution/provider/shell/` | `pkg/projection/provider/shell/` |
| `internal/execution/provider/service/` | `pkg/projection/provider/service/` |
| `internal/execution/provider/pkg/` | `pkg/projection/provider/pkg/` |
| `internal/execution/provider/template/` | `pkg/projection/provider/template/` |
| `internal/execution/provider/encryption/` | `pkg/projection/provider/encryption/` |
| `internal/execution/provider/net/` | `pkg/projection/provider/net/` |
| `internal/execution/provider/git/` | `pkg/projection/provider/git/` |
| `internal/execution/provider/archive/` | `pkg/projection/provider/archive/` |

Each provider's `actions_gen.go` changes its import from `internal/execution`
to `pkg/projection` for Action, Context, ActionRegistry, etc.

The `ui` provider remains at `internal/execution/provider/ui/` — it has no
actions_gen.go, no Action wrappers, and is consumed only by the immediate
receiver and CLI output wrappers.

### 3. Replace `//devlore:plannable` with `//+devlore:access=`

**Current**: `//devlore:plannable` on the Provider struct type. Binary: the
entire provider is either plannable or not.

**New**: `//+devlore:access=[immediate|planned|both]` on each method.
Per-method control over which projection surface the method appears on.

Default (no directive) = `immediate`.

Access level assignments:

| Provider | Method | Access | Rationale |
|----------|--------|--------|-----------|
| file | Link, Copy, Backup, Unlink, Remove, Write, Move | planned | Compensable state-changing |
| file | Source, Mkdir | planned | Graph-only execution |
| file | Exists, IsDir | both | Predicates for control flow + plan.choose |
| git | Clone, Checkout, Pull, Config | planned | Compensable state-changing |
| git | CurrentBranch, IsRepo | both | Predicates |
| archive | Extract | planned | Compensable |
| encryption | Decrypt | planned | Compensable |
| net | Download | planned | Compensable |
| pkg | Install, Remove, Upgrade | planned | Compensable state-changing |
| pkg | IsInstalled, Version | both | Predicates |
| service | Enable, Disable, Start, Stop, Restart | planned | Compensable |
| service | IsActive, IsEnabled | both | Predicates |
| shell | Run | planned | Side-effecting |
| template | Render | planned | Graph-only |
| ui | Note, Warn, Error, Success, Fail | immediate | Script-time feedback |

### 4. Update code generator

**File**: `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star`

Changes:
- Remove struct-level `//devlore:plannable` detection
- Add method-level `//+devlore:access=` parsing per method
- Group methods by access level: immediate-only, planned-only, both
- Generate per access level:
  - `access=immediate` → include in immediate receiver only
  - `access=planned` → include in planned receiver + action wrapper
  - `access=both` → include in all three artifacts
- Always generate all three files if ANY method targets that surface

### 5. Update templates

**`graph_actions.go.template`**:
- Change import from `internal/execution` to `pkg/projection`
- Only emit action wrappers for methods with `access=planned` or `access=both`

**`plan_receiver.go.template`**:
- Import path already uses `pkg/projection` — no change needed
- Only emit plan methods for `access=planned` or `access=both`

**`realtime_receiver.go.template`**:
- Only emit immediate methods for `access=immediate` or `access=both`
- Change any remaining `internal/execution` imports to `pkg/projection`

### 6. Update `internal/execution/`

What stays:
- `executor.go` — `GraphExecutor`, `NodeResult`, `ResultStatus`, `ApplyResults`
- `recovery.go` — `RecoveryStack`, `RecoveryEntry`
- `hooks.go` — `LifecycleHook`, `HookRegistry`
- `flow/` — flow control actions
- Remaining orchestration files

What changes:
- All imports of `Action`, `ActionRegistry`, `Context` → `pkg/projection`
- Old `action.go`, `registry.go`, `provider_registry.go` → type alias re-exports
  (allows compilation while old files exist; removed once user runs cleanup)
- `graph.go` → `ApplyResults` moves to `executor.go`; rest deleted after
  `HydrateGraph` moves to `pkg/projection/`
- `internal/execution/provider/` directory → dead code after providers move

### 7. Update consumers

- `internal/starlark/plan_registry.go`: `*execution.ActionRegistry` → `*projection.ActionRegistry`
- `internal/starlark/plan_root.go`: same import change
- `internal/starlark/output.go`: FillSlot bridge unchanged
- `internal/lore/builder.go`: import changes for ActionRegistry
- `internal/writ/graph_builder.go`: import changes for ActionRegistry
- `internal/cli/output.go`: import changes for ui provider path
- All generated `plan_*_gen.go` and `receiver_*_gen.go`: updated import paths
- All test files: updated import paths

### 8. Fix plan document status

Update this document's status line upon completion.

## Execution Order

1. Create Action/Context/Registry types in `pkg/projection/`
2. Replace old `internal/execution/` definition files with type alias re-exports
3. Update `pkg/projection/graph.go` — `Node.Action` becomes `Action`, add `HydrateGraph`
4. Copy 9 provider packages to `pkg/projection/provider/` with updated imports
5. Add `//+devlore:access=` directives to every provider method
6. Update `register.go` for new provider paths
7. Update all consumer imports (lore, writ, starlark, cli, flow, tests)
8. Update generated `plan_*_gen.go` and `actions_gen.go` files
9. Update `generate.star` to parse method-level directives
10. Update templates for new import paths and per-method filtering
11. `make check` — fix all issues
12. Update plan document status

## Files Created in `pkg/projection/`

- `action.go` — Action, CompensableAction, Context, Result, UndoState, NotCompensableError
- `registry.go` — ActionRegistry
- `provider_registry.go` — ProviderRegistrar, RegisterProvider, RegisterAllProviders
- `provider/` — 9 subdirectories with provider.go + actions_gen.go each
- `provider/register.go` — RegisterAll with blank imports

## Files Modified in `internal/execution/`

- `action.go` — gutted to type alias re-exports
- `registry.go` — gutted to type alias re-exports
- `provider_registry.go` — gutted to type alias re-exports
- `graph.go` — ApplyResults moves to executor.go; file becomes alias stub
- `executor.go` — absorbs ApplyResults, imports change
- `recovery.go` — import changes
- `hooks.go` — import changes
- `flow/*.go` — import changes

## Files Modified in `internal/starlark/`

- `plan_registry.go` — import change
- `plan_root.go` — import change
- `plan_*_gen.go` — import changes (9 files)
- `receiver_ui_gen.go` — import change for provider path

## Files Modified in Consumers

- `internal/lore/builder.go` — import changes
- `internal/writ/graph_builder.go` — import changes
- `internal/writ/commands.go` — import changes
- `internal/writ/migrate/*.go` — import changes
- `internal/cli/output.go` — import change for ui provider
- All test files referencing execution types

## Verification

1. `make check` passes (vet, lint, test)
2. Generated code imports `pkg/projection` — no references to `internal/execution` for Action/Registry/Context
3. Grep for `//devlore:plannable` — zero matches in provider source
4. Grep for `internal/execution/provider` — only ui and dead code awaiting deletion
5. Each provider method has `//+devlore:access=` or falls to default (immediate)
6. `pkg/projection/` has zero imports from `internal/`
7. Type alias re-exports in `internal/execution/` compile and are unused by production code
