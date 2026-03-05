# Phase 6: Provider Resource Types, Context Injection, and Method Migration

## Context

Phases 0–5 and 8 are complete. Phase 7 (master plan numbering) is the last
remaining piece of resource management: create resource types for the git,
service, and pkg providers; embed `ProviderBase` in all providers that lack
it; inject execution context at action dispatch time; remove the leaked
`output io.Writer` parameter from method signatures; and migrate compensable
methods to typed Tombstones.

The file provider is the reference implementation — it has `file.Resource`,
`file.Tombstone`, `ProviderBase` embedding, and constructor registration.
This phase replicates that pattern across the remaining providers.

## What will be true when this phase is complete

1. **Every stateful provider embeds `op.ProviderBase`** — git, service, pkg,
   shell all embed it. They access Platform, Writer, and other execution
   state through `p.Context()` instead of per-method parameters or
   directly-held fields.

2. **Three new resource types exist** — `git.Resource` (scheme `git://`),
   `service.Resource` (scheme `svc://`), `pkg.Resource` (scheme `pkg://`).
   Each embeds `op.ResourceBase`, implements `URI()/Scheme()/Host()/Path()`,
   and registers both execution-time and plan-time constructors.

3. **Three new tombstone types exist** — `git.Tombstone`, `service.Tombstone`,
   `pkg.Tombstone`. Each embeds `op.TombstoneBase`. Compensate methods accept
   typed tombstones instead of `any`/`map[string]any`.

4. **`output io.Writer` is gone from all method signatures** — git, service,
   and shell providers no longer take `output` as a parameter. They read
   `p.Context().Writer` instead. The `Params` maps no longer list `"output"`.

5. **`Platform` is gone from provider structs** — service and pkg providers
   no longer hold `Platform *op.Platform` directly. They access it via
   `p.Context().Platform`. In immediate mode, the ImmediateFactory creates
   a ProviderBase with a partial Context. In action mode, the ActionRegistrar
   creates the provider with the full execution Context.

6. **Provider instances are per-graph** — `ActionRegistrar` signature changes
   to `func(reg *ActionRegistry, ctx Context)`. The executor creates a fresh
   ActionRegistry per graph, calling each registrar with the full execution
   Context. Each provider gets its own instance scoped to the graph.

7. **`BindingConfig` includes `Platform`** — for immediate-mode providers
   that need platform access. The ImmediateFactory maps `cfg.Platform` into
   the ProviderBase's Context (not via `+devlore:bind`).

8. **pkg inputs are `[]pkg.Resource`** — `Install`, `Remove`, `Upgrade`
   accept `[]pkg.Resource` instead of `[]string`. Each package is a tracked
   resource with a URI (`pkg://brew/htop`).

9. **All generated code is regenerated** — `params.gen.go`, `immediate.gen.go`,
   `planned.gen.go`, `actions.gen.go`, and `actions_test.gen.go` for all
   affected providers reflect the new signatures.

10. **`make check` passes** — build, vet, lint, and all tests green.

## Design Decisions

### D1: Platform and Writer via ProviderBase.ctx — no direct fields

Platform and Writer are NOT direct fields on provider structs. They are
accessed via `p.Context().Platform` and `p.Context().Writer`.

In **immediate mode**, the generated ImmediateFactory creates a ProviderBase
with a partial Context populated from BindingConfig:

```go
ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
    p := &provider.Provider{
        ProviderBase: op.NewProviderBase(op.Context{
            Writer:   cfg.Writer,
            Platform: cfg.Platform,
        }),
    }
    return NewServiceReceiver(p)
}
```

In **action/graph mode**, the `ActionRegistrar` creates a provider instance
with the full execution Context (see D2). No per-call injection is needed.

This requires:
- `Platform *Platform` added to `BindingConfig`
- Codegen template change: when a provider embeds ProviderBase, emit
  `ProviderBase: op.NewProviderBase(op.Context{Writer: cfg.Writer, Platform: cfg.Platform})`
- `+devlore:bind Platform=Platform` is NOT used — Platform flows through Context

### D2: Per-graph provider allocation via ActionRegistrar

