// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// CurrentVersion is the graph format version (delegates to op.GraphFormatVersion).
const CurrentVersion = op.GraphFormatVersion

// NewGraph creates an op.Graph with common fields populated for writ.
//
// Parameters:
//   - cfg: resolved writ configuration
//
// Returns:
//   - *op.Graph: graph with context populated from config
func NewGraph(cfg *Config) *op.Graph {

	g := op.NewGraph(nil)
	g.Provenance = op.Provenance{
		Tool:       cfg.Tool,
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Projects:   cfg.Projects,
		Segments:   cfg.SegmentMap(),
	}
	return g
}

// NewScopedGraph creates an op.Graph tagged with a target scope and root.
// Used for multi-scope graph building where each scope gets its own graph.
//
// Parameters:
//   - cfg: resolved writ configuration
//   - scope: target scope identifier ("system", "home")
//   - targetRoot: target root path for this scope ("/" or "$HOME")
//
// Returns:
//   - *op.Graph: graph with scope and target root set
func NewScopedGraph(cfg *Config, scope, targetRoot string) *op.Graph {

	g := op.NewGraph(nil)
	g.Provenance = op.Provenance{
		Tool:       cfg.Tool,
		Scope:      scope,
		SourceRoot: cfg.SourceRoot,
		TargetRoot: targetRoot,
		Projects:   cfg.Projects,
		Segments:   cfg.SegmentMap(),
	}
	return g
}

// BuildTree walks the source directories and populates the graph with file nodes.
// This processes layers in order (base → team → personal) with collision detection.
// Returns discovered manifest source paths instead of creating nodes for them.
//
// Parameters:
//   - g: target graph to populate with nodes
//   - cfg: resolved writ configuration
//   - reg: action registry for node creation
//
// Returns:
//   - []string: manifest source paths discovered during tree walk
//   - error: tree build or node creation error
func BuildTree(g *op.Graph, cfg *Config, reg *op.ReceiverRegistry) (manifests []string, err error) {

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

	manifests = populateGraphNodes(g, result.Files, reg)
	recordCollisions(g, result.Collisions)
	return manifests, nil
}

// populateGraphNodes converts file entries into graph nodes on the given graph.
// Multi-op pipelines (e.g., ["decrypt", "render", "copy"]) become node chains.
// Manifest files are collected and returned instead of becoming nodes.
//
// Parameters:
//   - g: target graph to populate
//   - files: file entries from tree build
//   - reg: action registry for node creation
//
// Returns:
//   - []string: manifest source paths
func populateGraphNodes(g *op.Graph, files []*tree.FileEntry, reg *op.ReceiverRegistry) []string { //nolint:gocognit

	var manifests []string

	for _, f := range files {
		actions := f.Operations

		// Collect manifest files instead of creating nodes
		if len(actions) == 1 && actions[0] == "manifest.resolve" {
			manifests = append(manifests, f.Source)
			continue
		}

		if len(actions) == 1 {
			// Single operation — single node
			node := &op.Node{
				ID:     f.ID,
				Receiver: actions[0],
				Status: op.StatusPending,
				Origin: f.Project,
				Layer:  f.Layer,
			}
			node.SetSlot("source", op.ImmediateValue{Value: f.Source})
			node.SetSlot("path", op.ImmediateValue{Value: f.Target})
			if f.Mode != 0 {
				node.SetSlot("mode", op.ImmediateValue{Value: f.Mode})
			}
			g.AddNode(node)
		} else {
			// Multi-action pipeline → node chain
			var prevNode *op.Node
			for i, action := range actions {
				isLast := i == len(actions)-1
				nodeID := f.ID
				if !isLast {
					nodeID = f.ID + ":" + action
				}

				node := &op.Node{
					ID:     nodeID,
					Receiver: action,
					Status: op.StatusPending,
					Origin: f.Project,
					Layer:  f.Layer,
				}
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
					g.Edges = append(g.Edges, op.Edge{
						From: prevNode.ID, To: node.ID,
					})
				}
				prevNode = node
			}
		}
	}

	return manifests
}

// recordCollisions copies tree collisions into the graph as planning metadata.
//
// Parameters:
//   - g: target graph
//   - collisions: collisions from the unified tree build
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

