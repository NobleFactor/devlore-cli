# Plan: Resource-Provider Model

```yaml
title: Resource-Provider Model
issue: TBD
status: draft
created: 2026-02-14
updated: 2026-02-14
```

## Summary

Restructure `internal/execution` from a flat Service/Operation model into a
Resource-Provider architecture optimized for the Saga Pattern. Services become
Providers in domain subpackages (`provider/file`, `provider/pkg`, etc.).
Operations become Actions with `Do`/`Undo` methods. The three flow primitives
from the
[convergence operations architecture](../architecture/devlore-graph-convergence-operations.md)
move into an `execution/flow` package as typed Actions: `flow.Choose`
(OR-selector), `flow.Gather` (AND-join), and `flow.Elevate` (privilege
transition).

## Goals

1. **Action interface with Do/Undo.** Every action implements forward (Do) and
   compensating (Undo) methods, enabling the Saga Pattern at the action level
   rather than only at the phase level.
2. **Provider subpackages.** Each resource domain gets its own package with a
   Provider struct and typed Action structs: `file.Provider`, `file.Copy`,
   `pkg.Provider`, `pkg.Install`, etc.
3. **Dotted action names.** Actions return `"file.link"`, `"file.copy"`,
   `"pkg.install"` — the domain prefix matches the provider package.
4. **Flow package.** The three graph flow primitives — Choose (OR-selector),
   Gather (AND-join), and Elevate (privilege transition) — live in
   `execution/flow`. Each implements Action.
5. **No import cycles.** Provider packages import `execution` for the Action
   interface; `execution` never imports provider packages. Wiring happens at
   call sites (writ, lore).
6. **Generator-ready.** Action files use the `_gen.go` suffix and follow the
   patterns produced by `star gen.receiver`. Hand-written now, regenerable later.

## Current State

| Component | Current | Target |
|---|---|---|
| Operation interface | `Name()`, `Execute(ctx, node)` | Action: `Name()`, `Do(ctx, node) (Result, UndoState, error)`, `Undo(ctx, node, UndoState) error` |
| Services (5) | Flat in `execution` package | Provider structs in `provider/*` subpackages |
| Actions (20 structs) | Flat `_gen.go` files in `execution` | Action structs (`_gen.go`) in `provider/*` |
| Op names | Flat: `"link"`, `"copy"` | Dotted: `"file.link"`, `"file.copy"` |
| `AllOps()` | Returns all ops, lives in `execution` | Deleted — each provider has `Register(reg)` |
| `OperationRegistry` | Maps name → Operation | `ActionRegistry` maps name → Action |
| `Node.Operation` | Stores dispatch key | `Node.Action` stores dispatch key |
| ValidateOp | Hand-written in `ops_registry.go` | Deleted — no legacy |
| Gather | Starlark-only (`starlark/output.go`) | `flow.Gather` action in `execution/flow/` |
| — | No privilege model | `flow.Elevate` action in `execution/flow/` |
| `Node.Retry` | Not on Node (phase-level only) | `*RetryPolicy` on Node, settable from plan receivers |
| Plan receivers | Emit flat names | Emit dotted names |
| Generator templates | `graph_ops.go.template` in Ops extension | `graph_actions.go.template` in Actions extension |

## Vernacular

| Term | Definition | Replaces |
|---|---|---|
| **Action** | Unit of work with Do (forward) and Undo (compensate) | Operation |
| **Provider** | Stateless resource driver; methods are the source of truth for `star gen.receiver` | Service |
| **Choose** | OR-selector: evaluates alternatives, selects one (`flow.Choose`) | — |
| **Gather** | AND-join: waits for all predecessors (`flow.Gather`) | Starlark-only construct |
| **Elevate** | Privilege transition: acquire/release elevated execution context (`flow.Elevate`) | — |

## Package Layout

