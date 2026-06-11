// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/sops"
)

// BuildTree walks the source directories and populates `g` with file nodes, returning the
// manifest source paths discovered during the walk (those are deferred to the planner, not added
// as nodes).
//
// Layers are processed in source order (base → team → personal) with cross-layer collision
// detection; winning entries become nodes and losers are recorded on `g.Collisions`.
//
// Parameters:
//   - `g`: the target graph to populate.
//   - `cfg`: the resolved writ configuration; supplies source fsroot, target fsroot, layer sources,
//     projects, and segments to the tree builder.
//   - `reg`: the receiver registry used to materialize each file's action chain.
//
// Returns:
//   - []string: the manifest source paths discovered during the tree walk.
//   - `error`: non-nil when the tree build or node construction fails.
func BuildTree(g *op.Graph, cfg *Config, reg *op.ReceiverRegistry) ([]string, error) {

	result, err := tree.Build(tree.BuildConfig{
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Sources:    cfg.LayerSources,
		Projects:   cfg.Projects,
		Segments:   cfg.Segments,
	})
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	manifests, err := populateGraphNodes(g, result.Files, reg)
	if err != nil {
		return nil, err
	}
	recordCollisions(g, result.Collisions)
	return manifests, nil
}

// ConfigureSpec builds the [*op.RuntimeEnvironmentSpec] used to construct a per-graph
// [*op.GraphExecutor]. The `targetRoot` parameter overrides `cfg.TargetRoot` so each scope's
// graph can execute against its own fsroot (e.g., "/" for system, "$HOME" for home).
//
// Callers construct the executor inline: `op.NewGraphExecutor(graph, spec)`. The executor binds
// graph and spec at construction; each Run builds a fresh per-Run env from the spec and clones
// the graph's catalog onto it.
//
// Parameters:
//   - `cfg`: the resolved writ configuration; supplies template data, source fsroot, and the
//     application flag map.
//   - `targetRoot`: the fsroot directory for this scope's executor; overrides `cfg.TargetRoot`.
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the configured spec.
//   - `error`: non-nil when the target fsroot cannot be opened.
func ConfigureSpec(cfg *Config, targetRoot string) (*op.RuntimeEnvironmentSpec, error) {

	sopsClient, _ := sops.NewClient(cfg.SourceRoot) //nolint:errcheck // nil when no .sops.yaml found

	root, err := fsroot.OpenConfined(targetRoot)
	if err != nil {
		return nil, fmt.Errorf("open fsroot %s: %w", targetRoot, err)
	}

	return op.NewRuntimeEnvironmentSpec("writ").
		WithRoot(root).
		WithSops(sopsClient).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: map[string]any{"dry-run": cfg.DryRun},
		}), nil
}

// NewGraph constructs an [*op.Graph] with `cfg`-derived origin fields populated for writ.
//
// Parameters:
//   - `cfg`: the resolved writ configuration; supplies tool name, source fsroot, target fsroot,
//     projects, and segments.
//
// Returns:
//   - *op.Graph: the constructed graph with origin set; the fsroot is empty.
func NewGraph(cfg *Config) *op.Graph {

	g := op.NewGraph()
	g.Origin = op.Origin{
		Tool:       cfg.Tool,
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Projects:   cfg.Projects,
		Segments:   cfg.SegmentMap(),
	}
	return g
}

// NewScopedGraph constructs an [*op.Graph] tagged with a target scope and a per-scope fsroot, used
// for multi-scope graph building where each scope gets its own graph.
//
// Parameters:
//   - `cfg`: the resolved writ configuration; supplies tool name, source fsroot, projects, and
//     segments. `cfg.TargetRoot` is overridden by `targetRoot` on the returned graph.
//   - `scope`: the target scope identifier (e.g., "system", "home").
//   - `targetRoot`: the per-scope target fsroot path (e.g., "/" or "$HOME").
//
// Returns:
//   - *op.Graph: the constructed graph with origin set; the fsroot is empty.
func NewScopedGraph(cfg *Config, scope, targetRoot string) *op.Graph {

	g := op.NewGraph()
	g.Origin = op.Origin{
		Tool:       cfg.Tool,
		Scope:      scope,
		SourceRoot: cfg.SourceRoot,
		TargetRoot: targetRoot,
		Projects:   cfg.Projects,
		Segments:   cfg.SegmentMap(),
	}
	return g
}

