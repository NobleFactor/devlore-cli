# Operation Namespaces

This document describes how to add new operation namespaces to the devlore-cli execution engine.

## Architecture Overview

The execution engine processes a directed acyclic graph (DAG) of nodes, where each node specifies one or more operations to execute. Both `writ` and `lore` share the same engine:

```
┌─────────────────────────────────────────────────────────────┐
│                    Execution Engine                         │
│                  (internal/engine)                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐          ┌─────────────────┐          │
│  │  File Tree      │          │  Package Graph  │          │
│  │  Builder        │          │  Builder        │          │
│  │  (writ/tree)    │          │  (lore/graph)   │          │
│  └────────┬────────┘          └────────┬────────┘          │
│           │                            │                    │
│           │    ┌──────────────────┐    │                    │
│           └───►│ Execution Graph  │◄───┘                    │
│                │ (engine.Graph)   │                         │
│                └────────┬─────────┘                         │
│                         │                                   │
│                         ▼                                   │
│                ┌──────────────────┐                         │
│                │   Engine.Run()   │                         │
│                └──────────────────┘                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Current Namespaces

| Namespace | Operations | File |
|-----------|------------|------|
| file | `link`, `copy`, `expand`, `decrypt`, `backup`, `unlink`, `remove`, `mkdir`, `validate`, `rename` | `ops.go` |
| package | `package-install`, `package-upgrade`, `package-remove`, `package-update` | `ops_package.go` |
| shell | `shell`, `powershell` | `ops_package.go` |

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

### Operation Behavior

| Operation | PM Resolution | Notes |
|-----------|---------------|-------|
| Install | Explicit prefix > Preferred PM | Skip if already installed by any PM |
| Upgrade | Explicit prefix > InstalledBy > Preferred | Upgrades via the PM that installed it |
| Remove | Explicit prefix > InstalledBy > Preferred | Warns if installed by multiple PMs |
| Update | Explicit prefix > Preferred PM | Updates package index |

### Multi-PM Warning

When removing a package installed by multiple package managers, the operation:
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

Follow these steps to add a new operation namespace (e.g., `docker`).

### Step 1: Create the Operations File

Create `internal/engine/ops_<namespace>.go`:

```go
// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package engine

import (
    "fmt"
    "os/exec"
)

// =============================================================================
// Docker Operations
// =============================================================================

// DockerPullOp pulls a container image.
type DockerPullOp struct{}

func (o *DockerPullOp) Name() string         { return "docker-pull" }
func (o *DockerPullOp) Category() OpCategory { return OpDirect }

func (o *DockerPullOp) Execute(ctx *Context, node *Node) error {
    image := node.Metadata["image"]
    if image == "" {
        return fmt.Errorf("docker-pull: no image specified")
    }

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] docker pull %s\n", image)
        return nil
    }

    _, _ = fmt.Fprintf(ctx.Logger, "[docker] pull %s\n", image)
    cmd := exec.Command("docker", "pull", image)
    cmd.Stdout = ctx.Logger
    cmd.Stderr = ctx.Logger
    return cmd.Run()
}

// DockerBuildOp builds a container image.
type DockerBuildOp struct{}

func (o *DockerBuildOp) Name() string         { return "docker-build" }
func (o *DockerBuildOp) Category() OpCategory { return OpDirect }

func (o *DockerBuildOp) Execute(ctx *Context, node *Node) error {
    // Implementation...
}