```
internal/execution/
  action.go              — Action interface, Result, UndoState, Context
  graph.go               — Graph, Node (Action field), Edge, SlotValue, RetryPolicy
  registry.go            — ActionRegistry (renamed from OperationRegistry)
  plan.go                — Plan types
  stateview.go           — StateView
  dependencyview.go      — DependencyView

  engine/
    executor.go          — GraphExecutor (dispatches via Action.Do)
    phase.go             — Phase execution (saga pattern)
    recovery.go          — RecoveryStack (rollback via UndoState)

  build/
    builder.go           — GraphBuilder, SubgraphBuilder, ExpandDelegates
    preflight.go         — Preflight conflict detection

  flow/
    choose.go            — flow.Choose (OR-selector)
    gather.go            — flow.Gather (AND-join)
    elevate.go           — flow.Elevate (privilege transition)

  provider/
    file/
      provider.go        — file.Provider (from FileService)
      actions_gen.go     — file.Link, file.Copy, file.Render, file.Backup,
                           file.Unlink, file.Remove, file.Write, file.Move
      helpers.go         — pruneParents, isSubpath
    encryption/
      provider.go        — encryption.Provider (from EncryptionService)
      actions_gen.go     — encryption.Decrypt
    pkg/
      provider.go        — pkg.Provider (from PackageService)
      actions_gen.go     — pkg.Install, pkg.Upgrade, pkg.Remove, pkg.Update
      helpers.go         — resolvePM*, runBrewCask*
    shell/
      provider.go        — shell.Provider (from ShellService)
      actions_gen.go     — shell.Exec, shell.PowerShell
    service/
      provider.go        — service.Provider (from ServiceManagerService)
      actions_gen.go     — service.Start, service.Stop, service.Restart,
                           service.Enable, service.Disable
      helpers.go         — run()

  flow/
    choose.go            — flow.Choose (OR-selector)
    gather.go            — flow.Gather (AND-join)
    elevate.go           — flow.Elevate (privilege transition)
```

### File naming convention

| Suffix | Contents | Example |
|---|---|---|
| `provider.go` | Provider struct + methods (hand-written, source of truth) | `file.Provider.Link()` |
| `actions_gen.go` | Action structs: Name, Do, Undo, Register (generated by `star gen.receiver`) | `file.Link`, `file.Copy` |
| `helpers.go` | Package-private helpers (hand-written) | `pruneParents`, `resolvePM*` |

The `_gen.go` files are hand-written in this plan but follow the exact patterns
that `star gen.receiver` produces via the `graph_actions` template. Once the template
is updated (Phase 3), the files become nuke-safe: `rm *_gen.go` + regenerate.

## Flow Actions

The `execution/flow` package contains the three graph flow primitives — actions
that change how the executor traverses the graph rather than operating on
resources. Each implements the Action interface. All live in
`internal/execution/flow/`.

See: [Convergence Operations Architecture](../architecture/devlore-graph-convergence-operations.md)

### flow.Choose

OR-selector: evaluates alternatives and selects one. Multiple predecessors
represent options, not dependencies. The node picks based on criteria —
platform, availability, preference, or runtime condition. Only the selected
predecessor is executed; unchosen branches are skipped.

Choose implements the full OR-selector from the convergence operations
architecture: multiple alternatives, selection criteria, predecessor skipping.

```go
type Choose struct{}

func (a *Choose) Name() string { return "choose" }
func (a *Choose) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) { /* evaluate criteria */ }
func (a *Choose) Undo(ctx *execution.Context, node *execution.Node, _ execution.UndoState) error { return nil }
```

### flow.Gather

AND-join: waits for all predecessors to complete before proceeding. Equivalent
to `Promise.all()` — every input must succeed for the gather node to succeed.

Currently Gather is a Starlark-only construct (`starlark/output.go`) that
creates edges but is never dispatched by the executor. As a flow action, the
executor can enforce AND-join semantics, record results, and fail explicitly
when a predecessor fails.

```go
type Gather struct{}

func (a *Gather) Name() string { return "gather" }
func (a *Gather) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) { /* verify all predecessors succeeded */ }
func (a *Gather) Undo(ctx *execution.Context, node *execution.Node, _ execution.UndoState) error { return nil }
```

