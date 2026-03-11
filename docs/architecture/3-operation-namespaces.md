# Action Namespaces

This document describes how to add new action namespaces to the devlore-cli execution engine.

See also:

- [Execution Graph](2-execution-graph.md) — Core graph architecture
- [Typed Slots](2.1-typed-slots.md) — Slot model and type mappings
- [Emergent System Model](1-system-model.md) — System-level architecture,
  dependency taxonomy (structural, functional, procedural)

## Architecture Overview

The execution engine processes a directed acyclic graph (DAG) of nodes, where each node specifies an action to execute. Both `writ` and `lore` share the same engine:

```
┌─────────────────────────────────────────────────────────────┐
│                    Execution Engine                         │
│                (internal/execution)                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐          ┌─────────────────┐          │
│  │  File Tree      │          │  Package Graph  │          │
│  │  Builder        │          │  Builder        │          │
│  │  (writ/tree)    │          │  (lore/builder) │          │
│  └────────┬────────┘          └────────┬────────┘          │
│           │                            │                    │
│           │    ┌──────────────────┐    │                    │
│           └───►│ Execution Graph  │◄───┘                    │
│                │ (execution.Graph)│                         │
│                └────────┬─────────┘                         │
│                         │                                   │
│                         ▼                                   │
│                ┌──────────────────┐                         │
│                │ GraphExecutor    │                         │
│                │  .RunNodes()     │                         │
│                └──────────────────┘                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Current Namespaces

### Planned providers (graph node actions)

| Namespace | Access | Actions | Package |
|-----------|--------|---------|---------|
| file | planned | `file.backup`, `file.copy`, `file.link`, `file.move`, `file.remove`, `file.remove_all`, `file.unlink`, `file.walk_tree`, `file.write_bytes`, `file.write_text`, `file.exists`, `file.glob`, `file.is_dir`, `file.is_file`, `file.mkdir`, `file.read`, `file.join`, `file.name`, `file.parent`, `file.root` | `provider/file` |
| encryption | planned | `encryption.decrypt_sops_file` | `provider/encryption` |
| pkg | planned | `pkg.install`, `pkg.remove`, `pkg.upgrade`, `pkg.update`, `pkg.installed`, `pkg.not_installed`, `pkg.version_gte` | `provider/pkg` |
| shell | planned | `shell.exec`, `shell.power_shell` | `provider/shell` |
| service | planned | `service.start`, `service.stop`, `service.restart`, `service.enable`, `service.disable`, `service.enabled`, `service.exists`, `service.running` | `provider/service` |
| appnet | planned | `appnet.download` | `provider/appnet` |
| archive | planned | `archive.extract` | `provider/archive` |
| git | planned | `git.clone`, `git.checkout`, `git.pull` | `provider/git` |
| flow | planned | `flow.choose`, `flow.gather`, `flow.elevate`, `flow.wait_until`, `flow.complete`, `flow.degraded`, `flow.fatal` | `flow/` |

### Both (planned + immediate)

| Namespace | Access | Actions | Package |
|-----------|--------|---------|---------|
| template | both | `template.render` | `provider/template` |
| json | both | `json.encode`, `json.encode_indent`, `json.decode` | `provider/json` |
| yaml | both | `yaml.encode`, `yaml.decode` | `provider/yaml` |
| regexp | both | `regexp.match`, `regexp.find`, `regexp.find_all`, `regexp.find_submatch`, `regexp.find_all_submatch`, `regexp.replace`, `regexp.replace_literal`, `regexp.split` | `provider/regexp` |

### Immediate-only providers (execute directly, no graph nodes)

| Namespace | Access | Actions | Package |
|-----------|--------|---------|---------|
| ui | immediate | `ui.error`, `ui.fail`, `ui.note`, `ui.success`, `ui.warn` | `provider/ui` |
| staranalysis | immediate | `staranalysis.analyze` | `provider/staranalysis` |
| starcode | immediate | `starcode.capture` | `provider/starcode` |
| starcomplexity | immediate | `starcomplexity.compute_complexity` | `provider/starcomplexity` |
| starindex | immediate | `starindex.index_files` | `provider/starindex` |
| starstats | immediate | `starstats.compute_stats` | `provider/starstats` |

### Removed

| Namespace | Removed In | Reason |
|-----------|-----------|--------|
| content | binding-unification phase 8 | `content.literal` was a pure pass-through (`[]byte` in, `[]byte` out) — no-op |

## Darwin Package Manager Idempotence

On macOS, users may have both Homebrew and MacPorts installed. The package operations handle this with idempotent behavior:

### Package Manager Detection

The `Host` interface provides methods for package manager discovery:

```go
type Host interface {
    // PackageManager returns the preferred PM for new installs (port > brew)
    PackageManager() PackageManager

    // InstalledBy returns the PM that installed a package (nil if not installed)
    InstalledBy(name string) PackageManager

    // AllInstalledBy returns ALL PMs that have the package (for warnings)
    AllInstalledBy(name string) []PackageManager

    // GetPackageManager returns a specific PM by name ("brew", "port")
    GetPackageManager(name string) PackageManager
}
```

### Action Behavior

| Action | PM Resolution | Notes |
|-----------|---------------|-------|
| Install | Explicit prefix > Preferred PM | Skip if already installed by any PM |
| Upgrade | Explicit prefix > InstalledBy > Preferred | Upgrades via the PM that installed it |
| Remove | Explicit prefix > InstalledBy > Preferred | Warns if installed by multiple PMs |
| Update | Explicit prefix > Preferred PM | Updates package index |

### Multi-PM Warning

When removing a package installed by multiple package managers, the action:
1. Removes via the preferred PM (or explicit prefix)
2. Warns the user about other installations

```
[package] port remove wget
[warn] wget is also installed via brew; use 'brew:wget' to remove that copy
```

### Decommission Behavior

The `lore decommission` command removes packages from ALL package managers:

```go
func RemoveAll(name string) []Result {
    var results []Result
    for _, pm := range host.AllInstalledBy(name) {
        results = append(results, pm.Remove(name))
    }
    return results
}
```

### Cask Detection

Homebrew Cask apps are checked separately from formulae:

```go
func (m *brewManager) Installed(name string) bool {
    // Check formula
    if runShellCommand("brew list --formula "+name, false).OK {
        return true
    }
    // Check cask (GUI applications)
    return runShellCommand("brew list --cask "+name, false).OK
}
```

## Adding a New Namespace

Follow these steps to add a new action namespace (e.g., `docker`).

### Step 1: Create the Provider

Create `internal/execution/provider/docker/provider.go`:

```go
// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package docker

