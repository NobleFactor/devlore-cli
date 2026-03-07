# Plan: Resource-Provider Model

```yaml
title: Resource-Provider Model
issue: TBD
status: in-progress
created: 2026-02-14
updated: 2026-02-16
```

## Summary

Restructure `internal/execution` from a flat Service/Operation model into a
Resource-Provider architecture optimized for the Saga Pattern. Services become
Providers in domain subpackages (`provider/file`, `provider/pkg`, etc.).
Operations become Actions with `Do`/`Undo` methods that receive resolved slots,
not nodes. The three flow primitives from the
[convergence operations architecture](../architecture/2.3-orchestration-primitives.md)
live in `execution/flow` as typed Actions: `flow.Choose` (OR-selector),
`flow.Gather` (AND-join), and `flow.Elevate` (privilege transition).

## Goals

1. **Action interface with Do/Undo.** Every action implements forward (Do) and
   compensating (Undo) methods. Actions receive resolved slots as
   `map[string]any` — they never touch `*Node`.
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

## Current State (after Phase 3)

| Component | State |
|---|---|
| Action interface | `Name()`, `Do(ctx, slots map[string]any) (Result, UndoState, error)`, `Undo(ctx, slots, state) error` |
| Node.Action | `Action` interface field (`json:"-" yaml:"-"`), serialized via custom marshal |
| Node.ResolvedSlots | `ResolvedSlots(results map[string]any) map[string]any` — resolves promise slots from upstream results |
| Providers (11) | `provider/file`, `provider/encryption`, `provider/template`, `provider/content`, `provider/pkg`, `provider/shell`, `provider/service`, `provider/net`, `provider/archive`, `provider/git`, `provider/register_gen.go` |
| Actions (32) | 9 file + 1 encryption + 1 template + 1 content + 4 pkg + 2 shell + 5 service + 1 net + 1 archive + 3 git + 4 flow |
| Op names | Dotted: `"file.link"`, `"file.copy"`, `"pkg.install"`, etc. |
| ActionRegistry | `NewActionRegistry()`, `Register()`, `Get()`, `MustGet()`, `Names()` |
| GraphExecutor | No registry field — actions live on nodes, executor calls `node.Action.Do(ctx, slots)` directly |
| RecoveryStack | `RecoveryEntry{Node, UndoState}` — Unwind calls `node.Action.Undo(ctx, node.ResolvedSlots(results), state)` |
| Content pipeline | Deleted — content flows via Result + promise slots |
| Node.Mode | Deleted — mode is a slot (`SetSlotImmediate("mode", os.FileMode(0755))`) |
| Delegate/manifest | Deleted — no delegate nodes, no manifest.Resolve action, no SubgraphBuilder |
| Planner | `lore.Planner` — `PlanPackages(graph, manifestPath)`, `PlanByName(graph, packages)` |
| Writ → Lore wiring | `DeployGraphBuilder.Planner` field; BuildTree returns manifest paths |
| Code gen templates | `graph_actions` + `planned_receiver` emit Do/Undo/Register, `slots map[string]any`, `Impl *Provider`, `p.reg.MustGet()` |

## Vernacular

| Term | Definition | Replaces |
|---|---|---|
| **Action** | Unit of work with Do (forward) and Undo (compensate) | Operation |
| **Provider** | Stateless resource driver; methods are the source of truth for `star gen.receiver` | Service |
| **Choose** | OR-selector: evaluates alternatives, selects one (`flow.Choose`) | — |
| **Gather** | AND-join: waits for all predecessors (`flow.Gather`) | Starlark-only construct |
| **Elevate** | Privilege transition: acquire/release elevated execution context (`flow.Elevate`) | — |

## Package Layout (actual)

```
internal/execution/
  action.go              — Action interface, Result, UndoState, Context
  graph.go               — Graph, Node (Action interface field), Edge, SlotValue,
                           ResolvedSlots, ActionName, StubAction, custom JSON/YAML marshal
  registry.go            — ActionRegistry (Get, MustGet, Register, Names)
  executor.go            — GraphExecutor (dispatches via node.Action.Do with resolved slots)
  phase.go               — Phase execution (saga pattern), RetryPolicy
  recovery.go            — RecoveryStack (rollback via node.Action.Undo with resolved slots)
  builder.go             — GraphBuilder interface
  preflight.go           — Preflight conflict detection
  plan.go                — Plan types
  stateview.go           — StateView
  dependencyview.go      — DependencyView

  flow/
    choose.go            — flow.Choose (predicate branching)
    gather.go            — flow.Gather (parallel comprehension)
    elevate.go           — flow.Elevate (privilege transition)
    wait_until.go        — flow.WaitUntil (poll-based synchronization)
    register.go          — Register(reg)

  provider/
    register_gen.go      — RegisterAll(reg) — calls all provider Register functions
    file/
      provider.go        — file.Provider (from FileService)
      actions_gen.go     — file.Link, file.Copy, file.Backup, file.Unlink,
                           file.Remove, file.Write, file.Move, file.Mkdir, file.Source
      helpers.go         — pruneParents, isSubpath
    encryption/
      provider.go        — encryption.Provider
      actions_gen.go     — encryption.Decrypt
    template/
      provider.go        — template.Provider
      actions_gen.go     — template.Render
    content/
      provider.go        — content.Provider
      actions_gen.go     — content.Literal
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
    net/
      provider.go        — net.Provider
      actions_gen.go     — net.Download
    archive/
      provider.go        — archive.Provider
      actions_gen.go     — archive.Extract
    git/
      provider.go        — git.Provider
      actions_gen.go     — git.Clone, git.Checkout, git.Pull
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

## Action Contract

```go
type Result = any
type UndoState = any