### flow.Elevate

Privilege transition: makes the boundary between unprivileged and privileged
execution visible as an explicit graph node. Dry-run shows "root required here";
the receipt records when privilege was acquired and released.

```go
type Elevate struct{}

func (a *Elevate) Name() string { return "elevate" }
func (a *Elevate) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) { /* acquire privilege */ }
func (a *Elevate) Undo(ctx *execution.Context, node *execution.Node, state execution.UndoState) error { /* release privilege */ }
```

### Registration

```go
package flow

func Register(reg *execution.ActionRegistry) {
    reg.Register(&Choose{})
    reg.Register(&Gather{})
    reg.Register(&Elevate{})
}
```

## Code Generation Pipeline

The `star gen.receiver` extension (noblefactor-ops) reads Provider method
signatures and generates:

1. **Action structs** (`actions_gen.go`) — via the `graph_actions` template
2. **Plan receivers** (`plan_*_gen.go`) — via the `plan_receiver` template

The templates live in the devlore-cli Actions extension:
`star/extensions/com.noblefactor.devlore.Actions/templates/`

| Template | Generates | Updated in |
|---|---|---|
| `graph_actions.go.template` | Action structs (Do/Undo, Register) | Phase 3 |
| `plan_receiver.go.template` | Plan namespace structs (node creation) | Phase 3 |

Phase 2 hand-writes the `_gen.go` files following template patterns exactly.
Phase 3 updates the templates so the generator can reproduce them.

## Phase 1: Interface Rename

Mechanical rename across the repo. No structural changes.

**Repo**: devlore-cli
**Branch**: `feat/action-interface`

### 1a: Core types

`internal/execution/action.go` (rename from `operation.go`):

```go
// Result is data that flows to downstream nodes via edges (e.g., a checksum,
// a rendered template, a query result). The executor stores this on the node
// for edge-based slot resolution.
type Result = any

// UndoState is the state captured by Do and passed to Undo during saga
// rollback. Each action defines its own state shape. Actions with no rollback
// return nil from Do; their Undo ignores the state parameter.
type UndoState = any

type Action interface {
    Name() string
    Do(ctx *Context, node *Node) (Result, UndoState, error)
    Undo(ctx *Context, node *Node, state UndoState) error
}
```

All 20 existing op structs: rename `Execute` → `Do` (return `nil, nil, err`),
add no-op `Undo` (ignore state, return `nil`).

`internal/execution/registry.go`:

```go
type ActionRegistry struct {
    actions map[string]Action
}

func NewActionRegistry() *ActionRegistry
func (r *ActionRegistry) Register(a Action)
func (r *ActionRegistry) Get(name string) (Action, bool)
func (r *ActionRegistry) Names() []string
```

### 1b: Node field

`internal/execution/graph.go`:

```go
type Node struct {
    // ...
    Action string         `json:"action" yaml:"action"`  // was Operation
    Retry  *RetryPolicy   `json:"retry,omitempty" yaml:"retry,omitempty"`
    // ...
}
```

`RetryPolicy` already exists in `phase.go`. Adding it to Node lets any action
declare retry behavior. Plan receivers accept `retry_policy` as a standard
argument:

```python
retry = execution.retry_policy(max_attempts=3, backoff="exponential")
plan.pkg.install(packages=["docker"], retry_policy=retry)
```

Update `GetOperation()` → `GetAction()` (or delete — callers use the field
directly). Update all plan receivers that set `Operation:` to set `Action:`.

### 1c: Executor dispatch

`internal/execution/engine/executor.go`:

```go
func (e *GraphExecutor) executeNode(ctx *Context, node *Node) *Result {
    actionName := node.Action
    // ...
    action, ok := e.registry.Get(actionName)
    // ...
    result, rollbackState, err := action.Do(ctx, node)
    if err != nil {
        // retry if node.Retry is set, then fail
    }
    // store result on node for edge-based slot resolution
    node.StoreResult(result)
    // store rollbackState on recovery stack for saga rollback
    e.recovery.Push(action, node, rollbackState)
    // ...
}
```

