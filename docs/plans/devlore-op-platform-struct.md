---
title: "op.HostProvider to op.Platform Struct"
status: draft
created: 2026-02-24
updated: 2026-02-24
---

# Plan: op.HostProvider to op.Platform Struct

## Summary

Replace the `op.HostProvider` interface hierarchy (`HostProvider`, `PackageManagerProvider`, `ServiceManagerProvider`) with a single `op.Platform` struct carrying `PackageManager` and `ServiceManager` as interface-typed fields. Eliminate the adapter layer in `internal/execution/hostadapter.go`. Concrete implementations become private types in the `platform` package.

## Goals

1. **Eliminate interface duplication**: The op-level interfaces duplicate the concrete interfaces in the `host` package with different return types (`error` vs `Result`). Replace with a single canonical set.
2. **Unblock codegen**: Code generation fails for `pkg` and `service` providers because the codegen type mapper has no entries for op-level interfaces. Platform as a struct with interface fields resolves this.
3. **Remove adapter layer**: The `hostadapter.go` translation layer becomes unnecessary when providers access `Platform` directly.
4. **Simplify provider method signatures**: Provider methods take `PackageManager`/`ServiceManager` as method arguments instead of accessing them through state. Move to `Platform` field on Provider struct.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `op.HostProvider` interface | Exists | Duplicates `host.Host` with different return types |
| `op.PackageManagerProvider` interface | Exists | Duplicates `host.PackageManager` |
| `op.ServiceManagerProvider` interface | Exists | Duplicates `host.ServiceManager` |
| `internal/execution/hostadapter.go` | Exists | Adapter translating between host and op types |
| `graph.go` Platform struct | Exists | Minimal `{OS, Arch}` — insufficient |
| Codegen for pkg/service providers | Broken | Type mapper has no entries for op-level interfaces |

## New Types in `pkg/op/platform.go`

Replaces `pkg/op/host.go`.

```go
type Platform struct {
    // Serializable info (used by Graph)
    OS       string `json:"os" yaml:"os"`
    Arch     string `json:"arch" yaml:"arch"`
    Distro   string `json:"distro,omitempty" yaml:"distro,omitempty"`
    Version  string `json:"version,omitempty" yaml:"version,omitempty"`
    Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`

    // Runtime — not serialized
    PackageManager  PackageManager            `json:"-" yaml:"-"`
    PackageManagers map[string]PackageManager  `json:"-" yaml:"-"`
    ServiceManager  ServiceManager            `json:"-" yaml:"-"`
}

// Resolution methods
func (p *Platform) GetPackageManager(name string) PackageManager
func (p *Platform) InstalledBy(name string) PackageManager
func (p *Platform) AllInstalledBy(name string) []PackageManager

type PackageManager interface {
    Name() string
    Installed(name string) bool
    Version(name string) string
    Available(name string) bool
    Search(query string, limit int) []SearchResult
    Install(packages ...string) Result
    Remove(name string) Result
    Update() Result
    AddRepo(url, keyURL, name string) Result
    NeedsSudo() bool
}

type ServiceManager interface {
    Exists(name string) bool
    IsRunning(name string) bool
    IsEnabled(name string) bool
    Status(name string) string
    Start(name string) Result
    Stop(name string) Result
    Enable(name string) Result
    Disable(name string) Result
    NeedsSudo() bool
}

type Result struct {
    OK     bool
    Stdout string
    Stderr string
    Code   int
}

type SearchResult struct {
    Name        string
    Version     string
    Description string
}
```

### Deleted from `pkg/op/`

- `HostProvider` interface
- `PackageManagerProvider` interface
- `ServiceManagerProvider` interface
- `host.go` (the file)
- `graph.go` `Platform` struct `{OS, Arch}` (replaced by the new `Platform`)

## Visibility Model

| Type | Package | Exported | Role |
|------|---------|----------|------|
| `op.Platform` | `pkg/op` | yes | struct — carries info + PackageManager/ServiceManager |
| `op.PackageManager` | `pkg/op` | yes | interface — canonical package manager API |
| `op.ServiceManager` | `pkg/op` | yes | interface — canonical service manager API |
| `brewManager` | `platform` | no | implements `op.PackageManager` |
| `portManager` | `platform` | no | implements `op.PackageManager` |
| `aptManager` | `platform` | no | implements `op.PackageManager` |
| `dnfManager` | `platform` | no | implements `op.PackageManager` |
| `pacmanManager` | `platform` | no | implements `op.PackageManager` |
| `wingetManager` | `platform` | no | implements `op.PackageManager` |
| `launchdManager` | `platform` | no | implements `op.ServiceManager` |
| `systemdManager` | `platform` | no | implements `op.ServiceManager` |
| `windowsServiceManager` | `platform` | no | implements `op.ServiceManager` |

## Construction: platform.New()

`pkg/op/provider/platform/` (renamed from `host/`). Build-tag-selected files construct `*op.Platform` directly.

**darwin.go** (`//go:build darwin`):
```go
func newDarwin() *op.Platform {
    // detect info
    // detect brew, port (both may exist)
    // preferred PackageManager: port > brew
    return &op.Platform{
        OS: ..., Arch: ..., Distro: ..., Version: ..., Hostname: ...,
        PackageManager:  preferredPM,
        PackageManagers: map[string]op.PackageManager{"brew": brew, "port": port},
        ServiceManager:  &launchdManager{},
    }
}
```