type Action interface {
    Name() string
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
    Undo(ctx *Context, slots map[string]any, state UndoState) error
}
```

Actions receive resolved slots — they never touch `*Node`. The executor resolves
all promise slots before calling Do. Promise slots reference upstream node IDs;
the executor substitutes the stored Result from that node. Immediate slots pass
through unchanged.

### Data flow

- **Edges** are forward references (producer → consumer) for topological sort.
- **Promise slots** are backward references (consumer → producer) for data resolution.
- **Result** (`any`) flows from Do along an edge to a downstream slot where it is
  resolved by name. The executor stores each Result keyed by node ID in a
  transient `map[string]any` that lives only for the duration of execution.

### Node.ResolvedSlots

```go
func (n *Node) ResolvedSlots(results map[string]any) map[string]any
```

Walks `n.Slots`, substitutes promise refs from the results map, returns a flat
`map[string]any`. Called by the executor before `Do` and by the recovery stack
before `Undo`.

## Flow Actions

The `execution/flow` package contains three graph flow primitives — actions that
change how the executor traverses the graph rather than operating on resources.
Each implements the Action interface. All live in `internal/execution/flow/`.

See: [Convergence Operations Architecture](../architecture/2.3-orchestration-primitives.md)

### flow.Choose

OR-selector: evaluates alternatives and selects one. Multiple predecessors
represent options, not dependencies. The node picks based on criteria —
platform, availability, preference, or runtime condition. Only the selected
predecessor is executed; unchosen branches are skipped.

### flow.Gather

AND-join: waits for all predecessors to complete before proceeding. Equivalent
to `Promise.all()` — every input must succeed for the gather node to succeed.

### flow.Elevate

Privilege transition: makes the boundary between unprivileged and privileged
execution visible as an explicit graph node. Stub implementation; full
sudo/privilege integration is a separate plan.

### Registration

```go
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
2. **Planned receivers** (`planned_*_gen.go`) — via the `planned_receiver` template

The templates live in the devlore-cli Actions extension:
`star/extensions/com.noblefactor.devlore.Actions/templates/`

| Template | Generates | Updated in |
|---|---|---|
| `graph_actions.go.template` | Action structs (Do/Undo, Register) | Phase 3 |
| `planned_receiver.go.template` | Plan namespace structs (node creation) | Phase 3 |

## Completed Phases

### Phase 1: Interface Rename (PR #128)

Mechanical rename: Operation → Action, Execute → Do, add Undo stub.
`Node.Action` was still a string in this phase. Do/Undo still took `*Node`.

### Phase 2A: Provider Extraction (PR #129)

Extracted services into `provider/*` subpackages. Created `provider/file`,
`provider/encryption`, `provider/pkg`, `provider/shell`, `provider/service`.
`provider.RegisterAll(reg)` wires all providers.

### Phase 2B: Additional Providers (PR #130)

Created `provider/template`, `provider/content`, `provider/net`,
`provider/archive`, `provider/git`. Deleted manifest provider.

### Phase 2C: Typed Action Contract (current branch)

- Changed Action contract: `Do(ctx, slots map[string]any)` / `Undo(ctx, slots, state)`
- `Node.Action` became an interface (not string), with custom JSON/YAML marshal
- Added `Node.ResolvedSlots(results)` for slot resolution
- Added `ActionRegistry.MustGet(name)` for builder-time lookup
- Removed `registry` from `GraphExecutor` — no runtime dispatch
- Deleted content pipeline (`ContentFor`, `StoreContent`, `Edges`, `Outputs`)
- Deleted `Node.Mode` — mode is now a slot
- Threaded `ActionRegistry` through all builders (starlark, platform, writ, lore)
- Completed dotted action names across all builders and comparisons
- Deleted delegate concept entirely (no `flow.delegate`, no `nodeIsDelegate`)
- Deleted `SubgraphBuilder`, `ExpandDelegates`, `isDelegateNode`
- Added `StubAction(name)` export for test packages
- `Build()` auto-defaults `ActionRegistry` via `provider.RegisterAll`

See: [Phase 2C plan](./resource-provider/phase-2c.md)

## Remaining Phases

### Phase 2D: Planner.PlanPackages (current branch)

