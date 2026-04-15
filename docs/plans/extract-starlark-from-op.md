---
title: "Extract starlark infrastructure from pkg/op into pkg/op/bind"
issue: 264
status: in-progress
created: 2026-03-24
updated: 2026-04-14 (node/subgraph interchangeability principle added to Phase 10 invariant; Phase 11 reframed as multi-node bundler; Phase 12 expanded with known flow-provider defects)
---

# Plan: Extract Starlark Infrastructure from pkg/op

## Summary

Split `pkg/op` into a starlark-free core (`pkg/op`) and a starlark binding
package (`pkg/op/bind`). Clean up context ownership, receiver/action/method
relationships, and codegen. Remove redundancy and framework code from providers.

## Phase Status

| Phase | Status | PR |
|-------|--------|-----|
| 1. Create plan provider, flatten plan namespace | complete | #266 |
| 1.50. Add **kwargs to receiver bridges | complete | #267 |
| 2+3+4. Create bind, move files, sever starlark, update codegen | complete | #268 |
| 5+6. Sever starlark, consolidate registries, unify method dispatch | complete | #269 |
| 7. Graph/executor restructuring, context, codegen | in-progress (test fixes remaining) | — |
| 8. ReceiverType interface cleanup, unified dispatch | not-started | — |
| 9. Output specs and the companion triplet | in-progress (Steps 1–8 done; Step 9 triage paused pending Phase 10) | — |
| 10. Slot = (Parameter, Value): type-driven fill and dispatch | not-started | — |
| 11. Implement `plan.subgraph` as a flow provider method | not-started | — |
| 12. Address defects on the flow provider | not-started | — |
| 13. Catalog serialization, Rebind rehydration | not-started | — |

## What Phase 7 Covers (Current PR)

### Subgraph: universal execution unit

The graph is a system. A subgraph is a subsystem — a functional, structural,
and transactional boundary. Subgraphs are recursive: a subgraph contains
nodes and child subgraphs, forming a tree. The graph is the root of the tree.

`Phase` goes away. `Subgraph` replaces it as the universal execution unit.
All subgraphs participate in the saga pattern: retry, compensation, status
tracking, attempt history.

#### Structure

A `Subgraph` owns its children directly — not by ID reference into a flat
parent list:

```go
type Subgraph struct {
    ID         string           `json:"id" yaml:"id"`
    Name       string           `json:"name" yaml:"name"`
    Children   []SubgraphChild  `json:"children" yaml:"children"`
    Status     SubgraphStatus   `json:"status" yaml:"status"`
    Retry      *RetryPolicy     `json:"retry,omitempty" yaml:"retry,omitempty"`
    Compensate string           `json:"compensate,omitempty" yaml:"compensate,omitempty"`
    Attempts   []Attempt        `json:"attempts,omitempty" yaml:"attempts,omitempty"`
    State      map[string]any   `json:"state,omitempty" yaml:"state,omitempty"`
    Branch     bool             `json:"branch,omitempty" yaml:"branch,omitempty"`
}

// SubgraphChild is either a node or a child subgraph.
// Exactly one field is set.
type SubgraphChild struct {
    Node     *Node     `json:"node,omitempty" yaml:"node,omitempty"`
    Subgraph *Subgraph `json:"subgraph,omitempty" yaml:"subgraph,omitempty"`
}
```

Children are ordered — execution proceeds through the children list in
declaration order. A child node is executed directly. A child subgraph is
entered recursively, applying its own saga semantics (retry, compensation,
hooks) before returning to the parent.

The `Graph` struct changes accordingly:

```go
type Graph struct {
    // Root children — nodes and top-level subgraphs in declaration order.
    Children []SubgraphChild `json:"children" yaml:"children"`
    // ... Edges, Provenance, State, Summary, etc. remain
}
```

`graph.Nodes` and `graph.Subgraphs` as separate flat lists go away. The
graph's `Children` list IS the structure. `graph.Edges` still exist for
cross-node dependencies (promise resolution, ordering constraints).

A flat graph has children that are all nodes — no subgraphs. The executor
sees a flat children list and runs them in order with no saga boundaries.

#### Composability

A subgraph can appear anywhere a node can. The planner decides the structure:

- A choose branch can be a single node or a subgraph.
- A gather body can be a single action or a multi-step pipeline.
- A top-level phase is a subgraph.
- A single node can be wrapped in a subgraph to get retry/compensation.
- Subgraphs nest arbitrarily — each level is its own saga boundary.

A failure unwinds the current level's compensation before propagating up.
The executor walks the tree recursively, applying the same saga semantics
at every level.

#### Starlark syntax

A lambda defines a subgraph scope. The planner calls it, captures whatever
children (nodes and subgraphs) are created, and wraps them in a `Subgraph`.
A single node needs no wrapping. Operational metadata (retry, compensation)
is attached by the caller.

```python
# A node is a node.
plan.file.write_text(destination=dest, content="foo", mode=0o644)

# A node with retry — still just a node.
plan.file.write_text(destination=dest, content="foo", mode=0o644).retry(max_attempts=3)

# Choose with a single action — no subgraph needed.
plan.choose(when=exists, then=lambda: plan.file.remove(path=dest))

# Choose with a multi-step body — implicit subgraph.
plan.choose(when=exists, then=lambda: (
    plan.file.remove(path=dest),
    plan.file.write_text(destination=other, content="replaced", mode=0o644),
))

# Explicit subgraph with retry and compensation.
plan.subgraph("install",
    body=lambda: plan.pkg.install(name="vim"),
    retry=3,
    compensate=lambda: plan.pkg.remove(name="vim"),
)

# Nested subgraphs — each level has its own saga semantics.
plan.subgraph("deploy",
    body=lambda: (
        plan.subgraph("install",
            body=lambda: (
                plan.pkg.install(name="nginx"),
                plan.file.write_text(
                    destination="/etc/nginx/nginx.conf",
                    content=config, mode=0o644),
            ),
            retry=3,
            compensate=lambda: plan.pkg.remove(name="nginx"),
        ),
        plan.subgraph("verify",
            body=lambda: plan.shell.exec(command="nginx -t"),
            retry=2,
        ),
        plan.complete(output="deployed"),
    ),
)

# Gather with a subgraph body per item.
plan.gather(items=packages, do=lambda pkg: (
    plan.pkg.install(name=pkg.name),
    plan.file.write_text(
        destination=pkg.config, content=pkg.defaults, mode=0o644),
), retry=2)
```

#### Ordering: Kahn's algorithm at every level

Nodes and subgraphs are peers. At any level in the hierarchy, children
(nodes and subgraphs) are vertices in a dependency graph. Edges at that
level express ordering constraints between siblings. Kahn's algorithm
sorts them — same algorithm, every level.

Edges reference both node IDs and subgraph IDs uniformly. An edge
`{From: "install-sg", To: "verify-sg"}` works the same as
`{From: "write-node", To: "read-node"}`. A subgraph is opaque from its
parent's perspective — a single vertex with inputs and outputs. The parent
doesn't see inside it.

After sorting a level, the executor descends into each subgraph child and
sorts its children the same way. Recursive application of the same
algorithm at every level in the hierarchy.

#### Executor: recursive tree walk

