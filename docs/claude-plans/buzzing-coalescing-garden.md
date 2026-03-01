# Phase 2D: Planner.PlanPackages — Wire Manifest Resolution into Writ Deploy

## Context

When writ discovers a `packages-manifest.yaml` during tree walk, it currently
tags the file with a `["manifest.resolve"]` pipeline (in `tree.ProcessingPipeline`).
The graph builder then calls `reg.MustGet("manifest.resolve")` — which panics
because no provider registers this action. The deploy command comment says
"NOT YET IMPLEMENTED".

Phase 2D replaces this dead path with a real one: writ hands discovered manifest
files to lore's new `Planner`, which resolves packages and adds installation
nodes + phases to the graph.

**Worktree**: `devlore-cli.resource-provider`
**Branch**: `feat/flow-dotted-names`
**Base**: `origin/develop`

## Design

### lore.Planner

New struct in `lore/builder.go` that encapsulates package resolution. Extracted
from `Build()` — the core loop that parses a manifest, resolves each package from
the registry, and adds nodes/phases to a graph.

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
```

`PlanPackages` parses the manifest, resolves each entry (merging per-package
`With` features), and calls `buildPackageNodes` to add nodes + phases to the
graph. Returns the resolved package names.

`Build()` is refactored to create a `Planner` internally and call `PlanPackages`
(for manifest path) or a new `PlanByName` (for explicit package list). This
keeps `Build()`'s interface and behavior unchanged.

### BuildTree Change

```go
func BuildTree(g *execution.Graph, cfg *Config, reg *execution.ActionRegistry) (manifests []string, err error)
```

Returns manifest source paths instead of creating nodes for them. Inside the
file iteration loop, when `ops[0] == "manifest.resolve"`, append `f.Source` to
`manifests` and `continue` (skip node creation).

Callers that currently ignore the error return get `_, err =` or `manifests, err =`.

### DeployGraphBuilder Change

Add a `Planner *lore.Planner` field. In `Build()`, after `BuildTree`, iterate
manifests and call `b.Planner.PlanPackages(g, path)` for each.

```go
type DeployGraphBuilder struct {
    config  *Config
    reg     *execution.ActionRegistry
    Planner *lore.Planner   // new — nil means skip manifest resolution
}
```

### Writ Deploy Wiring

`runDeployV2` in `writ/commands.go` creates a `lore.Planner` and passes it to
the graph builder. The writ package gains a new import of `internal/lore`. This
is unidirectional (writ → lore; lore does NOT import writ).

### manifest.resolve Cleanup

Since manifest files no longer produce nodes in the execution graph, remove
`manifest.resolve` from all runtime switch/case sites:

- `stateview.go:isPackageNode` — remove from case list
- `graph.go:ComputeSummary` — remove from case list
- `execution_test.go:TestPreflightPackagesManifest` — delete test entirely
- `stateview_test.go` — remove manifest.resolve entry from test table
- `commands.go:42-43` — update "NOT YET IMPLEMENTED" comment

**Keep** `manifest.resolve` in the tree package as a detection sentinel:
- `tree/node.go:ProcessingPipeline` — still returns `["manifest.resolve"]`
- `tree/builder.go` — still validates manifest files via `hasAction`
- `tree/builder.go:PackagesCount` — still counts manifest entries
- `tree/tree_test.go` — all tree tests stay

## Steps

### 1. Add Planner struct and PlanPackages to lore/builder.go

**File**: `internal/lore/builder.go`

Add `Planner` struct with fields: Platform, ActionRegistry, RegistryClient,
Features, Settings, DryRun.

Add `PlanPackages(graph *execution.Graph, manifestPath string) ([]string, error)`:
- Load manifest via `manifest.Load(manifestPath)`
- Auto-detect platform if empty
- Auto-create RegistryClient if nil
- Auto-create ActionRegistry if nil
- Create host via `host.NewHost()`
- For each `PackageEntry`: merge entry.With with p.Features, resolve from
  registry, call `buildPackageNodes(graph, pkg, h, plat, cfg, reg)`
- Return package names

Add `PlanByName(graph *execution.Graph, packages []string) ([]string, error)`:
- Same as above but takes explicit package names (no manifest parsing)

### 2. Refactor Build() to use Planner

**File**: `internal/lore/builder.go`

`Build()` creates a `Planner` from `BuildConfig` fields, then delegates:
- ManifestPath → `p.PlanPackages(graph, cfg.ManifestPath)`
- Packages → `p.PlanByName(graph, cfg.Packages)`

`BuildFromManifest` and `BuildFromPackages` stay as thin wrappers (unchanged API).

### 3. Change BuildTree to return manifest paths

**File**: `internal/writ/graph_builder.go`

Change signature: `func BuildTree(...) (manifests []string, err error)`

In the file iteration loop, add:
```go
if len(ops) == 1 && ops[0] == "manifest.resolve" {
    manifests = append(manifests, f.Source)
    continue
}
```

Update callers of BuildTree:
- `DeployGraphBuilder.Build()` — capture manifests return
- Any other callers — add `_` for the new return value

### 4. Update DeployGraphBuilder

**File**: `internal/writ/graph_builder.go`

Add `Planner *lore.Planner` field to `DeployGraphBuilder`.

Update `NewDeployGraphBuilder` signature to accept optional planner (or set
via field after construction).

In `Build()`, after BuildTree:
```go
if b.Planner != nil && len(manifests) > 0 {
    for _, m := range manifests {
        pkgs, err := b.Planner.PlanPackages(g, m)
        if err != nil {
            return nil, fmt.Errorf("manifest %s: %w", m, err)
        }
        // pkgs available for display/logging
    }
}
```

### 5. Wire Planner in runDeployV2

**File**: `internal/writ/commands.go`

Add import: `"github.com/NobleFactor/devlore-cli/internal/lore"`

After creating the registry, create a planner:
```go
planner := &lore.Planner{
    ActionRegistry: reg,
}
```

Pass to graph builder:
```go
builder := NewDeployGraphBuilder(cfg, reg)
builder.Planner = planner
g, err := builder.Build()
```

Update the deploy command Long description — remove "NOT YET IMPLEMENTED".

### 6. Remove manifest.resolve from execution runtime

**File**: `internal/execution/stateview.go`
- `isPackageNode`: remove `"manifest.resolve"` from case list

**File**: `internal/execution/graph.go`
- `ComputeSummary`: remove `"manifest.resolve"` from case list

### 7. Update tests

**File**: `internal/execution/execution_test.go`
- Delete `TestPreflightPackagesManifest` (manifest.resolve nodes no longer exist)

**File**: `internal/execution/stateview_test.go`
- Remove `{"manifest.resolve", true}` from the isPackageNode test table

**File**: `internal/lore/builder_test.go`
- Existing tests continue to work (Build() API unchanged)
- Add test: `TestPlanner_PlanPackages` — creates Planner, calls PlanPackages
  with a temp manifest, verifies nodes added to graph

**File**: `internal/writ/graph_builder.go` (or graph_test.go)
- Verify that BuildTree returns manifest paths instead of creating nodes

### 8. Build and test

```bash
cd /Users/david-noble/Workspace/NobleFactor/devlore-cli.resource-provider
go build ./...
go vet ./...
go test ./internal/execution/... -count=1
go test ./internal/lore/... -count=1
go test ./internal/writ/... -count=1
```

## Order of Operations

1. **Steps 1-2**: Planner struct + Build() refactor (lore-only, no writ changes)
2. **Steps 3-4**: BuildTree + DeployGraphBuilder changes (writ)
3. **Step 5**: Wire planner in deploy command (writ imports lore)
4. **Steps 6-7**: Cleanup manifest.resolve references + tests
5. **Step 8**: Build and verify

## Files to Modify

| File | Change |
|---|---|
| `lore/builder.go` | Add Planner struct, PlanPackages, PlanByName; refactor Build() |
| `writ/graph_builder.go` | BuildTree returns manifests; DeployGraphBuilder gets Planner |
| `writ/commands.go` | Import lore, create Planner, update help text |
| `execution/stateview.go` | Remove manifest.resolve from isPackageNode |
| `execution/graph.go` | Remove manifest.resolve from ComputeSummary |
| `execution/execution_test.go` | Delete TestPreflightPackagesManifest |
| `execution/stateview_test.go` | Remove manifest.resolve test entry |
| `lore/builder_test.go` | Add TestPlanner_PlanPackages |

## What This PR Does NOT Touch

- tree/node.go ProcessingPipeline — manifest.resolve stays as detection sentinel
- tree/builder.go validation — manifest validation stays
- tree tests — all pass unchanged
- lore/commands.go — lore deploy has its own pipeline, doesn't use Planner (yet)
- Reconcile, decommission, adopt, migrate graph builders — no manifest handling