On rollback, the recovery stack unwinds in reverse order:

```go
func (s *RecoveryStack) Unwind(ctx *Context) error {
    for i := len(s.entries) - 1; i >= 0; i-- {
        entry := s.entries[i]
        if err := entry.Action.Undo(ctx, entry.Node, entry.State); err != nil {
            // record but don't mask original error
        }
    }
    return nil
}
```

### 1d: Callers

Update all callers in `writ`, `lore`, `starlark`, and tests:

- `execution.NewOperationRegistry()` → `execution.NewActionRegistry()`
- `execution.AllOps()` → `execution.AllActions()` (temporary — deleted in Phase 2)
- `execution.Operation` → `execution.Action`
- `node.Operation` → `node.Action`
- `node.GetOperation()` → `node.GetAction()` (if kept)

### 1e: Tests

Update `execution_test.go`, `phase_test.go`, `stateview_test.go`,
`dependencyview_test.go`, and all tests in `writ` and `lore` that reference
Operation/OperationRegistry.

### 1f: Graph serialization

`ComputeSummary` switches on `n.Action` (still flat names in this phase).
`Preflight` reads `node.Action`.
YAML output changes from `operation: link` to `action: link`.

### Files

| File | Change |
|---|---|
| `operation.go` → `action.go` | Rename interface + methods |
| `registry.go` | OperationRegistry → ActionRegistry |
| `graph.go` | Node.Operation → Node.Action, GetOperation → GetAction |
| `executor.go` → `engine/executor.go` | Dispatch via Action.Do, field rename |
| `phase.go` → `engine/phase.go` | ExecutePhaseInner: action.Do returns (result, state, error); retry via node.Retry |
| `recovery.go` → `engine/recovery.go` | RecoveryStack entries store Action + Node + UndoState |
| `builder.go` → `build/builder.go` | isDelegateNode: node.Action |
| `preflight.go` → `build/preflight.go` | node.Action |
| `ops_*_gen.go` (6 files) | Execute → Do, add Undo |
| `ops_registry.go` | AllOps → AllActions, ValidateOp deleted |
| `execution_test.go` | All references |
| `phase_test.go` | All references |
| `stateview.go` | Operation references |
| `dependencyview.go` | Operation references |
| `starlark/plan_*.go` | Operation: → Action: |
| `starlark/plan.go` | Operation: → Action: |
| `starlark/platform/*.go` | Operation: → Action: |
| `writ/graph_builder.go` | Registry references |
| `writ/commands.go` | Registry references |
| `writ/migrate/session.go` | Registry references |
| `lore/commands.go` | Registry references |
| `lore/builder_test.go` | Registry references |

## Phase 2: Provider Extraction

Move services and actions into domain subpackages. Adopt dotted names.
Wire providers from call sites.

**Repo**: devlore-cli
**Branch**: `feat/provider-model`

### 2a: Create provider/file

`internal/execution/provider/file/provider.go` — file.Provider (from FileService):

```go
package file

type Provider struct{}

func (p *Provider) Link(source, path string) error { /* ... */ }
func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, error) { /* ... */ }
func (p *Provider) Render(templateData map[string]any, source, path, project string, content []byte) ([]byte, error) { /* ... */ }
func (p *Provider) Backup(path, backupSuffix string) (string, error) { /* ... */ }
func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) error { /* ... */ }
func (p *Provider) Remove(path string, prune bool, pruneBoundary string) error { /* ... */ }
func (p *Provider) Write(content, path string, mode os.FileMode) error { /* ... */ }
func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) error { /* ... */ }
```

`internal/execution/provider/file/actions_gen.go` — action structs:

