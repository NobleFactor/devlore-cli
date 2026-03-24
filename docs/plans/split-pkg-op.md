---
title: "Split pkg/op into three modules: provider, starlark, workflow"
issue: TBD
status: draft
created: 2026-03-23
updated: 2026-03-23
---

# Plan: Split pkg/op into three modules

## Summary

Split `pkg/op` into three independent Go **modules**, each with its own
`go.mod`. The module boundary enforces the dependency DAG at the toolchain
level — not just by convention, but by `go mod tidy` rejecting cycles.

```
pkg/workflow/go.mod
  require pkg/starlark
  require pkg/provider

pkg/starlark/go.mod
  require pkg/provider

pkg/provider/go.mod
  (no internal requires)
```

A `go.work` workspace file at the repo root links all three modules plus the
root module during development.

## Goals

1. **Module-level isolation** — each layer is an independently versioned Go module
2. **Provider module has zero starlark dependency** — pure Go contracts and implementations
3. **Starlark module is a focused Go↔Starlark bridge** — no graph knowledge
4. **Workflow module is the orchestrator** — graph model, planning, execution
5. **External consumers can depend on `pkg/provider` alone** without pulling starlark
6. **Generated receiver code separates cleanly** from provider implementations

## Non-Goals

- Publishing the sub-modules to a separate repository
- Changing type names beyond what the module split requires
- Refactoring `internal/execution/` or `internal/writ/` beyond import path updates

## Why Modules, Not Packages

Packages within a single module share a `go.mod`. Any package can import any
other package in the same module — the compiler only rejects import cycles, not
"wrong direction" imports. A careless edit can break the layering silently.

Separate modules make the layering **structural**:

| Property | Single module (packages) | Multi-module |
| --- | --- | --- |
| Circular dependency | Compiler rejects import cycles | `go mod tidy` rejects dependency cycles |
| Wrong-direction import | Nothing prevents it | Cannot import a module you don't `require` |
| Starlark leakage into provider | Possible via transitive import | Impossible — provider module has no starlark `require` |
| External consumption | Must pull entire module | Can `go get` just `pkg/provider` |
| Versioning | All code versions together | Each module versioned independently |
| Release friction | None | Must tag and release sub-modules in dependency order |

## Dependency Model

```
devlore-cli (root module)
  ├── require pkg/workflow
  ├── require pkg/starlark
  └── require pkg/provider

pkg/workflow
  ├── require pkg/starlark
  ├── require pkg/provider
  └── require go.starlark.net/starlark

pkg/starlark
  ├── require pkg/provider
  └── require go.starlark.net/starlark

pkg/provider
  └── (no internal requires — leaf module)
```

## Key Design Decisions

### 1. Action.Do takes *ContextBase (provider module)

`Action` is defined in the provider module. Its `Do` method cannot reference
types from starlark or workflow without creating a module-level cycle.

```go
// pkg/provider
type ContextBase struct {
    context.Context
    Root        Root
    ProgramName string
    DryRun      bool
    Platform    *Platform
    Writer      io.Writer
    SopsClient  *sops.Client
    Data        map[string]any    // escape hatch for cross-module data
}

type Action interface {
    Name() string
    Params() []ParamInfo
    Do(ctx *ContextBase, slots map[string]any) (Result, Complement, error)
}
```

The workflow module defines `Context` extending `ContextBase` with Graph,
Thread, Results, etc. The executor stores `*workflow.Context` in
`ContextBase.Data["workflow.context"]` before dispatching. Flow actions
retrieve it:

```go
// pkg/workflow/flow
func (a *Choose) Do(ctx *provider.ContextBase, slots map[string]any) (...) {
    wfCtx := ctx.Data["workflow.context"].(*workflow.Context)
    graph := wfCtx.Graph
    // ...
}
```

Regular provider actions never touch this key — they use ContextBase directly.

### 2. Concrete providers split: implementation vs. receiver

A concrete provider (e.g., file) has two parts:

- **Implementation** (`provider.go`, `resource.go`) — pure Go, depends only on
  `pkg/provider`. Lives in the provider module.
- **Receiver glue** (`receiver.gen.go`) — generated code that bridges the
  provider into starlark. Depends on `pkg/starlark` and `pkg/workflow`. Lives
  in the workflow module.

```
pkg/provider/file/          # In provider module
  provider.go               # Provider struct + methods
  resource.go               # file.Resource, file.Tombstone

pkg/workflow/receiver/file/ # In workflow module
  receiver.gen.go           # Generated: ReceiverFactory, starlark bindings
```