import (
    "fmt"
    "os/exec"
)

// Provider implements Docker container operations.
type Provider struct{}

func (p *Provider) Pull(image string) error {
    _, _ = fmt.Fprintf(os.Stderr, "[docker] pull %s\n", image)
    cmd := exec.Command("docker", "pull", image)
    cmd.Stdout = os.Stderr
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func (p *Provider) Build(context, tag string) error {
    // Implementation...
}
```

### Step 2: Create Actions

Create `internal/execution/provider/docker/actions_gen.go`:

```go
package docker

import (
    "fmt"

    "github.com/NobleFactor/devlore-cli/internal/execution"
)

// Pull pulls a container image.
type Pull struct{ Impl *Provider }

func (o *Pull) Name() string { return "docker.pull" }

func (o *Pull) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.Complement, error) {
    image := slots["image"].(string)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] docker pull %s\n", image)
        return nil, nil, nil
    }
    return nil, nil, o.Impl.Pull(image)
}

func (o *Pull) Undo(_ *execution.Context, _ map[string]any, _ execution.Complement) error {
    return nil
}

// Register registers all docker actions with the given registry.
func Register(reg *execution.ActionRegistry) {
    p := &Provider{}
    reg.Register(&Pull{Impl: p})
}
```

### Step 3: Generate the Provider Descriptor

The code generator (`star devlore actions generate`) reads the Provider struct
and emits a provider descriptor that implements the `op.Provider` interface
(and optionally `op.PlannedProvider` / `op.ImmediateProvider`). The descriptor
announces itself via `init()` and receives callbacks from the framework.

```go
// Generated provider descriptor
type dockerProvider struct{}