Provider instances are scoped to their owner (graph or script). The
`ActionRegistrar` signature changes from `func(reg *ActionRegistry)` to
`func(reg *ActionRegistry, ctx Context)`.

In **action/graph mode**, the executor creates a fresh `ActionRegistry` per
graph by calling each `ActionRegistrar(reg, ctx)` with the full execution
Context. The registrar creates a new provider instance with that Context:

```go
ActionRegistrar: func(reg *op.ActionRegistry, ctx op.Context) {
    p := &provider.Provider{
        ProviderBase: op.NewProviderBase(ctx),
    }
    provider.RegisterReflectedActions(reg, p)
}
```

The planner does NOT need provider instances — it validates action names
against the binding registry metadata (Params maps, etc.).

In **immediate/script mode**, `ImmediateFactory` creates a separate instance
with a partial Context from `BindingConfig` (see D1). Scripts and graphs
have separate provider instances at the provider level.

### D3: io.Writer removal

The `output io.Writer` parameter on git, service, and shell methods is an
implementation leak. Starlark cannot provide an `io.Writer`, so it's always
nil in immediate mode. Providers read `p.Context().Writer` instead.
Removing `output` from signatures also removes it from `Params` and gen files.

### D4: Resource types are lightweight

- `git.Resource`: URL, Path, Ref fields. Constructor from string → path.
- `service.Resource`: Name field. Constructor from string → name.
- `pkg.Resource`: Name, Manager, Version fields. Constructor from string → name.

### D5: pkg inputs become []pkg.Resource

The pkg provider's `Install`, `Remove`, `Upgrade` accept `[]pkg.Resource`
instead of `[]string`. Each package is a tracked resource with a URI
(`pkg://brew/htop`). This enables catalog deduplication and lineage tracking.

### D6: Starlark callbacks are immediate-mode only (no changes needed)

Callbacks (e.g., `file.Reducer` used by `WalkTree`) are already gated to
immediate receivers only by codegen (`codegen.go` Gate 1: "callable
parameter not supported in <template> template"). They cannot be serialized
into graph slots, so they never appear in planned/actions templates.

In immediate mode, the callback's lifetime is bounded by the script
execution — the provider instance, the Starlark `Callable`, and the thread
all share the same scope. This phase's changes (ProviderBase embedding,
Context via factory) strengthen this by ensuring `p.Context()` is always
populated even in immediate mode.

**Note:** The callback mechanism is currently broken. Fixing it is out of
scope for this phase.

## Implementation Steps

### Step 1: Infrastructure (no provider changes yet)

**`pkg/op/binding_config.go`** — Add `Platform *Platform` field.

**`pkg/op/binding_registry.go`** — Change `ProviderRegistrar` type from
`func(reg *ActionRegistry)` to `func(reg *ActionRegistry, ctx Context)`.

**`internal/execution/executor.go`** — Create per-graph ActionRegistry by
calling each ActionRegistrar with the execution Context.

**`internal/starlark/binding_set.go`** — Update `NewPopulatedRegistry()` to
pass a zero/partial Context when calling ActionRegistrars (or adapt the call
site to the new signature).

**`star/.../commands/generate.star`** — Add to `BINDING_CONFIG_FIELDS`:
```python
"Platform": {"zero": "nil", "type": "*op.Platform"},
```

**Codegen template** — When a provider embeds ProviderBase, emit
`ProviderBase: op.NewProviderBase(op.Context{Writer: cfg.Writer, Platform: cfg.Platform})`
in the ImmediateFactory. This requires a template change in noblefactor-ops
(`templateFuncProviderInit` in `codegen.go`).

**Callers of `BindingConfig{}`** — Add `Platform: platform.New()` where
appropriate (~12 sites in `internal/lore/`, `internal/writ/`,
`internal/starlark/`, `internal/e2e/`).

### Step 2: git provider

**`pkg/op/provider/git/provider.go`**:
- Embed `op.ProviderBase`
- Remove `output io.Writer` from Clone, Checkout, Pull signatures
- Use `p.Context().Writer` for command output
- Keep `cloneFn` test hook but update its signature (remove output param)