The executor has one method for running a subgraph. It sorts the children
at that level (Kahn's), then walks the sorted list:

- **Node child**: resolve action, resolve slots, call `Do`, push recovery.
- **Subgraph child**: enter recursively — apply retry, hooks, compensation
  at that level, sort its children, execute them, then return the result
  to the parent.

```
executeSubgraph(sg):
  → sort sg.Children by edges at this level (Kahn's)
  → for each child in sorted order:
      → node: executeNode
      → subgraph: executeSubgraph (recurse)
          → retry logic (attempts, backoff)
          → hooks (subgraph start/complete)
          → on failure: compensate, propagate error
```

No `runFlat` vs `runSubgraphs` distinction. The graph root is treated
as a subgraph. The executor calls `executeSubgraph` once and the
recursion handles everything.

#### Executor: `Run` returns a result

`Run(graph *op.Graph) (op.Result, error)` — the graph produces a result, not
just a receipt. The result comes from the graph's terminal node:

- `flow.Complete(output)` — the output value
- `flow.Degraded(format, args, kwargs)` — the warning message
- `flow.Fatal(format, args, kwargs)` — error (returned as the error, not the
  result)

If the graph has no explicit terminal (falls off the edge), the result is the
result of the final node executed.

#### Executor: dead code removal — complete

All removed: `hydrateProviders`, `OrderNodes`, `topologicalSortNodes`,
`sortNodesByDepth`, `FillSlotsFromData`, `runFlat`, `runSubgraphs`.
Replaced by recursive tree walk (`executeChildren`/`executeSubgraph`).

#### SlotValue: `DataRef` kind

Fourth slot kind alongside immediate, promise (`NodeRef`), and proxy
(`GatherRef`). Binds a slot to a `RuntimeEnvironment.Property` key at plan
time, resolved at execution time.

```go
type SlotValue struct {
    DataRef   string `json:"data_ref,omitempty" yaml:"data_ref,omitempty"`
    Field     string `json:"field,omitempty" yaml:"field,omitempty"`
    GatherRef string `json:"gather_ref,omitempty" yaml:"gather_ref,omitempty"`
    Immediate any    `json:"immediate,omitempty" yaml:"immediate,omitempty"`
    NodeRef   string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`
    Slot      string `json:"slot,omitempty" yaml:"slot,omitempty"`
}
```

`ResolvedSlots` resolves `DataRef` from `env.Property(key)` via the node's
`RuntimeEnvironment`, same pattern as promise and proxy resolution.
Serializes to YAML as `data_ref: identity`.

Data is immutable after execution begins (enforced by the
`RuntimeEnvironment` interface — no public write path). DataRef resolution
is safe at any point during execution because the values cannot change.

This eliminates `FillSlotsFromData`. Every slot is declared, every binding is
explicit, and the graph is the complete specification of where every value
comes from.

#### Starlark surface: `plan.dataref(name)`

`DataRef` is instantiated in Starlark via the plan provider:

```python
identity = plan.dataref("identity")
plan.file.write_text(path="/etc/foo", owner=identity)
```

`plan.dataref("identity")` returns a `DataRef` — a `starlark.Value` that
carries the key name, analogous to `Promise`. It flows through `FillSlot` the
same way a promise does:

```go
if ref, ok := value.(*DataRef); ok {
    node.SetSlotDataRef(slotName, ref.Key)
    return nil
}
```

`Promise` is "the promise of a node's output." `DataRef` is "the promise of
an environment property from `RuntimeEnvironment.Property(key)`." Both
declare where a value comes from without baking it in. Both survive
serialization.

### Context restructuring: RuntimeEnvironment

The execution context splits into two concerns:

1. **`RuntimeEnvironment`** (interface) — immutable operational constraints
   set by the tool before execution begins. Read-only after `Run` starts.
2. **Execution state** (executor-internal) — mutable state accumulated
   during execution (`Results` map for promise resolution, recovery stack).

#### RuntimeEnvironment interface

```go
type RuntimeEnvironment interface {
    Console() Console
    DryRun() bool
    Extension(name string) any
    Platform() *Platform
    ProgramName() string
    Property(key string) any
    Root() Root
    Sops() *sops.Client
}
```

#### Console interface

```go
type Console interface {
    io.Writer                        // raw byte output (subprocess forwarding)
    Note(msg string)                 // informational progress
    Warn(msg string)                 // potential issue
    Error(msg string)                // non-fatal problem
    Success(msg string)              // completion confirmation
}
```

The tool constructs the `Console` at startup — decides color, verbosity,
output target, formatting, symbols. Sealed in the environment. The `ui`
provider becomes a thin bridge that delegates to `env.Console()`. Other
providers (shell, git, service) call `env.Console().Write()` for subprocess
output and `env.Console().Note()` for status messages instead of writing
to a raw `io.Writer`.

The executor constructs a `RuntimeEnvironment` from `RuntimeEnvironmentSpec`,
seals it, and passes it into the graph via `Rebind`. Providers and actions
receive `RuntimeEnvironment` — they can read the environment but cannot
mutate it.

#### RuntimeEnvironmentSpec → RuntimeEnvironment construction

`RuntimeEnvironmentSpec` is the configuration input. `RuntimeEnvironment` is
the sealed runtime output. The executor (or test harness) constructs the
environment from the spec.

`RuntimeEnvironmentSpec` includes a `Platform` field, set via
`WithPlatform(value op.Platform)`. When nil, the executor defaults to
`NewPlatform()` (detect the current host). When set, the executor uses the
provided platform — enabling cross-platform planning (plan on Mac, target
Linux) and testing (mock managers without host detection).

#### Platform construction

`op.Platform` must support creating a platform representing any target, not
just the current host. `NewPlatform()` remains for host detection.
`NewPlatformSpec(os, arch string) *Platform` creates a synthetic platform
with identity fields set and nil managers — suitable for cross-platform
planning where the planner needs to know the target OS/arch but does not
execute against real package or service managers. Tests that need mock
managers construct via `NewPlatformSpec` and set the manager fields directly.

- `Property(key string) any` — retrieves a named characteristic of the
  environment (template vars, identities, segments). Replaces the
  `Data map[string]any` field (now `Property`). No public write path exists after
  construction. Immutability is guaranteed — important for DataRef slot
  resolution, which depends on values not changing between sort time and
  execution time.
- `Extension(name string) any` — retrieves a provider instance by name.
  A pluggable capability provided by another component. The
  `ExecutionContext` implements this by looking up the factory in the
  `Registry`, constructing/caching the instance, and returning it. The
  consumer never sees the registry.

#### What moves out of ExecutionContext

- `Results map[string]any` — executor-internal. The executor owns it,
  passes it to `ResolvedSlots` for promise resolution, and actions never
  see it directly.
- `RecoverySite` — executor-internal. Managed by the executor's recovery
  stack.
- `Thread` — Starlark runtime state, not an environmental constraint.

#### What stays (behind the interface)

- `Platform` — OS, package manager, service manager. Provided via
  `RuntimeEnvironmentSpec.WithPlatform`; defaults to host detection when
  not specified.
- `Root` — filesystem boundary (confinedRoot for execution)
- `ProgramName` — tool identity ("lore", "writ")
- `Console` — user-facing output (structured messaging + raw subprocess forwarding)
- `DryRun` — mode flag
- `SopsClient` — encryption configuration
- `Property` — tool-provided context (template vars, identities, segments)
- `Extension` — provider instance lookup (backed by Registry internally)

#### ExecutionContext and RuntimeEnvironment

`RuntimeEnvironment` is immutable. It is constructed from
`RuntimeEnvironmentSpec` and sealed before execution begins.

`ExecutionContext` holds a `RuntimeEnvironment` as a private member and
exposes it via an accessor:

```go
type ExecutionContext struct {
    context.Context
    env          RuntimeEnvironment
    Catalog      *ResourceCatalog
    RecoverySite *RecoverySite
    Registry     *ReceiverRegistry
    Results      map[string]any
    Thread       starlark.Thread
}

