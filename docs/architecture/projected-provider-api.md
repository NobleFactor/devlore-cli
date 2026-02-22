# The Projected Provider API

## Core Idea

A **Provider** is a Go struct with methods that implement a capability domain
(files, packages, networking, etc.). Each Provider is *projected* into two
Starlark namespaces — **Immediate** and **Planned** — from a single source of
truth. The projection layer handles the mechanical differences between the two
modes so that Provider authors write business logic once.

```
                    ┌──────────────┐
                    │   Provider   │  One Go struct, hand-coded
                    │  (file.go)   │
                    └──────┬───────┘
                           │
                    code generation
                           │
              ┌────────────┼────────────┐
              ▼                         ▼
     ┌────────────────┐       ┌────────────────┐
     │   Immediate    │       │    Planned      │
     │   Projection   │       │   Projection    │
     │                │       │                 │
     │ file.exists()  │       │ plan.file.link()│
     │ → bool now     │       │ → Output handle │
     └────────────────┘       └─────────────────┘
```

**Immediate** methods execute during script evaluation and return concrete
values. They drive control flow — `if`, `for`, decisions.

**Planned** methods defer execution. They record an intent node in the
execution graph and return an `Output` handle — a promise that carries the
node's identity and can wire dependencies between nodes.

## The Three Generated Artifacts

Code generation reads the Provider struct and its doc-comment directives, then
produces three distinct artifacts from three templates:

| Artifact | Template | Purpose |
|----------|----------|---------|
| **Action wrappers** | `graph_actions.go.template` | Implements the `Action` interface: unpacks slots, calls Provider method, returns result + compensation state |
| **Plan receiver** | `plan_receiver.go.template` | Starlark namespace under `plan.*`: creates `Node`, fills slots, returns `Output` |
| **Realtime receiver** | `realtime_receiver.go.template` | Starlark namespace at top level: calls Provider method directly, returns value |

The action wrappers bridge Provider methods to the execution engine. The two
receivers bridge Provider methods to the Starlark script environment. All three
are generated from the same Provider struct — they cannot drift.

## The Promise Model

The planned projection doesn't just defer execution — it builds a dependency
graph through the values that flow between calls.

```python
# Starlark script
src = plan.file.source("config.toml")        # → Output (node A)
plan.file.copy(src, path="/etc/app.toml")     # → Output (node B, edge A→B)
```

When `src` (an `Output` from node A) is passed as an argument to `copy`, the
projection layer recognizes it as a promise. Instead of storing a concrete
value, it records a *slot promise* — "this input will be provided by node A's
output" — and creates an edge `A → B` in the graph. The executor later
resolves these promises by passing each node's output to its dependents.

Three slot value types make this work:

| Slot type | Set when | Resolved when |
|-----------|----------|---------------|
| **Immediate** | Argument is a concrete Starlark value (string, int, bool, list, dict) | Available immediately — stored directly in the node |
| **Promise** | Argument is an `Output` — the result of another planned call | At execution time — the producing node runs first, its result fills the slot |
| **Proxy** | Argument references a field within a `Gather` iteration | At execution time — the gather node fans out, each item fills the slot |

The `FillSlot` function encapsulates this dispatch. Generated plan receivers
call it for every argument; it examines the Starlark value type and sets the
appropriate slot variant. This is how the graph builds itself from the natural
flow of the script — no explicit edge declarations needed.

### Gather — Parallel Fan-Out

`Gather` collects multiple `Output` handles into a single value that represents
parallel completion:

```python
a = plan.pkg.install("go")
b = plan.pkg.install("node")
both = plan.gather(a, b)
plan.shell.run("go version && node --version", after=both)
```

When a `Gather` fills a slot, it creates edges from *all* gathered nodes to the
consumer — the consumer waits for every member to complete.

### Output Attributes

`Output` handles expose attributes back to Starlark:

- `node_id` — the unique identifier (useful for debugging)
- `slot` — which output of the node this handle represents
- `retry(max_attempts=, backoff=, ...)` — configures retry policy on the node
- Dynamic slot access — any slot name defined on the node

## Two Registration Paths

Providers register through two parallel systems that converge at runtime:

### 1. Action Registration (Execution Binding)

Each provider's generated `actions_gen.go` contains:
- One wrapper struct per method, implementing the `Action` interface
- An `init()` function that calls `execution.RegisterProvider(Register)`
- A `Register` function that instantiates the Provider and registers all actions

At startup, blank imports in `register.go` trigger all `init()` functions. Then
`RegisterAllProviders(reg)` executes every registrar, populating the
`ActionRegistry` with name → Action mappings (`"file.link"` → `&Link{Impl: p}`).

### 2. Plan Registration (Starlark Binding)

Each provider's generated `plan_*_gen.go` contains:
- A plan struct (e.g., `FilePlan`) embedding `projection.Receiver`
- An `init()` function that calls `registerPlan("file", factory)`
- A factory function that creates the plan struct with graph and registry references

When `PlanRoot` is constructed, it iterates the plan registry and calls each
factory, building the `plan.file`, `plan.net`, etc. sub-namespaces dynamically.

