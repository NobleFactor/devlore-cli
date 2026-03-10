---
title: "Hermeticity Guarantees"
issue:
status: draft
created: 2026-03-09
updated: 2026-03-09
---

# Plan: Hermeticity Guarantees

## Summary

Make writ graph planning hermetic by formalizing input boundaries and separating execution by target scope. Planning builds a single target tree from the 2×3 matrix of (System, Home) × (base, team, personal), then extracts execution graphs grouped by confinement reachability. Each layer source is pinned to a git commit hash for immutable, content-addressed snapshots.

## Goals

1. **Hermetic planning** — same inputs produce the same graph on any machine, every time
2. **Scope-separated execution** — System and Home graphs execute with their own `Context`, `Root`, and `RecoverySite`
3. **Git-native snapshots** — commit hashes as input identity; worktrees for immutable planning views
4. **Scoped receipts and state** — receipts record target scope; state view filters by scope

## Current State

| Component | Status | Notes |
|---|---|---|
| `op.Root` interface | Complete | `confinedRoot`, `RootReader`, `RootReaderWriter` (os-root-scoping plan) |
| `op.RecoverySite` | Complete | Shared service, context-aware, scoped to root |
| `Context` | Complete | Holds `Root`, `RecoverySite`, `Catalog` — one per execution |
| Executor | Complete | Creates context, opens confined root, runs graph |
| `CollectLayerSources()` | Working | Returns flat `[]LayerSource` with `TargetName` field (System/Home) |
| `buildMultiSource()` | Working | Processes all sources into one `entriesByTarget` map |
| Tree builder collision detection | Working | Layer precedence + specificity, metadata-only comparison |
| Receipt writing | Working | `<tool>-<timestamp>.yaml`, no scope tag |
| State view | Working | Loads all receipts, single `FileTree` with one `Root` |
| Git worktree support | Missing | No snapshot mechanism for layer sources |
| Multi-scope orchestration | Missing | Single graph, single executor, single receipt per deploy |

## Implementation Phases

### Phase 1: GraphContext Scope Identity

Add scope identity to `GraphContext` so receipts record which target scope they represent. Add `Scope` field to `GraphContext` and populate it during graph construction.

- [ ] Add `Scope string` field to `GraphContext` (`json:"scope,omitempty" yaml:"scope,omitempty"`)
- [ ] Populate `Scope` in `NewGraph()` — accept scope parameter or set from config
- [ ] Update `Graph.Filename()` to include scope: `<tool>-<scope>-<timestamp>.yaml`
- [ ] Update `LatestReceiptPath()` to include scope: `<producer>-<scope>-latest.yaml`
- [ ] Tests: verify filename format, receipt round-trip with scope

**Files**:

- `pkg/op/graph.go` — Modify: `GraphContext.Scope`, `Filename()`
- `internal/cli/receipts.go` — Modify: `LatestReceiptPath()`, `WriteReceipt()` symlink naming
- `internal/cli/receipts_test.go` — Modify: update for new filename format
- `pkg/op/graph_test.go` — Modify: if filename tests exist

### Phase 2: Partition Layer Sources by Target Scope

Split `CollectLayerSources()` output into per-scope source lists. This is the prerequisite for building separate graphs.

- [ ] Add `PartitionByScope(sources []tree.LayerSource) map[string][]tree.LayerSource` — keys are `TargetName` values ("System", "Home")
- [ ] Update `parseDeployConfig()` to store partitioned sources (or partition at build time)
- [ ] Tests: partition with mixed System/Home sources, partition with only Home, partition with empty layers

**Files**:

- `internal/writ/layer.go` — Modify: add `PartitionByScope()`
- `internal/writ/layer_test.go` — Create or modify: partition tests

### Phase 3: Multi-Scope Graph Building

Refactor `DeployGraphBuilder.Build()` to produce one graph per scope. The planning tree is still unified for collision detection; graph extraction happens after.