// DockerOps returns all Docker operations for registration.
func DockerOps() []Operation {
    return []Operation{
        &DockerPullOp{},
        &DockerBuildOp{},
        // Add more as needed
    }
}
```

### Step 2: Update AllOps()

In `internal/engine/ops.go`, add your namespace to `AllOps()`:

```go
// AllOps returns all operations (file + package + docker + ...) for registration.
// Both writ and lore use this to ensure the same operations are available.
func AllOps() []Operation {
    ops := FileOps()
    ops = append(ops, PackageOps()...)
    ops = append(ops, DockerOps()...)  // Add your namespace
    return ops
}
```

This single change makes your operations available to both `writ` and `lore`.

### Step 3: Add Starlark Plan Bindings

Update the plan bindings interface in `internal/starlark/interfaces.go`:

```go
type PlanBindings interface {
    // ... existing methods ...

    // Docker operations
    DockerPull(image string) *engine.Node
    DockerBuild(context, tag string) *engine.Node
}
```

### Step 4: Implement Platform Bindings

Update each platform binding file (`darwin.go`, `linux.go`, `windows.go`) in `internal/starlark/platform/`:

```go
// DockerPull adds a docker pull node.
func (d *DarwinPlanBindings) DockerPull(image string) *engine.Node {
    node := &engine.Node{
        ID:         darwinGenerateNodeID("docker-pull", image),
        Operations: []string{"docker-pull"},
        Project:    d.project,
        Metadata: map[string]string{
            "image": image,
        },
    }
    d.graph.Nodes = append(d.graph.Nodes, node)
    return node
}
```

### Step 5: Add Starlark Namespace

In the `ToStarlark()` method, add the nested struct for your namespace:

```go
func (d *DarwinPlanBindings) ToStarlark() starlark.Value {
    // ... existing namespaces ...

    // Docker operations namespace: plan.docker.*
    dockerOps := starlarkstruct.FromStringDict(starlark.String("docker"), starlark.StringDict{
        "pull":  starlark.NewBuiltin("pull", d.dockerPullBuiltin),
        "build": starlark.NewBuiltin("build", d.dockerBuildBuiltin),
    })

    return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
        "package":    packageOps,
        "file":       fileOps,
        "docker":     dockerOps,  // Add your namespace
        "service":    starlark.NewBuiltin("service", d.serviceBuiltin),
        "shell":      starlark.NewBuiltin("shell", d.shellBuiltin),
        "depends_on": starlark.NewBuiltin("depends_on", d.dependsOnBuiltin),
    })
}
```

### Step 6: Implement Starlark Builtins

Add the builtin functions that bridge Starlark calls to Go methods:

```go
func (d *DarwinPlanBindings) dockerPullBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var image string
    if err := starlark.UnpackArgs("pull", args, kwargs, "image", &image); err != nil {
        return nil, err
    }
    node := d.DockerPull(image)
    return nodeToStarlark(node), nil
}
```

## Starlark API Convention

Operations are exposed via nested structs under `plan`:

```starlark
def install(package, system, plan):
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

    # Docker operations (example)
    plan.docker.pull("nginx:latest")
    plan.docker.build(context=".", tag="myapp:latest")

    # System operations (not namespaced)
    plan.service(name="nginx", action="start")
    plan.shell("echo hello")
    plan.depends_on(node_a, node_b)
```

## Naming Conventions

| Layer | Convention | Example |
|-------|------------|---------|
| Engine operation | `<namespace>-<action>` | `docker-pull`, `package-install` |
| Go method | `<Namespace><Action>` | `DockerPull`, `PackageInstall` |
| Starlark builtin | `<namespace><Action>Builtin` | `dockerPullBuiltin` |
| Starlark API | `plan.<namespace>.<action>()` | `plan.docker.pull()` |

## Testing

Add tests for your operations in `internal/engine/ops_<namespace>_test.go`:

```go
func TestDockerPullDryRun(t *testing.T) {
    reg := NewRegistry()
    for _, op := range AllOps() {
        reg.Register(op)
    }

    eng := New(reg, Options{DryRun: true})

    graph := &Graph{
        Nodes: []*Node{
            {
                ID:         "test-docker-pull",
                Operations: []string{"docker-pull"},
                Metadata: map[string]string{
                    "image": "nginx:latest",
                },
            },
        },
    }

    results, err := eng.Run(context.Background(), graph)
    if err != nil {
        t.Fatalf("Run failed: %v", err)
    }

    if results[0].Status != StatusCompleted {
        t.Errorf("expected completed, got %s", results[0].Status)
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
def install(package, system, plan):
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
def install(package, system, plan):
    # Install multiple packages
    plan.package.install("curl", "jq", "ripgrep")

    # macOS: specify package manager
    plan.package.install("brew:wget", "cask:iterm2", "port:tree")
```
```

### Documentation Location

| Audience | Location | Content |
|----------|----------|---------|
| Engine developers | `docs/architecture/operation-namespaces.md` | How to implement namespaces |
| Package developers | `docs/guides/lore/plan-bindings.md` | How to use plan.* in Starlark |
| CLI users | `docs/cli/lore/` | Command-line usage |

### Keeping Docs in Sync

The Starlark API documentation should match the implementation. When updating bindings:

1. Update the Go implementation
2. Update `docs/guides/lore/plan-bindings.md` with new/changed functions
3. Add examples showing real usage patterns
4. Note any platform-specific behavior

## Checklist

When adding a new namespace:

- [ ] Create `internal/engine/ops_<namespace>.go` with operations
- [ ] Add `<Namespace>Ops()` function returning all operations
- [ ] Update `AllOps()` in `ops.go` to include your operations
- [ ] Update `PlanBindings` interface in `internal/starlark/interfaces.go`
- [ ] Implement methods in all platform bindings (darwin, linux, windows)
- [ ] Add nested struct to `ToStarlark()` in all platform bindings
- [ ] Implement Starlark builtin functions
- [ ] Add tests
- [ ] **Document for package developers** in `docs/guides/lore/plan-bindings.md`
- [ ] Update this architecture documentation