This is the only way to keep the provider module starlark-free. The generated
code imports the provider implementation cross-module.

### 3. ResourceBase.MarshalStarvalue moves to starlark module

`ResourceBase.MarshalStarvalue()` imports starlark. This method cannot exist
in the provider module. It moves to the starlark module as a standalone
function that the marshaler dispatches to for ResourceBase types.

### 4. RecoverySite takes Root, not Context

`RecoverySite` currently stores `Context` but only uses `ctx.Root`. It changes
to take `Root` directly, eliminating its dependency on Context entirely.

### 5. RecoveryStack.PushAction takes *ContextBase

The `Undo` closure captures the ContextBase at push time. Flow actions that
need the full workflow Context retrieve it from Data inside their Undo
implementation.

### 6. Flow actions live in workflow/flow

Flow actions (choose, gather, elevate, etc.) are graph-construction primitives.
They need `*workflow.Context` (via the Data escape hatch). They implement
`provider.Action` but live in the workflow module.

### 7. op.Gather moves to flow

The plan-time `op.Gather` starlark value type moves into `workflow/flow`
alongside the other flow planning bindings in `flow.Plan`. It becomes a
method on `flow.Plan` (like complete, degraded, fatal) rather than a
standalone type. `Output` stays in the workflow root — it's universal.

### 8. Module naming: pkg/starlark

The user chose `starlark` over `star`. Import aliasing handles the collision
with `go.starlark.net/starlark`:

```go
import (
    starlarknet "go.starlark.net/starlark"
    "github.com/NobleFactor/devlore-cli/pkg/starlark"
)
```

This is mildly annoying but conventional in Go. The alternative (`pkg/star`)
avoids it if preferred.

## Target Structure