func (p *dockerProvider) Name() string { return "docker" }

func (p *dockerProvider) Register(reg *op.ActionRegistry, ctx op.Context) {
    impl := &Provider{}
    reg.Register(&Pull{Impl: impl})
    reg.Register(&Build{Impl: impl})
}

func (p *dockerProvider) NewPlanned(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
    return NewDockerPlan(graph, project, reg)
}

func init() {
    op.Announce(&dockerProvider{})
}
```

The `init()` does exactly one thing: announce the provider's existence.
No instantiation, no factory creation, no action registration. All real work
happens in the callbacks (`Register`, `NewPlanned`, `NewImmediate`) when the
framework calls `op.InitAll(reg, ctx)`.

This single announcement makes the provider's actions available to all
consumers (`writ`, `lore`, `devlore-test`). See [Projected Provider API —
Provider Registration](3.2-projected-provider-api.md#provider-registration)
for the full interface definitions.

### Handwritten Providers — Same Pattern

Not all providers are generated. Control-flow actions (`flow.choose`,
`flow.gather`, `flow.elevate`, `flow.wait_until`) are handwritten because
they are graph-construction primitives, not resource operations. But they
register exactly the same way — same `Provider` interface, same
`op.Announce()` call, same callback from `op.InitAll()`:

```go
// internal/execution/flow/provider.go — handwritten
type flowProvider struct{}

func (p *flowProvider) Name() string { return "flow" }

func (p *flowProvider) Register(reg *op.ActionRegistry, _ op.Context) {
    reg.Register(&Choose{})
    reg.Register(&Gather{})
    reg.Register(&Elevate{})
    reg.Register(&WaitUntil{})
}

func init() {
    op.Announce(&flowProvider{})
}
```

The structure is identical to the generated `dockerProvider` above. The
framework cannot distinguish generated from handwritten providers and does
not need to. Any future handwritten provider follows this same pattern.

### Step 4: Add Plan Binding

The generated plan binding struct creates graph nodes when called from
Starlark. It is returned by the descriptor's `NewPlanned` callback:

```go
// internal/starlark/plan_docker_gen.go — generated
type DockerPlan struct {
    PlanBase
}

func (p *DockerPlan) Pull(image string) *op.Output {
    node := p.addNode("docker.pull")
    node.SetSlotImmediate("image", image)
    return op.NewOutput(node, p.graph, "")
}
```

All resource operations are sub-namespaces under `plan`:
- `plan.package.*`, `plan.file.*`, `plan.service.*`, `plan.shell.*`
- `plan.appnet.*`, `plan.archive.*`, `plan.git.*`
- `plan.template.*`, `plan.encryption.*`

Only graph construction primitives remain top-level: `plan.source()`,
`plan.gather()`, `plan.choose()`.

**NOTE:** `starlark.StringDict` and `FromStringDict` are BANNED for namespace
receivers. All namespaces use the `Attr`/`AttrNames` receiver pattern.

## Starlark API Convention

All resource operations are exposed via sub-namespaces under the `plan` global:

```starlark
def install(package, phase):
    # Package operations
    plan.package.install("nginx")
    plan.package.upgrade("curl")
    plan.package.remove("telnet")
    plan.package.update()

    # File operations
    plan.file.copy(source, target)
    plan.file.link(source, target)
    plan.file.configure(source, target)  # template + copy
    plan.file.mkdir(target)

    # Service operations
    plan.service.start("nginx")
    plan.service.enable("nginx")

    # Shell (escape hatch)
    plan.shell.exec("echo hello")

    # Network
    plan.appnet.download(url)

    # Docker operations (example)
    plan.docker.pull("nginx:latest")
    plan.docker.build(context=".", tag="myapp:latest")

    # Graph construction primitives (top-level)
    plan.depends_on(node_a, node_b)
    plan.choose(when=predicate, then=lambda: ...)
    plan.gather(items=list, do=lambda item: ...)