```go
// Code generated from gen-receiver templates; DO NOT EDIT.

package file

import (
    "fmt"
    "github.com/NobleFactor/devlore-cli/internal/execution"
)

type Link struct{ Impl *Provider }
func (a *Link) Name() string { return "file.link" }
func (a *Link) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) { /* ... */ }
func (a *Link) Undo(ctx *execution.Context, node *execution.Node, state execution.UndoState) error { return nil }

type Copy struct{ Impl *Provider }
func (a *Copy) Name() string { return "file.copy" }
func (a *Copy) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) { /* ... */ }
func (a *Copy) Undo(ctx *execution.Context, node *execution.Node, state execution.UndoState) error { return nil }

// ... all 8 actions

func Register(reg *execution.ActionRegistry) {
    p := &Provider{}
    reg.Register(&Link{Impl: p})
    reg.Register(&Copy{Impl: p})
    reg.Register(&Render{Impl: p})
    reg.Register(&Backup{Impl: p})
    reg.Register(&Unlink{Impl: p})
    reg.Register(&Remove{Impl: p})
    reg.Register(&Write{Impl: p})
    reg.Register(&Move{Impl: p})
}
```

`internal/execution/provider/file/helpers.go` — pruneParents, isSubpath.

### 2b: Create remaining provider packages

Same pattern for each:

| Package | Provider | Actions (`_gen.go`) | Helpers |
|---|---|---|---|
| `provider/encryption` | encryption.Provider | encryption.Decrypt | — |
| `provider/pkg` | pkg.Provider | pkg.Install, pkg.Upgrade, pkg.Remove, pkg.Update | resolvePM*, runBrewCask* |
| `provider/shell` | shell.Provider | shell.Exec, shell.PowerShell | — |
| `provider/service` | service.Provider | service.Start, service.Stop, service.Restart, service.Enable, service.Disable | run() |

### 2c: Create flow package

`internal/execution/flow/` — three flow actions.

**`choose.go`** — OR-selector. Evaluates alternatives, selects one, skips the
rest. ValidateOp is deleted, not migrated.

**`gather.go`** — AND-join. Verifies all predecessors succeeded. Promotes
Gather from a Starlark-only edge-creation construct to an executor-dispatched
action.

**`elevate.go`** — Privilege transition. Acquires or releases elevated
execution context. Stub implementation in this phase; full sudo/privilege
integration is a separate plan.

### 2d: Delete AllActions, wire from call sites

Delete `AllActions()` from `ops_registry.go`. Each caller builds its registry:

```go
import (
    "github.com/NobleFactor/devlore-cli/internal/execution"
    "github.com/NobleFactor/devlore-cli/internal/execution/flow"
    filep "github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
    encp  "github.com/NobleFactor/devlore-cli/internal/execution/provider/encryption"
    pkgp  "github.com/NobleFactor/devlore-cli/internal/execution/provider/pkg"
    shellp "github.com/NobleFactor/devlore-cli/internal/execution/provider/shell"
    svcp  "github.com/NobleFactor/devlore-cli/internal/execution/provider/service"
)

func newActionRegistry() *execution.ActionRegistry {
    reg := execution.NewActionRegistry()
    filep.Register(reg)
    encp.Register(reg)
    pkgp.Register(reg)
    shellp.Register(reg)
    svcp.Register(reg)
    flow.Register(reg)
    return reg
}
```

Callers updated: `writ/graph_builder.go`, `writ/commands.go`,
`lore/commands.go`, `writ/migrate/session.go`.

### 2e: Delete old files

| File | Disposition |
|---|---|
| `file_service.go` | Deleted — moved to `provider/file/provider.go` |
| `encryption_service.go` | Deleted — moved to `provider/encryption/provider.go` |
| `package_service.go` | Deleted — moved to `provider/pkg/provider.go` |
| `shell_service.go` | Deleted — moved to `provider/shell/provider.go` |
| `service_manager_service.go` | Deleted — moved to `provider/service/provider.go` |
| `ops_file_gen.go` | Deleted — moved to `provider/file/actions_gen.go` |
| `ops_encryption_gen.go` | Deleted — moved to `provider/encryption/actions_gen.go` |
| `ops_package_gen.go` | Deleted — moved to `provider/pkg/actions_gen.go` |
| `ops_shell_gen.go` | Deleted — moved to `provider/shell/actions_gen.go` |
| `ops_service_manager_gen.go` | Deleted — moved to `provider/service/actions_gen.go` |
| `ops_registry.go` | Deleted — AllActions deleted, Choose moved to `flow/choose.go` |