// populateGraphNodes converts file entries into graph nodes on `g`. Multi-op pipelines (e.g.,
// `["decrypt", "render", "copy"]`) become node chains with explicit edges; single-op entries
// become a single node. Manifest files (a single `manifest.resolve` action) are collected for
// later planner resolution rather than turned into nodes.
//
// Parameters:
//   - `g`: the target graph to populate.
//   - `files`: the file entries from the tree build.
//   - `reg`: the receiver registry used to build each file's action chain.
//
// Returns:
//   - []string: the manifest source paths collected from the entries.
//   - `error`: non-nil when the registry cannot build an action for a listed operation name.
func populateGraphNodes(g *op.Graph, files []*tree.FileEntry, reg *op.ReceiverRegistry) ([]string, error) { //nolint:gocognit

	var manifests []string

	for _, f := range files {
		actions := f.Operations

		if len(actions) == 1 && actions[0] == "manifest.resolve" {
			manifests = append(manifests, f.Source)
			continue
		}

		if len(actions) == 1 {
			singleAction, err := reg.BuildAction(actions[0])
			if err != nil {
				return nil, fmt.Errorf("populateGraphNodes %s: %w", actions[0], err)
			}
			node := op.NewNode(f.ID, singleAction)
			node.Origin = f.Project
			node.Layer = f.Layer
			node.SetSlot("source", op.ImmediateValue{Value: f.Source})
			node.SetSlot("path", op.ImmediateValue{Value: f.Target})
			if f.Mode != 0 {
				node.SetSlot("mode", op.ImmediateValue{Value: f.Mode})
			}
			g.AddNode(node)
			continue
		}

		var prevNode *op.Node
		for i, action := range actions {
			isLast := i == len(actions)-1
			nodeID := f.ID
			if !isLast {
				nodeID = f.ID + ":" + action
			}

			built, err := reg.BuildAction(action)
			if err != nil {
				return nil, fmt.Errorf("populateGraphNodes %s: %w", action, err)
			}
			node := op.NewNode(nodeID, built)
			node.Origin = f.Project
			node.Layer = f.Layer
			if i == 0 {
				node.SetSlot("source", op.ImmediateValue{Value: f.Source})
			}
			if isLast {
				node.SetSlot("path", op.ImmediateValue{Value: f.Target})
				if f.Mode != 0 {
					node.SetSlot("mode", op.ImmediateValue{Value: f.Mode})
				}
			}
			g.AddNode(node)

			if prevNode != nil {
				g.Root.AddEdge(op.Edge{From: prevNode.ID(), To: node.ID()})
			}
			prevNode = node
		}
	}

	return manifests, nil
}

// recordCollisions copies tree collisions into `g.Collisions` as planning metadata so downstream
// tooling can surface conflicts without re-running the tree walk.
//
// Parameters:
//   - `g`: the target graph.
//   - `collisions`: the collisions reported by the unified tree build.
func recordCollisions(g *op.Graph, collisions []tree.Collision) {

	for _, c := range collisions {
		g.Collisions = append(g.Collisions, op.Collision{
			Target:            c.Target,
			Winner:            c.Winner,
			WinnerLayer:       c.WinnerLayer,
			WinnerSpecificity: c.WinnerSpecificity,
			Loser:             c.Loser,
			LoserLayer:        c.LoserLayer,
			LoserSpecificity:  c.LoserSpecificity,
		})
	}
}

// resolveManifests delegates each manifest source path to the planner for package resolution. A
// nil planner or empty list is a no-op.
//
// Parameters:
//   - `g`: the target graph to add package nodes to.
//   - `planner`: the lore planner; nil skips resolution.
//   - `manifests`: the manifest source paths to resolve.
//
// Returns:
//   - `error`: non-nil when the planner fails on any manifest; wraps the failing source path.
func resolveManifests(g *op.Graph, planner *lore.Planner, manifests []string) error {

	if planner == nil || len(manifests) == 0 {
		return nil
	}
	for _, m := range manifests {
		if _, err := planner.PlanPackages(g, m); err != nil {
			return fmt.Errorf("manifest %s: %w", m, err)
		}
	}
	return nil
}

// scopeLayers returns the unique layer names that contribute to `scope`, in source order
// (base → team → personal).
//
// Parameters:
//   - `sources`: all layer sources to filter.
//   - `scope`: the target scope name to filter by (e.g., "System", "Home").
//
// Returns:
//   - []string: the unique layer names in source order; empty when no source targets `scope`.
func scopeLayers(sources []tree.LayerSource, scope string) []string {

	seen := make(map[string]bool)
	var layers []string
	for _, src := range sources {
		if src.TargetName == scope && !seen[src.Layer] {
			seen[src.Layer] = true
			layers = append(layers, src.Layer)
		}
	}
	return layers
}