### The Connection

Both registrations happen via `init()` — self-registration triggered by import.
The plan receiver references the action registry to resolve action names:

```go
node := &projection.Node{
    ID:     projection.GenerateNodeID("file-link"),
    Action: p.reg.MustGet("file.link"),  // ← looks up the Action wrapper
}
```

This ensures the plan node carries the same `Action` implementation that the
executor will call. The action registry is the single source of truth for what
"file.link" means at execution time.

## The Projection Layer

The infrastructure that makes projection work — `Receiver`, `Output`,
`FillSlot`, type conversions, node ID generation — lives in `pkg/projection/`.
It has no knowledge of specific providers, specific actions, or the execution
engine's internals.

### Type Ownership

`pkg/projection/` owns the concrete graph data model types directly:

- **`Node`** — graph node with ID, slots, status, and an `Action` field typed
  as `any`. The executor type-asserts `node.Action.(Action)` before calling Do.
  `ActionName()` uses duck-type assertion `interface{ Name() string }`.
- **`Graph`** — collection of nodes and edges with serialization, checksums,
  state tracking.
- **`Edge`**, **`SlotValue`** — graph connectivity and slot value types.
- **`Phase`**, **`RetryPolicy`** — execution grouping and retry configuration.
- **`Output`**, **`Gather`** — Starlark value types that represent graph nodes.
- **`FillSlot`** — dispatches slot values based on Starlark argument type.
- **`Receiver`** — base type for generated Starlark namespace receivers.

Since Output, Gather, and FillSlot operate on `*Node` and `*Graph` in the same
package, no interfaces are needed. The projection layer works with concrete
types.

### Standalone Execution Functions

Two operations that need execution-layer types live as standalone functions in
`internal/execution/`:

- **`HydrateGraph(g *projection.Graph, reg *ActionRegistry)`** — replaces stub
  actions on deserialized graph nodes with real Action implementations from the
  registry.
- **`ApplyResults(g *projection.Graph, results []*NodeResult)`** — updates graph
  nodes with execution outcomes.

This prevents `pkg/projection/` from importing `internal/execution/`.

### Package Boundary

```
pkg/projection/          ← public, no internal/ imports
  graph.go               ← Node, Graph, Edge, SlotValue, state/status types
  phase.go               ← Phase, RetryPolicy, BackoffStrategy
  output.go              ← Output, Gather, FillSlot
  receiver.go            ← Receiver base type, MakeAttr, helpers
  convert.go             ← Starlark ↔ Go type conversions
  access.go              ← Access level constants
  nodeid.go              ← GenerateNodeID (atomic counter)

internal/execution/      ← owns Action interface, executor, registry
  graph.go               ← HydrateGraph(), ApplyResults()
  action.go              ← Action, CompensableAction, Context
  executor.go            ← GraphExecutor (type-asserts node.Action)
  recovery.go            ← RecoveryStack (node: *projection.Node)

internal/starlark/       ← imports both; devlore-specific wiring
  plan_root.go           ← PlanRoot (choose, gather, source)
  plan_registry.go       ← PlanFactory type and registry
  plan_*_gen.go          ← generated plan receivers
  receiver_*_gen.go      ← generated realtime receivers
```

## The User Experience

The projection model gives script authors a simple mental model: the prefix
tells you *when* something happens.

| Syntax | When | Returns | Use for |
|--------|------|---------|---------|
| `pkg.is_installed("go")` | Now, during script evaluation | `True` / `False` | Control flow, decisions |
| `plan.pkg.install("go")` | Later, during graph execution | `Output` handle | Declaring desired state |

```python
# Immediate: query current state to make decisions
if not pkg.is_installed("golang"):
    note("Go not found — will install")

# Planned: declare desired state, let the engine sort out order
go = plan.pkg.install("golang")
plan.file.copy("env.sh", dest="/etc/profile.d/", after=go)

# Conditional: combine both projections
if host.is_macos():
    plan.pkg.install("coreutils")
```

The "Double-Check" bug — writing code to check *if* something exists and then
separate code to *make* it exist — disappears. The immediate projection
queries state; the planned projection declares intent. They compose naturally
because they're projections of the same underlying capability.

## Access Directives

Doc-comment directives on Provider methods control which projection surfaces
each method appears on:

| Directive | Immediate | Planned | Use case |
|-----------|-----------|---------|----------|
| `//devlore:plannable` | — | Yes | State-changing actions (install, link, copy) |
| *(none)* | Yes | — | Queries and facts (exists, is_installed) |

The current system uses struct-level `//devlore:plannable` as a binary flag.
Constants in `pkg/projection/access.go` prepare for future method-level
granularity:

```go
const (
    AccessImmediate = "immediate"  // query only — no graph node
    AccessPlanned   = "planned"    // graph node only — no immediate call
    AccessBoth      = "both"       // available in both projections
)
```

Method-level `//devlore:access=immediate|planned|both` would let a single
Provider struct contain both query methods and action methods, with the
generator routing each method to the correct projection surface.

## Consumers

Three tools consume the projection infrastructure. Each interacts with a
different stage of the Provider lifecycle.

### star — Code Generator