- [ ] Change `DeployGraphBuilder.Build()` return to `([]*op.Graph, error)` — one graph per populated scope
- [ ] Build unified tree via `tree.Build()` with all sources (collision detection needs cross-scope visibility)
- [ ] After tree build, partition `BuildResult.Files` by scope (using `FileEntry.Layer` + source's `TargetName`)
- [ ] Create one `op.Graph` per scope, populating `GraphContext.Scope` and `GraphContext.TargetRoot`
- [ ] Route manifests to the correct scope's graph
- [ ] Update `NewGraph()` to accept scope and target root
- [ ] Tests: multi-scope build produces correct graphs, collision detection works across scopes

This requires the tree builder to preserve scope information on `FileEntry`. Currently `FileEntry` has `Layer` but not `TargetName`.

- [ ] Add `TargetName string` to `tree.FileEntry` (or `Scope string`) — set during `walkDirectory()`
- [ ] Propagate from `LayerSource.TargetName` into each entry

**Files**:

- `internal/writ/tree/builder.go` — Modify: `FileEntry.TargetName`, propagate in walk
- `internal/writ/graph_builder.go` — Modify: `Build()` returns `[]*op.Graph`, partition by scope
- `internal/writ/graph_test.go` — Modify: multi-scope graph building tests

### Phase 4: Multi-Scope Execution Orchestration

Update the deploy command to execute multiple graphs in sequence, each with its own executor.

- [x] Refactor `runDeployV2()` to iterate over graphs from `Build()` (done in Phase 3)
- [x] For each graph: create executor via `ConfigureEngine()` with that graph's `TargetRoot`
- [x] Execute in deterministic order: system first, then home (`sortGraphsByScope`)
- [x] Write one receipt per graph (scope-tagged via Phase 1)
- [x] Handle failure policy: if a graph fails, log and continue to next graph (fail-forward for independent scopes)
- [x] Tests: scope ordering, unscoped-last behavior

**Dropped**: Confinement grouping (splitting Home into 1–3 graphs based on layer repo reachability) and `isReachableFrom()`. Phase 6 git worktree snapshots solve reachability by placing snapshots within `$HOME`, making all sources reachable from the confined root. This collapses the 1–3 Home graph model to always 1 Home graph. See design note below.

> **Design note — confinement reachability**: An executor confined to `$HOME` via `os.Root` can read sources within `$HOME` but not outside it. For sources outside `$HOME`: symlinks work (the link file is within `$HOME`; the target path is just a string), but copy/template/decrypt operations fail because they must read source content. Rather than splitting into separate unconfined graphs, Phase 6 solves this by placing git worktree snapshots within `$HOME`, ensuring all sources are reachable under confinement.

**Files**:

- `internal/writ/commands.go` — Modify: `runDeployV2()` scope ordering, per-graph engine, fail-forward
- `internal/writ/graph_builder.go` — Modify: `ConfigureEngine()` accepts per-graph target root

### Phase 5: Scope-Aware State View

Update state view to filter by scope, so decommission and reconcile operate on the correct scope.

- [x] Add `Scope` filter to `ViewOptions`
- [x] `includeGraph()` filters by `GraphContext.Scope` when `ViewOptions.Scope` is set
- [x] Update `FileTree.Root` to come from the filtered graph's `TargetRoot` (already works — `BuildFrom` takes first filtered graph's root)
- [x] `loadStateView()` accepts scope parameter; callers pass scope or empty for all
- [x] Add `DistinctScopes()` to discover unique scopes from receipts
- [x] Update `runDecommission()` to discover scopes, build per-scope state views, execute per-scope decommission graphs with fail-forward
- [x] Tests: scope filtering (in-memory and from disk), combined scope+tool filter, `DistinctScopes` with mixed/unscoped receipts

**Dropped**: `processGraph()` changes for multiple target roots. Per-scope filtering ensures each `StateView` has a consistent `Root` — no need to handle mixed roots within one view.

**Files**:

- `internal/execution/stateview.go` — Modify: `ViewOptions.Scope`, scope filtering, `DistinctScopes()`
- `internal/execution/stateview_test.go` — Modify: 6 new scope-aware tests
- `internal/writ/commands.go` — Modify: `loadStateView` scope param, `discoverScopes`, per-scope decommission

### Phase 6: Git Worktree Snapshots

Pin each layer source to a git commit hash during planning. Create detached worktrees for immutable planning views. Worktrees are placed under `${XDG_CACHE_HOME}/devlore/snapshots/`.

- [x] Create `internal/writ/snapshot` package
- [x] `Snapshot` struct: `Layer`, `RepoPath`, `CommitHash`, `WorktreePath`
- [x] `Pin(repoPath, layer)` — resolve HEAD commit, create detached worktree at `snapshots/<layer>-<hash[:12]>/`
- [x] `(*Snapshot).Close()` — remove worktree via `git worktree remove`, fallback to `RemoveAll` + prune
- [x] `PinAll(sources)` — pin each unique repo once (dedup by `LayerSource.Path`), return cleanup func
- [x] `RewriteSources(sources, snapshots)` — rewrite `SourceRoot` to worktree paths preserving subdirectory
- [x] `Hashes(snapshots)` — returns `layer → hash` map for `GraphContext.CommitHashes`
- [x] Add `CommitHashes map[string]string` to `GraphContext` (layer → full commit hash)
- [x] Wire into `runDeployV2()`: pin after config, rewrite sources, record hashes on graphs, defer cleanup
- [x] Tests: pin/close lifecycle, worktree excludes uncommitted changes, worktree reuse, PinAll dedup with shared repo, PinAll with distinct repos, RewriteSources, Hashes

**Files**:

