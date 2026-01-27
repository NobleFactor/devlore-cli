# Uniform Pipeline Interface — Design

**Date:** 2026-01-26
**Status:** Draft

## Summary

Every `LorePackage` provides a uniform pipeline interface. The engine's package-manifest delegate queries packages for pipeline phases and adds them to the execution graph without knowing whether the package is from the lore registry or a native package manager.

## Key Design Points

1. **Uniform interface** — `LorePackage.PhaseActions()` returns executable phases regardless of source
2. **Polymorphic implementation** — Lore packages return Starlark scripts; native PM packages return PM actions
3. **Required phases** — Each operation has exactly one required phase; native packages implement only this phase
4. **Graph-centric execution** — Phases are added to an execution graph, not executed directly
5. **Optimization opportunity** — Native PM operations can be batched at graph compilation time

## Required Phases

Each pipeline operation has exactly one **required phase** that must be implemented. Native PM packages implement only the required phase; lore packages may implement additional phases.

| Operation | Required Phase | Native PM Action | Optional Phases |
|-----------|---------------|------------------|-----------------|
| Deploy | `install` | `pm install` | prepare, provision, verify |
| Upgrade | `upgrade` | `pm upgrade` | prepare, migrate, verify |
| Decommission | `uninstall` | `pm remove` | unprovision, cleanup |

The **migrate** phase (Upgrade only) handles version-specific changes:
- Config file format migrations
- Data schema updates
- Service reconfiguration after upgrade

This is distinct from **provision** (Deploy only) which handles first-time setup.

This creates a clean separation:
- **Native PM packages**: Implement exactly the required phase (the PM does the work)
- **Lore packages**: May implement any/all phases (Starlark scripts do the work)

```go
// RequiredPhase returns the required phase for an operation.
func RequiredPhase(op Operation) string {
    switch op {
    case OpDeploy:       return "install"
    case OpUpgrade:      return "upgrade"
    case OpDecommission: return "uninstall"
    }
}
```

## Phase Representation

A phase action is either a Starlark script or a native package manager operation:

```go
// PhaseAction represents an executable phase action.
type PhaseAction interface {
    Type() PhaseActionType  // ScriptAction or NativePMAction
}

// ScriptAction executes a Starlark phase script.
type ScriptAction struct {
    Path  string  // Path to .star file
    Phase string  // Phase name (function to call)
}

// NativePMAction executes a native package manager operation.
type NativePMAction struct {
    Manager   PackageSource  // apt, dnf, brew, winget, etc.
    Operation PMOperation    // Install, Upgrade, Remove
    Packages  []string       // Package names
}
```

## Pipeline Resolution

### Lore Registry Package

For a lore package on Darwin:

```
docker/
├── lifecycle.yaml
├── Common/Deploy/
│   └── install.star    → ScriptAction{Path: "Common/Deploy/install.star", Phase: "install"}
├── Unix/Deploy/
│   └── install.star    → ScriptAction{Path: "Unix/Deploy/install.star", Phase: "install"}
└── Darwin/Deploy/
    └── install.star    → ScriptAction{Path: "Darwin/Deploy/install.star", Phase: "install"}
```

Result: Three `ScriptAction` items executed in order (general → specific).

### Native PM Package (Synthetic)

For a native package "curl" on Debian:

```
LorePackage{
    Name:   "curl",
    Source: SourceApt,
}
```

Pipeline returns only the required phase for each operation:

```
Deploy.install    → NativePMAction{apt, Install, ["curl"]}
Upgrade.upgrade   → NativePMAction{apt, Upgrade, ["curl"]}
Decommission.uninstall → NativePMAction{apt, Remove, ["curl"]}
```

Other phases (prepare, provision, verify, etc.) return empty for native packages.

## Graph Integration

The engine's delegate builds an execution graph:

```go
func (d *Delegate) ProcessManifest(manifest *Manifest) error {
    for _, pkgName := range manifest.Packages {
        pkg, _ := d.registry.Resolve(pkgName, d.platform)

        for _, phase := range registry.DeployPhaseOrder {
            actions := pkg.PhaseActions(d.platform, registry.OpDeploy, phase)
            for _, action := range actions {
                d.graph.AddNode(action)
            }
        }
    }
    return nil
}
```

The delegate doesn't distinguish between lore and native packages—both provide `PhaseAction` items.

## Graph Optimization (TODO)

Native PM operations can be batched during graph compilation:

```
Before optimization:
  NativePMAction{apt, Install, ["curl"]}
  NativePMAction{apt, Install, ["wget"]}
  NativePMAction{apt, Install, ["jq"]}

After optimization:
  NativePMAction{apt, Install, ["curl", "wget", "jq"]}
```

Constraints:
- Only batch operations with same manager and operation type
- Respect dependency ordering (if package B depends on A, A must install first)
- Lore package phases cannot be batched (Starlark execution is sequential)

**TODO:** Implement graph optimizer that identifies and batches homogeneous native PM operations.

## Implementation Plan

1. Define `PhaseAction` interface and concrete types in `internal/registry/`
2. Add `PhaseActions(platform, op, phase)` method to `Lifecycle`
3. Update `LorePackage` to delegate to `Lifecycle.PhaseActions()`
4. For synthetic lifecycles, return `NativePMAction` for install phase
5. Update executor to handle both action types
6. (Future) Implement graph optimizer for batching
