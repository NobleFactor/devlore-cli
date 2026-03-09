---
title: "BindingSet Redesign: Opt-In Providers and Module Loading"
status: draft
created: 2026-02-25
updated: 2026-02-25
---

# Plan: BindingSet Redesign

## Summary

Redesign `BindingSet` from opt-out (`Without()`) to opt-in (`With()`). Planned
providers aggregate into `plan` automatically but `plan` itself is not injected
unless requested. Immediate providers are never auto-injected — consumers either
opt in via `With()` or scripts import them via `load("@devlore//provider",
"provider")`. This introduces the `@devlore//` module namespace, a cached
thread loader, and a `Lifetime` field on `ProviderBinding` that declares
provider lifecycle semantics using a three-level model.

## Goals

1. **Opt-in immediate providers**: No immediate provider is injected as a global
   unless the consumer explicitly requests it via `With()` or the script imports
   it via `load()`
2. **`@devlore//` module loading**: Scripts can `load("@devlore//starlarkcode",
   "starlarkcode")` to import any registered provider on demand
3. **Provider lifetime declaration**: Each provider declares its lifecycle
   semantics — `stateless`, `phase`, or `session` — via a doc-comment directive
   on the Provider struct, enabling the runtime to manage caching, sharing, and
   cleanup correctly

## Current State

| Component         | Status                  | Notes                                     |
|-------------------|-------------------------|-------------------------------------------|
| `BindingSet`      | Opt-out via `Without()` | All providers included by default         |
| `Thread.Load`     | Not configured          | nil — `load()` statements fail at runtime |
| Provider lifetime | Undeclared              | No metadata about provider lifecycle      |
| Consumer control  | Coarse                  | Lore/Writ/Star all get the same globals   |

## Requirements

### Consumer Provider Sets

Each consumer declares what it needs:

| Consumer              | `With()`             | `load()` available       |
|-----------------------|----------------------|--------------------------|
| **star** (extensions) | `With("ui")`         | All registered providers |
| **lore**              | `With("ui", "plan")` | All registered providers |
| **writ**              | `With("ui", "plan")` | All registered providers |

Scripts in any consumer can import additional providers on demand:

```python
load("@devlore//starlarkcode", "starlarkcode")
load("@devlore//file", "file")

sources = starlarkcode.capture("**/*.star")
if file.exists("config.toml"):
    ui.note("Config found")
```

### Module Namespace: `@devlore//`

The `@devlore//` prefix identifies the devlore provider registry. The loader
resolves module names against registered `ProviderBinding` entries.

**Provider modules:**

```python
load("@devlore//starlarkcode", "starlarkcode")
load("@devlore//file", "file")
load("@devlore//git", "git")
```

**The `plan` aggregate:**

```python
load("@devlore//plan", "plan")
```

`plan` is special — it is not a single provider but an aggregate of all
registered `PlannedFactory` bindings. The loader constructs the `PlanRoot`
on first load, using the same factory aggregation that `BuildGlobals` uses
today.

### Provider Lifetime

Each `ProviderBinding` declares its lifecycle semantics via a `Lifetime` field.
This field is populated by the code generator from a `// +devlore:lifetime=`
directive on the Provider struct, following the same pattern as the existing
`// +devlore:access=` directive.

```go
// ProviderLifetime declares a provider's lifecycle semantics.
type ProviderLifetime string

const (
// LifetimeStateless providers hold no mutable state between calls.
// Safe to cache indefinitely and share across threads.
LifetimeStateless ProviderLifetime = "stateless"

// LifetimePhase providers accumulate state within a phase.
// Fresh instance per phase boundary. Cleanup between phases.
LifetimePhase ProviderLifetime = "phase"

// LifetimeSession providers persist across phases within a session.
// Single instance for the entire execution. Cleanup at session end.
LifetimeSession ProviderLifetime = "session"
)
```

```go
type ProviderBinding struct {
Name             string
Access           AccessType
Lifetime         ProviderLifetime
ActionRegistrar  ProviderRegistrar
PlannedFactory   PlannedFactory
ImmediateFactory ImmediateFactory
}
```

Both `Access` and `Lifetime` follow the same declaration model:

| Field      | Directive               | Values                          | Generator reads             | Generator emits                              |
|------------|-------------------------|---------------------------------|-----------------------------|----------------------------------------------|
| `Access`   | `// +devlore:access=`   | `immediate`, `planned`, `both`  | Provider struct doc comment | `Access: op.AccessImmediate` in `init()`     |
| `Lifetime` | `// +devlore:lifetime=` | `stateless`, `phase`, `session` | Provider struct doc comment | `Lifetime: op.LifetimeStateless` in `init()` |

#### Directive Declaration

The Provider struct is the single source of truth for both access and lifetime:

```go
// Provider performs Starlark source analysis.
// +devlore:access=immediate
// +devlore:lifetime=stateless
type Provider struct {
Root string
}
```

The code generator reads these directives and emits them into the generated
provider descriptor. The `init()` announces the provider; the framework
calls back to initialize it (see [Projected Provider API — Provider
Registration](../architecture/3.2-projected-provider-api.md#provider-registration)):

```go
type starlarkCodeProvider struct{}

func (p *starlarkCodeProvider) Name() string { return "starlarkcode" }

func (p *starlarkCodeProvider) NewImmediate(cfg op.BindingConfig) starlark.Value {
    provider := &Provider{}
    op.InitProvider(provider, op.Context{Root: root, RecoverySite: site})
    return NewStarlarkCodeReceiver(provider)
}

func init() {
    op.Announce(&starlarkCodeProvider{})
}
```

When the `// +devlore:lifetime=` directive is omitted, the generator defaults
to `LifetimeStateless`. This requires no changes to existing provider structs.

#### Stateless Providers (`LifetimeStateless`)

The default. The common case.

A stateless provider holds no mutable state between calls. Its
`ImmediateFactory` produces a value that is a pure function of `BindingConfig`.

**Contract:**

- Idempotent construction: same `cfg` → functionally identical receivers
- Safe to cache indefinitely within a `BindingSet`
- Safe to share across threads without synchronization
- No cleanup required between phases or at execution end
- Call-order independent: result of method B never depends on method A

**Examples:**

| Provider           | Why stateless                                                           |
|--------------------|-------------------------------------------------------------------------|
| `starlarkcode`     | Holds a root path. Analysis methods read files and return fresh results |
| `ui`               | Holds a writer reference. Each call writes independently                |
| `template`         | Holds no state. Renders from inputs each time                           |
| `file` (immediate) | Queries like `exists()` read filesystem on each call                    |

**Current providers:** All existing immediate providers are stateless.

#### Phase-Scoped Providers (`LifetimePhase`)

A phase-scoped provider accumulates state within a single phase execution. Its
receiver is an active participant in the phase, not a passive lens.

**Contract:**

- Independent construction: two calls to `ImmediateFactory(cfg)` produce
  independent instances with independent state
- NOT safe to share across phases
- The loader creates a fresh instance at each phase boundary
- If the receiver implements `io.Closer`, the runtime calls `Close()` at
  phase end
- Call-order dependence: result of method B may depend on method A

**Examples (hypothetical — none exist today):**

| Provider      | Would hold          | Why phase-scoped                                         |
|---------------|---------------------|----------------------------------------------------------|
| Log collector | `[]LogEntry` buffer | Entries accumulate within a phase, flushed at phase end  |
| Transaction   | `*sql.Tx`           | Transaction spans one phase; commit/rollback at boundary |
| Metrics batch | Counter maps        | Counters increment within a phase; reported at phase end |

#### Session-Scoped Providers (`LifetimeSession`)

A session-scoped provider persists across phases within a single execution
session. It is created once and shared across all phases.

**Contract:**

- Single instance for the lifetime of the `BindingSet`
- Safe to share across phases but NOT across concurrent threads unless
  the provider implements its own synchronization
- If the receiver implements `io.Closer`, the runtime calls `Close()` at
  session end (after all phases complete)
- May accumulate state across phases — this is the intended use case

**Examples (hypothetical — none exist today):**

| Provider      | Would hold                  | Why session-scoped                                     |
|---------------|-----------------------------|--------------------------------------------------------|
| Database pool | `*sql.DB` connection pool   | Connection reuse across phases; close at session end   |
| HTTP client   | `*http.Client` with cookies | Session state accumulates across phases                |
| Audit logger  | Append-only log             | Records events across all phases; flush at session end |

#### Cache Behavior by Lifetime

| Lifetime            | Cache behavior                                                                       |
|---------------------|--------------------------------------------------------------------------------------|
| `LifetimeStateless` | Cached for the lifetime of the `BindingSet`. Never cleared.                          |
| `LifetimePhase`     | Cached within a phase. Cleared at phase boundaries. `Close()` called if implemented. |
| `LifetimeSession`   | Cached for the lifetime of the `BindingSet`. `Close()` called at session end.        |

#### Why Declare Lifetime?

1. **Correctness**: A phase-scoped provider shared across phases would silently
   corrupt state. The declaration prevents this at the infrastructure level,
   not through developer discipline.

2. **Resource management**: Phase-scoped and session-scoped providers that
   implement `io.Closer` get cleanup calls at the appropriate boundaries.
   Without the declaration, the runtime has no way to know when to clean up.

3. **Cache safety**: The loader caches aggressively for stateless providers
   (same instance forever), conservatively for phase-scoped (fresh per phase),
   and carefully for session-scoped (shared but cleaned up at end). Without the
   declaration, the loader must treat everything as phase-scoped (pessimistic)
   or everything as stateless (optimistic and dangerous).

4. **Documentation**: The directive is self-documenting. A developer reading
   `// +devlore:lifetime=phase` immediately understands the lifecycle semantics.

5. **Static analysis**: A linter can flag a provider that declares
   `lifetime=stateless` but holds mutable fields — likely a bug. A provider
   that declares `lifetime=phase` but holds only immutable config — likely
   over-conservative.

6. **Future optimization**: When the runtime gains parallel phase execution,
   stateless providers participate in shared caches across goroutines.
   Phase-scoped providers get per-goroutine instances. Session-scoped providers
   need synchronization. The declaration is the gate — zero changes to provider
   code required.

#### Choosing the Right Lifetime

```
Does ImmediateFactory produce a receiver that is a pure function of BindingConfig?
  │
  ├─ YES → Does the receiver hold mutable fields that change across calls?
  │   │
  │   ├─ NO  → LifetimeStateless
  │   │
  │   └─ YES → Does correctness depend on those fields persisting?
  │       │
  │       ├─ NO (just a perf cache) → LifetimeStateless
  │       │
  │       └─ YES → Must the state persist across phase boundaries?
  │           │
  │           ├─ NO  → LifetimePhase
  │           │
  │           └─ YES → LifetimeSession
  │
  └─ NO (factory has side effects) → Must the side effects persist across phases?
      │
      ├─ NO  → LifetimePhase
      │
      └─ YES → LifetimeSession
```

When in doubt, declare `lifetime=phase`. The cost is per-phase recreation,
which is cheap. The risk of incorrectly declaring `stateless` is shared mutable
state across phases — a correctness bug that may be subtle and intermittent.

### BindingSet API

```go
// BindingSet selects which provider bindings a consumer uses and builds
// Starlark globals from them.
type BindingSet struct {
cfg      op.BindingConfig
included map[string]bool
cache    map[string]*loaderEntry
}

// NewBindingSet creates a BindingSet with the given configuration.
// No providers are included by default.
func NewBindingSet(cfg op.BindingConfig) *BindingSet

// With includes one or more providers as pre-injected globals.
// "plan" is a special name that includes the PlanRoot aggregate.
// Returns the BindingSet for chaining.
func (bs *BindingSet) With(names ...string) *BindingSet

// BuildGlobals constructs the Starlark globals dict.
// Only providers named in With() appear as globals.
func (bs *BindingSet) BuildGlobals(graph *op.Graph, project string,
reg *op.ActionRegistry) starlark.StringDict

// ConfigureThread sets thread.Load to the @devlore// module loader.
// The loader resolves provider names from the binding registry and
// caches instances. Must be called before ExecFileOptions.
func (bs *BindingSet) ConfigureThread(thread *starlark.Thread,
graph *op.Graph, project string, reg *op.ActionRegistry)

// RegisterActions registers all providers' actions with the registry.
// This is unchanged — all providers' actions are always registered
// regardless of With() selections.
func (bs *BindingSet) RegisterActions(reg *op.ActionRegistry)
```

### Thread Loader

```go
type loaderEntry struct {
globals starlark.StringDict
err     error
}

func (bs *BindingSet) makeLoader(graph *op.Graph, project string,
reg *op.ActionRegistry) func (*starlark.Thread, string) (starlark.StringDict, error) {

return func (_ *starlark.Thread, module string) (starlark.StringDict, error) {
if !strings.HasPrefix(module, "@devlore//") {
return nil, fmt.Errorf("unknown module: %s (use @devlore// prefix)", module)
}

name := strings.TrimPrefix(module, "@devlore//")

if e, ok := bs.cache[name]; ok {
return e.globals, e.err
}

globals, err := bs.resolveProvider(name, graph, project, reg)
bs.cache[name] = &loaderEntry{globals, err}
return globals, err
}
}

func (bs *BindingSet) resolveProvider(name string, graph *op.Graph,
project string, reg *op.ActionRegistry) (starlark.StringDict, error) {

// Special case: plan aggregate
if name == "plan" {
return bs.buildPlanModule(graph, project, reg)
}

binding, ok := op.BindingByName(name)
if !ok {
return nil, fmt.Errorf("no provider %q registered", name)
}

if binding.ImmediateFactory == nil {
return nil, fmt.Errorf("provider %q has no immediate factory", name)
}

value := binding.ImmediateFactory(bs.cfg)
return starlark.StringDict{name: value}, nil
}
```

### Load Semantics

`load()` is processed during `starlark.ExecFileOptions()`. Functions defined
in the script close over the module scope, which includes loaded names. When
a function is later invoked via `starlark.Call()`, loaded names are accessible
through the closure. No special handling is needed.

```python
load("@devlore//starlarkcode", "starlarkcode")

def install(package, phase):
    # starlarkcode is accessible here via module scope closure
    sources = starlarkcode.capture("**/*.star")
    report = sources.analyze(hotspots=True)
```

The loader cache lives on the `BindingSet`. For the current architecture
(fresh thread per phase, Option A), this means:

- Same `BindingSet` reused across phases → cache persists, stateless and
  session-scoped providers shared; phase-scoped providers cleared
- New `BindingSet` per phase → cache fresh, all providers re-created

Option A is the starting point. Cache promotion to cross-thread sharing is a
future optimization gated by the `Lifetime` declaration.

## Implementation Phases

### Phase 1: ProviderLifetime Type and ProviderBinding.Lifetime Field

- [ ] Add `ProviderLifetime` type and constants to `pkg/op/`
- [ ] Add `Lifetime ProviderLifetime` to `op.ProviderBinding`
- [ ] Update `RegisterBinding` merge logic for `Lifetime` field
- [ ] Verify all existing registrations work unchanged (zero value = stateless)
- [ ] No behavioral changes — declaration only

**Files:**

- `pkg/op/lifetime.go` — Create: `ProviderLifetime` type, constants, doc
- `pkg/op/binding_registry.go` — Modify: add `Lifetime` field, merge logic

### Phase 2: Code Generator Support

- [ ] Update the generator to parse `// +devlore:lifetime=` directives
- [ ] Emit `Lifetime: op.LifetimeStateless` (or `Phase`/`Session`) into
  generated `init()` registrations
- [ ] Default to `LifetimeStateless` when directive is omitted
- [ ] Validate directive values: `stateless`, `phase`, `session`

**Files:**

- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` — Modify: parse lifetime directive

### Phase 3: BindingSet Redesign

- [ ] Replace `excluded map` with `included map`
- [ ] Replace `Without()` with `With()`
- [ ] Update `BuildGlobals` to only include `With()` providers
- [ ] Handle `"plan"` as a special `With()` name
- [ ] Update tests

**Files:**

- `internal/starlark/binding_set.go` — Modify: With(), included map, BuildGlobals
- `internal/starlark/binding_set_test.go` — Modify: test With() semantics

### Phase 4: Thread Loader

- [ ] Implement `ConfigureThread` on BindingSet
- [ ] Implement `makeLoader` with `@devlore//` prefix dispatch
- [ ] Implement loader cache (`loaderEntry` map)
- [ ] Cache behavior respects `Lifetime`: stateless cached forever,
  phase-scoped cleared at boundaries, session-scoped cached forever
  with cleanup tracking
- [ ] Handle `@devlore//plan` as special case (PlanRoot construction)
- [ ] Add error handling for unknown modules and missing factories
- [ ] Add tests for loader: valid provider, plan aggregate, unknown provider,
  cache hit, lifetime-aware caching

**Files:**

- `internal/starlark/binding_set.go` — Modify: add ConfigureThread, makeLoader
- `internal/starlark/loader.go` — Create: loader implementation and cache types
- `internal/starlark/loader_test.go` — Create: loader tests

### Phase 5: Consumer Migration

- [ ] Update `internal/lore/builder.go`: `With("ui", "plan")` + `ConfigureThread`
- [ ] Update `internal/lore/commands.go`: `With("ui", "plan")`
- [ ] Update `internal/writ/commands.go`: `With("ui", "plan")`
- [ ] Star extensions use `With("ui")` where applicable
- [ ] Remove all `Without()` calls (method no longer exists)
- [ ] Verify `load()` works in phase scripts

**Files:**

- `internal/lore/builder.go` — Modify: With() + ConfigureThread
- `internal/lore/commands.go` — Modify: With()
- `internal/writ/commands.go` — Modify: With()

### Phase 6: Integration Test

- [ ] End-to-end test: execute a `.star` script that uses `load("@devlore//starlarkcode")`
- [ ] Verify `With("ui")` pre-injects ui but not other providers
- [ ] Verify `load("@devlore//plan", "plan")` works
- [ ] Verify unknown module produces clear error
- [ ] Verify cache deduplication (load same provider twice, same instance)

**Files:**

- `internal/starlark/integration_test.go` — Create: end-to-end load tests

## Migration Path

All consumers switch from `Without()` to `With()`. The change is mechanical:

**Before:**

```go
bs := loreStar.NewBindingSet(op.BindingConfig{
Writer:      os.Stdout,
ProgramName: "lore",
Color:       true,
})
globals := bs.BuildGlobals(graph, pkg.Name, reg)
```

**After:**

```go
bs := loreStar.NewBindingSet(op.BindingConfig{
Writer:      os.Stdout,
ProgramName: "lore",
Color:       true,
}).With("ui", "plan")
globals := bs.BuildGlobals(graph, pkg.Name, reg)
bs.ConfigureThread(thread, graph, pkg.Name, reg)
```

Scripts that previously relied on auto-injected immediate providers (other
than `ui`) must add `load()` statements. Since no scripts currently use
providers other than `ui` and `plan` as globals, this is a no-op migration.

## Files to Create/Modify

| File                                            | Action | Purpose                                                         |
|-------------------------------------------------|--------|-----------------------------------------------------------------|
| `pkg/op/lifetime.go`                            | Create | `ProviderLifetime` type and constants                           |
| `pkg/op/binding_registry.go`                    | Modify | Add `Lifetime` field to `ProviderBinding`, merge logic          |
| `star/extensions/.../generate.star`             | Modify | Parse `// +devlore:lifetime=` directive, emit into registration |
| `internal/starlark/binding_set.go`              | Modify | With(), included map, ConfigureThread                           |
| `internal/starlark/loader.go`                   | Create | @devlore// loader, lifetime-aware cache, plan resolution        |
| `internal/starlark/loader_test.go`              | Create | Loader unit tests                                               |
| `internal/starlark/binding_set_test.go`         | Modify | Test With() semantics                                           |
| `internal/starlark/integration_test.go`         | Create | End-to-end load tests                                           |
| `internal/lore/builder.go`                      | Modify | With("ui", "plan") + ConfigureThread                            |
| `internal/lore/commands.go`                     | Modify | With("ui", "plan")                                              |
| `internal/writ/commands.go`                     | Modify | With("ui", "plan")                                              |
| `docs/architecture/3.1-provider-loading.md` | Modify | Update for three-level lifetime model                           |

## Related Documents

- [Projected Provider API](../architecture/3.2-projected-provider-api.md)
- [Phase Execution](../architecture/2.2-phase-execution.md)
- [Provider Loading and Statefulness](../architecture/3.1-provider-loading.md)
- [Star Source Analysis Plan](./star-source-analysis.md)

## Open Questions

- [ ] Should `RegisterActions` also become opt-in? Currently all providers'
  actions are registered regardless of `With()`. This seems correct — the
  action registry is for the executor, not the script environment.