```

## Provider Method Contracts

Provider methods follow one of two return contracts:

| Contract | Signature | Expectation |
|----------|-----------|-------------|
| **Compensable** | `(T, U, error)` | `CompensateAction` and `ReconcileAction` companion methods exist |
| **Non-compensable** | `(T, error)` | No companion methods. Predicates, queries, or pure transforms |

T, U are concrete types chosen by the provider — not type aliases.

### Return value: the object of the action

The first return value (T) is the **object** of the action — the thing acted
upon. It answers "to whom or what?" in the sentence "the engine _verbed_ the
_object_."

| Method | Returns | Object |
|--------|---------|--------|
| `Link(source, path)` | `path` | The symlink created |
| `Copy(path, mode, content)` | `path` | The file written |
| `Backup(path, suffix)` | `backupPath` | The backup created |
| `Remove(path, ...)` | `path` | The file deleted |
| `Install(packages, ...)` | `packages` | The packages installed |
| `Start(name)` | `name` | The service started |
| `Clone(url, path)` | `path` | The directory cloned into |
| `Extract(source, prefix)` | `prefix` | The extraction directory |
| `Exists(path)` | `bool` | Whether the path exists |
| `Read(path)` | `[]byte` | The file content read |

Do not return derived or summary values (checksums, formatted strings). Return
the resource that was acted upon.

### Undo state: the compensation receipt

The second return value (U) in compensable methods is the **undo state** — an
opaque receipt that the corresponding `CompensateAction` method uses to reverse
the action. The executor stores it as-is; only the Compensate method interprets
it.

## Naming Conventions

| Layer | Convention | Example |
|-------|------------|---------|
| Action name | `<namespace>.<action>` | `docker.pull`, `pkg.install` |
| Action struct | `<Action>` in package | `docker.Pull`, `pkg.Install` |
| Provider method | `<Action>` | `Provider.Pull`, `Provider.Install` |
| Starlark builtin | `<namespace><Action>Builtin` | `dockerPullBuiltin` |
| Starlark API | `plan.<namespace>.<action>()` | `plan.docker.pull()` |

## Testing

Add tests in `internal/execution/execution_test.go` or create a dedicated
test file in the provider package:

```go
func TestDockerPullDryRun(t *testing.T) {
    reg := execution.NewActionRegistry()
    provider.RegisterAll(reg)

    node := &execution.Node{
        ID:     "test-docker-pull",
        Action: reg.MustGet("docker.pull"),
        Status: execution.StatusPending,
    }
    node.SetSlotImmediate("image", "nginx:latest")

    ctx := &execution.Context{
        Context: context.Background(),
        DryRun:  true,
        Logger:  io.Discard,
    }

    slots := node.ResolvedSlots(nil)
    _, _, err := node.Action.Do(ctx, slots)
    if err != nil {
        t.Fatalf("Do failed: %v", err)
    }
}
```

## Documentation

Plan binding documentation is auto-generated by `star devlore knowledge extract`
and lives in `devlore-registry/knowledge/package-authoring/bindings/reference.md`.
Do not maintain hand-written API references.

| Audience | Location | Content |
|----------|----------|---------|
| Engine developers | `docs/architecture/3-operation-namespaces.md` | How to implement action namespaces |
| Package developers | `devlore-registry/knowledge/package-authoring/bindings/reference.md` | Auto-generated plan.* API reference |
| CLI users | `docs/cli/lore/` | Command-line usage |

## Checklist

When adding a new namespace:

- [ ] Create `pkg/op/provider/<namespace>/provider.go` with Provider struct
- [ ] Run `star devlore actions generate` to produce action wrappers, planned receiver, immediate receiver, and provider descriptor
- [ ] Verify `init()` in generated code contains only `op.Announce()`
- [ ] Add tests
- [ ] Regenerate reference: `star devlore knowledge extract`
- [ ] Update this architecture documentation