```
go.work                                 # Workspace linking all modules
go.mod                                  # Root module (devlore-cli)

pkg/
├── provider/                           # MODULE: github.com/.../pkg/provider
│   ├── go.mod                          # No starlark dependency
│   ├── action.go                       # Action, CompensableAction, FallibleAction
│   ├── context.go                      # ContextBase
│   ├── fatal.go                        # FatalError
│   ├── nodeid.go                       # GenerateNodeID
│   ├── phase.go                        # Phase, RetryPolicy, RollbackEntry
│   ├── platform.go                     # Platform
│   ├── platform_darwin.go
│   ├── platform_linux.go
│   ├── platform_windows.go
│   ├── platform_helpers.go
│   ├── platform_new.go
│   ├── recovery.go                     # RecoveryStack
│   ├── recovery_site.go                # RecoverySite (takes Root)
│   ├── registry.go                     # ActionRegistry
│   ├── render.go                       # RenderError
│   ├── resource.go                     # Resource, ResourceBase, Tombstone, NoResult
│   ├── resource_catalog.go             # ResourceCatalog
│   ├── resource_announce.go            # ResourceFactory, AnnounceResource
│   ├── root.go                         # Root, Path, confinedRoot
│   ├── sops/                           # SOPS client
│   │   └── client.go
│   ├── file/                           # Concrete: file operations
│   │   ├── provider.go
│   │   ├── resource.go
│   │   └── gitignore/
│   │       └── tracker.go
│   ├── pkg/                            # Concrete: package management
│   │   ├── provider.go
│   │   └── resource.go
│   ├── shell/                          # Concrete: shell execution
│   │   └── provider.go
│   ├── service/                        # Concrete: service management
│   │   ├── provider.go
│   │   └── resource.go
│   ├── appnet/                         # Concrete: network downloads
│   │   ├── provider.go
│   │   └── resource.go
│   ├── archive/                        # Concrete: archive extraction
│   │   ├── provider.go
│   │   └── resource.go
│   ├── encryption/                     # Concrete: SOPS decryption
│   │   ├── provider.go
│   │   └── resource.go
│   ├── git/                            # Concrete: git operations
│   │   ├── provider.go
│   │   └── resource.go
│   ├── json/                           # Concrete: JSON encode/decode
│   │   ├── provider.go
│   │   └── resource.go
│   ├── yaml/                           # Concrete: YAML encode/decode
│   │   ├── provider.go
│   │   └── resource.go
│   ├── regexp/                         # Concrete: regexp operations
│   │   └── provider.go
│   ├── template/                       # Concrete: template rendering
│   │   └── provider.go
│   ├── platform/                       # Concrete: platform metadata
│   │   └── provider.go
│   ├── ui/                             # Concrete: terminal output
│   │   └── provider.go
│   └── mem/                            # Concrete: in-memory resources
│       ├── callable.go                 # NOTE: imports starlark — see Risks
│       ├── extract.go
│       ├── literals.go
│       └── resource.go
│
├── starlark/                           # MODULE: github.com/.../pkg/starlark
│   ├── go.mod                          # requires pkg/provider
│   ├── marshal.go                      # Marshal, Unmarshal, marshalReflect
│   ├── unmarshal.go                    # unmarshalValue, unmarshal
│   ├── struct.go                       # StructValue, typeInfo, getTypeInfo
│   ├── callable.go                     # CallableResource, CallableInput
│   ├── constructor.go                  # constructorRegistry, RegisterConstructor
│   ├── receiver_base.go               # receiver struct, builtinFunc, NoSuchAttrError
│   ├── classify.go                     # classifyReturn and variants
│   ├── naming.go                       # camelToSnake
│   ├── resource_marshal.go            # MarshalResourceBase (from ResourceBase.MarshalStarvalue)
│   └── starvalue/                      # Marshaler interface
│       └── starvalue.go
│
└── workflow/                           # MODULE: github.com/.../pkg/workflow
    ├── go.mod                          # requires pkg/provider, pkg/starlark
    ├── graph.go                        # Graph, Node, Edge, SlotValue, HydrateGraph
    ├── context.go                      # Context (extends ContextBase)
    ├── output.go                       # Output, FillSlot
    ├── receiver_factory.go             # ReceiverFactory interfaces
    ├── announce.go                     # AnnounceReceiver, InitAll
    ├── action_reflect.go               # RegisterActions, reflected action types
    ├── receiver_reflect.go             # ExecutingReceiver
    ├── planned_reflect.go              # PlanningReceiver
    ├── binding_config.go               # BindingConfig
    ├── runtime.go                      # StarlarkRuntime
    ├── flow/                           # Flow actions + plan-time bindings
    │   ├── choose.go
    │   ├── gather.go                   # flow.Gather action + plan-time gather
    │   ├── complete.go
    │   ├── degraded.go
    │   ├── elevate.go
    │   ├── fatal.go
    │   ├── planned.go                  # plan.flow namespace (all flow plan bindings)
    │   ├── provider.go
    │   └── wait_until.go
    └── receiver/                       # Generated receiver glue per provider
        ├── appnet/receiver.gen.go
        ├── archive/receiver.gen.go
        ├── encryption/receiver.gen.go
        ├── file/receiver.gen.go
        ├── git/receiver.gen.go
        ├── json/receiver.gen.go
        ├── mem/receiver.gen.go
        ├── pkg/receiver.gen.go
        ├── platform/receiver.gen.go
        ├── regexp/receiver.gen.go
        ├── service/receiver.gen.go
        ├── shell/receiver.gen.go
        ├── template/receiver.gen.go
        ├── ui/receiver.gen.go
        └── yaml/receiver.gen.go
```

## Workspace Configuration

```go
// go.work
go 1.24

use (
    .
    ./pkg/provider
    ./pkg/starlark
    ./pkg/workflow
)
```

During development, `go.work` resolves all cross-module imports locally.
For releases, each module is tagged in dependency order:
`pkg/provider/v0.x.0` → `pkg/starlark/v0.x.0` → `pkg/workflow/v0.x.0`.

## Phases

### Phase 1: Create pkg/provider module

Create `pkg/provider/go.mod`. Move all starlark-free types:

- Action interfaces, ActionRegistry, ParamInfo, Result, Complement
- ContextBase (extracted from context.go — no Thread, no Graph)
- Platform and all platform_*.go
- Root, Path, and all Root implementations
- Resource, ResourceBase (strip MarshalStarvalue), Tombstone, TombstoneBase, NoResult
- ResourceCatalog, extractResource
- RecoveryStack (PushAction takes *ContextBase), RecoverySite (takes Root)
- Phase, RetryPolicy, BackoffStrategy, Attempt, RollbackEntry
- FatalError, GenerateNodeID, RenderError
- ResourceFactory, AnnounceResource, ensureResourceInit
- sops/ subpackage

Move concrete provider implementations (provider.go, resource.go only — not
generated receiver code):
- file/, pkg/, shell/, service/, appnet/, archive/, encryption/, git/, json/,
  yaml/, regexp/, template/, platform/, ui/