// sortedKeys returns the keys of `m` in ascending lexical order.
//
// Parameters:
//   - `m`: the map whose keys to sort.
//
// Returns:
//   - []string: the keys in ascending lexical order.
func sortedKeys(m map[string][]*tree.FileEntry) []string {

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DecommissionGraphBuilder builds execution graphs for decommission operations from a state view.
//
// Decommission walks the state view's recorded file entries (filtered by project, when
// configured) and emits one removal node per entry: `file.unlink` for symlinks, `file.remove` for
// copied files.
type DecommissionGraphBuilder struct {
	config *Config
	reg    *op.ReceiverRegistry
	view   *execution.StateView
}

// NewDecommissionGraphBuilder constructs a [*DecommissionGraphBuilder] bound to the supplied
// configuration, state view, and receiver registry.
//
// Parameters:
//   - `cfg`: the resolved decommission configuration; embedded `*Config` supplies origin.
//   - `view`: the state view supplying recorded file entries to remove.
//   - `reg`: the receiver registry used to materialize `file.unlink` / `file.remove` actions.
//
// Returns:
//   - *DecommissionGraphBuilder: the configured builder.
func NewDecommissionGraphBuilder(
	cfg *DecommissionConfig, view *execution.StateView, reg *op.ReceiverRegistry,
) *DecommissionGraphBuilder {

	return &DecommissionGraphBuilder{
		config: &cfg.Config,
		reg:    reg,
		view:   view,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Build constructs the execution graph for the decommission operation. The graph's target fsroot is
// taken from the state view (not the configuration) so removal operates against the same fsroot the
// state was recorded against.
//
// Returns:
//   - *op.Graph: the assembled graph with one removal node per filtered state entry.
//   - `error`: non-nil when the registry cannot build a `file.unlink` or `file.remove` action.
func (b *DecommissionGraphBuilder) Build() (*op.Graph, error) {

	g := NewGraph(b.config)
	g.Origin.TargetRoot = b.view.Files.Root

	projects := projectSet(b.config.Projects)

	for relTarget, entry := range b.view.Files.Entries {
		if len(projects) > 0 && !projects[entry.Project] {
			continue
		}

		action := "file.unlink"
		if entry.IsCopied() {
			action = "file.remove"
		}

		target := filepath.Join(b.view.Files.Root, relTarget)
		decomAction, err := b.reg.BuildAction(action)
		if err != nil {
			return nil, fmt.Errorf("DecommissionGraphBuilder: %w", err)
		}
		node := op.NewNode(relTarget, decomAction)
		node.Origin = entry.Project
		node.Layer = entry.Layer
		node.SetSlot("source", op.ImmediateValue{Value: entry.Source})
		node.SetSlot("path", op.ImmediateValue{Value: target})

		g.AddNode(node)
	}

	return g, nil
}

// endregion

// endregion

// DeployGraphBuilder builds execution graphs for deploy operations. In single-source mode it
// returns one graph; in multi-source mode it returns one graph per populated target scope.
type DeployGraphBuilder struct {
	config  *Config
	reg     *op.ReceiverRegistry
	Planner *lore.Planner // nil skips manifest resolution
}

// NewDeployGraphBuilder constructs a [*DeployGraphBuilder] bound to the supplied configuration
// and receiver registry. The `Planner` field is left nil; callers wire it in when manifest
// resolution is required.
//
// Parameters:
//   - `cfg`: the resolved deploy configuration; embedded `*Config` supplies origin and layer
//     sources.
//   - `reg`: the receiver registry used to materialize per-file action chains.
//
// Returns:
//   - *DeployGraphBuilder: the configured builder; `Planner` is nil.
func NewDeployGraphBuilder(cfg *DeployConfig, reg *op.ReceiverRegistry) *DeployGraphBuilder {
	return &DeployGraphBuilder{config: &cfg.Config, reg: reg}
}

// region EXPORTED METHODS

// region Behaviors

// Build assembles execution graphs for the deploy operation.
//
// Single-source mode (no layer sources configured) returns a single graph populated from the
// unified tree walk. Multi-source mode partitions the winning entries by target scope and returns
// one graph per populated scope in deterministic (sorted) order.
//
// Returns:
//   - []*op.Graph: one graph per target scope.
//   - `error`: non-nil when the tree build fails, an action cannot be built, or the planner
//     rejects a manifest.
func (b *DeployGraphBuilder) Build() ([]*op.Graph, error) {

	result, err := tree.Build(tree.BuildConfig{
		SourceRoot: b.config.SourceRoot,
		TargetRoot: b.config.TargetRoot,
		Sources:    b.config.LayerSources,
		Projects:   b.config.Projects,
		Segments:   b.config.Segments,
	})
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	if len(b.config.LayerSources) == 0 {
		g := NewGraph(b.config)
		manifests, err := populateGraphNodes(g, result.Files, b.reg)
		if err != nil {
			return nil, err
		}
		recordCollisions(g, result.Collisions)
		if err := resolveManifests(g, b.Planner, manifests); err != nil {
			return nil, err
		}
		return []*op.Graph{g}, nil
	}

	filesByScope := make(map[string][]*tree.FileEntry)
	for _, f := range result.Files {
		filesByScope[f.TargetName] = append(filesByScope[f.TargetName], f)
	}

	scopeTargetRoots := make(map[string]string)
	for _, src := range b.config.LayerSources {
		scopeTargetRoots[src.TargetName] = src.TargetRoot
	}

	scopes := sortedKeys(filesByScope)
	graphs := make([]*op.Graph, 0, len(scopes))
	for _, scope := range scopes {
		files := filesByScope[scope]
		targetRoot := scopeTargetRoots[scope]

		g := NewScopedGraph(b.config, strings.ToLower(scope), targetRoot)
		g.Origin.Layers = scopeLayers(b.config.LayerSources, scope)

		manifests, err := populateGraphNodes(g, files, b.reg)
		if err != nil {
			return nil, err
		}
		recordCollisions(g, result.Collisions)

		if err := resolveManifests(g, b.Planner, manifests); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
	}

	return graphs, nil
}

// endregion

// endregion