### 2f: Adopt dotted action names in plan receivers

Update all plan receivers to emit dotted names:

| Old Name | New Name | Plan Receivers |
|---|---|---|
| `link` | `file.link` | plan_file.go, plan.go, platform/*.go |
| `copy` | `file.copy` | plan_file.go, plan.go, platform/*.go |
| `render` | `file.render` | plan_file.go, plan.go, platform/*.go |
| `write` | `file.write` | plan_file.go, plan.go, platform/*.go |
| `remove` | `file.remove` | plan_file.go, plan.go, platform/*.go |
| `backup` | `file.backup` | (not used in plan receivers) |
| `unlink` | `file.unlink` | (not used in plan receivers) |
| `move` | `file.move` | (not used in plan receivers) |
| `decrypt` | `encryption.decrypt` | (wired via executor, not plan receiver) |
| `package-install` | `pkg.install` | plan_package.go, plan.go, platform/*.go |
| `package-upgrade` | `pkg.upgrade` | plan_package.go, plan.go, platform/*.go |
| `package-remove` | `pkg.remove` | plan_package.go, plan.go, platform/*.go |
| `package-update` | `pkg.update` | plan_package.go, plan.go, platform/*.go |
| `shell` | `shell.exec` | plan_root.go, plan.go, platform/*.go |
| `powershell` | `shell.powershell` | platform/windows.go |
| `service-start` | `service.start` | plan_root.go, plan.go |
| `service-stop` | `service.stop` | plan_root.go, plan.go |
| `service-restart` | `service.restart` | plan_root.go, plan.go |
| `service-enable` | `service.enable` | plan_root.go, plan.go |
| `service-disable` | `service.disable` | plan_root.go, plan.go |
| `validate` | deleted | — |
| — | `gather` | (new — flow action) |
| — | `elevate` | (new — flow action) |

Platform-specific service names (`launchd-start`, `systemd-start`,
`winservice-start`) converge to `service.start` etc. The `service.Provider`
already detects platform at runtime via `runtime.GOOS`.

### 2g: Update ComputeSummary and Preflight

`ComputeSummary` switches on `n.Action` using dotted names:

```go
switch n.Action {
case "file.link":
    g.Summary.Links++
case "file.render":
    g.Summary.Templates++
case "encryption.decrypt":
    g.Summary.Secrets++
case "file.copy":
    g.Summary.Copies++
case "pkg.install", "pkg.upgrade", "pkg.remove":
    g.Summary.Packages++
}
```

`Preflight` checks `node.Action == "file.link" || node.Action == "file.copy"`.

### 2h: Tests

- Create provider package tests for each provider
- Update `execution_test.go` for dotted names
- Update `phase_test.go` mock actions
- Update `writ/graph_test.go`, `lore/builder_test.go`
- Update `starlark/receiver_test.go` expected node actions

## Phase 3: Generator Templates

Update the `star gen.receiver` templates so the `_gen.go` files become
regenerable. After this phase, `rm provider/*/actions_gen.go` + re-run
`star gen.receiver` produces identical output.

**Repos**: noblefactor-ops (template engine), devlore-cli (template files)

### 3a: graph_actions template (noblefactor-ops)

The `go.generate()` `graph_actions` template currently emits:

- `type FileLinkOp struct{ impl *FileService }`
- `func (o *FileLinkOp) Execute(ctx *Context, node *Node) error`
- `func FileOps(impl *FileService) []Operation`

Update to emit:

- `type Link struct{ Impl *Provider }`
- `func (a *Link) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error)`
- `func (a *Link) Undo(ctx *execution.Context, node *execution.Node, state execution.UndoState) error`
- `func Register(reg *execution.ActionRegistry)`
- `Name()` returns dotted: `"file.link"` (prefix from provider package)

The template receives the provider package name as a variable and uses it
for the dotted prefix.

**Branch**: `feat/action-templates`

### 3b: plan_receiver template (noblefactor-ops)

The `plan_receiver` template currently emits node creation with:
`Operation: "link"`. Update to emit `Action: "file.link"`.

### 3c: devlore-cli template files

Update the two `.go.template` files in
`star/extensions/com.noblefactor.devlore.Actions/templates/`:

| File | Change |
|---|---|
| `graph_actions.go.template` | Action Do/Undo, dotted names, Register func, provider import |
| `plan_receiver.go.template` | `Action:` field, dotted names |

**Branch**: `feat/template-update`

### 3d: Regenerate and verify

Run `star gen.receiver` against each provider to verify the templates
produce output matching the hand-written `_gen.go` files from Phase 2.

## Phase 4: Architecture Documentation

Update architecture docs to reflect the new model.

**Repo**: devlore-cli

| Document | Change |
|---|---|
| `devlore-operation-namespaces.md` | Retitle to "Action Namespaces". Update all Operation → Action references. Update package paths to `provider/*`. |
| `devlore-execution-graph.md` | Update Operation → Action, OperationRegistry → ActionRegistry, Node.Operation → Node.Action. |
| `devlore-graph-convergence-operations.md` | Update code snippets to use Action interface. Note Choose/Gather/Elevate implementations are live in `execution/flow`. |
| `devlore-typed-slots.md` | Update any Operation references to Action. |
| `devlore-phase-execution.md` | Update Operation → Action in saga/phase references. |

**Repo**: noblefactor-ops

| Document | Change |
|---|---|
| `star-extensions.md` | Update gen-receiver template descriptions for Action model. |

## Knowledge Extraction

`star devlore knowledge extract --domain actions` currently extracts Go method
signatures for Service structs via `go.mapping()`. The provider model changes
what gets extracted and broadens the scope — the knowledge surface must include
everything an LLM needs to generate a valid execution graph.

### Public-facing interface (lore package API)

What a lore package exposes to plan authors and users:

| Knowledge | Source | Description |
|---|---|---|
| Package context | Plan receivers | Available packages, metadata, slot types |
| System context | Platform receivers | OS, distro, arch, available package managers |
| Planning APIs | Plan receivers | Node creation, edge declaration, dependency wiring |
| Output functions | `starlark/output.go` | User messaging: status, progress, warnings, prompts |

### Internals (for LLM graph generation)

What an LLM needs to construct a correct execution graph:

| Knowledge | Source | Description |
|---|---|---|
| Provider actions | `provider/*/actions_gen.go` | Action catalog: name, slots, Do/Undo signatures |
| Flow actions | `flow/*.go` | Choose (OR-selector), Gather (AND-join), Elevate (privilege) |
| RetryPolicy | `execution/phase.go` | Retry configuration: max attempts, backoff, delays |
| Node structure | `execution/graph.go` | Node fields: Action, Retry, slots, edges, mode |
| Slot types | `execution/graph.go` | Typed slot resolution chain |
| Edge semantics | `execution/graph.go` | depends_on, orders, result flow |

### Impact on `knowledge extract`

- `--domain actions` targets `provider/*/provider.go` instead of flat `*_service.go`
- Add `--domain flow` for flow action signatures
- Add `--domain plan` for plan receiver APIs and output functions
- The extracted knowledge feeds LLM context for plan generation

## Naming Decisions

### shell.Shell → shell.Exec

The POSIX shell action is renamed from `Shell` to `Exec` to avoid the
self-referential `shell.Shell`. The action name becomes `"shell.exec"`.
PowerShell stays `shell.PowerShell` → `"shell.powershell"`.

### package → pkg

Go reserves `package` as a keyword. The provider package is `pkg`:
`pkg.Provider`, `pkg.Install`, `pkg.Upgrade`, `pkg.Remove`, `pkg.Update`.

### service (not servicemanager)

`ServiceManagerService` becomes `service.Provider`. The package name
`service` is sufficient context. Action names: `service.Start`,
`service.Stop`, `service.Restart`, `service.Enable`, `service.Disable`.

## Import Graph

```
writ/commands.go ──┬── execution (Action, ActionRegistry, Context, Node, Graph)
                   ├── execution/engine
                   ├── execution/build
                   ├── execution/flow
                   ├── execution/provider/file
                   ├── execution/provider/encryption
                   ├── execution/provider/pkg
                   ├── execution/provider/shell
                   └── execution/provider/service