**mem/ is the exception.** It imports `go.starlark.net/starlark` for
`Callable.Fn()` and `starlark.Thread`. Options:
  a. mem/ stays in provider module, provider module gains a starlark dependency (defeats purpose)
  b. mem/ moves to the starlark module (it's a bridge concern)
  c. mem/ splits: resource.go in provider, callable.go in starlark

Option (c) is cleanest. `mem.Resource` (a data type) lives in provider.
`mem.Callable` (starlark runtime state) lives in starlark.

**Verification:** `cd pkg/provider && go build ./...` succeeds with zero
`go.starlark.net` in `go.sum`.

### Phase 2: Create pkg/starlark module

Create `pkg/starlark/go.mod` requiring `pkg/provider`. Move:

- Marshal, Unmarshal, marshalReflect, unmarshalValue
- StructValue, typeInfo, discoverMethods, getTypeInfo, typeCache
- CallableResource interface, CallableInput, extractCallable, buildCallableFunc
- constructorRegistry, RegisterConstructor, RegisterTypeParams
- receiver base type, builtinFunc, NoSuchAttrError
- classifyReturn, classifyFallibleReturn, classifyCompensableReturn
- camelToSnake and naming utilities
- MarshalResourceBase (from ResourceBase.MarshalStarvalue)
- mem.Callable (the starlark-dependent half of mem)

**Verification:** `cd pkg/starlark && go build ./...` succeeds. Only
`require`s are `pkg/provider` and `go.starlark.net`.

### Phase 3: Create pkg/workflow module

Create `pkg/workflow/go.mod` requiring `pkg/provider` and `pkg/starlark`. Move:

- Graph, Node, Edge, SlotValue, GraphState, NodeStatus, GraphContext, Summary, Collision
- Context (extends ContextBase with Thread, Graph, Results, RecoverySite, Catalog)
- Output, FillSlot
- ReceiverFactory, PlanningReceiverFactory, ExecutingReceiverFactory
- AnnounceReceiver, InitAll, Receivers
- ProviderBase, ContextProvider
- BindingConfig, StarlarkRuntime
- RegisterActions, reflected action types (reflectedPureAction, etc.)
- ExecutingReceiver, WrapProviderInExecutingReceiver, buildMethodBridge
- PlanningReceiver, WrapProviderInPlanningReceiver, buildPlannedBridge
- HydrateGraph, ComputeSummary, Serialize, CanonicalContent
- Flow actions + flow plan bindings (workflow/flow/)
- Generated receiver glue (workflow/receiver/*)

Move op.Gather into workflow/flow alongside the other flow plan bindings.

**Verification:** `cd pkg/workflow && go build ./...` succeeds. `require`s
are `pkg/provider`, `pkg/starlark`, and `go.starlark.net`.

### Phase 4: Update code generator

Update the code generator that produces `receiver.gen.go` files:
- Output now targets `pkg/workflow/receiver/<provider>/`
- Import paths reference `pkg/provider/<provider>` for the implementation
  and `pkg/starlark` + `pkg/workflow` for the binding framework
- Regenerate all receiver files

**Verification:** `go generate ./...` produces identical output to phase 3 artifacts.

### Phase 5: Create go.work and update root module

Create `go.work` at repo root linking all four modules. Update the root
module's `go.mod` to require the three sub-modules. Update all consumers
in `internal/` and `cmd/` to use new import paths.

**Verification:** `make check` passes from repo root.

### Phase 6: Remove pkg/op

Delete `pkg/op/`. Verify no lingering imports across all modules.

**Verification:** `go work sync && go build ./...` from repo root.

## Risks

| Risk | Mitigation |
| --- | --- |
| mem/ imports starlark | Split mem: Resource in provider, Callable in starlark |
| Release ordering | Tag in dependency order: provider → starlark → workflow |
| go.work vs CI | CI uses `go.work` during build; releases use published versions |
| Receiver gen.go in different module than provider | Code generator updated in phase 4; verified by regeneration |
| ContextBase.Data escape hatch is stringly-typed | Define well-known keys as constants in workflow module; flow actions use typed accessor |
| Import alias noise from `pkg/starlark` name | Every file importing both uses `starlarknet` alias — consider `pkg/star` if friction is high |

## Open Questions

1. **Module path format:** `github.com/NobleFactor/devlore-cli/pkg/provider`
   or shorter? Go convention allows nested module paths but they can be verbose.

2. **Naming: `pkg/starlark` vs `pkg/star`:** The former matches the user's
   stated preference but causes import aliasing with `go.starlark.net/starlark`.
   The latter avoids it. Decision needed before implementation.

3. **mem.Callable split:** Is splitting mem into two packages (provider half +
   starlark half) acceptable, or should the entire mem package move to starlark?