// RuntimeEnvironment returns the immutable runtime environment.
func (ctx *ExecutionContext) RuntimeEnvironment() RuntimeEnvironment { return ctx.env }
```

Providers and actions receive `*ExecutionContext`. They access environmental
data through `ctx.RuntimeEnvironment()` — e.g.,
`ctx.RuntimeEnvironment().Platform()`,
`ctx.RuntimeEnvironment().DryRun()`,
`ctx.RuntimeEnvironment().Root()`. The environment is read-only;
the mutable execution state (`Results`, `RecoverySite`, `Catalog`) lives
on `ExecutionContext` directly.

The `Registry` stays on `ExecutionContext` (executor/framework
infrastructure). Providers access other providers via
`ctx.RuntimeEnvironment().Extension(name)` — which delegates to the
registry internally.

### ReceiverRegistry

- `NewReceiverRegistry()` returns a populated registry. Calls `Init()`
  internally — no separate `InitAll` step.
- `Init()` is a method on `ReceiverRegistry`, not a free function.
- `resetAnnounced()` removed — global state should not be destructively
  cleared.

### Plan provider

The plan provider is a legitimate executing provider with methods: `Complete`,
`Degraded`, `Fatal`, `Dataref`, `WaitUntil`, `Gather`, `Choose`. It owns its
`Graph` and `ReceiverRegistry`.

`Choose` and `Gather` operate on subgraphs — they call the same executor path
as the top-level saga runner. No special-case execution logic in the flow
provider.

`ResolveAttr` routes sub-namespace lookups (`plan.file`) by querying the
registry for the action receiver type and wrapping it with `bind.NewPlanner`.
The return value is marshaled by the framework.

### Planning receiver routing

Starlark resolves `plan.file.write_text(...)` as three attribute lookups:

1. `plan` → `ExecutingReceiver` wrapping plan Provider
2. `.file` → `plan.Attr("file")` → falls through to `AttributeResolver` →
   `ResolveAttr("file")` → registry lookup → `bind.NewPlanner(prt, graph)` →
   marshaled as starlark value
3. `.write_text` → `Attr("write_text")` on the planning receiver → callable

The framework handles marshaling. The plan provider handles routing via
`ResolveAttr`. `ProviderReceiverType` provides the type descriptor;
`bind.NewPlanner` wraps it for plan-mode dispatch.

### Providers and Resources

Providers and resources are both Go types with methods. The framework reflects
on them identically — same type description, same method dispatch, same
starlark representation via `ExecutingReceiver`. The difference is lifecycle
and role.

**Providers:**

- Singleton per session — acquired once, added to starlark globals as builtins
- Methods are actions — they do things (write files, install packages)
- Accept resources as arguments (slot values), produce resources as results
- Access: immediate (execute directly), planned (create graph nodes), or both
- Codegen emits: `AnnounceProvider` with constructor and method parameters

**Resources:**

- Created by provider actions, flow through the graph as data
- Marshaled to starlark (when returned by a method) and from starlark (when
  passed as an argument to fill a slot)
- Methods are properties/accessors — expose data (path, URI, content)
- Always immediate — no planning, no actions, no graph nodes
- Codegen emits: marshal/unmarshal support, method bridges for
  `ExecutingReceiver`, no action registration

### ReceiverType / Receiver architecture

A `ReceiverType` describes a receiver — its name, methods, and provider type.
It is shared across all instances. Created by codegen, stored in the registry.
Describes both providers and resources.

A receiver wraps a provider or resource and calls its methods via the
reflected type definition. Two kinds:

**`ExecutingReceiver`** — a `starlark.Value` that dispatches method calls to a
live provider or resource instance. Holds `receiverType *ReceiverType` (shared
type info) and `provider any` (per-instance — provider or resource). Used for
both provider builtins and marshaled resources. `Attr(name)` looks up the
method on the type, bridges the call via reflection.

**`PlanningReceiver`** — a `starlark.Value` that wraps each method call in an
action and adds it to a graph. Holds `receiverType *ReceiverType` and
`ctx *GraphExecutionContext`. Only for providers with planned access.
`Attr(name)` looks up the method on the type, returns a callable that creates
a node with an action wrapping that method.

Both receivers borrow their type — they don't own methods or metadata. The
type is the factory. The receiver is the instance.

### Actions wrap methods

Actions are what nodes hold. A method is the callable — reflected Go function
with params. An action wraps a method and carries the execution contract.

The planning receiver creates an action from a method when building a node:

```go
action := op.NewAction(m)  // wraps *Method in the correct Action kind
node := &op.Node{
    ID:     op.GenerateNodeID(m.ActionName),
    Action: action,
}
```

The codegen generates distinct action types per return signature:

- `() or (T)` → `Action` (pure)
- `(error) or (T, error)` → `FallibleAction`
- `(T, U, error)` + `Compensate<Name>` → `CompensableAction`

Each action holds: the `*Method` and the `*ProviderBase`. No factory
reference. The executor sets the provider on the action during hydration.
`Do` and `Undo` receive provider as a parameter:

```go
Do(ctx *ExecutionContext, provider *ProviderBase, slots map[string]any) (Result, Complement, error)
Undo(ctx *ExecutionContext, provider *ProviderBase, complement Complement) error
```

No `Method.Factory` back-pointer. No circular reference between type
descriptor and method. The method is pure metadata + dispatch logic. The
type descriptor owns the methods. The executor owns provider lifecycle.

### Resource identity and construction

Every resource constructor follows the contract:

```go
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error)
```

`value` is a descriptor carrying the information needed to formulate
identity. The constructor casts it, extracts identity components, builds
the URI inline, and returns a pointer. URI is immutable after construction.
`Resolve()` enriches metadata but does not alter identity.

#### Changes made

- **`op.PURL`** — new structured type in `pkg/op/purl.go`, modeled after
  `url.URL`. Struct with `Type`, `Namespace`, `Name`, `Version`,
  `Qualifiers`, `Subpath`. `String()` serializes, `ParsePURL()` deserializes.
- **`PackageManager.ParsePURL(id string) PURL`** — added to interface. Each
  manager (brew, port, apt/deb, dnf/rpm, pacman/alpm, winget) parses its
  own naming convention into purl components. Winget splits publisher
  namespace on first dot.
- **`pkg.NewResource`** — parses optional manager prefix (`"brew:jq"`,
  `"port:wget"`) via `strings.Cut`, delegates to `mgr.ParsePURL()` for URI.
  `NewTypedResource` and `ResourceURI` deleted. `ParsePackagePrefix` from
  `internal/lorepackage` replaced by inline `strings.Cut`.
- **`pkg.Resource.Resolve()`** — implemented. Queries local package manager
  for installed version. No-op if platform or manager unavailable.
- **`Resource.Resolve()` parameter removed** — interface changed from
  `Resolve(root Root) error` to `Resolve() error`. All implementations
  access root via `ExecutionContext().Root`. Four implementations and ~27
  call sites updated.
- **`file.Resource.Refresh()` / `RefreshWith()`** — `root` parameter
  removed. Access root via `ExecutionContext().Root`.
- **`file.NewResource`** — uses `ctx.Root.NewPath(path)` to canonicalize
  at construction time. Conflicts detectable at plan time because relative
  paths resolve to canonical absolute paths via the declared root.
- **`ResourceURI` functions eliminated** — all seven providers (appnet, mem,
  git, yaml, file, json, service) now build URIs inline in constructors.
  `appnet`, `yaml`, `json`, `service` deleted outright. `git` renamed to
  unexported `gitURI` helper.
- **`SetURI` stays deleted** — URI is immutable after construction.
  `Resource` interface doc updated to reflect this.
- **`mem.ResourceSpec`** — `Qualifier` replaced with `Namespace` + `Name`.
  `ResourceSpec.URI()` method formulates `mem:<contentType>/<namespace>/<name>`.
- **`mem.Function`** — `FuncType` and `Name` fields removed (redundant with
  embedded `Resource.Namespace` and `Resource.Name`). `fn` field and `Fn()`
  method removed (issue #273 — data race in gather). `Init()` returns
  `(starlark.Callable, error)` instead of storing on struct.
- **All `NewResource` constructors return `*Resource`** — standardized on
  pointer returns for implicit `op.Resource` interface satisfaction.

#### Resource identity contract

- URI is the sole identity — string equality via the catalog
- URI is immutable after construction — no `SetURI`
- `Resolve()` enriches metadata (stat, version) without altering identity
- Shadowing is same-URI, different-origin — lineage, not content comparison
- Fragment (`#...`) strips from catalog keys per design docs

### Summary cleanup — complete

`Graph.Summary()` returns `GraphExecutionSummary` (embeds
`ActionExecutionSummary`). No hardcoded receiver names. `ByAction()` returns
`map[string]ActionExecutionSummary` with per-action completed/failed/skipped
counts. Both interfaces embed `json.Marshaler` and `yaml.Marshaler`.
Domain-specific presentation (`formatGraphSummary`) moved to `cmd/writ`.

### Executor: `executeNode` fixes — complete

- `node.ExecutionContext()` accessor — done
- `node.Action()` method (resolves via `Receiver` + registry) — done
- Phantom `SessionSlotKey` / `GraphExecutionContext` — removed
- Type references qualified with `op.` — done
- `Run` returns `(any, error)` — terminal node's output value — done

### Codegen templates — complete

Source templates:

- `provider.gen.go.template` — emits `AnnounceProvider` with constructor
  and method parameters.
- `dependent_type.gen.go.template` — emits `AnnounceType` for dependent
  types.
- `resource.gen.go.template` — emits `AnnounceResource` with constructor
  and method parameters.

Test templates:

- `receiver_type.gen_test.go.template` — tests `ReceiverType` dispatch:
  name, type, method enumeration. Generated always.
- `module.gen_test.go.template` — tests starlark module protocol:
  `Attr`, `AttrNames`, `Type()` via `bind.NewProvider`. Generated when
  access is `immediate` or `both`.
- `action.gen_test.go.template` — tests action wrappers: dry-run dispatch,
  compensable interface, undo-nil. Generated when access is `planned` or
  `both`.
- `planner.gen_test.go.template` — tests planning receiver: `bind.NewPlanner`
  attr resolution, node creation from starlark calls. Generated when
  access is `planned` or `both`.

Templates embedded in `star` binary via `//go:embed extensions`.

### Test fixes — in progress (2026-04-08)

Production code builds clean (`make build` passes). Only test files are broken.

#### Structural changes made

- Moved executor from `internal/execution` to `pkg/op` (executor.go, hooks.go, activation.go, preflight.go, dependencyview.go)
- Added `ExecutionContext.ExecuteSubgraph()` — delegates to executor for flow provider
- Fixed `executingReceiver.Attr()` to check `AttributeResolver` fallback
- Fixed `executingReceiver.Type()` to return `receiverType.ReceiverName()` instead of "module"
- Fixed `Planner.Type()` to return `"plan." + name` instead of "module"
- Fixed `Graph.Rebind()` to propagate ctx to child nodes
- Fixed plan provider `NewProvider` to read graph from `ctx.Data["graph"]`
- Fixed executor `newContext` to create `ReceiverRegistry`
- Added resource coercion in `coerceSlotValue` (action_types.go) for execution path
- Added `Node.SetAction()` for test action injection
- Changed `Run()` to return `(any, error)`
- Changed all 21 provider method signatures from value to pointer resource types
- Deleted `ResourceFromValue` from json/resource.go
- Redesigned Summary → ActionExecutionSummary / GraphExecutionSummary interfaces
- Four codegen test templates: receiver_type, module, action, planner
- Flow provider access annotation fixed to `planned`
- Flow and plan providers added to Makefile

#### Test files deleted (dead code)

- `pkg/op/announce_test.go`
- `pkg/op/receiver_registry_test.go`
- `pkg/op/bind/action_test.go`
- `pkg/op/bind/callable_test.go`
- `internal/execution/compensation_test.go`
- `internal/execution/execution_flow_test.go`

#### Test files rewritten

- `pkg/op/provider/flow/flow_test.go` — uses Provider methods, testNode helper, SetAction
- `pkg/op/provider/flow/integration_test.go` — uses ctx.ActionByName
- `cmd/devlore-test/devloretest/runner_test.go` — removed dead imports and WithReceivers
- `internal/execution/lifecycle_test.go` → `pkg/op/lifecycle_test.go` — removed hydration
- `pkg/op/dependencyview_test.go` — uses Children instead of Nodes

#### 14 build failures remaining (tests don't compile)

1. `pkg/op` — `subgraph_test.go`: `fileAction` undefined, old `NewGraphExecutor()`. `graph_test.go`: partially fixed
2. `pkg/op/bind` — `promise_test.go`: `SetURI` removed. `receiver_factories_test.go`: `newReceiver` undefined
3. `pkg/op/provider/appnet` — `integration_test.go`: `op.contextBase`, `bind.WrapProviderInExecutingReceiver`
4. `pkg/op/provider/encryption` — `integration_test.go`: stale refs
5. `pkg/op/provider/file` — `provider_test.go`: `**Resource` double pointer
6. `pkg/op/provider/git` — `provider_test.go`: `ExecutionContext{}` value where pointer needed
7. `pkg/op/provider/mem` — `callable_test.go`: `Callable` type removed
8. `pkg/op/provider/pkg` — `provider_test.go`: `op.contextBase`, missing `ParsePURL`, value vs pointer
9. `pkg/op/provider/service` — `provider_test.go`: in progress — signatures changed to pointer, test uses `NewResource(nil, "name")` helper
10. `pkg/op/provider/shell` — `integration_test.go`: `op.contextBase`, `bind.WrapProviderInExecutingReceiver`
11. `internal/cli` — `receipts_test.go`: `g.Tool` field doesn't exist
12. `internal/execution` — `stateview_test.go`: stale refs
13. `cmd/lore/lore` — `runtime_test.go` (9 NewActionRegistry), `integration_test.go` (dead APIs)