execution/engine        ──── execution (Action, ActionRegistry, Context, Node, Graph)
execution/build         ──── execution (Node, Graph)
execution/flow          ──── execution (Action, Context, Node)
execution/provider/file ──── execution (Action, Context, Node)
execution/provider/pkg  ──── execution (Action, Context, Node)
                              host (PackageManager)
execution                     (no imports of engine/, build/, flow/, or provider/*)
```

No cycles. All subpackages import `execution` for core types.
`execution` never imports subpackages. Call sites wire everything.

## Action Count

Before: 21 registered (8 file + 1 encryption + 4 package + 2 shell + 5 service + 1 validate — deleted)
After: 23 registered (8 file + 1 encryption + 4 pkg + 2 shell + 5 service + 3 flow)

## Verification

```bash
# After Phase 1
go build ./...
go test ./internal/execution/... -count=1
go test ./internal/starlark/ -count=1
go test ./internal/writ/ -count=1
go test ./internal/lore/ -count=1
go vet ./...

# After Phase 2
go build ./...
go test ./internal/execution/... -count=1
go test ./internal/starlark/ -count=1
go test ./internal/writ/ -count=1
go test ./internal/lore/ -count=1
go vet ./...

# After Phase 3 — regenerate and diff
star gen.receiver --path internal/execution/provider/file --struct Provider
diff provider/file/actions_gen.go provider/file/actions_gen.go.new
```

## Open Questions

- [ ] Should `Undo()` methods and `Do()` rollback state be populated for
  file actions now (e.g., `file.Link.Do` returns the symlink path,
  `file.Link.Undo` removes it), or deferred until the saga controller
  integration? The interface is ready — the question is when to populate.
- [ ] Platform-specific plan receivers (`platform/darwin.go`,
  `platform/linux.go`, `platform/windows.go`) emit names like
  `launchd-start`, `systemd-start`. Do these converge to `service.start`
  now, or do they remain as platform-specific action names requiring
  platform-specific provider variants?
- [ ] Do the `source`, `literal`, `download`, `delegate` pseudo-operations
  become actions (with no provider), or are they handled differently?
  Currently `source` and `literal` are node types that don't have actions
  in the registry — the executor skips them.
- [ ] The `archive-extract`, `git-clone`, `git-checkout`, `git-pull`
  operations (currently in plan receivers) need providers. Should they
  become `archive.Provider` and `git.Provider` in this plan, or deferred?

## Related Documents

- [Convergence Operations Architecture](../architecture/devlore-graph-convergence-operations.md) — Defines Choose, Gather, Elevate
- [Execution Graph Architecture](../architecture/devlore-execution-graph.md) — Core graph state machine
- [Phase Execution Architecture](../architecture/devlore-phase-execution.md) — Saga pattern, phases, retry/rollback
- [Typed Slots Architecture](../architecture/devlore-typed-slots.md) — Slot resolution chain
- [Operation Namespaces](../architecture/devlore-operation-namespaces.md) — Current namespace guide (to be updated)
- [Phase 6: Typed Slots and Full Generation](./star-gen-receiver/phase-6.md) — Prior art for service extraction + _gen.go pattern
- [Star Extensions](../../noblefactor-ops/docs/architecture/star-extensions.md) — Extension system and gen-receiver