- `internal/writ/snapshot/snapshot.go` — Create: `Snapshot`, `Pin`, `PinAll`, `RewriteSources`, `Hashes`, `Close`
- `internal/writ/snapshot/snapshot_test.go` — Create: 7 tests with real git repos
- `pkg/op/graph.go` — Modify: `GraphContext.CommitHashes`
- `internal/writ/commands.go` — Modify: pin layers before planning, rewrite sources, record hashes, cleanup

### Phase 7: Dirty Tree Policy

Implement the `--allow-dirty` flag and dirty-tree detection.

- [x] Add `IsDirty(repoPath)` to snapshot package — checks `git status --porcelain` for staged, unstaged, and untracked
- [x] Add `CheckClean(sources)` — checks all unique repos, returns dirty layer names
- [x] Default behavior: refuse to plan if any layer has uncommitted changes
- [x] `--allow-dirty` flag on deploy command: warn but allow, using HEAD
- [x] Add `AllowDirty` to `DeployConfig`, parse from flag
- [x] Record dirty state in `GraphContext.DirtyLayers []string` — present only when `--allow-dirty` used
- [x] Tests: clean repo, unstaged changes, staged changes, untracked files, CheckClean with all clean, CheckClean with mixed dirty, CheckClean dedup on shared repo

**Files**:

- `internal/writ/snapshot/snapshot.go` — Modify: add `IsDirty()`, `CheckClean()`
- `internal/writ/snapshot/snapshot_test.go` — Modify: 7 new dirty detection tests
- `internal/writ/commands.go` — Modify: `--allow-dirty` flag, dirty check before pinning, record dirty layers
- `internal/writ/config.go` — Modify: parse `--allow-dirty` flag
- `internal/writ/graph_types.go` — Modify: `DeployConfig.AllowDirty`
- `pkg/op/graph.go` — Modify: `GraphContext.DirtyLayers`

### Phase 8: Ad-Hoc E2E Validation

Validate multi-scope deploy end-to-end using simulated layer repos with real content.

- [ ] Run `scripts/setup-test-layers.sh` to create base, team, and personal repos with Home/ and System/ directories
- [ ] Content: base has foundational config (git, vim, shell profile, system files), team has NobleFactor-specific config, personal draws non-secret files from `~/Workspace/Personal/Home/Configs`
- [ ] Execute `writ deploy noblefactor` against a fake `$HOME` with XDG paths pointing to the test root
- [ ] Verify: correct files deployed, correct collision winners, system files in system graph, home files in home graph
- [ ] Verify: scoped receipts written, state view filters correctly
- [ ] No lore packages in this round — file deployment only

**Files**:

- `scripts/setup-test-layers.sh` — Created: sets up 3 git repos, layer symlinks, fake home

## Files to Create/Modify

| File | Action | Purpose |
|---|---|---|
| `pkg/op/graph.go` | Modify | `GraphContext.Scope`, `GraphContext.CommitHashes`, `Filename()` |
| `internal/cli/receipts.go` | Modify | Scope-tagged receipt naming and symlinks |
| `internal/writ/layer.go` | Modify | `PartitionByScope()`, `isReachableFrom()` |
| `internal/writ/tree/builder.go` | Modify | `FileEntry.TargetName` propagation |
| `internal/writ/graph_builder.go` | Modify | `Build()` returns `[]*op.Graph`, per-scope graph construction |
| `internal/writ/graph_types.go` | Modify | Config changes if needed |
| `internal/writ/commands.go` | Modify | Multi-graph orchestration, `--allow-dirty` flag |
| `internal/execution/stateview.go` | Modify | `ViewOptions.Scope`, scope filtering |
| `internal/writ/snapshot/snapshot.go` | Create | Git worktree snapshot lifecycle |
| `internal/writ/snapshot/snapshot_test.go` | Create | Snapshot tests |
| `scripts/setup-test-layers.sh` | Created | Simulated layer repos for ad-hoc e2e testing |

## Related Documents

- [2.4-hermeticity-guarantees.md](../architecture/2.4-hermeticity-guarantees.md) — Architecture document (problem framing, resolved decisions, risk areas)
- [os-root-scoping.md](os-root-scoping.md) — Foundation: `op.Root`, `op.Path`, `RecoverySite` (complete)
- [resource-management.md](resource-management.md) — Resource model

## Open Questions

- [ ] Plan caching: the `(base_hash, team_hash, personal_hash, scope)` cache key enables skipping planning entirely when nothing changed. Implement in this plan or defer?

## Resolved Questions

- **Confinement reachability**: Resolved — no graph splitting needed. Git worktree snapshots (Phase 6) are placed within `$HOME`, making all sources reachable from the confined root. An executor confined to `$HOME` can't read sources outside `$HOME` (copy/template/decrypt fail), but symlinks work because `os.Symlink` doesn't read the source. Snapshots solve this for all operation types.
- **Failure policy**: Resolved — fail-forward. Independent scopes continue on failure. System and home target different roots with different confinement; a failed system graph does not block home execution.
