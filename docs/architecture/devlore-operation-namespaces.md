# Action Namespaces

This document describes how to add new action namespaces to the devlore-cli execution engine.

See also:

- [Execution Graph](devlore-execution-graph.md) — Core graph architecture
- [Typed Slots](devlore-typed-slots.md) — Slot model and type mappings
- [Emergent System Model](devlore-emergent-system-model.md) — System-level architecture,
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

| Namespace | Actions | Package |
|-----------|---------|---------|
| file | `file.link`, `file.copy`, `file.backup`, `file.unlink`, `file.remove`, `file.write`, `file.move`, `file.mkdir`, `file.source` | `provider/file` |
| encryption | `encryption.decrypt` | `provider/encryption` |
| template | `template.render` | `provider/template` |
| content | `content.literal` | `provider/content` |
| pkg | `pkg.install`, `pkg.upgrade`, `pkg.remove`, `pkg.update` | `provider/pkg` |
| shell | `shell.exec`, `shell.powershell` | `provider/shell` |
| service | `service.start`, `service.stop`, `service.restart`, `service.enable`, `service.disable` | `provider/service` |
| net | `net.download` | `provider/net` |
| archive | `archive.extract` | `provider/archive` |
| git | `git.clone`, `git.checkout`, `git.pull` | `provider/git` |
| flow | `flow.choose`, `flow.gather`, `flow.elevate` | `flow/` |

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

func (o *Pull) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
    image := slots["image"].(string)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] docker pull %s\n", image)
        return nil, nil, nil
    }
    return nil, nil, o.Impl.Pull(image)
}

func (o *Pull) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
    return nil
}

// Register registers all docker actions with the given registry.
func Register(reg *execution.ActionRegistry) {
    p := &Provider{}
    reg.Register(&Pull{Impl: p})
}
```

### Step 3: Update RegisterAll()

In `internal/execution/provider/register_gen.go`, add your namespace:

```go
func RegisterAll(reg *execution.ActionRegistry) {
    file.Register(reg)
    // ...existing providers...
    docker.Register(reg)  // Add your namespace
}
```

This single change makes your actions available to both `writ` and `lore`.

### Step 4: Add Plan Binding

Create a plan binding struct in `internal/starlark/` (or generate one via
`devlore ops.generate`). The plan binding creates graph nodes when called
from Starlark:

```go
// internal/starlark/plan_docker_gen.go — generated
type DockerPlan struct {
    PlanBase
}

func (p *DockerPlan) Pull(image string) *execution.Node {
    node := p.addNode("docker.pull")
    node.SetSlotImmediate("image", image)
    return node
}
```

### Step 5: Register on PlanRoot

Add the plan binding as a sub-namespace on `PlanRoot` in
`internal/starlark/plan_root.go`:

```go
// In PlanRoot.ToStarlark():
dockerNs := starlarkstruct.FromStringDict(starlark.String("docker"), starlark.StringDict{
    "pull":  starlark.NewBuiltin("pull", p.docker.pullBuiltin),
    "build": starlark.NewBuiltin("build", p.docker.buildBuiltin),
})
```

All resource operations are sub-namespaces under `plan`:
- `plan.package.*`, `plan.file.*`, `plan.service.*`, `plan.shell.*`
- `plan.net.*`, `plan.archive.*`, `plan.git.*`, `plan.content.*`
- `plan.template.*`, `plan.encryption.*`

Only graph construction primitives remain top-level: `plan.source()`,
`plan.gather()`, `plan.choose()`, `plan.depends_on()`.

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

    # Network and content
    plan.net.download(url)
    plan.content.literal(content)

    # Docker operations (example)
    plan.docker.pull("nginx:latest")
    plan.docker.build(context=".", tag="myapp:latest")

    # Graph construction primitives (top-level)
    plan.depends_on(node_a, node_b)
    plan.choose(when=predicate, then=lambda: ...)
    plan.gather(items=list, do=lambda item: ...)
```

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

## Documenting for Lore Package Developers

When adding a new namespace, you must document the Starlark API for lore package authors. This documentation lives in `docs/guides/lore/` and should be accessible to developers writing `install.star`, `configure.star`, and other phase scripts.

### Required Documentation

Create or update `docs/guides/lore/plan-bindings.md` with:

1. **Namespace overview** - What the namespace does and when to use it
2. **Function reference** - Each function with parameters, return value, and examples
3. **Complete examples** - Real-world usage in phase scripts

### Documentation Template

For each function in your namespace, document:

```markdown
### plan.docker.pull(image)

Pulls a container image from a registry.

**Parameters:**
- `image` (string, required): The image reference (e.g., "nginx:latest", "ghcr.io/org/app:v1.2")

**Returns:** A node object that can be used with `plan.depends_on()`

**Example:**
```starlark
def install(package, phase):
    # Pull the base image
    nginx = plan.docker.pull("nginx:1.25")

    # Build depends on the base image being available
    app = plan.docker.build(context=".", tag="myapp:latest")
    plan.depends_on(app, nginx)
```

**Notes:**
- Requires Docker to be installed on the target system
- Uses the default Docker daemon socket
- Respects `~/.docker/config.json` for registry authentication
```

### Example: Package Namespace Documentation

See `docs/guides/lore/plan-bindings.md` for the reference implementation:

```markdown
## Package Operations

The `plan.package` namespace provides cross-platform package management.
Operations use the system's native package manager (brew/port on macOS,
apt/dnf on Linux, winget on Windows).

### plan.package.install(*packages)

Installs one or more packages.

**Parameters:**
- `*packages` (strings): Package names to install

**Platform-specific prefixes (macOS only):**
- `brew:pkg` - Force Homebrew formula
- `cask:pkg` - Homebrew Cask (GUI applications)
- `port:pkg` - Force MacPorts

**Example:**
```starlark
def install(package, phase):
    # Install multiple packages
    plan.package.install("curl", "jq", "ripgrep")

    # macOS: specify package manager
    plan.package.install("brew:wget", "cask:iterm2", "port:tree")
```
```

### Documentation Location

| Audience | Location | Content |
|----------|----------|---------|
| Engine developers | `docs/architecture/devlore-operation-namespaces.md` | How to implement action namespaces |
| Package developers | `docs/guides/lore/plan-bindings.md` | How to use plan.* in Starlark |
| CLI users | `docs/cli/lore/` | Command-line usage |

### Keeping Docs in Sync

The Starlark API documentation should match the implementation. When updating bindings:

1. Update the Provider struct and regenerate actions
2. Regenerate plan bindings and receivers via `devlore ops.generate`
3. Update `docs/guides/lore/plan-bindings.md` with new/changed functions
4. Add examples showing real usage patterns

## Checklist

When adding a new namespace:

- [ ] Create `internal/execution/provider/<namespace>/provider.go` with Provider struct
- [ ] Create `internal/execution/provider/<namespace>/actions_gen.go` with action structs and `Register(reg)`
- [ ] Update `RegisterAll()` in `provider/register_gen.go` to include your provider
- [ ] Create plan binding struct in `internal/starlark/plan_<namespace>_gen.go`
- [ ] Register sub-namespace on `PlanRoot` in `internal/starlark/plan_root.go`
- [ ] Add tests
- [ ] **Document for package developers** in `docs/guides/lore/plan-bindings.md`
- [ ] Update this architecture documentation