Star is the code generation tool. Its Actions extension reads Provider structs
via Go reflection, detects `//devlore:plannable` directives, and renders the
three templates into generated code.

```
star devlore actions generate
    ↓
    reads Provider struct (e.g., file.Provider)
    ↓
    ├→ graph_actions.go.template    → provider/file/actions_gen.go
    ├→ plan_receiver.go.template    → starlark/plan_file_gen.go
    └→ realtime_receiver.go.template → starlark/receiver_file_gen.go
```

Star does not import `pkg/projection/` at generation time — it *emits code*
that imports it. The templates reference `projection.Receiver`,
`projection.FillSlot`, `projection.NewOutput`, etc. as literal strings that
appear in the generated Go source. Star's relationship to the projection layer
is authorial: it writes the code that uses projection, but never calls
projection functions itself.

### lore — Package Deployer

Lore installs packages defined in `packages-manifest.yaml`. It evaluates
Starlark install scripts that build an execution graph through the planned
projection.

```
lore deploy @packages-manifest.yaml
    ↓
    ActionRegistry ← provider.RegisterAll()     (execution binding)
    PlanRoot ← NewPlanRoot(graph, host, reg)     (Starlark binding)
    ↓
    execute install.star with plan as global
    ↓
    plan.pkg.install("go")      → Node + Output
    plan.file.link(src, path)   → Node + Output + Edge
    ↓
    graph complete → GraphExecutor.Run()
```

Lore uses both projection surfaces. The `plan.*` namespace (planned projection)
builds the graph. Realtime receivers like `note()`, `warn()`, and `fail()`
(immediate projection) provide feedback during script evaluation. Lore's
`builder.go` creates the `PlanRoot`, wires it into the Starlark globals, and
executes phase entry points (`install`, `provision`, `verify`).

### writ — Environment Orchestrator

Writ deploys entire environments — dotfiles, configurations, scripts, templates
— by walking source directories and creating file operation nodes. When it
encounters a `packages-manifest.yaml` in the source tree, it delegates to
Lore's `Planner` for package resolution.

```
writ deploy personal
    ↓
    ActionRegistry ← provider.RegisterAll()
    DeployGraphBuilder with injected lore.Planner
    ↓
    BuildTree() walks source dirs
    ├→ file.link nodes (symlinks)
    ├→ template.render nodes
    ├→ encryption.decrypt nodes
    └→ manifest found → Planner.PlanPackages(graph, path)
                            ↓
                            Starlark script evaluation (same as lore)
    ↓
    graph complete → GraphExecutor.Run()
```

Writ builds most of its graph imperatively — calling `projection.Node{}`
constructors directly for file operations discovered by tree walking. It only
enters the Starlark projection layer when it delegates package installation to
Lore's Planner. This makes Writ a hybrid consumer: it uses the execution layer
directly for its own concerns and the projection layer (via Lore) for package
management.

### Consumer Topology

```
                    pkg/projection/
                    (Node, Graph, Receiver, Output, FillSlot, convert, nodeid)
                         │
            ┌────────────┼────────────────┐
            │            │                │
         emits        imports          imports
         code           │             (via lore)
         that           │                │
         uses it        │                │
            │            │                │
          star          lore            writ
       (generator)   (packages)    (environments)
            │            │                │
            │            └───────┬────────┘
            │                    │
            │            internal/execution/
            │            (Action, ActionRegistry, GraphExecutor,
            │             HydrateGraph, ApplyResults)
            │                    │
            └────────────────────┘
                    generated code lives here
                    (plan_*_gen.go, actions_gen.go)
```

Star produces the generated code that both Lore and Writ consume. Lore
evaluates Starlark scripts through the projection layer to build graphs. Writ
builds graphs mostly imperatively but delegates to Lore's Planner when it
finds package manifests. All three tools share the execution layer — the
`ActionRegistry`, `HydrateGraph`, and `GraphExecutor` — as the common substrate.

## Design Properties

**No code duplication.** One Go method powers up to three generated artifacts
(action wrapper, plan receiver method, realtime receiver method). Business
logic exists in exactly one place.

**Safety by design.** A method marked `planned` cannot be called immediately —
the generator simply doesn't emit it in the realtime receiver. A method with no
`plannable` directive cannot create graph nodes — the generator doesn't emit it
in the plan receiver. Misuse is a compile error, not a runtime surprise.

**Automatic dependency wiring.** Passing an `Output` handle as an argument
creates a graph edge. The script author never writes explicit dependency
declarations — dependencies emerge from data flow.

**Decoupled projection.** The projection layer (`pkg/projection/`) knows
nothing about files, packages, or services. It knows about nodes, slots, edges,
and Starlark values. New providers plug in without touching projection code.

**Zero back-imports.** `pkg/projection/` imports only stdlib +
`go.starlark.net` + `gopkg.in/yaml.v3`. It never imports from `internal/`.
The execution layer imports projection; projection never imports execution.

**Self-registering providers.** Adding a new provider requires writing the Go
struct and running the generator. The `init()` pattern ensures the provider is
available everywhere its package is imported — no central registration manifest
to maintain.