#### 12 runtime failures remaining

- `pkg/op/provider/archive`: TestStarlark (1)
- `pkg/op/provider/plan/gen`: TestModule_Attr_Unknown (1)
- `pkg/op/provider/platform`: TestStarlark (1)
- `pkg/op/provider/ui`: TestActions_Note, TestActions_Fail (2)
- `cmd/devlore-test`: TestCLI_SummaryOnly, TestCLI_ReceiptOnly*, TestCLI_RoutToFiles (4)
- `cmd/devlore-test/devloretest`: TestCompensation, TestMkdirAndRemoveAll (2)
- `cmd/star/provider/goast`: TestConfigSchemas_ProviderPicksUpConfig (1)
- `cmd/star/provider/star*`: TestActions_Analyze, TestCaptureRecursive, TestActions_ComputeComplexity, TestActions_IndexFiles, TestActions_ComputeStats (5)
- `cmd/star/star`: TestLintCopyright_* (8), TestSourceFile_* (1) — 9 total

#### Common patterns across build failures

- `op.contextBase` — unexported struct removed. Fix: use `&op.ExecutionContext{Field: value}` directly
- `bind.WrapProviderInExecutingReceiver` — removed. Fix: use `bind.NewProvider(prt, instance)`
- `*.Receiver` exported variable — removed from gen packages. Fix: use `NewReceiverRegistry()` + lookup
- `Resource{}` struct literals — must use `NewResource(nil, value)` to get proper ResourceBase/URI
- `ExecutionContext{}` value — must be `&ExecutionContext{}` pointer
- `NewGraphExecutor()` — now takes `(name string, Options)` and returns `(*GraphExecutor, error)`
- `g.Tool` — field removed from Graph

## Phase 8: ReceiverType Interface Cleanup, Unified Dispatch

### Problem

`ReceiverType` conflates three concerns under the word "receiver":

1. The type descriptor itself (`ReceiverType`)
2. The starlark identity (`ReceiverName()`)
3. The starlark wrappers (`executingReceiver`)

The interface has naming problems: `ReceiverName()` is ambiguous (Go and
starlark both have receivers), `ProviderType()` collides with the subtype
`ProviderReceiverType`, and `Do()` suggests execution rather than dispatch.

There are also two parallel dispatch paths: `ReceiverType.Do()` compiles and
caches an optimized closure per method, while `executingReceiver` bypasses
it entirely and calls `Method.Do()` directly — which recomputes zero values,
variadic checks, and return extraction on every call.

Additionally, `executingReceiver.Type()` hard-codes `"module"` instead of
delegating to the descriptor's name. `Value.Type()` and `Resource.Type()`
exist solely to compensate for this bug by overriding with
`receiverType.ReceiverName()`. `Provider` doesn't override, so it inherits
the broken `"module"`, causing all `TestType` failures across `*/gen`
packages.

### Changes

#### Interface renames on `ReceiverType`

| Before | After | Rationale |
|--------|-------|-----------|
| `ReceiverName()` | `Name()` | The descriptor already identifies the kind; "Receiver" is redundant and ambiguous |
| `ProviderType()` | `Type()` | Returns the `reflect.Type`; `ProviderType` collides with the subtype name |
| `Do()` | `Dispatch()` | It's a cached dispatch table lookup, not a command |

#### Remove `MethodByName` from the interface

`MethodByName` has no callers outside `ReceiverType` itself and its tests.
Remove it from the `ReceiverType` interface. Keep the implementation as
unexported `methodByName` for internal use by `compileDispatcher` and
compensable method lookup.

#### Rewire `executingReceiver` to use `ReceiverType.Dispatch`

`executingReceiver.dispatchSimple` and `executingReceiver.dispatchVariadic`
currently call `method.Do(r.receiver, goArgs)`. Change both to call
`r.receiverType.Dispatch(methodName, r.receiver, goArgs)`. This eliminates
the duplicate dispatch path and ensures all method invocations go through
the cached dispatch table.

The `executingReceiver` still uses `Methods()` to enumerate method names
for building the starlark attribute map. That stays — enumeration and
dispatch are separate concerns.

#### Fix `executingReceiver.Type()`, remove redundant overrides

Change `executingReceiver.Type()` from `return "module"` to
`return r.receiverType.Name()`. Then delete `Value.Type()` and
`Resource.Type()` — they become identical to the inherited implementation.

### Blast area

| Change | Files touched | Non-test call sites |
|--------|---------------|---------------------|
| `ReceiverName()` → `Name()` | ~12 files | ~26 |
| `ProviderType()` → `Type()` | ~3 files | ~3 |
| `Do()` → `Dispatch()` | ~2 files | ~2 (interface + impl) |
| Remove `MethodByName` from interface | ~2 files | ~1 |
| Rewire `executingReceiver` dispatch | 1 file | 2 call sites |
| Fix `executingReceiver.Type()` | 1 file | 1 |
| Remove `Value.Type()` | 1 file | 1 |
| Remove `Resource.Type()` | 1 file | 1 |

The `ReceiverName` → `Name` rename is the widest change. Everything else
is narrow.

## Phase 9: Output Specs and the Companion Triplet

### Problem

The planner needs to know, at plan time, which resource each node will
produce — so it can shadow the output URI in the catalog, detect
conflicts early, and create implicit edges via URI matching. But given
a method signature like `Copy(source *Resource, destination *Resource)`,
the types alone don't distinguish source from destination. Both
parameters are `*file.Resource`.

Name-based heuristics ("the last Resource parameter of a compensable
method is the destination") fail on `Remove(path, prune, boundary)`
where the last resource parameter is a constraint, not an output. Any
heuristic this fragile is a fiction the framework and the providers
have to agree on, and it drifts.

### Solution: the `Planned` companion method

Every provider method that produces a resource declares a pure sibling
method — its **`Planned` companion** — that computes the identity of
the resource the forward method will produce, given the same arguments.
The companion is pure: no I/O, no target-machine state. The planner
calls it at plan time (via `Method.Plan`) to populate the catalog; the
forward method calls it at execution time to construct the result it
is about to return. One source of truth, shared between planning and
execution.

The pattern borrows from Bazel's analysis-phase `declare_file` and
Terraform's `PlanResourceChange`. It's the **applicative** case of the
"Build Systems à la Carte" framework. For monadic outputs (identity
depends on runtime state — cross-platform `pkg.Install`, cloud-assigned
instance IDs, content-addressed filenames), the spec returns a
`KnownAtExecution` sentinel borrowed from Terraform's `(known after
apply)` phrasing. The planner skips plan-time shadowing for these;
the executor shadows the real return value post-dispatch.

See `docs/architecture/4-resource-management.md` §6.8 "Output Specs"
and §6.9 "Comparison to Bazel Declared Outputs" for the full design.

### Companion triplet

Every resource-producing provider method becomes a **companion triplet**:

| Member | Required | Purity | Runs when |
|---|---|---|---|
| `X` (forward) | always | effectful | execution |
| `XPlanned` (output spec) | when `X` returns a resource | pure | plan phase, and internally by `X` |
| `CompensateX` (compensation) | when `X` is compensable | effectful | rollback |

Source order for the triplet: forward, planned, compensate.

Only `X` is registered as a starlark-callable action. `XPlanned` and
`CompensateX` are discovered by reflection at provider-registration
time (`pkg/op/receiver_type.go:methodFromReflectedMethod`) and attached
to the forward method as companions. They are not listed in the generated
`methodParameters` map: codegen's `filter_methods` already skips
`Compensate*` and must be extended to skip `*Planned` (see "Framework
pieces to add").

Concrete example — `file.Copy`:

```go
// Copy writes source to destinationPath and returns the destination resource.
func (p *Provider) Copy(source *Resource, destinationPath string, mode os.FileMode) (*Resource, Tombstone, error) {
    dest, err := p.CopyPlanned(source, destinationPath, mode)
    if err != nil {
        return nil, Tombstone{}, err
    }
    // ... prepareWrite, copy bytes, Resolve, return
}

// CopyPlanned is the output spec for Copy. Pure: no I/O.
func (p *Provider) CopyPlanned(source *Resource, destinationPath string, _ os.FileMode) (*Resource, error) {
    return NewResource(p.Context(), destinationPath)
}