**`pkg/op/provider/git/resource.go`** (new):
- `Resource` struct: `op.ResourceBase`, URL, Path, Ref
- `Tombstone` struct: `op.TombstoneBase`, ClonedPath
- URI/Scheme/Host/Path methods
- `RegisterConstructor` and `RegisterPlanTimeConstructor` in init()

**`pkg/op/provider/git/resource_test.go`** (new):
- URI generation, constructor round-trip, interface satisfaction

**`pkg/op/provider/git/provider_test.go`** — Update for new signatures.

### Step 3: service provider

**`pkg/op/provider/service/provider.go`**:
- Embed `op.ProviderBase`
- Remove `Platform *op.Platform` field
- Remove `output io.Writer` from all compensable method signatures
- Use `p.Context().Platform.ServiceManager` and `p.Context().Writer`
- Change compensate methods to accept typed `Tombstone` instead of `any`

**`pkg/op/provider/service/resource.go`** (new):
- `Resource` struct: `op.ResourceBase`, Name
- `Tombstone` struct: `op.TombstoneBase`, WasRunning, WasEnabled

**`pkg/op/provider/service/resource_test.go`** (new)

**`pkg/op/provider/service/provider_test.go`** — Update for new signatures.

### Step 4: pkg provider

**`pkg/op/provider/pkg/provider.go`**:
- Embed `op.ProviderBase`
- Remove `Platform *op.Platform` field
- Use `p.Context().Platform` for package manager resolution
- Change `Install`, `Remove`, `Upgrade` inputs to `[]pkg.Resource`
- Change compensate methods to accept typed `Tombstone` instead of `any`

**`pkg/op/provider/pkg/resource.go`** (new):
- `Resource` struct: `op.ResourceBase`, Name, Manager, Version
- `Tombstone` struct: `op.TombstoneBase`, Packages, Manager, Cask,
  AlreadyInstalled, PreviousVersions

**`pkg/op/provider/pkg/resource_test.go`** (new)

**`pkg/op/provider/pkg/provider_test.go`** — Update for new signatures.

### Step 5: shell provider

**`pkg/op/provider/shell/provider.go`**:
- Embed `op.ProviderBase`
- Remove `output io.Writer` from Exec, PowerShell
- Use `p.Context().Writer`

**`pkg/op/provider/shell/provider_test.go`** — Update.

### Step 6: Regenerate all codegen + docs

- Run `make build` to regenerate all `gen/` files
- Verify `make check` passes
- Update `docs/architecture/devlore-resource-management.md` Section 3.3
  and Section 11 to mark git, service, pkg as "Implemented"
- Update `docs/plans/resource-management.md` phase status

## Key Files

| File | Action |
|------|--------|
| `pkg/op/binding_config.go` | Add Platform field |
| `pkg/op/binding_registry.go` | Change ProviderRegistrar signature |
| `internal/execution/executor.go` | Per-graph ActionRegistry creation |
| `internal/starlark/binding_set.go` | Update ActionRegistrar call sites |
| `pkg/op/provider.go` | Reference: ProviderBase, Provider interface |
| `pkg/op/resource.go` | Reference: Resource interface, scheme constants |
| `pkg/op/provider/file/resource.go` | Reference: file.Resource pattern |
| `pkg/op/provider/git/provider.go` | Embed ProviderBase, remove output |
| `pkg/op/provider/git/resource.go` | New — git.Resource + Tombstone |
| `pkg/op/provider/service/provider.go` | Embed ProviderBase, remove output/Platform |
| `pkg/op/provider/service/resource.go` | New — service.Resource + Tombstone |
| `pkg/op/provider/pkg/provider.go` | Embed ProviderBase, remove Platform |
| `pkg/op/provider/pkg/resource.go` | New — pkg.Resource + Tombstone |
| `pkg/op/provider/shell/provider.go` | Embed ProviderBase, remove output |
| `star/.../commands/generate.star` | Add Platform to BINDING_CONFIG_FIELDS |

## Verification

```bash
make build    # regenerates all gen files
make vet      # no vet issues
make test     # all tests pass
make check    # full quality gate
```