**linux.go** (`//go:build linux`):
```go
func newLinux() *op.Platform {
    // detect info, detect PackageManager by distro
    return &op.Platform{
        ...,
        PackageManager:  pm,
        PackageManagers: map[string]op.PackageManager{pm.Name(): pm},
        ServiceManager:  &systemdManager{},
    }
}
```

**windows.go** (`//go:build windows`):
```go
func newWindows() *op.Platform {
    return &op.Platform{
        ...,
        PackageManager:  &wingetManager{},
        PackageManagers: map[string]op.PackageManager{"winget": &wingetManager{}},
        ServiceManager:  &windowsServiceManager{},
    }
}
```

**Entry point:**
```go
func New() *op.Platform  // dispatches by runtime.GOOS
```

The `Host` interface is eliminated. `RunCommand`, `ExpandPath`, `HomeDir` stay as package-internal utilities (used by PackageManager/ServiceManager impls, not exposed on Platform).

## Provider Changes

### pkg.Provider

**`pkg/op/provider/pkg/provider.go`**:
```go
type Provider struct {
    Platform *op.Platform
}

// No host/platform parameter — access via p.Platform.PackageManager
func (p *Provider) Install(packages []string, manager string, cask bool) ([]string, map[string]any, error) {
    pm := resolvePMForInstall(p.Platform, manager)
    ...
}
```

**`pkg/op/provider/pkg/helpers.go`**: `host op.HostProvider` to `platform *op.Platform`, PM access via `platform.PackageManager`, `platform.PackageManagers`, `platform.GetPackageManager()`.

### service.Provider

**`pkg/op/provider/service/provider.go`**:
```go
type Provider struct {
    Platform *op.Platform
}

// No svc parameter — access via p.Platform.ServiceManager
func (p *Provider) Disable(name string, output io.Writer) (string, map[string]any, error) {
    sm := p.Platform.ServiceManager
    ...
}
```

### op.Context and ExecutorOptions

**`pkg/op/action.go`**: `Context.Host HostProvider` to `Context.Platform *Platform`

**`internal/execution/executor.go`**: `ExecutorOptions.Host op.HostProvider` to `ExecutorOptions.Platform *op.Platform`

### Consumers

| File | Change |
|------|--------|
| `internal/lore/commands.go` | `Host: execution.NewHostProvider(host.NewHost())` to `Platform: platform.New()` |
| `internal/writ/commands.go` | Same |
| `internal/writ/migrate/session.go` | Same |
| `internal/writ/graph_builder.go` | Same |

## Codegen Changes (noblefactor-ops)

**`internal/starlark/codegen.go`**: The generated action wrappers change pattern. Instead of `Impl *Provider` set once at Register time, construct Provider per Do() call with Platform from context:

```go
// Generated Install action
type Install struct{}

func (o *Install) Do(ctx *op.Context, slots map[string]any) (...) {
    packages := slots["packages"].([]string)
    ...
    p := &Provider{Platform: ctx.Platform}
    result, state, err := p.Install(packages, manager, cask)
    return result, state, err
}

func (o *Install) Undo(ctx *op.Context, state op.UndoState) error {
    p := &Provider{Platform: ctx.Platform}
    return p.CompensateInstall(state)
}
```

**Register function simplifies:**
```go
func Register(reg *op.ActionRegistry) {
    reg.Register(&Install{})
    reg.Register(&Upgrade{})
    ...
}
```

The `Impl *Provider` pattern is replaced by per-call Provider construction. No shared state, no concurrency concern.

**Note**: This pattern applies only to providers whose Provider struct has a `Platform` field. Providers without Platform (file, template, shell, etc.) keep the current `Impl *Provider` pattern unchanged.

## Starlark-Facing platform.Provider

**New: `pkg/op/provider/platform/starlark_provider.go`**

`platform.Provider` is a read-only consumer of `op.Platform`. It reads the info fields and exposes them to Starlark scripts. It contributes nothing to `op.Platform` — it is purely a presentation layer.

```go
// +devlore:access=both
type Provider struct {
    Platform *op.Platform
}

// Methods read from p.Platform and return info to Starlark:
// OS(), Arch(), Distro(), Version(), Hostname() — all (string, error)
```

Starlark scripts: `platform.os`, `platform.arch`, `platform.distro`, etc.

## Implementation Phases

### Phase 1: Foundation Types

- [ ] Create `pkg/op/platform.go` with `Platform`, `PackageManager`, `ServiceManager`, `Result`, `SearchResult`
- [ ] Add resolution methods on `Platform`
- [ ] Remove old `Platform` struct from `graph.go`