// ConfigureEngine creates and configures an execution engine for a graph.
// The targetRoot parameter specifies the executor's root directory, allowing
// each scope's graph to execute against its own target (e.g., "/" for system,
// "$HOME" for home).
//
// Parameters:
//   - cfg: resolved writ configuration
//   - targetRoot: root directory for this executor (overrides cfg.TargetRoot)
//
// Returns:
//   - *op.GraphExecutor: configured executor
//   - error: configuration error
func ConfigureEngine(cfg *Config, targetRoot string) (*op.GraphExecutor, error) {

	// Build engine data
	engineData := graphBuiltinTemplateData(cfg.SegmentMap())
	for k, v := range cfg.TemplateData {
		engineData[k] = v
	}

	// Set up SOPS client
	sopsClient, _ := sops.NewClient(cfg.SourceRoot) //nolint:errcheck // nil when no .sops.yaml found

	// Create engine
	engine, err := op.NewGraphExecutor("writ", op.Options{
		Root:       targetRoot,
		SopsClient: sopsClient,
		Data:       engineData,
	})
	if err != nil {
		return nil, err
	}

	return engine, nil
}

// graphBuiltinTemplateData returns the built-in template variables for graph building.
func graphBuiltinTemplateData(segments map[string]string) map[string]any {

	data := make(map[string]any)

	// Add segments
	for k, v := range segments {
		data[k] = v
	}

	// Add common builtins
	data["os"] = runtime.GOOS
	data["arch"] = runtime.GOARCH

	return data
}

// scopeLayers returns the unique layer names that contribute to a given scope.
// Layers are returned in source order (base → team → personal).
//
// Parameters:
//   - sources: all layer sources
//   - scope: target scope name to filter by ("System" or "Home")
//
// Returns:
//   - []string: unique layer names in source order
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

// resolveManifests delegates manifest files to the planner for package resolution.
//
// Parameters:
//   - g: target graph to add package nodes to
//   - planner: lore planner (nil skips resolution)
//   - manifests: manifest source paths
//
// Returns:
//   - error: manifest resolution error
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

// ----------------------------------------------------------------------------
// DeployGraphBuilder
// ----------------------------------------------------------------------------

// DeployGraphBuilder builds execution graphs for deploy operations.
type DeployGraphBuilder struct {
	config  *Config
	reg     *op.ReceiverRegistry
	Planner *lore.Planner // nil means skip manifest resolution
}

// NewDeployGraphBuilder creates a new deploy graph builder.
//
// Parameters:
//   - cfg: resolved deploy configuration
//   - reg: action registry with registered actions
//
// Returns:
//   - *DeployGraphBuilder: configured builder
func NewDeployGraphBuilder(cfg *DeployConfig, reg *op.ReceiverRegistry) *DeployGraphBuilder {
	return &DeployGraphBuilder{config: &cfg.Config, reg: reg}
}