// CompensateCopy undoes a Copy by restoring the original file from recovery.
func (p *Provider) CompensateCopy(undo Tombstone) error {
    return p.compensateWrite(undo)
}
```

Note the signature change: `destinationPath string` instead of
`destinationFilename *Resource`. The destination is a raw path now,
and identity construction happens in `CopyPlanned` where both the
planner and the forward method can reuse it. Input Resources
(`source`) are typed; destinations expressed as paths become Resources
via the spec.

### Signature changes to provider methods

The rule: input Resources stay as `*Resource` parameters. Destinations
become `string` parameters. The output spec turns the string into a
typed resource using `NewResource` at plan time.

| Method | Before | After |
|---|---|---|
| `Copy` | `(source, destination *Resource, mode)` | `(source *Resource, destinationPath string, mode)` |
| `Move` | `(source, destination *Resource)` | `(source *Resource, destinationPath string)` |
| `Link` | `(source, target *Resource)` | `(source *Resource, targetPath string)` |
| `WriteText` | `(destination *Resource, content string, mode)` | `(destinationPath string, content string, mode)` |
| `WriteBytes` | `(destination *Resource, content []byte, mode)` | `(destinationPath string, content []byte, mode)` |
| `Mkdir` | `(resource *Resource, mode)` | `(path string, mode)` |
| `Backup` | `(resource *Resource, suffix)` | `(resource *Resource, suffix)` — unchanged; resource is input |
| `Remove` | `(resource *Resource, prune, boundary *Resource)` | `(resource *Resource, prune bool, boundary *Resource)` — unchanged; all inputs |
| `Clone` (git) | `(url *appnet.Resource, destination *file.Resource)` | `(url *appnet.Resource, destinationPath string)` |
| `Install` (pkg) | `(packages []*Resource, manager, cask)` | `(packages []*Resource, manager, cask)` — unchanged; returns monadic |

Every method in the first block gets a `Planned` sibling. Methods with
`KnownAtExecution` outputs (pkg.Install, similar) get a trivial `Planned`
sibling returning the sentinel.

### Framework pieces — DONE

Auto-discovery of the companion triplet replaces the originally planned
registration-based approach. No registration, no codegen-emitted spec
map, no `OutputSpec` type. Instead:

- **`KnownAtExecution` sentinel** (`pkg/op/resource.go`) — a distinguished
  `Resource` with URI `op:known-at-execution` returned by `Planned` companions
  whose output identity depends on runtime values. The planner skips plan-time
  shadowing when it sees the sentinel; the executor shadows post-dispatch. ✅
- **`Method.planned *reflect.Method`** (`pkg/op/method.go`) — companion slot
  next to `do` and `undo`. `HasPlanned()` reports presence; `Plan(receiver, args)`
  invokes the companion with reflection. `NewMethod` validates that the
  companion's parameter list matches the forward method exactly and that
  it returns `(T, error)` where T matches the forward method's first return
  type. ✅
- **Auto-discovery** in `pkg/op/receiver_type.go:methodFromReflectedMethod` —
  looks up `<Name>Planned` on the provider type symmetrically with the
  existing `Compensate<Name>` lookup. No registration, no announce option,
  no explicit wiring. ✅

### Framework pieces to add

1. **Codegen filter extension** in
   `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` —
   extend `filter_methods` to skip methods whose names end in `Planned`,
   symmetric with the existing `Compensate*` skip. Keeps `*Planned`
   companions out of the generated `methodParameters` map so they are
   discovered only as companions by reflection, not registered as
   standalone starlark-callable actions.

### Side work — DONE (not originally planned)

Work that landed alongside Phase 9 groundwork but was not in the original plan:

- **`ResourceCatalog.Resolve` signature change** — from `Resolve(uri string)`
  to `Resolve(r Resource)`. "Resources in, resources out" on the catalog
  boundary. Callers that held URI strings now construct the typed resource
  via the registered constructor and hand it to the catalog. ✅
- **Plan-time string→resource coercion** in the planner
  (`Planner.fillResourceSlot`, `pkg/op/bind/planner.go:342`). Every
  resource-typed parameter whose starlark argument is a string is coerced
  to a typed resource via the resource type's registered constructor at
  plan time, then routed through `catalog.Resolve` as a discovery entry.
  This makes all resource flow through the catalog, typed, before the
  executor runs. ✅
- **`executingReceiver` reads registry from context** — no more nil global
  registry; `coerceResource` reads `ctx.Registry` directly. Unblocks the
  registry-per-execution model. ✅
- **`devloretest.Runner` propagates `DryRun`** to the executor via
  `op.Options.DryRun`. Previously dry-run was a runner-level flag that
  did not reach preflight; with preflight re-enabled, this was needed so
  dry-run tests don't trigger filesystem resolution. ✅
- **`ResourceCatalog.Shadow(Resource, originID) (string, error)`** —
  takes a typed `Resource` (not a URI string) for symmetry with
  `Resolve`. Returns the new catalog ID and a write-write conflict error
  if the URI is already shadowed by a different origin. ✅

### Provider method rewrites

Apply the signature changes and author the `Planned` siblings for:

- `pkg/op/provider/file` — Copy, Move, Link, WriteText, WriteBytes, Mkdir, Backup, Clone
- `pkg/op/provider/git` — Clone
- `pkg/op/provider/archive` — Extract
- `pkg/op/provider/encryption` — DecryptSopsFile
- `pkg/op/provider/pkg` — Install/Upgrade/Remove (monadic — `Planned`
  returns `KnownAtExecution`)
- `pkg/op/provider/service` — Start/Stop/Enable/Disable (monadic or
  state-preserving; spec depends on the method)
- `pkg/op/provider/template` — Render (if it produces a file output)
- `pkg/op/provider/mem` — output methods if any

### Planner rewire — DONE

`pkg/op/bind/planner.go` `dispatch`:

1. Name-based output detection removed. ✅
2. Resource-typed parameters are coerced via the type's registered
   constructor and routed through `catalog.Resolve` as discovery
   entries. Non-resource parameters go through normal `FillSlot` /
   unmarshal. ✅ (`Planner.fillResourceSlot`)
3. After all slots are filled: if `method.HasPlanned()`, `shadowPendingOutput`
   constructs a receiver via `rt.Construct()(ctx)`, builds positional
   args from `node.Slots` in parameter order, calls `method.Plan(receiver, args)`,
   and shadows the returned pending resource in the catalog
   (`KnownAtExecution` skips shadowing). ✅
4. The promise carries the node ID; the catalog carries the pending
   resource identity. ✅

### Executor post-dispatch shadowing

`pkg/op/executor.go` `executeNode`:

1. Call `method.Do(ctx, slots)` as before.
2. If the result is a resource type, look up the catalog's pending
   entry for its URI. If present (plan-time shadowed), transition
   pending → resolved by copying metadata from the real result.
3. If not present (monadic case, `KnownAtExecution`), shadow the real
   result now with the node's ID. Fail if this creates a URI conflict
   with an already-resolved entry.

### Preflight re-enable — DONE

`pkg/op/executor.go` `Run`:

1. After `Rebind`, before executing any node, calls
   `ResolveResources(graph.Catalog)`. ✅
2. Discovery entries get their existence checked against the target
   machine. Pending entries (plan-time shadows) are skipped because
   their producer will create them at execution time. ✅
3. Preflight skipped in dry-run mode. ✅ (devloretest runner propagates
   `WithDryRun` → `op.Options.DryRun` → executor)

### Blast area

- `pkg/op/method.go` — `planned *reflect.Method` field, `HasPlanned()`,
  `Plan(receiver, args)`, signature validation in `NewMethod`. ✅
- `pkg/op/resource.go` — `KnownAtExecution` sentinel. ✅
- `pkg/op/receiver_type.go` — `methodFromReflectedMethod` auto-discovers
  `<Name>Planned` by reflection, symmetric with `Compensate<Name>`. ✅
- `pkg/op/bind/planner.go` — `dispatch` rewire. ✅
- `pkg/op/bind/executing_receiver.go` — `coerceResource` reads registry
  from context. ✅
- `pkg/op/executor.go` — preflight call (✅) + post-dispatch shadowing
  (not started)
- Every provider in `pkg/op/provider/*` and `cmd/star/provider/*`:
  signature changes for methods with resource destinations, `Planned`
  siblings (not started)
- Every starlark test script that calls `plan.file.copy`,
  `plan.file.write_text`, etc. — no changes needed if the starlark
  surface stays the same (kwargs are still kwargs, just that
  destination kwargs now pass through as strings)
- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` —
  extend `filter_methods` to skip `*Planned` suffix (not started)
- Integration tests: verify plan-time conflict detection, implicit
  edges via URI matching, dry-run via `Planned` companion

### Ordering

This phase is a large-footprint change. Suggested order so intermediate
states build cleanly:

1. ✅ Framework groundwork: `KnownAtExecution` sentinel,
   `Method.planned`/`HasPlanned`/`Plan`, auto-discovery of `<Name>Planned`
   by reflection in `methodFromReflectedMethod`.
2. ✅ Rewrite the planner's `dispatch` to use `method.HasPlanned()` +
   `method.Plan(receiver, args)` instead of name heuristics. Methods with
   no `Planned` companion fall through to normal slot filling (no shadowing).
3. ✅ Re-enable preflight in the executor with dry-run skip.
4. ✅ Wire post-dispatch catalog reconciliation in the executor.
   `executeNode` inspects the method result: if it's a `Resource` (not
   `KnownAtExecution`), it calls `catalog.Transition(result, node.ID)`
   for plan-time-shadowed outputs (metadata filled in place) or
   `catalog.Shadow(result, node.ID)` for monadic outputs (late shadow).
   Catalog failures push the action to the recovery stack before
   surfacing the error, so compensation can unwind the already-performed
   side effect. Implemented as part of the greenfield catalog rewrite.
5. ✅ Extend `filter_methods` in `generate.star` to skip `*Planned` suffix
   (symmetric with existing `Compensate*` skip). Regeneration of
   `provider.gen.go` files happens when the first `Planned` method lands
   in Step 6.
6. ✅ Rewrite `file.Copy` end to end as the reference implementation:
   signature change (`destinationFilename *Resource` → `destinationPath string`),
   add `CopyPlanned` sibling, source order forward/planned/compensate.
   `Method.Plan` extended to convert args via `reflect.Convert` so starlark
   int literals coerce to typed aliases like `os.FileMode`. Generated
   `provider.gen.go` now lists `"Copy": {"source", "destination_path", "mode"}`;
   `CopyPlanned` filtered out by the Step 5 generate.star change.
7. ✅ Apply the same pattern to the remaining file methods:
   - `WriteText(destinationPath string, content, mode)` + `WriteTextPlanned`
   - `WriteBytes(destinationPath string, content, mode)` + `WriteBytesPlanned`
   - `Move(source *Resource, destinationPath string)` + `MovePlanned`
   - `Link(source *Resource, targetPath string)` + `LinkPlanned`
   - `Mkdir(path string, mode)` + `MkdirPlanned`
   - `Backup` unchanged (resource is input; output identity is
     timestamp-dependent, so no Planned companion; post-dispatch
     late-shadows via `catalog.Shadow`)
   Non-test callers in `cmd/writ/writ/{commands,migrate_cmd}.go` and
   `cmd/writ/writ/migrate/execute.go` updated to pass path strings.
   Starlark test scripts bulk-renamed (`destination=`→`destination_path=`,
   `target=`→`target_path=`, `mkdir(resource=`→`mkdir(path=`). Test
   count 43 → 30 (13 file tests now passing).
8. ✅ Apply the pattern to other providers:
   - **git.Clone** → `(url *appnet.Resource, destinationPath string)` +
     `ClonePlanned`; unused `file` import dropped.
   - **archive.Extract** → `(source *file.Resource, prefixPath string)` +
     `ExtractPlanned`.
   - **encryption.DecryptSopsFile** → `(source *file.Resource, destinationPath string)` +
     `DecryptSopsFilePlanned`.
   - **service.{Disable, Enable, Restart, Start, Stop}** — no signature
     changes. Each gets a trivial `XPlanned(name *Resource) (*Resource, error)`
     that returns the input unchanged, so the catalog shadows the same
     URI under this node's origin and creates implicit edges from the
     state-mutation node to downstream service consumers.
   - **pkg.{Install, Upgrade, Remove}** SKIPPED — return `[]*Resource`
     (slice), not a single `*Resource`, so the signature doesn't fit
     the single-Resource `Planned` shape validated by `NewMethod`.
     Slice-shaped Planned companions would need a separate mechanism
     (future work).
   Total `Planned` companions in the codebase after Step 8: **14**
   (6 file, 1 git, 1 archive, 1 encryption, 5 service).
9. 🟡 Run all tests. Fix whatever remains.
   **Done so far:**
   - `pkg/op/executor.go:205` — added `stack.Unwind()` on execution
     failure so compensation runs for previously-successful nodes.
   - `pkg/op/method.go:481` — `Method.Undo` used `results[0].Interface().(error)`
     which panics on nil error returns; changed to `errFromValue(results[0])`.
   - `pkg/op/bind/marshal_test.go:818-861` — deleted `testConstructable`,
     its `init()` `AnnounceResource` call, `TestUnmarshal_WithConstructor`,
     and `TestUnmarshal_Constructor_InvalidInput`. These tests asserted
     that `unmarshalValue` should consult a type→constructor lookup in
     the receiver registry. That lookup does not exist and should not
     be built.
   - Provider test files updated for new signatures:
     `pkg/op/provider/file/provider_test.go` (Link ×4, WriteText ×6,
     WriteBytes ×3, Move ×2, Mkdir ×3),
     `pkg/op/provider/archive/provider_test.go` (Extract ×5),
     `pkg/op/provider/encryption/provider_test.go` (DecryptSopsFile ×4),
     `pkg/op/provider/git/provider_test.go` (Clone ×2).

   **32 test failures remain (from 43 at start of Phase 9):**

   Phase 7 incompleteness — flow provider + starlark function coercion:
   - `TestChooseExists`, `TestChooseNotExists`, `TestIsDir`, `TestIsFile` —
     `pkg/op/provider/plan/provider.go:52 Provider.Choose` takes
     `then func() error`; `pkg/op/bind/marshal.go:947 unmarshalValue`
     has no `reflect.Func` case. The fix is NOT a reflect.Func case —
     it is implementing `Unmarshaler` (`pkg/op/bind/marshal.go:63`) on
     the type that wraps starlark callables for Go consumption
     (`pkg/op/provider/mem/function.go:39 Function`), so the existing
     Unmarshaler check in the marshal path handles it.
   - `TestWalkTreePlanned` — same class: `*starlark.Function` cannot
     coerce to `file.Reducer` (`pkg/op/provider/file/provider.go`).

   Phase 7 incompleteness — preflight + shell provenance:
   - `TestSource` — `cmd/devlore-test/devloretest/data/test_source.star`
     uses `plan.shell.exec` to create a file then `plan.file.read_text`
     to read it. Preflight fails because `shell.exec` has no `Planned`
     companion to shadow the output URI.

   Phase 7 incompleteness — executor recovery site:
   - `TestGraphLifecycle`, `TestGraphLifecycleWithPipeline` —
     `pkg/op/lifecycle_test.go`: `file.link` fails with
     `renameat . .devlore/recovery/...: invalid argument` (recovery
     site rename into a subdirectory of the source).
   - `TestActions_WriteText_ReadText` —
     `pkg/op/provider/file/integration_test.go`: same recovery-site
     rename failure.

   Phase 7 incompleteness — provider test nil ExecutionContext:
   - `TestExtractTarGz` — `pkg/op/provider/archive/provider_test.go:85`:
     `ExtractPlanned` calls `p.ExecutionContext()` which is nil because
     the test constructs `Provider{}` without an ExecutionContext.

   Phase 7 incompleteness — receiver attribute resolution:
   - `TestStarlark` (file, `pkg/op/provider/file/integration_test.go`) —
     `result_root: expected String, got builtin_function_or_method`.
   - `TestStarlark` (platform, `pkg/op/provider/platform/integration_test.go`) —
     `result_arch: expected String, got builtin_function_or_method`.
   - `TestLintCopyright_*` ×8 (`cmd/star/star/lint_copyright_test.go`) —
     `cfg.lint.copyright` resolves `lint` as `builtin_function_or_method`
     instead of a struct. Star config provider regression.
   - `TestSourceFile_StarlarkIntegration`
     (`cmd/star/star/sourcefile_integration_test.go`) — same class.
   - `TestConfigSchemas_ProviderPicksUpConfig`
     (`cmd/star/provider/goast/config_schema_test.go`) — same class.
   - `TestIntegrationEndToEnd` (`cmd/star/provider/starcode/integration_test.go`) — same class.

   Phase 7 incompleteness — git clone in test:
   - `TestActions_Clone` (`pkg/op/provider/git/integration_test.go`) —
     `exit status 128` from `git clone` (test environment issue or
     missing test hook).

   Phase 7 incompleteness — encryption integration:
   - `TestActions_DecryptSopsFile`
     (`pkg/op/provider/encryption/integration_test.go`) — destination
     path resolves to a directory, not a file.

   Devlore-test CLI output routing:
   - `TestCLI_SummaryOnly`, `TestCLI_ReceiptOnlyYAML`,
     `TestCLI_ReceiptOnlyJSON`, `TestCLI_RoutToFiles`
     (`cmd/devlore-test/cli_test.go`) — output channel filtering
     doesn't suppress shell.exec output when redirected.

   Pkg provider (deferred — `[]*Resource` return shape):
   - `TestPkgActions`, `TestEngineRunsPackageInstallActions`,
     `TestEngineRunsNamespacedPackageActions`
     (`cmd/lore/lore/runtime_test.go`, `cmd/lore/lore/integration_test.go`).

## Phase 10: Slot = (Parameter, Value) — Type-Driven Fill and Dispatch

### Invariant

- A Node is a Go method call site. An Action wraps the Go method; the
  Node places that call site in the graph.
- **Nodes and subgraphs are interchangeable anywhere a reference is
  valid.** There is one kind — an executable unit — that resolves to
  either. Any API that takes a node reference must equally accept a
  subgraph reference; any API that takes a subgraph reference must
  equally accept a node reference. Node IDs and subgraph IDs share
  one ID space, resolved uniformly.
- Slots map one-to-one to the method's parameters. Slot identity =
  `op.Parameter` (Name + Go Type).
- **Values flowing through the graph are Go values.** `starlark.Value`
  never crosses the `bind` → `op` boundary. Conversion happens exactly
  once, at plan time, at the point of filling.
- Edges carry Go values. A promise is a handle to a Go value produced
  by an executable unit (node or subgraph) that will flow to a
  consumer's slot.
- Slot fill is binary:
  1. **Immediate** — a Go value set at plan time.
  2. **Promise** — a reference to an executable unit's output (node or
     subgraph), resolved to a Go value at execution.

No third slot mode. No starlark values in slots. No untyped
`map[string]any` on the provider API.

### Problem

1. `op.SlotValue` today is a tri-mode payload union keyed externally by
   name, carrying no `Parameter` identity. The authoritative
   `Parameter.Name` / `Parameter.Type` contract lives on `*op.Method`
   and never meets the value it governs.
2. `bind.Planner.dispatch` (`pkg/op/bind/planner.go:141-156`) explodes
   `Parameter{Name, Type}` into three parallel collections
   (`regularParams`, `knownKwargs`, `paramTypes`), then partially
   reassembles them — name-only on the general `FillSlot` path
   (`:221`), name + type only for the resource special case in
   `fillResourceSlot` (`:210`). Type integrity is broken at a
   data-structure level before any conversion runs.
3. The free `bind.FillSlot` (`pkg/op/bind/promise.go:302`) is dead code,
   broken by construction: its signature has no channel to the
   parameter's expected Go type, so it cannot verify or drive
   conversion against the slot's contract.
4. `bind.FillSlot`'s internal type switch over `starlark.Value` kinds
   is exactly the "suss out the intent" spaghetti the registry was
   built to prevent. `fillResourceSlot` compounds the problem by
   hard-coding "resource accepts string" inside the planner instead
   of letting resource types declare that capability themselves.
5. `Action.Do(ctx, slots map[string]any)` (`pkg/op/action.go:40`) is a
   framework-internal transport adapter that leaked into the provider
   API. Every provider has to be complicit in the map, cast `any` to
   expected Go types, and reimplement variadic/zero-value handling
   that `*op.Method` reflection already does.
6. The 32 remaining Phase 7 test failures are downstream symptoms of
   these defects. Triage against an incoherent model is noise; the
   model has to be made coherent first.

### Goals

1. `op.Slot = { Parameter op.Parameter; Value op.SlotValue }` — slot
   identity bound to payload.
2. `op.Node` carries both `Slots []*op.Slot` (ordered, matches
   `method.Parameters()` order) and `SlotsByName map[string]*op.Slot`
   (indexed). Slice for positional dispatch; map for name lookup in
   promise / edge wiring.
3. `op.SlotValue` collapses to **binary**:
   `Immediate any | Promise{NodeRef, Slot}`. The proxy mode
   (`GatherRef + Field`) does not survive — per-iteration bindings in
   gather comprehensions are expressed as ordinary promises emitted
   by iteration-step nodes.
4. **Registry-owned per-type conversion.** `ReceiverType` (or a sibling
   interface) exposes `FromStarlark(sv starlark.Value) (any, error)`.
   Primitives (string, int, bool, float, bytes) and composite targets
   (`[]T`, `map[K]V`, `time.Duration`, and struct-like resource
   targets) register under the same uniform interface. Types declare
   their own capabilities; `bind.FillSlot` never switches on
   `starlark.Value` kind.
5. `bind.FillSlot` becomes a thin graph-edge dispatcher. New signature
   takes `*op.Slot` (carrying the Parameter) plus the `starlark.Value`.
   Dispatch order:
   - `*Promise` → edge + promise-ref in slot.
   - list of `*Promise` → fan-in edges + indexed sub-slots.
   - `*receiver` → unwrap the Go instance, optional provenance edge,
     immediate in slot.
   - otherwise → delegate to `slot.Parameter.Type`'s registered
     converter to produce a Go value, immediate in slot.
6. `fillResourceSlot` (`pkg/op/bind/planner.go:365-432`) is **deleted**.
   String acceptance moves onto the resource type's converter; the
   planner stops special-casing resources.
7. `Planner.dispatch` stops exploding `Parameter`. It iterates
   `method.Parameters()` once, building `[]*op.Slot` directly.
8. **`Action.Do` is deleted from the public Action interface.**
   Providers are pure Go structs with plain Go methods. `Planned` and
   `Compensate` companions are discovered by reflection (Phase 9
   already does this). The framework's internal reflection dispatcher
   on `*op.Method` reads `Slot.Value` by `Parameter.Name`, assembles
   `reflect.Value` args positionally, invokes the Go method, and
   interprets the return tuple per `MethodKind`. **Nothing in the
   provider's surface contains a `map[string]any`.**
9. `(*op.Graph).Bind(ctx *ExecutionContext)` and
   `(*op.Node).Bind(method *Method)` rebind `Parameter.Type` from the
   registry on load, via `node.Receiver`. Slots serialize
   `Parameter.Name` + `Value` only; `Parameter.Type` is reattached at
   load. Mismatches (provider removed, method renamed, parameter
   renamed) surface as explicit load errors.

### Step outline

1. **Introduce `op.Slot`.** Redefine `op.Node.Slots` to `[]*Slot`; add
   `SlotsByName map[string]*Slot`. Update all `Set*Slot*` /
   `SlotByName` / `RequireStringSlot` / `ResolvedSlots`. Collapse
   `SlotValue` to `{ Immediate any, NodeRef string, Slot string }`;
   remove gather-proxy fields.
2. **Type-converter contract.** Add `FromStarlark(sv starlark.Value)
   (any, error)` (or equivalent) on `ReceiverType`, with
   implementations for primitive kinds and composites. Register
   primitive types in `ReceiverRegistry` at init; resource types
   already self-register.
3. **Rewrite `bind.FillSlot`.** New signature takes `*op.Slot` + `starlark.Value`.
   Graph-edge dispatch first; delegate to
   `slot.Parameter.Type`'s converter for everything else. Delete the
   dead free-function internals and its starlark-kind switch.
4. **Collapse `Planner.dispatch`.** Single pass over
   `method.Parameters()` producing `[]*op.Slot`. Delete
   `regularParams` / `knownKwargs` / `paramTypes` parallel maps.
   Delete `fillResourceSlot` entirely.
5. **Delete `Action.Do`.** Remove the method from the `Action` interface
   in `pkg/op/action.go`. Move reflection dispatch to
   `(*op.Method).Invoke(receiver any, slots map[string]*Slot) (result, complement any, err error)`
   (or similar) inside `pkg/op/method.go`. Update executor call sites
   and the dispatch cache accordingly.
6. **Retire the gather proxy.** Remove `GatherRef` / `Field` from
   `SlotValue`. Refactor gather execution to emit per-iteration
   promises from iteration-step nodes; consumer slots hold ordinary
   `NodeRef + Slot` promises. Update `ResolvedSlots` and the gather
   provider to match. See "Open question" below on scope.
7. **Rebind.** Implement `(*Node).Bind(method)` and
   `(*Graph).Bind(ctx)`. Update JSON / YAML round-trip so slots
   serialize `Parameter.Name` + `Value` only; `Parameter.Type` is
   reattached on load from the registry via `node.Receiver`.
8. **Provider update.** Delete every provider's `Do` boilerplate.
   Regenerate `*_gen.go` files under the new model. Hand-written
   provider methods stay untouched — they already look right.
9. **Executor update.** Gather, choose, recovery, and promise
   resolution consume `Slot` / `SlotValue` under the new model. Any
   `slot.Immediate.(T)` casts that survive replace with typed Slot
   accessors.
10. **Test triage.** Run the full suite. Most Phase 7 failures should
    resolve as side effects of the coherent model. Triage residual
    failures; fold legitimate bugs into follow-up fixes.

### Open question — gather / choose starlark surface

Intended semantics:

- **gather** — parallel comprehension over a list: for each element,
  execute a node or subgraph; collect results.
- **choose** — serial short-circuit over **chained** `(when, then)`
  cases. First match's `then` executes; rest are skipped. The `then`
  clause accepts a **node (Promise) or a subgraph handle directly**.
  No lambda.

The slot invariant (binary fill; no proxy mode) holds regardless of
which form the starlark surface takes. Reshaping the surface to match
these semantics is **out of scope for Phase 10**; it becomes a
follow-up phase with the following sub-decisions to resolve at that
time:

1. **How chained choose cases are expressed in starlark.** Kwargs
   cannot repeat, so `when=...`, `then=...` pairs cannot stack.
   Candidate forms:
   - positional list of `(predicate, node_or_subgraph)` tuples:
     `plan.choose((p1, n1), (p2, n2), otherwise=default)`;
   - dedicated case constructor consumed positionally:
     `plan.choose(plan.when(p1, n1), plan.when(p2, n2), plan.otherwise(default))`;
   - chained API:
     `plan.choose().when(p1, n1).when(p2, n2).otherwise(default)`.
2. **How a subgraph is constructed in starlark.** Starlark has no
   `with` blocks. Candidate forms:
   - `plan.subgraph(n1, n2, ...)` takes explicit node handles built
     eagerly in-place and bundles them;
   - `plan.sequence(n1, n2, ...)` ordered variant;
   - a single node handle is implicitly a single-node subgraph.
3. **Eager node-handle semantics.** Today, calling
   `plan.file.write_text(...)` appends a node to the main graph
   eagerly. Passing that node as `then = node` requires the planner
   to either (a) move it into a branch subgraph post-hoc, or
   (b) defer its main-graph attachment until it is known whether any
   choose claims it. The follow-up phase picks one.

Phase 10's proxy-retirement step is unaffected: gather per-iteration
bindings become ordinary promises emitted by iteration-step nodes
regardless of whether the starlark surface is reshaped.

### Blast radius

- `pkg/op` — `Node`, new `Slot` type, `SlotValue` shrink, `Method`
  dispatcher, `ReceiverType` converter contract, `ReceiverRegistry`
  primitive registration, `Graph.Bind` / `Node.Bind`, executor,
  recovery, serialization.
- `pkg/op/bind` — `FillSlot` rewrite, `Planner.dispatch` collapse,
  `fillResourceSlot` delete, `Promise` method signatures tighten.
- `pkg/op/provider/*` — every provider's `Do` boilerplate deleted;
  generated code regenerated; hand-written methods unchanged.
- Codegen — templates stop emitting `Do(ctx, slots map[string]any)`.
- Tests — any test reading `slot.Immediate.(T)` replaces with a typed
  accessor; any test building nodes via ad-hoc `map[string]any`
  slot-setting replaces with Slot-aware builders.

### Dependencies and ordering

- **Prerequisite for Phase 11.** Phase 11's rebind walk operates on the
  new slot model and piggybacks on `(*Node).Bind(method)` introduced
  here. Phase 11 cannot land first.
- **Partially subsumes Phase 8's `Do` work.** Phase 8's
  `ReceiverName` → `Name`, `ProviderType` → `Type`, and
  `executingReceiver` rewiring remain narrow and may land first as a
  separate PR. The `Do` → `Dispatch` rename becomes moot once `Do` is
  deleted from the Action interface.
- **Phase 9 Step 9 failure triage is paused** until this phase lands.
  The 32 failures are expected to resolve or sharpen under the new
  model.

## Phase 11: Implement `plan.subgraph` as a Flow Provider Method

### Problem

End users author graphs in starlark, not Go. The Phase 7 work made
`op.Subgraph` the Go-side universal execution unit, but did not ship
a starlark constructor. For any use case that needs a **multi-node**
executable unit — bundling several nodes into one branch body,
grouping nodes under a single iteration body, etc. — users have no
way to construct one.

Single-node cases do **not** need `plan.subgraph`: under the
node/subgraph interchangeability principle (see Phase 10 invariant),
a node handle is itself a valid executable-unit reference, so
`plan.flow.choose(when=predicate, then=some_node_promise)` works
directly. Phase 11 is specifically about the N-node bundling case.

### Goal

`plan.subgraph(n1, n2, …)` is a starlark-callable that constructs an
`op.Subgraph` from N node handles (and/or child subgraph handles) and
returns a handle usable anywhere a node handle is accepted.
Implemented as a method on the flow provider — reached via the plan
namespace's Planner routing as `plan.flow.subgraph(…)`, with no
special-case plumbing on `plan.Provider`.

### To resolve during design

- Signature in starlark: positional node handles bundled into a
  subgraph, a callable body, or another form — to be settled with
  the user during Phase 11 design.
- Routing: whether `plan.subgraph` dispatches directly on the plan
  provider, or is reached via the sub-namespace (`plan.flow.subgraph`)
  through planner routing. Ties into the separate question of whether
  `plan.choose` / `plan.gather` should themselves be namespaced as
  `plan.flow.*`.
- Eager-node reclamation: how content authored before the
  `plan.subgraph(…)` call is moved from the main execution path into
  the new subgraph (same concern as the open sub-decision about
  eager node-handle semantics under Phase 10).

### Dependencies

Follows Phase 10's slot model. Precedes follow-up work that reshapes
the `plan.choose` chained builder or the `plan.gather` parallel form
to consume subgraph handles.

## Phase 12: Address Defects on the Flow Provider

### Problem

The flow provider (`pkg/op/provider/flow/provider.go`) is the
execution-time backbone for choose, gather, and subgraph bodies. It
has accumulated defects during prior work that must be enumerated
and fixed before it can be trusted in that role.

### Known defects (non-exhaustive — user will add)

1. **`Gather` is a stub.** `flow/provider.go:134` returns
   `fmt.Errorf("gather: not yet implemented")`. The parallel
   comprehension semantics (per-item body, concurrency limit,
   ordered result collection) are not wired at all.
2. **`Choose`'s `then` is an execution-time subgraph ID string.** Per
   the node/subgraph interchangeability principle (Phase 10
   invariant), it must accept any executable-unit reference — node
   or subgraph. The internal lookup at `flow/provider.go:102`
   (`p.Graph.SubgraphByID(then)`) needs to become a polymorphic
   `ResolveByID` on the graph that returns either an `op.Node` or an
   `op.Subgraph` under a shared interface.
3. **`WaitUntil`'s `predicate func(any) (bool, error)`** cannot be
   populated from a plan-time starlark expression. A plan-time user
   has a Promise for a bool-producing node, not a Go `func`. The
   signature needs to accept a Promise/predicate-node-reference,
   with execution-time polling invoking that node's resolver.
4. **`Gather`'s `do string` subgraph ID** has the same execution-time
   specificity as `Choose`'s `then`. Same fix — accept any
   executable-unit reference.
5. **Plan-side hand-wired wrappers on `plan.Provider`.** Any
   remaining `plan.Provider.<flow_method>` exists only because the
   generic Planner routing cannot bind to the current flow
   signatures. Once (2)-(4) are fixed and Phase 11 ships, those
   wrappers disappear.

### Graph-side work implied

A single `(*op.Graph).ResolveByID(id string) (op.Executable, bool)`
(or equivalent) that returns either a `*Node` or a `*Subgraph`
behind a common `Executable` interface. Today `Graph.SubgraphByID`
is the only lookup; node lookup goes through the `Results` map at
execution time. Unifying these is prerequisite for (2) and (4).

### Dependencies

Precedes Phase 11 closure: once flow signatures accept executable
references, `plan.subgraph` slots cleanly into the same Planner
routing as every other flow method, and every `plan.Provider.<X>`
hand-wrapper can be deleted without leaving user-facing regressions.

## Phase 13: Catalog Serialization and Rebind Rehydration

### Problem

A graph is planned once and executed on many machines. The catalog is
the authoritative record of every resource the graph references —
discovery entries, shadowed outputs, implicit edges. Today the catalog
is stripped on serialize (`json:"-" yaml:"-"`) and not rebuilt on load.
`Rebind` sets the execution context and relinks nodes but does nothing
with the catalog. After load, the catalog is empty, preflight has
nothing to check, and the executor's post-dispatch shadowing has no
plan-time entries to transition.

### Approach: rebuild the catalog from slot values on Rebind

Rather than serialize the full catalog structure, rebuild it
deterministically from slot data at load time. Every resource-typed
slot value carries its URI in its `ResourceBase`, which **does** need
to survive the round trip. Every resource-producing node's output URI
can be reconstructed by calling the method's `Planned` spec with the
recorded input slot values.

`Rebind` walks the graph's nodes:

1. For each input slot holding a typed resource, reconstruct the
   resource from its URI (stored in `ResourceBase`) and route it
   through `catalog.Resolve` as a discovery entry. Existing shadows
   from later in the walk supersede earlier discoveries with the same
   URI.
2. For each node whose method has a `Planned` companion
   (`method.HasPlanned()`), call `method.Plan(receiver, args)` with the
   unmarshaled input slots to rebuild the pending resource and
   `catalog.Shadow(result, node.ID)`. If the companion returns
   `KnownAtExecution`, skip (the node will shadow at execution time
   as in the fresh-plan case).
3. Walk order matters: predecessors first, so when a consumer node
   resolves its inputs, the producer's shadow is already in place.

This keeps the catalog purely derivative of the node slots — no
separate serialized format to maintain, no schema drift risk. The
`Planned` siblings double as hydration functions.

### ResourceBase marshal/unmarshal

For the above to work, `ResourceBase.uri` needs to survive JSON/YAML
round-trip. Today `uri`, `id`, `originID` are unexported and don't
serialize. Custom `MarshalJSON` / `UnmarshalJSON` /
`MarshalYAML` / `UnmarshalYAML` on `ResourceBase` that emit the URI
(and optionally id/originID for debuggability, though Rebind regenerates
them) solves this.

### Blast area

- `pkg/op/resource.go` — custom marshal/unmarshal for `ResourceBase`
- `pkg/op/graph.go` — `Rebind` walks nodes and calls `method.Plan` for
  each node whose method has a `Planned` companion; new helper to walk
  in topological order
- `pkg/op/resource_catalog.go` — confirm `Resolve(typed Resource)`
  handles the "already shadowed" case correctly on rehydration (the
  catalog signature already takes a typed `Resource` — side work from
  Phase 9)
- Tests: round-trip a graph through YAML, verify the catalog is
  reconstructed, verify preflight runs correctly against the rebuilt
  catalog, verify post-dispatch shadowing matches on pending entries

### Ordering

Phase 13 depends on Phase 9 (`Planned` companions exist on every
resource-producing provider method, and the executor's post-dispatch
shadowing is wired) **and Phase 10** (slot model carries `Parameter`
identity; `(*Node).Bind(method)` exists to piggyback on). Within
Phase 13:

1. Implement `ResourceBase` custom marshal/unmarshal. Test round-trip
   at the resource level.
2. Implement `Rebind` walk that routes input slots through
   `catalog.Resolve`.
3. Extend the walk to call `method.Plan(receiver, args)` for nodes
   whose methods have `Planned` companions.
4. Verify preflight and post-dispatch shadowing work correctly after
   `Rebind` on a loaded graph.
5. Verify receipt loading (`internal/cli/receipts.go`) produces a
   fully-hydrated graph.

## What Remains After Phase 7

- Rewrite `resource.gen.go` files as `receiverFactory` + `Acquire`
- Remove `ResourceFactory`/`AnnounceResource`/`resourceRegistry`/
  `constructorRegistry`
- Sever starlark: remove `MarshalStarvalue`
- `pkg/op` minimal `go.starlark.net` imports (Thread on ExecutionContext)

## Goals

1. `pkg/op` starlark-free except for `Thread` on `ExecutionContext`
2. `pkg/op/bind` owns all starlark binding infrastructure
3. Single `ReceiverRegistry` for providers, resources, and their methods
4. Actions fully generated — one struct per method, correct interface per kind
5. Providers and Resources unified — same registry, same bridges
6. Generated receiver code in one file with inline params
7. No `Override` on receivers
8. Plan provider routes via `ResolveAttr` + registry lookup, no bind imports
9. `RuntimeEnvironment` interface — immutable operational constraints,
   read-only after execution begins, no mutable state leaks to providers
10. `Subgraph` is the universal execution unit — phases, branches, gather
    bodies all use the same type and the same executor path
11. `Run` returns a result — the terminal node's output, not just error/nil
12. Every slot binding is explicit — `DataRef` eliminates ambient slot filling

## Dependency Model

```
pkg/op/bind
  -> pkg/op                     (core types: Graph, Node, ExecutionContext, ...)
  -> go.starlark.net/starlark   (starlark runtime)

pkg/op
  -> go.starlark.net/starlark   (Thread on ExecutionContext)

pkg/op/provider/plan
  -> pkg/op                     (ProviderBase, Graph, Node, ReceiverRegistry)
  -> pkg/op/bind                (Promise)

pkg/op/provider/*/gen
  -> pkg/op                     (core types, ReceiverType)
  -> pkg/op/bind                (WrapProviderIn*Receiver, MethodParams)
  -> pkg/op/provider/*          (provider implementation)
```
