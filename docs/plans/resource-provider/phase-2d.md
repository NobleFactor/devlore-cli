# Phase 2D: Planner.PlanPackages — Wire Manifest Resolution into Writ Deploy

```yaml
title: "Phase 2D: Planner.PlanPackages — Wire Manifest Resolution"
issue: https://github.com/NobleFactor/devlore-cli/issues/131
status: complete
created: 2026-02-15
updated: 2026-02-15
```

## Context

Phase 2C (current branch) deleted the manifest provider, SubgraphBuilder,
ExpandDelegates, and the delegate concept. When writ discovers a
`packages-manifest.yaml` during tree walk, the tree tags the file with a
`["manifest.resolve"]` pipeline in `tree.ProcessingPipeline`. The graph builder
then called `reg.MustGet("manifest.resolve")` — which panicked because no
provider registers this action. The deploy command comment said "NOT YET
IMPLEMENTED".

Phase 2D replaces this dead path with a real one: writ hands discovered
manifest files to lore's new `Planner`, which resolves packages and adds
installation nodes + phases to the graph.

## Design

### lore.Planner

New struct in `lore/builder.go` that encapsulates package resolution.
Extracted from `Build()` — the core loop that parses a manifest, resolves
each package from the registry, and adds nodes/phases to a graph.

```go
type Planner struct {
    Platform       string
    ActionRegistry *execution.ActionRegistry
    RegistryClient *lorepackage.Registry
    Features       []string
    Settings       map[string]string
    DryRun         bool
}

func (p *Planner) PlanPackages(graph *execution.Graph, manifestPath string) ([]string, error)
func (p *Planner) PlanByName(graph *execution.Graph, packages []string) ([]string, error)
```

`PlanPackages` parses the manifest, resolves each entry (merging per-package
`With` features via existing `mergeFeatures()`), and calls `buildPackageNodes`
to add nodes + phases to the graph. Returns the resolved package names.

`PlanByName` takes explicit package names instead of a manifest file.

`Build()` is refactored to create a `Planner` internally and delegate:
- ManifestPath → `p.PlanPackages(graph, cfg.ManifestPath)`
- Packages → `p.PlanByName(graph, cfg.Packages)`

This keeps `Build()`'s interface and behavior unchanged.

### BuildTree Change

```go
func BuildTree(g *execution.Graph, cfg *Config, reg *execution.ActionRegistry) (manifests []string, err error)
```

Returns manifest source paths instead of creating nodes for them. In the file
iteration loop, when `ops[0] == "manifest.resolve"`, append `f.Source` to
`manifests` and `continue` (skip node creation).

### DeployGraphBuilder Change

```go
type DeployGraphBuilder struct {
    config  *Config
    reg     *execution.ActionRegistry
    Planner *lore.Planner   // nil means skip manifest resolution
}
```

In `Build()`, after `BuildTree`, iterates manifests and calls
`b.Planner.PlanPackages(g, path)` for each.

### Writ Deploy Wiring

`runDeployV2` in `writ/commands.go` creates a `lore.Planner` and passes it to
the graph builder. The writ package gains a new import of `internal/lore`.
This is unidirectional (writ → lore; lore does NOT import writ).

### manifest.resolve Cleanup

Manifest files no longer produce nodes in the execution graph. Removed
`manifest.resolve` from:

- `stateview.go:isPackageNode` — removed from case list
- `graph.go:ComputeSummary` — removed from case list
- `execution_test.go:TestPreflightPackagesManifest` — deleted test
- `stateview_test.go` — removed from test table

**Kept** `manifest.resolve` in the tree package as a detection sentinel:
- `tree/node.go:ProcessingPipeline` — still returns `["manifest.resolve"]`
- `tree/builder.go` — still validates manifest files via `hasAction`

## Steps

### 1. Add Planner struct and PlanPackages/PlanByName to lore/builder.go

Added `Planner` struct with fields: Platform, ActionRegistry, RegistryClient,
Features, Settings, DryRun.

Added `PlanPackages(graph, manifestPath)`: loads manifest, resolves each
entry via registry, merges per-package `With` features, calls
`buildPackageNodes`.

Added `PlanByName(graph, packages)`: resolves explicit package names.

Added private `resolve()` helper: auto-detects platform, auto-creates
ActionRegistry and RegistryClient when nil.

### 2. Refactor Build() to use Planner

`Build()` creates a `Planner` from `BuildConfig` fields, then delegates.
`BuildFromManifest` and `BuildFromPackages` stay as thin wrappers.

### 3. Change BuildTree to return manifest paths

Changed signature to `(manifests []string, err error)`. Added manifest
detection in file loop. Updated all callers.

### 4. Update DeployGraphBuilder

Added `Planner *lore.Planner` field. In `Build()`, after BuildTree, iterates
manifests and calls `b.Planner.PlanPackages(g, m)` for each.

### 5. Wire Planner in runDeployV2

Added `lore` import. Created `lore.Planner{ActionRegistry: reg}` and set on
graph builder. Updated deploy command help text.

### 6. Remove manifest.resolve from execution runtime

Removed from `isPackageNode` (stateview.go) and `ComputeSummary` (graph.go).

### 7. Update tests

- Deleted `TestPreflightPackagesManifest`
- Removed `manifest.resolve` from `isPackageNode` test table
- Added `TestPlanner_PlanPackages`, `TestPlanner_PlanByName`, `TestMergeFeatures`

## Files Modified

| File | Change |
|---|---|
| `lore/builder.go` | Add Planner, PlanPackages, PlanByName, resolve(); refactor Build() |
| `writ/graph_builder.go` | BuildTree returns manifests; DeployGraphBuilder gets Planner |
| `writ/commands.go` | Import lore, create Planner, update help text |
| `execution/stateview.go` | Remove manifest.resolve from isPackageNode |
| `execution/graph.go` | Remove manifest.resolve from ComputeSummary |
| `execution/execution_test.go` | Delete TestPreflightPackagesManifest |
| `execution/stateview_test.go` | Remove manifest.resolve test entry |
| `lore/builder_test.go` | Add 3 Planner tests |

## What This PR Does NOT Touch

- tree/node.go ProcessingPipeline — manifest.resolve stays as detection sentinel
- tree/builder.go validation — manifest validation stays
- tree tests — all pass unchanged
- lore/commands.go — lore deploy has its own pipeline, doesn't use Planner (yet)
- Reconcile, decommission, adopt, migrate graph builders — no manifest handling

## Deviations from Plan

### Method naming

The original parent plan called this `PlanSequence`. Implementation uses
`PlanPackages` (manifest file) and `PlanByName` (explicit names) — clearer
API surface.

### mergeFeatures reuse

The plan called for implementing `mergeFeatures` in `builder.go`. The function
already existed in `lore/commands.go`. Reused it directly.

## Related Documents

- [Resource Provider Plan](../resource-provider.md) — Parent plan
- [Phase 2C](./phase-2c.md) — Typed Action Contract