// Build creates execution graphs for a deploy operation.
// In single-source mode, returns one graph. In multi-source mode, builds a
// unified tree for cross-scope collision detection, then partitions winning
// entries by target scope and creates one graph per populated scope.
//
// Returns:
//   - []*op.Graph: one graph per target scope
//   - error: tree build or graph creation error
func (b *DeployGraphBuilder) Build() ([]*op.Graph, error) {

	// Build unified tree (all sources, cross-scope collision detection)
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

	// Single-source mode: one graph, no scope partitioning
	if len(b.config.LayerSources) == 0 {
		g := NewGraph(b.config)
		manifests := populateGraphNodes(g, result.Files, b.reg)
		recordCollisions(g, result.Collisions)
		if err := resolveManifests(g, b.Planner, manifests); err != nil {
			return nil, err
		}
		return []*op.Graph{g}, nil
	}

	// Multi-source mode: partition winning entries by target scope
	filesByScope := make(map[string][]*tree.FileEntry)
	for _, f := range result.Files {
		filesByScope[f.TargetName] = append(filesByScope[f.TargetName], f)
	}

	// Determine target root per scope from sources
	scopeTargetRoots := make(map[string]string)
	for _, src := range b.config.LayerSources {
		scopeTargetRoots[src.TargetName] = src.TargetRoot
	}

	// Create one graph per populated scope (deterministic order)
	scopes := sortedKeys(filesByScope)
	graphs := make([]*op.Graph, 0, len(scopes))
	for _, scope := range scopes {
		files := filesByScope[scope]
		targetRoot := scopeTargetRoots[scope]

		g := NewScopedGraph(b.config, strings.ToLower(scope), targetRoot)
		g.Provenance.Layers = scopeLayers(b.config.LayerSources, scope)

		manifests := populateGraphNodes(g, files, b.reg)
		recordCollisions(g, result.Collisions)

		if err := resolveManifests(g, b.Planner, manifests); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
	}

	return graphs, nil
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string][]*tree.FileEntry) []string {

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ----------------------------------------------------------------------------
// DecommissionGraphBuilder
// ----------------------------------------------------------------------------

// DecommissionGraphBuilder builds execution graphs for decommission operations.
type DecommissionGraphBuilder struct {
	config *Config
	reg    *op.ReceiverRegistry
	view   *execution.StateView
	force  bool
}

// NewDecommissionGraphBuilder creates a new decommission graph builder.
func NewDecommissionGraphBuilder(cfg *DecommissionConfig, view *execution.StateView, reg *op.ReceiverRegistry) *DecommissionGraphBuilder {
	return &DecommissionGraphBuilder{
		config: &cfg.Config,
		reg:    reg,
		view:   view,
		force:  cfg.Force,
	}
}

// Build creates an execution graph for a decommission operation.
func (b *DecommissionGraphBuilder) Build() (*op.Graph, error) {

	// Create the graph
	g := NewGraph(b.config)
	g.Provenance.TargetRoot = b.view.Files.Root

	// Build project set for filtering
	projects := projectSet(b.config.Projects)

	// Convert state entries to removal nodes
	for relTarget, entry := range b.view.Files.Entries {
		// Filter by project if specified
		if len(projects) > 0 && !projects[entry.Project] {
			continue
		}

		// Determine operation: unlink for symlinks, remove for copied files
		action := "file.unlink"
		if entry.IsCopied() {
			action = "file.remove"
		}

		target := filepath.Join(b.view.Files.Root, relTarget)
		node := &op.Node{
			ID:     relTarget,
			Receiver: action,
			Status: op.StatusPending,
			Origin: entry.Project,
			Layer:  entry.Layer,
		}
		node.SetSlot("source", op.ImmediateValue{Value: entry.Source})
		node.SetSlot("path", op.ImmediateValue{Value: target})

		g.AddNode(node)
	}

	return g, nil
}

// ----------------------------------------------------------------------------
// UpgradeGraphBuilder
// ----------------------------------------------------------------------------

// UpgradeGraphBuilder builds execution graphs for upgrade operations.
type UpgradeGraphBuilder struct {
	config *Config
	view   *execution.StateView
	force  bool
}

// NewUpgradeGraphBuilder creates a new upgrade graph builder.
func NewUpgradeGraphBuilder(cfg *UpgradeConfig, view *execution.StateView) *UpgradeGraphBuilder {
	return &UpgradeGraphBuilder{
		config: &cfg.Config,
		view:   view,
		force:  cfg.Force,
	}
}

// Build creates an execution graph for an upgrade operation.
func (b *UpgradeGraphBuilder) Build() (*op.Graph, error) {
	// TODO: implement upgrade graph building
	return nil, fmt.Errorf("upgrade graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// ReconcileGraphBuilder
// ----------------------------------------------------------------------------

// ReconcileGraphBuilder builds execution graphs for reconcile operations.
type ReconcileGraphBuilder struct {
	config *Config
}

// NewReconcileGraphBuilder creates a new reconcile graph builder.
func NewReconcileGraphBuilder(cfg *ReconcileConfig) *ReconcileGraphBuilder {
	return &ReconcileGraphBuilder{config: &cfg.Config}
}

// Build creates an execution graph for a reconcile operation.
func (b *ReconcileGraphBuilder) Build() (*op.Graph, error) {
	// TODO: implement reconcile graph building
	return nil, fmt.Errorf("reconcile graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// AdoptGraphBuilder
// ----------------------------------------------------------------------------

// AdoptGraphBuilder builds execution graphs for adopt operations.
type AdoptGraphBuilder struct {
	config    *Config
	files     []string
	layer     string
	layerPath string
	project   string
}

// NewAdoptGraphBuilder creates a new adopt graph builder.
func NewAdoptGraphBuilder(cfg *AdoptConfig) *AdoptGraphBuilder {
	return &AdoptGraphBuilder{
		config:    &cfg.Config,
		files:     cfg.Files,
		layer:     cfg.Layer,
		layerPath: cfg.LayerPath,
		project:   cfg.Project,
	}
}

// Build creates an execution graph for an adopt operation.
func (b *AdoptGraphBuilder) Build() (*op.Graph, error) {
	// TODO: implement adopt graph building
	return nil, fmt.Errorf("adopt graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// MigrateGraphBuilder
// ----------------------------------------------------------------------------

// MigrateGraphBuilder builds execution graphs for migrate operations.
type MigrateGraphBuilder struct {
	config     *Config
	sourcePath string
}

// NewMigrateGraphBuilder creates a new migrate graph builder.
func NewMigrateGraphBuilder(cfg *Config, sourcePath string) *MigrateGraphBuilder {
	return &MigrateGraphBuilder{
		config:     cfg,
		sourcePath: sourcePath,
	}
}

// Build creates an execution graph for a migrate operation.
func (b *MigrateGraphBuilder) Build() (*op.Graph, error) {
	// TODO: implement migrate graph building
	return nil, fmt.Errorf("migrate graph building not yet implemented")
}