Added `lore.Planner` struct with `PlanPackages(graph, manifestPath)` and
`PlanByName(graph, packages)`. Wired into writ deploy: `BuildTree` returns
manifest paths, `DeployGraphBuilder.Planner` calls `PlanPackages` for each.
Removed `manifest.resolve` from execution runtime (stateview, graph summary).
Kept as tree detection sentinel. `Build()` refactored to use Planner internally.

See: [Phase 2D plan](./resource-provider/phase-2d.md)

### Phase 3: Generator Templates (current branch)

Updated `star gen.receiver` templates and noblefactor-ops helper functions so
`_gen.go` files become regenerable. Template helpers emit Do/Undo/Register
pattern with `slots map[string]any`, three-value returns, `Impl *Provider`.
Planned receiver template threads `reg *execution.ActionRegistry` and uses
`p.reg.MustGet()`. Renamed `graph_ops` → `graph_actions` throughout.

**Repos**: noblefactor-ops (template helpers in `receiver_go_gen.go`),
devlore-cli (template files in `com.noblefactor.devlore.Actions`)

See: [Phase 3 plan](./resource-provider/phase-3.md)

### Phase 4: Architecture Documentation (current branch)

Updated architecture docs to reflect the Action model. All five architecture
docs updated: Operation→Action, Service→Provider, Execute→Do/Undo,
ContentFor/StoreContent→Result+promise slots. Cross-references updated in
index.md, receipt-integrity.md, and this plan. Code doc comment in graph.go
fixed.

See: [Phase 4 plan](./resource-provider/phase-4.md)

### Phase 5: Comprehensive Action Testing (current branch)

Added `Graph.Hydrate(reg)` method and 52 new tests covering all 34 actions
(Do() tests for every provider and flow action), graph serialization round-trips
(YAML and JSON), and full lifecycle tests (build → serialize → deserialize →
hydrate → run). External-tool actions tested via dry-run; file actions via real
filesystem; net.download via httptest; archive.extract via real tar.gz/zip.

See: [Phase 5 plan](./resource-provider/phase-5.md)

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
                   ├── execution/flow
                   ├── execution/provider/file
                   ├── execution/provider/encryption
                   ├── execution/provider/template
                   ├── execution/provider/content
                   ├── execution/provider/pkg
                   ├── execution/provider/shell
                   ├── execution/provider/service
                   ├── execution/provider/net
                   ├── execution/provider/archive
                   └── execution/provider/git

execution/flow          ──── execution (Action, Context)
execution/provider/file ──── execution (Action, Context)
execution/provider/pkg  ──── execution (Action, Context)
                              host (PackageManager)
execution                     (no imports of flow/ or provider/*)
```

No cycles. All subpackages import `execution` for core types.
`execution` never imports subpackages. Call sites wire everything.

## Action Count

32 registered:
- 9 file (link, copy, backup, unlink, remove, write, move, mkdir, source)
- 1 encryption (decrypt)
- 1 template (render)
- 1 content (literal)
- 4 pkg (install, upgrade, remove, update)
- 2 shell (exec, powershell)
- 5 service (start, stop, restart, enable, disable)
- 1 net (download)
- 1 archive (extract)
- 3 git (clone, checkout, pull)
- 4 flow (choose, gather, elevate, wait_until)

## Open Questions

### Resolved

- **Delegate pseudo-operation**: Eliminated. No delegate nodes, no delegate
  action. Manifest reading is a build-time concern handled by `Planner.PlanSequence`.
- **Source/Literal**: `file.Source` and `content.Literal` are real actions that
  return content as Result. Downstream nodes receive content via promise slots.
- **Archive/Git providers**: Created in Phase 2B. `archive.Provider` (`archive.Extract`)
  and `git.Provider` (`git.Clone`, `git.Checkout`, `git.Pull`).
- **engine/build subpackages**: Not created. Executor, phase, and recovery stay
  flat in the `execution` package. Builder and preflight stay flat. The planned
  `engine/` and `build/` moves are dropped — the flat layout works.
- **Undo methods**: Populated as part of this plan (Goal 1: "every action
  implements Do and Undo").
- **Platform-specific action names**: Resolved — all platforms already use
  `service.*` action names. Node ID prefixes updated from `launchd`/`systemd`/
  `winservice` to `service` for consistency.

## Related Documents

- [Convergence Operations Architecture](../architecture/2.3-orchestration-primitives.md) — Defines Choose, Gather, Elevate
- [Execution Graph Architecture](../architecture/2-execution-graph.md) — Core graph state machine
- [Phase Execution Architecture](../architecture/2.2-phase-execution.md) — Saga pattern, phases, retry/rollback
- [Typed Slots Architecture](../architecture/2.1-typed-slots.md) — Slot resolution chain
- [Action Namespaces](../architecture/3-operation-namespaces.md) — Action namespace guide
- [Orchestration Primitives Architecture](../architecture/2.3-orchestration-primitives.md) — Gather, Choose, WaitUntil, SlotProxy, hooks
- [Orchestration Primitives Plan](orchestration-primitives.md) — Implementation plan for orchestration primitives