**Files**:
- `pkg/op/platform.go` - Create
- `pkg/op/graph.go` - Modify

### Phase 2: Platform Package

- [ ] Rename `pkg/op/provider/host/` to `pkg/op/provider/platform/`
- [ ] Rewrite `darwin.go`, `linux.go`, `windows.go` to construct `*op.Platform`
- [ ] Make concrete manager types private
- [ ] Implement `New()` entry point
- [ ] Internalize `RunCommand`, `ExpandPath`, `HomeDir`

**Files**:
- `pkg/op/provider/platform/` - Create (renamed from host)

### Phase 3: Provider Migration

- [ ] `pkg.Provider`: add `Platform *op.Platform` field, remove host parameter from methods
- [ ] `service.Provider`: add `Platform *op.Platform` field, remove svc parameter from methods
- [ ] Update helper functions

**Files**:
- `pkg/op/provider/pkg/provider.go` - Modify
- `pkg/op/provider/pkg/helpers.go` - Modify
- `pkg/op/provider/service/provider.go` - Modify

### Phase 4: Context and Executor

- [ ] `Context.Host` to `Context.Platform` in `pkg/op/action.go`
- [ ] `ExecutorOptions.Host` to `ExecutorOptions.Platform` in `internal/execution/executor.go`

**Files**:
- `pkg/op/action.go` - Modify
- `internal/execution/executor.go` - Modify

### Phase 5: Consumer Wiring

- [ ] Update `internal/lore/commands.go`
- [ ] Update `internal/writ/commands.go`
- [ ] Update `internal/writ/migrate/session.go`
- [ ] Update `internal/writ/graph_builder.go`

**Files**:
- `internal/lore/commands.go` - Modify
- `internal/writ/commands.go` - Modify
- `internal/writ/migrate/session.go` - Modify
- `internal/writ/graph_builder.go` - Modify

### Phase 6: Starlark Provider

- [ ] Create `pkg/op/provider/platform/starlark_provider.go`

**Files**:
- `pkg/op/provider/platform/starlark_provider.go` - Create

### Phase 7: Cleanup

- [ ] Delete `pkg/op/host.go`
- [ ] Delete `internal/execution/hostadapter.go`
- [ ] Delete `internal/execution/hostadapter_test.go`
- [ ] Update tests

**Files**:
- `pkg/op/host.go` - Delete
- `internal/execution/hostadapter.go` - Delete
- `internal/execution/hostadapter_test.go` - Delete

### Phase 8: Codegen (noblefactor-ops)

- [ ] Update `internal/starlark/codegen.go` for per-call Provider construction pattern
- [ ] Regenerate all provider receivers

**Files**:
- noblefactor-ops `internal/starlark/codegen.go` - Modify

## Deleted Files

| File | Reason |
|------|--------|
| `pkg/op/host.go` | Replaced by `platform.go` |
| `internal/execution/hostadapter.go` | No adapter needed |
| `internal/execution/hostadapter_test.go` | No adapter needed |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/host.go` | Delete | Replaced by platform.go |
| `pkg/op/platform.go` | Create | Platform struct + interfaces |
| `pkg/op/graph.go` | Modify | Remove old Platform struct |
| `pkg/op/action.go` | Modify | Context.Host to Context.Platform |
| `pkg/op/provider/platform/` | Create | Renamed from host/, rewritten |
| `pkg/op/provider/platform/starlark_provider.go` | Create | Starlark-facing read-only provider |
| `pkg/op/provider/pkg/provider.go` | Modify | Add Platform field, remove host param |
| `pkg/op/provider/pkg/helpers.go` | Modify | Update PM resolution |
| `pkg/op/provider/pkg/provider_test.go` | Modify | Update mock |
| `pkg/op/provider/service/provider.go` | Modify | Add Platform field, remove svc param |
| `pkg/op/provider/service/provider_test.go` | Modify | Update mock |
| `internal/execution/executor.go` | Modify | Host to Platform |
| `internal/execution/hostadapter.go` | Delete | No adapter needed |
| `internal/execution/hostadapter_test.go` | Delete | No adapter needed |
| `internal/execution/provider_test.go` | Modify | Update refs |
| `internal/lore/commands.go` | Modify | Wire platform.New() |
| `internal/writ/commands.go` | Modify | Wire platform.New() |
| `internal/writ/migrate/session.go` | Modify | Wire platform.New() |
| `internal/writ/graph_builder.go` | Modify | Wire platform.New() |
| noblefactor-ops `internal/starlark/codegen.go` | Modify | Provider construction pattern |

## Verification

1. `make check` — 0 lint issues, all tests pass
2. Rebuild star, delete `*_gen.go`, regenerate all providers
3. `make check` again with regenerated code

## Related Documents

- [Binding Unification](./binding-unification.md)
- [Projected Provider API](./projected-provider-api.md)
- [Star Gen Receiver](./star-gen-receiver.md)
